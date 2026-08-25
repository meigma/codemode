---
id: 009
title: New work session
started: 2026-08-25
---

## 2026-08-25 09:55 — Kickoff
Goal for the session: start a new journal session; awaiting the developer's substantive request.
Current state of the world: worker-only execution isolation is complete on `master` (session 008, PRs #27–#31, issue #12 closed). Latest master commit is `3b541fc` "feat(worker): require fresh-process execution (#31)". Open threads: issue #23 (wider binding matrix needs a request-scoped intermediate materialization budget), no OS-level tenant quotas, Windows support compile-only.
Plan: wait for the developer's request, then scope and proceed.

## 2026-08-25 10:00 — Goal set: docs review
Developer goal: focused review/improvement of user-facing docs after session 008's worker refactor. Three concerns: (1) technical accuracy vs current master, (2) language-style skill conformance, (3) diataxis conformance per page.
Scope: docs/docs/ (7 pages), README.md, SECURITY.md, CONTRIBUTING.md. Excluded: CHANGELOG.md, agent/session files.
Dispatched three parallel read-only reviews: AccuracyReview (qa, docs vs source), StyleReview (conformance, language-style skill + cross-doc terminology), DiataxisReview (conformance, type fit per page). Findings-only; improvements follow after the report.

## 2026-08-25 10:25 — Review complete, scope chosen
Three reviews returned 45 findings: 9 HIGH accuracy (two systemic errors: "only final value crosses the boundary" in 4 docs — native args/results do cross the private worker protocol; "MaxExecutionTime includes worker cleanup" in 3 docs — kill/reap overhead is outside the budget per limits.go:28-31; plus probe overpromise and MaxConcurrentExecutions scope in public-api.md), 20 style (repeated ungrounded "normally", multi-action steps, 3 cross-doc terminology drifts), 16 diataxis (type bleed everywhere).
Developer chose scope: all accuracy + all style + cheap diataxis fixes only (index rework to orientation page, tutorial prerequisites move, verification steps in both how-tos, explanation retitle). No new pages, no tutorial restructure, no README/SECURITY shortening.
Reports: agent://AccuracyReview, agent://StyleReview, agent://DiataxisReview. Implementation branch: docs/post-refactor-review.

## 2026-08-25 11:05 — Fixes applied, verified, PR opened
Five programmer agents applied the approved scope across 11 files on branch docs/post-refactor-review. I trimmed the index intro (agent had front-loaded limit contracts onto the orientation page) and repaired the Rego how-to after functional testing.
Functional verification (functional-tester + my re-run of the fixed procedure): tutorial PASS verbatim; disable-capabilities PASS verbatim; Rego how-to initially FAILED as a procedure — err shadowing compile error, denial text unobservable through the tutorial's callTool, const scope ambiguity, missing go mod tidy. All four fixed in the doc; fixed procedure re-run reproduces `execute: {"result":{"count":2,"key":"alpha"}}` and `execute: permission denied` exactly.
Validation: moon run docs:build (strict) PASS; go test ./... PASS. Commit f1a1a43; PR #32 open.
Durable lesson for close-out: docs that publish exact expected outputs must be executed, not source-grounded — the Rego guide was accuracy-clean on paper yet unreproducible in practice.

## 2026-08-25 11:15 — PR #32 merged
All checks passed (ci, GitHub Pages, Kusari). Squash-merged to master as 906541d; implementation worktree docs/post-refactor-review removed. Docs review/improvement work complete.

## 2026-08-25 11:40 — Follow-up docs pass started
Eight PRs merged after the docs PR (#33–#40): reduced first-touch API ceremony (#34), widened scalar inputs (#35), intermediate native-result budget (#36 — issue #23 thread), composite outputs (#37), plus tests and matrix docs (#39). All doc pages rewritten alongside. Dispatched follow-up review wave: AccuracyReview2 (qa vs 715a716), StyleDiataxisReview2 (conformance on the new text), VerifyDocsProcedures2 (functional-tester, all executable claims — added to the review wave this time after last round proved source-grounding insufficient).

## 2026-08-25 12:10 — Follow-up findings and fix dispatch
Review wave returned: 1 blocking HIGH (rego how-to "Verify the policy" orphaned by #34's tutorial rewrite — all three reviewers converged; outcomes verified correct via real MCP client, procedure dead), plus mcp-tools.md literal-wire-schema mismatch ($ref/$defs and result:{} vs inlined/true), composite example root-shape error (list[...] vs field items), SECURITY.md missing MaxIntermediateValueBytes, reintroduced hedges ("normally", "common"), "output universe" jargon, conditions-after-code in three docs, procedural creep in public-api reference, non-executable disable-capabilities verification.
Skipped as structural vs fresh #34 intent (mirrors developer's earlier scope call): named-agent tutorial path, tutorial explanation relocation, MCP recovery-section move, README snippet removal.
Dispatched four programmer agents on branch docs/followup-review.

## 2026-08-25 12:55 — Follow-up fixes verified, PR #41 open
Four programmer agents applied fixes across 7 files; I added the execute `source`-argument pointer both how-tos were missing (functional tester's only substantive friction). Functional re-verification (MCP client over CommandTransport as agent stand-in): every documented expected string reproduces verbatim — tutorial result, rego allowed/denied (plus embed variant), all three disable-capabilities checks. Docs build (strict) and go test ./... pass. Commit 2fe9197; PR #41.
Recurring failure mode worth TECH_NOTES promotion at close: code PRs that rewrite one doc page (here #34's tutorial rewrite) orphan procedures in sibling pages that patch its code; cross-page procedure references need functional re-verification after any tutorial change.
