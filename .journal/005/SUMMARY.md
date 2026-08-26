---
id: 005
title: UX review and v0.1.0 release preparation
date: 2026-08-24
status: complete
repos_touched: [codemode]
related_sessions: [001, 002, 003]
---

## Goal

Critically review CodeMode's UX from a product-owner perspective — host-developer ergonomics, model-side power, safety, and learnability — then drive the resulting improvements and prepare the first public release.

## Outcome

Goal met and exceeded. The initial review found the product safe but not yet expressing the Code Mode thesis; by session end, every finding was implemented (three by concurrent agents from issues filed here), verified live end-to-end, and the repository was tagged `v0.1.0`.

Filed issues #23 (widen binding matrix), #24 (model-derived error diagnostics), #25 (first-touch ceremony), and #42 (fixed pure-compute stdlib); all four were implemented via PRs #34–#46 and closed. Verified each live by building the tutorial-shaped stdio server against local checkouts and driving it with a real MCP client: composite loop programs work, diagnostics echo positions and argument names while handler secrets stay hidden, the stdlib fills the Python-habit gap. This session directly authored and merged the README rework (PRs #47, #48), the release retarget (PR #49), and all four Dependabot merges (#1, #3, #4, #5). The user merged release PR #15; tag `v0.1.0` exists with a draft GitHub release pending manual publish.

## Key Decisions

- Review by functional smoke, not code reading -> built and drove the real server; caught the `sum` failure that became #42 and grounded #24 empirically.
- Binding fix targets the whitelist, not the translation -> handlers returning JSON text would strand models with unparseable strings (no `json` module then existed) and forfeit derived `describe_api` schemas; keep typed `Capability[Input, Output]`, widen what the types may contain.
- Error diagnostics echo only model-derived data (its own source positions, its own argument names) -> host-derived text stays non-disclosing; noted PR #22 had made that boundary a documented contract requiring deliberate amendment.
- Stdlib is always-on with no knob -> per-deployment opt-in creates language dialects; pure compute under existing budgets has nothing to opt out of; `lib/json`/`lib/math` ship in the already-pinned starlark-go revision (zero new dependencies).
- First-touch fixes are defaults, not hidden security -> explicit `authz.AllowAll()` stays as the one deliberate speed bump; `ServeWorkerAndExit` stays as the load-bearing worker entry.
- Release version fixed with `initial-version: "0.1.0"` rather than `release-as` -> the knob self-expires after the first release instead of pinning every future release until removed.
- README comparison with Cloudflare's Code Mode framed as same-thesis/opposite-sides -> agent-side TS-in-isolates wrapping existing servers vs server-side typed Go capabilities; no winner-framing.

## Changes

- `README.md` — rewrote around differentiators: plain-language intro, code-first Get started with inline comments, `v0.1.0` install, compressed security boundary, Cloudflare comparison placed above Security boundary (PRs #47, #48).
- `release-please-config.json` — added `"initial-version": "0.1.0"`; release-please regenerated PR #15 from 1.0.0 to 0.1.0 (PR #49).
- `.github/workflows/*`, `docs/uv.lock` — merged four verified Dependabot bumps (checkout 7.0.1, mise-action 4.2.5, cache 6.1.0, mkdocs-material 9.7.7); each action SHA independently checked against its upstream tag.
- Issues #23, #24, #25, #42 — authored with contracts, invariants, and acceptance sketches; implemented by concurrent agents (PRs #34–#46) and verified live here.
- Issue #24 comment — recorded the documented-contract tension from PR #22 and the missing language-surface documentation.

## Open Threads

- The `v0.1.0` GitHub release is a **draft** (`"draft": true` in config); the tag exists so `go get @v0.1.0` resolves, but the release needs manual publishing.
- #45 (open): `lib/json` encode/decode recursion is not step-budgeted; deep nesting can exhaust the Go stack — worker containment holds but the model sees a coarse internal failure. Scoped and owned.
- Composite capability *inputs* remain scalar-only (deferred phase 2 of #23); revisit when a real capability needs a list argument.
- `Limits` has grown to ten fields with two overlapping value budgets; acceptable while zero-defaulting hides it from first touch, worth watching.
- The gh token was refreshed with near-admin scopes (`workflow`, `admin:org`, ...) after workflow-file merges were blocked; the old `repo`-only token caused three failures that day.

## Lessons

- Reviewing by exercising the real artifact catches what reading cannot: the `sum()` failure, the bare error strings, and the search output shape change were all found by driving the server, not the source.
- Model-facing error taxonomies need a "model-derived vs host-derived" distinction from day one; retrofitting it required amending a documented non-disclosure contract, not just code.
- Coordinating concurrent agents through precisely-scoped issues works: contracts, invariants, and acceptance sketches written into #23/#24/#25/#42 were implemented faithfully without conversation access.
- Release Please defaults a first release to 1.0.0 regardless of `bump-minor-pre-major`; `initial-version` is the self-expiring fix.

## References

- [PR #47 — README rewrite](https://github.com/meigma/codemode/pull/47)
- [PR #48 — Cloudflare comparison](https://github.com/meigma/codemode/pull/48)
- [PR #49 — initial-version 0.1.0](https://github.com/meigma/codemode/pull/49)
- [Issue #23 — binding matrix](https://github.com/meigma/codemode/issues/23), [#24 — diagnostics](https://github.com/meigma/codemode/issues/24), [#25 — first-touch ceremony](https://github.com/meigma/codemode/issues/25), [#42 — stdlib](https://github.com/meigma/codemode/issues/42)
- [Issue #45 — JSON recursion hardening (open)](https://github.com/meigma/codemode/issues/45)
- [v0.1.0 draft release](https://github.com/meigma/codemode/releases/tag/v0.1.0)
- [Session 002 summary](../002/SUMMARY.md), [Session 003 summary](../003/SUMMARY.md)
