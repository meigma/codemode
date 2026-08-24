# CodeMode Manual Functional Test Plan

Status: proposed; not yet executed.

## Purpose

Exercise CodeMode as an external Go consumer and as an MCP server used by real one-shot OMP agents. The test must cover every observable promise in the public documentation, identify library defects and documentation mismatches, and record developer and model-facing UX problems.

This is not a source review and does not use package-internal tests as evidence. The test operator must build standalone harnesses against the public packages, observe raw results and side effects, and compare them with the published contracts.

## Contract under test

Use these public documents as the oracle:

- `README.md`
- `docs/docs/index.md`
- `docs/docs/tutorials/first-server.md`
- `docs/docs/how-to/disable-capabilities.md`
- `docs/docs/how-to/use-rego-authorization.md`
- `docs/docs/reference/public-api.md`
- `docs/docs/reference/mcp-tools.md`
- `docs/docs/explanation/security-model.md`

At execution time, record:

- the CodeMode module version and commit
- whether the module came from `github.com/meigma/codemode@master` or a local candidate through a temporary `replace`
- `go version`
- `omp --version`
- the OMP model used for each one-shot run
- the SHA-256 digest of each harness source file and prompt

Run the clean-install and tutorial cases against `@master`, because that is the documented installation path. When testing an unmerged candidate, run the remaining cases against a temporary module whose `replace github.com/meigma/codemode => <candidate-worktree>` points at that candidate. Do not copy CodeMode source into the harness.

## Boundaries

### Included

- All four public Go packages: `codemode`, `authz`, `authz/rego`, and `mcpserver`.
- Public constructors, types, lifecycle rules, sentinels, supported binding matrix, catalog operations, execution behavior, authorization order, Rego decision behavior, limits, cancellation classifications, concurrency promises, and MCP wire contracts.
- Real official MCP transports. OMP-facing cases use a local stdio MCP server.
- Developer UX: installation, compilation, configuration, diagnostics, and recovery.
- Model UX: discovery, description, composition, error recovery, and output clarity.

### Excluded

- Reading private implementation or tests to derive expected behavior.
- Re-testing OPA or the official MCP SDK beyond the behavior CodeMode promises at their boundaries.
- Fuzzing, long soak tests, benchmarks, performance targets, production authentication, remote transports, or container isolation.
- Claims that CodeMode explicitly does not make: hard tenant/heap/CPU isolation, forced interruption of blocking Go code, transport ownership, listener management, authentication, credential storage, or downstream MCP forwarding.
- Exhaustive combinations of invalid Go field types. Use one representative for each documented rejection class.

A case is in scope only when a public document makes an observable promise. For numeric limits, test below, exactly at, and above the boundary where the representation permits it. For concurrency, use a bounded stress probe rather than an open-ended soak.

## External harness

Create a disposable directory outside the CodeMode repository. Do not add `_test.go` files or import `internal` packages.

```text
codemode-functional/
├── go.mod
├── go.sum
├── cmd/
│   ├── codemode-probe/       # Direct public-Go-API and raw MCP observations
│   └── codemode-mcp/         # Stdio MCP host selected by --fixture
├── fixtures/                 # Public input/output structs and host adapters
├── policies/                 # Trusted Rego v1 modules
├── prompts/                  # One-shot OMP assignments
├── .omp/mcp.json             # Project-local MCP server definitions
└── evidence/                 # Commands, stdout, stderr, agent reports, and findings
```

### `codemode-probe`

A scenario runner that uses only exported APIs. Each invocation performs one named scenario and prints newline-delimited observation records. Records include the operation, supplied public inputs, returned public values, `errors.Is` classifications, handler/authorizer/resolver counters, and sanitized event order. The runner exits nonzero only when the harness itself cannot start; it does not encode pass/fail assertions for CodeMode behavior.

Use host-side observer adapters to make otherwise hidden boundaries visible:

- an authorizer that records subject, capability ID, capability name, canonical arguments, call count, and event order
- handlers that record the typed subject/input and can return a value, error, panic, wait for cancellation, or mutate host state
- a resolver that records calls and can return a subject, empty subject, diagnostic error, or panic
- a custom `mcpserver.Service` that returns each documented error classification and a secret canary in wrapped diagnostics

The observer must copy received values before recording them. It must not retain borrowed maps and accidentally create a false mutation result.

### `codemode-mcp`

A real stdio MCP host built with the official MCP Go SDK. Select a deterministic fixture with `--fixture`; never expose a fixture-selection tool to the model. Every fixture writes host observations to its own file under `evidence/host/` and keeps stdout exclusively for MCP frames.

