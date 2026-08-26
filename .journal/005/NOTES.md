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

## 2026-08-24 10:44 — Filed first-touch DX issue
Filed #25: reduce first-touch API ceremony in one PR. Four changes: (1) stock resolvers mcpserver.StaticSubject + mcpserver.ContextSubject with exported authz.WithSubject/SubjectFromContext, (2) per-field zero-value Limits defaulting at Build, (3) Register stops returning error and Build joins all registration failures, (4) ID defaults to Name (flagged rename-drift tradeoff; droppable item). Deliberately kept: explicit AllowAll, no Serve() helper, no registration sugar. Target: ~50-line real stdio server replacing the ~180-line in-memory tutorial. Session outcomes so far: #23 binding matrix, #24 error diagnostics, #25 host DX.

## 2026-08-24 (later) — Re-verified #25 after worker cutover
Parallel sessions landed PRs #26–#31: fresh-process worker execution (ServeWorkerAndExit required as first statement of main, Build runs a same-executable worker probe), MaxResultBytes -> MaxValueBytes, new MaxConcurrentExecutions, MaxExecutionTime now covers spawn+exchange. Issues #12/#13 closed by that work.
Re-verified all four #25 items against 3b541fc: still applicable and unimplemented (empty ID still rejected in ValidateRegistration, zero Limits still fail Validate, Register still returns error, no stock resolvers in mcpserver/authz).
Amended #25 body: target example gains codemode.ServeWorkerAndExit(), Limits text updated to nine-field shape, ServeWorkerAndExit added to deliberately-stays; left explanatory comment on the issue.

## Fresh UX review at a5031b5
Merged since last look: #32–#41. Issues #23 (binding matrix) and #25 (first-touch ceremony) implemented and closed; #24 still open.
Functional smoke performed (not just reading): built the tutorial-shaped stdio server verbatim against the local checkout plus one composite-output capability (list of structs), drove it with a real MCP client over CommandTransport.
Verified live: search/describe show composite notation list[{key: str, size: int, public: bool, score: float}]; the Code Mode thesis program (fetch collection -> comprehension filter -> chained per-item native calls -> digest) returns correct results through fresh worker processes; zero-config Limits, void Register with joined Build errors, ID/Description defaults, StaticSubject all work as documented.
New findings: (1) my first program failed on Starlark-missing sum() with bare "invalid program" — empirical demonstration of #24; (2) PR #22 formalized non-disclosure of source locations as a documented contract, so #24 now requires a deliberate contract amendment; (3) no model-facing statement of the Starlark language surface (builtin list) anywhere — cheap doc fix. Commented all three on #24.
Remaining open UX threads: #24, composite inputs (deferred phase 2 of #23), ten-field Limits growth (acceptable, hidden by defaults).

## Filed stdlib issue
Filed #42: fixed pure-compute stdlib in the worker universe (sum + lib/json + lib/math), always-on with no knob. Rationale recorded: one language not dialects, nothing to opt out of (pure compute under existing budgets), zero new go.mod entries (lib/json + lib/math ship in the pinned go.starlark.net revision; spec-standard modules). Exclusions: lib/time (nondeterminism), regex, filter/map, host preludes. Flagged coordination with in-flight #24 agent: single documented language surface + drift test asserting docs match actual universe; also flagged namespace-collision registration check (json/math/sum as capability namespace roots).

## Reviewed diagnostics + stdlib + search merges at 344b79b
PRs #43 (closes #24), #44 (relevance-ranked search, new), #46 (closes #42) merged; new follow-up issue #45 (json stdlib Go-recursion not step-budgeted; worker contains but error goes coarse-internal).
Re-ran the smoke suite live against a rebuilt server:
- Original sum() composite program now succeeds verbatim ({"kept":1,...,"total":10}).
- Diagnostics: syntax error -> "invalid program: <codemode>:1:11: got ':', want ')'"; unknown keyword -> "invalid capability arguments: unknown argument \"keu\""; undefined name -> "invalid program: <codemode>:2:9: undefined: time".
- Stdlib: json round-trip and math.sqrt work; time correctly undefined.
- Non-disclosure held: handler error with secret string projects to bare "capability failed".
- search_api output shape changed (breaking): {"results": [...], "truncated": false} wrapper with relevance ranking.
All three of this session's filed issues (#23 via earlier merge, #24, #42) plus #25 are now implemented and verified end-to-end.

## README rewrite PR
Rewrote README per user direction: plain-language intro (agents write code instead of chaining tool calls), differentiator list (one-round-trip composition, type-derived schemas, per-deployment disabling, fail-closed authz + Rego, fresh worker processes), install pinned to v0.1.0 ahead of first release, security boundary compressed to the contract with links, dropped SearchTerms/TestMain digressions. Work done in worktree docs/readme-intro; master checkout restored clean. PR #47 open, all checks green. Merge note: @v0.1.0 resolves only after the tag exists.

## README Get started made code-first
Per user feedback on PR #47: Get started now leads with the annotated main() (worker entry, no-default-authorizer, schema-from-types, StaticSubject vs ContextSubject notes moved into Go comments), tutorial linked below the code, prose paragraphs removed. Pushed 2ca6da0 to docs/readme-intro.

## PR #47 merged
Squash-merged as 502c363 (docs: rewrite README around differentiators). Local master fast-forwarded; docs/readme-intro worktree and branch removed.

## Cloudflare comparison PR
PR #48: README section "How this differs from Cloudflare's Code Mode" — same-thesis/opposite-sides framing, agent-side TS-in-isolates vs server-side typed Go + local worker, grounded in their blog post. Per user review of the draft: dropped the composability closer, rewrote the policy sentence to plain mechanism ("authorized before it dispatches", "disable capabilities it does not want to expose") removing the LLM-shaped aphorism/contrast.
