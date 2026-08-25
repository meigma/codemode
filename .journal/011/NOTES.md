---
id: 011
title: Review capability binding matrix
started: 2026-08-25
---

## 2026-08-25 11:05 — Kickoff
Goal for the session: Review GitHub issue #23, which proposes composite capability outputs and richer scalar inputs while preserving typed authorization and derived discovery.
Current state of the world: `master` has worker-only Starlark execution and per-crossing `MaxValueBytes`; capability binding remains scalar-limited, and prior sessions identified a separate request-scoped intermediate materialization budget as the main unresolved design constraint.
Plan: Inspect the live issue and current binding, worker, discovery, limit, test, and documentation contracts; validate the proposed acceptance cases and failure modes; then report an evidence-backed recommendation and implementation shape.

## 2026-08-25 11:13 — Issue review complete
Issue #23 is valid and should proceed after its implementation notes are refreshed for the worker-only architecture. A disposable public-API reproduction confirmed that `[]struct` output registration fails at `items` and richer scalar input registration fails at the first required `int64`; the focused binding and dispatch tests remained green.

The issue's direct Go-to-Starlark conversion design is stale. Handler output now stays in the parent, becomes process-neutral `map[string]any`/`[]any` data, crosses the type-preserving worker protocol, and only then becomes Starlark in the child. The new compiled reflection walker must therefore produce bounded process-neutral values and preserve `bind -> canonicalize -> authorize -> dispatch`.

Required design corrections:

- Define a structural reflected type universe rather than promise every `encoding/json`-marshalable type. Reject interfaces, custom marshalers, `json.RawMessage`, non-string map keys, functions, channels, and cyclic types at registration; explicitly decide array, byte-slice, named scalar, signed/unsigned integer, and nil slice/map behavior.
- Pass depth and byte-derived node limits into output conversion so slices and maps are preflighted before proportional allocation. Keep exact encoded-size enforcement at the worker frame boundary.
- Add a request-scoped aggregate intermediate-value budget. Per-crossing `MaxValueBytes` permits up to `MaxNativeCalls * MaxValueBytes` of native results, and composite values make that retained child-heap exposure material.
- Keep `FieldShape` non-recursive and extend its deterministic `Type` notation for lists, objects, maps, nullable values, and nested optional keys. A spike using recursive `FieldShape`/`ValueShape` types failed `jsonschema.For` with `cycle detected for type ...fieldShape`, which would prevent `mcpserver.New`.
- Compile immutable output node plans once, preserve exact numeric kinds through normalization, sort runtime map keys, deep-copy any nested discovery state, and report nested registration/conversion paths.

Recommended implementation increments: first settle and spike the reflected output universe, null/omission semantics, notation, and aggregate budget; then widen scalar input binding on both child and authoritative parent paths; then add the compiled output walker and resource accounting; finally prove an end-to-end list/filter/aggregate program, derived discovery, failure classification, immutability, docs, and MCP schema behavior.

## 2026-08-25 11:51 — Implementation plan delegated
A planning agent verified the review against current symbols and produced [`IMPLEMENTATION_PLAN.md`](./IMPLEMENTATION_PLAN.md). The reviewed plan resolves the structural output universe, nil/omission behavior, flat discovery notation, input matrix, conversion-limit placement, and error classifications; it keeps the aggregate intermediate-value default and dispatch-budget plumbing behind a bounded spike rather than inventing unsupported precision.

The plan has five implementation increments: widen scalar inputs; spike and add request-wide native-result accounting while removing duplicate encoding; compile and convert composite outputs; prove discovery plus real same-binary composition; then update all affected documentation. It includes file/symbol targets, focused tests and commands, traceability, risks, non-goals, and a definition of done.

## 2026-08-25 15:29 — Issue implementation complete
Issue #23 was implemented, documented, verified, and closed. Six reviewed pull requests were squash-merged to `master`:

- PR #35 widened required inputs to `string`, `int64`, `bool`, and `float64`, and optional inputs to pointers of those four kinds while preserving child validation, authoritative parent rebinding, canonical authorization arguments, omission semantics, and exact numeric kinds.
- PR #36 added the positive 8 MiB default `MaxIntermediateValueBytes` request budget, debited encoded successful native-result bodies exactly once, kept it independent of per-crossing `MaxValueBytes`, and removed duplicate native-result encoding.
- PR #37 compiled immutable recursive output plans for supported structs, arrays, slices, string-keyed maps, pointers, named scalars, signed and bounded unsigned integers, finite floats, and byte sequences. Conversion preflights depth and allocation-derived node limits, reports nested paths, sorts map keys, and derives deterministic flat `FieldShape.Type` notation.
- PR #38 added the real same-binary MCP list/filter/aggregate proof, derived nested discovery and flat-schema checks, canonical authorization and exact typed-handler assertions, catalog immutability checks, and public capability/resource failure projections.
- PR #39 updated the public API, MCP tool, security model, tutorial, and README contracts for the widened matrix, aggregate budget, parent/worker ordering, nil and `omitempty` behavior, and deterministic notation.
- PR #40 stabilized an existing elapsed-budget test exposed by final race validation without weakening its authorizer, error, deadline, or zero-handler-call assertions.

The final merged-tree validation passed format checking, `go build ./...`, strict MkDocs build, full golangci-lint, `go test ./...`, both real MCP smoke tests, and `go test -race ./... -count=1`. A transient Moon `root:check --force` artifact race was avoided by running the same gates sequentially; the latest GitHub CI run also passed the repository `root:check` graph.