Use these fixtures:

| Fixture | Purpose |
| --- | --- |
| `happy` | Several enabled capabilities covering all supported input/output fields, deterministic state, and explicit `AllowAll`. |
| `filtered` | One enabled and one disabled capability under the same namespace. |
| `deny` | Subject- and argument-aware recognized denials, with dispatch counters. |
| `policy-failure` | Ordinary authorizer error and authorizer panic with secret canaries. |
| `handler-failure` | Handler error, handler panic, and invalid/non-finite output with secret canaries. |
| `resolver-failure` | Resolver error and empty-subject modes. |
| `tight-limits` | Small positive limits that an MCP caller can cross quickly and safely. |
| `rego` | Prepared total Boolean Rego policy plus separate undefined, non-Boolean, and builtin-failure modes. |

Keep all handlers deterministic. A stateful fixture may expose host-observed call counts, but no CodeMode capability should exist only to tell the test agent whether CodeMode passed. The agent must infer behavior from the same public surface available to a real client.

### OMP configuration

Use project-local `codemode-functional/.omp/mcp.json`, not the developer's shared `~/.omp/agent/mcp.json`. Give each fixture a unique stdio server name and an absolute `cwd`. A representative entry is:

```json
{
  "$schema": "https://raw.githubusercontent.com/can1357/oh-my-pi/main/packages/coding-agent/src/config/mcp-schema.json",
  "mcpServers": {
    "codemode-happy": {
      "type": "stdio",
      "command": "go",
      "args": ["run", "./cmd/codemode-mcp", "--fixture", "happy"],
      "cwd": "/absolute/path/to/codemode-functional",
      "timeout": 30000
    }
  },
  "disabledServers": []
}
```

Before agent testing, open one interactive OMP session in the harness directory and run `/mcp list` and `/mcp test <name>` for every fixture. Record configuration, connection, initialization, and `tools/list` failures separately from CodeMode findings.

Run each test agent with an isolated profile and ephemeral session:

```sh
omp --profile codemode-functional \
  --cwd /absolute/path/to/codemode-functional \
  --no-session --no-skills --no-rules --no-extensions --no-lsp \
  --auto-approve --max-time 10m -p @prompts/<assignment>.md
```

Do not pass `--no-tools`; MCP tools must remain enabled. Capture stdout and stderr separately. Limit each assignment to 30 MCP calls and ten minutes. Restart the OMP process between assignments so each run rediscovers the MCP servers and does not retain conversation state.

## Execution order

### 1. Prove the consumer path

1. Create a clean Go module with the documented Go version.
2. Run `go get github.com/meigma/codemode@master`.
3. Copy the first-server tutorial exactly, without corrections.
4. Run `go mod tidy` and `go run .`.
5. Compare all three printed tool results with the tutorial.
6. Record every copy/paste, compile, dependency, and runtime problem as product or documentation UX evidence.
7. Build minimal external programs that import each public package. Confirm that CodeMode supplies a library and SDK server, while the harness must choose and own its transport lifecycle.

Stop if the published onboarding path cannot produce a running server. Preserve the failure, then make the smallest harness-only correction needed to continue and label it clearly.

### 2. Exercise the public Go API directly

Run each `codemode-probe` case as a separate command. Preserve raw stdout and stderr before classifying the result.

### 3. Exercise the MCP surface with protocol probes

Use an official MCP SDK client for deterministic schema and envelope checks. Use a custom `mcpserver.Service` only when a root `*codemode.Server` cannot intentionally produce a documented adapter classification.

### 4. Exercise the MCP surface with one-shot OMP agents

Run the blind UX assignment first, before prompts reveal expected names or workflows. Then run focused assignments against freshly started fixture servers. Agents may call only the assigned CodeMode MCP server; prompts must forbid filesystem reads, shell commands, and source inspection.

### 5. Reproduce and triage every anomaly

Re-run each anomaly once with the same harness and prompt. If it reproduces, reduce it to one direct probe or one MCP transcript. Distinguish CodeMode defect, documentation mismatch, poor UX, OMP/client behavior, and harness defect. Do not change library source during this test campaign.

## Coverage matrix

### Consumer setup and registration

