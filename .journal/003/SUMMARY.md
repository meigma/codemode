---
id: 003
title: Add optional Rego authorization
date: 2026-08-23
status: complete
repos_touched: [codemode]
related_sessions: [001, 002]
---

## Goal

Determine whether the policy layer discussed in the original architecture had shipped, then design, prove, implement, document, and land an optional OPA/Rego-backed implementation of CodeMode's existing `authz.Authorizer` port.

## Outcome

Goal met. The existing engine-neutral authorization seam was confirmed, a software-architecture proposal was filtered through an adversarial complexity review, and two disposable spikes proved the required OPA mechanics and measured their cost. The approved root-module design was implemented as `authz/rego`, verified locally and through an external consumer, reviewed in PR #14, and squash-merged to `master` as `aac86bc8cf4bcebb91e9257767fe6c31981847bd`.

The adapter prepares trusted in-memory Rego v1 modules before serving, evaluates one direct ground Boolean `data` decision for every valid native call, preserves the existing trusted input and dispatch ordering, and exposes only `Authorizer`, `New`, and `Authorize`. Core CodeMode and MCP contracts did not change.

## Key Decisions

- Keep OPA behind the existing `authz.Authorizer` port -> the core packages remain policy-engine-neutral and hosts opt in through `authz/rego`.
- Simplify the first public API to `Authorizer`, `New`, and `Authorize` -> option types, module wrappers, a constructor sentinel, reload state, and a synthetic decision probe did not protect useful invariants.
- Query one canonical ground `data` reference directly -> `rego.ResultValue[bool]` then distinguishes `true`, `false`, and invalid decision shapes without binding-query machinery.
- Prepare static trusted policy before building the server -> syntax, unavailable builtins, remote schema references, and invalid queries fail before serving; replacing policy requires constructing a new authorizer and server.
- Remove every OPA-declared nondeterministic builtin, keep `AllowNet` empty, make builtin errors fatal, disable print, and expose no extension hooks -> network, runtime, random, current-time, and signing paths are excluded without claiming hard isolation.
- Keep the adapter in the root module after measuring the dependency cost -> this avoids separate module tags, CI, and release coordination; consumers link OPA only when importing `authz/rego`, while the root module graph still grows.
- Treat policy source as trusted deployment configuration -> mutually untrusted policy authors require a process or container boundary rather than a broader in-process adapter.

## Changes

- `authz/rego/doc.go` - documents the optional in-process adapter and its security boundary.
- `authz/rego/rego.go` - implements deterministic module preparation, restricted OPA capabilities, exact input projection, direct Boolean evaluation, cancellation preservation, and fail-closed authorization.
- `authz/rego/rego_test.go` - covers construction, decisions, trusted input, nondeterministic builtins, remote schemas, builtin failures, cancellation, input ownership, print posture, examples, and concurrent use with real OPA.
- `go.mod` and `go.sum` - pin `github.com/open-policy-agent/opa v1.19.1` and its dependency graph.
- `docs/docs/how-to/use-rego-authorization.md` - shows default-deny Rego v1 policy, static or embedded source, constructor-before-builder wiring, and result handling.
- `docs/docs/reference/public-api.md`, `docs/docs/index.md`, `docs/docs/explanation/security-model.md`, and `docs/mkdocs.yml` - add the fourth public package and its exact API, input, restrictions, cancellation, and containment limits.
- `.journal/003/REGO_ADAPTER_ARCHITECTURE.md` - stores the complexity-reviewed architecture.
- `.journal/003/REGO_ADAPTER_SPIKE.md` - stores executable mechanics, cancellation, dependency, build, and binary-size evidence.

## Open Threads

- GitHub Pages push runs still fail because Pages is not enabled for GitHub Actions. This predates PR #14.
- Release Please push runs still fail because the GitHub App client ID secret is empty. This predates PR #14.
- Policy files, base-data stores, bundles, reload, custom builtins, structured decisions, telemetry, remote OPA, and subject-specific discovery remain intentionally absent. Add one only for a concrete deployment contract.
- OPA remains an in-process dependency. Use process or container containment when policy authors or tenants are mutually untrusted.

## References

- [PR #14 — feat(authz): add Rego authorizer adapter](https://github.com/meigma/codemode/pull/14)
- [Merged commit `aac86bc`](https://github.com/meigma/codemode/commit/aac86bc8cf4bcebb91e9257767fe6c31981847bd)
- [Complexity-reviewed adapter architecture](./REGO_ADAPTER_ARCHITECTURE.md)
- [Disposable spike evidence](./REGO_ADAPTER_SPIKE.md)
- [Session 001 architecture](../001/ARCHITECTURE.md)
- [Session 002 summary](../002/SUMMARY.md)
- [Rego authorization how-to](https://github.com/meigma/codemode/blob/master/docs/docs/how-to/use-rego-authorization.md)
- [Public API reference](https://github.com/meigma/codemode/blob/master/docs/docs/reference/public-api.md)
- [Security model](https://github.com/meigma/codemode/blob/master/docs/docs/explanation/security-model.md)

## Lessons

- Empty `AllowNet` blocks remote schema references and runtime hosts, but it does not make `http.send` fail preparation; removing OPA-declared nondeterministic builtins does.
- `EnablePrintStatements(false)` erases `print()` calls instead of rejecting the policy.
- OPA cancellation errors do not unwrap to `context.DeadlineExceeded`; checking `ctx.Err()` after evaluation preserves CodeMode's existing cancellation classification.
- Embedded OPA is substantial: the spike observed 126 additional modules in the graph, 172 additional compiled non-standard packages, and approximately 29 MiB of linked binary growth in the minimal comparison.
