---
id: 006
title: Review execution deadline isolation
date: 2026-08-24
status: complete
repos_touched: [codemode]
related_sessions: [001, 002, 004]
---

## Goal

Review [issue #12](https://github.com/meigma/codemode/issues/12) (execution deadlines cannot preempt monolithic Starlark built-ins), decide the remediation shape, and produce an approved architecture and implementation plan for it.

## Outcome

Goal met. This was a design session: no product code changed and no PRs were opened. The issue was confirmed as a valid CWE-400 against `master` commit `ccac0332a64c52530ebf0102849f73eecf867a12` with live reproductions (26–157x deadline overrun in-process). A disposable spike proved the chosen remediation on all seven gates, a three-agent pipeline (software-architect → complexity review → DevEx/UX review) produced a frozen architecture, and a planner agent produced a five-increment implementation plan against it.

**A future session implementing the fix should start from these artifacts, in this folder:**

- [`WORKER_ARCHITECTURE.md`](./WORKER_ARCHITECTURE.md) — the approved, frozen architecture: worker-only `Server.Execute` via same-binary re-exec, interpreter-only child, `ServeWorkerAndExit()` single-call host entry, bounded frame protocol with type-preserving numbers, build-time probe, `MaxResultBytes` → `MaxValueBytes` cutover, `MaxConcurrentExecutions = 8`. Appendices A and B record the complexity and DevEx review dispositions.
- [`WORKER_IMPLEMENTATION_PLAN.md`](./WORKER_IMPLEMENTATION_PLAN.md) — the execution-ready plan: five squash-merge increments with file-level targets, Conventional Commit PR titles, test work, and verification commands. Dependency order 1 → {2 ∥ 3} → 4 → 5; full suite/race/lint/docs gate increment 5 only.
- [`WORKER_SPIKE.md`](./WORKER_SPIKE.md) — spike evidence backing the design: kill-at-deadline reduces the issue payloads from 26–157x overrun to 1.2–1.3x on a 1 ms budget; spawn median 2.51 ms; `TestMain` ergonomics proven race-clean.

## Key Decisions

- Issue #12 is valid and broader than `list` -> `tuple`, `enumerate`, `reversed`, `set`, `sorted`, `zip`, and list methods all perform unmetered Go work; Starlark-Go observes `Thread.Cancel` only between bytecode steps, and upstream recommends process isolation for hostile scripts.
- Process isolation, not built-in shadowing -> shadowing cannot intercept methods/operators, `starlark.Universe` mutation is process-global, and a cancellation-aware fork of the runtime provides no heap or crash isolation.
- One binary via re-exec (`os.Executable()` + env marker) -> no second binary to build or ship; Docker `reexec` precedent.
- Interpreter-only child ("Cut B") -> subject, authorizer, handlers, and credentials never enter the killable sandbox; native calls are forwarded as canonical JSON the binding layer already produces.
- Worker-only public `Execute`, no in-process mode -> pre-1.0 with zero consumers; adding an escape hatch later is additive while removing a mode is breaking; one `MaxExecutionTime` contract (deadline + bounded kill/reap overhead).
- Fresh child per execution, no pooling -> spawn median 2.51 ms against a 5 s default budget.
- Single-call `ServeWorkerAndExit()` host entry (DevEx review) -> removes forgotten-guard, ignored-error, and fall-through mistakes; `IsWorker` retained for inspection only.
- Complexity review applied in full -> dropped public `WorkerStderr`, collapsed internal packages, removed redundant protocol elements, renamed `MaxResultBytes` to `MaxValueBytes` covering every boundary-crossing value.

## Changes

- `.journal/006/WORKER_SPIKE.md` - disposable-spike evidence of record (spike code in `/tmp` was intentionally not retained).
- `.journal/006/WORKER_ARCHITECTURE.md` - approved frozen architecture with both review-disposition appendices.
- `.journal/006/WORKER_IMPLEMENTATION_PLAN.md` - five-increment implementation plan grounded in current file paths and symbols.
- No repository source, test, or documentation files changed in this session.

## Open Threads

- Implementation has not started. Increment 1 of the plan (process-neutral binding and value conversion) is the entry point.
- Planner escalation awaiting maintainer decision: architecture §10's documentation list omits `README.md` and `SECURITY.md`, whose verbatim in-process containment claims become false after the cutover. Recommendation on record: amend §10 to include both in increment 5 as factual-accuracy fixes; not yet approved.
- Issue #12 stays open until increment 5 lands; the plan includes a regression test for its exact payloads.
- Explicit architecture non-goals deferred with it: cgroup memory caps (Linux-grade guidance only), Windows production support (compile-only until CI proves it), worker pooling.

## References

- [Issue #12 — Execution deadlines cannot preempt monolithic Starlark built-ins](https://github.com/meigma/codemode/issues/12)
- [Approved architecture](./WORKER_ARCHITECTURE.md)
- [Implementation plan](./WORKER_IMPLEMENTATION_PLAN.md)
- [Spike evidence](./WORKER_SPIKE.md)
- [Starlark-Go on process isolation for hostile scripts](https://github.com/google/starlark-go/issues/160#issuecomment-466794230)
- [Session 002 summary — secure MVP implementation](../002/SUMMARY.md)
- [Session 004 summary — functional verification campaign](../004/SUMMARY.md)

## Lessons

- Starlark-Go cancellation is checked between bytecode operations only; step limits and `Thread.Cancel` both fail to bound work inside one Go built-in, so no in-process configuration closes the availability gap.
- The binding layer's canonical JSON argument maps and converted outputs double as a process-boundary wire format with no new serialization design.
- A raw JSON round trip erases `int64` to `float64`; any Go-to-Go JSON protocol carrying typed numbers needs an explicit numeric-preservation decision up front.
- Freezing a design before handing it to a planner works: the planner surfaced a genuine gap (§10 omissions) as an escalation instead of silently patching the design.
