---
id: 014
title: Review pure-compute Starlark stdlib
date: 2026-08-25
status: complete
repos_touched: [codemode]
related_sessions: [011, 012]
---

## Goal

Review issue #42 against CodeMode's worker-only execution architecture and model-facing contract, then implement the approved fixed pure-compute Starlark surface without adding host configuration, native dispatch paths, or nondeterminism.

## Outcome

Goal met. CodeMode now exposes numeric `sum(iterable)`, selected JSON helpers, and the complete pinned math module in every worker. Capability roots cannot shadow the fixed language surface. The exact surface is documented and checked against runtime names, and a real same-binary MCP program proves JSON round-trip, summation, and math composition. PR #46 passed local and GitHub validation, was squash-merged to `master` as `344b79b48adb040914b8f7dfa8b22b7b35b1272b`, and closed issue #42.

## Key Decisions

- Add one fixed always-on surface with no option -> models see one CodeMode language across hosts, and the additions introduce no I/O, clock, randomness, authorization, or native dispatch.
- Implement only numeric one-argument `sum(iterable)` -> matches the observed failure while avoiding an unproven Python-compatible `start` or sequence-concatenation commitment.
- Expose exactly `json.decode`, `json.encode`, and `json.indent` -> the pinned module also exports `encode_indent`, but issue #42 did not justify adding it to the permanent contract.
- Expose the complete pinned `math` module but no top-level `round` -> `math.round` already has precise upstream semantics that differ from Python's builtin.
- Reserve every Starlark universe root plus `sum`, `json`, and `math` -> dynamic capability roots otherwise shadow fixed names; nested leaves such as `stats.sum` remain valid.
- Reserve dialect-gated `set` without advertising it as available -> the pinned universe contains `set`, but CodeMode's file options disable set support.
- Keep `MaxExecutionSteps` defined as bytecode steps -> Go-backed builtin internals are bounded by elapsed worker time, not charged per element to the interpreter counter.
- Defer recursive JSON hardening to issue #45 -> a correct fix must cover both encode and decode and define an internal-compute depth contract rather than repurpose crossing-value limits or add a decode-only guard.

## Changes

- `internal/universe/` - added the canonical fixed predeclared surface, numeric sum builtin, filtered JSON module, complete math module, copied sorted name queries, and shared reserved-root predicate.
- `internal/execution/` - merged fixed and dynamic namespaces, rejected collisions defensively, added stdlib behavior coverage, and migrated stale undefined-`sum` diagnostics to `filter`.
- `internal/catalog/`, `builder.go`, `capability.go` - rejected reserved capability roots through the existing deferred `Register` / `Build` contract with clear host-side diagnostics.
- `internal/worker/` - rejected forged reserved roots in child manifests and updated stale diagnostic fixtures.
- `mcpserver/` - updated the live `execute` description, added exact documentation drift coverage, and proved the additions through a real same-binary MCP execution.
- `docs/docs/` - documented the exact language surface, reserved-root breaking change, bytecode-versus-elapsed budget semantics, absent built-in time module, and updated security boundary.
- `moon.yml` - added the pure-compute MCP program to the smoke gate.

## Open Threads

- Issue #45 tracks recursive depth amplification in upstream JSON encode/decode. The worker-process boundary contains a runtime abort to one execution, but excessive nesting can still terminate that worker before CodeMode returns a classified program error.

## Lessons

- An upstream universe name is not necessarily available in the active dialect: `set` remains reserved even though default `syntax.FileOptions` reject its use.
- Go-backed Starlark builtin loops do not increase `Thread.ExecutionSteps`; bytecode and elapsed-time budgets must remain described separately.
- Documentation drift tests need separate canonical lists for reserved roots and actually available names.

## References

- [Issue #42: Add a fixed pure-compute stdlib](https://github.com/meigma/codemode/issues/42)
- [PR #46: feat(execution)!: add fixed compute stdlib](https://github.com/meigma/codemode/pull/46)
- Merged commit `344b79b48adb040914b8f7dfa8b22b7b35b1272b`
- [Issue #45: Harden JSON stdlib recursion inside the worker](https://github.com/meigma/codemode/issues/45)
- [Session 011 capability composition](../011/SUMMARY.md)
- [Session 012 model-facing diagnostics](../012/SUMMARY.md)
