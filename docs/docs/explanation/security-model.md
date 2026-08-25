---
title: Security model
description: Trust boundaries, authorization order, cancellation, and worker-process containment in CodeMode.
---

# Security model

CodeMode separates client-controlled Starlark from host-controlled identity, policy, and Go handlers. The separation depends on the host establishing a trusted request context, installing the worker entry point, and ensuring authorizers and handlers follow their contracts. CodeMode does not authenticate clients or provide operating-system tenant quotas.

## The host establishes identity

The host authenticates a connection or request before the MCP adapter uses it. Authentication middleware or process composition stores a non-secret `authz.Subject` in typed, host-owned Go context. An `mcpserver.InvocationResolver` reads that context for every `search_api`, `describe_api`, and `execute` call.

Typed Go context matters because it gives the host an identity channel outside model-visible data. These values are untrusted and cannot establish or replace a subject:

- MCP tool arguments
- Starlark source or values
- MCP request `_meta`
- other client-controlled request metadata

Credentials stay in the host's authentication layer. They are not fields on `authz.Subject`, authorization arguments, capability inputs, or Starlark values. The resolver returns only a stable, non-secret subject ID. A resolver error or empty subject ID produces the coarse `unauthenticated` failure before discovery or execution starts.

`mcpserver.New` creates an SDK server, not a complete network service. The host owns authentication, official MCP transport creation, listeners, request cancellation, connection lifecycle, and shutdown. CodeMode does not take ownership of those resources and does not provide a generic downstream MCP forwarding path.

## Validation precedes authority

A native capability call crosses three boundaries in a fixed order:

1. **Exact binding and canonicalization.** Duplicate keyword syntax is rejected by the Starlark parser as `ErrInvalidProgram` before this step. Positional, missing, unknown, incorrectly typed, and out-of-range arguments reach binding and map to `ErrInvalidArguments`. Successful binding creates the exact registered Go input and a fresh JSON-shaped argument map.
2. **Authorization.** CodeMode passes the trusted subject, stable capability ID, dotted capability name, and canonical arguments to `authz.Authorizer`.
3. **Handler dispatch.** CodeMode calls the typed handler only if authorization returns `nil`.

This order prevents policy from interpreting malformed Starlark values and prevents a handler from running before policy has evaluated the exact input it will receive. The canonical map is separate from the typed handler input, so policy cannot rewrite the handler's arguments by mutating the map.

An authorizer reports a recognized denial with an error that wraps `authz.ErrDenied`. CodeMode classifies that outcome as `permission denied` and does not dispatch the handler. Any other authorizer error, and an authorizer panic recovered at the boundary, becomes `authorization policy failure`. Policy diagnostic text does not cross the MCP boundary.

Authorization is evaluated for every attempted native call whose arguments bind successfully. A prior allowed call does not grant authority to a later call.

## `AllowAll` is an explicit policy choice

`authz.AllowAll()` returns an authorizer because the server never treats a missing authorizer as permission. The simple example uses it deliberately so that a minimal server is complete and the absence of policy logic is visible.

`AllowAll` approves every valid native call for every resolved subject. It is not authentication and does not inspect capability identity or arguments. Production hosts normally supply an authorizer that evaluates the resolved subject, the stable capability ID, and the canonical arguments. A production host should use `AllowAll` only when unrestricted access to every enabled capability is the intended policy.

## Rego policy runs in process

The optional `authz/rego` adapter prepares trusted Rego module source as an OPA library inside the CodeMode host. It does not contact a remote OPA service. Module source must come from trusted deployment configuration, such as a compiled-in string or `go:embed` file. The adapter's restrictions reduce the policy evaluator's capabilities; they do not make policy from an untrusted author safe to run in the host process.

The adapter starts with OPA's Rego v1 capabilities and removes every builtin that OPA marks nondeterministic. That removal is what takes away runtime network-capable builtins, including `http.send`, plus DNS, runtime, random, time, and UUID builtins, before policy preparation.