| ID | Manual scenario | Expected observation |
| --- | --- | --- |
| C01 | Clean `@master` install and unchanged first-server tutorial | Dependency resolution succeeds under documented Go version; all three tools run and `execute` returns the documented record. |
| C02 | Register one capability containing required `string`, optional `*int64`, and output `string`, `int64`, `bool`, `float64` fields | Registration and build succeed; generated signatures and shapes use the documented field names, types, required flags, and declaration order. |
| C03 | Metadata rejection variants | Empty or whitespace-padded ID, invalid dotted name, empty or padded summary/description, nil handler, duplicate ID/name, and namespace/name collision each classify as `ErrInvalidRegistration`. |
| C04 | Binding-shape rejection representatives | Pointer input/output structs, unexported or embedded fields, unsupported tags/options, input `string` with `omitempty`, unsupported scalar/container/pointer fields, and output `omitempty` classify as invalid registration. |
| C05 | Nil/closed builder registration | Nil builder and registration after any build attempt classify as invalid registration without panic. |
| C06 | Build validation | Nil and typed-nil authorizer, zero/non-positive limits, duplicate disabled IDs, and unknown disabled IDs classify as invalid registration. |
| C07 | One-shot builder lifecycle | The first build closes the builder even when validation fails; later register/build operations classify as invalid registration. |
| C08 | Option-copy probe | Mutating caller-owned disabled-ID and limits values after `New` does not alter the built server. |
| C09 | Immutable concurrent server | 32 goroutines perform 20 mixed search/describe/execute operations each without inconsistent results, panic, deadlock, or shared interpreter state. Host adapters are concurrency-safe. |

### Catalog and static filtering

| ID | Manual scenario | Expected observation |
| --- | --- | --- |
| D01 | Blank and whitespace-only search | Empty, non-nil result. |
| D02 | Search with surrounding whitespace and mixed case | Case-normalized substring matching covers names and summaries. |
| D03 | Multiple matches registered out of order | Results are sorted by exact dotted name and capped at `MaxSearchResults`. |
| D04 | Query byte boundary, including multibyte text | At/below the byte limit proceeds; above it classifies as `ErrResourceLimit`. |
| D05 | Exact describe and case/prefix/unknown variants | Only the exact case-sensitive enabled name succeeds; others classify as `ErrNotFound`. |
| D06 | Description shape | Name, signature, summary, description, input, and output exactly match registration and preserve field order. |
| D07 | Disabled capability across every surface | It is absent from search and describe, missing from the Starlark namespace, cannot dispatch, and does not invoke authorizer or handler. Enabled siblings remain available. |

### Execution, binding, authorization, and results

| ID | Manual scenario | Expected observation |
| --- | --- | --- |
| E01 | Nil context and empty subject | Nil context is rejected without panic; empty subject classifies as `ErrUnauthenticated`. |
| E02 | Valid zero-argument `main()` | A fresh interpreter executes and returns only `main()`'s final converted value. |
| E03 | Invalid entry-point forms | Missing/non-function `main`; positional, keyword-only, variadic positional, or variadic keyword parameters; invalid source; and invalid runtime behavior classify as `ErrInvalidProgram`. |
| E04 | Top-level native call and module load | Both are rejected as invalid program behavior; no handler dispatch occurs. |
| E05 | Argument binding rejection classes | Positional, unknown, duplicate, missing required, wrong type, and out-of-range integer calls classify as `ErrInvalidArguments`; authorization and handler counters remain zero. |
| E06 | Optional integer variants | Omitted and explicit `None` yield nil typed input and omit the canonical map key; a supplied integer becomes `int64` in both paths. |
| E07 | Authorization input and order | Each valid attempted native call records bind → authorize → handler order with trusted subject, stable ID, dotted name, and a fresh canonical map. |
| E08 | Canonical-map mutation isolation | An authorizer mutation of its received map does not alter the typed input observed by the handler or a later invocation. |
| E09 | Authorization outcomes | `nil` allows; wrapped `authz.ErrDenied` maps to `ErrPermissionDenied`; other errors and authorizer panic map to `ErrPolicyFailure`; denied/failed calls never dispatch. |
| E10 | Handler outcomes | Handler success converts the exact registered output; handler error maps to `ErrCapabilityFailure`; handler panic maps to `ErrInternal`; malformed or non-finite registered output maps to capability failure. |
| E11 | Final-value matrix | `None`, bool, string, signed 64-bit int, finite float, tuple, list, and string-keyed dictionary convert as documented. Non-string dictionary keys, out-of-range integers, non-finite floats, functions, and other unsupported values classify as invalid program. |
| E12 | Final-only output | Printed text, globals, intermediate expressions, and unused native results are absent; root result contains only the converted final value. |
| E13 | Fresh state | A second execute cannot observe globals or mutable Starlark state from the first, and budgets restart. |
| E14 | Resource limits | Source bytes, steps, elapsed time, native calls, converted depth, and encoded result each succeed below/at the boundary and classify as `ErrResourceLimit` above it. Use tight positive limits to avoid expensive cases. |
| E15 | Request cancellation | Pre-canceled and cooperatively canceled executions return `context.Canceled`; handler/authorizer observers show prompt cancellation handling. |
| E16 | Deadline classification | Root execution budget or request deadline classifies as `ErrResourceLimit` and wraps `context.DeadlineExceeded`. A blocking host-adapter demonstration is time-boxed in a child process and confirms only the documented limitation: CodeMode cannot forcibly stop non-cooperative Go code. |
| E17 | Sentinel coverage | Every exported root sentinel is observed through a public operation and matches through `errors.Is`; wrapped diagnostic text remains available only at the root Go API where documented. |

