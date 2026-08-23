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

## In-process containment

CodeMode executes each submitted program in a fresh in-process Starlark interpreter. Module loading is disabled, native capabilities are limited to the immutable set registered and enabled by the host, and native calls are rejected during top-level program loading. A program must define a zero-argument `main()` function, and only its converted return value is exposed.

Configured limits bound source bytes, interpreter steps, elapsed time, attempted native calls, converted-value depth, result bytes, search query bytes, and search results. Each validated native call passes through the host-supplied `authz.Authorizer` before its handler runs. The MCP adapter resolves the subject from trusted Go context and ignores identity or credential claims in tool arguments and request `_meta`.

These controls restrict Starlark evaluation inside the host process. They do not form a hard tenant boundary. The authorizer and capability handlers are ordinary Go code with the host process's privileges. CodeMode can cancel Starlark evaluation when a request or elapsed deadline ends, but it cannot forcibly interrupt a blocking Go authorizer or handler. Host implementations must honor context cancellation and add process or workload isolation when their threat model requires it.
