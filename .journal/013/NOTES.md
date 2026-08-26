---
id: 013
title: Design bounded capability search
started: 2026-08-25
---

## 2026-08-25 17:31 — Kickoff
Goal for the session: Produce a bounded architecture for improved agent-facing capability search, review it adversarially for unnecessary complexity, apply that feedback, and leave the final document in this journal session for review.
Current state of the world: The current catalog uses bounded literal substring search over enabled names and summaries. A disposable 1,000-capability spike showed strong retrieval gains from weighted token ranking, description indexing, explicit search terms, and bounded typo tolerance. A software architecture review is running from the grounded spike brief; the repository remains unchanged.
Plan: Complete the bounded architecture, send it to a complexity reviewer, apply justified simplifications, write the final architecture here, and record the evidence and decisions.

## 2026-08-25 17:45 — Proposed architecture
The software architecture agent produced a bounded relevance-ranked lexical design grounded in the spike and current repository contracts. It preserves the immutable filtered catalog and three-tool MCP surface, proposes explicit search vocabulary plus a `{results, truncated}` cutover, and specifies bounded normalization, ranking, response size, and typo work.
Next: The complexity reviewer is challenging the proposal's packed index, numeric ceilings, pooling, scoring detail, and rollout size before the final journal architecture is written.

## 2026-08-25 17:59 — Complexity review applied
The complexity reviewer returned `SUBSTANTIALLY SIMPLIFY` while preserving relevance-ranked lexical search, description indexing, explicit `SearchTerms`, deterministic full-catalog ranking, bounded output, and truncation signaling.
Applied changes: replaced packed postings/token arenas/pooling/heap machinery with an immutable document slice and bounded direct scan; collapsed overlapping ceilings into a small independent bound set; made the response-byte cap internal; replaced giant score boosts with an explicit comparator; deferred phrase boosts and fuzzy matching; and made future packed indexes, heaps, interning, and pooling profile-triggered only.
Final review document: `SEARCH_ARCHITECTURE.md`. The repository remains unchanged; only this journal session contains the architecture.

## 2026-08-25 18:09 — Implementation started
Created isolated Worktrunk branch/worktree `feat/bounded-search` at `.wt/feat-bounded-search` from fetched `origin/master`. A persistent bounded reviewer is assigned for the core, MCP, and final documentation/full-diff increments.
Programmer-agent sequence: core/public catalog search first, MCP contract cutover second, then documentation. Agents skip validation; the parent will run targeted tests, the project validation suite, and an actual MCP search scenario before opening the PR.

## 2026-08-25 19:24 — Core and MCP increments reviewed
Programmer agents implemented the direct immutable catalog scorer, public `SearchTerms` and `SearchResponse` cutover, bounded metadata/query/response behavior, and the MCP object schema/version-2 cutover.
The persistent reviewer caught and drove fixes for disabled-index placement hazards, caller-slice ownership, query error classification, `MySQL`/`sql` acronym tokenization, request-local map allocation, connector vocabulary, missing build helper, and MCP version coverage. Core and MCP scopes are now reviewer-clean. Documentation is in progress; validation remains intentionally deferred to the parent.

## 2026-08-25 19:49 — Implementation delivered for review
Completed the bounded relevance-ranked search design on `feat/bounded-search` and opened PR #44 at `https://github.com/meigma/codemode/pull/44` from commit `24e7670`.
Final review fixed exact-versus-prefix precedence within one field, added a discriminating regression case, and documented the registration, searchable-metadata, and single-result build ceilings. Targeted root/catalog/MCP tests passed; `moon run root:check` passed format, lint, build, MCP smoke, race, and docs build; and verbose `TestActualMCPSecureLoop` passed against the real in-memory MCP transport with populated and empty search responses.

## 2026-08-25 19:53 — Close
The developer approved PR #44, which was squash-merged to `master` as `f50f113181a0333eabf094a3ee22932e49dcd01a`. Local `master` was fast-forwarded, and the local and remote `feat/bounded-search` branches plus the implementation worktree were removed.
Session handoff: the reviewed architecture remains in `SEARCH_ARCHITECTURE.md`; implementation, tests, and user documentation are on `master`; `SUMMARY.md` records decisions and deferred evidence-gated work. No known open defect remains.
