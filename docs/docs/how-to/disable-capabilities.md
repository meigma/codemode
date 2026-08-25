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

The following block retains the tutorial's `authz.AllowAll()` only to isolate
the filtering change. In an existing deployment, keep its current
`authz.Authorizer`.

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

Static filtering fixes the deployment-wide surface; the authorizer
decides whether a trusted subject may make an enabled native call.

## Replace the server

1. Build the server:

```sh
go build -o codemode-first-server .
```

2. Reload the binary in the agent.

## Verify the filter

1. Call `search_api` with `{"query":"records.lookup"}`. The result is `[]` and no longer lists `records.lookup`.

2. Call `describe_api` with `{"name":"records.lookup"}`. The call returns the tool error `capability not found`.

3. Call `execute` with this zero-argument program as the `source` argument:

```python
def main():
    return records.lookup(key="alpha", limit=2)
```

The call returns the tool error `invalid program`.

See the [public API reference](../reference/public-api.md#static-capability-filtering) for the complete filter contract, and the [MCP tools reference](../reference/mcp-tools.md) for discovery and execution results.
