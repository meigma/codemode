---
title: Public API reference
description: Exported contracts for CodeMode, authorization, the in-process Rego authorizer, and the MCP server adapter.
---

# Public API reference

CodeMode has four public packages:

- `github.com/meigma/codemode` registers capabilities and runs bounded Starlark programs.
- `github.com/meigma/codemode/authz` defines subjects and authorization decisions.
- `github.com/meigma/codemode/authz/rego` implements the authorization interface with a prepared in-process Rego decision.
- `github.com/meigma/codemode/mcpserver` exposes a CodeMode service through the official MCP Go SDK.

## `codemode`

### Host wiring

Every final binary that calls `Builder.Build` must call
`ServeWorkerAndExit()` as the first statement of `main`, before flag parsing or
ordinary host setup:

```go
func main() {
	codemode.ServeWorkerAndExit()
	// Parse flags and construct the host.
}
```

`ServeWorkerAndExit` returns immediately in an ordinary host process. In a
worker process, it serves exactly one private probe or execution request over
standard input and output and then terminates the process. Because it may call
`os.Exit`, deferred functions in the worker do not run.

Test binaries that call `Builder.Build` need the equivalent first statement in
`TestMain`:

```go
func TestMain(m *testing.M) {
	codemode.ServeWorkerAndExit()
	os.Exit(m.Run())
}
```

`IsWorker()` reports whether the current process was re-executed as a CodeMode
worker. Unless a framework requires minimal setup before worker delegation,
call `ServeWorkerAndExit` directly. For such frameworks, use `IsWorker` only
to perform that setup; `IsWorker` does not serve worker mode.

### Capability registration

`CapabilityID` is the stable deployment and policy identity of a capability. `CapabilityName` is the dotted name visible in discovery and Starlark. A name has at least two valid Starlark identifier segments, such as `records.lookup`. The first dotted segment must not be a reserved root: any standard Starlark universe name, plus `sum`, `json`, and `math`. Nested segments and leaf functions are unaffected, so `stats.sum` remains legal. Roots that were previously legal capability namespaces, including `list`, `str`, `type`, `print`, `range`, `min`, and `max`, are now rejected. A colliding registration is recorded by `Register` and returned by `Build` as `ErrInvalidRegistration` with a host-side diagnostic. There is no built-in `time` module; a host-defined `time.*` capability remains legal.

`Capability[Input, Output]` contains:

| Field | Contract |
| --- | --- |
| `ID CapabilityID` | Stable deployment and policy identity. An empty ID defaults to `Name`; explicit IDs must have no surrounding whitespace and must be unique. An explicit `ID` preserves policy and filter identity across `Name` changes. |
| `Name CapabilityName` | A unique dotted Starlark name. A complete capability name cannot also be another capability's namespace. |
| `Summary string` | Non-empty compact text used by `Search` and returned by discovery, with no surrounding whitespace. |
| `Description string` | Detail returned by `Describe` and used by `Search`. An empty value defaults to `Summary`; an explicit value must have no surrounding whitespace. |
| `SearchTerms []string` | Optional alternative task and resource phrases used only by `Search`. |
| `Handler Handler[Input, Output]` | A non-nil function called after argument binding and authorization. |

`SearchTerms` is discovery-only metadata. `Register` clones the slice. Search
terms are not returned by `Search` or `Describe`, are not callable aliases, and
are not accepted as names by `Describe` or `Execute`. Callers can still infer
indexed vocabulary by probing search results. Do not include secrets,
credentials, policy facts, tenant identifiers, or sensitive examples.

A capability can register at most 16 non-empty search-term phrases with no
surrounding whitespace. Their combined raw size must not exceed 1,024 bytes.
For example:

```go
SearchTerms: []string{
	"fetch entry",
	"find stored item",
},
```

`Handler[Input, Output]` has this signature:

```text
func(context.Context, authz.Subject, Input) (Output, error)
```

The subject is the trusted subject supplied to `Server.Execute`. The input and output are the exact generic types registered for the capability.

The policy and deployment-filter examples use this explicit capability identity:

