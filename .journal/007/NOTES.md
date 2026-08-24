---
id: 007
title: Review final-value allocation limit
started: 2026-08-24
---

## 2026-08-24 12:41 — Kickoff
Goal for the session: Review GitHub issue #13 about oversized final-value list preallocation and propose an evidence-backed fix.
Current state of the world: `master` is at `ccac0332a64c52530ebf0102849f73eecf867a12`; issue #13 reports that rejected Starlark lists can trigger destination allocations far above `MaxResultBytes` before the converter enforces its node budget.
Plan: Inspect the issue, conversion implementation, and existing limit tests; reproduce the relevant allocation behavior if practical; then propose the smallest consistent container-allocation fix and regression coverage.

## 2026-08-24 12:48 — Issue review and prototype
Issue #13 is open with no comments. `ConvertFinal` decrements the current container node before dispatch, but `convertTuple`, `convertList`, and `convertDict` all allocate from the complete source length before any child consumes the remaining budget. Dictionary conversion also calls `Dict.Items()`, whose Starlark implementation allocates a full tuple slice and a `2 * Len()` value array.

The supplied public `Server.Execute` reproduction was confirmed unchanged on current `master`: `ErrResourceLimit` followed `128,034,304` bytes of both `TotalAlloc` and `HeapAlloc` with GC disabled.

A disposable clone validated the proposed invariant: a private bounded-container-length check rejects when a container's direct child count exceeds `remainingNodes`, and every tuple, list, and dictionary allocation uses only the validated length. Existing `internal/binding` tests passed. The same public reproduction still returned `ErrResourceLimit` but allocated `64,013,840` bytes, removing about 64 MiB of conversion-layer amplification; the remaining allocation is the Starlark source list and confirms that process isolation is still required for hard heap containment.

A disposable regression prebuilt a 262,144-element Starlark list outside the measurement window, then measured only `ConvertFinal` with a 1 KiB result budget. It failed on current code because conversion allocated `4,213,432` bytes against a 1 MiB ceiling and passed with the prototype. No product files were changed.

Next: Deliver the exact code and test proposal, including the tuple/list/dictionary invariant and the unchanged process-isolation boundary.
