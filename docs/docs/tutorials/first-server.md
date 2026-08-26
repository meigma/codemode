---
title: Build your first CodeMode server
description: Register a typed Go capability and expose it through a real stdio MCP server.
---

# Build your first CodeMode server

This tutorial builds a stdio MCP server with one typed capability. After you add
the server to an agent, the agent can discover and call `records.lookup`.

## Prerequisites

The repository currently requires Go 1.26.6.

This tutorial builds a local, single-user stdio server. Process ownership is
the authentication boundary, and every capability is allowed for that one
subject.

## Create a module

CodeMode has not published a release. Create a module, add CodeMode from
`master`, and add the official MCP Go SDK. The SDK command resolves to its
latest release:

```sh
mkdir codemode-first-server
cd codemode-first-server
go mod init example.com/codemode-first-server
go get github.com/meigma/codemode@master github.com/modelcontextprotocol/go-sdk/mcp
```

## Add the server

Create `main.go`:

```go
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/mcpserver"
)

type lookupInput struct {
	Key   string `json:"key"`
	Limit *int64 `json:"limit,omitempty"`
}

type lookupOutput struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

func lookup(_ context.Context, _ authz.Subject, in lookupInput) (lookupOutput, error) {
	count := int64(0)
	if in.Limit != nil {
		count = *in.Limit
	}
	return lookupOutput{Key: in.Key, Count: count}, nil
}

func main() {
	codemode.ServeWorkerAndExit()

	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})

	codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
		Name:        "records.lookup",
		Summary:     "Look up one record by key.",
		SearchTerms: []string{"fetch entry"},
		Handler:     lookup,
	})

	server, err := builder.Build()
	if err != nil {
		log.Fatal(err)
	}

	srv, err := mcpserver.New(server, mcpserver.StaticSubject(authz.Subject{ID: "local"}))
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(srv.Run(context.Background(), &mcp.StdioTransport{}))
}
```

`authz.AllowAll()` is deliberate. CodeMode has no default authorizer, so
disabling authorization requires an explicit decision.

`StaticSubject` matches this single-user stdio deployment: possession of the
process is the authentication boundary. A multi-user host must not use
`StaticSubject`. Authenticate each request, store the resulting identity with
`authz.WithSubject`, and use `mcpserver.ContextSubject`.

`Build` supplies bounded defaults for each zero-valued `Limits` field. It also
reports all invalid registrations together. An omitted capability `ID` defaults
to `Name`; set `ID` explicitly before writing authorization policy or deployment
filters that must survive a capability rename. `SearchTerms` supplies
discovery-only task and resource vocabulary; it does not create callable names.
An omitted `Description` defaults to `Summary`.

## Build and configure the server

Clean up the module metadata, then build the server:

```sh
go mod tidy
go build -o codemode-first-server .
```

The binary speaks MCP over stdin/stdout and is not useful to run directly.
Configure it in an agent as shown next.

Agents that use the `mcpServers` configuration shape accept:

```json
{
  "mcpServers": {
    "codemode-first-server": {
      "command": "/absolute/path/to/codemode-first-server"
    }
  }
}
```

Restart or reload the agent's MCP servers.

Ask the agent to search for a capability that can `fetch entry`. This phrase
comes from `SearchTerms`, so the agent can discover `records.lookup` even though
the phrase does not appear in its name or summary.

Ask the agent to call `records.lookup` with `key="alpha"` and `limit=2`. The
agent can use `search_api`, `describe_api`, and `execute`. The final structured
result is equivalent to:

```json
{"result":{"count":2,"key":"alpha"}}
```

`ServeWorkerAndExit` must be the first statement in `main`, before flag parsing,
credential loading, client construction, or other setup. `execute` first binds
the keyword arguments in the worker process. The parent rebinds them to `lookupInput`,
creates a fresh canonical authorization map, authorizes the native call, and
then dispatches `lookup`. The parent converts the handler output to a
process-neutral value, and the worker converts it to Starlark. Each `execute`
call runs a fresh bounded interpreter in a re-executed worker. Only the
final converted value returned by a zero-argument `main()` is exposed in the
successful MCP result; printed text, globals, and interpreter-local intermediate
values are not returned. Each native-call argument map, native result, and final
value is independently subject to `MaxValueDepth` and `MaxValueBytes`.
Successful native-result value bodies also consume the fresh request-scoped
`MaxIntermediateValueBytes` budget.

For shorter compile-checked forms, see the [typed registration
example](https://github.com/meigma/codemode/blob/master/example_test.go) and
[official transport
example](https://github.com/meigma/codemode/blob/master/mcpserver/example_test.go).
