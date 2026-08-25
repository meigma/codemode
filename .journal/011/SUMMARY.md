---
id: 011
title: Widen capability binding matrix
date: 2026-08-25
status: complete
repos_touched: [codemode]
related_sessions: [006, 007, 008]
---

## Goal

Review issue #23 against the worker-only execution architecture, determine a safe implementation shape, and implement composite capability outputs plus richer scalar inputs without weakening authorization, discovery, or resource limits.

## Outcome

Goal met. Issue #23 was validated, redesigned for the parent/worker boundary, implemented in five reviewed increments, documented, verified, and closed. Required and optional scalar inputs now bind exactly; handlers can return bounded structural composite values; discovery remains derived and deterministic; and a request-scoped aggregate budget limits successful native-result bodies. A real same-binary MCP program proves one native call can return a collection that Starlark filters and aggregates into a compact digest.

PR #40 also stabilized an existing elapsed-budget test flake found during final race validation. All six PRs were squash-merged to `master`, the local default branch was fast-forwarded, and their Worktrunk worktrees were removed.

## Key Decisions

- Use a restricted structural reflection universe instead of promising every `encoding/json`-marshalable type -> derived discovery remains exact, while interfaces, custom marshalers, cyclic graphs, and unsupported map keys fail at registration.
- Convert handler outputs to bounded process-neutral values in the parent, then convert those values to Starlark in the worker -> preserves the worker-only architecture and type-preserving protocol.
- Keep `FieldShape` flat and encode nesting in deterministic `Type` strings -> recursive schema types caused `jsonschema.For` cycle failures and would prevent MCP server construction.
- Preserve worker binding followed by authoritative parent rebinding and canonicalization, authorization, and dispatch -> policy evaluates the same validated typed arguments the handler receives.
- Add `MaxIntermediateValueBytes` independently of `MaxValueBytes` -> per-crossing limits do not bound cumulative successful native results retained during one execution.
- Debit aggregate result bytes after one successful encoding and before frame write -> avoids duplicate encoding and counts exactly the value body that crosses to the worker.
- Treat nil pointers, nil containers, and `omitempty` explicitly -> runtime null and omission behavior remains predictable and appears accurately in derived notation.

## Changes

- `internal/binding/` - widened scalar input compilation and canonical binding; added immutable recursive output plans, bounded conversion, nested path diagnostics, deterministic notation, and structural type rejection.
- `internal/worker/`, `dispatch.go`, `limits.go`, and `server.go` - added request-scoped aggregate native-result accounting, threaded the remaining budget through parent dispatch, and preserved coarse public error projection.
- `internal/catalog/`, `mcpserver/`, and root tests - proved sorted immutable discovery, the unchanged flat MCP schema, public registration and runtime failures, and real same-binary list/filter/aggregate composition.
- `moon.yml` - added the composite MCP program to the smoke gate.
- `README.md` and `docs/docs/` - documented the widened matrix, deterministic notation, nil and `omitempty` behavior, aggregate budget, error classifications, and parent/worker ordering.
- `server_test.go` - increased the elapsed-budget test window so race-instrumented worker startup reliably reaches the required authorizer call without weakening assertions.

## Open Threads

- Five unrelated local `master` edits were stashed before the required fast-forward. Stash commit `ccde4bcd9cd627d57e2d284bfde0811ebb71eae8` is labeled `session-close: preserve unrelated master changes before closing 011`; its owner must restore and reconcile it.
- `moon run root:check --force` exposed a transient race between strict documentation output replacement and golangci-lint scanning `docs/build`. Standard GitHub `root:check` passed, and the same local gates passed sequentially. No issue was filed during this session.
- Composite capability inputs remain intentionally unsupported and evidence-gated.

## References

- [Issue #23: Widen the capability binding matrix](https://github.com/meigma/codemode/issues/23)
- [PR #35: widen scalar capability inputs](https://github.com/meigma/codemode/pull/35)
- [PR #36: bound intermediate native results](https://github.com/meigma/codemode/pull/36)
- [PR #37: support composite capability outputs](https://github.com/meigma/codemode/pull/37)
- [PR #38: prove composite capability programs](https://github.com/meigma/codemode/pull/38)
- [PR #39: document widened capability matrix](https://github.com/meigma/codemode/pull/39)
- [PR #40: stabilize elapsed-budget coverage](https://github.com/meigma/codemode/pull/40)
- [`IMPLEMENTATION_PLAN.md`](./IMPLEMENTATION_PLAN.md)
