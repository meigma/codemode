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

## 2026-08-25 20:39 — Implementation kickoff
Created `feat/pure-compute-stdlib` from current `master` in `.wt/feat-pure-compute-stdlib`.

Settled the implementation contract:
- `internal/universe` owns the canonical fixed surface and reserved-root predicate.
- `sum(iterable)` accepts one iterable and numeric values only.
- `json` exposes exactly `decode`, `encode`, and `indent`; `math` exposes the complete pinned module; no top-level `round` or built-in `time` module is added.
- Catalog, execution, and worker-manifest validation share one reserved-root predicate while preserving deferred `Build` errors.
- The live MCP description stays concise; the MCP reference carries a machine-checked exact name block.
- One same-binary MCP program proves JSON round-trip, `sum`, and `math.sqrt` together.

Dispatched three concurrent programmer slices for universe/execution, reserved-root validation, and MCP/docs/e2e work. A persistent read-only reviewer is reviewing the contract and each slice incrementally before the final integrated pass.

## 2026-08-25 21:18 — Implementation delivered
Implemented the accepted issue #42 contract on `feat/pure-compute-stdlib` and opened PR #46.

Delivered:
- Added canonical `internal/universe` with numeric-only `sum(iterable)`, the selected three-member JSON module, the complete pinned math module, exact sorted surface queries, and one reserved-root predicate.
- Merged the fixed surface into every execution engine and rejected colliding roots at catalog, execution, namespace assembly, and child-manifest boundaries.
- Updated live MCP wording, the exact MCP reference surface, capability-name reference, security explanation, and smoke gate.
- Added unit, defense-in-depth, docs/runtime drift, and real same-binary MCP coverage.
- Replaced #24's stale undefined-`sum` diagnostic fixtures with undefined `filter`.

Verification:
- Focused tests passed for `internal/universe`, `internal/execution`, `internal/catalog`, `internal/worker`, and `mcpserver`.
- `mise exec -- moon run root:check --force` passed all eight tasks, including format, lint, build, docs build, MCP smoke, and the full race suite.
- The persistent reviewer approved the final integrated diff with no remaining findings.

The review identified recursive depth amplification in upstream JSON encode/decode. The worker process contains a runtime abort to one execution, but a correct remediation must cover both directions and define an internal-compute depth contract. Filed follow-up issue #45 instead of adding a decode-only guard to #42.

Commit `9384308` is pushed. PR: https://github.com/meigma/codemode/pull/46. Follow-up: https://github.com/meigma/codemode/issues/45.

## 2026-08-25 21:20 — Pull request checks
PR #46 is mergeable. GitHub CI, GitHub Pages, and Kusari Inspector passed; the Pages deployment job was intentionally skipped.
