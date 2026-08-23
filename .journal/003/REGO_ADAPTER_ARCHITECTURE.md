# OPA/Rego authorizer adapter

## Status

Proposed architecture, complexity-reviewed on 2026-08-23. No implementation has started.

## Decision

Add an optional `authz/rego` package that implements the existing `authz.Authorizer` port. Do not change core CodeMode, execution ordering, authorization input, static filtering, or the MCP surface.

The adapter uses OPA's low-level in-process Rego API. It accepts static in-memory modules and one ground Boolean data decision, prepares the policy before serving, evaluates the existing trusted authorization input, and fails closed.

## Package boundary

```text
authz/
└── rego/
    ├── doc.go
    ├── rego.go
    └── rego_test.go
```

Dependency direction:

```text
host ──> authz/rego ──> authz
                  └──> OPA v1/rego

codemode ──> authz
mcpserver ──> codemode
```

Neither `codemode`, `authz`, `mcpserver`, nor `internal/*` imports OPA. Hosts opt in by importing `authz/rego`.

Keep the adapter in the existing Go module initially. Reconsider a nested module only if measured dependency-resolution, CI, download, or version-conflict costs justify separate release machinery.

## Public API

```go
package rego

// Authorizer evaluates one prepared in-process Rego authorization decision.
type Authorizer struct {
    prepared oparego.PreparedEvalQuery
}

// New validates a ground data decision, compiles the supplied in-memory Rego
// modules, and returns an authorizer ready for concurrent use.
func New(
    ctx context.Context,
    decision string,
    modules map[string]string,
) (*Authorizer, error)

// Authorize implements authz.Authorizer.
func (a *Authorizer) Authorize(
    ctx context.Context,
    input authz.AuthorizationInput,
) error
```

Example wiring:

```go
policy, err := regoauthz.New(
    ctx,
    "data.codemode.authz.allow",
    map[string]string{
        "authorization.rego": policySource,
    },
)
if err != nil {
    return err
}

builder := codemode.New(codemode.Options{
    Authorizer: policy,
    Limits:     codemode.DefaultLimits(),
})
```

Do not add `Options`, `DecisionPath`, `Module`, or `ErrInvalidPolicy` in the first adapter. Those types do not protect an invariant or enable useful error recovery.

## Construction lifecycle

`New` is synchronous and one-shot:

1. Reject nil or already-cancelled context.
2. Require at least one in-memory module.
3. Reject blank module filenames.
4. Parse `decision` with OPA's AST parser.
5. Require a ground reference rooted at `data`.
6. Build restricted Rego v1 capabilities.
7. Add each module through OPA's in-memory module API.
8. Prepare the decision using `PrepareForEval`.
9. Return an immutable `*Authorizer`.

Compilation and query preparation happen before the adapter can be installed in a CodeMode server. There is no constructor probe, filesystem access, reload loop, background goroutine, mutable store, or shutdown method.

A synthetic constructor probe cannot prove that a decision returns a Boolean for all real inputs. Runtime validation remains required. Replacing policy means preparing a new authorizer and constructing a new server.

## Policy contract

The configured decision is one ground data reference, for example:

```text
data.codemode.authz.allow
```

Recommended policy shape:

```rego
package codemode.authz

default allow := false

allow if {
    input.subject.id == "subject-1"
    input.capability.id == "records.entry.lookup"
    input.arguments.key != "forbidden"
}
```

The adapter queries the decision directly. It does not generate an intermediate binding query or expose arbitrary query expressions.

Runtime result contract:

| Rego result | Adapter result |
|---|---|
| Exactly one Boolean `true` | `nil` |
| Exactly one Boolean `false` | `authz.ErrDenied` |
| Undefined decision | Policy failure |
| Multiple results | Policy failure |
| Non-Boolean result | Policy failure |
| Evaluation or builtin error | Policy failure |
| Cancelled or expired context | Original context error |

Use OPA's typed result extraction if the spike confirms its exact behavior against the selected version. Only `false` is an intentional denial. Broken policy contracts remain policy failures while still failing closed.

## Policy input

Each invocation receives this exact document:

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

Mapping:

- `subject.id`: trusted `authz.Subject.ID`
- `capability.id`: stable policy identity
- `capability.name`: model-facing dotted name
- `arguments`: canonical arguments produced by the existing binder

Do not add credentials, headers, MCP `_meta`, Starlark source, results, environment data, inferred tenants, or current time.

Use private projection structs with JSON tags. Borrow `AuthorizationInput.Arguments` read-only during synchronous evaluation. Do not add another deep copy unless the spike disproves OPA's evaluator-owned input normalization behavior.

## Runtime ordering

Existing execution ordering remains authoritative:

```text
Starlark call
  → exact binding
  → canonical arguments
  → Rego Authorizer
  → typed handler
  → output conversion
```

`Authorize`:

