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

## 2026-08-24 13:07 — Compatibility with issue 23
Issue #23 is open with no comments and the current scalar-only binding matrix remains unchanged. There is no semantic conflict: issue #13 hardens the final Starlark-to-JSON conversion after `main()` returns, while issue #23 widens the earlier Go-handler-output-to-Starlark conversion in `Plan.ConvertOutput`. A container child count above `MaxResultBytes` cannot possibly encode within that byte limit, so the issue #13 preflight does not reject any final composite value that issue #23 could validly return.

The patches may overlap mechanically in `internal/binding/output.go` and `output_test.go`, but issue #23 can preserve `finalConverter` and its bounded-container check while replacing the separate scalar `ConvertOutput` path. The issue #13 allocation regression should remain direct against `ConvertFinal`; a public composite-capability test would include the earlier Go-to-Starlark allocation and stop isolating the defect.

Issue #23 exposes a separate unresolved resource decision. Current code and documentation define `MaxValueDepth` and `MaxResultBytes` only for the final value; `Plan.ConvertOutput` receives neither limit. Blindly applying `MaxResultBytes` to intermediate capability outputs would undermine issue #23's core use case of filtering a large intermediate collection to a small digest. Its generic Go-to-Starlark walker therefore needs an explicitly designed, request-scoped intermediate materialization bound and must preflight slice/map lengths before allocating. That budget should remain separate from the final-result converter and from hard process isolation.

Recommended order: land issue #13 independently, then implement issue #23 on top of the hardened final converter while resolving the intermediate-output budget explicitly.

## 2026-08-24 13:22 — Implemented issue 13 fix
Created `fix/issue-13-result-allocation` from fetched `origin/master`. Two isolated Programmer agents implemented the production and regression slices, which were reviewed and cherry-picked. `finalConverter` now validates tuple, list, and dictionary child counts before proportional allocation, allocates only from the validated length, and streams dictionary keys instead of calling `Dict.Items()`.

The regression prebuilds 128,000-element tuple, list, and dictionary values, measures only `ConvertFinal`, and caps conversion allocation at 1 MiB. On unchanged `master` it failed with 2,067,136 bytes for tuples, 2,064,480 bytes for lists, and 17,680,784 bytes for dictionaries. The integrated fix passes the regression and all `internal/binding` tests.

The public four-million-element `Server.Execute` reproduction still returns `ErrResourceLimit` and now allocates 64,013,840 bytes instead of 128,034,304 bytes. `mise exec -- moon run root:check` passed after clearing a stale cross-worktree golangci-lint cache; format, lint, build, MCP smoke, race, and documentation build all completed. Gopls reports no diagnostics in either changed file. No user documentation changed because the public limits and error behavior are unchanged.

Next: Push the feature branch, open the issue-closing PR, and inspect its checks.

## 2026-08-24 13:24 — Pull request ready
Pushed `fix/issue-13-result-allocation` and opened PR #26, `fix(binding): reject oversized final-value allocations`, closing issue #13. The PR is mergeable with a clean merge state. CI, GitHub Pages, and Kusari Inspector passed; the GitHub Pages deployment job was correctly skipped for the pull request.

Next: Await review and squash merge on GitHub. Keep the implementation and supporting agent worktrees until the PR is merged or abandoned.
