# Issue #23 implementation plan

## Verified current architecture

- `Register` (`builder.go`) calls `binding.CompileFor`, then stores the immutable `*binding.Plan` in catalog registration.
- `catalog.Build` (`internal/catalog/build.go`) derives signatures and input/output `FieldShape` slices once and sorts enabled capabilities by name.
- The child receives only IDs, names, and input shapes (`server.go::capabilityBindings`). Handlers and output conversion stay in the parent.
- Child execution binds with `binding.BindShape` in `internal/execution/execute.go::callCapability`, then sends a normalized `native_call`.
- Parent `dispatch.go::dispatcher.dispatch` performs authoritative `Plan.BindValue` -> fresh canonical map -> `authorize` -> typed `invoke` -> `Plan.ConvertOutput` -> `binding.ValidateValue`.
- `internal/worker/parent.go::handleNative` writes the normalized native result. The child converts it with `binding.ToStarlark` through `execution.wrapNativeCall`. Direct parent Go-to-Starlark conversion would violate the worker boundary.
- `binding.ConvertFinal` is absent. `ValidateValue`, `FromStarlark`, and `ToStarlark` in `internal/binding/value.go` define the process-neutral nil/bool/string/int64/finite-float64/list/object domain and preflight container lengths.
- `MaxValueBytes` is exact per crossing at the worker frame boundary and is also used as a conservative node/materialization bound. There is no request-wide native-result budget.
- `mcpserver.New` derives `describe_api`’s schema with `jsonschema.For[codemode.Description]`; recursive discovery shapes fail schema inference. Keep `FieldShape` flat.

## Resolved contracts

### Output universe

Root `Input` and `Output` remain non-pointer structs. Compile nested output types once at registration into a flat immutable node arena referenced by indexes; runtime conversion follows compiled nodes and does not repeat reflection type analysis.

Accept nested:

- exported, non-embedded structs using the existing strict single-`json`-tag rules at every level;
- arrays and slices, converted to `[]any`/Starlark lists;
- maps with string-kind keys, including named string aliases; sort keys before value traversal;
- pointers to supported nodes;
- named scalar aliases;
- strings, bools, all signed integer kinds, `uint`/`uint8`/`uint16`/`uint32`/`uint64`, float32/64;
- byte slices/arrays as lists of integers 0–255, not base64.

Normalize integers to int64. Reject runtime unsigned values above `math.MaxInt64`. Normalize floats to float64 and reject NaN/Inf. Reject `uintptr`.

Reject at registration anywhere in the graph: interfaces/`any`, `json.RawMessage`, value or pointer method sets implementing `json.Marshaler` or `encoding.TextMarshaler`, funcs, channels, complex/unsafe pointers, non-string map keys, embedding/unexported fields, ignored/additional/unsupported tags, and cyclic type graphs through any container.

### Optional/null semantics

- Pointer without `omitempty`: required key; nil -> `None`; discovery type `T | None`.
- Pointer with `omitempty`: optional key; nil omits it; non-nil includes dereferenced value. Reject `omitempty` on non-pointer output fields.
- Nil slice/map -> `None`; non-nil empty slice/map -> empty list/object.
- Nil pointer inside a list/map -> `None`; omission applies only to struct fields.
- Omitted optional inputs and explicit `None` both produce nil pointers and omit the canonical authorization key.

### Flat deterministic discovery notation

Keep `FieldShape{Name, Type, Required}` unchanged. Extend only `Type` strings:

- `str`, `int`, `bool`, `float`
- `list[T]` for arrays/non-nil slices
- `dict[str, T]` for non-nil maps
- `{field: T, optional?: U}` for structs, in declaration order
- append ` | None` once for nullable values

Root optionality remains `FieldShape.Required`; nested optionality uses `?`. Lock exact spacing/punctuation in tests. Arrays intentionally share `list[T]` with slices because Starlark sees the same runtime category. Do not add recursive `FieldShape`/`ValueShape`, custom capability schemas, or hand-authored JSON Schema.

