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

### Capability registration

`CapabilityID` is the stable deployment and policy identity of a capability. `CapabilityName` is the dotted name visible in discovery and Starlark. A name has at least two valid Starlark identifier segments, such as `records.lookup`.

`Capability[Input, Output]` contains:

| Field | Contract |
| --- | --- |
| `ID CapabilityID` | Non-empty, with no surrounding whitespace. IDs must be unique. Use this value for deployment filters and policy rules. |
| `Name CapabilityName` | A unique dotted Starlark name. A complete capability name cannot also be another capability's namespace. |
| `Summary string` | Non-empty compact text searched by `Search`, with no surrounding whitespace. |
| `Description string` | Non-empty detail returned by `Describe`, with no surrounding whitespace. |
| `Handler Handler[Input, Output]` | A non-nil function called after argument binding and authorization. |

`Handler[Input, Output]` has this signature:

```text
func(context.Context, authz.Subject, Input) (Output, error)
```

The subject is the trusted subject supplied to `Server.Execute`. The input and output are the exact generic types registered for the capability.

The site examples use one capability contract throughout:

| Property | Value |
| --- | --- |
| Stable ID | `records.entry.lookup` |
| Dotted name | `records.lookup` |
| Required input | `Key string` with `json:"key"` |
| Optional input | `Limit *int64` with `json:"limit,omitempty"` |
| Output | `Key string` with `json:"key"`; `Count int64` with `json:"count"` |

#### Supported input and output types

Both generic type arguments must be non-pointer structs. Fields must be exported, direct fields; embedded fields are not supported. A field name defaults to its Go name. A single `json` tag can replace it with a valid Starlark identifier. Other struct tags, ignored fields, and unsupported JSON tag options are rejected.

The shipped input matrix is:

| Go field type | Starlark input | Required | JSON tag rule |
| --- | --- | --- | --- |
| `string` | `str` | Yes | `omitempty` is rejected. |
| `*int64` | `int` or `None` | No | `omitempty` is accepted but is not required for optional binding. |

Inputs accept keyword arguments only. Positional, unknown, duplicate, missing required, incorrectly typed, and out-of-range arguments are rejected. An omitted optional integer and an explicit `None` both produce a nil pointer.

The shipped output matrix is:

| Go field type | Starlark value | Final JSON value |
| --- | --- | --- |
| `string` | `str` | string |
| `int64` | `int` | signed 64-bit integer |
| `bool` | `bool` | boolean |
| `float64` | `float` | number |

Every output field is required. `omitempty` is rejected on output fields. A `float64` output must be finite. Nested structs, slices, maps, other pointer fields, and other scalar field types are outside the shipped registration matrix.

`Register(builder, capability)` compiles and validates the generic binding contract and retains the capability. It rejects a nil or closed builder, a nil handler, invalid metadata, unsupported types, and duplicate IDs or names.

### Options and builder lifecycle

`Options` configures one server build:

| Field | Contract |
| --- | --- |
| `Authorizer authz.Authorizer` | Required. Nil and typed-nil implementations are rejected by `Build`. |
| `DisabledCapabilities []CapabilityID` | Stable IDs removed when the immutable catalog is built. `New` copies the slice. |
| `Limits Limits` | Positive execution and discovery budgets. `New` copies the value. |

`New(options)` returns a mutable `*Builder`. A builder is single-threaded and one-shot:

1. Call `Register` for each capability.
2. Call `Build` once.
3. Use the returned immutable `*Server` concurrently.

The first `Build` call closes the builder before full validation. This remains true when the build fails. Later `Register` and `Build` calls return `ErrInvalidRegistration`. Create another builder to change registrations, options, or capability visibility.

`Build` also rejects a missing authorizer, non-positive limits, namespace collisions, duplicate disabled IDs, and disabled IDs that do not match a registered capability.

### Static capability filtering

`Options.DisabledCapabilities` uses `CapabilityID`, not the dotted name. Filtering happens once during `Build`. A disabled capability is absent from:

- `Server.Search`
- `Server.Describe`
- the Starlark namespace
- handler dispatch

The filter is deployment configuration, not a per-request policy. Use an `authz.Authorizer` for decisions that depend on the subject or validated arguments.

