# CodeMode functional campaign 004 — findings remediation plan

## Status and scope

Campaign 004 exercised candidate and oracle commit `aac86bc8cf4bcebb91e9257767fe6c31981847bd` through an external Go consumer, the official MCP in-memory and stdio transports, and fresh OMP 17.3.4 agents. The accepted ledger is **57 PASS, 1 PARTIAL/BLOCKED (`R03`), and 1 FAIL (`R04`)**. The campaign found no reachable CodeMode execution, authorization, filtering, or transport-boundary defect. The required work is a set of contract corrections and model/developer-UX improvements, plus disposable-harness repairs and one upstream OMP/client report.

This plan is read-only. It does not include changes to the campaign evidence. Product increments target the current CodeMode repository. Harness increments target `/Users/josh/code/meigma/codemode-functional-004` only and must not be merged into CodeMode. OMP/client work is upstream and must not be compensated for in CodeMode.

## Decisions

1. **Preserve the three-tool surface and security order.** Keep exactly `search_api`, `describe_api`, and `execute`; keep bind/canonicalize → authorize → dispatch; keep disabled capabilities absent from search, description, the Starlark namespace, and dispatch. `C01–C09`, `D01–D07`, `E01–E17`, `A01–A02`, `R01–R02`, `R05–R06`, and `M01–M12` establish these as working constraints.
2. **Fix authoring failures proactively, not by disclosing trusted diagnostics.** Put the zero-argument `main()` contract, short-substring search guidance, and exact-name workflow in the model-visible MCP tool descriptions. Keep client errors coarse. A model must not receive resolver diagnostics, policy reasons, handler errors, panic values, stack traces, credentials, submitted source, or argument values.
3. **Define diagnostic detail by layer.** Internal binding, catalog, execution, and OPA errors may retain detailed causes for implementation and trusted-host debugging. `authz/rego.Authorize` may distinguish an undefined decision from a non-Boolean value because that API is host-facing and the module is trusted deployment configuration. Root `codemode.Server` continues to return public sentinels and deliberately discards execution, policy, and handler detail. `mcpserver` continues to emit only fixed coarse strings. The MCP SDK may report schema-validation detail about client-supplied malformed tool arguments; it runs before the resolver and never sees trusted subject, policy, or handler data.
4. **Resolve `R03` by correcting the contract, not inventing coverage.** `rego.New` accepts a direct ground `data` query, which produces at most one result through the public constructor. Remove “multiple results” from the public result matrix. Retain fail-closed handling for any structurally unexpected OPA result, but do not add a fake fixture or claim a public reproduction.
5. **Resolve `R04` as a documentation/test-precision correction.** Runtime network builtins remain unavailable because nondeterministic builtins, including `http.send`, are removed. Empty `AllowNet` remains a deny-all host list. A `schema["https://…"]` metadata annotation is accepted and ignored when no schema set or resolver is installed; it is neither fetched nor used for validation. A metadata `$ref` that asks OPA to load a remote document remains rejected. Document this distinction instead of changing the evaluator.
6. **Remove host Go output type names from generated signatures.** Render only the callable Starlark form, for example `records.lookup(*, key: str, limit: int | None)`. `describe_api.output` remains the authoritative result shape. This also removes the observed `->` sequence that was HTML-escaped in the SDK text mirror without replacing the SDK serializer.
7. **Keep deterministic literal-substring search for this campaign.** Do not introduce fuzzy ranking, stemming, stop-word logic, or a second search convention. Repeated OMP failures justify model-visible guidance to use one short substring and retry with a shorter term, but they do not yet justify a new matching algorithm.
8. **Keep exact `Describe` lookup and pre-normalization query budgeting.** Exact, case-sensitive, untrimmed names prevent silent normalization of identifiers. `MaxSearchQueryBytes` continues to apply to raw input before trimming so untrusted padding is still bounded. Document the operational consequence and put exact-name guidance on the wire.
9. **Do not change CodeMode for OMP argument rewriting.** OMP’s `code`/`program` → `source` rewrite and extra-field removal happen before CodeMode. Raw SDK/stdio evidence remains the authority for CodeMode schema enforcement.

## Canonical finding inventory