### Input widening

Accept exact child and parent pairs:

| Go field | Shape | Required |
|---|---|---|
| string | `str` | true |
| int64 | `int` | true |
| bool | `bool` | true |
| float64 | `float` | true |
| *string | `str | None` | false |
| *int64 | `int | None` | false |
| *bool | `bool | None` | false |
| *float64 | `float | None` | false |

Named aliases with those underlying kinds remain valid. Integers require signed-64 range; floats must be finite on both child and authoritative parent paths. Do not coerce int to float or bool to int. Required fields reject `omitempty`; optional pointer inputs may use it but do not require it.

### Limit placement and error classification

Change `Plan.ConvertOutput` to take positive depth and materialization-node limits. Root object is depth 1; nested struct/list/map adds one; pointers add no depth. Before allocating destination maps/lists/sorted-key slices, preflight child count against remaining nodes. Count included optional struct fields before allocating. Handler-owned Go allocation is outside CodeMode; the guarantee covers CodeMode destination allocation.

Keep exact encoded-size enforcement at the worker frame writer.

- Depth/node/per-value-byte/aggregate exhaustion -> resource limit.
- Runtime unsigned overflow/non-finite float/invalid value from a registered type -> capability failure.
- Unsupported/cyclic type -> registration failure.
- Preserve current root and MCP projection text.

## Spike gate: aggregate intermediate budget

Before composite outputs can ship, add a disposable `internal/worker/intermediate_budget_spike_test.go`, then delete it after recording the decision in the PR.

Compare only:

1. **Preferred:** cumulative exact canonical encoded value-body bytes for every successful parent-to-child `native_result`, counted once per delivery. Exclude arguments, frame envelopes, final result, failed handlers, handler-owned memory, and Starlark-created values.
2. **Fallback:** reserve the per-call maximum before dispatch and release the difference after encoding. This is safer early but penalizes small results.
3. Reject live/high-water semantics unless the spike finds an existing Starlark lifetime hook; current code exposes none, and runtime/GC instrumentation is out of scope.

Drive sequential results through the real parent connection/child engine. Prototype one bounded encoding that returns encoded bytes and value length, reuses those bytes in the frame, rejects the crossing result before write, and passes a remaining-byte-derived node limit to reflected conversion before allocation. Measure representative nested values.

```text
go test ./internal/worker -run '^TestIntermediateBudgetSpike$' -count=1 -v
go test ./internal/worker -run '^$' -bench '^BenchmarkIntermediateBudgetSpike$' -benchmem -count=3
```

The spike record must choose semantics, debit point, exact public field default, and whether remaining budget must be added to the internal `worker.Dispatch` signature. It must confirm the counter is per `Server.Execute` and whether protocol changes are unnecessary. Preferred implementation: public `MaxIntermediateValueBytes`, parent-owned per execution, cumulative exact result-body bytes, debit after one bounded encode and before frame write. If frame construction currently forces re-encoding, refactor it to consume pre-encoded value bytes; do not retain double encoding. No composite-output increment proceeds until this is decided.

## Ordered implementation increments

### 1. Widen scalar inputs

**Dependency:** none; parallelizable with the spike.

**Files/symbols**

- `internal/binding/plan.go`: make `fieldKind` input-only; extend `compileInput`.
- `internal/binding/signature.go`: extend constants, `ValidateInputShape`, shape predicates, and `inputKindSignature`.
- `internal/binding/input.go`: extend `BindShape`, `convertShapeArgument`, and `Plan.BindValue` with exact type/range/finiteness and optional canonicalization.

**Tests**

- `internal/binding/{plan,input,signature}_test.go`: all eight forms, named aliases, omission/None, wrong categories, overflow, NaN/Inf, malformed normalized parent maps.
- `internal/execution/execute_test.go::TestExecuteBindsThenDispatches` and malformed-argument cases.
- `dispatch_test.go::{TestDispatchBindsAuthorizesThenInvokes,TestDispatchTranslatesEveryBindValueFailureInternally,TestDispatchReturnsFreshCanonicalMaps}`: authorizer sees only fresh canonical values; invalid values never reach policy; handler receives exact typed input.

