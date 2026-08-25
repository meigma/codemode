---
title: CodeMode
slug: /
description: Find the tutorial, how-to guides, reference, and explanation for CodeMode.
---

# CodeMode

CodeMode is a Go library that exposes registered, typed Go capabilities to Model Context Protocol (MCP) clients through Starlark programs. Clients use three MCP tools: `search_api`, `describe_api`, and `execute`. Each `execute` call runs Starlark in a fresh worker process, and only the final converted value returned by zero-argument `main()` is exposed to the caller. The pages below are organized by reader need.

## Tutorials

Learn the workflow by building a working server.

- [Build your first CodeMode server](tutorials/first-server.md) — register one typed capability and call `search_api`, `describe_api`, and `execute` over the official MCP in-memory transport.

## How-to guides

Complete a specific host-configuration task.

- [Use Rego for authorization](how-to/use-rego-authorization.md) — replace `authz.AllowAll()` with a prepared in-process Rego decision.
- [Disable capabilities for a deployment](how-to/disable-capabilities.md) — remove registered capabilities from every live surface by stable capability ID.

## Reference

Look up exact contracts, defaults, and wire shapes.

- [Public API reference](reference/public-api.md) — exported Go types, builder lifecycle, limits, and authorization contracts.
- [MCP tool reference](reference/mcp-tools.md) — inputs, listed descriptions, successful outputs, and errors for the three MCP tools.

## Explanation

Understand why the trust boundaries exist and what they do not cover.

- [Understanding CodeMode's security model](explanation/security-model.md) — identity, authorization order, worker containment, cancellation, and tenant-isolation limits.
