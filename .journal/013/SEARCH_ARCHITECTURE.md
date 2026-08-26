# Architecture: Bounded Relevance-Ranked Capability Search

## Status

Proposed architecture for review. It replaces the current literal-substring discovery contract with bounded relevance-ranked lexical retrieval. Numeric ceilings marked **provisional** must be checked against representative host catalogs before implementation freezes them.

This document incorporates the search spike and a subsequent complexity review. The complexity review preserved the product direction but removed an unprofiled packed posting index, token arenas, `sync.Pool`, a top-K heap, multiple overlapping memory budgets, giant score boosts, and premature fuzzy machinery.

## Decision

CodeMode will compile one immutable search document per enabled capability and perform one deterministic, bounded full-catalog scan for each query.

The query pipeline will:

1. enforce raw query and token-count bounds;
2. normalize and tokenize the query with the same code used at build time;
3. compare distinct query tokens with weighted name, explicit search-term, summary, and description tokens;
4. rank exact tokens above prefixes and rare tokens above catalog-wide terms;
5. reject candidates that match too little of the query;
6. score every enabled capability before applying result limits;
7. sort eligible candidates by exact-name equality, score, and exact dotted name;
8. pack the highest-ranked prefix under count and response-byte bounds; and
9. return `{results, truncated}`.

The first production increment will not include fuzzy matching. The spike reached 96.6% top-1 before fuzzy matching and 97.7% after it. Typo tolerance is a separate increment with its own quality and precision gate.

The design introduces no external service, embeddings, mutable index, query language, search plugins, configurable ranking policy, or general-purpose search package.

## Product goal

An agent should identify the intended capability with one `search_api` call whenever registration metadata contains the task vocabulary.

This matters more than catalog CPU time. The minimum successful workflow already requires:

1. `search_api`;
2. `describe_api`; and
3. `execute`.

Every failed discovery query adds another model inference, MCP round trip, response payload, and delay.

## Evidence

A disposable Go spike at `/tmp/codemode-search-spike` compared the current search with progressively richer lexical retrieval. It used 21 hand-written capabilities, generated distractors up to 1,000 capabilities, 66 task paraphrases, 21 known literal fragments, unrelated-query checks, typo cases, and model-emitted query samples.

| Variant | Top-1 | Empty results |
|---|---:|---:|
| Current literal substring | 24.1% | 75.9% |
| Ranked name and summary | 67.8% | 32.2% |
| Add description | 90.8% | 9.2% |
| Add explicit search terms | 96.6% | 3.4% |
| Add bounded fuzzy matching | 97.7% | 2.3% |

At 1,000 capabilities:

| Query cohort | Current top-1 | Prototype top-1 |
|---|---:|---:|
| Single-term model queries | 95.2% | 100% |
| Compound model queries | 9.5% | 100% |
| Task paraphrases | 0% | 97.0% |
| Known literal fragments | 100% | 100% |

The query `sql` exposed a concrete current-search defect. Literal infix matching treated `sql` as part of `mysql`, so alphabetically early MySQL capabilities filled the capped result set before `snowflake.queries.execute`. Token matching ranked the Snowflake capability first.

The unoptimized final spike measured about 515 microseconds/op, 32.5 KiB/op, and 514 allocations/op at 1,000 capabilities. That establishes feasibility, not a production performance target. The allocation profile came from request-local maps and repeated fuzzy expansion. The production design removes those structures before considering packed indexes or pooling.

The spike is not proof of a universal 97.7% retrieval rate. Its generated distractors did not adequately represent catalogs containing many closely related operations. The fixture is a starting regression surface; real catalog and query replay must tune the policy.

## Goals

- Rank compound task/resource queries without requiring contiguous substrings.
- Preserve exact dotted names as the only names accepted by `describe_api` and Starlark execution.
- Prevent token-boundary errors such as `sql` matching `mysql`.
- Search explicit host vocabulary and low-weight descriptions without returning either in compact results.
- Preserve deterministic output, immutable concurrency safety, static filtering, and coarse errors.
- Bound registered catalog size, searchable metadata, query work, candidate memory, result count, and structured response bytes.
- Support hundreds to low thousands of capabilities without an operational search dependency.
- Keep ranking policy internal and corpus-tested.

## Non-goals