**Proof**

```text
go test ./internal/binding -run '^(TestCompileAcceptsRepresentativeTypes|TestBindShapeAndBindValueAgreeOnSupportedCalls|TestBindShapeRejectsMalformedArguments|TestBindValueRejectsNormalizedWireMismatches|TestSignature|TestValidateInputShape)' -count=1
go test ./internal/execution -run '^(TestExecuteBindsThenDispatches|TestExecuteRejectsMalformedArgumentsBeforeNativeCall)$' -count=1
go test . -run '^TestDispatch(BindsAuthorizesThenInvokes|TranslatesEveryBindValueFailureInternally|ReturnsFreshCanonicalMaps)$' -count=1
```

**Complete when:** child and parent accept/reject identically and bind -> canonicalize -> authorize -> invoke remains proven.

### 2. Add request-wide accounting and remove duplicate encoding

**Dependency:** spike decision; must land before composite registration.

**Files/symbols**

- `limits.go`: add documented `Limits.MaxIntermediateValueBytes`, exact spike-selected default, and positive validation. Zero is invalid.
- `server.go::newServer`: project it to `worker.Limits`; do not add it to child `execution.Limits` under the preferred parent-owned design.
- `internal/worker/parent.go`: extend parent `Limits`/`Runner`; create fresh accounting in `runExecExchange`/its per-execution connection, never mutable shared Runner state; map exhaustion through existing resource-limit abort.
- `internal/worker/value.go`, `frame.go`: replace validate+encode+encode with one bounded canonical value-body encoding reused by native-call/native-result/final frame builders. Keep per-value `MaxValueBytes` independent. Debit only native-result value bodies.
- If the spike requires pre-allocation remaining budget, cleanly extend `worker.Dispatch` and migrate `handleNative`, `dispatcher.dispatch`, tests, and mocks; do not use context values or overload aliases.

**Tests**

- `limits_test.go`: default and non-positive new field.
- `internal/worker/parent_test.go`: below/crossing cap, inclusive edge, fresh next execution, concurrent executions independent.
- `frame_test.go`, `value_test.go`: one encoding, value body excludes envelope, numeric/map determinism unchanged.
- `server_test.go`: aggregate exhaustion projects only `ErrResourceLimit`.

**Proof**

```text
go test ./internal/worker -run '^(TestRunnerNewValidatesBudgetsAndManifest|TestRunnerIntermediateBudget|TestProtocolExecWithNativeCalls|TestValueCodec)' -count=1
go test . -run '^(TestDefaultLimitsAreValid|TestLimitsRejectNonPositiveValues|TestServerExecuteEnforcesAggregateIntermediateLimit)$' -count=1
```

**Complete when:** every execution owns a fresh budget, native results are encoded once, exhaustion occurs before write, and protocol shape/version remains unchanged unless the spike demonstrated a required change.

### 3. Compile and convert composite outputs

**Dependency:** Increment 2.

**Files/symbols**

- `internal/binding/plan.go`: replace scalar `outputField.kind` with root fields referencing a flat immutable node arena; expand `compileOutput` into the type-graph compiler with active-stack cycle detection, reuse cache, nested paths, and exact accepted/rejected universe.
- `internal/binding/output.go`: make `Plan.ConvertOutput` limit-aware and node-driven; implement structs, arrays/slices, sorted string maps, pointers, scalars, bytes, omission/null rules, range/finiteness, and stable paths such as `output.items[3].score` / `output.by_id["a"].count`.
- `internal/binding/value.go`: extract/reuse existing node/depth preflight state if necessary rather than creating a second budget convention.
- `internal/binding/signature.go`: compile exact flat output notation once.
- `dispatch.go::dispatcher.dispatch`: call limit-aware conversion; map `ErrValueLimit` to resource and other conversion failures to capability failure; remove the too-late redundant `ValidateValue` traversal once compiled conversion and worker encoding cover it.