1. Checks `ctx.Err()`.
2. Evaluates the prepared query with the original context.
3. Checks `ctx.Err()` again to resolve cancellation races.
4. Extracts one Boolean decision.
5. Returns allow, `authz.ErrDenied`, or an ordinary policy error.

Do not recover panics locally. CodeMode's existing execution boundary already converts authorizer panics to policy failure. Add no locks, retries, caches, detached timeout goroutines, or policy-version state.

The returned authorizer holds one immutable prepared query. OPA documents prepared queries as safe for concurrent use; the spike and package tests must still exercise concurrent authorization under the race detector.

## Capability restrictions

The first adapter should:

- use Rego v1 semantics;
- begin with OPA's Rego-v1 capabilities;
- remove every builtin OPA marks `Nondeterministic`;
- set `AllowNet` to an explicit empty slice;
- enable fatal builtin errors;
- disable print statements;
- expose no custom builtins, compiler, store, resolver, schema source, tracer, print hook, SDK manager, or plugin.

This posture is intended to exclude supported external-I/O and nondeterministic paths. The spike must verify the actual behavior against the selected OPA version.

Do not describe compiler strict mode as a security boundary. Do not remove `internal.print` or `trace` as purported I/O controls when print is disabled and no tracer is installed. Decision-path parsing constrains result semantics; it does not prevent I/O.

The adapter is an in-process capability restriction, not a CPU, heap, process, or tenant sandbox.

## Disposable spike

Before production code, run and delete a narrow external spike against the selected OPA version.

Prove:

1. Ground `data` reference parsing and canonical rendering.
2. Direct Boolean decision extraction for `true`, `false`, undefined, non-Boolean, and conflicting or multiple results.
3. Capability filtering rejects representative network and nondeterministic builtins.
4. Empty `AllowNet` blocks the relevant compiler network path.
5. Disabled-print behavior is understood rather than guessed.
6. Fatal builtin errors cannot silently become an undefined branch while another branch allows.
7. `int64` values retain exact policy semantics.
8. Evaluation does not mutate the canonical argument map.
9. One prepared query is race-clean under concurrent evaluation.
10. Cancellation responsiveness and goroutine stability are acceptable.
11. Actual dependency, build-time, and linked-binary costs are recorded.

A failed assumption returns to architecture. Do not patch around it by expanding the public API.

## Production verification

Package-local tests with real OPA must cover:

- valid module preparation;
- invalid decision and module syntax;
- exact trusted input projection;
- stable capability ID versus dotted capability name;
- absent optional arguments;
- `true` allows;
- `false` satisfies `errors.Is(err, authz.ErrDenied)`;
- undefined and non-Boolean decisions are policy failures;
- fatal builtin failures are policy failures;
- prohibited I/O and nondeterministic policies fail preparation;
- cancelled construction and evaluation preserve the context error;
- concurrent authorization under `-race`;
- unchanged canonical arguments;
- one compile-checked example composed with `codemode.Options`.

Do not add a root or MCP integration test. Existing tests already defend canonical binding, authorization-before-handler ordering, failure classification, zero handler calls after denial or policy failure, trusted subject resolution, and the exact three-tool MCP surface.

## Documentation impact

Add or update only:

- `docs/docs/how-to/use-rego-authorization.md`
- public API reference
- documentation index and package list
- security-model explanation

Document construction before `Builder.Build`, the exact input schema, total Boolean and default-deny guidance, the trusted policy-source assumption, capability restrictions, cooperative cancellation, and the lack of hard isolation.

Leave the README, MCP tool reference, and introductory `AllowAll` tutorial unchanged initially.

## Non-goals

Deferred:

- filesystem or environment policy loading;
- base-data documents or general OPA stores;
- bundles and bundle verification;
- high-level OPA SDK and plugins;
- remote OPA;
- policy distribution or control plane;
- watchers, polling, or hot reload;
- mutable policy;
- structured reasons or obligations;
- arbitrary queries and partial evaluation;
- Wasm;
- custom builtins;
- caller-selectable capability sets;
- caching, metrics, tracing, or decision logging;
- subject expansion;
- subject-specific discovery;
- changes to static capability filtering;
- changes to MCP tools or wire shapes;
- hard-isolation claims for untrusted policy authors.

## Review decisions

Recommended defaults:

1. Treat policy source as trusted deployment configuration. If policy authors are mutually untrusted, reject the in-process architecture and use process or container containment.
2. Accept all OPA-valid ground `data` references. Do not invent a custom dotted-path grammar.
3. Keep the adapter in the root module. Revisit only after the spike demonstrates unacceptable dependency cost.

## Sources

- `.journal/001/ARCHITECTURE.md`
- `.journal/001/IMPLEMENTATION_PLAN.md`
- `.journal/002/SUMMARY.md`
- `authz/authz.go`
- `builder.go`
- `internal/binding/input.go`
- `internal/execution/execute.go`
- [OPA Go integration](https://www.openpolicyagent.org/docs/integration#integrating-with-the-go-api)