| Canonical item | Source IDs and report names | Disposition | Resolution |
| --- | --- | --- | --- |
| `CM-01` model cannot discover the required program entry point | U01 `UX-1`, U02 `UX-1 recurrence`, U03 `UX-8`/`UX-9`, U05 UX finding 2, U06 UX finding 2; ledger “zero-argument `main()`” | implement in CodeMode | Publish the exact `def main():` contract and phase rule in the `execute` tool description; keep coarse errors. |
| `CM-02` natural-language discovery queries silently miss | Catalog `UX-4`; U02 `UX-5`; U03 empty first search; U04 UX finding 2; U06 QA search repeats | implement in CodeMode | Publish one-short-substring and shorter-retry guidance in `search_api`; keep the existing deterministic search algorithm. |
| `CM-03` generated signatures expose host Go output types | Consumer `UX-01` | implement in CodeMode | Drop the return suffix from `binding.Plan.Signature`; direct callers to `Description.Output`. |
| `CM-04` advertised MCP output schemas permit `null` | `M02` / MCP `UX-1` | implement in CodeMode | Supply explicit non-null output schemas and normalize successful collection values to non-nil slices. |
| `CM-05` host-side Rego errors collapse undefined and non-Boolean decisions | Authorization `F-2` | implement in CodeMode | Distinguish the two at `authz/rego.Authorize`; keep both mapped to `ErrPolicyFailure` and the same MCP text. |
| `DOC-01` unreachable Rego multiple-result contract | `R03.multi-result` PARTIAL/BLOCKED; Authorization `H-1` part 2 | blocked-contract decision | Remove the unreachable public promise and fake fixture; retain an unclaimed defensive guard. |
| `DOC-02` remote-schema wording overstates `AllowNet` | `R04` FAIL; Authorization `F-1` | documentation-only | Document ignored `schema[...]` annotations, rejected remote `$ref`, and the unchanged runtime network boundary. |
| `DOC-03` duplicate keyword classification | `E05` / `F-E05-DUP` | documentation-only | State that duplicate keywords are a Starlark parse error (`ErrInvalidProgram`); binding-time argument defects remain `ErrInvalidArguments`. |
| `DOC-04` converted-value depth convention | `E14` depth supplement | documentation-only | Define inclusive depth with a scalar at level 1 and each containing list/dict adding one level. |
| `DOC-05` tutorial dependency ordering | Consumer `F-01` | documentation-only | Add the separately imported MCP Go SDK to the initial dependency command; keep `go mod tidy`. |
| `DOC-06` exact description, raw query budget, and nil-context behavior | Catalog `UX-1`/`UX-2`; Execution `F-E01-NIL` | documentation-only | Explicitly state no trimming for `Describe`, raw-byte budgeting before normalization, and nil `Execute` context → `ErrInternal`. No runtime change. |
| `DOC-07` coarse-error usability and layer contract | Catalog `UX-3`; U01 `UX-2`; U03 `UX-8`/`UX-11`; U04 UX finding 1; U05 UX findings; U06 major UX finding | documentation-only | Define which layer may expose which detail, add safe recovery guidance, and warn that an allow/deny response does not reveal policy completeness. Do not expose trusted detail. |
| `DOC-08` Rego decision-path leniency | Authorization `F-3` | documentation-only | State that whitespace is accepted by OPA’s ref parser and that a syntactically valid but undefined ground path can fail only at evaluation. |
| `NOACT-01` text mirror HTML-escapes `>` | MCP `UX-2` | accepted/no-action | The text is valid JSON and parses identically. `CM-03` removes the observed arrow. Do not replace typed SDK handlers or duplicate SDK validation for cosmetic escaping. |
| `NOACT-02` deterministic catalog semantics | `D01–D07`, including literal substring, exact description, sort, cap, and static filtering | accepted/no-action | Preserve behavior; only improve wire/reference guidance. |
| `NOACT-03` standard Go toolchain switch and fixture-specific output | Consumer `F-02`; U02 `UX-6` | accepted/no-action | Go toolchain selection is standard; `qty:0` is the disposable fixture handler’s output, not a CodeMode contract. |
| `NOACT-04` SDK-owned `tools/list` fields and `xd://` presentation | MCP non-standard `ttlMs`/`cacheScope`; U01 `UX-4` | accepted/no-action | Record as client/SDK observations; no CodeMode contract or fix. |
| `UP-01` OMP rewrites tool arguments | U03 `UX-12`; U04 first-party normalization finding; U06 new OMP/client finding | upstream OMP/client | File upstream with emitted-vs-dispatched evidence. Do not relax or alter CodeMode schemas. |
| `HAR-01` option-copy probe is non-discriminating | Consumer `H-01` | harness-only | Replace `caseC08` with the durable five-capability, two-limit, two-sided disabled-ID probe. |
| `HAR-02` clean-install script stops before the tutorial | Consumer `H-02` | harness-only | Make the script resolve the module-cache tutorial, prove provenance, extract the program, tidy, run, and assert all three outputs. |
| `HAR-03` original concurrency/fresh-state probe cannot detect leaked globals | Consumer C09 revision history | harness-only | Fold the writer/reader stale-global negative-control module into `caseC09`; keep the unique-token contention probe for argument/result integrity. |
| `HAR-04` catalog emission destroys field order | Catalog harness note (`fixtures.CopyAny`) | harness-only | Preserve typed/direct JSON for D06 rather than round-tripping the result through `map[string]any`. |
| `HAR-05` merged observer logs do not prove interleaving | Execution observer limitation; Authorization A01 cross-reference caveat | harness-only | Use one shared event recorder across resolver/authorizer/handler or an observing wrapper in the ordering case. |
| `HAR-06` original E13 state probe and pipelined transcript are insufficient | E13 report corrections | harness-only | Use valid writer/read-only programs and a sequential driver that sends request N+1 only after response N. |
| `HAR-07` E14/E16 time attribution and depth labels are confounded | `F-E14-ATTR`; E14 depth corrections | harness-only | Isolate one budget at a time, set the step budget out of reach for time tests, and label scalar/container depths from the actual convention. |
| `HAR-08` E17 ledger is hardcoded | `F-E17-LEDGER` | harness-only | Replace `caseE17`’s static map with the preserved derived-evidence script or equivalent machine aggregation. |
| `HAR-09` Rego constructor/result fixtures do not exercise their labels | Authorization `H-1` parts 1 and 2 | harness-only | Use `data.codemode.authz[x]` for non-ground rejection; delete/relabel `MultiResultPolicy` as unreachable-by-construction. |
| `HAR-10` MCP harness duplicates resolver events and a fake empty-subject row | MCP harness notes | harness-only | Stop appending resolver events twice in `emitTool`; remove the fallback “empty” loop row and retain the genuine empty-subject resolver. |
| `HAR-11` MCP tight-result probe was undersized | MCP M07 harness defect | retracted/closed | Current `caseM07` now returns a value over 64 bytes; no further change. |
| `HAR-12` OMP evidence/prompt defects | U01 text pilot; U03 superseded coverage/count; U04 attempt 2 prompt violation; U06 undefined attempt 1 | harness-only | Make JSON mode mandatory, promote the accepted replacement prompts, ban non-MCP tools explicitly, and retain superseded runs as negative evidence rather than verdict carriers. |
| `HAR-13` disposable stdio host treats EOF as failure | M12 EOF observation | harness-only | In `cmd/codemode-mcp.run`, treat the SDK’s normal stdin EOF close as clean teardown while preserving genuine startup/runtime errors. |
| `CLOSED-01` elapsed-deadline mismatch claim | `F-E16-DL` | retracted/closed | The isolated supplement disproved it; no product or documentation change. |
| `CLOSED-02` original U03 bad-argument interpretation and failure count | U03 superseded claims | retracted/closed | Accepted QA rerun proves `ErrInvalidArguments`; authoritative counts are 12 failures and a longest identical-error streak of 9. |
| `CLOSED-03` U04 attempt-2 compliance verdict | U04 attempt 2 | retracted/closed | Attempt 3 is the compliant evidence of record. |

