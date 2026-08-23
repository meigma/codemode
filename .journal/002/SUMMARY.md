---
id: 002
title: Implement CodeMode secure MVP
date: 2026-08-23
status: complete
repos_touched: [codemode]
related_sessions: [001]
---

## Goal

Prove the proposed CodeMode product loop against the official MCP Go SDK, then implement the secure source-only Go library described by session 001's architecture and implementation plan. Finish with an actual MCP boundary, hardened repository automation, compile-checked examples, and public documentation grounded only in shipped behavior.

## Outcome

Goal met. A disposable spike resolved the SDK and Starlark mechanics and was deleted. Six reviewed pull requests then replaced the template application with the complete CodeMode library, capability catalog, binding layer, bounded execution engine, trusted-context MCP adapter, source-only repository automation, examples, and Diátaxis documentation. PRs #6 through #11 were squash-merged; all local worktrees were removed and local `master` was fast-forwarded to `origin/master` at `bf7ca675e19d55772c5d20e9cf1bf649ceccc90a`.

The resulting module exposes exactly `search_api`, `describe_api`, and `execute` over the official MCP SDK. It statically filters capabilities by stable ID, binds and canonicalizes arguments before fail-closed authorization, resolves trusted subjects only from host-owned typed Go context, runs each program in a fresh restricted Starlark interpreter, and returns only zero-argument `main()`'s final converted value. Actual in-memory MCP client/server tests, race tests, lint, builds, strict documentation builds, browser checks, and independent reviews passed throughout the increments.

## Key Decisions

- Spike outside the repository first -> verified current MCP SDK transport, tool-result, context, and Starlark behavior without committing speculative production structure.
- Keep one official MCP tool per operation rather than one per capability -> preserves the compact CodeMode surface and centralizes discovery, policy, and execution.
- Separate stable `CapabilityID` from model-facing dotted `CapabilityName` -> deployment policy remains stable while generated Starlark namespaces stay readable.
- Compile bindings and generated namespace data during the one-shot build -> every live `Server` is immutable, internally consistent, and safe for concurrent use.
- Bind and canonicalize before authorization, then dispatch -> policy evaluates the exact typed arguments the handler receives, and every valid native call crosses the same authorization boundary.
- Resolve subjects from host-owned typed context and ignore client arguments, source, and MCP metadata -> model-controlled data cannot establish identity or credentials.
- Treat runtime budgets as in-process restrictions, not tenant isolation -> Starlark can be cancelled, but blocking Go authorizers and handlers remain cooperative and share the host process.
- Remove executable, image, and binary-release machinery -> the repository now ships a Go library; retained automation tests the real package and actual MCP boundary instead of stale template paths.
- Document only the implemented type matrix and boundary -> avoids promising generic binding, hard isolation, transport ownership, or downstream MCP proxying that CodeMode does not provide.

## Changes

- Root `codemode` package - added typed capability registration, validated build options and limits, immutable server construction, discovery, description, and bounded program execution.
- `authz/` - added stable subjects, canonical authorization input, fail-closed errors, explicit `AllowAll`, and generated mocks.
- `internal/binding/` - added the shipped struct type matrix, canonical keyword binding, compact Starlark signatures, result conversion, and generated namespaced program data.
- `internal/catalog/` - added complete registration validation, stable-ID filtering, namespace collision checks, name-sorted discovery, and immutable catalog construction.
- `internal/execution/` - added fresh-thread Starlark execution, program validation, authorization-before-handler dispatch, cancellation, limits, panic containment, and final-value conversion.
- `mcpserver/` - added the official MCP Go SDK adapter with exactly three tools, typed-context `InvocationResolver`, coarse error projection, official in-memory transport coverage, and generated mocks.
- Root and `mcpserver/` examples - added deterministic, compile-checked registration and actual transport examples using explicit `authz.AllowAll()`.
- `docs/docs/`, `docs/mkdocs.yml`, `README.md`, and `CONTRIBUTING.md` - added the first-server tutorial, static-filter how-to, exact public API and MCP references, security-model explanation, navigation, and concise repository entry points.
- Repository automation - removed the template executable, Cobra/Viper application code, binary/image publication and scanning paths, and obsolete tool pins; retained source-only Release Please, GitHub Pages, and hardened CI with format, lint, build, race, docs, and actual MCP smoke checks.
- `LICENSE`, `SECURITY.md`, and repository metadata - established the canonical Meigma Apache-2.0 license and aligned security and repository guidance with the shipped library.

## Open Threads

- No work remains from session 001's settled implementation plan, and no session-owned branch or pull request remains open.
- Rego policy, worker isolation, richer binding and search, observability, fuzzing, benchmarks, and Testcontainers remain intentionally evidence-gated. Revisit them only when observed product needs justify the added surface.

## Lessons

- The official MCP Go SDK's in-memory transport is sufficient to test the complete resolver and tool boundary without a network listener; hosts still own connection lifecycle and shutdown.
- MCP tool-level failures and malformed JSON-RPC/input-schema failures are distinct and require separate assertions.
- Starlark thread cancellation bounds interpreter work but cannot preempt arbitrary Go authorizer or handler code; hard multi-tenant containment requires a process or container boundary.
- A copied tutorial program is a valuable acceptance check: it exposed lifecycle, output-shape, and sequential error-flow details that compile-only examples would not prove.

## References

- [PR #6 — refactor: cut over to codemode library module](https://github.com/meigma/codemode/pull/6)
- [PR #7 — feat: add capability contracts and catalog](https://github.com/meigma/codemode/pull/7)
- [PR #8 — feat: add secure execution server](https://github.com/meigma/codemode/pull/8)
- [PR #9 — feat: add MCP server adapter](https://github.com/meigma/codemode/pull/9)
- [PR #10 — chore(repo): complete source-only cutover](https://github.com/meigma/codemode/pull/10)
- [PR #11 — docs: converge public surface](https://github.com/meigma/codemode/pull/11)
- [Session 001 architecture](../001/ARCHITECTURE.md)
- [Session 001 implementation plan](../001/IMPLEMENTATION_PLAN.md)
- [Public API reference](https://github.com/meigma/codemode/blob/master/docs/docs/reference/public-api.md)
- [MCP tool reference](https://github.com/meigma/codemode/blob/master/docs/docs/reference/mcp-tools.md)
- [Security model](https://github.com/meigma/codemode/blob/master/docs/docs/explanation/security-model.md)
