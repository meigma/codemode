---
title: CodeMode
slug: /
description: Build MCP servers that expose bounded Go capabilities through Starlark.
---

# CodeMode

CodeMode is a Go library for building code-native Model Context Protocol (MCP) servers. An application registers typed Go capabilities, selects the capabilities enabled for one deployment, and exposes them through a restricted Starlark runtime.

## MCP boundary

The `mcpserver` package exposes exactly three tools:

| Tool | Input | Output |
| --- | --- | --- |
| `search_api` | A bounded query string | Enabled capability names, signatures, and summaries |
| `describe_api` | One exact capability name | Its signature, description, and supported input and output fields |
| `execute` | One bounded Starlark program | `{"result": <main return value>}` |

Each tool call resolves an authenticated subject from trusted Go context before it reaches the CodeMode service. Tool arguments and MCP `_meta` cannot provide or replace identity, credentials, execution budgets, modules, or capability allow-lists.

`execute` creates a fresh interpreter for each call. The program must define a zero-argument `main()` function. Only the value returned by `main()` crosses the MCP boundary; printed text and intermediate values do not.

## Integration outline

1. Build an immutable `codemode.Server` from typed capabilities and deployment options.
2. Implement `authz.Authorizer` for each native capability call.
3. Implement `mcpserver.InvocationResolver` to read the authenticated subject from host-owned typed context.
4. Call `mcpserver.New` and connect the returned official MCP SDK server to the transport owned by the host application.

Disabled capabilities are absent from search, description, and execution. Authorization denial stops the native handler from running. Public tool errors use coarse classifications and do not include source text, arguments, credentials, policy details, stack traces, or handler results.