## Phased implementation plan

Each increment is independently reviewable and shippable. Increments marked parallel may proceed independently; increments that both edit `mcpserver/server.go` should be serialized to avoid needless conflict.

### Increment 1 — Correct the Rego result contract and trusted-host diagnostics

**Scope:** `DOC-01`, `CM-05`, `DOC-08`. Independent of all MCP changes.

**Files and symbols**

- `authz/rego/rego.go` — `(*Authorizer).Authorize`.
- `authz/rego/rego_test.go` — `TestAuthorizeDecodesDirectBooleanResults`.
- `docs/docs/reference/public-api.md` — `authz/rego` `New` and `Authorize` sections.
- `docs/docs/how-to/use-rego-authorization.md` — “Preserve the decision outcomes”.
- `docs/docs/explanation/security-model.md` — “Rego policy runs in process”.

**Change**

- After OPA evaluation, classify an empty `rego.ResultSet` as the trusted-host error `rego: decision is undefined`.
- Keep `oparego.ResultValue[bool]` as the Boolean decoder. If the one returned value is not Boolean, return a distinct trusted-host error such as `rego: decision must be boolean`. Preserve `authz.ErrDenied` only for Boolean `false`.
- Keep evaluation and strict-builtin errors wrapped with their existing trusted OPA cause.
- Do not add a public-constructor “multiple results” test. A ground `data` query cannot honestly reproduce it. A structurally unexpected result may still fail closed through the generic Boolean decoder, but it is not a documented outcome or coverage row.
- Rewrite the three public documents to say a ground decision is either undefined or yields one value; that value must be Boolean. Remove “exactly one” and “multiple results” wording. Explain that `New` validates reference syntax and prepares policy, but cannot prove the decision is defined and Boolean for every future input. Mention OPA’s accepted surrounding whitespace only as parser behavior, not as a second CodeMode normalization rule.

**Cutover**

- No API signature or sentinel changes. Hosts that compare error strings may see more precise text; they must continue to use `errors.Is` only for `authz.ErrDenied` and context errors. Root `Server.Execute` continues mapping both new ordinary errors to `ErrPolicyFailure`; MCP clients continue receiving `authorization policy failure`.

**Focused verification**

- Extend `TestAuthorizeDecodesDirectBooleanResults` to assert different messages for undefined and string/set-valued decisions while asserting neither matches `authz.ErrDenied`.
- Keep true, false, strict-builtin-failure, pre-canceled, mid-evaluation cancellation, and concurrent-use tests unchanged as regression coverage.
- At the root/MCP boundary, exercise undefined and non-Boolean decisions and assert both remain `ErrPolicyFailure` / `authorization policy failure` with no Rego path, module, type, or builtin detail.

**Acceptance criteria**

- The public contract contains no reachable claim for multiple results.
- Direct host callers can distinguish undefined from non-Boolean decisions.
- Boolean false is still the only recognized denial.
- No new detail crosses `codemode.Server` or MCP.

