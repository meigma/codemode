---
id: 010
title: Reduce first-touch API ceremony
started: 2026-08-25
---

## 2026-08-25 10:55 — Kickoff
Goal for the session: Review GitHub issue #25 and reduce first-touch API ceremony through stock subject resolvers, per-field default limits, deferred registration errors, and capability IDs defaulting to names.
Current state of the world: `master` includes the fresh-process worker cutover through PR #31 and the documentation correction in PR #32; issue #25 was re-verified against that worker architecture and remains open.
Plan: Inspect the live issue and current APIs, implement the four clean-cutover changes with focused tests and documentation updates, then exercise the target stdio server end to end.

## 2026-08-25 11:14 — Implemented and verified
Implemented issue #25 on `feat/issue-25-first-touch` in commit `7565d1c`. Added `mcpserver.StaticSubject` and `ContextSubject`, plus `authz.WithSubject` and `SubjectFromContext`; zero limit fields now resolve independently from `DefaultLimits`; `Register` defers capability failures for joined `Build` diagnostics; and an omitted capability ID derives from its name. `Register` panics on nil or closed builders because no later build can report those lifecycle violations.

The issue's exact target omitted `Description` even though registration previously required it. Defaulted an empty description to `Summary` so the stated server is actually runnable, and documented that behavior.

Replaced the first-server tutorial with a real stdio host, updated README, reference, security, how-to, and both compile-checked examples, and left `mcp-tools.md` unchanged. A disposable external client launched the exact target binary over `mcp.CommandTransport`; `search_api`, `describe_api`, and `execute` succeeded, with execute returning `{"result":{"count":2,"key":"alpha"}}`.

Focused root, authz, and mcpserver tests passed. `moon run root:check` passed format, lint, build, MCP smoke, strict docs build, and the full race suite. The pre-existing race deadline test used a 20 ms budget that expired during race-instrumented worker startup before reaching its expected authorizer; increased that test-only budget to two seconds and confirmed the targeted race case before the full gate.

Next: push the implementation branch, open the issue-closing PR, and monitor its required checks.