**Tests**

- `plan_test.go`: all accepted containers/pointers/aliases/bytes plus all rejected interfaces, map keys, marshalers, RawMessage, tags, embedding, omitempty misuse, and direct/indirect cycles. Assert nested registration paths.
- `output_test.go`: nested values; nil vs empty; nullable vs omitted; pointer elements; signed/unsigned boundaries; float32/64 NaN/Inf; bytes; wrong root; sorted deterministic map failure; depth/node limits.
- Add allocation regression modeled on `value_test.go::TestValueConversionRejectsOversizedContainersBeforeAllocation` for reflected slices/arrays/maps/structs.
- `signature_test.go`: exact grammar, declaration order, required flags, nullability de-duplication.
- `dispatch_test.go`: invalid numeric output -> capability failure; depth/node -> resource; ordering unchanged.

**Proof**

```text
go test ./internal/binding -run '^(TestCompile|TestConvertOutput|TestOutputShape|TestValueConversionRejectsOversizedContainersBeforeAllocation)' -count=1
go test . -run '^(TestDispatchBindsAuthorizesThenInvokes|TestDispatchClassifiesParentOutputLimits|TestDispatchClassifiesInvalidCompositeOutputs)$' -count=1
```

**Complete when:** accepted graphs register and convert without runtime type analysis or proportional preflight violations; rejected types fail registration; no composite result bypasses aggregate accounting.

### 4. Discovery, MCP schema, and real same-binary composition

**Dependency:** Increments 1–3.

**Files/tests**

- `internal/catalog/catalog_test.go`: extend `TestBuildFiltersOnceAndDerivesEverySurface` with composite notation; mutate returned Input and Output shapes and re-describe to prove immutability; retain name sorting.
- `mcpserver/server_test.go`: keep `requireDescribeAPIOutputSchema` asserting the same non-recursive array-of-FieldShape schema; prove `mcpserver.New` still resolves it and lists exactly three tools.
- `mcpserver/e2e_test.go`: add `TestActualMCPCompositeProgram` using existing real in-memory MCP transport and same-binary worker setup. Register a capability with required int64/bool/float64, optional *string, and output containing `Items []NamedItem` where item has ID/Active/Score. Search and describe it; assert exact derived nested notation and no Go type names. Execute Starlark that calls it, loops over `resp["items"]`, filters rows, accumulates count/score, collects IDs, and returns only a digest. Assert digest, canonical authorizer map, exact typed handler input, and call count. Use multiple rows.
- `builder_test.go`, `server_test.go`: public registration rejection and public projections for NaN/Inf, unsigned overflow, depth/per-value/aggregate limits.
- `moon.yml`: update `root:mcp-smoke` regex to run both existing `TestActualMCPSecureLoop` and new composite test.

**Proof**

```text
go test ./internal/catalog -run '^(TestBuildFiltersOnceAndDerivesEverySurface|TestSearchIsSortedNormalizedAndBounded|TestCatalogSupportsConcurrentReadOnlyUse)$' -count=1
go test ./mcpserver -run '^(TestNewRegistersExactlyThreeTools|TestActualMCPCompositeProgram)$' -count=1
go test . -run '^(TestRegisterRejectsInvalidContractsBeforeRetention|TestServerExecute.*Output|TestServerExecute.*ValueLimit)$' -count=1
moon run root:mcp-smoke
```

**Complete when:** the unchanged three-tool surface derives composite discovery and a real re-executed worker performs list/filter/aggregate and returns the asserted digest with canonical parent authorization.

### 5. Documentation

**Dependency:** spike and Increments 1–4, so no provisional default/notation is published.

