---
id: 001
title: Bootstrap codemode repository
started: 2026-08-22
---

## 2026-08-22 18:27 — Kickoff
Goal for the session: Create the public `meigma/codemode` repository from `meigma/template-go`, initialize the session journal, and start the first session.
Current state of the world: The repository exists publicly and is cloned on `master`; session setup created and pushed `journal/jmgilman` in `.wt/journal-jmgilman`.
Plan: Use this session for the initial codemode work and record meaningful checkpoints as it develops.

## 2026-08-22 18:56 — Reviewed reference article
Reviewed Ralf Schmid's “Code Mode for Tool Calling in Go” and its linked implementation. The core pattern is MCP catalog discovery through a single search tool, followed by model-generated synchronous JavaScript that orchestrates selected helpers inside QuickJS and returns only the final value.

The prototype clearly demonstrates reduced prompt surface and fewer LLM round trips for dependent tool workflows. Production concerns identified: the example forces an initial search even for no-tool requests, exposes the full catalog inside QuickJS rather than only discovered helpers, relies on domain-specific lexical search, executes helper calls synchronously, and needs capability, call-count, output, and whole-run budgets beyond VM memory and evaluation limits.

## 2026-08-22 19:06 — Evaluated Python execution
Confirmed that Go can embed the workstation's CPython 3.13.3 through cgo: a temporary Go probe initialized CPython, evaluated generated Python, and printed the expected result `30`.

The feasible runtime shapes are in-process CPython through cgo, CPython in a child process, CPython compiled to WASI and hosted by wazero, or a Python-like/pure-Go interpreter such as Starlark or gpython. In-process CPython provides full compatibility but is not a security boundary and complicates distribution, GIL handling, and interpreter lifecycle. A subprocess is operationally simpler to isolate but requires an RPC bridge. WASI offers a stronger embedded boundary but needs substantially more integration work. gpython is only a partial Python 3.4 implementation; Starlark is intentionally not Python.

## 2026-08-22 19:12 — Reviewed FastMCP Code Mode
FastMCP added experimental Code Mode in 3.1.0. It does have the LLM write async Python using only an injected `call_tool(name, params)` function, but it does not execute that code in CPython. The default `MontySandboxProvider` uses Pydantic Monty, a deliberately incomplete Rust Python interpreter built for agent-generated code.

FastMCP defaults to a 30-second wall-clock limit, 100 MB memory ceiling, and 50 tool calls per execution; discovery and schemas remain request/auth scoped. FastMCP 3.4.7 pins Monty 0.0.17, which is patched after Monty's first public sandbox escape but still executes in a native worker thread. Current Monty development has moved Python bindings to worker subprocesses for crash isolation. Both FastMCP Code Mode and Monty remain explicitly experimental.

## 2026-08-22 19:20 — Selected Starlark starting point
Decision: use Starlark-Go as the initial code runtime. The Go host will create a fresh Starlark thread for each execution and inject only explicit Go-backed builtins. The simplest vertical slice should expose one generic `call_tool(name, params)` builtin; per-tool functions or a `tools` namespace can follow only if model ergonomics justify them.

The LLM cannot learn those runtime bindings through introspection before generating code. The MCP-facing surface should therefore expose discovery/schema meta-tools plus `execute`: discovery returns selected tool metadata rendered as Starlark-oriented signatures and examples, while the execute description defines the supported Starlark dialect, result convention, and `call_tool` contract. Runtime authorization remains independent of discovery and is enforced by the Go host.

## 2026-08-22 19:29 — Corrected tool binding direction
Correction to the preceding checkpoint: a generic `call_tool(name, params)` reproduces MCP's RPC envelope inside Starlark and discards Code Mode's central advantage. Cloudflare's design converts MCP schemas into a native language API because models are better at ordinary function calls than synthetic tool-call envelopes.

The Starlark runtime should therefore expose one Go-backed native function per selected MCP tool, preferably namespaced by server, for example `github.search_code(query="...", page=1)`. Discovery should return generated Starlark API reference from the same binding descriptors used at runtime. The spike may use a tiny hard-coded catalog, but it should prove native bindings rather than a generic dispatcher.

## 2026-08-22 19:34 — Clarified the MCP boundary
Correction: CodeMode itself should be the MCP server. MCP is needed only for the small model-facing meta-tool surface such as API discovery and execution. Functions visible inside Starlark are native CodeMode capabilities backed directly by Go handlers, not MCP tools and not internal MCP round trips.

The internal catalog should pair schemas and documentation with Go functions. The same descriptors generate model-facing Starlark API reference and runtime builtins; builtin callbacks invoke the registered Go handlers directly. Handlers may call real HTTP APIs, SDKs, databases, or other application services. An adapter for importing an external MCP server could exist later for interoperability, but it is not part of the core architecture and is generally inferior to a native API adapter when one is available.

## 2026-08-22 19:51 — Locked product scope
CodeMode is strictly a framework for authoring new code-native MCP servers in Go. It will not import, proxy, translate, or otherwise provide compatibility for existing MCP servers. Developers register native Go capabilities backed by existing APIs, SDKs, databases, and services.

The product addresses four traditional MCP problems: progressive discovery reduces context usage; Go handlers own authentication without exposing credentials to Starlark; Starlark composes multiple capability calls without model round trips; and native Go adapters speak established service protocols directly without a downstream MCP middleman. The only MCP surface is the small client-facing set of discovery and execution meta-tools.

## 2026-08-22 19:59 — Defined authorization seam
Authorization should be enforced at every native Starlark builtin invocation, after arguments are decoded and normalized but before the Go handler runs. No Starlark source translation or AST policy analysis is required: each builtin closure already knows the trusted capability identity and can combine it with trusted request identity plus untrusted canonical arguments to form a policy input.

Use two layers initially. A deployment-level capability filter removes disabled functions from discovery, API documentation, and runtime bindings. A runtime `Authorizer` evaluates argument-aware invocation records and defaults to deny on policy failure. Keep the core engine-agnostic; an embedded OPA/Rego adapter can compile a prepared query at startup. Credentials and subject identity come only from trusted Go context, and policy denials occur before any external side effect.
