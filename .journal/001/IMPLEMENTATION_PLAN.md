# CodeMode implementation plan

## Goal and delivery shape

Build the smallest secure vertical slice first, then remove the remaining template machinery and converge the public documentation. The settled end state has six substantive Go packages:

```text
codemode
├── authz
├── mcpserver
└── internal
    ├── binding
    ├── catalog
    └── execution
```

The final request path is:

```text
official MCP client and supported transport
  -> search_api | describe_api | execute
  -> trusted InvocationResolver
  -> immutable filtered codemode.Server
  -> fresh restricted Starlark execution
  -> compiled keyword binder
  -> fail-closed authorization
  -> typed native handler
  -> final main() result only
```

Work in the increments below. An increment may merge independently when it produces tested behavior and does not expose an execution path that omits trusted resolution, static filtering, phase enforcement, authorization-before-dispatch, or safe result projection. Never merge a temporarily permissive production path.

## Increment 0 — disposable PRODUCT spike outside the repository

**Targets:** a temporary directory outside `/Users/josh/code/meigma/codemode`; no repository files.

Before renaming the module or changing repository code, build one disposable product loop with:

- an official MCP Go client and one official supported transport;
- exactly `search_api`, `describe_api`, and `execute`;
- a tiny hard-coded catalog;
- one manually bound namespaced Starlark function such as `records.lookup`;
- top-level loading followed by a zero-argument `main()`;
- an execute response containing only `main()`'s final value.

Use explicit `AllowAll` if useful. The spike proves mechanics only and makes no production security or isolation claim. It must not contain a generic operation-name builtin, dynamic tool forwarding, repository packages, or a reusable abstraction layer.

Exercise the loop through the official client, not by directly calling handlers. Record in the implementation PR notes:

1. the official client/server construction and transport APIs that actually compile;
2. whether server/connection context reaches tool handlers and how request context is derived;
3. how valid tool failures differ from malformed protocol input;
4. how structured results appear in `StructuredContent` and `Content`;
5. how Starlark thread locals, execution-step limits, cancellation, top-level loading, and the later `main()` call behave.

If the in-memory transport cannot carry trusted context, prove the same loop with the SDK's official Streamable HTTP client/server over `httptest.Server`, with middleware installing a typed context value. Do not invent a transport and do not accept identity in tool arguments or `_meta`.

After the probe, select and pin a reviewed current MCP SDK release and a reviewed compatible Starlark-Go revision in the production module. Do not copy an assumed dependency version from this plan. Encode durable conclusions in focused tests, then delete the entire temporary spike.

**Acceptance:** the three tools work through an official client; one program calls the namespaced function from `main()`; output contains only the final result; context and error semantics are known; the temporary directory is gone.

**Verification:** run the spike's executable and test once from its temporary directory, then confirm that directory and all spike files have been removed before repository cutover.

## Increment 1 — minimal repository cutover required for the MVP

**Dependency:** Increment 0.

**Remove now because the renamed library cannot retain the template application:**

- `cmd/template-go/main.go` and `cmd/template-go/`;
- `internal/cli/root.go`, `internal/cli/root_test.go`, and `internal/cli/`;
- `internal/config/config.go` and `internal/config/`;
- `internal/templateinfo/info.go`, `internal/templateinfo/info_test.go`, and `internal/templateinfo/`.

**Update now:**

- `go.mod`, `go.sum`: rename the module to `github.com/meigma/codemode`, remove Cobra/Viper, and pin only the reviewed MCP and Starlark releases selected after the spike;
- `.golangci.yml`: replace the template import prefix;
- `moon.yml`: include root `*.go`, `authz/**/*.go`, `mcpserver/**/*.go`, and `internal/**/*.go`; make `build` run `go build ./...` without a template binary output.

Do not yet rewrite release workflows, image files, docs, repository scripts, or all mise tooling. Change `.github/workflows/ci.yml` only if the existing `moon ci` invocation cannot run the MVP gate.

**Acceptance:** the old CLI packages are gone; the renamed empty/library module builds; no unreviewed dependency version is introduced.

**Verification:** `go mod tidy`, `go list ./...`, and `go build ./...`.

## Increment 2 — compile-time capability, authorization, and catalog contracts

**Dependency:** Increment 1. The `authz` and binder work are independent and may proceed in parallel; catalog construction follows the binder plan.

### 2.1 Public and authorization contracts

**Targets:**

- `doc.go`, `capability.go`, `limits.go`, `errors.go`;
- `authz/doc.go`, `authz/authz.go`, `authz/authz_test.go`;
- `.mockery.yaml`, `authz/mocks/authorizer.go` only if a generated port mock is needed.