### Increment 2 — Correct the Rego network/schema explanation without moving the boundary

**Scope:** `DOC-02`. Parallel with Increment 1 except for `rego_test.go`/Rego docs; serialize if implemented by the same branch.

**Files and symbols**

- `docs/docs/explanation/security-model.md` — “Rego policy runs in process”.
- `docs/docs/reference/public-api.md` — `authz/rego.New` evaluator restrictions.
- `authz/rego/rego_test.go` — rename/narrow `TestNewRejectsRemoteSchemas`; add an observation for the accepted annotation form.
- `authz/rego/doc.go` — package comment only if needed to keep the schema/no-resolver statement findable in Godoc.

**Change**

- State that removing OPA-declared nondeterministic builtins removes the runtime network-capable builtins, including `http.send`; empty `AllowNet` is a deny-all host list and defense in depth.
- State that CodeMode installs no schema set or resolver. A `schema["https://example.invalid/schema.json"]` metadata annotation is accepted but ignored: it performs no validation and no fetch. Do not tell users that acceptance means the schema was applied.
- Preserve the narrower fact already covered by the repository: a metadata object containing an external `$ref` that asks OPA to load a remote schema is rejected with remote reference loading disabled.
- Rename the existing test so its name and assertion describe the `$ref` form rather than all “remote schemas”. Add a behavior test for the ignored `schema[...]` form that constructs the authorizer and authorizes successfully; use `example.invalid` so any attempted fetch would fail.
- Do not change `restrictedCapabilities`, add a schema resolver, or reject ignored annotations solely to make the old sentence true.

**Cutover**

- No runtime migration. This fixes what operators can rely on: unavailable runtime network operations, rejected external `$ref` loading, and ignored unconfigured schema annotations are separate facts.

**Focused verification**

- Run the two schema metadata tests plus `TestNewRejectsNondeterministicBuiltins` and `TestAuthorizeErasesDisabledPrintCalls`.
- Repeat the campaign’s minimal public construction with `schema[...]`; require construction and authorization success and no network output.

**Acceptance criteria**

- Public wording matches both observed annotation forms.
- `http.send`, DNS, time, random, UUID, and runtime builtins remain unavailable at preparation.
- No document implies hard process isolation or untrusted-policy safety.

### Increment 3 — Correct public reference edges and the tutorial

**Scope:** `DOC-03`, `DOC-04`, `DOC-05`, `DOC-06`. Parallel with Rego and signature work.

**Files and symbols**

- `docs/docs/reference/public-api.md` — supported input rules, limits, `Describe`, `Execute`, and error classifications.
- `docs/docs/reference/mcp-tools.md` — `execute` errors and value-depth text.
- `docs/docs/explanation/security-model.md` — “Validation precedes authority” parser/binder wording.
- `docs/docs/tutorials/first-server.md` — “Create a module”.
- `internal/execution/execute_test.go` — entry-point/program classification coverage.
- `internal/binding/output_test.go` — `TestConvertFinalEnforcesDepthAndEncodedSize`.

**Change**

- Separate parser rejection from native binding: a duplicated keyword is syntactically invalid Starlark and maps to `ErrInvalidProgram`; positional, unknown, missing, incorrectly typed, and out-of-range native arguments reach binding and map to `ErrInvalidArguments`.
- Define `MaxValueDepth` as an inclusive maximum: scalar/`None` is depth 1; each enclosing tuple/list/dictionary adds one. Use the same convention in public and MCP references.
- Explicitly say `Describe` does not trim or case-fold; clients should pass the exact `name` returned by search. Preserve the existing exact lookup.
- Retain the raw-before-normalization search-byte order and explain that padding counts against the limit.
- Document that a nil `Server.Execute` context is a caller-contract violation currently classified as `ErrInternal`; do not add a new sentinel for this one misuse.
- Change the tutorial’s dependency command to install both `github.com/meigma/codemode@master` and `github.com/modelcontextprotocol/go-sdk/mcp` before the program is introduced. Retain `go mod tidy` as the final module cleanup.
- Add only behavioral tests: duplicate keyword → `ErrInvalidProgram` with no authorization/handler call; scalar at depth 1 and one-level container at depth 2 succeed; one additional container fails.

**Cutover**

- Documentation now describes current behavior. No runtime migration.

**Focused verification**

- Run the focused execution and binding tests.
- In a clean external module with the pinned Go requirement, execute the tutorial commands in order and run the unmodified fenced Go program.

**Acceptance criteria**

- A reader can predict both duplicate-keyword classification and every tested depth boundary.
- The program builds after the dependency-install step rather than relying on a later command to repair an intermediate missing import.
- No new public sentinel or input-normalization convention is introduced.

### Increment 4 — Publish model-authoring guidance on the MCP surface

**Scope:** `CM-01`, `CM-02`, the actionable part of `DOC-07`. This increment should precede the next OMP UX rerun. It serializes with Increment 6 because both edit `mcpserver/server.go`.

**Files and symbols**

