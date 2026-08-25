---
id: 011
title: Review capability binding matrix
started: 2026-08-25
---

## 2026-08-25 11:05 — Kickoff
Goal for the session: Review GitHub issue #23, which proposes composite capability outputs and richer scalar inputs while preserving typed authorization and derived discovery.
Current state of the world: `master` has worker-only Starlark execution and per-crossing `MaxValueBytes`; capability binding remains scalar-limited, and prior sessions identified a separate request-scoped intermediate materialization budget as the main unresolved design constraint.
Plan: Inspect the live issue and current binding, worker, discovery, limit, test, and documentation contracts; validate the proposed acceptance cases and failure modes; then report an evidence-backed recommendation and implementation shape.