Define `CapabilityID`, `CapabilityName`, generic `Capability[Input, Output]`, and typed handlers of the form `func(context.Context, authz.Subject, Input) (Output, error)`. Define `authz.Subject` with only a stable, non-secret ID and `AuthorizationInput` with subject, stable capability ID, model-facing name, and fresh canonical arguments.

`authz.Authorizer` is required. Provide an explicit `authz.AllowAll()`; nil never means allow. Keep denial distinguishable from policy failure without exposing trusted error text. Keep MCP, Starlark, credentials, headers, clients, source, and transport metadata out of `authz`.

Define positive limits for source bytes, Starlark steps, elapsed execution, attempted native calls, converted-value depth, final-result bytes, and bounded search input/results. Zero is invalid, not unlimited.

### 2.2 Restricted compiled binder

**Targets:**

- `internal/binding/doc.go`, `plan.go`, `input.go`, `output.go`, `signature.go`;
- `internal/binding/plan_test.go`, `input_test.go`, `output_test.go`, `signature_test.go`.

Compile each input/output type once during registration. For the MVP support only the types used by the representative capability: required `string`, optional `*int64`, and simple result structs. Reject unsupported kinds, pointers in the wrong place, ambiguous embedded fields, invalid or duplicate JSON names, and non-Starlark identifiers.

Use one immutable field plan to generate the keyword-only signature, bind a typed Go input, and build a new canonical JSON-shaped authorization map in one keyword traversal. Omitted and explicit `None` optional values both produce nil and omit the canonical key. Apply no defaults.

Reject positional, missing, duplicate, unknown, mistyped, and overflowing arguments before authorization. Convert handler outputs directly to Starlark. Separately convert only the final Starlark value to MCP-safe null, Boolean, string, signed integer, finite float, list, or string-keyed object values under depth and size limits; reject cycles and executable/unsupported values.

### 2.3 Immutable statically filtered catalog

**Targets:**

- `internal/catalog/doc.go`, `build.go`, `catalog.go`, `search.go`;
- `internal/catalog/build_test.go`, `catalog_test.go`, `search_test.go`.

Validate all registrations before filtering: non-empty stable IDs, dotted names, metadata, handlers, duplicate IDs/names, and function/namespace collisions. Apply `Options.DisabledCapabilities` once at build time and reject unknown disabled IDs.

Copy retained metadata, compile binding plans, sort one enabled slice by name, and build exact lookup plus immutable namespace data from that same enabled set. A disabled capability must be absent from search, description, namespace construction, and dispatch. Use deterministic case-normalized name/summary substring search; add no search interface or speculative index.

**Acceptance:** malformed binding never reaches policy or handlers; signature, typed input, and canonical authorization arguments cannot drift; disabled entries have no live runtime representation; AllowAll remains explicit.

**Verification:**

```sh
go test ./authz ./internal/binding ./internal/catalog -count=1
go test -race ./authz ./internal/binding ./internal/catalog -count=1
go build ./...
```

## Increment 3 — immutable server and secure execution core

**Dependency:** Increment 2. This increment must land as a complete secure native execution path, not as an exported bypass around authorization.

**Targets:**

- `builder.go`, `server.go`, and focused `builder_test.go`, `server_test.go`, `limits_test.go`;
- `internal/execution/doc.go`, `execute.go`, `phase.go`, `limits.go`;
- `internal/execution/execute_test.go`, `phase_test.go`, `limits_test.go`.

Implement a mutable, single-threaded, one-shot `Builder`; generic `Register` is a function because Go methods cannot add type parameters. Compile binding before handler erasure. `Build` validates authorizer, limits, registrations, and filtering, copies retained state, and returns an immutable concurrency-safe `Server` with concrete search, describe, and execute methods.

For every execute call:

1. reject oversized source and derive the elapsed deadline;
2. create a fresh `starlark.Thread`, counters, thread-local phase, and step limit;
3. predeclare only immutable namespaces/functions from the enabled catalog;
4. reject `load`, discard `print`, and expose no filesystem, environment, network, client, or dynamic dispatch builtin;
5. evaluate top-level code in `loading`;
6. require one callable zero-argument `main()`;
7. switch to `running`, call `main()`, then make the execution unusable;
8. convert and bound only `main()`'s final value.

Every generated builtin must perform this order: running-phase check; attempted-call charge; positional rejection; keyword binding; canonical argument capture; fail-closed authorizer call with trusted subject and stable identity; dispatch only on allow; output conversion. Denial, policy error, cancellation, deadline, and authorizer panic all produce zero handler calls. Recover authorizer, handler, and interpreter panics into coarse safe errors.