- `mcpserver/server.go` — `New` tool registrations.
- `mcpserver/server_test.go` — `TestNewRegistersExactlyThreeTools` or a focused sibling test for listed descriptions.
- `docs/docs/reference/mcp-tools.md` — record the descriptions and recommended workflow.
- `docs/docs/index.md` — only if the concise three-tool table needs the same workflow cue; avoid duplicating the full reference.

**Change**

- Change `search_api`’s description to say it performs a short literal substring search over enabled names/summaries and that an empty result should be retried with a shorter term.
- Change `describe_api`’s description to require the exact name returned by `search_api`, without added whitespace or case changes.
- Change `execute`’s description to include a minimal valid shape: source defines zero-argument `def main():`; capability calls occur inside `main`; only names confirmed through search/describe should be called; `main` returns the final result. Keep the description concise enough to remain useful in `tools/list`.
- Add recovery guidance to the reference rather than error payloads: compare native arguments against `describe_api`; shorten a failed search query; reduce program/result complexity after a resource limit; treat policy results as outcomes for this call, not a disclosure of policy rules.

**Cutover**

- Tool names and input schemas do not change. Clients that display tool descriptions receive additional prose only.

**Focused verification**

- Through the official in-memory client, list tools and assert the exact three names plus the presence of `def main()`, short-substring guidance, and exact-name guidance in the corresponding descriptions.
- Over raw stdio, inspect `tools/list` to confirm the same descriptions reach the wire.
- Run fresh blind U01/U02/U03-style OMP agents in JSON mode. Judge dispatched arguments from `tool_execution_end.details.xdev.args`, not the model-emitted record. The agent should write a valid zero-argument `main` without a missing-entry-point retry and should use a short discovery term after reading the tool description.

**Acceptance criteria**

- The required program shape is discoverable without repository documentation or a failed call.
- Search and describe hygiene is visible in band.
- Error payloads remain fixed and non-disclosing.

### Increment 5 — Remove Go implementation names from signatures

**Scope:** `CM-03`, and the observed arrow portion of `NOACT-01`. Parallel with Increment 4 because it changes binding/catalog behavior, but update MCP tests after both changes are present.

**Files and symbols**

- `internal/binding/signature.go` — `(*Plan).Signature`.
- `internal/binding/signature_test.go` — both signature tests.
- `internal/catalog/catalog_test.go` — exact search/description signature expectations.
- `mcpserver/e2e_test.go` — secure-loop signature expectation.
- `mcpserver/server_test.go` — mock search/description fixtures that still contain `-> object`.
- `docs/docs/reference/public-api.md` and `docs/docs/reference/mcp-tools.md` — `signature` field meaning.

**Change**

- Stop appending `) -> <Go type name>` in `Plan.Signature`; close the call after the parameter list.
- Preserve dotted capability name, keyword-only marker, parameter order, field names, and `str` / `int | None` notations.
- Document that `signature` describes invocation only and that `Description.Output` is the stable result contract.
- Migrate every exact signature fixture and assertion in the repository. Do not retain an alias, compatibility flag, deprecated form, or alternate signature field.

**Cutover and risk**

- This is a visible string change. Although the old return token was not documented as a stable Go type, clients that parsed it must stop and use `describe_api.output`. Announce that migration in the behavior-changing PR body and user reference.
- `Plan.outputType` remains necessary for conversion; remove only its use in model-facing signature rendering.

**Focused verification**

- Run binding, catalog, and MCP e2e tests.
- Register both exported and unexported output structs and assert their Go identifiers appear nowhere in `Search`, `Describe`, structured MCP content, or text mirrors.
- Confirm a described signature still suffices to write a correct keyword-only capability call.

**Acceptance criteria**

- No host Go output type name is model-visible.
- All input-call information proven useful by C02/U02 remains unchanged.
- The campaign’s observed `->` text-mirror escape disappears without custom serialization code.

### Increment 6 — Make advertised MCP output schemas match runtime values

**Scope:** `CM-04`. Independent in behavior but serialized after Increment 4 because both edit `mcpserver/server.go`.

**Files and symbols**

- `mcpserver/server.go` — `New`, `(*adapter).search`, and `(*adapter).describe`.
- `mcpserver/server_test.go` — tool-list and successful-output tests.
- `mcpserver/e2e_test.go` — runtime non-null array assertions if not already explicit.
- `go.mod` — promote `github.com/google/jsonschema-go v0.4.3` from indirect to direct if `mcpserver` imports it; `go.sum` should not need a new version.
- `docs/docs/reference/mcp-tools.md` — retain the non-null schemas and state that listed schemas are the advertised `outputSchema`, not only prose.

**Change**

- In `mcpserver.New`, use the pinned SDK’s supported explicit `Tool.OutputSchema` path. Infer schemas with `jsonschema.For` from the existing output types, then replace only the nullable slice nodes: `search_api` root is `array`, and `describe_api.input`/`output` are `array`, never `null`. Leave `execute.result` unconstrained as today.
- In successful handlers, preserve `search`’s existing nil-to-empty normalization and normalize nil `Description.Input`/`Output` slices to empty non-nil slices before the SDK validates/serializes them. This keeps the wire contract true even for a custom `mcpserver.Service`, while errors still return no successful payload.
- Do not hand-write a second complete JSON schema or replace the typed `mcp.AddTool` wrappers; keep SDK input validation and structured-output validation.