### `authz` and Rego

| ID | Manual scenario | Expected observation |
| --- | --- | --- |
| A01 | `authz.AllowAll()` with several valid subjects/calls | Every valid call is allowed; invalid binding still fails before authorization. |
| A02 | Subject and canonical argument data | Subject contains only the stable non-secret ID; canonical arguments contain only documented JSON-shaped values. |
| R01 | Valid total Boolean Rego policy | Trusted subject, stable ID, dotted name, supplied arguments, and omitted optional argument produce the documented allow/deny decisions. |
| R02 | Constructor validation | Nil/already-canceled context, no modules, blank filename, invalid Rego, non-ground or non-`data` decision, and unavailable builtin return constructor errors without a package sentinel. |
| R03 | Decision result matrix | Exactly one Boolean true allows; false returns `authz.ErrDenied`; undefined, non-Boolean, multi-result, evaluation failure, and strict builtin failure return ordinary policy errors. |
| R04 | Restricted evaluator behavior | Nondeterministic builtins and remote schema/network references are unavailable; print does not appear in host or client output. No claim of hard isolation is made. |
| R05 | Nil and cancellation behavior | Nil receiver and nil context return ordinary errors without panic; canceled/expired evaluation returns `ctx.Err()`. |
| R06 | Rego immutability and concurrency | After successful synchronous preparation, 32 goroutines perform 20 decisions each with stable results; replacing module strings afterward has no effect. |

### MCP adapter and wire contract

| ID | Manual scenario | Expected observation |
| --- | --- | --- |
| M01 | `mcpserver.New` dependency validation | Nil and typed-nil service/resolver classify as `ErrInvalidRegistration`. |
| M02 | Official `tools/list` | Exactly `search_api`, `describe_api`, and `execute` appear; each has one required string property and rejects additional properties. |
| M03 | Successful tool envelopes | Structured content has the documented unwrapped shape, and one JSON text item mirrors it. Empty search is `[]`, not `null`; execute envelope contains only `result`. |
| M04 | Malformed tool arguments | Missing required property, wrong type, and extra property are rejected by SDK schema validation before resolver/service counters increment. |
| M05 | Resolver ordering | Every well-formed call resolves a subject first. Resolver error, panic, or empty subject prevents service work and returns coarse `unauthenticated` or recovered `internal failure` as documented by the observed boundary. |
| M06 | Discovery semantics over MCP | Search normalization/sorting/limit and exact describe behavior match root operations; disabled capabilities remain absent. |
| M07 | Execute semantics over MCP | Namespaced calls, optional values, final-value conversion, final-only envelope, fresh state, and tight limits match root operations. |
| M08 | Coarse error projection | Produce every documented adapter text: unauthenticated, not found, invalid program, invalid arguments, permission denied, policy failure, resource limit, capability failure, canceled, bare deadline, and internal failure. `isError` is set. |
| M09 | Diagnostic non-disclosure | Resolver, policy, handler, panic, source, and argument canaries never appear in MCP structured content, text content, or agent transcript. |
| M10 | Identity spoof attempts | Subject-like tool properties are schema-rejected; subject strings in source, values, and MCP `_meta` cannot replace the resolver subject or alter authorization input. Use the raw SDK probe for `_meta`. |
| M11 | Static-filter no-backdoor check | Search, describe, and execute all fail to expose the disabled capability; observer logs show no resolver-independent dispatch path. |
| M12 | Transport ownership and teardown | The external host starts stdio, OMP connects through the official protocol, and terminating the one-shot OMP session permits clean host shutdown without CodeMode claiming listener/process ownership. |