| Property | Value |
| --- | --- |
| Stable ID | `records.entry.lookup` |
| Dotted name | `records.lookup` |
| Required input | `Key string` with `json:"key"` |
| Optional input | `Limit *int64` with `json:"limit,omitempty"` |
| Output | `Key string` with `json:"key"`; `Count int64` with `json:"count"` |

#### Supported input and output types

Both generic type arguments must be non-pointer structs. Input fields must be
direct exported fields. Output structs can be nested, but every struct in the
output graph must contain only direct exported fields. Embedded fields are not
supported.

A field name defaults to its Go name. One `json` tag can replace it with a
valid Starlark identifier. Other struct tags, ignored fields, duplicate names,
and unsupported JSON tag options are rejected.

Inputs support these scalar fields and named types with the same underlying
kind:

| Go field type | Starlark input | Required | `omitempty` |
| --- | --- | --- | --- |
| `string` | `str` | Yes | Rejected |
| `int64` | `int` | Yes | Rejected |
| `bool` | `bool` | Yes | Rejected |
| `float64` | `float` | Yes | Rejected |
| `*string` | `str` or `None` | No | Accepted but not required |
| `*int64` | `int` or `None` | No | Accepted but not required |
| `*bool` | `bool` or `None` | No | Accepted but not required |
| `*float64` | `float` or `None` | No | Accepted but not required |

Inputs accept keyword arguments only. Integers must fit the signed 64-bit
range, and floats must be finite. CodeMode does not coerce an integer to a
float or a Boolean to an integer. An omitted optional input and an explicit
`None` both produce a nil pointer and omit that key from the fresh canonical
authorization map. Duplicate keyword syntax is rejected by the Starlark parser
as `ErrInvalidProgram` before authorization or handler dispatch. Positional,
unknown, missing required, incorrectly typed, and out-of-range arguments reach
binding and map to `ErrInvalidArguments`.

An output can recursively contain these Go types:

| Go type | Starlark value | Rules |
| --- | --- | --- |
| `string`, `bool`, and named types with those underlying kinds | `str`, `bool` | Values are preserved. |
| `int`, `int8`, `int16`, `int32`, `int64`, and named types with those underlying kinds | `int` | Values are normalized to signed 64-bit integers. |
| `uint`, `uint8`, `uint16`, `uint32`, `uint64`, and named types with those underlying kinds | `int` | Values are normalized to signed 64-bit integers. A value above `math.MaxInt64` is invalid. |
| `float32`, `float64`, and named types with those underlying kinds | `float` | Values are normalized to finite `float64` values. NaN and infinity are invalid. |
| Pointer to a supported type | The referenced value or `None` | Pointers can appear in fields and inside containers. |
| Struct | Dictionary-like object | Fields follow the same export and tag rules at every nesting level. |
| Array or slice of a supported type | List | Arrays and slices have the same model-facing type notation. |
| Map with a string or named-string key and a supported value type | Dictionary | Other map key types are rejected. |
| Slice or array whose element has underlying type `uint8` | List of integers from 0 through 255 | Byte sequences are not base64 encoded. |

A nil pointer field without `omitempty` remains present with `None`; its
discovery type is `T | None`. A nil pointer field with `omitempty` is absent;
at the root its field shape has `required: false`, and inside a struct its name
uses `?`, as in `{detail?: str}`. The omitted field's type does not gain
`| None` from that pointer. `omitempty` is rejected on every non-pointer output
field.

Nil slices, maps, and byte slices become `None`. This remains true even though
their discovery types are `list[T]` or `dict[str, T]`. Non-nil empty slices and
maps become empty lists and dictionaries. A nil pointer inside a list or map
becomes `None`; `omitempty` omits only struct fields.