**Cutover**

- Generated clients may narrow `null | array` to `array`. Runtime output is unchanged because the campaign already observed only arrays.

**Focused verification**

- List tools through the official SDK and assert `search_api.outputSchema.type == "array"`; assert `describe_api`’s `input` and `output` schema nodes each have `type == "array"`; assert none contains `null`.
- Call blank search and a description with empty field collections through both in-memory and raw stdio transports; assert `[]`, not `null`, in structured content and its parsed text mirror.
- Retain malformed-input tests to prove explicit output schemas did not weaken SDK input validation.

**Acceptance criteria**

- Advertised and documented schemas are identical on nullability.
- Successful runtime collections remain non-null.
- Exactly three tools remain registered, and resolver/service ordering is unchanged.

### Increment 7 — Make the error-boundary contract exact without broadening disclosure

**Scope:** `DOC-07`, remaining coarse-error and policy-inference observations. Parallel with schema/signature work after their user-facing wording is settled.

**Files and symbols**

- `docs/docs/explanation/security-model.md` — “Client errors are intentionally coarse”.
- `docs/docs/reference/public-api.md` — “Error classifications”.
- `docs/docs/reference/mcp-tools.md` — “Errors”.
- `server.go` — review `projectExecutionError` comments only; no behavioral change planned.
- `mcpserver/server.go` — review `projectToolError` comments only; no behavioral change planned.

**Change**

- Replace the misleading suggestion that root errors generally retain wrapped diagnostic detail. State the exact layers:
  - internal packages and trusted adapter implementations may hold detailed causes;
  - direct `authz/rego.Authorize` errors may name undefined/non-Boolean/evaluation causes;
  - root `codemode.Server` intentionally projects execution, policy, and handler failures to sentinels, preserving only the documented context deadline wrapping;
  - `mcpserver` emits the fixed table strings and redacts resolver/custom-service details and panics;
  - SDK schema errors can identify malformed client-owned fields/values because they occur before trusted resolution.
- Add safe recovery actions without echoing offending input: discover and describe before execution; compare calls with the published signature; shorten search queries; simplify source/result after resource exhaustion; contact the host for authorization-policy failures. State that one allowed or denied input cannot establish whether a policy is default-open, default-deny, complete, or incomplete.
- Preserve the eleven coarse MCP texts and the canary-based non-disclosure guarantee. Do not expose budget values, filtered capability identity, unknown names, argument names, source locations, Rego paths, rule names, or handler messages.

**Cutover**

- Documentation-only. Existing clients and sentinels do not change.

**Focused verification**

- Re-run the existing `TestToolsProjectStableServiceErrors`, `TestToolsSanitizePanics`, and secure-loop canary assertions.
- Re-run the campaign response-only leak corpus with resolver, policy, handler, panic, source, argument, and credential canaries; require zero occurrences.
- Confirm direct Rego errors are more useful after Increment 1 while the same failures remain coarse at root and MCP.

**Acceptance criteria**

- The documentation no longer promises root detail that does not exist.
- A maintainer can identify exactly where trusted diagnostics may be logged or returned.
- Model-visible behavior retains the proven non-disclosure boundary.

### Increment 8 — Repair the disposable campaign harness

**Scope:** `HAR-01` through `HAR-10`, `HAR-12`, and `HAR-13`. This work is independent of CodeMode and belongs only in `/Users/josh/code/meigma/codemode-functional-004` if the harness will be reused.

**Files and symbols**

- `internal/probe/consumer.go` — `caseC08`, `caseC09`.
- `scripts/c01-clean-install.sh` — complete C01 workflow and provenance.
- `internal/probe/emit.go` and `fixtures/copy.go` — preserve typed field order for D06 while still cloning borrowed maps where ownership matters.
- `fixtures/server.go` — `(*ServerBundle).Snapshot`; or install a shared event recorder only in order-sensitive cases.
- `internal/probe/execute.go` — `caseE13`, `caseE14`, `caseE16`, `caseE17`.
- `internal/probe/authzrego.go` — `caseR02`, `caseR03`.
- `fixtures/rego.go` — remove/relabel `MultiResultPolicy`.
- `internal/probe/mcp.go` — `caseM05`, `emitTool`; retain the already-fixed `caseM07` source length.
- `cmd/codemode-mcp/main.go` — `run` EOF classification.
- `prompts/U03-error-recovery.md`, `prompts/U04-filter-spoof.md`, and the reusable U06 mode prompts — promote the accepted QA prompt forms from `evidence/runs/U03/qa-rerun/`, `U04/qa-json-attempt3/`, and `U06/qa-json-*`.

**Change and verification**