- Semantic retrieval that invents synonyms absent from host metadata.
- Pagination, cursors, facets, field filters, query operators, or a query language.
- Returning descriptions, search terms, match explanations, scores, edit distances, or total candidate counts.
- Embeddings, vector databases, LLM calls during search, external search services, or heavyweight search libraries.
- Runtime registration, mutable indexes, background indexing, or search-specific admission control.
- Subject-specific search ranking or authorization-aware discovery.
- Fuzzy or normalized `describe_api` behavior.
- A reusable search framework, tokenizer interface, scorer interface, plugin surface, or ranking DSL.

## Preserved current invariants

The architecture keeps these repository contracts:

1. **Registration remains the source of truth.** `Capability` metadata continues to drive discovery, description, binding, authorization identity, and dispatch (`capability.go`, `builder.go`).
2. **All registrations are validated before static filtering.** Disabled IDs cannot hide malformed contracts (`internal/catalog/build.go`, `Build`).
3. **Filtering precedes live search compilation.** Disabled capabilities contribute no searchable tokens, document frequency, ranking, truncation, description, namespace, or execution binding.
4. **The catalog remains immutable.** `Build` produces owned slices; search uses request-owned scratch only (`internal/catalog/catalog.go`, `Catalog`).
5. **Describe remains exact.** `Catalog.Lookup` continues to use the exact `byName` map. Search terms, case changes, whitespace, prefixes, and typos never resolve through `Describe`.
6. **Discovery remains deployment-visible.** `mcpserver.InvocationResolver` authenticates search calls, but search does not call the per-capability authorizer or vary by subject.
7. **No request-time reflection.** Signatures, shapes, result records, and search documents are compiled during registration/build.
8. **The MCP surface remains exactly three tools.** The `search_api` schema changes; no fourth tool or versioned alias is added (`mcpserver/server.go`, `New`).
9. **Errors remain coarse and non-disclosing.** Query resource failures map to `ErrResourceLimit`; unexpected failures map to `ErrInternal`.

## Public API cutover

### Explicit search vocabulary

Add one optional field:

```go
type Capability[Input, Output any] struct {
    ID          CapabilityID
    Name        CapabilityName
    Summary     string
    Description string

    // SearchTerms contains alternative task vocabulary used only for discovery.
    // Terms are not callable aliases and are not accepted by Describe or Execute.
    SearchTerms []string

    Handler Handler[Input, Output]
}
```

`Register` clones `SearchTerms` before storing it in `catalog.Registration`. A caller mutation after registration cannot change the eventual catalog.

Each entry is a short phrase. Appropriate examples include:

```go
SearchTerms: []string{
    "open ticket",
    "file bug report",
    "refund customer",
}
```

Search terms must not contain secrets, policy facts, credentials, tenant identifiers, or sensitive examples. They are not returned, but callers can infer searchable vocabulary by probing results.

### Search response

Make a clean breaking cutover:

```go
type SearchResponse struct {
    Results   []SearchResult `json:"results"`
    Truncated bool           `json:"truncated"`
}

func (server *Server) Search(query string) (SearchResponse, error)
```

`Results` is always non-nil on success. `SearchResult` remains exactly:

```json
{
  "name": "github.issues.create",
  "signature": "github.issues.create(*, ...)",
  "summary": "Create a repository issue"
}
```

`Truncated` is true when at least one eligible result is omitted by `MaxSearchResults` or the internal structured-response byte cap. It does not expose a total count and does not imply pagination.

Update `mcpserver.Service.Search`, generated mocks, `adapter.search`, schemas, examples, tests, and documentation together. Increment the MCP implementation version from `"1"` to `"2"`. Do not retain the old array response, add `search_api_v2`, or select behavior from client metadata.

The tool description should state that search accepts task, resource, or exact-name vocabulary; results are relevance-ranked; and the exact returned name belongs in `describe_api`. If `truncated` is true and no result fits, the agent should submit a more specific task/resource query.

## Internal architecture

Keep the implementation in `internal/catalog`. Do not create a generic search package.

### Owned immutable documents

After validation, static filtering, and exact-name sorting, build one document aligned with each entry in `Catalog.enabled`:

```go
type searchIndex struct {
    documents []searchDocument
}

type searchDocument struct {
    normalizedName string

    nameTokens        []searchToken
    searchTermTokens  []searchToken
    summaryTokens     []searchToken
    descriptionTokens []searchToken

    resultJSONBytes int
}

type searchToken struct {
    text string
    idf  uint8
}
```

The exact small integer type for `idf` is an implementation detail; ordinary `int` is also acceptable if it produces clearer checked arithmetic.

