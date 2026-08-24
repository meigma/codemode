# Worker Re-exec Spike

Disposable spike proving the worker-only execution design for issue #12.
Code lived in `/tmp/codemode-worker-spike` (single `main.go` + `main_test.go`)
and is not retained; this document is the evidence of record.

## Environment

- Date: 2026-08-24
- Machine: Apple M4 Max, macOS (darwin 25.4.0, arm64)
- Go: 1.26.x toolchain, module `go 1.26.6`
- Starlark: `go.starlark.net v0.0.0-20260708150628-5395d018f003` (same pin as `master`)
- Reference in-process baseline: issue #12 reproduction on `ccac0332a64c52530ebf0102849f73eecf867a12`

## Method

One binary. `main` (and `TestMain`) branch on the `CODEMODE_SPIKE_WORKER` env
marker: marker present → serve exactly one worker request on stdin/stdout and
exit; otherwise run the parent scenarios. Parent spawns `os.Executable()` with
the marker, speaks 4-byte length-prefixed JSON frames (`probe`/`probe_ack`,
`exec`, `native_call`/`native_result`, `final`), enforces a 1 MiB frame cap on
reads, and hard-kills the child (`Process.Kill` + `Wait` reap) when the
deadline timer fires. The child runs a fresh Starlark thread with a step limit
and a `records.lookup` namespace stub that forwards canonical keyword args to
the parent and blocks for the converted JSON result. The child constructs no
handler and no authorizer.

## Results

Probe handshake (spawn + ack + reap), 10 runs: min 2.32 ms, median 2.51 ms,
max 3.23 ms.

Trivial execution `def main(): return 42` end-to-end (spawn + exec + final +
reap), 10 runs: min 2.25 ms, median 2.33 ms, max 2.93 ms.

Native forwarding: `records.lookup(value="alpha")` executed in the child,
handler ran in the parent (result carried the parent pid), value round-tripped
correctly; total 2.81 ms.

Kill-at-deadline, 1 ms deadline, versus the in-process baseline:

| Payload | In-process (issue repro) | Worker window | Worker overrun |
|---|---:|---:|---:|
| `list(range(8000000))` | 25.7–30.8 ms (25.7–30.8x) | 1.22–1.35 ms | 1.2–1.3x |
| `list.extend(range(8000000))` | 143.5–157.0 ms (143.5–157.0x) | 1.30–1.32 ms | 1.3x |

Absolute kill + timer overhead was ~0.25–0.35 ms past the deadline; total
including spawn stayed under 2.0 ms. Against the 5 s default budget the
overhead is noise.

Secondary gates:

- Child-side `SetMaxExecutionSteps` still enforced: step-limited loop returned
  `Starlark computation cancelled: too many steps` via the final error frame.
- Fork-bomb guard: `executeWorker` observing the worker marker refused to
  spawn (`refusing to spawn a worker inside a worker process`).
- `TestMain` ergonomics: an 8-line marker branch in `TestMain` was sufficient;
  `go test -v` and `go test -race` passed, including a deadline-kill test
  asserting the window stays under 50 ms.

## Gate verdicts

1. Same-binary re-exec (`os.Executable()` + env marker) — PASS.
2. Length-prefixed JSON frames over inherited pipes, bounded reads — PASS.
3. Native call forwarded child→parent, handler parent-side only — PASS.
4. Kill-at-deadline defeats the issue #12 payloads — PASS (26–157x overrun
   reduced to 1.2–1.3x on a 1 ms budget).
5. Build-time probe handshake — PASS, ~2.5 ms including spawn and reap.
6. Fork-bomb guard — PASS.
7. `TestMain` ergonomics — PASS, single small branch, race-clean.

## Learnings for the real design

- The child needed no handler, authorizer, or credential to execute programs
  and forward native calls. This empirically supports the plans-only worker
  build: the child requires only namespace/binding data.
- JSON round-tripping erased integer types (`int64` → `float64` across the
  wire). The real protocol must fix numeric encoding (e.g. `json.Number` or a
  typed envelope) to preserve CodeMode's `int64` output contract.
- Plain `Process.Kill` sufficed; the child spawns nothing, so process-group
  kill is defense in depth rather than a correctness requirement. This keeps
  the kill path portable.
- The parent's frame-reader goroutine can linger briefly on kill; the real
  implementation needs deterministic shutdown (close pipes, drain, join).
- stderr passthrough was used for spike debugging; the real design must pick a
  worker stderr policy (discard by default, host-configurable sink).
- Spawn cost (~2.5 ms median) makes fresh-child-per-execution viable without
  pooling; revisit only if a real workload shows spawn dominating.

## Not exercised

- Hard memory caps (Linux cgroups); macOS remains dev-grade by design.
- Concurrency semaphore across parallel executions.
- Windows kill semantics.
- MCP boundary integration; the spike drove the executor directly.