`Register(builder, capability)` compiles the complete reflected type graph.
Capability-specific failures, including nil handlers, invalid metadata,
unsupported types, and duplicate IDs or names, are accumulated and returned
together by `Build`; each diagnostic names the capability. Unsupported types
include pointer roots; interfaces including `any`; `json.RawMessage`; types
whose value or pointer method set implements `json.Marshaler` or
`encoding.TextMarshaler`; functions, channels, complex values,
`unsafe.Pointer`, and `uintptr`; non-string map keys; cyclic type graphs; and
invalid fields or tags. `Register` panics for a nil or already closed builder
because no future `Build` call can report those lifecycle violations.

Returned values are checked at call time. An unsigned result above
`math.MaxInt64` or a non-finite float maps to `ErrCapabilityFailure`.
Output-depth, per-value byte, and aggregate intermediate-byte exhaustion map to
`ErrResourceLimit`.

### Options and builder lifecycle

`Options` configures one server build:

| Field | Contract |
| --- | --- |
| `Authorizer authz.Authorizer` | Required. Nil and typed-nil implementations are rejected by `Build`. |
| `DisabledCapabilities []CapabilityID` | Stable IDs removed when the immutable catalog is built. `New` copies the slice. |
| `Limits Limits` | Execution and discovery budgets. `New` copies the value; `Build` replaces each zero-valued field with its `DefaultLimits()` value. |

`New(options)` returns a mutable `*Builder`. A builder is single-threaded and one-shot. It accepts registrations until its single `Build` call. `Build` validates the accumulated registrations and returns an immutable, concurrency-safe `*Server`. The first `Build` call closes the builder before full validation. This remains true when the build fails. A later `Build` returns `ErrInvalidRegistration`, and a later `Register` panics. A later change to registrations, options, or capability visibility requires a new builder.

`Build` returns all capability-specific registration failures as one joined
error. It also rejects a missing authorizer, negative signed limits, namespace
collisions, duplicate disabled IDs, disabled IDs that do not match a registered
capability, more than 4,096 registrations, aggregate searchable metadata that
exceeds the internal build budget, and any enabled compact `SearchResult` that
cannot fit the internal structured-response cap. It re-executes the current
binary and performs a fixed five-second private worker probe only after
construction validation succeeds.
The probe detects an absent or nonfunctional worker entry and returns
`ErrInvalidRegistration` with a host-wiring diagnostic that identifies the
missing or misplaced `ServeWorkerAndExit` call. It cannot detect ordinary host
work that completes silently before `ServeWorkerAndExit`; first-statement
ordering is a host obligation. The fixed probe deadline is independent of
`MaxExecutionTime`.

### Static capability filtering

`Options.DisabledCapabilities` uses `CapabilityID`, not the dotted name. Filtering happens once during `Build`. A disabled capability is absent from:

- `Server.Search`
- `Server.Describe`
- the Starlark namespace
- handler dispatch

When a capability omits `ID`, its dotted `Name` is also its filter identity.

Static filtering is build-scoped deployment configuration. Subject- or argument-dependent decisions belong to an `authz.Authorizer`.

### Limits

`DefaultLimits()` returns development defaults for every field:

| Field | Default | Bounds |
| --- | ---: | --- |
| `MaxSourceBytes int` | 65,536 bytes (64 KiB) | Starlark source before execution. |
| `MaxExecutionSteps uint64` | 1,000,000 | Starlark bytecode steps for one execution. |
| `MaxExecutionTime time.Duration` | 5 seconds | Elapsed time starting before waiting for a worker slot; covers spawn, protocol exchange, Starlark execution, and parent dispatch. |
| `MaxNativeCalls uint64` | 100 | Attempted native calls in one execution. |
| `MaxValueDepth int` | 32 | Inclusive nesting depth of any value crossing the worker boundary. |
| `MaxValueBytes int` | 1,048,576 bytes (1 MiB) | Type-preserving encoding of any value crossing the worker boundary. |
| `MaxIntermediateValueBytes int` | 8,388,608 bytes (8 MiB) | Cumulative encoded successful parent-to-child native-result value bodies in one `Execute` call. |
| `MaxSearchQueryBytes int` | 256 bytes | Raw search query before trimming or tokenization. Whitespace padding counts. |
| `MaxSearchResults int` | 20 | Maximum number of entries in `SearchResponse.Results`. |
| `MaxConcurrentExecutions int` | 8 | Concurrent spawn attempts and live worker processes. |

