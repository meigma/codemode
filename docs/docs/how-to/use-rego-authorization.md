---
title: Use Rego for authorization
description: Replace the allow-all authorizer with a prepared in-process Rego authorization decision.
---

# Use Rego for authorization

Use `github.com/meigma/codemode/authz/rego` when a CodeMode host needs a static Rego policy instead of `authz.AllowAll()`. Supply policy source from trusted deployment configuration. The adapter prepares the policy in-process before you build the server.

## Define a total Boolean decision

Create a Rego v1 module whose decision is always either `true` or `false`. A default-deny rule makes the decision total when no allow rule matches:

```rego
package codemode.authz

default allow := false

allow if {
    input.subject.id == "local"
    input.capability.id == "records.entry.lookup"
    input.arguments.key != "forbidden"
}
```

Use `input.capability.id` for policy identity. In this example, `records.entry.lookup` is the stable capability ID. `input.capability.name` is the dotted discovery and Starlark name, `records.lookup`; it can change independently of the policy identity.

Set `ID: "records.entry.lookup"` in the tutorial's capability registration
before writing this policy:

```go
codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
	ID:      "records.entry.lookup",
	Name:    "records.lookup",
	Summary: "Look up one record by key.",
	Handler: lookup,
})
```

An omitted ID defaults to `records.lookup`. Explicit policy identity
prevents a later capability rename from changing the decision input.

The adapter supplies only this input shape:

```json
{
  "subject": {
    "id": "local"
  },
  "capability": {
    "id": "records.entry.lookup",
    "name": "records.lookup"
  },
  "arguments": {
    "key": "alpha",
    "limit": 2
  }
}
```

`subject.id` comes from the trusted `authz.Subject`. The capability fields come from registration. `arguments` contains the canonical, validated keyword arguments for that native call. An omitted optional argument is absent rather than set to `null`. The input does not include credentials, request metadata, Starlark source, environment data, or the current time.

## Prepare the authorizer before the builder

Starting from the [first-server tutorial](../tutorials/first-server.md), import the adapter:

```go
import "github.com/meigma/codemode/authz/rego"
```

Fetch the adapter's dependencies:

```sh
go mod tidy
```

Define the policy at package scope:

```go
const authorizationPolicy = `package codemode.authz

default allow := false

allow if {
    input.subject.id == "local"
    input.capability.id == "records.entry.lookup"
    input.arguments.key != "forbidden"
}`
```

Then replace the tutorial's builder initialization with the following code:

```go
ctx := context.Background()
authorizer, err := rego.New(
	ctx,
	"data.codemode.authz.allow",
	map[string]string{
		"authorization.rego": authorizationPolicy,
	},
)
if err != nil {
	log.Fatal(err)
}

builder := codemode.New(codemode.Options{Authorizer: authorizer})
```

`rego.New` validates the ground reference syntax, compiles every supplied
module, and prepares the policy synchronously. It cannot prove that the
decision is defined and Boolean for every future input; those outcomes are
checked during authorization. Do not build the server when preparation fails.
The returned authorizer is immutable and can serve concurrent calls.

To embed the policy, save the same module as `policies/authorization.rego` and replace the string constant with:

```go
import _ "embed"

//go:embed policies/authorization.rego
var authorizationPolicy string
```

Pass `authorizationPolicy` to the same `rego.New` call. Replacing policy requires preparing a new authorizer and building a new server; the adapter does not load files or reload policy.

## Preserve the decision outcomes

CodeMode calls the authorizer after argument binding and before handler dispatch. A ground decision is either undefined or yields one value. That value must be Boolean. The outcomes are:

- Boolean `true` allows handler dispatch.
- Boolean `false` returns `authz.ErrDenied`, which CodeMode classifies as `codemode.ErrPermissionDenied`.
- An undefined or non-Boolean decision is a policy failure, as is any evaluation or builtin error. CodeMode classifies these errors as `codemode.ErrPolicyFailure`.
- The adapter returns `ctx.Err()` for a canceled or expired evaluation context. At `Server.Execute`, cancellation returns `context.Canceled`; a deadline is `codemode.ErrResourceLimit` and also wraps `context.DeadlineExceeded`.

Do not use an undefined decision as denial behavior. Keep `default allow := false` so an unmatched input produces the intentional Boolean `false` result.

## Verify the policy

Rebuild the server binary:

```sh
go build -o codemode-first-server .
```

Reload the server in the configured agent.

### Verify an allowed call

Ask the agent to run this program through `execute` (the program text is the tool's `source` argument):

```python
def main():
    return records.lookup(key="alpha", limit=2)
```

The structured result is `{"result":{"count":2,"key":"alpha"}}`.

### Verify a denied call

Ask the agent to run this program through `execute`:

```python
def main():
    return records.lookup(key="forbidden", limit=2)
```

`execute` returns a tool error whose text is `permission denied`.
`search_api` and `describe_api` still succeed.

See the [`authz/rego` API reference](../reference/public-api.md#authzrego) for constructor validation and exact result semantics. See [Understanding CodeMode's security model](../explanation/security-model.md#rego-policy-runs-in-process) for the policy trust boundary and runtime restrictions.