Within each field, retain each normalized token once for contribution scoring. The same token may remain in multiple fields because field precedence matters. No ordered phrase arena is needed in the first increment because general phrase boosts are deferred.

The document stores no handler, authorization, or binding state. It is aligned by index with the existing enabled entry, so result projection reads the existing name, signature, and summary.

### Build-time compilation

`catalog.Build` performs these search steps after static filtering:

1. Verify the finite supported registration count and aggregate searchable metadata budget.
2. Normalize and tokenize each enabled name, search term, summary, and description.
3. Enforce the per-capability `SearchTerms` shape and the compiled token-work bound.
4. Deduplicate tokens within each field.
5. Compute document frequencies with build-only maps.
6. Assign each retained token a small monotone integer document-frequency factor.
7. Marshal each immutable `SearchResult` with the standard compact `encoding/json` behavior and retain its byte length.
8. Verify every individual result can fit within the structured-response cap.
9. Discard build-only maps and return the immutable owned slices.

Map iteration order never reaches retained state or query results. Enabled entries remain sorted by exact dotted name.

No packed posting representation, token arena, custom serializer, scratch pool, or mutable cache is part of the first implementation.

## Normalization and tokenization

Build and query paths call the same unexported tokenizer.

It will:

- check raw query bytes before normalization;
- trim surrounding Unicode whitespace;
- lowercase deterministically;
- treat punctuation and whitespace as separators;
- split dotted names, underscores, hyphens, camel-case transitions, acronym-to-word transitions, and letter/number transitions;
- discard empty tokens;
- discard a small fixed internal set of grammatical connector tokens validated by the fixture, such as articles, conjunctions, and prepositions;
- deduplicate query tokens for scoring; and
- preserve no caller-owned memory.

Examples:

```text
GitHub.Pulls.createReview → github, pulls, create, review
pull_request              → pull, request
sql                       → sql
mysql                     → mysql
```

There is no stemming or built-in synonym table. Product abbreviations and synonyms belong in `SearchTerms` or descriptive metadata.

A blank or separator/connective-only query returns:

```json
{"results": [], "truncated": false}
```

## First-increment matching and ranking

### Token matching

For each document and each distinct query token, choose only the best contribution across all fields and document tokens.

Match-quality precedence:

1. exact token;
2. token prefix, when the query token meets a corpus-selected minimum length.

Prefix matching compares the query token directly with the current document's bounded token slices. It is never disabled because a global vocabulary grew. Arbitrary infix token matching is not supported.

Field precedence:

1. capability name;
2. explicit `SearchTerms`;
3. summary;
4. description.

Each query token contributes at most once per document. Repeating a word in metadata cannot stuff the score.

### Document-frequency factor

Build computes a bounded monotone integer factor from enabled-document frequency. Rare terms contribute more than catalog-wide terms. The first implementation needs only a small range; exact numeric weights are internal policy selected by fixture replay, not architecture or public configuration.

The factor must be deterministic and integer-based. It does not need logarithms, floating-point scores, BM25 document-length normalization, or host tuning.

### Eligibility and coverage

A document is eligible only when it matches enough distinct query tokens:

```text
q = 1: match 1
q = 2: match 2
q ≥ 3: match ceil(2q / 3)
```

This rule comes from the spike's unrelated-query precision check. In particular, a two-token query such as `deploy rocket` must not return a deployment capability from one shared token.

Apply at most one linear coverage adjustment:

```text
finalScore = rawScore × matched / q
```

Do not combine strict eligibility with a second squared coverage penalty unless the fixture demonstrates a ranking defect that the linear rule fixes.

### Exact-name priority and deterministic ordering

Do not encode exact-name priority with a billion-point boost.

Sort eligible candidates by:

1. normalized query exactly equals the normalized dotted name: true first;
2. integer score: descending;
3. exact dotted capability name: ascending.

This makes the exact-name guarantee explicit and keeps score constants independent.

General name, search-term, summary, and description phrase boosts are deferred. Add one only if an ablation against production-shaped queries proves token coverage cannot rank a required phrase correctly.

## Candidate collection and response packing

Search scans every enabled document before limiting results.

For each eligible document, append one bounded candidate:

```go
type searchCandidate struct {
    document int
    score    uint64
    exact    bool
}
```

The supported catalog ceiling bounds this slice. Sort it with the comparator above. Do not introduce a top-K heap until a profile shows candidate sorting is material.

Walk the ranked candidates and append results until either:

- `MaxSearchResults` is reached; or
- the next compact structured result would exceed the internal response-byte cap.

