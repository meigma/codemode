# Contributing

Contributions should preserve CodeMode's public package boundaries, typed capability binding, per-call authorization, bounded Starlark execution, and exact three-tool MCP surface.

Report suspected vulnerabilities through the private process in [SECURITY.md](SECURITY.md), not through public GitHub channels.

## Report a bug

Open a GitHub issue for non-security bugs. Include:

- the CodeMode commit or version you used
- your Go version and relevant environment details
- a minimal capability registration or Starlark program that reproduces the problem
- the steps you followed
- the expected and actual behavior
- relevant error classifications or logs with credentials and sensitive arguments removed

## Write documentation

User documentation lives under `docs/docs/` and follows the Diátaxis layout:

- `tutorials/` teaches through a complete, guided path.
- `how-to/` gives task-focused procedures.
- `reference/` describes the public Go API and MCP tool contracts.
- `explanation/` describes the security model and other design context.

Keep the root [README](README.md) concise and link to the detailed pages instead of duplicating them. Use plain language, keep examples consistent with shipped APIs, and add compile-checked Go examples to [`example_test.go`](example_test.go) or [`mcpserver/example_test.go`](mcpserver/example_test.go) when they document executable integration code.

## Prepare a change

1. Install the pinned tools:

   ```sh
   mise install
   ```

2. Keep the change focused.
3. When a public contract changes, add or update behavior tests.
4. When behavior visible through the root, `authz`, or `mcpserver` packages changes, update the user documentation.
5. When compile-checked examples or documentation change, validate them directly:

   ```sh
   go test ./... -count=1
   moon run docs:build
   ```

   `go test` compiles and runs the Go examples with the package tests. `docs:build` runs the strict MkDocs build with the locked documentation environment.
6. Before requesting review, run the complete repository check:

   ```sh
   moon run root:check
   ```

   `root:check` includes formatting, linting, compilation, the official MCP secure-loop test, the race detector, and the documentation build.

For focused security-boundary investigation, run:

```sh
moon run root:mcp-smoke
moon run root:race
```

`root:mcp-smoke` runs `TestActualMCPSecureLoop` through the official in-memory MCP transport. `root:race` runs all Go packages with the race detector and disables cached test results.

## Submit a pull request

- Explain the user-visible or security-relevant behavior that changed.
- Keep capability metadata, authorization behavior, MCP output, tests, and documentation consistent.
- Do not expose credentials, policy diagnostics, handler failures, Starlark source, or arguments in client-facing errors.
- Ensure `moon run root:check` passes before requesting review.
- Use a Conventional Commit subject, for example `feat(mcpserver): add trusted invocation resolver` or `fix(authz): stop dispatch after denial`.

Release Please uses Conventional Commit subjects to prepare source version changes, tags, changelog entries, and draft GitHub releases. Use the commit type that matches the change; routine documentation, CI, and maintenance changes should use the corresponding non-feature type.
