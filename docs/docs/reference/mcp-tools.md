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

Search enabled capabilities using task, resource, or exact-name vocabulary.
Results are relevance-ranked. Pass the exact returned name to `describe_api`.
If `truncated` is `true` and no result fits, submit a more specific
task/resource query.

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

The raw query is limited by `MaxSearchQueryBytes` before trimming or
tokenization. Whitespace padding counts. CodeMode trims surrounding Unicode
whitespace, treats every rune that is not a Unicode letter or digit as a
separator, splits camel-case, acronym-to-word, and letter/number transitions,
and lowercases each token. Dots, underscores, and hyphens are therefore
separators.

For example, `GitHub.Pulls.createReview` becomes `github`, `pulls`, `create`,
`review`; `pull_request` becomes `pull`, `request`; and `sql` and `mysql`
remain different tokens.

Search removes the connector tokens `a`, `an`, `and`, `by`, `for`, `from`,
`in`, `of`, `on`, `or`, `the`, `to`, and `with`, then deduplicates the
remaining query tokens. A query with more than 16 distinct normalized tokens
returns `resource limit exceeded`.

Search compares the distinct query tokens with tokens from each enabled
capability's name, registered `SearchTerms`, summary, and description. Exact
token matches are supported. A query token of at least three Unicode characters
can also match the prefix of a capability token. Arbitrary infix matching,
fuzzy matching, stemming, and built-in synonym expansion are not supported.

For one query token within one field, an exact match ranks above a prefix
match. Search retains only the strongest contribution for that query token.
The field precedence used for weighting is the capability name, then
`SearchTerms`, then the summary, then the description. Terms found in fewer
enabled capabilities contribute more than catalog-wide terms. Numeric scoring
weights are internal.

Eligibility depends on the number `q` of distinct normalized query tokens:

| Query tokens | Required matched tokens |
| ---: | ---: |
| 1 | 1 |
| 2 | 2 |
| 3 or more | `ceil(2q / 3)` |

Search ranks every eligible capability before applying output bounds. A
case-insensitive exact dotted-name query, after trimming surrounding
whitespace, ranks first. Remaining ordering is relevance score descending,
then exact dotted name ascending. Static filtering happens before indexing, so
disabled capabilities cannot match or affect ranking.

### Successful structured output

```json
{
  "type": "object",
  "required": ["results", "truncated"],
  "additionalProperties": false,
  "properties": {
    "results": {
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
    },
    "truncated": {
      "type": "boolean"
    }
  }
}
```

A populated successful value is:

```json
{
  "results": [
    {
      "name": "records.lookup",
      "signature": "records.lookup(*, key: str, limit: int | None)",
      "summary": "Look up one record by key."
    }
  ],
  "truncated": false
}
```

A blank query, a separator- or connector-only query, and a query with no
eligible matches all succeed with this exact object:

```json
{
  "results": [],
  "truncated": false
}
```

`results` is always a non-null array on success. CodeMode packs the
highest-ranked prefix under `MaxSearchResults` and an internal structured
response-byte bound. `truncated` is `true` when either bound omits at least one
eligible capability. It does not expose a total count or provide pagination.
The byte bound covers the compact JSON representation of the structured
`{results, truncated}` response. The surrounding JSON-RPC envelope and the MCP
SDK's JSON `TextContent` mirror are outside that cap.

| Field | Meaning |
| --- | --- |
| `results[].name` | Exact enabled dotted capability name, such as `records.lookup`. |
| `results[].signature` | Invocation-only keyword signature. It ends after the parameter list and never contains a Go output type. |
| `results[].summary` | Registered compact summary. |
| `truncated` | Whether at least one eligible result was omitted by the result-count or structured-response byte bound. |

`signature` contains the dotted name, a `*` keyword-only marker when there are
parameters, and the ordered input fields with their type notations. It ends at
`)`. The exact forms are
`records.lookup(*, key: str, limit: int | None)` and `records.status()`. The
result contract is `describe_api.output`, not `signature`.

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

Execute one Starlark program that defines `def main():` with zero arguments, calls only names confirmed through `search_api` and `describe_api` inside `main`, and returns `main`'s final result. Standard Starlark builtins plus `sum(iterable)`, `json.decode/encode/indent`, and `math.*` are directly available without import. `import`, `while`, f-strings, `filter`, and `map` are unavailable; `load` is disabled; `print` is discarded.

### Language surface

The following block is the exact fixed language surface. These names are
directly available without `import` or `load`. `set` is reserved as a
capability root but is not available. There is no built-in `time` module.

```language-surface
top-level: False, None, True, abs, all, any, bool, bytes, chr, dict, dir, enumerate, fail, float, getattr, hasattr, hash, int, json, len, list, math, max, min, ord, print, range, repr, reversed, sorted, str, sum, tuple, type, zip
json: decode, encode, indent
math: acos, acosh, asin, asinh, atan, atan2, atanh, ceil, copysign, cos, cosh, degrees, e, exp, fabs, floor, gamma, hypot, log, mod, pi, pow, radians, remainder, round, sin, sinh, sqrt, tan, tanh
```

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

