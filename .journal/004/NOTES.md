---
id: 004
title: New work session
started: 2026-08-23
---

## 2026-08-23 18:34 — Kickoff
Goal for the session: Start a new journal session and await the developer's substantive request.
Current state of the world: CodeMode's secure source-only Go library and optional Rego authorizer are merged on `master`; no work has been requested for this session yet.
Plan: Receive the developer's request, inspect the relevant repository context, implement it incrementally, and verify the resulting behavior.

## 2026-08-23 18:43 — Functional test plan drafted
Goal for the session: Plan a full manual functional test of every documented CodeMode public contract without using source or repository tests as evidence.
What was done: Inventoried the public README and documentation, grounded OMP's project-local MCP configuration and one-shot CLI workflow, and drafted `FUNCTIONAL_TEST_PLAN.md`.
Current proposal: Use a disposable external Go module with a direct public-API probe, real stdio MCP fixtures configured in project-local `.omp/mcp.json`, official SDK protocol probes, and six bounded one-shot OMP agent assignments. Preserve raw observations, host event order, error leakage canaries, and UX findings.
Next: Obtain the requested adversarial complexity review, apply justified boundedness corrections, then publish the reviewed plan in the journal.

## 2026-08-23 18:54 — Functional test plan complexity-reviewed
Review: The requested `complexity-reviewer` found the three-boundary architecture justified but identified duplicated executions, overbuilt evidence fan-out, an impossible transcript-wide secrecy oracle, multi-server OMP fragility, and missing stop conditions.
What changed: Reduced the OMP harness to one active server and five fixture modes, converted duplicate scenarios into coverage ledgers, narrowed agent work to model-visible UX, corrected request-versus-response canary checks, removed the non-cooperative blocking demonstration, paired code and documentation revisions, consolidated evidence, and added explicit command, retry, reduction, preflight, and onboarding bounds.
Verification: The same reviewer inspected the revision and reported no remaining execution-boundedness blocker after three final retry/preflight/onboarding caps were added.
Result: `FUNCTIONAL_TEST_PLAN.md` is the finalized proposal for developer review. No functional harness or product code was created or executed in this planning task.

## 2026-08-23 19:06 — Functional campaign started
Goal: Execute every row of `FUNCTIONAL_TEST_PLAN.md` through external consumer harnesses, official MCP transports, and real one-shot OMP agents.
Setup: Created disposable harness root `/Users/josh/code/meigma/codemode-functional-004`. Recorded Go `1.26.6` and candidate/oracle commit `aac86bc8cf4bcebb91e9257767fe6c31981847bd`.
Orchestration: Assigned a programmer to build the external public-API probe, stdout-clean stdio MCP fixture host, project-local OMP configuration, prompts, and evidence skeleton. The assignment explicitly forbids reading CodeMode source, tests, or internal packages and forbids changing the product repository.
Next: Validate and smoke-test the harness once, then fan out independent functional-test domains and verify every returned evidence claim.

## 2026-08-23 20:31 — Functional campaign completed
Scope: Executed all 59 plan IDs against candidate/oracle commit `aac86bc8cf4bcebb91e9257767fe6c31981847bd` without using CodeMode source, repository tests, or internal packages as evidence. Surfaces were an external Go module, official in-memory and stdio MCP transports, and fresh one-shot OMP `17.3.4` agents using `openai-codex/gpt-5.6-sol`.

Result: 57 IDs passed independent raw-evidence QA, R03 is partial/blocked, and R04 failed a published documentation contract. This is not an unqualified functional-verification result. No reachable CodeMode library behavior defect was confirmed.

Coverage:

