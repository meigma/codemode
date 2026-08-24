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

## 2026-08-23 22:19 — Increment 2 merged and Increment 3 implemented
Merge: PR #17 passed CI, GitHub Pages, and Kusari Inspector, then squash-merged to `master` as `2f30c5a`.
Request: Continue with Increment 3 after merging Increment 2.
Implementation: Created `fix/public-contract-edges` from the fetched merged `origin/master`. Four parallel programmers split the public references/security explanation, duplicate-keyword execution regression, inclusive-depth binding regression, and first-server tutorial. Documentation now separates parser-level duplicate keywords (`ErrInvalidProgram`) from native argument binding (`ErrInvalidArguments`), defines inclusive value depth, preserves raw-before-normalization search budgeting, makes exact non-normalizing `Describe` lookup explicit, and records nil `Server.Execute` context as a caller-contract violation classified `ErrInternal`.
Tutorial: The dependency command now installs both CodeMode at `master` and the official MCP Go SDK before the program is introduced. `go mod tidy` is described and retained as final module cleanup.
Verification: Focused execution and binding tests passed. A clean external Go 1.26.6 module ran the tutorial commands in order with the unmodified fenced program: both dependencies installed, `go build .` succeeded before tidy, `go mod tidy` succeeded, and `go run .` returned successful search, describe, and execute results. `mise exec -- moon run root:check` completed all eight tasks. LSP diagnostics reported no issues in the changed Go files. The disposable module was removed, and the implementation branch tracks no `.journal` files.
Result: Commit `1aec320` pushed to `fix/public-contract-edges`; PR #18 opened at https://github.com/meigma/codemode/pull/18.

## 2026-08-23 22:39 — Increment 3 merged and Increment 4 implemented
Merge: PR #18 passed CI, GitHub Pages, and Kusari Inspector, then squash-merged to `master` as `253323d`.
Request: Continue with Increment 4 after merging Increment 3.
Implementation: Created `feat/mcp-authoring-guidance` from the fetched merged `origin/master`. Two parallel programmers split the live MCP surface from its reference documentation. The three `tools/list` descriptions now direct models to use short literal search terms with shorter-term retry, pass exact discovered names to `describe_api`, and execute a zero-argument `def main():` that calls only discovered and described capabilities inside `main` and returns the final value. The MCP reference records the same authoring path plus recovery after resource limits and the per-call, non-disclosing meaning of policy outcomes. Error payloads, tool names, schemas, handlers, and execution behavior remain unchanged.
Tests: `TestNewRegistersExactlyThreeTools` now obtains descriptions through the official in-memory MCP client and asserts durable authoring cues rather than complete prose.
Functional verification: A raw stdio MCP initialize and `tools/list` exchange exposed exactly the three expected descriptions. Three fresh blind OMP JSON-mode agents then completed the onboarding lookup, one-program multi-capability composition, and three-error recovery cases using only the public MCP tools. They selected short discovery terms, used exact discovered names, authored zero-argument `main` programs, and recovered from `capability not found`, `invalid capability arguments`, and `invalid program` without repository or host-log inspection. No description iteration was required.
Verification: The focused listed-description test passed. `mise exec -- moon run root:check` completed all eight tasks, including formatting, lint, build, MCP smoke, race suite, and docs build. LSP diagnostics reported no issues in `mcpserver/*.go`. The disposable branch-built stdio/OMP harness was removed, and the implementation branch tracks no `.journal` files.
Result: Commit `ba6420f` pushed to `feat/mcp-authoring-guidance`; PR #19 opened at https://github.com/meigma/codemode/pull/19. GitHub CI was in progress and GitHub Pages had passed when the PR was checked.

## 2026-08-24 09:48 — Increment 4 merged and Increment 5 implemented
Merge: PR #19 passed CI, GitHub Pages, and Kusari Inspector, then squash-merged to `master` as `f1d3736`.
Request: Continue with Increment 5 after merging Increment 4.
Implementation: Created `fix/mcp-signature-contract` from the fetched merged `origin/master`. Three parallel programmers split binding, catalog/MCP regressions, and public references. `binding.Plan.Signature` now ends after the parameter list and no longer renders ` -> <GoType>`. Dotted capability names, keyword-only markers, parameter order, field names, and input type notations remain unchanged. `Plan.outputType` remains available for conversion.
Contract and migration: `signature` is invocation-only. `Description.Output` and `describe_api.output` are the stable result contracts. The references tell clients that parsed the removed arrow suffix to stop and use output field shapes; there is no compatibility suffix, alias, or alternate field.
Tests: Binding, catalog, and official in-memory MCP regressions register exported and unexported Go output structs. They prove the identifiers appear in neither Search nor Describe, including MCP structured content and JSON text mirrors, while the existing described keyword-only signature still drives a successful execute program.
Verification: Focused binding, catalog, and MCP tests passed. An external Go 1.26.6 smoke consumer registered exported and unexported outputs and observed only `records.lookup(*, key: str)` and `health.status()` plus stable output field shapes. `mise exec -- moon run root:check` completed all eight tasks, including formatting, lint, build, MCP smoke, race suite, and docs build. LSP diagnostics reported no issues in binding, catalog, or MCP packages. The disposable smoke module was removed, and the implementation branch tracks no `.journal` files.
Result: Commit `0683883` pushed to `fix/mcp-signature-contract`; PR #20 opened at https://github.com/meigma/codemode/pull/20. GitHub CI, Pages, and inspection checks were queued when the PR was checked.

## 2026-08-24 10:25 — Increment 5 merged and Increment 6 implemented
Merge: PR #20 passed CI, GitHub Pages, and Kusari Inspector, then squash-merged to `master` as `8b5302b`.
Request: Continue with Increment 6 after merging Increment 5.
Implementation: Created `fix/mcp-output-schema-contract` from the fetched merged `origin/master`. Three parallel agents split the MCP adapter, regressions, and reference documentation. All three tools now advertise explicit `OutputSchema` values inferred from their existing Go output types through `jsonschema.For`. The `search_api` root and `describe_api` `input` and `output` properties are narrowed from nullable slices to non-null arrays without duplicating the inferred item, property, or required-field schemas. Successful nil service slices are normalized to `[]`; non-empty values, errors, tool names, inputs, descriptions, and execute payloads are unchanged. `github.com/google/jsonschema-go v0.4.3` is now a direct dependency.
Tests and docs: Official in-memory MCP regressions inspect every `tools/list` output schema and prove nil, empty, and populated search results plus empty description field shapes match both `StructuredContent` and the JSON `TextContent` mirror. The real secure-loop test now asserts the same non-null schemas and runtime arrays. The MCP reference identifies the displayed schemas as the advertised `outputSchema` values and states that zero results or fields use `[]`, not `null`.
Functional verification: A disposable external Go 1.26.6 server exposed CodeMode over the SDK's raw stdio transport. Direct JSON-RPC initialize, `tools/list`, empty `search_api`, and zero-field `describe_api` exchanges observed array schemas and `[]` in both structured content and text mirrors. The harness was removed.
Verification: The focused schema/runtime tests passed. `mise exec -- moon run root:check` completed all eight tasks after the initial lint findings were corrected, including formatting, lint, build, MCP smoke, race suite, and docs build. LSP diagnostics reported no issues in `mcpserver/*.go`. `git diff --check` passed, and the implementation branch tracks no `.journal` files.
Result: Commit `ebf9aff` pushed to `fix/mcp-output-schema-contract`; PR #21 opened at https://github.com/meigma/codemode/pull/21. GitHub CI, Pages, and inspection checks were queued or in progress when the PR was checked.
