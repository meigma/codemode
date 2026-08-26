# CodeMode

CodeMode is a Go library for building MCP servers where the agent writes code
instead of chaining tool calls. You register plain Go functions as
capabilities. An agent discovers them, then submits a small Starlark program
that calls several capabilities, filters and combines their results in the
program, and returns one value.

Every CodeMode server exposes the same three MCP tools through the official
MCP Go SDK:

- `search_api` — ranked discovery over your capability names, summaries, and
  search terms
- `describe_api` — exact call signatures and result shapes, generated from
  your Go types
- `execute` — run one Starlark program against those capabilities

Compared to a conventional MCP server with one tool per function:

- Loops, filtering, and aggregation happen inside the program, so multi-step
  work takes one round trip and intermediate data never enters the model's
  context window.
- The schemas the agent sees are derived from your registered Go types, so
  they cannot drift from handler behavior.
- Capabilities can be disabled per deployment by stable ID without code
  changes.
- Every capability call is authorized before dispatch, fail-closed, with the
  arguments the handler will receive. The optional [`authz/rego`](docs/docs/how-to/use-rego-authorization.md)
  adapter evaluates OPA/Rego policy in-process.
- Each program runs in a fresh worker process under execution budgets, and
  only the program's final value is returned to the caller.

## Install

```sh
go get github.com/meigma/codemode@v0.1.0
```

The module requires Go 1.26.6.

## Get started

```go
func main() {
	// Serve worker mode when this binary is re-executed for a program run.
	// This must be the first statement of main.
	codemode.ServeWorkerAndExit()

	// AllowAll is an explicit choice; CodeMode has no default authorizer.
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})

	// A capability is a plain typed Go function. The schema agents see is
	// generated from lookupInput and lookupOutput.
	codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
		Name:    "records.lookup",
		Summary: "Look up one record by key.",
		Handler: lookup,
	})

	server, err := builder.Build()
	if err != nil {
		log.Fatal(err)
	}

	// StaticSubject suits a single-user stdio server; multi-user hosts
	// resolve each authenticated request with ContextSubject.
	srv, err := mcpserver.New(server, mcpserver.StaticSubject(authz.Subject{ID: "local"}))
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(srv.Run(context.Background(), &mcp.StdioTransport{}))
}
```

For the full walk-through — the input and output types, building the binary,
and adding it to an agent — follow
[Build your first CodeMode server](docs/docs/tutorials/first-server.md).
Shorter compile-checked examples: [`example_test.go`](example_test.go) and
[`mcpserver/example_test.go`](mcpserver/example_test.go).

## Documentation

- [Documentation home](docs/docs/index.md)
- [First-server tutorial](docs/docs/tutorials/first-server.md)
- [Disable capabilities for a deployment](docs/docs/how-to/disable-capabilities.md)
- [Use Rego for authorization](docs/docs/how-to/use-rego-authorization.md)
- [Public Go API](docs/docs/reference/public-api.md)
- [MCP tools](docs/docs/reference/mcp-tools.md)
- [Security model](docs/docs/explanation/security-model.md)

## How this differs from Cloudflare's Code Mode

Cloudflare's [Code Mode](https://blog.cloudflare.com/code-mode/) and CodeMode
share a thesis: models are better at writing code than at emitting tool
calls. They apply it on opposite sides of the protocol.

Cloudflare's Code Mode is agent-side. Their Agents SDK converts the tool
schemas of the MCP servers an agent already uses into a TypeScript API; the
model writes TypeScript that runs in a V8 isolate on the Workers platform,
and each call proxies back to the original servers.

CodeMode is server-side. You author the server itself: capabilities are
native typed Go functions, not wrapped remote tools, and the server runs the
agent's program in a local worker process. Because code execution is part of
the server's own contract, any MCP client gets one-round-trip composition
without a special agent framework or hosting platform. The server also
enforces its own policy: every capability call is authorized before it
dispatches, and a deployment can disable capabilities it does not want to
expose.

## Security boundary

Each `execute` request runs Starlark in a fresh worker process that CodeMode
can kill on deadline or cancellation. Arguments are bound and canonicalized
before authorization, authorization completes before handler dispatch, and the
trusted subject comes only from host-owned context — never from tool
arguments, program source, or MCP `_meta`. Execution budgets bound source
size, steps, time, native calls, and value sizes; they are not operating-system
CPU or memory quotas, and CodeMode cannot forcibly stop a dispatched Go
handler. The [security model](docs/docs/explanation/security-model.md) defines
the full boundary; see [SECURITY.md](SECURITY.md) for vulnerability reporting.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the documentation layout, repository checks, and pull request expectations.

## License

CodeMode is licensed under the [Apache License 2.0](LICENSE).
