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
