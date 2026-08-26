---
id: 013
title: Design bounded capability search
date: 2026-08-25
status: complete
repos_touched: [codemode]
related_sessions: []
---

## Goal

Determine whether `search_api` would scale to hundreds of capabilities, design a bounded replacement for literal substring discovery, and review the design for unnecessary complexity. After design approval, implement, document, validate, and land the replacement.

## Outcome

Goal met. The session measured the existing path, tested lexical retrieval approaches against a disposable 1,000-capability corpus, produced and complexity-reviewed a bounded architecture, and merged the implementation in PR #44. CodeMode now uses deterministic relevance-ranked token search with explicit discovery vocabulary, bounded object responses, and truncation signaling while retaining the three-tool MCP surface.

## Key Decisions

- Keep an immutable build-time search document per enabled capability and scan it for each query -> hundreds to low thousands of capabilities do not justify an external search service, mutable index, cache, heap, or pooling machinery.
- Rank normalized lexical tokens across exact names, name tokens, `SearchTerms`, summaries, and descriptions -> this handles model-generated task and resource language while preserving deterministic behavior.
- Make exact match quality dominate prefix quality within a field and use rarity only between equal-quality matches -> a rare prefix must not overturn the documented exact-match guarantee.
- Add `Capability.SearchTerms` as discovery-only registration metadata -> host authors can supply domain vocabulary that cannot be inferred reliably, while callers still use the exact registered name for `Describe` and execution.
- Treat `SearchTerms` as inferable public metadata -> they must not contain credentials, secrets, tenant identifiers, policy facts, or sensitive examples.
- Replace the bare search array with `SearchResponse{Results, Truncated}` and bump the MCP implementation version to `2` -> callers can distinguish a complete result set from a bounded prefix without a compatibility shim.
- Bound registration count, aggregate searchable metadata, raw query bytes, result count, and structured response bytes -> CPU and memory remain predictable and oversized catalogs fail during `Build` instead of becoming silently undiscoverable.
- Defer fuzzy matching and packed indexes -> the spike showed lexical ranking was sufficient to justify implementation, but not enough evidence for typo-tolerance complexity or a second indexing representation.

## Changes

- `capability.go`, `builder.go`, `server.go` - added `SearchTerms`, the public `SearchResponse`, registration ownership rules, and the clean search contract cutover.
- `internal/catalog/` - added immutable search-document compilation, normalized tokenization, deterministic relevance ranking, disabled-capability filtering, truncation, and catalog/search bounds.
- `mcpserver/` - changed `search_api` to a required non-null object output, normalized empty results, regenerated the service mock, and bumped the MCP implementation version.
- Root, catalog, and MCP tests - added ranking, normalization, limits, schema, truncation, empty-result, and real-transport coverage.
- `README.md`, `SECURITY.md`, and `docs/` - documented discovery vocabulary, ranking behavior, public and internal bounds, exact-name handoff, and sensitive-metadata restrictions.
- `.journal/013/SEARCH_ARCHITECTURE.md` - retained the reviewed design and deferred optimization criteria.

## Open Threads

- Add fuzzy matching only after representative agent queries show repeatable typo-related misses that token ranking and `SearchTerms` cannot address.
- Replace the direct scan or bounded result selection only if production profiling at materially larger catalog sizes shows it is a bottleneck.
- No known correctness, security, documentation, or validation failures remain from this session.

## Lessons

- The old linear scan was computationally acceptable; first-result alphabetical truncation and literal substring semantics were the actual scaling failures.
- Description indexing and explicit search vocabulary improved retrieval more than fuzzy matching in the spike.
- Ranking guarantees should be encoded as comparator precedence rather than assumed to emerge from multiplied score weights.

## References

- [PR #44: feat: add bounded relevance-ranked search](https://github.com/meigma/codemode/pull/44)
- `.journal/013/SEARCH_ARCHITECTURE.md`
- `.journal/013/NOTES.md`
