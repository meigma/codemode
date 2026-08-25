# Security Policy

## Supported versions

CodeMode has no released version yet. Security fixes target the `master` branch until the first release establishes a versioned support policy.

## Report a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/meigma/codemode/security/advisories/new) to report a suspected vulnerability. Do not open a public issue, pull request, or discussion, and do not disclose the report in another public channel.

Include the following details when available:

- the affected commit
- the affected public package or MCP tool
- the security impact and the conditions required to trigger it
- reproduction steps or a minimal proof of concept
- whether the issue crosses the host context, authorization, capability, Starlark, or MCP output boundary
- relevant logs or traces with credentials and other secrets removed
- a suggested mitigation or fix, if known

## Execution containment

CodeMode executes each submitted program in a fresh worker process created by
re-executing the host binary. Module loading is disabled, native capabilities
are limited to the immutable set registered and enabled by the host, and native
calls are rejected during top-level source loading. A program must define a
zero-argument `main()` function, and only its converted return value is
exposed.

Configured limits bound source bytes, interpreter steps, elapsed time,
attempted native calls, concurrent workers, and the depth and encoded
size of every value that crosses the worker boundary. Search query bytes and
search result counts are bounded separately in the parent. An elapsed deadline
or request cancellation kills and reaps the worker. Each native call whose
arguments bind successfully is rebound in the parent and passes through the
host-supplied `authz.Authorizer` before its handler runs. The MCP adapter
resolves the subject from trusted Go context and ignores identity or
credential claims in tool arguments and request `_meta`.

The worker boundary prevents Starlark from sharing interpreter state with the
host and permits hard preemption of Starlark execution. It is not a hard tenant
or memory boundary: workers run as the host operating-system user and
re-execute the host binary. CodeMode gives them an environment containing only
its private worker marker and passes no extra file descriptors, but it provides
no operating-system CPU or memory quota. Package initialization still runs
before the worker entry point with that user's filesystem and network
authority.

The authorizer, optional Rego evaluator, and capability handlers run in the
parent with the host process's privileges. CodeMode returns after cancellation
without waiting for dispatched Go code, but it cannot forcibly stop that code
or undo its side effects. Host implementations must honor context cancellation
and add container, process, or workload controls when their threat model
requires stronger isolation.