The source must define `def main():` as a function with zero arguments. Standard Starlark builtins plus `sum(iterable)`, `json.decode/encode/indent`, and `math.*` are directly available without import. `import`, `while`, f-strings, `filter`, and `map` are unavailable; use comprehensions instead of `filter` and `map`. Top-level source loading cannot make native calls; calls are accepted only while `main` runs, and only for names confirmed through `search_api` and `describe_api`. Module loading is disabled. `print` output is discarded.

Capabilities are available by dotted name. The sample native call is `records.lookup(key="alpha", limit=2)`. Native calls accept keyword arguments only. Duplicate keyword syntax is rejected by the Starlark parser as `invalid program` before authorization or handler dispatch. Positional, unknown, missing, incorrectly typed, and out-of-range arguments reach binding and map to `invalid capability arguments`. For the sample, `key` is required and `limit` can be omitted, `None`, or an integer in the signed 64-bit range. The first dotted segment of a capability name must not be a reserved root (any standard Starlark universe name, plus `sum`, `json`, and `math`); nested leaves such as `stats.sum` remain legal. There is no built-in `time` module; a host-defined `time.*` capability remains legal.

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

Each `execute` call gets a fresh interpreter and fresh source, bytecode-step,
elapsed-time, native-call, conversion-depth, per-value size, and aggregate
intermediate-value budgets. `MaxExecutionSteps` counts Starlark bytecode steps
and does not claim that Go builtin internals consume steps. `MaxExecutionTime`
bounds elapsed worker execution. There is no interpreter or aggregate accounting
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

The listed descriptions above are the model-facing contract. Recovery uses the nine fixed texts and two stable prefixes on this page. When recording or reporting a failed call, keep the error text and the recovery action; do not echo credentials, unknown requested names, or host-derived handler or policy text.

- Search with task, resource, or exact-name vocabulary. If `truncated` is `true`, use a more specific task/resource query. Pass an exact returned `name` to `describe_api`.
- After `capability not found`, search again and pass `describe_api` an exact returned `name`, without whitespace or case changes.
- After `invalid capability arguments`, use any suffix after the stable prefix to identify the rejected argument, then compare the call with the published `signature` and `input` field shapes.
- After `invalid program`, use any suffix after the stable prefix. A parse or resolve suffix includes a `<codemode>:line:col:` position in the submitted source. Check the program against these requirements:
  - Write Starlark, not Python: `import`, `while`, f-strings, `filter`, and `map` are unavailable; `sum(iterable)`, `json.decode/encode/indent`, and `math.*` are directly available without import.
  - Define `main` with zero arguments.
  - Call only names confirmed through `search_api` and `describe_api`, and call them only inside `main`.
  - Return the final value from `main`.
- Use `describe_api.output` as the result contract. Do not parse a type name from `signature`.
- After `resource limit exceeded`, reduce the applicable bounded quantity:
  query or source bytes, distinct normalized query tokens, execution steps or
  time, native calls, crossing-value depth or per-value encoded size, or the
  cumulative encoded size of successful native results. Then retry the call.
- After `permission denied` or `authorization policy failure`, contact the host if the access was expected. One allowed or denied input cannot establish whether the policy is default-open, default-deny, complete, or incomplete.

## Errors

After a well-formed call reaches the adapter, a resolver or service failure becomes a successful MCP protocol response with `isError` set. Nine texts are fixed. Two classes keep a stable prefix and may append model-derived detail: `invalid program: ...` and `invalid capability arguments: ...`. The adapter removes resolver and custom-service details and recovered panic values. It does not expose budget values, filtered capability identities, unknown requested names, host-derived argument values, Rego decision paths or rule names, handler messages, credentials, panic values, or stack details. Parse and resolve suffixes may include a source position in the submitted program. Binding suffixes may include an argument name from the submitted call.

| Text | Meaning |
| --- | --- |
| `unauthenticated` | The resolver failed or returned an empty subject ID. |
| `capability not found` | `describe_api` did not find an enabled exact name. |
| `invalid program` | Fixed prefix. Source, including duplicate keyword syntax, entry point, runtime behavior, or final-value conversion was invalid. A parse or resolve failure may append `<codemode>:line:col: message`. |
| `invalid capability arguments` | Fixed prefix. A native call failed binding: positional, unknown, missing, incorrectly typed, or out-of-range arguments. A binding failure may append the model-derived argument diagnostic. |
| `permission denied` | Policy returned a recognized denial. |
| `authorization policy failure` | Policy evaluation failed. |
| `resource limit exceeded` | A discovery, execution, depth, per-value, or aggregate intermediate-value budget was exceeded. |
| `capability failed` | A handler failed or returned an invalid value, including a non-finite float or an unsigned integer above `math.MaxInt64`. |
| `context canceled` | The request context was canceled. |
| `context deadline exceeded` | A service returned a bare deadline error. Root CodeMode execution deadlines are projected as `resource limit exceeded`. |
| `internal failure` | Any unknown service error or recovered adapter failure. |

Malformed MCP arguments are rejected by the SDK's input-schema validation and do not call the invocation resolver. These errors can identify malformed client-owned fields or values because validation occurs before trusted resolution. For well-formed inputs, resolver failure stops the request before search, description, or execution and becomes `unauthenticated` without resolver detail.

See [Public API reference](public-api.md) for Go contracts and [Understanding CodeMode's security model](../explanation/security-model.md) for the resolver and execution trust boundaries.
