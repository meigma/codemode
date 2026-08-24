---
id: 006
title: Review execution deadline isolation
started: 2026-08-24
---

## 2026-08-24 12:27 — Kickoff
Goal for the session: Review GitHub issue #12 about monolithic Starlark built-ins overrunning `MaxExecutionTime` and propose a correct remediation.
Current state of the world: CodeMode runs Starlark in-process with cooperative thread cancellation, documents in-process budgets rather than hard tenant isolation, and has not implemented worker-process containment.
Plan: Inspect the live issue and current execution path, reproduce and measure the deadline overrun, evaluate remediation boundaries, and propose a focused fix with verification criteria.

## 2026-08-24 12:32 — Issue review
Reviewed issue #12 and reproduced it through the public `Server.Execute` API on `master` commit `ccac0332a64c52530ebf0102849f73eecf867a12`. With a 1 ms budget, `list(range(8000000))` completed after 25.7–30.8 ms, `tuple(range(8000000))` after 26.5–27.4 ms, and `list.extend(range(8000000))` after 143.5–157.0 ms; every call then returned `ErrResourceLimit` wrapping `context.DeadlineExceeded`.

Starlark-Go checks `Thread.Cancel` between interpreter bytecode operations, not while standard Go built-ins run. The current dependency has multiple unmetered allocation and iteration paths beyond `list`, including `tuple`, `enumerate`, `reversed`, `set`, `sorted`, `zip`, and list methods. Upstream maintainers explicitly recommend a separate OS process for hostile scripts.

Decision: treat the issue as valid CWE-400. Correct the in-process contract and regression coverage immediately, but do not claim that shadowing selected built-ins closes the availability boundary. Hard closure requires host-level worker process or container isolation with a parent-enforced deadline, memory/CPU controls, bounded concurrency, and worker termination. CodeMode cannot transparently move the current in-memory Go handler and authorizer callbacks across a process boundary, so the isolated execution mode needs an explicit host/worker protocol and should be proven in a disposable spike before public API design.
