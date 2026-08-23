---
id: 002
title: New work session
started: 2026-08-23
---

## 2026-08-23 08:23 — Kickoff
Goal for the session: Start a new journal session; the substantive task has not yet been provided.
Current state of the world: Session 001 settled the CodeMode architecture and spike-first implementation plan. No implementation work has begun.
Plan: Await the user's request, then record meaningful checkpoints as work proceeds.

## 2026-08-23 08:31 — Increment 0 started
Goal: Execute Increment 0 from session 001 as a disposable product spike outside the repository.
Constraints: Use the official MCP Go client and transport, expose exactly `search_api`, `describe_api`, and `execute`, call one manually bound namespaced Starlark function from zero-argument `main()`, return only the final value, and make no production security claim.
Verification plan: Exercise the loop through the official client, probe context/error/result and Starlark runtime semantics, run the executable and focused tests once, record durable findings here, then delete every temporary spike file. No implementation PR is required.

## 2026-08-23 08:52 — Increment 0 findings
Built the disposable product loop at `/tmp/codemode-increment0-spike` with the official MCP Go SDK's in-memory transport. The executable registered exactly `search_api`, `describe_api`, and `execute`; the official client listed and called all three. `execute` loaded top-level Starlark, separately called zero-argument `main()`, manually bound `records.lookup`, discarded prints and intermediates, and returned only `{"result": <main value>}`.

Dependency selection:
- Official MCP Go SDK: `github.com/modelcontextprotocol/go-sdk v1.7.0`, commit `bc72835f62eb94d0fb484439f886b6885b075f36`, latest stable reviewed release. It supports protocol `2026-07-28` and requires Go 1.25 or later.
- Starlark-Go: `go.starlark.net v0.0.0-20260708150628-5395d018f003`, commit `5395d018f003e2a08bfbca6dcb2562acee700f62`, latest proxy revision reviewed against the current APIs. It requires Go 1.25 or later.
- These are the revisions to pin during the production-module cutover; Increment 0 intentionally changed no repository files.

Observed MCP mechanics:
- Construction that compiles: `mcp.NewServer`, generic `mcp.AddTool`, `mcp.NewClient`, `mcp.NewInMemoryTransports`, server `Connect` first, client `Connect` second, then `ListTools` and `CallTool`.
- The in-memory transport still crosses a newline-delimited JSON boundary. Generic `AddTool` populates `CallToolResult.StructuredContent` as generic decoded JSON on the client and duplicates that value as JSON `TextContent` in `Content`. SDK v1.7.0 documentation still has stale `StructuredOutput` wording in places; the actual Go field is `StructuredContent`.
- Handler contexts derive from the server's `Connect` context, so its values reach handlers. A value added only to the client `CallTool` context does not cross the transport. In-memory `RequestExtra` is nil. The in-memory connection preserves `Connect` context values but suppresses cancellation/deadline propagation from that root; individual client request cancellation uses MCP cancellation instead.
- A typed tool handler's ordinary error and typed input-validation errors return `CallToolResult{IsError:true}` with text content and a nil client error. An unknown tool is a protocol-level `*jsonrpc.Error` with `InvalidParams` (`-32602`) and no result. Low-level `ToolHandler` errors are protocol errors; the production adapter should prefer typed handlers.

Observed Starlark mechanics:
- Use `starlark.ExecFileOptions` for top-level loading, then retrieve and validate `globals["main"]` before `starlark.Call(thread, main, nil, nil)`. Missing `main` must be guarded because calling a nil value panics. Validate zero positional/keyword-only parameters and no variadic parameters.
- `ExecFileOptions` freezes returned globals. `main()` cannot mutate a list created at module top level.
- A `starlarkstruct.Module` provides the `records.lookup` namespace. Go-only trusted values fit in `Thread.SetLocal` before execution and are unavailable to Starlark source except through the builtin.
- `Thread.SetMaxExecutionSteps` counts bytecode operations cumulatively across loading and `main`; zero means unlimited on first execution. The default limit path calls `Cancel("too many steps")`. Cancellation is observed between Starlark operations, not while blocked inside a Go builtin, persists until `Uncancel`, and returns an `EvalError`.
- Starlark-Go has no official general `starlark.Value` to Go/JSON converter. The spike used a deliberately local converter for supported JSON-like scalars, lists/tuples, and string-keyed dictionaries.

Verification completed once from the disposable module:
- `go test ./...` passed: `ok codemode-increment0-spike`.
- `go run .` printed the exact tool list and successful structured results. The execute line was `execute={"result":{"context_marker":"server-connect","id":"alpha","name":"Alpha Record","status":"active"}}`, proving final-result-only projection and server-context propagation.
