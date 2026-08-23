# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- CodeMode authors new code-native MCP servers; it never imports, proxies, translates, or calls downstream MCP servers. The client surface is exactly `search_api`, `describe_api`, and `execute`.
- Starlark receives namespaced native Go capabilities, no ambient host access, and a fresh thread per execution. Only the required zero-argument `main()` result crosses MCP.
- Trusted subject and credentials stay in Go context. Static filtering removes disabled capabilities everywhere; canonical keyword binding precedes fail-closed authorization, which precedes every handler call.
- The initial package boundaries are root `codemode`, public `authz`, `mcpserver`, `internal/binding`, `internal/catalog`, and `internal/execution`.
- `.journal/001/ARCHITECTURE.md` is the settled architecture. `.journal/001/IMPLEMENTATION_PLAN.md` starts with a disposable actual-MCP spike and then builds the secure MVP; use those artifacts together.
- Rego, worker isolation, richer binding/search, observability, fuzzing, benchmarks, and Testcontainers require the evidence gates in the architecture and implementation plan.
