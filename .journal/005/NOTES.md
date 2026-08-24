---
id: 005
title: New work session
started: 2026-08-24
---

## 2026-08-24 10:09 — Kickoff
Goal for the session: not yet stated; the developer asked to start a new journal session and has not made a substantive request.
Current state of the world: sessions 001–003 are complete (secure MVP, source-only cutover, optional Rego authorizer all merged to `master`); session 004 is in-progress in parallel. Local `master` is 5 behind `origin/master`. Known pre-existing CI blockers: GitHub Pages not enabled for Actions, Release Please app client ID secret empty.
Plan: await the developer's request, then scope and execute it under the session protocol.

## 2026-08-24 10:22 — UX review checkpoint
Goal became: critical product-owner UX review of CodeMode (niceness, power, safety, learnability).
Fast-forwarded repo master aac86bc -> 8b5302b (5 commits: invalid-Rego-decision fix, doc contract fixes, model authoring guidance, invocation-only signatures).
Read: capability/builder/server/limits/errors + example, internal/binding (plan, signature), internal/catalog (search, describe), internal/execution (engine, execute, namespace), mcpserver adapter, README, first-server tutorial, mcp-tools reference.
Key findings forming: safety layer is the strongest dimension; binding matrix (string + *int64 in, flat scalar structs out, no lists) undercuts the core Code Mode composition thesis; coarse error taxonomy discards model-supplied diagnostics (Starlark parse position, failing argument name) that could be safely echoed; host boilerplate (resolver pattern, per-Register error checks) is repetitive.
Next: deliver review in chat; no code changes this checkpoint.