Use the build-time `resultJSONBytes` values plus fixed envelope/comma accounting. Do not skip an oversized higher-ranked result in favor of a lower-ranked result. Build validation guarantees every allowed single result fits.

Set `Truncated` when `eligibleCount > len(Results)`.

The byte cap covers the compact structured `{results,truncated}` value. It does not claim to cover the MCP SDK's surrounding JSON-RPC envelope or the SDK's JSON text mirror.

Each request owns its query-token and candidate slices. Do not allocate a map or object per token/document match. Do not add `sync.Pool` initially. Pool only a measured hot allocation later, with an explicit retained-capacity limit.

## Bounds

The architecture deliberately uses a small independent bound set. It does not expose index layout or scoring as configuration.

| Quantity | Ownership and behavior |
|---|---|
| Raw query bytes | Keep public `MaxSearchQueryBytes`, default 256. Check before normalization. |
| Returned result count | Keep public `MaxSearchResults`, default 20. The finite supported catalog count already caps allocation even if a host configures a larger value. |
| Registered capabilities | One internal finite supported ceiling checked before proportional build allocation. **Initial candidate: 4,096 [provisional].** This supports the stated low-thousands target with headroom. |
| Aggregate searchable source metadata | One internal raw-byte budget over names, summaries, descriptions, and search terms across all validated registrations. **Initial candidate: 32 MiB [provisional].** This bounds build input and direct-scan token work without representation-specific budgets. |
| `SearchTerms` shape | One per-capability phrase-count limit and one aggregate byte limit. **Initial candidates: 16 phrases and 1,024 bytes [provisional].** |
| Distinct query tokens | One internal work limit after normalization and deduplication. **Initial candidate: 16 [provisional].** Excess maps to `ErrResourceLimit`. |
| Structured search response | One internal compact-JSON cap. **Initial candidate: 64 KiB [provisional].** A result that cannot fit fails `Build`; query packing sets `truncated` when results are omitted. |

Before implementation freezes these values, run the fixture plus representative real catalogs and record:

- registered and enabled capability counts;
- aggregate searchable bytes;
- search-term counts and bytes;
- normalized tokens per document;
- distinct query-token counts; and
- compact result and response sizes.

Raise or lower the candidates with documented headroom. Do not add separate vocabulary, posting, token-arena, prefix-fanout, compiled-index, or dual memory-budget limits unless a chosen representation later requires them.

The aggregate source-byte budget and registration ceiling bound build and direct-scan work. The query-token ceiling, registration ceiling, and owned candidate slice bound each request. The result count and response-byte cap independently bound returned structured content.

## Fuzzy matching increment

Typo tolerance is justified by the spike but remains separate because it added only 1.1 percentage points and introduces the highest precision/work complexity.

After the exact/prefix scorer is in production shape:

1. add a typo cohort containing substitutions, insertions, deletions, and adjacent transpositions;
2. apply thresholded transposition-aware edit distance only to query tokens with no exact or prefix match in the candidate document;
3. restrict fuzzy-eligible token lengths;
4. enforce both a finite fuzzy-query-token limit and a total token-comparison work limit;
5. retain only the best fuzzy contribution per query token/document; and
6. rank fuzzy below prefix and exact matches.

Do not select an arbitrary fixed number of global vocabulary neighbors or scan until a time deadline. Select numeric work limits from the typo/adversarial fixture. Fuzzy ships only when it improves typo top-1 without regressing literal queries, compound queries, unrelated empties, deterministic ordering, or bounded work.

A sorted global vocabulary, postings, or a specialized fuzzy index remains profile-driven. The direct document scan is the baseline.

## Failure and truncation behavior

| Condition | Behavior |
|---|---|
| Registration count, aggregate metadata, or search-term shape exceeds a supported bound | `Build` fails atomically as `ErrInvalidRegistration`; no partial catalog. |
| One compact `SearchResult` cannot fit the internal response cap | `Build` fails; the capability is not silently undiscoverable. |
| Raw query exceeds `MaxSearchQueryBytes` | Existing internal search-limit classification; root/MCP project `ErrResourceLimit`. |
| Distinct normalized query tokens exceed the internal work limit | Same coarse `ErrResourceLimit`; do not echo query text. |
| Blank or connector-only query | Successful non-null empty response, `truncated:false`. |
| No eligible document | Successful non-null empty response, `truncated:false`. |
| Eligible candidates exceed count or response-byte cap | Return the highest-ranked prefix, `truncated:true`. |
| Unknown or disabled exact description name | Existing `ErrNotFound`; unchanged. |
| Panic or unexpected search failure | Existing `ErrInternal` projection; unchanged. |

