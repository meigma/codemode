# OPA/Rego adapter spike

## Status

Completed on 2026-08-23 with Go 1.26.6 on Darwin arm64 and OPA v1.19.1.

**Verdict:** The adapter mechanics are viable. Production implementation is a conditional go because the embedded OPA dependency adds substantial module, compilation, and binary weight. The developer must accept that cost or choose a stronger isolation boundary before implementation.

No CodeMode repository file changed. The scratch modules and caches were deleted after the spike.

## Scope

Two independent disposable scratch modules tested:

- Rego query, decision, input, capability, and concurrency mechanics.
- Dependency, build, link, cancellation, and goroutine behavior.

The spike used the repository's mise-pinned Go 1.26.6 with `GOTOOLCHAIN=local`. Both modules pinned `github.com/open-policy-agent/opa v1.19.1`, the latest official OPA release observed on 2026-08-23.

## Decision-query contract

OPA's direct query supports the proposed contract:

```go
ref, err := ast.ParseRef("data.authz.policy.allow")
query := rego.New(rego.Query(ref.String()), /* modules and restrictions */)
prepared, err := query.PrepareForEval(ctx)
results, err := prepared.Eval(ctx, rego.EvalInput(input))
decision, ok := rego.ResultValue[bool](results)
```

Observed results:

| Decision shape | `ResultValue[bool]` | Adapter meaning |
|---|---|---|
| Complete Boolean `true` | `true, true` | Allow |
| Complete Boolean `false` | `false, true` | `authz.ErrDenied` |
| Undefined | `false, false`, zero results | Policy failure |
| String | `false, false` | Policy failure |
| Multi-value set query | `false, false`, two results | Policy failure |

`ResultValue[bool]` requires exactly one result, no bindings, one expression, and a Boolean value. A generated binding query such as `decision = data.authz.policy.allow` cannot use this helper because the result contains a binding. The production adapter should query the canonical data reference directly.

`ast.ParseRef` plus `Ref.IsGround`, `Ref.HasPrefix(ast.DefaultRootRef)`, and canonical `Ref.String` support the proposed ground `data` reference validation. Dynamic references and `input`-rooted references were rejected by the proposed checks.

## Capability restrictions

OPA v1.19.1 declares these nine builtins nondeterministic:

- `http.send`
- `io.jwt.decode_verify`
- `io.jwt.encode_sign`
- `io.jwt.encode_sign_raw`
- `net.lookup_ip_addr`
- `opa.runtime`
- `rand.intn`
- `time.now_ns`
- `uuid.rfc4122`

After removing every builtin for which `Builtin.IsNondeterministic()` returns true, representative policies using `http.send`, `net.lookup_ip_addr`, `opa.runtime`, `rand.intn`, and `time.now_ns` failed during `PrepareForEval` with an undefined-function type error.

### `AllowNet`

An empty `Capabilities.AllowNet` is not the prepare-time mechanism that blocks `http.send`. `http.send` still prepared when its builtin remained available. Empty `AllowNet` did:

- block a remote JSON Schema `$ref` during compilation;
- reject an `http.send` host during evaluation if `http.send` remained enabled.

Production must remove nondeterministic builtins to reject `http.send` during preparation. Keep `AllowNet` empty as a separate defense for remote schemas and any future network-capable path.

### Print and trace

`EnablePrintStatements(false)` erases `print()` calls. A policy containing `print()` prepared and evaluated successfully; the call did not execute.

With default capabilities and no tracer, `trace("note")` evaluated as true and produced no external effect. Removing `internal.print` or `trace` can make explicit calls fail preparation, but those removals are not the primary I/O boundary. The production contract should rely on disabled print statements, no print hook, and no tracer without overstating these controls.

### Builtin errors

`StrictBuiltinErrors(true)` is required. With it enabled, `to_number("not-a-number")` caused an evaluation error even when another rule branch would allow. With it disabled, the failing branch became undefined and the other branch allowed.

## Input projection

The authorization-shaped input preserved the proposed semantics:

```json
{
  "subject": {"id": "subject-1"},
  "capability": {
    "id": "records.entry.lookup",
    "name": "records.lookup"
  },
  "arguments": {"limit": 42}
}
```

A Go `int64(42)` compared exactly with Rego `42`. Evaluation did not mutate the source arguments map or change the Go value type. This supports borrowing the fresh canonical arguments map read-only for synchronous evaluation instead of adding another deep copy.

## Concurrency and cancellation

One `PreparedEvalQuery` passed concurrent mixed-input evaluation under the race detector with 16 goroutines.

Cancellation used an expensive deterministic `numbers.range` query with a 25 ms deadline. Eight race-enabled trials returned in:

```text
25.570834 ms
26.059167 ms
26.115250 ms
26.059125 ms
26.069791 ms
26.064250 ms
26.066542 ms
26.048667 ms
```

