---
id: 008
title: Implement worker execution isolation
date: 2026-08-25
status: complete
repos_touched: [codemode]
related_sessions: [006, 007]
---

## Goal

Review session 006's approved worker-only architecture and five-increment implementation plan against the current repository, correct any stale assumptions, and implement the plan end to end.

## Outcome

Goal met. All five increments were implemented as separately reviewed pull requests and squash-merged to `master`. CodeMode now executes every valid `Server.Execute` program in a fresh re-executed worker process, while capability binding, authorization, and handlers remain authoritative in the parent. Deadline or request cancellation kills and reaps the Starlark worker, closing issue #12's cooperative-cancellation gap for monolithic built-ins.

The public cutover is complete rather than transitional. Final binaries and test binaries install `ServeWorkerAndExit` before ordinary setup; `Builder.Build` probes that wiring; `Server` retains an immutable worker runner; `MaxValueBytes` bounds every crossing value; `MaxConcurrentExecutions` bounds live execution workers; and obsolete in-process elapsed cancellation, `MaxResultBytes`, and `ConvertFinal` paths were removed. README, SECURITY, package Godoc, tutorial, API reference, MCP reference, documentation home, and security model now describe the actual process boundary and its limits.

PRs #27 through #31 passed local validation, GitHub CI, GitHub Pages, Kusari Inspector, and bounded continuous review before merge. The final repository also passed the required worker and root race gates, strict documentation build, Windows cross-build, and full Go test suite.

## Key Decisions

- Preserve session 006's worker-only design -> the architecture still matched the repository and directly addressed the measured issue #12 failure; no compatibility mode, worker pool, launcher abstraction, or external worker binary was justified.
- Amend Increment 5 to update `README.md` and `SECURITY.md` -> their prior all-in-process claims would have become materially false after the public cutover.
- Re-execute the final host binary with a private marker-only environment -> this provides one deployable artifact, avoids inherited credentials and configuration, and lets `Builder.Build` verify the actual integration path.
- Run one Starlark execution per child and kill/reap on cancellation -> process termination preempts interpreter work even when a monolithic built-in does not observe cooperative cancellation.
- Keep authorization and handlers in the parent -> trusted identity, policy, typed Go handlers, and host resources never become child-owned protocol configuration.
- Dispatch parent Go code asynchronously after cancellation-aware rechecks -> `Server.Execute` can return after killing its child, while documentation remains explicit that CodeMode cannot forcibly stop or undo already-dispatched Go work.
- Replace final-only `MaxResultBytes` with `MaxValueBytes` on each native argument, native result, and final value -> one type-preserving wire contract now bounds every process crossing without pretending to be an aggregate intermediate-materialization budget.
- Bound live workers with `MaxConcurrentExecutions = 8`, with elapsed time starting before semaphore wait -> queueing, spawn, protocol, interpreter, parent dispatch, and cleanup all consume the same caller-visible budget.
- Keep protocol symbols internal and use strict type-first concrete decoding -> malformed, oversized, out-of-state, and version-incompatible frames fail closed without expanding the public API.

## Changes

- `internal/binding` - added process-neutral input shapes, authoritative parent re-binding, recursive Go/Starlark value conversion, exact numeric-kind preservation, deterministic dictionary order, and retained allocation preflight defenses from session 007.
- `internal/execution` - replaced catalog-owned native dispatch with process-neutral capability bindings and a native-call function port, then reduced the engine to interpreter-only execution behind child-owned limits.
- `dispatch.go` and `server.go` - moved authoritative bind/authorize/invoke/convert sequencing into the parent and delegated every public execution to the immutable worker runner.
- `internal/worker` - added bounded uint32 framing, strict typed protocol decoding, type-preserving value encoding, manifest and cap validation, child serving, same-binary launch, build probing, semaphore control, native forwarding, cancellation, pipe closure, deterministic kill/reap, and protocol diagnostics.
- `worker.go`, `builder.go`, and `limits.go` - exposed final-binary worker entry facades, probed worker wiring during build, added worker concurrency and crossing-value limits, and removed obsolete result-only and in-process deadline configuration.
- Root and MCP tests - installed worker-aware `TestMain` entrypoints and added same-binary regressions for issue #12 payloads, parent native dispatch, numeric preservation, cancellation, queued execution, abort behavior, concurrency, repeated kill/reap, and actionable miswiring diagnostics.
- `README.md`, `SECURITY.md`, `doc.go`, and `docs/docs` - documented mandatory host/test wiring, the fixed build probe, complete elapsed-budget coverage, marker-only child environment, parent-code cancellation semantics, and the absence of CodeMode-provided operating-system tenant quotas.

## Open Threads

- Issue #23 remains evidence-gated. A wider capability binding matrix with composite intermediate outputs still needs a separate request-scoped intermediate materialization budget; `MaxValueBytes` bounds each crossing value, not aggregate handler work across a program.
- CodeMode does not provide operating-system CPU, memory, filesystem, network, or tenant quotas. Hosts needing those guarantees must add container or workload controls. Package initialization still runs in every re-executed child with the host operating-system user's authority.
- Worker code cross-compiles for Windows, but production Windows support remains unclaimed until Windows CI exercises marker startup, anonymous pipes, cancellation, kill latency, exactly-once reap, and race behavior.

## References

- [Session 006 worker architecture](../006/WORKER_ARCHITECTURE.md)
- [Session 006 implementation plan](../006/WORKER_IMPLEMENTATION_PLAN.md)
- [Session 007 summary](../007/SUMMARY.md)
- [Issue #12 — Execution deadlines cannot preempt monolithic Starlark built-ins](https://github.com/meigma/codemode/issues/12)
- [Issue #23 — Widen the capability binding matrix](https://github.com/meigma/codemode/issues/23)
- [PR #27 — refactor(binding): add process-neutral value conversion](https://github.com/meigma/codemode/pull/27)
- [PR #28 — refactor(execution): move native dispatch behind a port](https://github.com/meigma/codemode/pull/28)
- [PR #29 — feat(worker): add bounded protocol framing](https://github.com/meigma/codemode/pull/29)
- [PR #30 — feat(worker): add worker process supervision](https://github.com/meigma/codemode/pull/30)
- [PR #31 — feat(worker): require fresh-process execution](https://github.com/meigma/codemode/pull/31)
- [Final merged commit `3b541fc`](https://github.com/meigma/codemode/commit/3b541fca34a0fdf3bcf5775fbebe7de1bd5a51df)

## Lessons

- Exact process-boundary tests found failures that unit-only interpreter tests could not: probe ordering, stderr truncation, EOF discipline, native abort suppression, child cleanup, semaphore reuse, and numeric-kind preservation all required the real same-binary path.
- Cancellation correctness has two separate contracts. The worker can be forcibly killed and reaped; parent authorizers and handlers remain cooperative Go code and may continue after `Execute` returns if they ignore context.
- Protocol safety depends on validating the discriminator before concrete decoding, then enforcing state transitions and frame-size arithmetic independently. Treating generic JSON shape validation as a substitute rejected legal frames and obscured version diagnostics.
- A marker-only child environment reduces accidental credential inheritance, but package initialization precedes `main`; keeping credential and client construction after `ServeWorkerAndExit` remains part of the security boundary.
