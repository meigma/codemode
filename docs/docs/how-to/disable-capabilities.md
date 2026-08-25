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

The explicit `authz.AllowAll()` remains deliberate only for the simple tutorial. Use `authz.AllowAll()` only when every resolved subject may call every enabled capability; otherwise supply an `authz.Authorizer`. Static filtering and per-call authorization solve different problems: filtering fixes the deployment-wide surface, while the authorizer decides whether a trusted subject may make an enabled native call.

## Replace the built server

After registering all capabilities, call `Build`. Pass the returned server to `mcpserver.New`. To deploy a different enabled set, replace the existing server instance and its host-owned transport lifecycle.

## Verify the filtered surface

The tutorial's `callTool` helper returns on the first tool error, so `describe_api` would stop the program before `execute`. Replace the `result.IsError` branch in `callTool` with:

```go
if result.IsError {
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return fmt.Errorf("%s returned a tool error: %v", params.Name, result.Content)
	}
	fmt.Printf("%s: %s\n", params.Name, text.Text)
	return nil
}
```

Run `go run .`. The program prints:

```
search_api: []
describe_api: capability not found
execute: invalid program
```

See the [public API reference](../reference/public-api.md#static-capability-filtering) for the complete filter contract, and the [MCP tools reference](../reference/mcp-tools.md) for discovery and execution results.
