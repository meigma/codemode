# CodeMode

CodeMode is a source-only Go library for exposing typed Go capabilities to Model Context Protocol (MCP) clients through bounded Starlark programs. A host registers capabilities, selects the deployment-wide surface, and authorizes each validated native call.

The `mcpserver` adapter uses the official MCP Go SDK and exposes exactly `search_api`, `describe_api`, and `execute`. CodeMode does not authenticate callers, start transports or listeners, or own request cancellation and shutdown. Those responsibilities remain with the host.

## Install

CodeMode has not published a release. To evaluate the current `master` branch from a Go module, run:

```sh
go get github.com/meigma/codemode@master
```

The module currently requires Go 1.26.6.

## Get started

Follow [Build your first CodeMode server](docs/docs/tutorials/first-server.md) to register the typed `records.entry.lookup` capability, expose it as `records.lookup`, connect the official in-memory MCP transport, and call all three tools.

The repository also contains shorter, compile-checked examples:

- [`example_test.go`](example_test.go) — typed capability registration, explicit `authz.AllowAll()`, default limits, and direct execution
- [`mcpserver/example_test.go`](mcpserver/example_test.go) — trusted typed-context resolution and the official in-memory MCP transport

`authz.AllowAll()` is deliberate in the simple examples and permits every native call whose arguments bind successfully. Use `authz.AllowAll()` only when every resolved subject may call every enabled capability; otherwise supply an `authz.Authorizer`.

The final host binary must call `codemode.ServeWorkerAndExit()` as the first
statement of `main`, before flag parsing or any other setup. Test binaries that
call `Builder.Build` must do the same from `TestMain` before `m.Run`.

## Documentation

- [Documentation home](docs/docs/index.md)
- [First-server tutorial](docs/docs/tutorials/first-server.md)
- [Disable capabilities for a deployment](docs/docs/how-to/disable-capabilities.md)
- [Public Go API](docs/docs/reference/public-api.md)
- [MCP tools](docs/docs/reference/mcp-tools.md)
- [Security model](docs/docs/explanation/security-model.md)

## Security boundary

Each `execute` request runs Starlark in a fresh worker process created by re-executing
the host binary. The submitted program must define a zero-argument `main()`,
and only its final converted value is exposed to the caller. Printed text,
globals, and interpreter-local intermediate values are not returned. Each
native-call argument map, native result, and final value is independently
subject to `MaxValueDepth` and `MaxValueBytes`. Successful native-result value
bodies also consume the fresh request-scoped `MaxIntermediateValueBytes`
budget.

The worker binds keyword arguments first. The parent then rebinds them to the
registered Go input, creates a fresh canonical authorization map, authorizes
the call, and dispatches the handler. The parent converts the handler output to
a process-neutral value, and the worker converts that value to Starlark.
Trusted subjects come
from typed, host-owned Go context through `mcpserver.InvocationResolver`; tool
arguments, Starlark source, and MCP `_meta` are not trusted identity or
credential sources.

The worker process lets CodeMode kill Starlark when an execution deadline or
request cancellation occurs. Authorizers and handlers remain ordinary Go code
in the host process; CodeMode cannot forcibly stop them after dispatch. Worker
processes also have no operating-system CPU or memory quota. See the
[security model](docs/docs/explanation/security-model.md) and
[SECURITY.md](SECURITY.md) for the full boundary and
vulnerability-reporting process.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the documentation layout, repository checks, and pull request expectations.

## License

CodeMode is licensed under the [Apache License 2.0](LICENSE).
