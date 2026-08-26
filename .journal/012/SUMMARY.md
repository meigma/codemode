---
id: 012
title: Review model-facing diagnostics
date: 2026-08-25
status: complete
repos_touched: [codemode]
related_sessions: [006, 011]
---

## Goal

Review issue #24 against CodeMode's worker-isolated execution path and non-disclosure boundary. Implement the approved model-derived diagnostic channel without exposing host-derived errors.

## Outcome

Goal met. CodeMode now returns source positions and parser or resolver messages after the stable `invalid program` prefix, and binding messages after `invalid capability arguments`. Handler, policy, panic, generic runtime, resource, protocol, and internal failures remain coarse. PR #43 passed local and GitHub validation, was squash-merged to `master`, and closed issue #24.

## Key Decisions

- Carry approved detail in a private `internal/execution` wrapper whose `Error()` remains coarse. This preserves `errors.Is` classification without making arbitrary wrapped text model-visible.
- Accept only direct ordinary `syntax.Error`, direct `resolve.ErrorList`, and binding diagnostics captured at the child-side binding origin. Never read `starlark.EvalError.Msg`, and reject parser panic-recovery messages prefixed with `internal error:`.
- Allow worker `final_error.detail` only for `invalid_program` and `invalid_arguments`. Cap it independently at 4 KiB, reject malformed wire values, and fall back to code-only output before an oversized write.
- Keep root error strings coarse. Format `<sentinel>: <detail>` only at the MCP projection, where the model-facing contract applies.
- Do not add call-site positions to host-derived failures. Exact `capability failed` behavior is part of the acceptance boundary.
- Treat the worker connection's terminal state as authoritative after `native_abort`. The execution engine intentionally coarsens the private unwind error.

## Changes

- `internal/execution/` — added private safe-detail provenance, selective parser/resolver/binding extraction, and coarse-classification tests.
- `internal/worker/` — added bounded optional diagnostic framing, strict decoding, child/parent transport mapping, payload-cap accounting, and native-abort state handling.
- `server.go` and `server_test.go` — preserved approved metadata through the root projection while retaining coarse public error text.
- `mcpserver/` — formatted the two approved stable prefixes, kept custom service failures coarse, updated the live `execute` description, and added an actual MCP non-disclosure canary.
- `docs/docs/reference/mcp-tools.md` and `docs/docs/explanation/security-model.md` — documented diagnostic recovery, Starlark language cues, compatibility, and the precise security exception.

## Open Threads

- None for issue #24.

## Lessons

- A direct parser error type is not sufficient provenance by itself: Starlark scanner recovery can convert an internal panic into `syntax.Error`. The message prefix must remain part of the rejection gate.
- Worker protocol budgets cannot derive every binding diagnostic from submitted source size because required field names come from registered capability shapes. A fixed private diagnostic cap is simpler and complete.
- Engine error classification can erase a private callback sentinel. Protocol state must carry terminal parent-owned abort information independently.

## References

- [Issue #24: Echo model-derived diagnostics through the coarse error taxonomy](https://github.com/meigma/codemode/issues/24)
- [PR #43: feat(diagnostics): echo safe model-derived error detail](https://github.com/meigma/codemode/pull/43)
- Merged commit `4bbb36734256501a8e7d204b556addd8c65e9350`
- [Session 006 worker execution design](../006/SUMMARY.md)
- [Session 011 capability binding matrix](../011/SUMMARY.md)