### Limits

`DefaultLimits()` returns development defaults for every field:

| Field | Default | Bounds |
| --- | ---: | --- |
| `MaxSourceBytes int` | 65,536 bytes (64 KiB) | Starlark source before execution. |
| `MaxExecutionSteps uint64` | 1,000,000 | Starlark bytecode steps for one execution. |
| `MaxExecutionTime time.Duration` | 5 seconds | Elapsed time for one execution. |
| `MaxNativeCalls uint64` | 100 | Attempted native calls in one execution. |
| `MaxValueDepth int` | 32 | Nesting depth of the converted final value. |
| `MaxResultBytes int` | 1,048,576 bytes (1 MiB) | JSON encoding of the final value. |
| `MaxSearchQueryBytes int` | 256 bytes | Search query before normalization. |
| `MaxSearchResults int` | 20 | Search results returned. |

`Limits.Validate()` returns `ErrInvalidRegistration` if any field is zero or otherwise non-positive. Zero never means unlimited. Passing a zero-value `Limits` does not select defaults; use `DefaultLimits()` explicitly.

Execution limits constrain an in-process interpreter. `MaxExecutionTime` cancels Starlark evaluation, but it cannot forcibly interrupt blocking Go authorizers or handlers. See [Security model](../explanation/security-model.md#cancellation-and-host-code).

### Server operations

`Server` is immutable and safe for concurrent use. Registered authorizers and handlers can be called concurrently and must provide their own concurrency safety.

#### `Search`

```text
Search(query string) ([]SearchResult, error)
```

`Search` first enforces `MaxSearchQueryBytes`, then trims surrounding whitespace and normalizes case. It performs substring matching against enabled capability names and summaries. Results are sorted by exact capability name and capped by `MaxSearchResults`. A blank normalized query returns an empty, non-nil result.

`SearchResult` contains these JSON fields:

| Field | Type | Meaning |
| --- | --- | --- |
| `name` | string | Exact enabled dotted name. |
| `signature` | string | Keyword-only Starlark signature generated from the registered binding. |
| `summary` | string | Registered summary. |

An oversized query returns `ErrResourceLimit`. An unexpected server-state failure returns `ErrInternal`.

#### `Describe`

```text
Describe(name CapabilityName) (Description, error)
```

`Describe` performs an exact, case-sensitive lookup of an enabled dotted name. An unavailable, unknown, or disabled name returns `ErrNotFound`.

`Description` contains:

| JSON field | Type | Meaning |
| --- | --- | --- |
| `name` | string | Exact dotted name. |
| `signature` | string | Generated Starlark signature. |
| `summary` | string | Registered summary. |
| `description` | string | Registered full description. |
| `input` | array of field shapes | Ordered input fields. |
| `output` | array of field shapes | Ordered output fields. |

Each field shape has `name` (string), `type` (string), and `required` (boolean). Input shapes use `str` and `int | None`. Output shapes use `str`, `int`, `bool`, and `float`. Output fields always report `required: true`.

#### `Execute`

```text
Execute(ctx context.Context, subject authz.Subject, program Program) (any, error)
```

`Program` is Starlark source. The context must be non-nil and the subject ID must be non-empty.

Every call creates a fresh interpreter and fresh budgets. Module loading is disabled. The enabled capability names form the predeclared namespace. Source loading must define a function named `main` that accepts no positional parameters, keyword-only parameters, variadic positional parameters, or variadic keyword parameters. Native capability calls are accepted only while `main` is running.

For each native call, CodeMode performs these operations in order:

1. Bind exact keyword arguments to the registered Go input and create a fresh canonical JSON-shaped argument map.
2. Call the authorizer with that canonical input.
3. Dispatch the handler only when authorization succeeds.
4. Convert the handler's exact registered output to Starlark.

After `main` returns, CodeMode converts only its final value. Supported final values are `None`, booleans, strings, signed 64-bit integers, finite floats, tuples, lists, and dictionaries with string keys, recursively within configured limits. `None` becomes JSON `null`, and tuples and lists become JSON arrays.

Printed text, globals, and intermediate values do not cross the execution boundary.

### Error classifications

Use `errors.Is` to inspect these exported sentinel errors. Client adapters can replace wrapped diagnostic detail with the sentinel text.

| Error | Classification |
| --- | --- |
| `ErrInvalidRegistration` | Invalid capability metadata or types, builder use, limits, static filters, or server construction. |
| `ErrUnauthenticated` | Missing trusted subject. |
| `ErrNotFound` | Unknown, unavailable, or disabled capability. |
| `ErrInvalidProgram` | Invalid source, entry point, runtime behavior, or unsupported final value. |
| `ErrInvalidArguments` | Native arguments rejected before authorization. |
| `ErrPermissionDenied` | Authorizer error that wraps `authz.ErrDenied`. |
| `ErrPolicyFailure` | Other authorizer error or recovered authorizer panic. |
| `ErrResourceLimit` | Source, step, time, native-call, conversion, result, or search budget exceeded. |
| `ErrCapabilityFailure` | Handler error or invalid handler output conversion. |
| `ErrInternal` | Unexpected framework state, recovered handler panic, or other recovered internal failure. |

Request cancellation returns `context.Canceled`. A deadline is classified as `ErrResourceLimit` and also wraps `context.DeadlineExceeded` at the root API.

## `authz`

### Subjects

`SubjectID` is a stable, non-secret authenticated identity. `Subject` contains one field, `ID SubjectID`. An empty ID is unauthenticated.

Credentials do not belong in `Subject`. The host keeps credentials in its authentication layer and places only the non-secret subject in trusted Go context.

### Authorization input

`AuthorizationInput` contains:

| Field | Contract |
| --- | --- |
| `Subject Subject` | Trusted subject for the current execution. |
| `CapabilityID string` | Stable registered policy identity. |
| `CapabilityName string` | Dotted model-facing name. |
| `Arguments map[string]any` | Fresh canonical projection of validated keyword arguments. |

Canonical arguments contain JSON-shaped values from the shipped input matrix. A supplied string is a string and a supplied optional integer is an `int64`. An omitted optional integer or explicit `None` is absent from the map. The canonical map is separate from the typed input passed to the handler.

### Authorizers

`Authorizer` has one method:

```text
Authorize(context.Context, AuthorizationInput) error
```

Return `nil` to allow dispatch. Return an error that wraps `ErrDenied` for a recognized denial. Any other error is a policy evaluation failure. CodeMode maps these outcomes to `ErrPermissionDenied` and `ErrPolicyFailure`, respectively, without dispatching the handler.

`AllowAllAuthorizer` permits every valid native call. `AllowAll()` constructs it explicitly. This policy is deliberate in the simple example so that the authorization choice is visible; production hosts normally supply policy that evaluates the trusted subject, stable capability ID, and canonical arguments.

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

See [Use Rego for authorization](../how-to/use-rego-authorization.md) for server wiring and [Security model](../explanation/security-model.md#rego-policy-runs-in-process) for the in-process trust boundary.

## `mcpserver`

### `Service`

`Service` is the adapter's application port:

```text
Search(query string) ([]codemode.SearchResult, error)
Describe(name codemode.CapabilityName) (codemode.Description, error)
Execute(context.Context, authz.Subject, codemode.Program) (any, error)
```

A root `*codemode.Server` implements `Service`. The adapter relies on the service to enforce catalog filtering, discovery limits, authorization, and execution limits.

### `InvocationResolver`

`InvocationResolver` has one method:

```text
Resolve(context.Context) (authz.Subject, error)
```

The resolver reads a subject from typed, host-owned Go context. It must not derive identity from tool arguments, Starlark source, MCP `_meta`, or other client-controlled request metadata. A resolver error or empty subject ID stops every tool before discovery or execution. A resolver must not return credential material.

### `New`

```text
New(service Service, resolver InvocationResolver) (*mcp.Server, error)
```

`New` rejects nil and typed-nil dependencies with `ErrInvalidRegistration`. The returned official SDK server exposes exactly `search_api`, `describe_api`, and `execute`. It does not own authentication, transport creation, listeners, cancellation, or shutdown. The host connects the returned server to an official MCP transport and owns that lifecycle.

See [MCP tool reference](mcp-tools.md) for the wire contracts and [Security model](../explanation/security-model.md) for the trust boundary.
