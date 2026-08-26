package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/meigma/codemode/internal/binding"
)

const (
	maxSupportedRegistrations     = 4096
	maxSearchableMetadataBytes    = 32 * 1024 * 1024
	maxSearchTermPhrases          = 16
	maxSearchTermBytes            = 1024
	maxDistinctQueryTokens        = 16
	maxSearchResponseBytes        = 64 * 1024
	minPrefixQueryTokenLength     = 3
	fieldWeightName               = 12
	fieldWeightSearchTerms        = 10
	fieldWeightSummary            = 5
	fieldWeightDescription        = 2
	matchQualityExact             = 100
	matchQualityPrefix            = 72
	idfBucketCount                = 8
	idfMinFactor                  = 1
	tokenizeScratchCap            = 8
	tokensPerSearchPhrase         = 2
	twoTokenQuery                 = 2
	coverageMatchNumerator        = 2
	coverageMatchDenominator      = 3
	coverageMatchRoundUp          = 2
	queryTokenOverflowSlot        = 1
	searchResponsePrefixJSONBytes = len(`{"results":[`)
	searchResponseTruncatedFalse  = len(`],"truncated":false}`)
	searchResponseCommaJSONBytes  = 1
)

var (
	// ErrSearchQueryLimit classifies a search query that exceeds its byte or token budget.
	ErrSearchQueryLimit = errors.New("search query limit exceeded")
)

// SearchResult is one compact model-facing capability discovery record.
type SearchResult struct {
	// Name is the enabled capability's exact dotted name.
	Name string `json:"name"`

	// Signature is generated from the same compiled plan used for binding.
	Signature string `json:"signature"`

	// Summary is the compact searchable description.
	Summary string `json:"summary"`
}

// SearchResponse is one bounded ranked discovery result set.
type SearchResponse struct {
	// Results contains the packed ranked prefix of eligible capabilities.
	Results []SearchResult `json:"results"`

	// Truncated reports whether at least one eligible result was omitted.
	Truncated bool `json:"truncated"`
}

// Description is one exact model-facing capability description.
type Description struct {
	// Name is the enabled capability's exact dotted name.
	Name string `json:"name"`

	// Signature is generated from the same compiled plan used for binding.
	Signature string `json:"signature"`

	// Summary is the compact searchable description.
	Summary string `json:"summary"`

	// Description is the full registered description.
	Description string `json:"description"`

	// Input is the ordered supported input field shape.
	Input []binding.FieldShape `json:"input"`

	// Output is the ordered supported output field shape.
	Output []binding.FieldShape `json:"output"`
}

// searchIndex is the immutable owned document slice compiled after static filtering.
type searchIndex struct {
	// documents is aligned by index with Catalog.enabled.
	documents []searchDocument
}

// searchDocument is one enabled capability's owned searchable tokens.
type searchDocument struct {
	// normalizedName is the lowercase dotted capability name used for exact-name priority.
	normalizedName string

	// nameTokens contains distinct tokens compiled from the capability name.
	nameTokens []searchToken

	// searchTermTokens contains distinct tokens compiled from explicit search terms.
	searchTermTokens []searchToken

	// summaryTokens contains distinct tokens compiled from the compact summary.
	summaryTokens []searchToken

	// descriptionTokens contains distinct tokens compiled from the full description.
	descriptionTokens []searchToken

	// resultJSONBytes is the compact encoding/json size of the projected SearchResult.
	resultJSONBytes int
}

// searchToken is one retained field token with a monotone document-frequency factor.
type searchToken struct {
	// text is the normalized token text.
	text string

	// idf is the integer rarity factor assigned at catalog build.
	idf uint64
}

// searchCandidate is one eligible document retained until packing.
type searchCandidate struct {
	// document is the index into Catalog.enabled and searchIndex.documents.
	document int

	// score is the integer ranking score after coverage adjustment.
	score uint64

	// exact reports whether the normalized query equals the normalized dotted name.
	exact bool
}

