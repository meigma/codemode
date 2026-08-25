---
title: MCP tool reference
description: Exact inputs, listed descriptions, successful structured outputs, discovery behavior, and errors for the three CodeMode MCP tools.
---

# MCP tool reference

`mcpserver.New` registers exactly three tools on an official MCP Go SDK server:

- `search_api`
- `describe_api`
- `execute`

Each input is an object with one required string property. Additional properties are rejected by the SDK before subject resolution or service work. Every valid call then resolves a trusted subject through `mcpserver.InvocationResolver` before it reaches the CodeMode service.

On success, `CallToolResult.StructuredContent` contains the value described by the tool's `outputSchema`, and one JSON `TextContent` item mirrors the same value. The schemas below are the `outputSchema` values advertised by `tools/list`; they describe the successful value itself, not the surrounding MCP result. Each tool's listed `tools/list` description is the model-facing authoring contract for that tool.

## `search_api`

Search enabled names and summaries with a short literal substring. Retry an empty result with a shorter term.

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

The raw query is limited by `MaxSearchQueryBytes` before trimming or case normalization. Whitespace padding counts. CodeMode then trims surrounding whitespace and normalizes case. Matching is a short literal substring over capability names and summaries. A blank normalized query or any other empty result is `[]`, not `null`. Retry an empty result with a shorter term. Search does not add fuzzy matching, aliases, or extra query rewriting.

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
| `signature` | Invocation-only keyword signature. It ends after the parameter list and never contains a Go output type. |
| `summary` | Registered compact summary. |

`signature` contains the dotted name, a `*` keyword-only marker when there are parameters, and the ordered input fields with their type notations. It ends at `)`. The exact forms are `records.lookup(*, key: str, limit: int | None)` and `records.status()`. The result contract is `describe_api.output`, not `signature`.

## `describe_api`

Describe one enabled capability by the exact name returned by `search_api`, without whitespace or case changes.

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

