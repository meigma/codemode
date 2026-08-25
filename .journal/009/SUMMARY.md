---
id: 009
title: Post-refactor documentation review
date: 2026-08-25
status: complete
repos_touched: [codemode]
related_sessions: [008, 010, 011]
---

## Goal

Independently review all user-facing documentation for technical accuracy after session 008's worker-isolation refactor, verify language-style conformance against the `language-style` skill and Diátaxis conformance per page, and land the approved fixes. A second pass was requested mid-session after sessions 010/011 merged eight more PRs (#33–#40).

## Outcome

Goal met across two merged PRs. Pass one (PR #32, merged as `906541d`) reviewed ten docs with three parallel read-only reviewers, found 45 findings, and fixed the maintainer-approved scope: all 9 HIGH accuracy findings, all 20 style findings, and the cheap Diátaxis fixes. Pass two (PR #41, merged as `a5031b5`) re-reviewed after #33–#40, added functional reproduction to the review wave itself, and fixed 1 blocking HIGH plus accuracy, style, and Diátaxis regressions in the rewritten pages.

The dominant pass-one errors were systemic: four docs claimed only the final value crosses the worker boundary (native arguments and results cross the private worker protocol; only `main`'s final converted value is *exposed to the caller*), and three docs claimed `MaxExecutionTime` includes worker cleanup (kill/reap overhead is outside the budget per `limits.go`). The pass-two blocking defect was procedural: PR #34's tutorial rewrite deleted the in-process client that the Rego how-to's verification section patched, orphaning the entire procedure while its documented outcomes stayed correct.

Every executable claim in the final docs was reproduced verbatim by driving the built stdio binaries with a real MCP client: the tutorial result, the Rego allowed/denied outcomes (including the embed variant), and all three disable-capabilities checks.

## Key Decisions

- Review with three independent axes (accuracy vs source, language-style, Diátaxis) run by separate read-only agents -> converging findings carry more weight, and each axis has a different ground truth.
- Maintainer chose the minimal improvement scope both passes: fix accuracy/style plus cheap Diátaxis; reject new pages, tutorial restructure or voice change, reference-content relocation, and README/SECURITY shortening -> structural moves stayed aligned with fresh product intent (#34) instead of fighting it.
- Add functional reproduction to the review wave in pass two -> pass one proved source-grounded review insufficient: the Rego guide was accuracy-clean on paper yet unreproducible in practice (variable shadowing, unobservable denial text, missing tidy step).
- Show literal `tools/list` wire schemas in `mcp-tools.md` instead of weakening the "advertised values" claim -> the wire-contract test pins the shape, so literal values are stable and the reference stays a true lookup source.
- Canonicalize terminology once ("worker process" then "worker"; "native call"; "top-level source loading") and re-check it as a regression axis in pass two -> cross-doc drift returned within one merge cycle and was caught cheaply.

## Changes

- `docs/docs/index.md` - reworked into a pure Diátaxis orientation page routing all six pages; stale budget claims removed with the old reference body.
- `docs/docs/tutorials/first-server.md` - boundary-exposure correction, prerequisites-first structure, single-user/AllowAll conditions before the program, `go get` prose fix, stdio-binary note, terminology.
- `docs/docs/reference/public-api.md` - probe-detection and `MaxConcurrentExecutions` scope corrections, elapsed-budget phases, descriptive reference voice for builder lifecycle and filtering.
- `docs/docs/reference/mcp-tools.md` - literal wire schemas (inlined field shapes, `"result": true`), composite example root-shape fix (`items` field), identifier formatting, concrete recovery quantities, hedge and jargon removal.
- `docs/docs/explanation/security-model.md` - exposure and budget corrections, retitled "Understanding CodeMode's security model", terminology.
- `docs/docs/how-to/use-rego-authorization.md` - reproducibility fixes (err shadowing, tidy step, const scope, denial-text unwrap), then a full verification rewrite against the stdio tutorial after #34; full `Register` literal; `source` argument named.
- `docs/docs/how-to/disable-capabilities.md` - condition-first AllowAll framing, numbered executable verification with exact inputs and expected tool errors.
- `README.md`, `SECURITY.md`, `CONTRIBUTING.md` - exposure claim, conditions before snippets, `MaxIntermediateValueBytes` in the limit inventory, canonical exposure sentence, one-action steps, missing Rego how-to link.

## Open Threads

- Deliberately unaddressed Diátaxis findings (maintainer-rejected scope): the tutorial's agent-agnostic ending is not a fully controlled learning path; `mcp-tools.md` retains its embedded recovery section; README retains the assembly snippet duplicating the tutorial's program; tutorial explanation paragraphs remain in place. Revisit only if reader friction is observed.
- The `chore/comment-cleanup` worktree existed at close and belongs to other work; untouched.

## References

- [PR #32 — docs: correct worker-boundary claims and align style](https://github.com/meigma/codemode/pull/32) (merged `906541d`)
- [PR #41 — docs: repair procedures orphaned by API ceremony rewrite](https://github.com/meigma/codemode/pull/41) (merged `a5031b5`)
- [Session 008 summary](../008/SUMMARY.md) — the refactor under review
- Sessions 010 and 011 — the #33–#40 merges that triggered pass two

## Lessons

- Source-grounded accuracy review does not prove documentation: the Rego guide passed accuracy review while being unreproducible as a procedure. Docs that publish exact expected outputs must be executed end to end, reader-style, from the doc text alone.
- A code PR that rewrites one doc page silently orphans sibling pages whose procedures patch its code. After any tutorial change, functionally re-verify every how-to that starts "from the tutorial".
- The same wrong claim replicates across docs ("only the final value crosses the boundary" appeared in four files). Fixing systemic claims requires a canonical sentence and a sweep, not per-file edits.
- Terminology and hedge discipline regress within one merge cycle when code PRs carry doc updates; a cheap conformance re-check per batch keeps drift bounded.