- Replace weak observations with the durable supplements already accepted by QA; do not preserve duplicate weak rows as PASS evidence.
- Generate E17 from raw classified records and exact evidence paths.
- Require OMP `--mode json`; count schema reads, writes/server calls, and non-MCP calls separately; reconcile writes against resolver counts. Retain emitted and dispatched argument records separately.
- Add negative controls where the campaign established they are necessary: C08 limits, C09 stale globals, and ordering.
- Treat stdin EOF after a stdio session as a clean fixture-host shutdown; still fail on construction/listen/runtime errors unrelated to normal close.
- Re-run each repaired case once and require the original durable supplement to agree. A mismatch is a harness failure, not a reason to weaken the oracle.

**Acceptance criteria**

- No final verdict depends on a non-discriminating, hardcoded, confounded, pipelined, or prompt-noncompliant observation.
- Every harness row is reproducible from retained source and one recorded command.
- `HAR-11`, `CLOSED-01`, `CLOSED-02`, and `CLOSED-03` remain explicitly closed rather than being silently deleted from campaign history.

### Increment 9 — Report OMP/client argument rewriting upstream

**Scope:** `UP-01`. Independent of every CodeMode and harness increment.

**Evidence to attach**

- `evidence/runs/U03/qa-rerun/stdout.json` and the U03 report’s `UX-12` comparison.
- `evidence/runs/U04/qa-json-attempt3/stdout.json`, especially the `program` → `source` rewrite and dropped `subject`/`role`/`key` fields.
- `evidence/runs/U06/qa-json-allow/stdout.json`, showing `code` → `source`.
- Raw SDK M04 evidence proving CodeMode rejects unknown properties when they actually reach the server.

**Upstream request**

- Ask OMP/client owners either to dispatch the model-emitted object exactly and surface schema validation, or to make normalization explicit in the transcript and user/model feedback. At minimum, tool records must distinguish emitted arguments from dispatched arguments and must not present the former as wire truth.
- Do not request a CodeMode alias for `code` or `program`, do not permit additional properties, and do not add server-side heuristics for client-rewritten calls.

**Acceptance criteria**

- The upstream issue reproduces both renamed keys and dropped fields.
- The CodeMode repository has no compensating change.

## Dependencies and parallelization

- Increments 1, 2, 3, 5, 8, and 9 are mutually independent in behavior. Rego increments 1 and 2 touch the same test/docs files and should be serialized if owned by separate branches.
- Increment 4 should land before the OMP UX rerun. Increment 6 should follow Increment 4 to avoid conflicting edits to `mcpserver.New`.
- Increment 5 should land before judging whether MCP text-mirror escaping still harms the observed signature workflow.
- Increment 7 should be written after Increments 1, 4, 5, and 6 so the final reference describes their exact surfaces.
- Harness repairs do not gate product fixes. They gate any claim from a repeated campaign.
- The upstream OMP issue does not gate CodeMode. Raw SDK and stdio verification remain authoritative.

## Risks and decision points for the executor

- **Signature consumers:** the old return suffix may have undocumented downstream parsers. Use a clean cutover and state the migration to `Description.Output`; do not carry both forms.
- **SDK schema customization:** stay on the pinned `go-sdk v1.7.0` explicit `Tool.OutputSchema` API and `jsonschema-go v0.4.3`. Do not replace typed tool registration or duplicate SDK input validation.
- **Custom `mcpserver.Service` outputs:** explicit non-null schemas will reject nil collections unless the adapter normalizes them. Normalize successful collection values at the adapter boundary and test that path.
- **Security regression:** proactive descriptions are safe; diagnostic payload enrichment is not. Any proposal to include source snippets, names, argument keys/values, policy paths, budget configuration, handler errors, or resolver causes in MCP must be rejected against M09/U03/U04/U06 evidence.
- **R03 invariant:** if a future API accepts non-ground or binding queries, the result contract must be redesigned then. Do not pre-design that API now.
- **R04 wording:** distinguish ignored metadata from rejected external loading. “No fetch occurred” does not mean the annotation was validated.
- **Search behavior:** the evidence supports better in-band instructions. A matching-algorithm change would need a separate behavioral contract for tokenization, ranking, caps, and backward compatibility.
- **Harness ownership:** the campaign tree is disposable and uncommitted. Do not add it to the CodeMode module or CI.

## Ordered verification sequence

No verification is run during this planning task. The executor should verify in this order so failures remain attributable:

1. Run focused `authz/rego` tests for result classes, cancellation, restricted builtins, both schema annotation forms, and concurrency.
2. Run focused `internal/execution` and `internal/binding` tests for duplicate-keyword classification and inclusive depth.
3. Run binding/catalog/MCP tests after the signature cutover; search all repository fixtures for stale `) ->` expectations.
4. Run MCP tool-list tests after description and output-schema changes; inspect the raw advertised schemas before exercising calls.
5. Run MCP in-memory and stdio successful calls, including blank search and empty description collections; verify structured/text equality after parsing.
6. Run the complete CodeMode package test suite and race suite according to repository CI conventions.
7. Execute the first-server tutorial from a clean external module using the published command order.
8. Re-run resolver, policy, handler, panic, source, argument, and credential canary non-disclosure probes; require zero client-response occurrences.
9. Run fresh JSON-mode blind OMP U01/U02/U03 scenarios against one server at a time. Verify model usability from the dispatched argument record and host counters, not from the model-emitted call alone.
10. Run the repaired harness cases and regenerate the coverage ledger from raw evidence. Compare the result with the original durable supplements and preserve any superseded runs as historical evidence.