Maximum observed return latency was 26.115250 ms, or 1.115250 ms after the deadline. A batch of 20 cancelled evaluations settled from two goroutines back to two after 500 ms. Concurrent cancelled evaluations against one prepared query also passed under the race detector.

OPA returned a `topdown.Error` with `eval_cancel_error`; `errors.Is(err, context.DeadlineExceeded)` was false. The production adapter should retain the architecture's post-evaluation context check and return `ctx.Err()` before interpreting OPA's error. This preserves CodeMode's existing cancellation classification without adding OPA error types to the public API.

These observations do not establish a CPU, heap, or tenant boundary. Cancellation remains cooperative and was measured only for the tested deterministic workload on this host.

## Dependency and build cost

The comparison used:

- baseline: a 63-byte `fmt.Println("ok")` program;
- OPA: a 627-byte program that prepared and evaluated one in-memory Rego decision, then printed `ok`.

Both binaries executed successfully.

| Measurement | Baseline | OPA | Delta |
|---|---:|---:|---:|
| Module graph (`go list -m all`) | 1 | 127 | +126 |
| Non-standard packages compiled, excluding main | 0 | 172 | +172 |
| All dependency packages | 60 | 380 | +320 |
| `go.mod` | 51 B / 3 lines | 1,600 B / 37 lines | +1,549 B / 34 lines |
| `go.sum` | absent | 12,134 B / 135 lines | +12,134 B / 135 lines |
| Linked binary | 2,492,466 B | 32,936,018 B | +30,443,552 B |

The OPA binary was 31.41 MiB versus the 2.38 MiB baseline, a 29.03 MiB increase. `CGO_ENABLED=0` and the default `CGO_ENABLED=1` produced identical sizes on this machine.

Clean-cache build wall times used separate empty `GOCACHE` directories and one populated module cache:

| Trial | Baseline | OPA |
|---|---:|---:|
| 1 | 1.848 s | 6.196 s |
| 2 | 1.480 s | 8.400 s |
| 3 | 1.506 s | 9.271 s |

After cache fill, repeated warm builds were approximately 0.106–0.110 s for the baseline and 0.560–0.563 s for OPA.

These are raw measurements from an Apple M4 Max during concurrent work, not portable performance claims. The module graph contains dependencies that the small program did not compile: the imported compiled set represented 26 external modules and 172 non-standard packages.

## Architecture consequences

The spike confirms:

- Keep `authz/rego` behind the existing `authz.Authorizer` port.
- Use OPA's low-level `v1/rego` API.
- Parse and query one direct ground `data` reference.
- Decode with `ResultValue[bool]`.
- Prepare once and share the immutable prepared query.
- Filter every OPA-declared nondeterministic builtin.
- Keep `AllowNet` empty as a separate schema and runtime host restriction.
- Enable fatal builtin errors.
- Disable print statements and expose no hooks, tracer, store, resolver, plugins, or custom builtins.
- Borrow canonical arguments read-only.
- Check `ctx.Err()` before and after evaluation.

The spike corrects these possible misconceptions:

- Empty `AllowNet` does not make `http.send` fail preparation; nondeterministic builtin removal does.
- Disabled print statements are erased, not rejected.
- A binding query cannot use `ResultValue[bool]`.
- OPA's cancellation error does not unwrap to `context.DeadlineExceeded`; the adapter must prefer the post-evaluation `ctx.Err()`.

## Remaining decision

Embedding OPA is mechanically sound but not lightweight. Before production implementation, decide whether the optional root-module package justifies:

- 126 additional modules in the selected graph;
- 172 additional compiled non-standard packages when the adapter is imported;
- approximately 29 MiB of linked binary growth in this minimal comparison;
- longer clean and warm builds.

If that cost is acceptable, proceed with the three-file root-module adapter. If module-graph isolation is required for consumers that do not use Rego, revisit the nested-module tradeoff before implementation. If policy authors are mutually untrusted, reject the in-process adapter and use process or container containment.

## Commands and cleanup

Representative successful commands:

```text
mise x -- go -C <mechanics-scratch> test -count=1 -v .
mise x -- go -C <mechanics-scratch> test -count=1 -race -run 'TestPreparedQueryConcurrentEvalRace|TestInt64InputExactAndUnmutated'
mise x -- go -C <cost-scratch>/cancel test -count=1 -timeout 180s -race -v .
mise x -- go -C <cost-scratch>/cancel test -count=1 -timeout 180s -run TestUncancelledDuration -v .
```

Deleted scratch paths:

```text
/tmp/rego-mech-cCwxNi
/tmp/rego-cost-zOwm1S
/tmp/rego-cost-gocache-dWKruq
```

A parent-process verification found all three paths absent. `git status --porcelain --untracked-files=all` remained empty in the CodeMode repository, and `go.mod` contains no OPA dependency.
