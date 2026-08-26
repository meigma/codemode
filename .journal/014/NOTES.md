---
id: 014
title: Review pure-compute Starlark stdlib
action: review
started: 2026-08-25
---

## 2026-08-25 19:54 — Kickoff
Goal for the session: Review GitHub issue #42, which proposes an always-on pure-compute Starlark surface containing `sum`, `json`, and `math` while preserving worker budgets, namespace safety, and the documented model-facing contract.
Current state of the world: `master` includes the worker-only execution architecture, richer capability binding and composite outputs, and issue #24's bounded model-facing diagnostics in merged PR #43. Issue #42 is open and coordinates directly with the language-surface wording changed by #24.
Plan: Inspect the live issue and current implementation, trace universe construction and capability namespace validation, verify upstream module behavior, and report correctness, scope, risks, and an implementation recommendation.

## 2026-08-25 20:11 — Issue review
Reviewed the live issue, current `master` execution/catalog/worker validation paths, the merged #24 tool wording, and pinned `go.starlark.net` source.

Confirmed:
- The proposal fits the worker-only architecture. `json` and `math` are already in the pinned module, execute inside the child, require no module loading, and do not change crossing-value conversion.
- Current dynamic predeclared roots shadow standard universe names. A spike passed a predeclared `len` value and observed it replace the standard builtin, so every fixed universe/stdlib root must be rejected, not only `json`, `math`, and `sum`.
- The pinned JSON module exports `decode`, `encode`, `encode_indent`, and `indent`; the issue names only three. The implementation and documented compatibility commitment must choose explicitly rather than accidentally exposing `encode_indent`.
- The pinned math module already exports `math.round`, which rounds halves away from zero and returns a float. A Python-like top-level `round` is not trivially equivalent and should remain excluded without evidence and a precise contract.
- Upstream Go builtins do not debit work performed inside the builtin from Starlark bytecode steps. A spike measured 16 steps for both 10-element and 100,000-element `json.decode`, and 14 steps for both sizes of `json.encode`. `MaxExecutionTime` still kills the worker; `MaxExecutionSteps` only bounds interpreter bytecode as the public API already states.

Required issue corrections:
- Replace “fails at registration” with the current deferred contract: `Register` records the failure and `Build` returns `ErrInvalidRegistration` with a clear host-side reserved-root diagnostic.
- State that adding reserved roots is a breaking registration change for currently valid names such as `json.fetch`, `math.add`, `sum.x`, and existing universe collisions such as `len.lookup`.
- Define the exact fixed top-level and module-member lists for the documentation drift test.
- Update `security-model.md` (“only predeclared application functions...”) and the public capability-name reference in addition to `mcp-tools.md` and the live `execute` description.
- Phrase the `time` exclusion as “no built-in time module”; a host may still deliberately register a `time.*` capability namespace.

Recommended proof is one real same-binary MCP program combining `sum`, JSON round-trip, and `math.sqrt`, plus reserved-root validation at catalog, execution, and worker-manifest boundaries and an empty-engine check that no built-in `time` module exists.
