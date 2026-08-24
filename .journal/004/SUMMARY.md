---
id: 004
title: Functional verification and findings remediation
date: 2026-08-24
status: complete
repos_touched: [codemode, oh-my-pi]
related_sessions: [001, 002, 003]
---

## Goal

Design and execute a source-independent manual functional test of every documented CodeMode public contract, turn every grounded finding into an explicit remediation decision, and land the justified product, documentation, harness, and upstream changes.

## Outcome

Goal met. A complexity-reviewed plan drove 59 public-contract cases through an external Go consumer, official in-memory and stdio MCP transports, and fresh OMP agents. Independent evidence QA produced 57 initial passes, one unreachable Rego branch, and one documentation mismatch; it also identified contract, model-UX, harness, and OMP/client findings without confirming a reachable CodeMode runtime defect.

Every finding was then assigned and executed through nine increments. Seven CodeMode increments squash-merged in PRs #16–#22, the disposable campaign harness was repaired and reverified outside the product repository, and OMP argument rewriting was reported upstream as issue #9653. Final `master` is `ccac0332a64c52530ebf0102849f73eecf867a12`.

## Key Decisions

- Test the public module and MCP wire from an external harness without reading CodeMode source or repository tests as evidence -> the campaign measures shipped consumer behavior rather than implementation assumptions.
- Keep the three-tool surface, literal discovery, exact `Describe`, authorization order, raw query budgeting, and coarse MCP errors -> observed behavior matched the intended deterministic and non-disclosing contracts; better in-band authoring guidance addressed model friction without widening disclosure.
- Remove the unreachable Rego multiple-result promise -> `rego.New` accepts a ground `data` reference, whose prepared query yields zero or one value; fake coverage would misstate the public constructor.
- Distinguish trusted Rego result diagnostics from root and MCP projections -> direct hosts can act on undefined and non-Boolean decisions while clients continue to receive only `authorization policy failure`.
- Treat `signature` as invocation syntax only and `Description.Output` as the stable result contract -> host Go output type names no longer leak into discovery, with no compatibility suffix or alias.
- Advertise explicit MCP output schemas and normalize successful nil collections to `[]` -> the wire schema and runtime values now match the published non-null contracts.
- Keep harness corrections in the disposable external tree -> campaign mechanics do not become product dependencies or CI weight.
- Report OMP normalization upstream rather than weakening CodeMode schemas -> `code`/`program` rewrites and dropped fields are client behavior; strict server validation remains authoritative.

## Changes

- `.journal/004/FUNCTIONAL_TEST_PLAN.md` - records the complexity-reviewed 59-case campaign, evidence rules, stop conditions, and OMP workflow.
- `.journal/004/FINDINGS_REMEDIATION_PLAN.md` - classifies every campaign finding and defines the nine executed increments.
- `authz/rego/rego.go`, package documentation, and regressions - distinguish undefined and non-Boolean decisions, remove unreachable multiple-result claims, and make schema/network guarantees exact.
- `server.go`, `mcpserver/server.go`, and boundary regressions - document and preserve trusted root causes, sentinel projection, fixed MCP errors, panic recovery, and response-only non-disclosure.
- `internal/binding`, catalog regressions, and public references - remove Go output types from generated signatures and direct consumers to output field shapes.
- `mcpserver` schemas and tests - advertise exact non-null output schemas and normalize empty successful collections.
- MCP tool descriptions and `docs/docs/reference/mcp-tools.md` - publish short-query discovery, exact-name description, zero-argument `main()`, recovery, and policy-inference guidance.
- Public API, security, and tutorial documentation - correct duplicate-keyword classification, inclusive value depth, exact/raw search behavior, nil-context behavior, Rego schema semantics, and dependency installation order.
- `/Users/josh/code/meigma/codemode-functional-004` - retains the repaired external harness, prompts, raw evidence, reports, and final coverage ledger; generated binaries and temporary workspaces were removed.
- `can1357/oh-my-pi#9653` - reports silent MCP argument renaming and dropped fields with emitted-versus-dispatched transcript evidence and direct SDK controls.

## Open Threads

- [OMP issue #9653](https://github.com/can1357/oh-my-pi/issues/9653) remains open. CodeMode must not add aliases, relax `additionalProperties`, or infer rewritten arguments while upstream decides whether to dispatch exactly or disclose normalization.
- The retained 59-case campaign targets pre-remediation commit `aac86bc8cf4bcebb91e9257767fe6c31981847bd`. Each remediation received focused tests and functional checks, and repaired harness cases were rerun, but the complete campaign was not repeated as a new final-master baseline.
- Worker isolation, richer binding/search, observability, fuzzing, benchmarks, and Testcontainers remain evidence-gated rather than implied follow-up work.

## References

- [PR #16 — fix(authz): distinguish invalid Rego decisions](https://github.com/meigma/codemode/pull/16)
- [PR #17 — docs(authz): correct Rego schema guarantees](https://github.com/meigma/codemode/pull/17)
- [PR #18 — docs: correct public contract edges](https://github.com/meigma/codemode/pull/18)
- [PR #19 — feat(mcp): publish model authoring guidance](https://github.com/meigma/codemode/pull/19)
- [PR #20 — fix(api): make capability signatures invocation-only](https://github.com/meigma/codemode/pull/20)
- [PR #21 — fix(mcp): advertise exact output schemas](https://github.com/meigma/codemode/pull/21)
- [PR #22 — docs(security): define exact error boundary](https://github.com/meigma/codemode/pull/22)
- [OMP issue #9653](https://github.com/can1357/oh-my-pi/issues/9653)
- [Functional test plan](./FUNCTIONAL_TEST_PLAN.md)
- [Findings remediation plan](./FINDINGS_REMEDIATION_PLAN.md)
- [Session 003 summary](../003/SUMMARY.md)

## Lessons

- Model-emitted tool arguments and MCP-dispatched arguments are separate facts. OMP JSON transcripts must compare `tool_execution_start.args.content` with `tool_execution_end.result.details.xdev.args`; the former is not wire truth.
- A prepared ground Rego decision can be undefined or return one value, but cannot honestly exercise a multiple-result branch through the current constructor.
- An unconfigured Rego `schema[...]` annotation is accepted and ignored; an external schema `$ref` requests loading and is rejected. Empty `AllowNet` is not a general annotation validator.
- Converted-value depth is inclusive and scalar-based: a scalar is depth 1 and a one-level object is depth 2.
- Explicit SDK output schemas require runtime normalization at the adapter boundary; schema-only narrowing would make successful nil slices invalid.
- Proactive tool descriptions can materially improve model behavior without adding diagnostic payload detail or weakening the security boundary.