There is no partial metadata indexing and no pagination token.

## Security and visibility

- Disabled capabilities must not influence searchable tokens, document frequency, candidate scores, truncation, or result sizes.
- Search remains authenticated but deployment-visible. It does not perform per-capability authorization.
- All indexed metadata is model-visible in effect. Search terms are inferable even though they are not returned.
- Registration documentation must prohibit secrets and sensitive policy/tenant facts in names, summaries, descriptions, and search terms.
- The catalog does not log raw queries.
- Results expose no scores, matched fields, terms, edit distances, totals, disabled counts, or authorizer outcomes.
- Integer scoring and exact-name tie-breaking prevent map-order and floating-point ordering drift.
- Query resource errors remain coarse and do not echo untrusted input.

## Integration points

| File | Change |
|---|---|
| `capability.go` | Add and document `SearchTerms`. |
| `builder.go` | Clone search terms and pass them into `catalog.Registration`. |
| `internal/catalog/catalog.go` | Add search metadata and the immutable `searchIndex`; remove lowercase substring fields. |
| `internal/catalog/build.go` | Validate the small bound set, filter first, compile search documents and document frequency, and precompute result JSON lengths. |
| `internal/catalog/search.go` | Define `SearchResponse`; implement tokenizer, direct scorer, eligibility, deterministic sorting, response packing, and exact `Describe`. Keep retained constants together here initially. |
| `server.go` | Return `SearchResponse` and preserve coarse error projection. |
| `mcpserver/service.go` | Change the search port return type. |
| `mcpserver/server.go` | Generate the object output schema, normalize `Results`, update tool guidance, and increment implementation version. Keep exactly three tools. |
| Generated mocks and callers | Regenerate/update for the clean method-signature cutover. |
| Tests and user documentation | Replace literal-substring, name-sorted, and bare-array claims with ranked/tokenized/truncation behavior. |

No binding, execution, worker, authorizer, resolver, or namespace interface needs a search concept.

## Agile implementation sequence

### Measurement and fixture setup

This is preparation, not a product phase.

- Move the disposable corpus, deterministic distractors, model-query cohorts, unrelated cases, SQL/MySQL boundary, and search-term dependency cases into a durable behavioral fixture.
- Add closely competing capabilities, not only broad distractors.
- Measure real catalog and query shapes before freezing provisional bounds and initial internal weights.

### Increment 1: direct bounded ranked lexical search

Ship one complete clean cut:

- `SearchTerms` cloning and validation;
- `SearchResponse{Results, Truncated}` through root and MCP;
- shared normalization/tokenization;
- immutable direct-search documents;
- name, terms, summary, and low-weight description scoring;
- integer document-frequency weighting;
- exact and prefix token matching;
- strict eligibility and one linear coverage factor;
- exact-name priority and deterministic full-catalog sorting;
- count and internal response-byte packing;
- affected tests, examples, and documentation.

Do not include fuzzy matching, postings, token arenas, interning, pooling, a top-K heap, general phrase boosts, or public response-byte configuration.

### Increment 2: bounded typo tolerance

Add thresholded transposition-aware edit distance with measured token/work limits. Ship only on demonstrated typo gain and no precision regression.

### Profile-driven refinement

After functional behavior is proven, benchmark/profile 100, 1,000, and the supported maximum under expected concurrency. Add only the optimization supported by evidence:

- postings for direct-scan CPU;
- interning/arenas for retained catalog memory;
- a heap for candidate sorting;
- capped pooling for GC pressure.

This is not a promised phase and creates no dormant abstraction.

## Quality gates

Increment 1 must prove observable behavior:

- known literal fragments retain intended top-1;
- compound model queries retrieve intended top-1;
- task paraphrase performance meets or exceeds the no-fuzzy spike result within an agreed fixture tolerance;
- unrelated queries remain empty and non-truncated;
- `sql` ranks Snowflake ahead of MySQL distractors and does not match `mysql` as an infix token;
- the two unresolved paraphrases become discoverable only when corresponding `SearchTerms` are registered;
- exact dotted name is always first for an exact-name query;
- disabling a capability removes it from every live surface and does not affect enabled ranking/truncation;
- case changes, whitespace, search terms, prefixes, and typos remain invalid for exact `Describe`;
- repeated and concurrent searches produce byte-equivalent ordered results;
- every retained bound has boundary coverage;
- count and byte omission set `truncated`; empty results are non-null;
- MCP tool listing remains exactly `search_api`, `describe_api`, and `execute`;
- resolver, resource-limit, panic, and internal failures retain existing coarse projections.

