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