The adapter also sets `AllowNet` to a non-nil empty slice. That empty list is a deny-all host allowlist and defense in depth. It is not the mechanism that removes `http.send`. If a remaining code path still tried to reach a network host, the empty list would deny it.

CodeMode installs no schema set and no schema resolver. Metadata `schema["https://example.invalid/schema.json"]` names a schema in a set that was never installed, so the annotation is accepted but ignored: there is no validation and no fetch. Metadata with an external `$ref: "https://example.invalid/schema.json"` asks OPA to load a remote schema and is rejected because remote reference loading is disabled.

`StrictBuiltinErrors(true)` makes builtin errors fatal. A failing builtin cannot become an undefined rule branch while another branch allows the call. `EnablePrintStatements(false)` erases print calls during compilation. The adapter installs no print hook, tracer, custom builtin, data store, or other policy hook.

The configured decision is one direct, ground `data` reference. Construction validates that reference syntax and prepares the policy; it cannot prove that the decision is defined and Boolean for every future input. A ground decision is either undefined or yields one value. That value must be Boolean. Boolean `true` allows a call. Boolean `false` is a recognized denial. Undefined and non-Boolean decisions are policy failures, as are evaluation and builtin errors. A total decision with `default allow := false` turns unmatched input into an intentional denial while still failing closed when the policy contract is broken.

These controls restrict policy inputs and evaluator capabilities, not resource consumption or process authority. OPA, authorizers, and handlers run inside the host process; moving Starlark to a worker does not isolate Rego policy. A host that does not trust its policy authors needs an external process or container boundary for policy evaluation.