`Build` replaces every zero-valued field with the corresponding value from
`DefaultLimits()`, then validates the complete result. A zero field never means
unlimited. This permits `Limits{}` and partial overrides without restating
unrelated budgets. Negative signed fields return `ErrInvalidRegistration`.
Calling `Limits.Validate()` directly still rejects zero and otherwise
non-positive fields because it validates an already resolved limit set.

The structured search response also has an internal byte bound. This bound
applies to the compact JSON representation of `SearchResponse`; its exact value
is not part of the public configuration contract. The surrounding JSON-RPC
envelope and the MCP SDK's JSON text mirror are outside this bound.

The registration ceiling, aggregate searchable-metadata budget, and
single-result fit check are hard build constraints rather than configurable
`Limits` fields. The aggregate covers the raw bytes of every registered
capability's name, summary, description, and `SearchTerms`; it is checked
before static filtering, so disabling a capability does not reduce that
accounting. The single-result check applies after filtering to each enabled
compact object containing its name, generated signature, and summary.
Exceeding any of these constraints returns `ErrInvalidRegistration`.

To reduce registration count or aggregate metadata, remove capabilities from
the server build, split them across servers, or shorten their discovery
metadata. To make one compact result fit, shorten its capability name, summary,
or input field names that form the generated signature. The internal
structured-response cap is not public configuration and cannot be raised
through `Limits`.

`MaxValueDepth` is inclusive. A scalar or `None` is depth 1. Each tuple, list,
or dictionary wrapper adds one. A scalar with limit 1 succeeds, a one-level
container with limit 2 succeeds, and one more wrapper with limit 2 fails.

`MaxValueDepth` and `MaxValueBytes` apply independently to native-call
arguments, native results, and the final value. `MaxValueBytes` measures
CodeMode's type-preserving worker encoding, not canonical JSON and not the
complete protocol frame. The worker frame cap adds the fixed envelope and, for
native calls, the longest enabled encoded capability ID. Build rejects a
catalog and limit combination whose largest legal frame cannot be represented.

`MaxIntermediateValueBytes` is independent of `MaxValueBytes`. Each
`Execute` call starts with a fresh aggregate budget. After a successful native
result is encoded, CodeMode checks and debits its value-body byte length from
that request's remaining budget. The sum excludes frame envelopes, native-call
arguments, failed handlers, and the final program value. It also does not
measure handler-owned memory, Starlark object memory, process RSS, or a
high-water mark.

A native result must satisfy both limits: `MaxValueBytes` bounds that one
crossing, while `MaxIntermediateValueBytes` bounds the sum of successful
native-result bodies. A sequence of individually valid results can therefore
exhaust the aggregate limit, and a single result can exceed `MaxValueBytes`
while aggregate capacity remains.

`MaxExecutionTime` starts before waiting for a worker slot and covers spawn,
protocol exchange, Starlark execution, and parent dispatch. Killing and
reaping the worker can add operating-system overhead beyond the budget.
If the request context ends first, its cancellation or deadline wins.
`MaxConcurrentExecutions` bounds concurrent spawn attempts and live worker
processes. The one-shot build probe is outside that semaphore.