// Search ranks enabled capabilities for one bounded query.
func (catalog *Catalog) Search(query string) (SearchResponse, error) {
	if len(query) > catalog.maxSearchQueryBytes {
		return SearchResponse{}, fmt.Errorf(
			"%w: query is %d bytes; maximum is %d",
			ErrSearchQueryLimit,
			len(query),
			catalog.maxSearchQueryBytes,
		)
	}
	tokens, err := tokenizeQuery(query)
	if err != nil {
		return SearchResponse{}, err
	}
	if len(tokens) == 0 {
		return emptySearchResponse(), nil
	}
	normalizedName := strings.ToLower(strings.TrimSpace(query))
	candidates := make([]searchCandidate, 0, len(catalog.search.documents))
	for index, document := range catalog.search.documents {
		score, matched := scoreDocument(document, tokens)
		if matched < requiredMatches(len(tokens)) {
			continue
		}
		candidates = append(candidates, searchCandidate{
			document: index,
			score:    score * matched / uintCount(len(tokens)),
			exact:    document.normalizedName == normalizedName,
		})
	}
	slices.SortFunc(candidates, func(left searchCandidate, right searchCandidate) int {
		if order := compareSearchCandidates(left, right); order != 0 {
			return order
		}
		return strings.Compare(catalog.enabled[left.document].Name, catalog.enabled[right.document].Name)
	})
	return catalog.packSearchResponse(candidates), nil
}

// Describe returns the exact description of one enabled capability without fuzzy expansion.
func (catalog *Catalog) Describe(name string) (Description, bool) {
	entry, ok := catalog.Lookup(name)
	if !ok {
		return Description{}, false
	}
	return Description{
		Name:        entry.Name,
		Signature:   entry.signature,
		Summary:     entry.Summary,
		Description: entry.Description,
		Input:       slices.Clone(entry.inputShape),
		Output:      slices.Clone(entry.outputShape),
	}, true
}

// emptySearchResponse returns a successful empty discovery payload.
func emptySearchResponse() SearchResponse {
	return SearchResponse{Results: []SearchResult{}}
}

// tokenizeQuery normalizes and deduplicates query tokens after the raw-byte check.
func tokenizeQuery(query string) ([]string, error) {
	tokens := tokenize(query)
	unique := make([]string, 0, min(len(tokens), maxDistinctQueryTokens+queryTokenOverflowSlot))
	for _, token := range tokens {
		if slices.Contains(unique, token) {
			continue
		}
		unique = append(unique, token)
		if len(unique) > maxDistinctQueryTokens {
			return nil, fmt.Errorf(
				"%w: query has %d distinct tokens; maximum is %d",
				ErrSearchQueryLimit,
				len(unique),
				maxDistinctQueryTokens,
			)
		}
	}
	return unique, nil
}

// uniqueTokens retains first-seen tokens while preserving order.
func uniqueTokens(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tokens))
	unique := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		unique = append(unique, token)
	}
	return unique
}

// tokenize splits value on punctuation, case, and letter/number boundaries and drops connectors.
func tokenize(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	tokens := make([]string, 0, tokenizeScratchCap)
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		token := strings.ToLower(current.String())
		current.Reset()
		if isConnectorToken(token) {
			return
		}
		tokens = append(tokens, token)
	}
	runes := []rune(value)
	startedUpper := false
	for index, currentRune := range runes {
		if !unicode.IsLetter(currentRune) && !unicode.IsDigit(currentRune) {
			flush()
			startedUpper = false
			continue
		}
		if current.Len() > 0 && shouldSplitBefore(runes, index, startedUpper) {
			flush()
			startedUpper = unicode.IsUpper(currentRune)
		} else if current.Len() == 0 {
			startedUpper = unicode.IsUpper(currentRune)
		}
		current.WriteRune(currentRune)
	}
	flush()
	return tokens
}

// shouldSplitBefore reports a camel, acronym, or letter/number boundary before runes[index].
// startedUpper is true when the current token began with an uppercase letter, which keeps MySQL and GitHub whole.
func shouldSplitBefore(runes []rune, index int, startedUpper bool) bool {
	previous := runes[index-1]
	current := runes[index]
	if unicode.IsLetter(previous) && unicode.IsDigit(current) {
		return true
	}
	if unicode.IsDigit(previous) && unicode.IsLetter(current) {
		return true
	}
	if unicode.IsLower(previous) && unicode.IsUpper(current) {
		return !startedUpper
	}
	if unicode.IsUpper(previous) && unicode.IsUpper(current) &&
		index+1 < len(runes) && unicode.IsLower(runes[index+1]) {
		return true
	}
	return false
}

// isConnectorToken reports whether token is a dropped grammatical connector.
func isConnectorToken(token string) bool {
	switch token {
	case "a", "an", "and", "by", "for", "from", "in", "of", "on", "or", "the", "to", "with":
		return true
	default:
		return false
	}
}

// uintCount converts a non-negative int count to uint64.
func uintCount(value int) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}

// scoreDocument returns the best per-token contributions and the distinct match count.
func scoreDocument(document searchDocument, queryTokens []string) (uint64, uint64) {
	var score uint64
	var matched uint64
	for _, queryToken := range queryTokens {
		contribution := bestTokenContribution(document, queryToken)
		if contribution == 0 {
			continue
		}
		matched++
		score += contribution
	}
	return score, matched
}

