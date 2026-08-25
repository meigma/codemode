---
title: Build your first CodeMode server
description: Register a typed Go capability and call it through the official MCP in-memory transport.
---

# Build your first CodeMode server

This tutorial builds an MCP server with one typed capability. You will connect an official MCP client over the SDK's in-memory transport and call `search_api`, `describe_api`, and `execute`.

## Create a module

CodeMode has not published a release. To follow the tutorial against the current `master` branch, create a module and add CodeMode and the official MCP Go SDK:

```sh
mkdir codemode-first-server
cd codemode-first-server
go mod init example.com/codemode-first-server
go get github.com/meigma/codemode@master github.com/modelcontextprotocol/go-sdk/mcp
```

The repository currently requires Go 1.26.6.

## Add the server

Create `main.go` with the following program:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/mcpserver"
)

// invocationContextKey is the private key for trusted invocation context.
type invocationContextKey struct{}

// lookupInput is the records.lookup input contract.
type lookupInput struct {
	// Key is the required record identifier.
	Key string `json:"key"`

	// Limit is the optional result limit.
	Limit *int64 `json:"limit,omitempty"`
}

// lookupOutput is the records.lookup output contract.
type lookupOutput struct {
	// Key is the record identifier returned by the handler.
	Key string `json:"key"`

	// Count is the limit observed by the handler.
	Count int64 `json:"count"`
}

// subjectResolver reads the authenticated subject from host-owned Go context.
type subjectResolver struct{}

// Resolve returns the authenticated subject stored by the host.
func (subjectResolver) Resolve(ctx context.Context) (authz.Subject, error) {
	subject, ok := ctx.Value(invocationContextKey{}).(authz.Subject)
	if !ok || subject.ID == "" {
		return authz.Subject{}, codemode.ErrUnauthenticated
	}
	return subject, nil
}

// withSubject stores a non-secret authenticated subject in trusted host context.
func withSubject(ctx context.Context, subject authz.Subject) context.Context {
	return context.WithValue(ctx, invocationContextKey{}, subject)
}

// lookupRecords returns a deterministic result for the tutorial capability.
func lookupRecords(_ context.Context, _ authz.Subject, input lookupInput) (lookupOutput, error) {
	count := int64(0)
	if input.Limit != nil {
		count = *input.Limit
	}
	return lookupOutput{Key: input.Key, Count: count}, nil
}

// callTool calls one MCP tool and prints its structured output.
func callTool(ctx context.Context, session *mcp.ClientSession, params *mcp.CallToolParams) error {
	result, err := session.CallTool(ctx, params)
	if err != nil {
		return fmt.Errorf("call %s: %w", params.Name, err)
	}
	if result.IsError {
		return fmt.Errorf("%s returned a tool error: %v", params.Name, result.Content)
	}

	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return fmt.Errorf("encode %s result: %w", params.Name, err)
	}
	fmt.Printf("%s: %s\n", params.Name, encoded)
	return nil
}

// run builds the server, connects the in-memory transport, and calls each tool.
func run(ctx context.Context) error {
	builder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     codemode.DefaultLimits(),
	})

	err := codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
		ID:          "records.entry.lookup",
		Name:        "records.lookup",
		Summary:     "Look up one record by key.",
		Description: "Returns one deterministic record for the supplied key.",
		Handler:     lookupRecords,
	})
	if err != nil {
		return fmt.Errorf("register capability: %w", err)
	}

	server, err := builder.Build()
	if err != nil {
		return fmt.Errorf("build CodeMode server: %w", err)
	}
	mcpServer, err := mcpserver.New(server, subjectResolver{})
	if err != nil {
		return fmt.Errorf("build MCP server: %w", err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverContext := withSubject(ctx, authz.Subject{ID: "example-user"})
	serverSession, err := mcpServer.Connect(serverContext, serverTransport, nil)
	if err != nil {
		return fmt.Errorf("connect MCP server: %w", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "codemode-first-server",
		Version: "tutorial",
	}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return fmt.Errorf("connect MCP client: %w", err)
	}
	defer clientSession.Close()

	if err := callTool(ctx, clientSession, &mcp.CallToolParams{
		Name:      "search_api",
		Arguments: map[string]any{"query": "record"},
	}); err != nil {
		return err
	}
	if err := callTool(ctx, clientSession, &mcp.CallToolParams{
		Name:      "describe_api",
		Arguments: map[string]any{"name": "records.lookup"},
	}); err != nil {
		return err
	}
	return callTool(ctx, clientSession, &mcp.CallToolParams{
		Name: "execute",
		Arguments: map[string]any{
			"source": `def main():
    return records.lookup(key="alpha", limit=2)
`,
		},
	})
}

// main serves re-executed workers before starting the ordinary host.
func main() {
	codemode.ServeWorkerAndExit()
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

`authz.AllowAll()` is deliberate in this small example: it makes the authorization decision explicit without adding application policy, and it permits every validated invocation. Do not use it in a production host unless that is the intended policy. Production hosts normally supply an `authz.Authorizer` that evaluates the trusted subject, stable capability ID, exposed name, and canonical arguments.

The resolver reads only the typed, server-side Go context. In a production host, authentication middleware establishes that context before the MCP server handles a request. Do not accept a subject or credentials from Starlark source, tool arguments, or MCP `_meta`.

## Run the server

Clean up the module metadata, then run the program:

```sh
go mod tidy
go run .
```

The program prints structured results for all three tools. The final line contains an `execute` result equivalent to:

```json
{"result":{"count":2,"key":"alpha"}}
```

`execute` first binds and canonicalizes the keyword arguments in the parent, then authorizes the native call, and then dispatches `lookupRecords` in the parent. Each `execute` call runs a fresh bounded interpreter in a re-executed worker process. Only the final converted value returned by a zero-argument `main()` crosses the worker boundary; printed text, globals, and intermediate values do not.

`ServeWorkerAndExit` must be the first statement in `main`, before flag parsing, credential loading, client construction, or other application setup. It returns immediately in the ordinary host. In a re-executed worker, it serves one private probe or execution request and terminates the process.

The in-memory MCP transport keeps the client and server in one parent process; Starlark still runs in fresh worker processes. In an application, the host owns authentication, transport selection and startup, listeners, request cancellation, and shutdown. A Go authorizer or handler that can block must honor its context. CodeMode can kill the Starlark worker, but it cannot forcibly stop Go code after parent dispatch.

For shorter compile-checked forms of this setup, see the [typed registration example](https://github.com/meigma/codemode/blob/master/example_test.go) and [official transport example](https://github.com/meigma/codemode/blob/master/mcpserver/example_test.go).