- C01-C09 and D01-D07 pass. The clean tutorial was rerun with `GOTOOLCHAIN=go1.26.6`; registration, binding, builder lifecycle, option-copy isolation, catalog behavior, concurrent immutability, and fresh interpreter state all have discriminating external evidence.
- E01-E17 pass. Supplements removed step/time confounding, established inclusive converted-value depth boundaries, proved cancellation and deadline behavior, and generated the sentinel ledger from raw `errors.Is` records rather than a hardcoded map.
- A01-A02 and R01-R02/R05-R06 pass. Mid-evaluation Rego cancellation reproduced twice: a roughly 5.1-second baseline was canceled at roughly 250 ms with `context.Canceled`; the deadline variant returned `context.DeadlineExceeded`; the authorizer remained reusable.
- M01-M12 pass over the real official MCP transport. Exact schemas, envelopes, resolver ordering, coarse errors, filtering, identity spoof resistance, response-only non-disclosure, and transport lifecycle were independently reconciled against host counters.
- U01-U06 pass after JSON-mode transcript QA. Every accepted run used one fresh process and one active `codemode-test` server. Prompt-compliance replacements are preserved and explicitly superseded in their reports.

Failed and blocked coverage:

- R04 documentation mismatch, reproduced identically by the minimal adjunct: `security-model.md` says a non-nil empty `AllowNet` rejects remote schema references, but a remote schema annotation is accepted and ignored by `rego.New` and authorization succeeds. Runtime network builtins remain unavailable, so this is not evidence of a network escape.
- R03 multiple-result outcome is blocked through the public constructor. `rego.New` requires a ground `data` reference, and the resulting prepared ground query yields at most one result. True, false, undefined, non-Boolean, and builtin-failure outcomes were executed; the defensive multiple-result branch could not be reached honestly.

Confirmed contract and documentation findings:

- MCP `outputSchema` values are advertised as nullable where `mcp-tools.md` promises non-null array/object shapes.
- A duplicate Starlark keyword is rejected at parse time as `ErrInvalidProgram`; `public-api.md` groups duplicate arguments with native binding failures described as `ErrInvalidArguments`.
- `MaxValueDepth` is inclusive and counts a scalar leaf as one level. The public limit description does not document that convention.
- The tutorial's initial `go get` omits the separately imported MCP Go SDK. The later documented `go mod tidy` makes the complete sequence succeed, so this is an ordering/ergonomics issue rather than a broken tutorial.

Confirmed model and developer UX findings:

- The MCP wire description does not expose the required zero-argument `def main()` entry point. Three independent agents repeatedly reached the same coarse `invalid program` error before discovering it.
- Coarse errors preserve the documented non-disclosure boundary but collapse actionable distinctions: missing entry point versus syntax/runtime/final conversion, filtered capability versus malformed source, which resource budget failed, and undefined versus non-Boolean Rego decisions.
- Literal substring discovery makes natural-language searches silently return `[]`; root `Search` trims whitespace and normalizes case while `Describe` requires an exact untrimmed name.
- Generated signatures expose host Go output type names, including unexported identifiers, instead of a stable Starlark result type.
- OMP silently rewrites model-emitted execute argument keys such as `code` or `program` to `source` and can discard extra fields before dispatch. This is an OMP/client behavior, not a CodeMode result; model-side tool-call records alone do not prove the exact MCP arguments received by CodeMode.
- Dotted capability composition and structured final-value conversion worked well once the agent discovered the program entry-point shape.

Evidence corrections: Independent QA rejected initial observations that were non-discriminating, confounded, hardcoded, missing full OMP transcripts, or insufficient to prove fresh concurrent state. Durable supplements replaced each one. Harness defects remain documented in the domain reports; no final PASS relies on a defective probe alone.

Evidence: `/Users/josh/code/meigma/codemode-functional-004/evidence/COVERAGE.md` is the final 59-row ledger. Raw evidence and per-domain reports live under its sibling `runs/` directory. `CAMPAIGN.json` records the toolchain, revision, interval, and verdict counts.

Cleanup: Copied all U01-U06 evidence into the primary evidence tree, then removed compiled harness binaries, both OMP agent clones, and known `/tmp` modules. Retained the source harness and evidence tree so every result remains reviewable and reproducible. The CodeMode product worktree remained unchanged.

