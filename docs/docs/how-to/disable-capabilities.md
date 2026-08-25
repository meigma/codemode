---
title: Disable capabilities for a deployment
description: Remove registered capabilities from every live CodeMode surface by stable capability ID.
---

# Disable capabilities for a deployment

Use `codemode.Options.DisabledCapabilities` to remove capabilities from one server build. The filter is static: construct and build another server to change the enabled set.

## Set a stable capability ID

Starting from the [first-server tutorial](../tutorials/first-server.md), set an
explicit ID before creating a deployment filter:

```go
codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
	ID:      "records.entry.lookup",
	Name:    "records.lookup",
	Summary: "Look up one record by key.",
	Handler: lookup,
})
```

An omitted ID defaults to the capability name. Set the ID explicitly before
writing policy or filters so renaming `records.lookup` does not silently change
its deployment identity.

## Add the ID to the filter

Replace the builder initialization:

```go
builder := codemode.New(codemode.Options{
	Authorizer: authz.AllowAll(),
	DisabledCapabilities: []codemode.CapabilityID{
		"records.entry.lookup",
	},
})
```

Use the stable ID, `records.entry.lookup`, not the model-facing dotted name,
`records.lookup`. IDs are the deployment and authorization identity. Dotted
names define discovery and Starlark namespace access.

Keep the capability registered before `builder.Build()`. CodeMode validates the
complete registration set before applying the filter, and `Build` rejects a
disabled ID that does not identify a registered capability. Static filtering
cannot hide invalid or conflicting registrations.

The explicit `authz.AllowAll()` remains deliberate only for the simple
tutorial. Static filtering fixes the deployment-wide surface; the authorizer
decides whether a trusted subject may make an enabled native call.

## Replace and verify the server

Build and replace the existing server process, then reload the server in your
agent. `search_api` no longer returns `records.lookup`, `describe_api` reports
`capability not found`, and an `execute` program that references
`records.lookup` reports `invalid program`.

See the [public API reference](../reference/public-api.md#static-capability-filtering) for the complete filter contract, and the [MCP tools reference](../reference/mcp-tools.md) for discovery and execution results.
