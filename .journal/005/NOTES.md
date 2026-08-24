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

## 2026-08-24 10:31 — Issues filed
Filed the two review outcomes as GitHub issues:
- #23 Widen the capability binding matrix: composite outputs and richer scalar inputs — keep Capability[Input, Output] API, replace fieldKind whitelist with generic Go->Starlark walker (ConvertFinal proves the pattern), inputs widen but stay typed pre-authorization, describe_api shapes stay reflection-derived. Non-goals: JSON text passthrough, starlarkjson exposure.
- #24 Echo model-derived diagnostics through the coarse error taxonomy — type-selective pass-through of syntax positions, binding argument names, optional call-site line; host-derived text (handler/policy/panic) stays hidden; sentinel text kept as stable prefix. Danger note recorded: EvalError.Msg embeds wrapped handler cause, so no blanket pass-through.
Discussion context: user challenged whether typed output translation is unnecessary given JSON at the MCP boundary; conclusion was the whitelist is the dispensable part, not the translation (Starlark needs values, not JSON text; no json module predeclared; describe_api schema derivation depends on typed Output).
Next: await direction on whether to implement either issue this session.
