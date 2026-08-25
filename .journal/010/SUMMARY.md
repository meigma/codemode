---
id: 010
title: Reduce first-touch API ceremony
date: 2026-08-25
status: complete
repos_touched: [codemode]
related_sessions: [008, 011]
---

## Goal

Implement GitHub issue #25's four host-side ergonomics changes without weakening fail-closed authorization, per-invocation subject resolution, the one-shot builder lifecycle, or fresh-process worker execution. Replace the first tutorial's embedded MCP client with a pasteable real stdio server that an agent can call.

## Outcome

Goal met. PR #34 was reviewed, rebased across the completed binding-matrix work from session 011, passed local and GitHub validation, and squash-merged to `master` as `715a716bd6bd5d436368db6367310452960fa8d6`. Issue #25 closed through the merge.

A first server now needs only an explicit authorizer and the mandatory worker entry beyond its capability and transport wiring. The library supplies single-user and trusted-context subject resolvers, bounded defaults for each omitted limit, consolidated registration diagnostics at `Build`, and name-derived capability identity. The exact issue target was exercised over real stdio through `search_api`, `describe_api`, and `execute`, including a fresh worker child.

## Key Decisions

- Keep authorization explicit and fail closed -> `authz.AllowAll()` remains a deliberate host decision; nil authorizers still fail `Build`.
- Add `StaticSubject` only for process-owned single-user identity -> stdio gets an honest low-ceremony resolver while Godoc and user documentation prohibit its use by multi-user hosts.
- Add `ContextSubject` over `authz.WithSubject` and `SubjectFromContext` -> the library owns the private context key while authentication middleware still owns credential validation and identity installation.
- Apply `DefaultLimits()` per zero-valued field during `Build` -> zero remains bounded rather than unlimited, partial overrides remain concise, and negative signed values still fail validation.
- Defer capability-specific registration errors to `Build` -> callers lose repetitive error branches and receive joined diagnostics naming every invalid registration.
- Panic on `Register` with a nil or closed builder -> no future `Build` exists to report those lifecycle violations, so silently retaining or dropping the call would hide programmer misuse.
- Default an omitted capability ID to its name -> simple servers avoid dual identity; public docs require an explicit ID before policy or deployment filters must survive renames.
- Default an omitted description to the summary -> issue #25's exact target omitted `Description`, while the prior validator required it; the fallback makes that stated target runnable without weakening discovery metadata.
- Preserve session 011's widened binding matrix during rebase -> migrated its new composite registration tests to deferred build errors and extended per-field defaulting to `MaxIntermediateValueBytes`.

## Changes

- `authz/authz.go` - added private-key trusted subject context storage and lookup.
- `mcpserver/resolver.go` - added fixed single-user and trusted-context stock resolvers with fail-closed empty-subject behavior.
- `builder.go`, `capability.go`, and `limits.go` - added deferred joined registration errors, ID and description derivation, and bounded per-field defaults for all current limits.
- Root and MCP tests - covered stock resolvers, empty subjects, default and partial limits, joined diagnostics, derived authorization/filter identity, description fallback, and migrated all registration callers to the clean signature cutover.
- `README.md`, `docs/docs/tutorials/first-server.md`, public API and security documentation, how-to guides, and both `example_test.go` files - documented and demonstrated the reduced assembly, real stdio deployment, resolver trust boundaries, and rename tradeoff.
- `docs/docs/reference/mcp-tools.md` - intentionally unchanged because the client-facing MCP contract did not change.

## Open Threads

- No issue #25 work remains. Multi-user hosts must continue to authenticate each request and use `ContextSubject`; `StaticSubject` is intentionally limited to single-user process boundaries.
- A capability that relies on the default ID will change policy and deployment identity when renamed. Hosts that need stable identity must set `Capability.ID` before writing policy or filters.

## References

- [Issue #25 — Reduce first-touch API ceremony](https://github.com/meigma/codemode/issues/25)
- [PR #34 — feat(api): reduce first-touch server ceremony](https://github.com/meigma/codemode/pull/34)
- [Merged commit `715a716`](https://github.com/meigma/codemode/commit/715a716bd6bd5d436368db6367310452960fa8d6)
- [Session 008 worker execution isolation](../008/SUMMARY.md)
- [Session 011 widened binding matrix](../011/SUMMARY.md)

## Lessons

- A runnable target example is an acceptance contract, not illustrative prose. The missing `Description` line exposed a validator mismatch that the four-item issue text did not name.
- Per-field defaulting must evolve with the limit struct. Rebasing over session 011's new aggregate intermediate-result limit required explicitly adding `MaxIntermediateValueBytes` to the resolver.
- Signature cutovers must migrate tests added concurrently on the base branch, not only call sites present when the feature branch started.
