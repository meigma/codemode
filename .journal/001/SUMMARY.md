---
id: 001
title: Bootstrap codemode repository
date: 2026-08-22
status: complete
repos_touched: [meigma/codemode]
related_sessions: []
---

## Goal
Create the public `meigma/codemode` repository from `meigma/template-go`, initialize its session journal, and establish a sound product and implementation direction for CodeMode.

## Outcome
The goal was met. The public repository and personal journal were created, the product scope and security boundaries were settled, and complete architecture and spike-first implementation-plan artifacts were produced. No implementation work was started or merged during this planning session.

## Key Decisions
- Build a Go framework for new code-native MCP servers, not a proxy, importer, translator, or compatibility layer for downstream MCP servers.
- Expose exactly `search_api`, `describe_api`, and `execute`; generated Starlark calls namespaced native Go capabilities and returns only `main()`'s final value.
- Use Starlark-Go initially because it provides a small embeddable capability surface; do not claim hard heap or process isolation from the in-process runtime.
- Keep authentication and credential selection in trusted Go context. Apply static deployment filtering and fail-closed per-invocation authorization after canonical binding and before handler dispatch.
- Use six substantive initial packages: `codemode`, `authz`, `mcpserver`, `internal/binding`, `internal/catalog`, and `internal/execution`. This preserves meaningful contracts without the original speculative package and port count.
- Start implementation with a disposable actual-MCP product spike, then build the secure production MVP. Defer Rego, workers, richer schemas/search, observability, fuzzing, benchmarks, and Testcontainers until a real consumer or measurement crosses an evidence gate.

## Changes
- `.journal/001/ARCHITECTURE.md` — defines the settled CodeMode product boundary, trust model, six-package structure, public API direction, execution and authorization ordering, safety posture, and evidence-gated architecture.
- `.journal/001/IMPLEMENTATION_PLAN.md` — orders delivery around a disposable end-to-end product spike, followed by the secure six-package MVP, repository cutover, and public convergence.
- `.journal/001/NOTES.md` — records the source review, runtime research, product corrections, authorization decision, architecture review, and plan refinement chronologically.
- `.journal/TECH_NOTES.md` — captures compact durable context for future implementation sessions.
- `meigma/codemode` — created publicly from `meigma/template-go` and initialized with the personal journal branch `journal/jmgilman`.

## Open Threads
- Begin with Increment 0 in `IMPLEMENTATION_PLAN.md`: prove the official MCP client/transport, three-tool surface, namespaced Starlark call, `main()`, context propagation, result semantics, and final-result-only output in a disposable external spike.
- Select and pin reviewed current MCP Go SDK and Starlark-Go revisions from observed spike behavior rather than from an assumed version.
- Implement the secure MVP before exposing a production execution path: trusted resolution, static filtering, canonical keyword binding, phase enforcement, fail-closed authorization, side-effect-free denial, fresh execution state, and credential-canary evidence.
- Revisit evidence-gated features only after real capabilities, deployment requirements, or measurements justify them.

## References
- [CodeMode architecture](./ARCHITECTURE.md)
- [Spike-first implementation plan](./IMPLEMENTATION_PLAN.md)
- [Published architecture](https://github.com/meigma/codemode/blob/journal/jmgilman/.journal/001/ARCHITECTURE.md)
- [Published implementation plan](https://github.com/meigma/codemode/blob/journal/jmgilman/.journal/001/IMPLEMENTATION_PLAN.md)
- [Ralf Schmid, “Code Mode for Tool Calling in Go”](https://blog.rasc.ch/2026/03/codemode.html)
- [Public `meigma/codemode` repository](https://github.com/meigma/codemode)

## Lessons
- A native-language API inside the code runtime preserves Code Mode's advantage; a generic `call_tool(name, params)` merely recreates MCP RPC inside Starlark.
- Package boundaries can be justified by distinct invariants and dependency direction before a second implementation exists, but interfaces and public extension points still require concrete consumers.
- A disposable product spike should establish SDK and interpreter facts before repository-wide cutover, while security invariants that shape dispatch must be present in the first production MVP.
