# CodeMode

CodeMode is a source-only Go library for exposing typed Go capabilities to Model Context Protocol (MCP) clients through bounded Starlark programs. A host registers capabilities, applies a static deployment filter, and authorizes each validated native call.

CodeMode does not start a transport, authenticate callers, open listeners, or manage shutdown. The host application owns those responsibilities. The module has no executable entry point and no generic downstream MCP proxy.

## MCP tools

The `mcpserver` package constructs an official Go SDK server with exactly three tools:

| Tool | Input | Successful structured output |
| --- | --- | --- |
| `search_api` | `{"query": string}` | A bounded array of enabled capability names, generated signatures, and summaries |
| `describe_api` | `{"name": string}` for one exact capability name | The capability name, signature, summary, description, and supported input and output fields |
| `execute` | `{"source": string}` containing one bounded Starlark program | `{"result": <main return value>}` |

Every tool resolves an authenticated `authz.Subject` from host-owned Go context before discovery or execution. Tool arguments and MCP `_meta` are untrusted and cannot supply identity, credentials, limits, modules, or capability visibility.

Each `execute` call uses a fresh interpreter. The program must define a zero-argument `main()` function. CodeMode returns only the converted value from `main`; it does not separately expose printed text, globals, or intermediate values.

## Prerequisites

- Go 1.26.6, as declared by `go.mod`
- [mise](https://mise.jdx.dev) for repository development; `mise install` installs the pinned Go, Python, `golangci-lint`, uv, Moon, GitHub CLI, and mockery versions

CodeMode has no released version yet. To evaluate the current `master` branch from another Go module, run:

```sh
go get github.com/meigma/codemode@master
```

## Integrate CodeMode

The following outline assembles a root CodeMode server and the MCP adapter. Replace the example policy and context plumbing with host-specific authentication and authorization.

```go
package integration

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/mcpserver"
)

type lookupInput struct {
	Key string `json:"key"`
}

type lookupOutput struct {
	Key string `json:"key"`
}

type policy struct{}

func (policy) Authorize(_ context.Context, input authz.AuthorizationInput) error {
	if input.CapabilityID != "records.entry.lookup" {
		return authz.ErrDenied
	}
	return nil
}

type subjectKey struct{}

type subjectResolver struct{}

func (subjectResolver) Resolve(ctx context.Context) (authz.Subject, error) {
	subject, ok := ctx.Value(subjectKey{}).(authz.Subject)
	if !ok || subject.ID == "" {
		return authz.Subject{}, codemode.ErrUnauthenticated
	}
	return subject, nil
}

// withSubject is called by trusted host middleware after authentication.
func withSubject(ctx context.Context, subject authz.Subject) context.Context {
	return context.WithValue(ctx, subjectKey{}, subject)
}

func newMCPServer() (*mcp.Server, error) {
	builder := codemode.New(codemode.Options{
		Authorizer: policy{},
		Limits:     codemode.DefaultLimits(),
	})

	err := codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
		ID:          "records.entry.lookup",
		Name:        "records.lookup",
		Summary:     "Look up one record by key.",
		Description: "Returns the record identified by key.",
		Handler: func(_ context.Context, _ authz.Subject, input lookupInput) (lookupOutput, error) {
			return lookupOutput{Key: input.Key}, nil
		},
	})
	if err != nil {
		return nil, err
	}

	root, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return mcpserver.New(root, subjectResolver{})
}
```

Connect the returned `*mcp.Server` to a transport only after the host has authenticated the request and placed the non-secret subject in trusted context. The host must also manage listener lifecycle, request cancellation, and shutdown.

## Security boundary

Capability input is converted to the registered Go type before authorization. The authorizer receives the trusted subject, stable capability ID, exposed capability name, and a fresh JSON-shaped copy of the validated arguments. A recognized denial prevents the handler from running. Capabilities disabled at build time are absent from search, description, and execution.

The Starlark runtime bounds source bytes, interpreter steps, elapsed time, native calls, value depth, result bytes, search query bytes, and search results. Module loading is disabled. These in-process controls reduce the reachable surface; they are not a hard isolation boundary between mutually untrusted tenants. Authorizers and handlers run as Go code in the host process and must honor context cancellation. See [SECURITY.md](SECURITY.md) for vulnerability reporting and containment limits.

## Repository checks

Moon is the task entry point after `mise install`:

```sh
moon run root:mcp-smoke
moon run root:race
moon run root:check
```

`root:mcp-smoke` runs the official MCP secure-loop test. `root:race` runs all Go packages with the race detector. `root:check` depends on format, lint, build, MCP smoke, race, and `docs:build`.

CI runs Moon's affected-task graph, including `root:check` and the standalone `root:test` task:

```sh
moon ci --summary minimal
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local checks and pull request expectations.

## Security

Report vulnerabilities through the private process in [SECURITY.md](SECURITY.md), not through a public issue.

## License

CodeMode is licensed under the [Apache License 2.0](LICENSE).