## Finding-to-plan coverage matrix

| Finding/source | Plan section | Expected end state |
| --- | --- | --- |
| C01 tutorial ergonomics `F-01` | Increment 3 | Direct dependencies are installed before the code is introduced. |
| C02–C09 positive contracts | Decisions 1, 6; Increment 5 | Binding, one-shot builder, copying, immutability, and fresh state remain unchanged. |
| Consumer `UX-01` | Increment 5 | No Go output type names in signatures. |
| Consumer `H-01`, `H-02`, C09 probe corrections | Increment 8 | Durable discriminating harness rows replace weak probes. |
| D01–D07 | Decisions 1, 7, 8 | Exact, deterministic filtering/search/description behavior remains. |
| Catalog `UX-1`/`UX-2`/`UX-4` | Increments 3 and 4 | Exact/raw-limit behavior is explicit; model sees short-query/exact-name guidance. |
| Catalog `UX-3` | Increment 7 | Coarse root behavior is documented accurately without leakage. |
| Catalog `CopyAny` note | Increment 8 | Field order is observed without map round-trip distortion. |
| E01 / `F-E01-NIL` | Increment 3 | Nil context behavior is explicit; no new sentinel. |
| E02–E04, E06–E13, E15–E17 positive contracts | Decisions 1 and 3; Increment 7 | Execution state, phase, authorization order, conversion, cancellation, and taxonomy remain. |
| E05 / `F-E05-DUP` | Increment 3 | Parser and binder classifications are separated in docs/tests. |
| E14 depth gap | Increment 3 | Inclusive scalar-based depth is documented and permanently tested. |
| `F-E14-ATTR`, E13 transcript, E17 ledger | Increment 8 | Confounded/pipelined/hardcoded harness evidence is removed from verdict paths. |
| `F-E16-DL` | Closed inventory | No change; retraction remains recorded. |
| A01–A02, R01–R02, R05–R06 | Decisions 1 and 3 | Trusted input, fail-closed order, cancellation, and concurrency remain. |
| R03 blocked row / Auth `H-1` | Increment 1 and Increment 8 | Unreachable contract removed; no fake multi-result coverage. |
| R04 / Auth `F-1` | Increment 2 | Documentation matches actual annotation and runtime network behavior. |
| Auth `F-2` | Increment 1 | Trusted host gets undefined/non-Boolean distinction; client stays coarse. |
| Auth `F-3` | Increment 1 documentation | Constructor/evaluation timing is explicit. |
| M01, M03–M12 positive contracts | Decisions 1 and 3; Increments 6 and 7 | Three tools, resolver ordering, envelopes, filtering, non-disclosure, and host-owned transport remain. |
| M02 / MCP `UX-1` | Increment 6 | Advertised schemas are non-null where runtime is non-null. |
| MCP `UX-2` | Increment 5 / accepted no separate serializer work | Observed arrow escape disappears; valid JSON mirroring remains SDK-owned. |
| MCP M07, M05, `emitTool`, M12 EOF | Increment 8 or closed inventory | Fixed result probe retained; remaining fixture accounting/lifecycle corrected. |
| U01–U06 task PASS and positive composition/security observations | Decisions 1 and 3; verification steps 8–9 | Product fixes preserve successful composition, recovery, isolation, authorization, and non-disclosure. |
| U01/U02/U03/U05/U06 entry-point recurrence | Increment 4 | Valid program form is on the wire before the first execute. |
| U02/U03/U04/U06 search recurrence | Increment 4 | Short literal-query strategy is on the wire; algorithm remains deterministic. |
| U03 superseded claims/count | Closed inventory; Increment 8 | QA rerun remains authoritative. |
| U04 attempt-2 violation | Closed inventory; Increment 8 | Attempt 3 remains authoritative and prompt is reusable. |
| U06 default-open inference | Increment 7 | Docs warn against inferring policy completeness from a single outcome. |
| U03/U04/U06 OMP rewriting | Increment 9 | Upstream issue filed; CodeMode remains strict. |

## Explicitly out of scope

- Fuzzy search, stemming, ranking, embeddings, pagination, or subject-specific discovery.
- New public error sentinels, client-visible stack/source/argument/policy detail, or speculative telemetry.
- Rego bundles, stores, policy reload, custom builtins, remote OPA, or a schema resolver.
- Process/container isolation, worker architecture, hard CPU/heap isolation, or changing the documented host-owned network/transport boundary.
- Importing, proxying, translating, or calling downstream MCP servers.
- Changing CodeMode to accept `code`, `program`, spoof fields, or any other OMP-rewritten argument shape.
- Product changes for Go toolchain auto-selection, disposable fixture `qty:0`, SDK `ttlMs`/`cacheScope`, or OMP’s `xd://` presentation.