Performance gates begin with measurement, not invented targets:

- benchmark 100, 1,000, and supported-maximum catalogs on a documented machine;
- record allocations, bytes, latency, and retained catalog memory;
- require no request-local maps and no per-token/document match objects;
- require request memory to stay within the architecture's finite catalog/query/candidate bounds; and
- set absolute regression gates only after the direct scorer establishes a baseline.

Increment 2 adds typo, fuzzy-work-bound, and precision-regression gates.

## Rejected alternatives

| Alternative | Decision | Evidence required to revisit |
|---|---|---|
| Keep literal substring search | Rejected by the spike's compound/paraphrase failures and SQL/MySQL collision. | None; retain only as a benchmark baseline. |
| Stop after the first K name-sorted matches | Rejected because catalog order is not relevance. | None while relevance is the goal. |
| Arbitrary infix token matching | Rejected because it recreates token-boundary false positives. | Representative intended-infix queries that explicit search terms cannot express safely. |
| Pagination | Rejected because page walking adds the round trips and tokens being removed. | Production evidence that good metadata and refinement still miss intended capabilities beyond the ranked prefix. |
| Return descriptions, scores, or matched terms | Rejected to keep results compact and preserve `describe_api` as the exact next step. | Agent evaluation showing compact ranked records cannot support correct selection. |
| Configurable scoring weights or ranking DSL | Rejected as an untestable framework. | Multiple real corpora proving no fixed policy works and a stable minimal control requirement. |
| Packed postings, token arenas, interning | Deferred. | CPU or retained-heap profile showing direct document scans or string headers are material. |
| Top-K heap | Deferred. | Candidate sorting appears in a production-shaped CPU profile. |
| `sync.Pool` | Deferred. | GC profile shows request-owned bounded slices materially affect latency or throughput. |
| Embeddings, vector search, LLM search, external service | Rejected by bounded deterministic deployment requirements. | Explicit product constraint change, lexical quality failure, and separate security/operations review. |
| Mutable/runtime index | Rejected by one-shot immutable `Builder` semantics. | Approved runtime registration with snapshot and concurrency semantics. |
| Subject-filtered discovery | Deferred outside this feature. | Explicit confidentiality requirement for capability existence and a security-model redesign. |
| Compatibility wrapper or second search tool | Rejected; use a clean cutover. | None for this change. |

## Risks and open decisions

- **Metadata quality is the recall ceiling.** Search terms and descriptions need practical authoring guidance and fixture review.
- **Closely related commands remain under-tested.** Add adversarial families before fixing weights.
- **Stop-word handling can hide meaningful short terms.** Keep the connector list small, internal, and corpus-tested.
- **Prefix matching can create broad candidates.** Strict two-token eligibility and document-frequency weighting are required; fixture failures determine whether prefix minimum length changes.
- **Hard bounds can reject a large existing host catalog.** Measure real catalogs before freezing provisional values; fail clearly rather than silently dropping metadata.
- **The MCP response cutover is breaking.** Migrate root callers, adapter mocks, schemas, examples, tests, and docs in one release.
- **Response-byte accounting covers structured content, not the SDK envelope or text mirror.** Verify standard encoding behavior and state the boundary precisely.
- **Fuzzy false positives are the main Increment 2 risk.** Do not ship typo tolerance on recall gain alone; unrelated-query precision and deterministic work must remain intact.

## Complexity-review changes applied

The final design applies the complexity review as follows:

- replaced packed lexemes, postings, token arenas, fixed-width offsets, and scratch pooling with one immutable document slice and direct scan;
- removed global prefix-expansion cutoffs whose behavior changed with catalog vocabulary size;
- collapsed overlapping metadata/index limits into a small independent bound set;
- kept response bytes bounded but deferred public byte-limit configuration;
- replaced giant score boosts and dominance arithmetic with an explicit comparator;
- deferred general phrase boosts;
- retained one simple linear coverage adjustment after strict eligibility;
- replaced the top-K heap with a bounded candidate slice and ordinary deterministic sort;
- deferred fuzzy matching to a separately gated increment;
- removed invented allocation/latency thresholds in favor of measured baselines; and
- made packed indexes, heaps, interning, and pooling profile-triggered refinements rather than planned architecture.
