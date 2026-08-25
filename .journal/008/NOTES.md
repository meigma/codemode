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

## 2026-08-24 17:37 — Increment 2 started
Maintainer approved PR #27. Squash-merged it to `master` as `81f899b`, refreshed the default worktree, removed the integrated Increment 1 worktree, and created `refactor/execution-native-dispatch` from `origin/master`.

Increment 2 keeps execution behavior in-process while moving native dispatch behind process-neutral ports. Two parallel Programmer slices will own: (1) `internal/execution` capability bindings, native-call function port, namespace stubs, and interpreter tests; (2) root `dispatcher`, transitional `Server` wiring, typed-output-to-process-neutral conversion, and dispatch/server tests. Shared contract: `execution.CapabilityBinding` carries only ID, dotted name, and `Plan.InputShape()`; `execution.NativeCall` accepts an ID plus canonical map and returns a normalized value; `Engine.Execute` retains the temporary context/deadline watcher; the root dispatcher translates every `BindValue` failure to internal/protocol classification before authorization. One bounded Reviewer will review both slices continuously.

## 2026-08-24 18:24 — Increment 2 implementation complete
Parallel Programmer slices completed the process-neutral interpreter port and authoritative root dispatcher. `internal/execution` now depends only on capability ID/name/input shapes plus a request-specific native function; authorization, subjects, handlers, catalog entries, and parent panic recovery moved out. Root `dispatcher` now owns lookup, authoritative `BindValue`, fresh authorization arguments, policy/context/handler sequencing, typed output conversion, and error classification. Transitional `Server.Execute` gives the engine and dispatch closure the same elapsed-budget context while retaining the temporary in-engine watcher.

The persistent Reviewer performed contract, slice, integration, fix, and lint-only passes. Fixed findings included elapsed context sharing, child binding/value classification, namespace/input validation, handler input-drift classification, mid-execution cancellation coverage, a goroutine test hazard, invalid Testify map identity assertions, and deletion of obsolete `BindAs`/`Plan.Bind` Starlark-to-typed compatibility paths. Final review approved with zero blockers. Native result dictionaries now deliberately expose sorted key order; Increment 2 tests pin it.

Configured formatting, scoped lint, `mise exec -- go test ./internal/execution -count=1`, and `mise exec -- go test . -run '^(TestDispatch|TestServerExecute)' -count=1` all pass. Non-blocking accepted debt: catalog and execution independently validate dotted namespace legality, and `BindShape` revalidates immutable shapes per call; neither justifies widening this increment.

## 2026-08-24 18:26 — Increment 2 pull request
Committed and pushed `refactor/execution-native-dispatch`, then opened PR #28, `refactor(execution): move native dispatch behind a port`. The PR records the deliberate sorted-key behavior change and the bounded review evidence. GitHub CI, GitHub Pages, and Kusari Inspector all pass.

## 2026-08-24 19:19 — Increment 3 started
Maintainer approved PR #28. Squash-merged it to `master` as `561bae0`, refreshed the default worktree, removed the integrated Increment 2 worktree, and created `feat/worker-protocol-framing` from `origin/master`.

Increment 3 adds only the private bounded protocol and type-preserving frame codec in a new `internal/worker` package; it starts no processes. One Programmer agent will own protocol production code while a second writes the complete behavioral test matrix against the shared semantic contract in parallel, coordinating exact private symbols over IRC. One persistent bounded Reviewer will review the contract, production slice, test slice, and fixes. Required invariants: uint32 big-endian length prefix; strict UTF-8/JSON/concrete-frame decoding; legal state transitions; pre-allocation length caps; one outstanding call; payload-free parent abort; exact int64/finite-float preservation; checked manifest/source/value frame arithmetic; no public protocol or launcher seam.

## 2026-08-24 21:16 — Increment 3 implementation complete
Parallel Programmer slices completed the private worker protocol and behavioral matrix. `internal/worker` now owns uint32 big-endian framing, strict type-first concrete decoding, exact `int64`/finite-`float64` JSON conversion, manifest and limit validation, checked payload-cap arithmetic, and parent/child state machines with one outstanding native call and terminal abort/final behavior. Source and tests were split by responsibility to keep every Go file below the repository's 1,000-line cap.

The persistent Reviewer found and closed four blockers: strict discriminator preview initially rejected every multi-field frame, the working contract incorrectly rejected the architecture's legal `final_error(resource_limit)`, semantic sentinel identities diverged, and version errors discarded the child-reported version needed by later probe diagnostics. Subsequent review and the final test-file split both received zero-blocker verdicts.

Configured formatting, scoped `golangci-lint`, `mise exec -- go test ./internal/worker -count=1`, and `mise exec -- go test ./internal/... -count=1` pass. Increment 3 remains worker-internal and starts no process; process supervision stays in Increment 4.

## 2026-08-24 21:19 — Increment 3 pull request
Committed and pushed `feat/worker-protocol-framing` as `d643ea4`, then opened PR #29, `feat(worker): add bounded protocol framing`. The PR is mergeable and clean; GitHub CI, GitHub Pages, and Kusari Inspector all pass.
