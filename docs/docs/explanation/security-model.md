---
title: Security model
description: Trust boundaries, authorization order, cancellation limits, and in-process containment in CodeMode.
---

# Security model

CodeMode separates client-controlled Starlark from host-controlled identity, policy, and Go handlers. The separation depends on the host establishing a trusted request context and on authorizers and handlers following their contracts. CodeMode does not authenticate clients or isolate tenants in separate processes.

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

1. **Exact binding and canonicalization.** CodeMode rejects positional, missing, duplicate, unknown, incorrectly typed, and out-of-range arguments. It creates the exact registered Go input and a fresh JSON-shaped argument map.
2. **Authorization.** CodeMode passes the trusted subject, stable capability ID, dotted capability name, and canonical arguments to `authz.Authorizer`.
3. **Handler dispatch.** CodeMode calls the typed handler only if authorization returns `nil`.

This order prevents policy from interpreting malformed Starlark values and prevents a handler from running before policy has evaluated the exact input it will receive. The canonical map is separate from the typed handler input, so policy cannot rewrite the handler's arguments by mutating the map.

An authorizer reports a recognized denial with an error that wraps `authz.ErrDenied`. CodeMode classifies that outcome as `permission denied` and does not dispatch the handler. Any other authorizer error, and an authorizer panic recovered at the boundary, becomes `authorization policy failure`. Policy diagnostic text does not cross the MCP boundary.

Authorization is evaluated for every attempted native call whose arguments bind successfully. A prior allowed call does not grant authority to a later call.

## `AllowAll` is an explicit policy choice

`authz.AllowAll()` returns an authorizer because the server never treats a missing authorizer as permission. The simple example uses it deliberately so that a minimal server is complete and the absence of policy logic is visible.

`AllowAll` approves every valid native call for every resolved subject. It is not authentication and does not inspect capability identity or arguments. Production hosts normally supply an authorizer that evaluates the resolved subject, the stable capability ID, and the canonical arguments. A production host should use `AllowAll` only when unrestricted access to every enabled capability is the intended policy.

## Static filtering reduces the exposed catalog

`Options.DisabledCapabilities` removes capabilities by stable `CapabilityID` when the immutable server is built. Disabled entries are absent from search results, exact description, the Starlark namespace, and execution. An unknown disabled ID fails the build rather than silently leaving a capability exposed.

Static filtering is useful for deployment-wide availability, but it is not dynamic authorization. It cannot express subject-specific or argument-specific decisions. Conversely, authorization alone does not hide a capability's metadata from discovery. Use static filtering to remove a capability from the deployment surface and authorization to decide whether an enabled native call may dispatch.

## Client errors are intentionally coarse

The root API exposes stable sentinel classifications. The MCP adapter projects failures to short text such as `permission denied`, `capability failed`, `resource limit exceeded`, and `internal failure`.

This projection prevents trusted diagnostic detail from becoming model-visible. In particular, the MCP response does not forward:

- resolver and authentication diagnostics
- policy reasons
- handler error text
- panic values or stack details
- credentials
- source or argument values copied into wrapped errors

The host can log trusted details on its side of the boundary if its authorizer, resolver, and handlers implement that logging. CodeMode's client response remains coarse. Unknown service errors and recovered adapter panics become `internal failure`.

## Execution state does not cross calls

Every `Server.Execute` call creates a fresh bounded Starlark interpreter. Module loading is disabled, and the only predeclared application functions are the enabled capability namespace. Native calls are rejected while top-level source is loading and are accepted only while the required zero-argument `main()` function runs.

After `main` returns, CodeMode converts only its final value to JSON-shaped data under the depth and encoded-size limits. Printed text is discarded. Globals, source-loading values, intermediate expressions, and unrelated native results do not cross the boundary. No Starlark globals or mutable interpreter state carry into the next execute call.

These rules narrow the model-visible result and stop one request from intentionally using interpreter globals as storage for another. They do not make registered Go code untrusted or confined: a capability handler still runs inside the host process with the privileges the host gave that process.

## Cancellation and host code

CodeMode derives an elapsed deadline from `MaxExecutionTime` and the request context. It watches that context and cancels Starlark evaluation when the request is canceled or the elapsed budget expires. The interpreter also enforces source, bytecode-step, attempted-native-call, conversion-depth, and encoded-result limits.

Cancellation is cooperative once execution enters Go code. CodeMode calls authorizers and handlers synchronously and cannot forcibly interrupt them. A blocking authorizer or handler can therefore keep the `Execute` call blocked after the Starlark deadline or request cancellation.

Authorizers and handlers must:

- honor the supplied context for I/O, locks, waits, and downstream calls
- return promptly after cancellation
- bound their own memory, network, storage, and retry behavior
- avoid exposing credentials or trusted diagnostics in returned values or client-visible errors
- be safe for concurrent calls when the immutable server is shared

CodeMode recovers panics at selected boundaries and returns a coarse classification. Panic recovery does not replace normal error handling, cancellation, or resource control in host code.

## In-process limits are not tenant isolation

CodeMode's limits bound specific interpreter operations and final-value conversion. They do not establish a hard heap limit, a process boundary, or a security boundary between mutually untrusted tenants. The interpreter, authorizer, and handlers share the host process and its memory, CPU, file descriptors, credentials, and operating-system privileges.

A host that needs hard tenant or heap containment must supply it outside CodeMode, for example with separate processes or containers and operating-system resource controls. CodeMode's in-process budgets remain useful inside that boundary, but they are not a substitute for it.

See [Public API reference](../reference/public-api.md) for the Go contracts and [MCP tool reference](../reference/mcp-tools.md) for the exact client-visible surface.