Use the original execution context synchronously for authorizer and handler. Wire cancellation to `Thread.Cancel` without leaking a watcher. Do not wrap native calls in detached timeout goroutines, retry them, or claim blocked Go code has been interrupted.

Focused behavior tests must cover phase guarding, fresh globals/counters, load/print restriction, argument failures before authorization, authorization event order, denial with zero handler calls, authorizer/handler panic and error projection, cancellation, limits, final-value conversion, and concurrent calls. Split these by behavior; do not create one test that asserts the entire architecture.

**Acceptance:** top-level native calls cannot run; every callable native path authorizes before dispatch; denial is side-effect free at the handler boundary; executions share no mutable interpreter state; only the final result escapes.

**Verification:**

```sh
go test ./internal/execution ./... -run 'Phase|Authorization|Denied|Fresh|Limits|FinalResult' -count=1
go test -race ./... -count=1
go build ./...
```

## Increment 4 — exactly three MCP tools and focused actual-MCP evidence

**Dependency:** Increment 3.

**Targets:**

- `mcpserver/doc.go`, `mcpserver/service.go`, `resolver.go`, `server.go`;
- `mcpserver/server_test.go`, `mcpserver/resolver_test.go`, `mcpserver/e2e_test.go`;
- `mcpserver/mocks/invocation_resolver.go`, `mcpserver/mocks/service.go` only when generated mocks are justified.

Define the adapter-owned narrow `Service` interface implemented by `*codemode.Server`. Define required `InvocationResolver.Resolve(context.Context) (authz.Subject, error)`. It reads only typed trusted context and returns no credential material. `mcpserver.New` rejects nil service/resolver and returns an official SDK server with exactly `search_api`, `describe_api`, and `execute`.

Resolve the subject before all three operations. Search and description remain deployment-visible after authentication; native calls alone are authorizer-controlled. Tool input contains only bounded query, exact capability name, or Starlark source. Identity, credential, timeout, budgets, modules, and allow-lists are not tool arguments, and `_meta` cannot affect resolution.

Map malformed envelopes through SDK validation. Map valid domain failures to `IsError` with stable coarse text. The logical execute payload is exactly `{"result": finalValue}`; if the SDK mirrors it into text content, both representations carry only that payload. Recover and sanitize at the adapter boundary.

Create one compact actual-MCP smoke scenario, `TestActualMCPSecureLoop`, using the official client and transport proven by Increment 0. Register:

- enabled ID `records.entry.lookup`, name `records.lookup`, with `key string`, `limit *int64`, and a deterministic typed result;
- disabled ID `records.entry.hidden`, name `records.hidden`, filtered through `Options.DisabledCapabilities`.

Put a unique credential canary and typed subject only in trusted server context. The real enabled handler records subject, input, call count, and whether it can read the canary. Through the official client, prove: exactly three tool names; enabled discovery and hidden absence; one allowed namespaced call from `main()`; final-result-only output; trusted subject use; canary absence from every serialized result; and one valid denied call that increments authorization but leaves the handler count unchanged. Include misleading `_meta` on the denied call and prove it has no effect.

Keep argument matrices, namespace collisions, filtering across individual surfaces, resolver failures, safe handler errors, phase cases, limits, and concurrency in focused package tests. The actual-MCP smoke is evidence of integration semantics, not a mega test for every branch.

**Acceptance:** an official client exercises the complete secure loop; exactly three tools exist; trusted resolution precedes service work; static filtering is visible over MCP; denial dispatches nothing; no canary or intermediate value crosses MCP.

**Verification:**

```sh
go test ./mcpserver -run '^TestActualMCPSecureLoop$' -count=1
go test -race ./mcpserver -run '^TestActualMCPSecureLoop$' -count=1
go test -race ./... -count=1
go build ./...
```

The secure MVP is complete here. Unit-only or direct adapter calls are not substitutes for the actual-MCP smoke.

## Increment 5 — immediate remaining template cutover

**Dependency:** the Increment 4 smoke is green. This cleanup follows the MVP; it does not block discovering the product contract.

**Remove:** `.goreleaser.yaml`, `melange.yaml`, `apko.yaml`, `.github/workflows/release.yml`, `.github/workflows/security-scan.yml`, and `DELETE_ME.md` after resolving surviving checklist items.

**Update:**

