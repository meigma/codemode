---
id: 008
title: Implement worker execution isolation
started: 2026-08-24
---

## 2026-08-24 16:35 — Kickoff
Goal for the session: Review the approved worker architecture and implementation plan from session 006, then use them as the basis for today's work.
Current state of the world: `master` is at `e2b1e5641bd0800e2f88667e294debe69d84f5f7`; session 006 confirmed issue #12, proved same-binary re-exec in a spike, and froze a worker-only architecture plus a five-increment plan. Implementation has not started. Session 007 subsequently removed avoidable oversized final-value allocations in PR #26 without changing the worker design.
Plan: Re-read the session 006 architecture and plan against the current repository state, identify any stale assumptions or required amendments, then begin with plan increment 1.

## 2026-08-24 16:39 — Session 006 architecture review
Reviewed `WORKER_ARCHITECTURE.md` and `WORKER_IMPLEMENTATION_PLAN.md` in full against current `master`. The architecture remains sound: worker-only `Execute`, same-binary re-exec, interpreter-only child, parent-side authorization/handlers, one execution per child, bounded framing, hard kill/reap, and an eight-child semaphore preserve the trust boundary while closing issue #12's cooperative-cancellation gap.

Current drift from the plan's base commit is limited to PR #26 in `internal/binding/output.go` and `output_test.go`. Increment 1 must preserve its direct-child node-budget preflight for tuple/list/dict values and streaming dictionary iteration while extracting process-neutral conversion. Baseline `mise exec -- go test ./internal/binding ./internal/execution -count=1` passes.

Two plan corrections are required before the final cutover: add `README.md` and `SECURITY.md` to increment 5 because their all-in-process containment claims will become false, and strengthen the issue #12 regression so it proves the worker reached a monolithic built-in before deadline kill rather than passing because a 1 ms budget expired during process startup. No other redesign is justified; increment 1 remains the correct starting point.

## 2026-08-24 16:48 — Increment 1 started
Fetched `origin/master` and created isolated Worktrunk branch `refactor/worker-binding-values` at `.wt/refactor-worker-binding-values`. Two Programmer agents are implementing independent slices in parallel: shape/input binding and process-neutral value/output conversion. One bounded Reviewer agent owns continuous review across both slices. The shared contract preserves the narrow binding matrix, fresh parent canonical maps, exact `int64`/`float64` identity, PR #26 allocation defenses, current callers, and the five-increment cutover sequence.

## 2026-08-24 17:18 — Increment 1 implementation complete
Parallel Programmer slices landed on `refactor/worker-binding-values`. Input binding now validates exact manifest shapes, binds child Starlark calls to process-neutral maps, and authoritatively re-binds decoded maps to exact registered inputs plus fresh canonical authorization maps. Value conversion now centralizes validation and both Starlark conversion directions for the supported recursive domain while preserving numeric identity, cycle/depth/materialization defenses, PR #26's direct-child preflight, streaming dictionary conversion, and current `ConvertFinal`/`ConvertOutput` callers.

The persistent Reviewer completed three bounded passes. Fixed findings: removed an invalid Testify map `NotSame` assertion, added direct shape-validation tests, and made Go-slice cycle identity distinguish visible ranges while skipping empty slices. Final review approved with zero merge blockers. Deliberate decisions: `ToStarlark` inserts map keys in sorted order for deterministic Increment 2 behavior; current invalid-UTF-8 string handling remains unchanged; Increment 2 must translate every `BindValue` failure to internal/protocol classification.

Formatting completed with the configured `golangci-lint` formatters. Targeted verification passes: `mise exec -- go test ./internal/binding -count=1` and `mise exec -- go test ./internal/execution -run '^TestExecute' -count=1`.

## 2026-08-24 17:24 — Increment 1 pull request
Committed and pushed the implementation, then opened PR #27, `refactor(binding): add process-neutral value conversion`. The first CI run exposed five lint-only findings: an exhaustive-switch annotation, two shared signature constants, and two Godoc links. The two Programmer agents fixed their independent slices in parallel; the persistent Reviewer approved the lint-only changes with zero blockers. Scoped lint and binding/execution tests pass locally. The updated GitHub checks pass: CI, GitHub Pages, and Kusari Inspector.
