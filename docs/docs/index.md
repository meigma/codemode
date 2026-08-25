---
title: CodeMode
slug: /
description: Reference for bounded Starlark access to registered Go capabilities over MCP.
---

# CodeMode

CodeMode is a Go library that exposes registered, typed Go capabilities to Model Context Protocol (MCP) clients through Starlark programs. The current implementation consists of an immutable capability server, a fresh-process Starlark worker, an authorization interface, an optional in-process Rego authorizer, and an adapter for the official MCP Go SDK.

## Public packages

CodeMode has four public packages:

- `github.com/meigma/codemode` registers typed capabilities, filters disabled capabilities, searches and describes the enabled catalog, and executes bounded Starlark programs.
- `github.com/meigma/codemode/authz` defines the trusted subject, canonical authorization input, authorizer interface, and recognized denial.
- `github.com/meigma/codemode/authz/rego` prepares a static in-memory Rego decision that implements the authorizer interface.
- `github.com/meigma/codemode/mcpserver` binds a CodeMode service and trusted invocation resolver to an official MCP SDK server.

See [Use Rego for authorization](how-to/use-rego-authorization.md) to replace `authz.AllowAll()`. The [public API reference](reference/public-api.md#authzrego) defines the exact Go and policy contracts, and the [security model](explanation/security-model.md#rego-policy-runs-in-process) explains the Rego host-process boundary.

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

The host owns transport startup, authentication, listeners, request context, and shutdown. CodeMode has no generic downstream MCP forwarding path. The final host binary must install `codemode.ServeWorkerAndExit()` as the first statement of `main`, and test binaries that build a server must install it before `m.Run` in `TestMain`.

## Runtime limits

`codemode.DefaultLimits()` currently sets:

| Limit | Default |
| --- | ---: |
| Starlark source | 64 KiB |
| Interpreter steps | 1,000,000 |
| Elapsed execution time | 5 seconds |
| Attempted native calls | 100 |
| Crossing-value depth | 32 |
| Encoded crossing value | 1 MiB |
| Search query | 256 bytes |
| Search results | 20 |
| Concurrent execution workers | 8 |

Every configured limit must be positive; zero does not mean unlimited. Each
execution gets a fresh interpreter in a re-executed worker process and its own
budgets. The elapsed budget includes semaphore waiting, process startup,
protocol exchange, Starlark work, parent dispatch, and worker cleanup. Module
loading is disabled, and only the frozen namespace built from enabled
capabilities is predeclared.

The parent can kill and reap Starlark workers when a deadline or request
cancellation occurs. This is not a complete tenant boundary: workers have no
CodeMode-provided operating-system CPU or memory quota, and authorizers and
handlers run as Go code with the host process's privileges. Parent Go code must
honor context cancellation and return promptly because CodeMode cannot forcibly
stop it after dispatch.