// bestTokenContribution returns the highest field/quality contribution for one query token.
func bestTokenContribution(document searchDocument, queryToken string) uint64 {
	best := tokenFieldContribution(document.nameTokens, queryToken, fieldWeightName)
	if contribution := tokenFieldContribution(
		document.searchTermTokens,
		queryToken,
		fieldWeightSearchTerms,
	); contribution > best {
		best = contribution
	}
	if contribution := tokenFieldContribution(
		document.summaryTokens,
		queryToken,
		fieldWeightSummary,
	); contribution > best {
		best = contribution
	}
	if contribution := tokenFieldContribution(
		document.descriptionTokens,
		queryToken,
		fieldWeightDescription,
	); contribution > best {
		best = contribution
	}
	return best
}

// tokenFieldContribution returns the best exact-then-prefix contribution in one field.
// Exact quality dominates prefix quality; rarity is compared only among equal-quality matches.
func tokenFieldContribution(tokens []searchToken, queryToken string, weight uint64) uint64 {
	var bestQuality uint64
	var bestIDF uint64
	for _, token := range tokens {
		quality := tokenMatchQuality(queryToken, token.text)
		if quality == 0 {
			continue
		}
		if quality > bestQuality || (quality == bestQuality && token.idf > bestIDF) {
			bestQuality = quality
			bestIDF = token.idf
		}
	}
	if bestQuality == 0 {
		return 0
	}
	return weight * bestIDF * bestQuality
}

// tokenMatchQuality scores exact token equality above a bounded prefix match.
func tokenMatchQuality(queryToken string, documentToken string) uint64 {
	if queryToken == documentToken {
		return matchQualityExact
	}
	if utf8.RuneCountInString(queryToken) < minPrefixQueryTokenLength {
		return 0
	}
	if strings.HasPrefix(documentToken, queryToken) {
		return matchQualityPrefix
	}
	return 0
}

// requiredMatches returns the strict eligibility threshold for q distinct query tokens.
func requiredMatches(queryTokens int) uint64 {
	switch {
	case queryTokens <= 1:
		return uintCount(queryTokens)
	case queryTokens == twoTokenQuery:
		return twoTokenQuery
	default:
		return uintCount((coverageMatchNumerator*queryTokens + coverageMatchRoundUp) / coverageMatchDenominator)
	}
}

// compareSearchCandidates orders exact-name hits first, then score descending, then name ascending.
func compareSearchCandidates(left searchCandidate, right searchCandidate) int {
	switch {
	case left.exact != right.exact:
		if left.exact {
			return -1
		}
		return 1
	case left.score != right.score:
		if left.score > right.score {
			return -1
		}
		return 1
	default:
		return 0
	}
}

// packSearchResponse walks ranked candidates under count and compact-response byte bounds.
func (catalog *Catalog) packSearchResponse(candidates []searchCandidate) SearchResponse {
	if len(candidates) == 0 {
		return emptySearchResponse()
	}
	results := make([]SearchResult, 0, min(catalog.maxSearchResults, len(candidates)))
	used := searchResponsePrefixJSONBytes
	for _, candidate := range candidates {
		if len(results) == catalog.maxSearchResults {
			return SearchResponse{Results: results, Truncated: true}
		}
		resultBytes := catalog.search.documents[candidate.document].resultJSONBytes
		separator := 0
		if len(results) > 0 {
			separator = searchResponseCommaJSONBytes
		}
		if used+separator+resultBytes+searchResponseTruncatedFalse > maxSearchResponseBytes {
			return SearchResponse{Results: results, Truncated: true}
		}
		entry := catalog.enabled[candidate.document]
		results = append(results, SearchResult{
			Name:      entry.Name,
			Signature: entry.signature,
			Summary:   entry.Summary,
		})
		used += separator + resultBytes
	}
	return SearchResponse{Results: results, Truncated: false}
}