- `mise.toml`, `mise.lock`: remove GoReleaser, Syft, Cosign, Melange, apko, and `image-local`; add and lock Mockery only if generated mocks remain;
- `.gitignore`: remove obsolete binary/image artifacts only;
- `moon.yml`: remove release/image inputs, add focused MCP and race tasks, and include them in `check`;
- `.github/workflows/ci.yml`: preserve the hardened Moon entrypoint and ensure the MVP smoke/race tasks run;
- `release-please-config.json`: rename the source-only package to `codemode`; keep `.release-please-manifest.json` and `.github/workflows/release-please.yml` only for source/version automation;
- `.github/scripts/configure_github_repo.py` and `test_configure_github_repo.py`: either update active CodeMode fixtures or remove completed bootstrap tooling;
- `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `docs/mkdocs.yml`, `docs/moon.yml`, `docs/pyproject.toml`, `docs/uv.lock`, `docs/docs/index.md`: remove template CLI/image/bootstrap claims and describe only runnable MVP behavior.

Do not add a replacement `cmd/`; the consuming host owns transport startup, authentication middleware, listeners, and shutdown.

**Acceptance:** no template application, binary/image publication path, or stale branding remains; source-only release automation is internally consistent; normal build/check commands exercise the library and actual-MCP smoke.

**Verification:**

```sh
go mod tidy
go list ./...
go test -race ./... -count=1
go build ./...
moon run root:check
moon ci --summary minimal
```

Confirm the package list contains only the six substantive packages plus generated mock subpackages, with no `cmd`, `internal/cli`, `internal/config`, or `internal/templateinfo` package and no Cobra/Viper dependency.

## Increment 6 — concise public convergence

**Dependency:** the secure MVP and cutover are green.

**Targets:** `example_test.go`, `mcpserver/example_test.go`, `README.md`, `docs/docs/tutorials/first-server.md`, `docs/docs/how-to/disable-capabilities.md`, `docs/docs/reference/public-api.md`, `docs/docs/reference/mcp-tools.md`, `docs/docs/explanation/security-model.md`, `docs/mkdocs.yml`, and `CONTRIBUTING.md`.

Add compile-checked registration and official-transport examples. Document only the shipped type matrix, three tool contracts, explicit AllowAll, trusted resolver boundary, static filtering, authorization order, final-result-only behavior, cancellation limits, and honest in-process containment. Make one clean pre-release naming cutover; add no aliases.

**Acceptance:** a consumer can copy the examples, register a typed capability, construct the adapter, and connect an official transport without undocumented internals or credentials in model-visible values.

**Verification:** `go test ./... -count=1`, `moon run docs:build`, and `moon run root:check`.

## Later evidence gates

- Extend binder types only with a real consuming capability; update `internal/binding/{plan,input,output,signature}.go` and focused tests together.
- Change search only after representative discovery failures; keep one filtered catalog source of truth.
- Add Rego, subject-specific discovery, admission control, structured safe handler errors, observability, fuzzing, benchmarks, or Testcontainers only when a named deployment contract requires them.
- Measure large Starlark workloads, cancellation, heap/RSS, and goroutine stability before claiming hostile multi-tenant containment. If hard memory or tenant isolation is required, stop for a worker-process architecture decision rather than growing `internal/execution` speculatively.
- After one real consuming application, rehearse consumption from a clean external module, review exported compatibility/Godoc, and make the last clean cutover before the first stable tag.

The full architecture remains the six-package library above: no first-party binary, downstream MCP proxy, capability-specific MCP tools, positional arguments, Starlark `load`, dynamic operation selection, hot registration, automatic retries, generic HTTP/SQL/shell/filesystem/environment access, public codec hierarchy, or speculative worker/observability framework.

## Risks and decision points

1. **SDK semantics:** use the probe's observed APIs and result representation; never assume a release number or duplicate mirrored structured/text output.
2. **Trusted context:** if in-memory transport drops connection context, use official Streamable HTTP plus middleware; never read identity from source, arguments, or `_meta`.
3. **Authorization drift:** typed input, signature, and canonical policy arguments must come from one compiled plan with no post-authorization defaults or decoding.
4. **Starlark cancellation:** cancellation can be delayed inside blocked Go handlers; synchronous context-aware handlers are mandatory, and errors must not imply rollback.
5. **Credential leakage:** framework projection must suppress trusted causes and pass the canary tests, while documentation must state that a malicious handler can still return a secret deliberately.
6. **Namespace collisions:** validate the complete registration set before filtering so disabled entries cannot hide invalid configuration.
7. **Containment:** step/time/depth/result limits are not a heap boundary; make no production isolation claim without measurement and a separate architecture decision.
8. **Ports and mocks:** keep only `Authorizer`, `InvocationResolver`, and adapter-owned `Service`; generate mocks when used and avoid Runner, Invoker, registry, codec, or filter interfaces.