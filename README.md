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

`authz.AllowAll()` is deliberate in the simple examples and permits every validated invocation. Production hosts normally supply an authorization policy.

## Documentation

- [Documentation home](docs/docs/index.md)
- [First-server tutorial](docs/docs/tutorials/first-server.md)
- [Disable capabilities for a deployment](docs/docs/how-to/disable-capabilities.md)
- [Public Go API](docs/docs/reference/public-api.md)
- [MCP tools](docs/docs/reference/mcp-tools.md)
- [Security model](docs/docs/explanation/security-model.md)

## Security boundary

Each `execute` request gets a fresh bounded interpreter. The submitted program must define a zero-argument `main()`, and only its final converted value crosses the execution boundary. Prints, globals, and intermediate values do not.

Capabilities are bound and canonicalized before authorization, and authorization completes before handler dispatch. Trusted subjects come from typed, host-owned Go context through `mcpserver.InvocationResolver`; tool arguments, Starlark source, and MCP `_meta` are not trusted identity or credential sources.

CodeMode's in-process limits are not a hard tenant or heap boundary. Go authorizers and handlers must honor context cancellation because CodeMode cannot forcibly interrupt blocking Go code. See the [security model](docs/docs/explanation/security-model.md) and [SECURITY.md](SECURITY.md) for the full boundary and vulnerability-reporting process.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the documentation layout, repository checks, and pull request expectations.

## License

CodeMode is licensed under the [Apache License 2.0](LICENSE).
