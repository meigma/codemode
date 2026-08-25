---
id: 012
title: Review model-facing diagnostics
started: 2026-08-25
---

## 2026-08-25 16:47 — Kickoff
Goal for the session: Review GitHub issue #24, which proposes echoing model-derived syntax, resolution, and binding diagnostics through CodeMode's coarse MCP error taxonomy without disclosing host-derived failures.
Current state of the world: `master` includes worker-only execution, widened scalar binding and composite outputs, aggregate native-result limits, and the current fixed model-facing error taxonomy documented in `docs/docs/reference/mcp-tools.md`; issue #24 is open and no implementation work has started in this session.
Plan: Inspect the live issue and relevant projection, execution, binding, MCP, test, and documentation paths; validate the proposed trust boundary against actual error shapes; then report feasibility, required changes, risks, and acceptance-test refinements.

## 2026-08-25 16:57 — Issue review complete
Confirmed the UX defect and the proposed trust principle. A pinned-revision probe observed `syntax.Error` as `<codemode>:2:12: got newline, want primary expression`, `resolve.ErrorList` as `<codemode>:2:12: undefined: sum`, and a binding failure as `*starlark.EvalError` whose message and unwrap chain preserve `invalid capability arguments: unknown argument "keu"` plus model-source call frames.

The issue's projection chain is incomplete after worker-only execution. The child currently reduces every engine failure to a code-only `final_error`; `childPayloadCap` budgets only that fixed frame; the parent maps the code back to a bare execution sentinel before `projectExecutionError` and `projectToolError`. Implementation therefore must update `internal/worker` frame encoding, strict decoding, payload-cap arithmetic, child mapping, and parent mapping in addition to the three named projection sites.

Do not make either public projection echo arbitrary wrapped text. `mcpserver.Service` is a host-implementable port, and existing tests deliberately return host strings wrapped around `ErrInvalidProgram` and `ErrInvalidArguments`; blanket suffix preservation would leak those strings. Carry an internally constructible typed safe diagnostic through execution, the child-owned worker frame, root projection, and MCP projection. Keep the root error string coarse if possible and let MCP format the stable prefix plus trusted safe metadata. Derive program detail only from exact `syntax.Error` and `resolve.ErrorList` types, and binding detail only at the child binding origin; never use `starlark.EvalError.Msg`.

Reject the optional handler/policy/internal call-site suffix in this increment. It conflicts with the acceptance requirement that handler failure remain exactly `capability failed` and with the stated unchanged host-derived classes. Keep runtime evaluator messages, call stacks, handler text, policy diagnostics, panic values, and internal causes absent.

The worker diagnostic field must be legal only for `invalid_program` and `invalid_arguments`, bounded independently from value output using `MaxSourceBytes` plus envelope/JSON escaping overhead, and defensively fall back to the coarse code if an impossible oversized detail occurs. No protocol-version bump is needed under the same-binary worker invariant.

Tests should cover parse, resolve (`sum`), unknown/missing/mistyped binding arguments, a generic runtime `EvalError` whose message is not echoed, worker detail round-trip and caps, custom-service forged wrappers staying coarse, and an actual-root MCP handler returning `db password rejected` with exact `capability failed` and no occurrence in serialized content. Existing policy, panic, and protocol non-disclosure tests remain required.

Documentation must update both `docs/docs/reference/mcp-tools.md` and `docs/docs/explanation/security-model.md`. Replace the fixed-text claim with eleven stable categories/prefixes, acknowledge that exact-string consumers of the two annotated classes will observe a contract change, and add the requested Starlark-not-Python language-surface guidance to both the live tool description and reference recovery section.

Verification: the focused current-boundary suite passed for `internal/execution`, `internal/worker`, and `mcpserver`; the standalone pinned-Starlark probe produced the exact error types and positions above and was removed afterward.
