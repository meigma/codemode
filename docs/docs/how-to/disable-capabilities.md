---
title: Disable capabilities for a deployment
description: Remove registered capabilities from every live CodeMode surface by stable capability ID.
---

# Disable capabilities for a deployment

Use `codemode.Options.DisabledCapabilities` to remove capabilities from one server build. The filter is static: construct and build another server to change the enabled set.

## Add stable capability IDs to the filter

Starting from the [first-server tutorial](../tutorials/first-server.md), keep the capability registered and add its stable `codemode.CapabilityID` to the builder options. In the tutorial's `run` function, replace the builder initialization with:

```go
builder := codemode.New(codemode.Options{
	Authorizer: authz.AllowAll(),
	DisabledCapabilities: []codemode.CapabilityID{
		"records.entry.lookup",
	},
	Limits: codemode.DefaultLimits(),
})
```

Use the stable ID, `records.entry.lookup`, not the model-facing dotted name, `records.lookup`. IDs are the deployment and authorization identity. Dotted names define discovery and Starlark namespace access.

Continue to call `codemode.Register` for `records.entry.lookup` before `builder.Build()`. CodeMode validates the complete registration set before applying the filter, and `Build` rejects a disabled ID that does not identify a registered capability. Static filtering cannot hide invalid or conflicting registrations.

The explicit `authz.AllowAll()` remains deliberate only for the simple tutorial. Production hosts normally supply an authorization policy. Static filtering and per-call authorization solve different problems: filtering fixes the deployment-wide surface, while the authorizer decides whether a trusted subject may make an enabled native call.

## Replace the built server

Call `Build` after registering all capabilities, then pass the resulting server to `mcpserver.New` as usual. An existing immutable server does not change when the builder options change. Replace the server instance and its host-owned transport lifecycle to deploy a different enabled set.

## Check the resulting surface

With the tutorial's unchanged call order, `search_api` prints `[]`, then `describe_api` returns the expected tool error and the helper exits. The later `execute` call is not reached in that run. The list below describes the complete contract of the built server, not the output of one sequential client run.

After `records.entry.lookup` is disabled:

- `search_api` does not return `records.lookup`, even when the query matches its name or summary.
- `describe_api` for `records.lookup` returns the same not-found classification used for any unavailable name.
- The generated Starlark namespace has no `records.lookup` callable. If no other enabled capability starts with `records.`, the `records` namespace is absent; otherwise only `lookup` is absent.
- An `execute` program cannot call the disabled capability. The request fails before authorization or handler dispatch.

These effects come from one filtered catalog used by discovery and execution. The filter does not leave a hidden execution path.

See the [public API reference](../reference/public-api.md) for `Options` and build validation, and the [MCP tools reference](../reference/mcp-tools.md) for discovery and execution results.