- `docs/docs/reference/public-api.md`: exact widened input and structural output matrices; roots; aliases; numeric rules; bytes; marshalers/RawMessage; maps/cycles; pointer/omitempty/nil; notation; new limit/default/validation/accounting; corrected child bind -> parent rebind/canonicalize -> authorize -> handler -> process-neutral -> child ToStarlark path.
- `docs/docs/reference/mcp-tools.md`: expanded shape table/grammar, optional vs nullable, composition example, aggregate recovery, unchanged schema/tool surface.
- `docs/docs/explanation/security-model.md`: parent process-neutral output, child Starlark conversion, per-crossing vs cumulative budgets, and explicit statement that this is not RSS/OS quota.
- `README.md` and the affected boundary paragraph in `docs/docs/tutorials/first-server.md`: correct the current abbreviated parent-binding and independently-limited-crossing statements; keep the valid scalar tutorial.
- Leave blank `CHANGELOG.md` unchanged unless implementation establishes a real changelog convention; do not invent unrelated release structure.

**Proof**

```text
moon run docs:build
```

**Complete when:** required references and every now-false boundary summary match tested strings and selected budget semantics.

## Traceability

| Requirement/review correction | Increment |
|---|---|
| Keep Capability API and non-pointer roots | 3, 4 |
| Bind/canonicalize/authorize/dispatch and fresh canonical args | 1, 4 |
| Widened exact scalar inputs, range/finiteness, omission/None | 1, 4 |
| Structural outputs and explicit arrays/slices/maps/pointers/aliases/bytes | 3 |
| Explicit nil and omitempty semantics | 3, 5 |
| Registration rejection of interfaces, RawMessage, marshalers, bad maps/tags, cycles | 3, 4 |
| Compile immutable nodes once; no repeated reflection | 3 |
| Process-neutral parent result and child ToStarlark | 3, 4, 5 |
| Allocation preflight before destination allocation | 2, 3 |
| Preserve exact MaxValueDepth/MaxValueBytes per crossing | 2–4 |
| Request aggregate budget: counted values, location, bytes, default, validation, protocol | spike, 2, 5 |
| Aggregate resource classification; invalid output capability classification | 2–4 |
| Single encode/no JSON round trip | 2, 3 |
| Flat deterministic discovery; no recursive/manual schema | 3, 4 |
| Sorted maps, deterministic diagnostics, name-sorted discovery | 3, 4 |
| Returned-description immutability | 4 |
| Same-binary []struct loop/filter/aggregate digest | 4 |
| Exactly three MCP tools and unchanged error projection | 4 |
| Required docs and false matrix/limit text | 5 |
| No shims/aliases/deprecated paths | all |

## Risks

- Cumulative encoded bytes bound deterministic delivered work, not RSS, Starlark object overhead/copies, or handler memory.
- Exact bytes are known after reflected conversion; remaining-node preflight bounds CodeMode containers but cannot undo handler allocation.
- `?` and `| None` must remain distinct and test-locked.
- Check both value and pointer method sets for custom marshalers.
- Normalize all integer kinds carefully; reject uint overflow and uintptr.
- Sort reflect map keys before traversing values, not only during final encoding.
- Adding a positive public limit is a clean pre-1.0 break for incomplete `Limits` literals; zero must not become an implicit default.

## Non-goals

Composite inputs; pointer roots; any/interfaces/RawMessage/custom marshaling; JSON/Starlark modules; more MCP tools; OS memory/CPU quotas; parent handler/authorizer isolation; full encoding/json behavior; compatibility shims; unrelated refactors.

## Definition of done

- Spike decision recorded and temporary file removed; exact aggregate default documented.
- Eight input forms agree in child and parent and preserve authorization ordering.
- Structural outputs compile once, convert to bounded process-neutral values in parent, and become Starlark only in child.
- Allocation preflight, per-crossing, and aggregate limits have behavioral tests and correct projections; no native result is encoded twice.
- Unsupported types fail registration; runtime invalid numerics fail as capability failure.
- Flat derived discovery remains deterministic, immutable, schema-resolvable, and manual-schema-free.
- Real MCP/same-binary composite test loops, filters, aggregates, returns a digest, and confirms canonical authorization.
- Exactly three MCP tools and existing coarse errors remain.
- Required docs build strictly and no spike artifact, compatibility path, or temporary code remains.