The parent kills and reaps the Starlark worker when the execution context ends.
Authorizers and handlers run in the parent. CodeMode can return without waiting
for dispatched Go code, but it cannot forcibly stop that code or undo its side
effects. See [Understanding CodeMode's security model](../explanation/security-model.md#cancellation-and-host-code).

### Server operations

`Server` is immutable and safe for concurrent use. Registered authorizers and handlers can be called concurrently and must provide their own concurrency safety.

#### `Search`

```text
Search(query string) (SearchResponse, error)
```

`SearchResponse` and `SearchResult` have these JSON shapes:

```go
type SearchResponse struct {
	Results   []SearchResult `json:"results"`
	Truncated bool           `json:"truncated"`
}

type SearchResult struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Summary   string `json:"summary"`
}
```

`Search` first enforces `MaxSearchQueryBytes` on the raw query. Whitespace
padding counts. It then:

1. trims surrounding Unicode whitespace;
2. treats every rune that is not a Unicode letter or digit as a separator;
3. splits camel-case, acronym-to-word, and letter/number transitions;
4. lowercases each token;
5. removes `a`, `an`, `and`, `by`, `for`, `from`, `in`, `of`, `on`, `or`,
   `the`, `to`, and `with`; and
6. deduplicates the remaining query tokens.

A query with more than 16 distinct normalized tokens returns
`ErrResourceLimit`. A blank query, or one containing only separators and
removed connector tokens, succeeds with:

```json
{"results":[],"truncated":false}
```

Search compares the distinct query tokens with tokens from each enabled
capability's name, `SearchTerms`, summary, and description. Exact token matches
are supported. A query token of at least three Unicode characters can also
match the prefix of a capability token. Arbitrary infix matching, fuzzy
matching, stemming, and built-in synonym expansion are not supported. Hosts
must put alternative task and resource vocabulary in `SearchTerms` or other
descriptive metadata.

For one query token within one field, an exact match ranks above a prefix
match. Search retains only the strongest contribution for that query token.
The field precedence used for weighting is the capability name, then
`SearchTerms`, then the summary, then the description. Terms that occur in
fewer enabled capabilities contribute more than catalog-wide terms. The
numeric scoring weights are internal.

Eligibility depends on the number `q` of distinct normalized query tokens:

| Query tokens | Required matched tokens |
| ---: | ---: |
| 1 | 1 |
| 2 | 2 |
| 3 or more | `ceil(2q / 3)` |

Search ranks every eligible capability before applying output bounds. A
case-insensitive exact dotted-name query, after trimming surrounding
whitespace, ranks first. Remaining ordering is relevance score descending,
then exact dotted name ascending. Static filtering happens before indexing, so
disabled capabilities cannot match or affect ranking.

`Results` is always a non-nil array on success. CodeMode packs the
highest-ranked prefix under `MaxSearchResults` and the internal structured
response-byte bound. `Truncated` is `true` if either bound omits at least one
eligible capability; it does not expose a total count or provide pagination.
The response-byte bound covers the compact JSON representation of
`SearchResponse`, not a surrounding JSON-RPC envelope or the MCP SDK's JSON
text mirror.

`SearchResult` contains:

| JSON field | Type | Meaning |
| --- | --- | --- |
| `name` | string | Exact enabled dotted name. |
| `signature` | string | Invocation-only keyword signature. It ends after the parameter list and never contains a Go output type. |
| `summary` | string | Registered summary. |

`signature` contains the dotted name, a `*` keyword-only marker when there are
parameters, and the ordered input fields with their type notations. It ends at
`)`. The exact forms are
`records.lookup(*, key: str, limit: int | None)` and `records.status()`. The
result contract is `Description.Output`, not `signature`.

An oversized or over-tokenized query returns `ErrResourceLimit`. An unexpected
server-state failure returns `ErrInternal`.

#### `Describe`

```text
Describe(name CapabilityName) (Description, error)
```

`Describe` performs an exact lookup of an enabled dotted name. It neither trims nor case-folds. Clients must pass the exact `name` returned by `Search`. An unavailable, unknown, or disabled name returns `ErrNotFound`.

`Description` contains:

| JSON field | Type | Meaning |
| --- | --- | --- |
| `name` | string | Exact dotted name. |
| `signature` | string | Invocation-only keyword signature. It ends after `)` and never contains a Go output type. |
| `summary` | string | Registered summary. |
| `description` | string | Registered full description. |
| `input` | array of field shapes | Ordered input fields. |
| `output` | array of field shapes | Stable result contract. Models and clients must use these ordered field shapes. |

Each field shape remains the flat object `{name, type, required}`. Input
`type` values are `str`, `int`, `bool`, and `float`; optional pointer inputs
append ` | None` and set `required` to `false`.

Output `type` strings use deterministic compact notation:

| Go shape | Notation |
| --- | --- |
| String, integer, Boolean, or float | `str`, `int`, `bool`, or `float` |
| Array or slice | `list[T]` |
| String-keyed map | `dict[str, T]` |
| Struct | `{field: T, optional?: U}` |
| Pointer without an enclosing `omitempty` field | <code>T &#124; None</code> |

Fields remain in Go declaration order, including fields inside a struct
notation. Arrays and slices share `list[T]` because they produce the same
Starlark value. Repeated pointer layers append ` | None` once. Root output
optionality stays in `required`; nested output optionality uses `?`. Examples
include `list[{title: str, score: float}]`, `dict[str, bool]`,
`{value: str, detail?: str}`, and the nested nullable
`list[str | None] | None`.

`Description.Output` is the stable result contract. Clients that parsed a trailing ` -> <GoType>` suffix on `signature` must stop parsing it and use `output` instead. There is no compatibility suffix, alias, or alternate signature field.

#### `Execute`

```text
Execute(ctx context.Context, subject authz.Subject, program Program) (any, error)
```

`Program` is Starlark source. The context must be non-nil and the subject ID must be non-empty. A nil context is a caller-contract violation and is currently classified as `ErrInternal`.

Every call creates a fresh interpreter and fresh budgets. Module loading is disabled. The predeclared environment is the fixed language surface (`sum`, `json`, `math`, and the standard Starlark builtins) plus the enabled capability namespace. Top-level source loading must define a function named `main` that accepts no positional parameters, keyword-only parameters, variadic positional parameters, or variadic keyword parameters. Native calls are accepted only while `main` is running. There is no built-in `time` module; a host-defined `time.*` capability remains legal.

For each native call, CodeMode performs these operations in order:

1. The worker binds exact keyword arguments against the registered input shape
   and sends normalized values to the parent.
2. The parent rebinds those values to the exact registered Go input and creates
   a fresh canonical authorization map.
3. The parent calls the authorizer with that canonical input.
4. The parent dispatches the handler only when authorization succeeds.
5. The parent converts the exact registered handler output to a bounded
   process-neutral value. The worker then converts that value to Starlark.

After `main` returns, CodeMode converts only its final value. Supported final
values are `None`, booleans, strings, signed 64-bit integers, finite floats,
tuples, lists, and dictionaries with string keys, recursively within configured
limits. `None` becomes JSON `null`, and tuples and lists become JSON arrays.

Only that converted return value is exposed to the caller. Printed text,
globals, and interpreter-local intermediate values are not returned. Each
native-call argument map, native result, and final value is independently
subject to `MaxValueDepth` and `MaxValueBytes`. Successful native-result value
bodies also consume the request-scoped `MaxIntermediateValueBytes` budget,
including results that `main` does not return.

### Error classifications

Use `errors.Is` to inspect these exported sentinel errors. `Server.Execute` removes trusted wrapped causes and returns only the classifications below. Client adapters must not depend on diagnostic text or an unwrapped internal, policy, or handler error.

| Error | Classification |
| --- | --- |
| `ErrInvalidRegistration` | Invalid capability metadata or types, builder use, limits, static filters, or server construction. |
| `ErrUnauthenticated` | Missing trusted subject. |
| `ErrNotFound` | Unknown, unavailable, or disabled capability. |
| `ErrInvalidProgram` | Invalid source, including duplicate keyword syntax, invalid entry point, runtime behavior, or unsupported final value. |
| `ErrInvalidArguments` | Native arguments rejected at binding before authorization: positional, unknown, missing required, incorrectly typed, or out-of-range. |
| `ErrPermissionDenied` | Authorizer error that wraps `authz.ErrDenied`. |
| `ErrPolicyFailure` | Other authorizer error or recovered authorizer panic. |
| `ErrResourceLimit` | Source, step, time, native-call, conversion, result, or search budget exceeded. |
| `ErrCapabilityFailure` | Handler error or invalid handler output conversion. |
| `ErrInternal` | Unexpected framework state, recovered handler panic, other recovered internal failure, or a nil `Execute` context. |

Request cancellation returns `context.Canceled`. A deadline is classified as `ErrResourceLimit` and also wraps `context.DeadlineExceeded` at the root API.

Internal packages and trusted authorizer and handler implementations can hold detailed causes before this projection. A direct `authz/rego.Authorize` call can return an ordinary error that names an undefined or non-Boolean decision or wraps an OPA evaluation failure. `Server.Execute` maps that error to `ErrPolicyFailure` without retaining the cause. If the host needs the detail, record it inside the trusted implementation before returning.

## `authz`

### Subjects

`SubjectID` is a stable, non-secret authenticated identity. `Subject` contains one field, `ID SubjectID`. An empty ID is unauthenticated.

Credentials do not belong in `Subject`. The host keeps credentials in its authentication layer and places only the non-secret subject in trusted Go context.

`WithSubject(ctx, subject)` stores a subject under an `authz`-owned private
context key. `SubjectFromContext(ctx)` returns that subject and reports whether
it was present. Authentication middleware must validate credentials before
calling `WithSubject`; these functions do not authenticate a caller.

### Authorization input

`AuthorizationInput` contains:

| Field | Contract |
| --- | --- |
| `Subject Subject` | Trusted subject for the current execution. |
| `CapabilityID string` | Stable registered policy identity. |
| `CapabilityName string` | Dotted model-facing name. |
| `Arguments map[string]any` | Fresh canonical projection of validated keyword arguments. |

Canonical arguments contain JSON-shaped scalar values from the supported input
matrix: strings, `int64` integers, Booleans, and finite `float64` values. An
omitted optional value or explicit `None` is absent from the map. The canonical
map is separate from the typed input passed to the handler.

### Authorizers

`Authorizer` has one method:

```text
Authorize(context.Context, AuthorizationInput) error
```

Return `nil` to allow dispatch. Return an error that wraps `ErrDenied` for a recognized denial. Any other error is a policy evaluation failure. CodeMode maps these outcomes to `ErrPermissionDenied` and `ErrPolicyFailure`, respectively, without dispatching the handler.

`AllowAllAuthorizer` permits every native call whose arguments bind successfully. `AllowAll()` constructs it explicitly. Use `AllowAll()` only when every resolved subject may call every enabled capability; otherwise supply an `authz.Authorizer` that evaluates the trusted subject, stable capability ID, and canonical arguments.

## `authz/rego`

The optional `github.com/meigma/codemode/authz/rego` package implements `authz.Authorizer` with OPA's in-process Rego evaluator. Importing this package adds the OPA dependency; the core `codemode`, `authz`, and `mcpserver` packages do not import OPA.

### `Authorizer`

`Authorizer` is a concrete authorizer that holds one immutable prepared query. A successfully constructed `*Authorizer` is safe for concurrent calls.

### `New`

```text
New(ctx context.Context, decision string, modules map[string]string) (*Authorizer, error)
```

`New` accepts one ground `data` reference, such as `data.codemode.authz.allow`, and one or more in-memory Rego modules keyed by non-blank filename. It validates the decision's reference syntax, sorts the filenames, compiles the modules with Rego v1 semantics, and prepares the direct decision query synchronously. `New` cannot prove that the prepared decision is defined and Boolean for every future input; those outcomes are checked when `Authorize` evaluates the query.

Surrounding whitespace on the decision string is accepted by OPA's reference parser. CodeMode does not add a second trim or normalization rule.

`New` rejects a nil context, an already-canceled context, no modules, blank filenames, a non-ground or non-`data` decision, invalid Rego, and policy that uses an unavailable builtin. Context cancellation takes precedence over an OPA preparation error when they race. Other failures are ordinary constructor errors; this package defines no error sentinel.

The prepared evaluator starts from OPA's Rego v1 capabilities and removes every builtin that OPA marks nondeterministic. That removal takes away runtime network-capable builtins, including `http.send`, plus DNS, runtime, random, time, and UUID builtins. `AllowNet` is a separate non-nil empty deny-all host list. It is defense in depth, not the mechanism that removes `http.send`. The evaluator also enables `StrictBuiltinErrors(true)` and uses `EnablePrintStatements(false)`. The package installs no custom builtins, hooks, tracer, store, schema set, schema resolver, or remote policy service.

Metadata `schema["https://example.invalid/schema.json"]` is accepted but ignored: there is no validation and no fetch. Metadata with an external `$ref: "https://example.invalid/schema.json"` asks OPA to load a remote schema and is rejected because remote reference loading is disabled.

### `Authorize`

```text
Authorize(ctx context.Context, input authz.AuthorizationInput) error
```

`Authorize` evaluates the prepared decision once. A nil receiver or nil context returns an ordinary policy error instead of panicking. An already-canceled context, or cancellation that becomes visible during evaluation, returns `ctx.Err()`.

The Rego input contains exactly these fields:

```json
{
  "subject": {
    "id": "subject-1"
  },
  "capability": {
    "id": "records.entry.lookup",
    "name": "records.lookup"
  },
  "arguments": {
    "key": "alpha",
    "limit": 2
  }
}
```

`subject.id` is the trusted subject ID. `capability.id` is the stable policy identity; `capability.name` is the dotted discovery and Starlark name. `arguments` is the canonical map from `AuthorizationInput`, borrowed read-only for the synchronous evaluation. An omitted optional argument is absent from the map.

A ground decision is either undefined or yields one value. That value must be Boolean.

| Result | Return value |
| --- | --- |
| Boolean `true` | `nil` |
| Boolean `false` | `authz.ErrDenied` |
| Undefined decision | Ordinary policy error with text `rego: decision is undefined` |
| Non-Boolean decision | Ordinary policy error with text `rego: decision must be boolean` |
| Evaluation or builtin failure | Ordinary policy error |
| Canceled or expired context | `ctx.Err()` |

Only Boolean `false` is a recognized denial. CodeMode maps it to `ErrPermissionDenied`; it maps the ordinary policy errors to `ErrPolicyFailure`. Define a total Boolean decision with `default allow := false` so that unmatched input is an intentional denial rather than an undefined policy failure.

See [Use Rego for authorization](../how-to/use-rego-authorization.md) for server wiring and [Understanding CodeMode's security model](../explanation/security-model.md#rego-policy-runs-in-process) for the in-process trust boundary.

## `mcpserver`

### `Service`

`Service` is the adapter's application port:

```text
Search(query string) (codemode.SearchResponse, error)
Describe(name codemode.CapabilityName) (codemode.Description, error)
Execute(context.Context, authz.Subject, codemode.Program) (any, error)
```

A root `*codemode.Server` implements `Service`. The adapter relies on the service to enforce catalog filtering, discovery limits, authorization, and execution limits.

### `InvocationResolver`

`InvocationResolver` has one method:

```text
Resolve(context.Context) (authz.Subject, error)
```

Every valid MCP tool call runs the resolver before discovery or execution. A
resolver error or empty subject ID stops the operation. Resolvers must not
derive identity from tool arguments, Starlark source, MCP `_meta`, or other
client-controlled metadata, and must not return credential material.

`StaticSubject(subject)` returns a resolver that uses one fixed identity. It is
only for single-user transports where process ownership is the authentication
boundary, such as a local stdio server. Multi-user hosts must not use it.

`ContextSubject()` reads subjects stored with `authz.WithSubject`. The host's
authentication middleware remains responsible for validating credentials and
installing a subject for each request.

### `New`

```text
New(service Service, resolver InvocationResolver) (*mcp.Server, error)
```

`New` rejects nil and typed-nil dependencies with `ErrInvalidRegistration`. The returned official SDK server exposes exactly `search_api`, `describe_api`, and `execute`. It does not own authentication, transport creation, listeners, cancellation, or shutdown. The host connects the returned server to an official MCP transport and owns that lifecycle.

See [MCP tool reference](mcp-tools.md) for the wire contracts and [Understanding CodeMode's security model](../explanation/security-model.md) for the trust boundary.