// compileSearchIndex builds owned documents and document-frequency factors after filtering.
func compileSearchIndex(enabled []Entry, searchTerms [][]string) (searchIndex, error) {
	documents := make([]searchDocument, len(enabled))
	documentFrequency := make(map[string]int)
	for index, entry := range enabled {
		nameTokens := uniqueTokens(tokenize(entry.Name))
		termTokens := uniqueTokens(tokenizeJoined(searchTerms[index]))
		summaryTokens := uniqueTokens(tokenize(entry.Summary))
		descriptionTokens := uniqueTokens(tokenize(entry.Description))
		seen := make(map[string]struct{})
		markDocumentTokens(seen, nameTokens)
		markDocumentTokens(seen, termTokens)
		markDocumentTokens(seen, summaryTokens)
		markDocumentTokens(seen, descriptionTokens)
		for token := range seen {
			documentFrequency[token]++
		}
		result := SearchResult{
			Name:      entry.Name,
			Signature: entry.signature,
			Summary:   entry.Summary,
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return searchIndex{}, fmt.Errorf("%w: search result %q: %w", ErrInvalidRegistration, entry.Name, err)
		}
		if !resultFitsResponseCap(len(encoded)) {
			return searchIndex{}, fmt.Errorf(
				"%w: search result %q is %d bytes and cannot fit the %d-byte response cap",
				ErrInvalidRegistration,
				entry.Name,
				len(encoded),
				maxSearchResponseBytes,
			)
		}
		documents[index] = searchDocument{
			normalizedName:    strings.ToLower(entry.Name),
			nameTokens:        tokensWithoutIDF(nameTokens),
			searchTermTokens:  tokensWithoutIDF(termTokens),
			summaryTokens:     tokensWithoutIDF(summaryTokens),
			descriptionTokens: tokensWithoutIDF(descriptionTokens),
			resultJSONBytes:   len(encoded),
		}
	}
	documentCount := len(enabled)
	assignIDF(documents, documentFrequency, documentCount)
	return searchIndex{documents: documents}, nil
}

// tokenizeJoined tokenizes each phrase and concatenates the tokens.
func tokenizeJoined(phrases []string) []string {
	tokens := make([]string, 0, len(phrases)*tokensPerSearchPhrase)
	for _, phrase := range phrases {
		tokens = append(tokens, tokenize(phrase)...)
	}
	return tokens
}

// markDocumentTokens records each token as present in the current document.
func markDocumentTokens(seen map[string]struct{}, tokens []string) {
	for _, token := range tokens {
		seen[token] = struct{}{}
	}
}

// tokensWithoutIDF constructs search tokens before document-frequency assignment.
func tokensWithoutIDF(tokens []string) []searchToken {
	if len(tokens) == 0 {
		return nil
	}
	converted := make([]searchToken, len(tokens))
	for index, token := range tokens {
		converted[index] = searchToken{text: token}
	}
	return converted
}

// assignIDF writes a small monotone integer rarity factor onto every retained token.
func assignIDF(documents []searchDocument, documentFrequency map[string]int, documentCount int) {
	factors := make(map[string]uint64, len(documentFrequency))
	for token, frequency := range documentFrequency {
		factors[token] = idfFactor(frequency, documentCount)
	}
	applyIDF := func(tokens []searchToken) {
		for index, token := range tokens {
			tokens[index].idf = factors[token.text]
		}
	}
	for index := range documents {
		applyIDF(documents[index].nameTokens)
		applyIDF(documents[index].searchTermTokens)
		applyIDF(documents[index].summaryTokens)
		applyIDF(documents[index].descriptionTokens)
	}
}

// idfFactor maps document frequency onto a small increasing rarity scale.
func idfFactor(frequency int, documentCount int) uint64 {
	if frequency <= 0 || documentCount <= 0 {
		return idfMinFactor
	}
	numerator := (documentCount - frequency) * (idfBucketCount - idfMinFactor)
	return idfMinFactor + uintCount(numerator/documentCount)
}

// resultFitsResponseCap reports whether one compact result can occupy a successful response.
func resultFitsResponseCap(resultJSONBytes int) bool {
	return searchResponsePrefixJSONBytes+resultJSONBytes+searchResponseTruncatedFalse <= maxSearchResponseBytes
}

// searchableMetadataBytes returns the aggregate raw searchable metadata size of one registration.
func searchableMetadataBytes(registration Registration) int {
	total := len(registration.Name) + len(registration.Summary) + len(registration.Description)
	for _, term := range registration.SearchTerms {
		total += len(term)
	}
	return total
}

// validateSearchTerms reports whether one capability's discovery phrases stay in bounds.
func validateSearchTerms(terms []string) error {
	if len(terms) > maxSearchTermPhrases {
		return fmt.Errorf("search terms exceed %d phrases", maxSearchTermPhrases)
	}
	total := 0
	for index, term := range terms {
		if term != strings.TrimSpace(term) {
			return fmt.Errorf("search term %d must not have surrounding whitespace", index)
		}
		if term == "" {
			return fmt.Errorf("search term %d must not be empty", index)
		}
		total += len(term)
	}
	if total > maxSearchTermBytes {
		return fmt.Errorf("search terms exceed %d bytes", maxSearchTermBytes)
	}
	return nil
}