## One-shot OMP assignments

Run six assignments. The first three use no expected tool or capability names in the prompt.

| Assignment | Fixture | Prompt objective | UX evidence |
| --- | --- | --- | --- |
| U01 Blind onboarding | `happy` | Discover what the server can do and complete a natural record lookup task. | Whether exactly three generic tools are discoverable, understandable, and sufficient without documentation. |
| U02 Multi-capability composition | `happy` | Find relevant APIs, inspect only what is needed, then write one Starlark program combining several native calls and return a nested result. | Search quality, signature clarity, description usefulness, namespace ergonomics, result predictability, unnecessary calls. |
| U03 Error recovery | `happy` | Complete a task after deliberately receiving one unknown-capability, one bad-argument, and one invalid-program response. | Whether coarse errors support recovery or force guesswork; agent retry quality. |
| U04 Filtering and spoofing | `filtered` and `deny` | Attempt to discover/call the disabled capability and bypass subject/argument policy through source or tool data. | Hidden-surface integrity, denial clarity, accidental leakage or bypass. |
| U05 Limits and isolation | `tight-limits` | Trigger each safe execution limit, then run a valid request and try to observe prior Starlark state. | Limit clarity, recovery after failure, fresh budgets/state. |
| U06 Rego outcomes | `rego` | Exercise allowed, denied, unmatched, and broken-decision inputs. | Whether policy outcomes map consistently and whether failures expose policy detail. |

Each prompt requires the agent to report:

- every MCP tool called and the intent, not hidden chain-of-thought
- observed success/error content
- whether the next action was obvious
- surprising schema, naming, signature, error, or result behavior
- the minimum reproduction for each suspected defect
- a final task-success verdict separate from a CodeMode-contract verdict

The operator compares the agent report with the raw OMP transcript and host observer log. Agent interpretation alone is not proof.

## UX review questions

For every case, answer only when direct evidence exists:

- Could a Go developer construct the server from public docs without reading repository code?
- Do compile-time errors identify the invalid registration clearly enough to fix it?
- Can a client distinguish search, exact description, and execution without guessing?
- Are generated signatures valid, compact, and sufficient to write a correct call?
- Does `describe_api` expose all information needed for supported inputs and outputs?
- Can an agent recover from each coarse error without unsafe trial-and-error?
- Do coarse errors prevent leakage while retaining enough actionability?
- Are stable IDs and dotted names used consistently across registration, filtering, policy, discovery, and execution?
- Does an optional `None` behave consistently in Starlark, typed Go input, canonical policy input, description, and JSON output?
- After failures, does the next independent request behave normally?
- Does the host-observed bind/authorize/dispatch order match the security promise?
- Does any result or error contain source, arguments, credentials, policy diagnostics, handler text, panic values, or stack details that should remain host-side?

## Evidence and finding format

Create one evidence folder per case or OMP assignment:

```text
evidence/<ID>/
├── command.txt
├── environment.txt
├── stdout.txt
├── stderr.txt
├── host-events.jsonl
├── agent-report.txt          # OMP assignments only
└── finding.md                # only when behavior is unexpected or poor UX
```

A finding contains:

- ID and severity: blocker, major, minor, or observation
- contract citation by document and heading
- candidate commit and environment
- exact external-harness reproduction
- expected and observed behavior
- raw evidence paths
- whether the issue reproduced twice
- classification: library defect, documentation mismatch, UX problem, OMP/client behavior, or harness defect
- smallest plausible product change; do not implement it during the campaign

Treat a contract mismatch as a defect even when the behavior seems reasonable. Treat a confusing but conforming result as UX evidence, not a contract failure.

## Completion criteria

The campaign is complete when:

- every matrix row has raw external evidence or an explicit blocked reason
- every public package has been imported and exercised by a standalone consumer
- all three MCP tools have been used through an official transport and by one-shot OMP agents
- every root sentinel and every documented MCP coarse error has been observed
- every configured limit has a boundary observation
- authorization order, dispatch suppression, canonical inputs, filtering, fresh state, and diagnostic non-disclosure have host-side evidence
- Rego constructor, decision, restriction, cancellation, and concurrent-use promises have external evidence
- all anomalies have one repeat run and one reduced reproduction
- the final report separates product defects, documentation defects, UX findings, OMP/client findings, and blocked coverage

Do not declare CodeMode functionally verified when any contract row is unobserved. A blocked row is an honest test result, not a pass.
