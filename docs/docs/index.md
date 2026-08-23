---
title: CodeMode
slug: /
description: Reference for bounded Starlark access to registered Go capabilities over MCP.
---

# CodeMode

CodeMode is a Go library that exposes registered, typed Go capabilities to Model Context Protocol (MCP) clients through Starlark programs. The current implementation consists of an immutable capability server, an authorization interface, and an adapter for the official MCP Go SDK.

## Public packages

- `github.com/meigma/codemode` registers typed capabilities, filters disabled capabilities, searches and describes the enabled catalog, and executes bounded Starlark programs.
- `github.com/meigma/codemode/authz` defines the trusted subject, canonical authorization input, authorizer interface, and recognized denial.
- `github.com/meigma/codemode/mcpserver` binds a CodeMode service and trusted invocation resolver to an official MCP SDK server.

## MCP surface

`mcpserver.New` exposes exactly three tools:

| Tool | Required input | Successful structured output |
| --- | --- | --- |
| `search_api` | `{"query": string}` | An array of `{name, signature, summary}` records for matching enabled capabilities |
| `describe_api` | `{"name": string}` with one exact capability name | `{name, signature, summary, description, input, output}` |
| `execute` | `{"source": string}` with one Starlark program | `{"result": <main return value>}` |

Search is case-normalized over capability names and summaries, returns name-sorted results, and returns an empty array for a blank query. Description uses exact names. A disabled capability is absent from search, description, the Starlark namespace, and execution.

Each program must define a zero-argument `main()` function. CodeMode rejects native capability calls made while top-level source is loading. It discards printed text and returns only the converted value from `main` under the `result` key.

## Trust model

The host authenticates the request and stores a non-secret `authz.Subject` in typed, host-owned Go context. An `mcpserver.InvocationResolver` reads that context for every tool call. Resolver failure or an empty subject stops the request before discovery or execution. Tool arguments and MCP `_meta` are untrusted and cannot replace the resolved subject or provide credentials, budgets, modules, or capability filters.

For a native capability call, CodeMode first binds the Starlark arguments to the registered Go input type. It then calls the host's `authz.Authorizer` with:

- the resolved subject
- the stable capability ID
- the model-facing capability name
- a fresh JSON-shaped copy of the validated arguments

A recognized denial prevents handler dispatch. Client-facing MCP failures use coarse error classifications rather than forwarding source, arguments, credentials, policy diagnostics, stack traces, or handler error text.

The host owns transport startup, authentication, listeners, request context, and shutdown. CodeMode has no executable entry point and no generic downstream MCP forwarding path.

## Runtime limits

`codemode.DefaultLimits()` currently sets:

| Limit | Default |
| --- | ---: |
| Starlark source | 64 KiB |
| Interpreter steps | 1,000,000 |
| Elapsed execution time | 5 seconds |
| Attempted native calls | 100 |
| Converted-value depth | 32 |
| Encoded final result | 1 MiB |
| Search query | 256 bytes |
| Search results | 20 |

Every configured limit must be positive; zero does not mean unlimited. Each execution gets a fresh interpreter and its own budgets. Module loading is disabled, and only the frozen namespace built from enabled capabilities is predeclared.

These are in-process restrictions, not a hard isolation boundary between mutually untrusted tenants. Authorizers and handlers run as Go code with the host process's privileges. They must honor context cancellation and return promptly because CodeMode cannot forcibly interrupt blocking Go code.
