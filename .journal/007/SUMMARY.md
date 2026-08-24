---
id: 007
title: Review final-value allocation limit
date: 2026-08-24
status: complete
repos_touched: [codemode]
related_sessions: [002]
---

## Goal

Review GitHub issue #13's report that final-value list conversion allocated from untrusted source length before enforcing the byte-derived node budget, propose an evidence-backed fix, and land the approved remediation.

## Outcome

Goal met. The public `Server.Execute` reproduction confirmed 128,034,304 bytes of allocation before `ErrResourceLimit`. A bounded-container prototype removed the conversion-layer half of that allocation, the implementation and regression were produced in isolated Programmer worktrees, and PR #26 passed local and GitHub validation before squash-merging to `master` as `e2b1e5641bd0800e2f88667e294debe69d84f5f7`. Issue #13 closed automatically.

Final conversion now rejects tuple, list, and dictionary source lengths that cannot fit the remaining node budget before proportional destination allocation. Dictionary conversion streams keys instead of materializing `Dict.Items()`. The public reproduction now allocates 64,013,840 bytes while returning the same resource-limit error; the remaining allocation is the Starlark source value rather than avoidable final conversion.

## Key Decisions

- Preflight every final container's direct child count against `remainingNodes` -> a result that cannot possibly fit the encoded byte limit is rejected before untrusted length drives destination allocation.
- Apply one invariant to tuples, lists, and dictionaries -> widening capability output shapes later cannot reintroduce the same final-conversion bug through another supported container.
- Iterate dictionary keys and perform exact lookups instead of calling `Dict.Items()` -> avoids its full tuple slice and `2 * Len()` value-array materialization.
- Measure `ConvertFinal` directly with prebuilt source containers -> the regression isolates destination conversion allocation instead of counting Starlark source construction.
- Keep final-result and intermediate capability-output budgets separate -> issue #23's intended in-runtime filtering would be undermined if `MaxResultBytes` silently limited intermediate values.
- Preserve the documented isolation boundary -> this fix removes conversion amplification but does not claim a hard in-process heap limit.

## Changes

- `internal/binding/output.go` - validates container lengths before allocation, allocates from validated lengths, and streams dictionary iteration.
- `internal/binding/output_test.go` - adds an allocation regression for 128,000-element tuples, lists, and dictionaries.
- PR #26 - carries the issue-closing implementation, validation evidence, and process-isolation caveat.

## Open Threads

- Issue #23 remains open. Its proposed generic Go-to-Starlark output walker needs an explicitly designed, request-scoped intermediate materialization budget; `Plan.ConvertOutput` currently receives neither `MaxValueDepth` nor `MaxResultBytes`.
- A Starlark program can still allocate a large source value before final conversion. Use process or container isolation where callers are mutually untrusted or require a hard heap boundary.

## References

- [Issue #13 — Final-value conversion preallocates oversized lists before enforcing limits](https://github.com/meigma/codemode/issues/13)
- [PR #26 — fix(binding): reject oversized final-value allocations](https://github.com/meigma/codemode/pull/26)
- [Merged commit `e2b1e56`](https://github.com/meigma/codemode/commit/e2b1e5641bd0800e2f88667e294debe69d84f5f7)
- [Issue #23 — Widen the capability binding matrix](https://github.com/meigma/codemode/issues/23)
- [Session 002 summary](../002/SUMMARY.md)

## Lessons

- `starlark.Dict.Items()` performs two proportional transient allocations in addition to the destination map, so source-length preflight alone should not justify retaining it on a materialization-sensitive path.
- The four-million-element reproduction split almost exactly in half after the fix, distinguishing unavoidable Starlark source construction from avoidable final-conversion allocation.
- A process-wide allocation assertion is stable when the source is built before the measurement window, package tests are non-parallel, GC state is restored, and the failure margin is several multiples of the ceiling.
