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

## Prepare a change

1. Install the pinned tools:

   ```sh
   mise install
   ```

2. Keep the change focused. Add or update behavior tests when a public contract changes. Update the user documentation when behavior visible through the root, `authz`, or `mcpserver` packages changes.
3. Before requesting review, run:

   ```sh
   moon run root:check
   ```

   `root:check` includes formatting, linting, compilation, the official MCP secure-loop test, the race detector, and the documentation build.

For focused investigation, the security-boundary checks are available separately:

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