See [Use Rego for authorization](../how-to/use-rego-authorization.md) for configuration and the [`authz/rego` API reference](../reference/public-api.md#authzrego) for the exact input and result contracts.

## Static filtering reduces the exposed catalog

`Options.DisabledCapabilities` removes capabilities by stable `CapabilityID` when the immutable server is built. Disabled entries are absent from search results, exact description, the Starlark namespace, and execution. An unknown disabled ID fails the build rather than silently leaving a capability exposed.

Static filtering is useful for deployment-wide availability, but it is not dynamic authorization. It cannot express subject-specific or argument-specific decisions. Conversely, authorization alone does not hide a capability's metadata from discovery. Use static filtering to remove a capability from the deployment surface and authorization to decide whether an enabled native call may dispatch.

## Client errors are intentionally coarse

Detailed causes exist only on the trusted side of the public boundary. Internal packages and host authorizers, resolvers, and handlers can hold or log those causes. A direct call to `authz/rego.Authorize` can return an ordinary error that identifies an undefined or non-Boolean decision, or carries an OPA evaluation or builtin failure.

`codemode.Server.Execute` removes those trusted causes. It returns the documented public sentinel for execution, policy, handler, resource, and internal failures. Request cancellation returns `context.Canceled`. A deadline returns `ErrResourceLimit` and preserves `context.DeadlineExceeded` for `errors.Is`; no other execution cause remains wrapped at the root API.

The MCP adapter narrows the boundary again. It emits only the fixed error texts in the [MCP tool reference](../reference/mcp-tools.md#errors). Resolver and custom-service details and recovered panic values become coarse responses. SDK input-schema errors are different: they occur before trusted subject resolution and can identify malformed client-owned fields or values.

This projection prevents trusted diagnostic detail from becoming model-visible. In particular, MCP responses do not expose budget values, filtered capability identities, unknown requested names, argument names or values, source locations or text, Rego decision paths or rule names, handler messages, credentials, panic values, or stack details.

If a host needs detailed diagnostics, its trusted authorizer, resolver, or handler must record them before returning. CodeMode cannot recover a discarded cause after the root or MCP projection. Apply the host's normal access controls and redaction rules to those logs.

An allowed or denied call proves only the result for that subject, capability, and canonical argument set. It does not show whether the policy is default-open, default-deny, complete, or incomplete. Treat an unexpected denial or policy failure as a reason to contact the host, not as evidence about the policy's rules or defaults.

## Execution state does not cross calls

Every `Server.Execute` call starts a fresh process by re-executing the host
binary. The child receives only the immutable enabled-capability manifest,
positive execution limits, and one submitted program through CodeMode's private
protocol. It constructs a fresh Starlark interpreter. Module loading is
disabled, and the only predeclared application functions are the enabled
capability namespace. Native calls are rejected while top-level source is
loading and are accepted only while the required zero-argument `main()`
function runs.

After `main` returns, the worker converts only its final value to
type-preserving wire data under the depth and encoded-size limits. Printed text
is discarded. Globals, source-loading values, intermediate expressions, and
unrelated native results do not cross the boundary. No Starlark globals or
mutable interpreter state carry into the next execute call.

Capability handlers do not run in the worker. A native call crosses the private
protocol, is rebound to the exact registered input type in the parent, is
authorized there, and is then dispatched to the parent handler. The validated
native result crosses back to the worker so Starlark can continue.

These rules isolate interpreter state and make Starlark execution killable.
They do not confine registered Go code: authorizers, the optional Rego
evaluator, and handlers run in the host process with the privileges the host
gave that process.

## Cancellation and host code

CodeMode derives one execution context from `MaxExecutionTime` and the request
context. The elapsed budget starts before waiting for a worker slot and covers
process startup, protocol exchange, Starlark execution, parent authorization
and dispatch, and worker cleanup. The interpreter separately enforces source,
bytecode-step, attempted-native-call, crossing-value depth, and crossing-value
size limits.

When the execution context ends, the parent closes the worker pipes, kills the
worker if necessary, and reaps it exactly once. This hard-preempts Starlark,
including a monolithic built-in that does not observe interpreter cancellation.

Cancellation remains cooperative after a native call reaches parent Go code.
Parent dispatch runs asynchronously so `Server.Execute` can return after
canceling and reaping its worker, but CodeMode cannot forcibly stop an
authorizer or handler goroutine or undo its side effects. A non-cooperative
authorizer or handler may continue consuming host resources after `Execute`
returns.

The Rego adapter passes the context into OPA evaluation and checks cancellation
both before and after that call. The second check preserves `context.Canceled`
or `context.DeadlineExceeded` when cancellation races with an OPA error.
Cancellation does not forcibly interrupt arbitrary parent Go code.

Authorizers and handlers must:

- honor the supplied context for I/O, locks, waits, and downstream calls
- return promptly after cancellation
- bound their own memory, network, storage, and retry behavior
- avoid exposing credentials or trusted diagnostics in returned values or client-visible errors
- be safe for concurrent calls when the immutable server is shared

CodeMode recovers panics at selected boundaries and returns a coarse
classification. Panic recovery does not replace normal error handling,
cancellation, or resource control in host code.

## Worker processes are not tenant isolation

The worker boundary separates Starlark interpreter state from the parent and
permits process kill and reap. It does not establish a complete security
boundary between mutually untrusted tenants. A worker runs as the host
operating-system user and re-executes the host binary. CodeMode supplies an
environment containing only its private worker marker and passes no extra file
descriptors, but it provides no operating-system CPU or memory quota. Package
initialization and any setup placed before the worker entry point still run in
the child with that user's filesystem and network authority.

The restricted Starlark environment exposes no file, network, environment, or
process built-ins. Native access is limited to the enabled capability manifest
and every call returns to the parent authorization boundary. Those language
restrictions reduce reachability; they do not replace operating-system
containment.

A host that needs a hard tenant, CPU, heap, filesystem, credential, or network
boundary must add container or workload isolation and operating-system resource
controls. Keep credentials and other parent-only setup after
`ServeWorkerAndExit` so worker processes do not initialize resources they do
not need.

See [Public API reference](../reference/public-api.md) for the Go contracts and [MCP tool reference](../reference/mcp-tools.md) for the exact client-visible surface.
