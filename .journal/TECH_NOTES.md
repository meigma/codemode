# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- CodeMode authors new code-native MCP servers; it never imports, proxies, translates, or calls downstream MCP servers. The client surface is exactly `search_api`, `describe_api`, and `execute`.
- The secure MVP and source-only repository cutover are complete on `master`. The repository ships a Go library, not an executable, binary release, or OCI image.
- The shipped binding matrix is intentionally narrow: non-pointer input structs with required `string` and optional `*int64` fields; non-pointer output structs with `string`, `int64`, `bool`, and `float64` fields.
- A `Builder` is single-threaded and one-shot. It validates every registration before static filtering, compiles binding and namespace data, and returns an immutable concurrency-safe `Server`.
- Stable `CapabilityID` values drive deployment and authorization policy; dotted `CapabilityName` values drive discovery and generated Starlark namespaces.
- Trusted subjects stay in host-owned typed Go context. The `mcpserver.InvocationResolver` runs before every tool operation and must ignore arguments, source, MCP `_meta`, and other client-controlled metadata.
- Canonical keyword binding precedes fail-closed authorization, which precedes every native handler call. Disabled capabilities are absent from discovery, description, generated namespaces, and execution.
- Every execution gets a fresh restricted Starlark thread and only zero-argument `main()`'s final converted value crosses MCP. Prints, globals, and intermediate expressions do not.
- Execution budgets and Starlark cancellation are in-process restrictions, not hard tenant or heap isolation. Go authorizers and handlers must honor context cancellation; use process or container isolation for mutually untrusted tenants.
- The substantive packages are root `codemode`, public `authz`, `mcpserver`, `internal/binding`, `internal/catalog`, and `internal/execution`, plus generated adapter mocks.
- `.journal/001/ARCHITECTURE.md` is the settled architecture; `.journal/001/IMPLEMENTATION_PLAN.md` is complete. Session 002's summary records the merged implementation and verification.
- Rego, worker isolation, richer binding/search, observability, fuzzing, benchmarks, and Testcontainers remain evidence-gated. Add them only when observed product needs justify the extra surface.
