---
title: MCP tool reference
description: Exact inputs, successful structured outputs, discovery behavior, and errors for the three CodeMode MCP tools.
---

# MCP tool reference

`mcpserver.New` registers exactly three tools on an official MCP Go SDK server:

- `search_api`
- `describe_api`
- `execute`

Each input is an object with one required string property. Additional properties are rejected by the SDK before subject resolution or service work. Every valid call then resolves a trusted subject through `mcpserver.InvocationResolver` before it reaches the CodeMode service.

On success, the official SDK returns the documented value in `CallToolResult.StructuredContent` and one JSON `TextContent` item that mirrors it. The schemas below describe the structured value, not the surrounding MCP result.

## `search_api`

Search the names and summaries of enabled capabilities.

### Input

```json
{
  "type": "object",
  "required": ["query"],
  "additionalProperties": false,
  "properties": {
    "query": {
      "type": "string"
    }
  }
}
```

The raw query is limited by `MaxSearchQueryBytes` before trimming or case normalization. Whitespace padding counts. CodeMode then trims surrounding whitespace and normalizes case. Matching is a substring search over capability names and summaries. A blank normalized query returns an empty array.

Results are sorted by exact dotted name and limited by `MaxSearchResults`. Static filtering happens before search, so disabled capabilities never appear.

### Successful structured output

```json
{
  "type": "array",
  "items": {
    "type": "object",
    "required": ["name", "signature", "summary"],
    "additionalProperties": false,
    "properties": {
      "name": { "type": "string" },
      "signature": { "type": "string" },
      "summary": { "type": "string" }
    }
  }
}
```

The structured content itself is the array described above, not an object that wraps the array. When there are no matches, it is `[]`, not `null`.

| Field | Meaning |
| --- | --- |
| `name` | Exact enabled dotted capability name, such as `records.lookup`. |
| `signature` | Keyword-only Starlark signature generated from the registered Go binding. |
| `summary` | Registered compact summary. |

## `describe_api`

Describe one enabled capability by the exact dotted `name` returned by `search_api`.

### Input

```json
{
  "type": "object",
  "required": ["name"],
  "additionalProperties": false,
  "properties": {
    "name": {
      "type": "string"
    }
  }
}
```

Name lookup is exact. It neither trims nor case-folds, and it does not perform search, prefix expansion, or fuzzy matching. Clients must pass the exact `name` returned by `search_api`. An unknown or disabled name returns `capability not found`.

For the site-wide sample, the requested name is `records.lookup`. Its stable ID, `records.entry.lookup`, is intentionally not part of this tool input or output.

### Successful structured output

```json
{
  "type": "object",
  "required": [
    "name",
    "signature",
    "summary",
    "description",
    "input",
    "output"
  ],
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "signature": { "type": "string" },
    "summary": { "type": "string" },
    "description": { "type": "string" },
    "input": {
      "type": "array",
      "items": { "$ref": "#/$defs/fieldShape" }
    },
    "output": {
      "type": "array",
      "items": { "$ref": "#/$defs/fieldShape" }
    }
  },
  "$defs": {
    "fieldShape": {
      "type": "object",
      "required": ["name", "type", "required"],
      "additionalProperties": false,
      "properties": {
        "name": { "type": "string" },
        "type": { "type": "string" },
        "required": { "type": "boolean" }
      }
    }
  }
}
```

`input` and `output` preserve Go field declaration order. Field-shape types have these values:

| Position | Go field type | `type` value | `required` |
| --- | --- | --- | --- |
| Input | `string` | `str` | `true` |
| Input | `*int64` | `int | None` | `false` |
| Output | `string` | `str` | `true` |
| Output | `int64` | `int` | `true` |
| Output | `bool` | `bool` | `true` |
| Output | `float64` | `float` | `true` |

The sample capability therefore describes input fields `key` (`str`, required) and `limit` (`int | None`, optional), and output fields `key` (`str`, required) and `count` (`int`, required).

## `execute`

Execute one bounded Starlark program against the enabled capability namespace.

### Input

```json
{
  "type": "object",
  "required": ["source"],
  "additionalProperties": false,
  "properties": {
    "source": {
      "type": "string"
    }
  }
}
```

The source must define `main` as a function with no parameters. Source loading cannot call native capabilities; calls are accepted only while `main` runs. Module loading is disabled.

Capabilities are available by dotted name. The sample native call is `records.lookup(key="alpha", limit=2)`. Native calls accept keyword arguments only. Duplicate keyword syntax is rejected by the Starlark parser as `invalid program` before authorization or handler dispatch. Positional, unknown, missing, incorrectly typed, and out-of-range arguments reach binding and map to `invalid capability arguments`. For the sample, `key` is required and `limit` can be omitted, `None`, or an integer in the signed 64-bit range.

Each `execute` call gets a fresh interpreter and fresh source, step, elapsed-time, native-call, conversion-depth, and result-size budgets. There is no interpreter state shared between calls.

### Successful structured output

```json
{
  "type": "object",
  "required": ["result"],
  "additionalProperties": false,
  "properties": {
    "result": {}
  }
}
```

The `result` property is the final converted return value from `main`. Its runtime value is one of:

- `null`
- a boolean
- a string
- a signed 64-bit integer
- a finite floating-point number
- an array containing supported values
- an object with string keys and supported values

`MaxValueDepth` is inclusive. A scalar or `None` is depth 1. Each tuple, list, or dictionary wrapper adds one. A scalar with limit 1 succeeds, a one-level container with limit 2 succeeds, and one more wrapper with limit 2 fails. Nested values are subject to that limit, and the encoded `result` value is subject to `MaxResultBytes`. Starlark tuples and lists become arrays. `None` becomes `null`. Dictionaries must have string keys.

Only the final converted value crosses the execution boundary. `print` output is discarded. Globals, source-loading values, intermediate expressions, and native results that are not included in the final return value are not added to structured output. The successful envelope contains only `result`.

## Errors

After a well-formed call reaches the adapter, a resolver or service failure becomes a successful MCP protocol response with `isError` set and one coarse text item. The adapter does not expose wrapped policy diagnostics, handler errors, panic values, stack details, credentials, source, or arguments.

| Text | Meaning |
| --- | --- |
| `unauthenticated` | The resolver failed or returned an empty subject ID. |
| `capability not found` | `describe_api` did not find an enabled exact name. |
| `invalid program` | Source, including duplicate keyword syntax, entry point, runtime behavior, or final-value conversion was invalid. |
| `invalid capability arguments` | A native call failed binding: positional, unknown, missing, incorrectly typed, or out-of-range arguments. |
| `permission denied` | Policy returned a recognized denial. |
| `authorization policy failure` | Policy evaluation failed. |
| `resource limit exceeded` | A discovery, execution, or conversion budget was exceeded. |
| `capability failed` | A handler failed or returned an invalid output. |
| `context canceled` | The request context was canceled. |
| `context deadline exceeded` | A service returned a bare deadline error. Root CodeMode execution deadlines are normally projected as `resource limit exceeded`. |
| `internal failure` | Any unknown service error or recovered adapter failure. |

Malformed MCP arguments are rejected by the SDK's input-schema validation and do not call the invocation resolver. For well-formed inputs, resolver failure stops the request before search, description, or execution.

See [Public API reference](public-api.md) for Go contracts and [Security model](../explanation/security-model.md) for the resolver and execution trust boundaries.