## 2026-08-23 21:17 — Findings remediation plan completed
Request: Deliver every campaign finding to a planning agent and store its remediation plan in the bound session journal.
Planning: A read-only `planner` inspected the final ledger, all eleven domain and OMP reports, and the current CodeMode source, tests, dependencies, and documentation. It assigned every finding to CodeMode implementation, documentation, disposable harness, upstream OMP/client, accepted/no-action, retracted/closed, or blocked-contract work.
Decisions: Preserve the three-tool surface, authorization order, deterministic literal search, exact `Describe`, raw query budgeting, and coarse MCP non-disclosure. Remove the unreachable R03 multiple-result promise instead of faking coverage. Correct R04 as a schema/network documentation distinction. Put the required `main()` shape and search workflow on the MCP surface, remove host Go types from signatures, correct advertised output-schema nullability, distinguish trusted-host Rego result errors, repair reusable harness probes, and report OMP argument rewriting upstream without weakening CodeMode schemas.
Verification: The planner returned 36 structured coverage entries with no unresolved item. Exact key symbols and pinned SDK/schema dependencies were checked in the current repository; every proposed target exists. The document contains nine independently reviewable increments, focused verification, acceptance criteria, dependencies, risks, a finding-to-plan matrix, and explicit no-action/out-of-scope decisions.
Result: `.journal/004/FINDINGS_REMEDIATION_PLAN.md`.

## 2026-08-23 21:37 — Increment 1 implemented
Request: Execute Increment 1 of the findings remediation plan with parallel programmer agents and open a pull request.
Implementation: Created `fix/rego-result-contract` from fetched `origin/master` in `.wt/fix-rego-result-contract`. Three parallel programmers split the Rego implementation/tests, root and MCP boundary regressions, and public documentation. `authz/rego.Authorize` now returns `rego: decision is undefined` for an empty OPA result set and `rego: decision must be boolean` for a non-Boolean value; Boolean `false` remains the only `authz.ErrDenied` path. Public docs now state that a ground decision is undefined or yields one value, remove unreachable multiple-result claims, distinguish construction-time from evaluation-time guarantees, and identify surrounding whitespace as OPA parser behavior.
Boundary: New root and official in-memory MCP transport tests use real Rego authorizers for undefined and non-Boolean decisions. Both remain exactly `authorization policy failure`; no handler dispatch or trusted Rego detail crosses either boundary.
Verification: Focused direct, root, and MCP tests passed. `mise exec -- moon run root:check` completed all eight tasks: formatting, lint, build, MCP smoke, race suite, docs install, docs build, and aggregate check. LSP diagnostics reported no issues in the changed Go files. The implementation branch tracks no `.journal` files.
Result: Commit `cb73031` pushed to `fix/rego-result-contract`; PR #16 opened at https://github.com/meigma/codemode/pull/16. GitHub CI and Pages checks were in progress when the PR was opened.

## 2026-08-23 21:53 — Increment 1 merged and Increment 2 implemented
Merge: PR #16 passed CI, GitHub Pages, and Kusari Inspector, then squash-merged to `master` as `057faa8`.
Request: Continue with Increment 2 after merging Increment 1.
Implementation: Created `fix/rego-schema-contract` from the fetched merged `origin/master`. Two parallel programmers split public documentation/Godoc from Rego regression tests. The security model, API reference, and package Godoc now distinguish removed nondeterministic runtime builtins from the empty `AllowNet` deny-all host list. They state that CodeMode installs no schema set or resolver: unconfigured `schema["https://example.invalid/schema.json"]` metadata is accepted but ignored without validation or fetch, while an external `$ref` asks OPA to load a remote schema and is rejected because remote reference loading is disabled.
Tests: Renamed the existing remote-schema test to describe the external `$ref` case and added a real construction-plus-authorization regression for the accepted ignored `schema[...]` form. The nondeterministic-builtin matrix and disabled-print regression remain unchanged.
Verification: The focused four-test schema/builtin/print command passed. `mise exec -- moon run root:check` completed all eight tasks, including formatting, lint, build, MCP smoke, race suite, and docs build. LSP diagnostics reported no Rego package issues. The implementation branch tracks no `.journal` files.
Result: Commit `d216292` pushed to `fix/rego-schema-contract`; PR #17 opened at https://github.com/meigma/codemode/pull/17.