Name lookup is exact. It neither trims nor case-folds, and it does not perform search, prefix expansion, or fuzzy matching. Pass the exact `name` returned by `search_api`, without whitespace or case changes. An unknown or disabled name returns `capability not found`.

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
      "items": {
        "type": "object",
        "required": ["name", "type", "required"],
        "additionalProperties": false,
        "properties": {
          "name": { "type": "string" },
          "type": { "type": "string" },
          "required": { "type": "boolean" }
        }
      }
    },
    "output": {
      "type": "array",
      "items": {
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
}
```

The required `input` and `output` properties are always arrays. A capability with no input or output fields uses `[]`, not `null`.

`input` and `output` preserve Go field declaration order. Each array item keeps
the flat field-shape schema with only `name`, `type`, and `required`; nested
types are encoded in the `type` string rather than in recursive field-shape
objects.

Input field shapes are:

| Go field type | `type` | `required` |
| --- | --- | --- |
| `string` | `str` | `true` |
| `int64` | `int` | `true` |
| `bool` | `bool` | `true` |
| `float64` | `float` | `true` |
| `*string` | <code>str &#124; None</code> | `false` |
| `*int64` | <code>int &#124; None</code> | `false` |
| `*bool` | <code>bool &#124; None</code> | `false` |
| `*float64` | <code>float &#124; None</code> | `false` |

Named input types with these underlying kinds have the same shapes. Required
integers use the signed 64-bit range, and all floats must be finite. An omitted
optional argument and explicit `None` both produce a nil pointer and omit the
field from the canonical authorization map.

Output `type` strings use this compact notation:

| Value shape | Notation |
| --- | --- |
| String, integer, Boolean, or finite float | `str`, `int`, `bool`, or `float` |
| Array or slice | `list[T]` |
| String-keyed map | `dict[str, T]` |
| Struct | `{field: T, optional?: U}` |
| Pointer | <code>T &#124; None</code> |

Struct fields appear in declaration order. Arrays and slices share `list[T]`.
Pointer layers append ` | None` once. At the root, an `omitempty` pointer uses
`required: false` and the pointed-to type without the outer ` | None`. Inside a
struct, the field name uses `?`, as in `{value: str, detail?: str}`.

For example, one flat `output` array can contain:

```json
[
  {
    "name": "items",
    "type": "list[{title: str, score: float}]",
    "required": true
  },
  {
    "name": "alias",
    "type": "dict[str, bool]",
    "required": true
  },
  {
    "name": "nested",
    "type": "{value: str, detail?: str}",
    "required": true
  },
  {
    "name": "values",
    "type": "list[str | None] | None",
    "required": true
  },
  {
    "name": "extra",
    "type": "str",
    "required": false
  }
]
```

Supported output types include nested structs, arrays and slices, string-keyed
maps, pointers, named scalars, all signed and unsigned integer kinds, finite
`float32` and `float64` values, and byte slices or arrays as integer lists from
0 through 255. Integers are projected through signed 64-bit values, so a
returned unsigned value above `math.MaxInt64` is invalid. Byte sequences are
not base64 encoded.

A nil pointer without `omitempty` remains present as `None`. A nil pointer with
`omitempty` is omitted. Nil slices, maps, and byte slices also become `None`
even though discovery shows `list[T]` or `dict[str, T]`; non-nil empty
containers remain empty. A nil pointer inside a list or map becomes `None`.

The host rejects unsupported reflected shapes when it registers a capability,
before the MCP service can expose it. Rejected output graphs include
interfaces, `json.RawMessage`, types with value or pointer methods that
implement JSON or text marshaling, non-string map keys, cyclic type graphs,
unsupported scalar kinds, embedded or unexported fields, invalid tags, and
`omitempty` on a non-pointer field.

`signature` is the same invocation-only keyword signature returned by
`search_api`. `output` is the stable result contract. Models and clients must
use these field shapes. Clients that parsed a trailing ` -> <GoType>` suffix
on `signature` must stop parsing it and use `describe_api.output` instead.
There is no compatibility suffix, alias, or alternate signature field.

## `execute`

Execute one Starlark program that defines `def main():` with zero arguments, calls only names confirmed through `search_api` and `describe_api` inside `main`, and returns `main`'s final result.

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

The source must define `def main():` as a function with zero arguments. Top-level source loading cannot make native calls; calls are accepted only while `main` runs, and only for names confirmed through `search_api` and `describe_api`. Module loading is disabled.

Capabilities are available by dotted name. The sample native call is `records.lookup(key="alpha", limit=2)`. Native calls accept keyword arguments only. Duplicate keyword syntax is rejected by the Starlark parser as `invalid program` before authorization or handler dispatch. Positional, unknown, missing, incorrectly typed, and out-of-range arguments reach binding and map to `invalid capability arguments`. For the sample, `key` is required and `limit` can be omitted, `None`, or an integer in the signed 64-bit range.

For example, a capability described with the signature
`records.search(*, count: int, active: bool, score: float, label: str | None)`
has a `describe_api.output` field `items` of type
`list[{id: str, active: bool, score: float}]`. It can be composed inside one
program:

```python
def main():
    response = records.search(count=3, active=True, score=1.5)
    ids = []
    total = 0.0
    count = 0
    for item in response["items"]:
        if item["active"]:
            ids.append(item["id"])
            total += item["score"]
            count += 1
    return {"count": count, "score": total, "ids": ids}
```

Each `execute` call gets a fresh interpreter and fresh source, step,
elapsed-time, native-call, conversion-depth, per-value size, and aggregate
intermediate-value budgets. There is no interpreter or aggregate accounting
shared between calls.

### Successful structured output

```json
{
  "type": "object",
  "required": ["result"],
  "additionalProperties": false,
  "properties": {
    "result": true
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

`MaxValueDepth` is inclusive. A scalar or `None` is depth 1. Each tuple, list,
or dictionary wrapper adds one. A scalar with limit 1 succeeds, a one-level
container with limit 2 succeeds, and one more wrapper with limit 2 fails.
Native arguments, native results, and the final value are independently subject
to `MaxValueDepth` and `MaxValueBytes`.

`MaxIntermediateValueBytes` separately bounds the request-scoped sum of
successful parent-to-child native-result value bodies. After each successful
result is encoded, its encoded body length is checked and debited. The sum
excludes frame envelopes, native-call arguments, failed handlers, and the final
program value. Results consume this aggregate budget even when `main` does not
return them. A new `execute` call starts fresh. Starlark tuples and lists become
arrays. `None` becomes `null`. Dictionaries must have string keys.

Only the final converted value from the worker process is exposed in the successful MCP result. `print` output is discarded. Globals, values created during top-level source loading, intermediate expressions, and native results that are not included in the final return value are not added to structured output. The successful envelope contains only `result`.

## Authoring and recovery

The listed descriptions above are the model-facing contract. Recovery uses the same fixed coarse errors on this page. When recording or reporting a failed call, keep the coarse text and the recovery action; do not echo the failed source, arguments, credentials, or unknown requested name.

- Search with a short literal substring over enabled names and summaries. If the result is empty, retry with a shorter term.
- After `capability not found`, search again and pass `describe_api` an exact returned `name`, without whitespace or case changes.
- After `invalid capability arguments`, compare the call with the published `signature` and `input` field shapes.
- After `invalid program`, check the program against these requirements:
  - Define `main` with zero arguments.
  - Call only names confirmed through `search_api` and `describe_api`, and call them only inside `main`.
  - Return the final value from `main`.
- Use `describe_api.output` as the result contract. Do not parse a type name from `signature`.
- After `resource limit exceeded`, reduce the applicable bounded quantity:
  query or source bytes, execution steps or time, native calls, crossing-value
  depth or per-value encoded size, or the cumulative encoded size of successful
  native results. Then retry the call.
- After `permission denied` or `authorization policy failure`, contact the host if the access was expected. One allowed or denied input cannot establish whether the policy is default-open, default-deny, complete, or incomplete.

## Errors

After a well-formed call reaches the adapter, a resolver or service failure becomes a successful MCP protocol response with `isError` set and one of the eleven fixed text values below. The adapter removes resolver and custom-service details and recovered panic values. It does not expose budget values, filtered capability identities, unknown requested names, argument names or values, source locations or text, Rego decision paths or rule names, handler messages, credentials, panic values, or stack details.

| Text | Meaning |
| --- | --- |
| `unauthenticated` | The resolver failed or returned an empty subject ID. |
| `capability not found` | `describe_api` did not find an enabled exact name. |
| `invalid program` | Source, including duplicate keyword syntax, entry point, runtime behavior, or final-value conversion was invalid. |
| `invalid capability arguments` | A native call failed binding: positional, unknown, missing, incorrectly typed, or out-of-range arguments. |
| `permission denied` | Policy returned a recognized denial. |
| `authorization policy failure` | Policy evaluation failed. |
| `resource limit exceeded` | A discovery, execution, depth, per-value, or aggregate intermediate-value budget was exceeded. |
| `capability failed` | A handler failed or returned an invalid value, including a non-finite float or an unsigned integer above `math.MaxInt64`. |
| `context canceled` | The request context was canceled. |
| `context deadline exceeded` | A service returned a bare deadline error. Root CodeMode execution deadlines are projected as `resource limit exceeded`. |
| `internal failure` | Any unknown service error or recovered adapter failure. |

Malformed MCP arguments are rejected by the SDK's input-schema validation and do not call the invocation resolver. These errors can identify malformed client-owned fields or values because validation occurs before trusted resolution. For well-formed inputs, resolver failure stops the request before search, description, or execution and becomes `unauthenticated` without resolver detail.

See [Public API reference](public-api.md) for Go contracts and [Understanding CodeMode's security model](../explanation/security-model.md) for the resolver and execution trust boundaries.
