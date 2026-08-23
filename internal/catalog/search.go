package catalog

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/meigma/codemode/internal/binding"
)

var (
	// ErrSearchQueryLimit classifies a search query that exceeds its byte budget.
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

// Search performs a deterministic case-normalized linear scan over enabled names and summaries.
func (catalog *Catalog) Search(query string) ([]SearchResult, error) {
	if len(query) > catalog.maxSearchQueryBytes {
		return nil, fmt.Errorf(
			"%w: query is %d bytes; maximum is %d",
			ErrSearchQueryLimit,
			len(query),
			catalog.maxSearchQueryBytes,
		)
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return []SearchResult{}, nil
	}

	results := make([]SearchResult, 0, min(catalog.maxSearchResults, len(catalog.enabled)))
	for _, entry := range catalog.enabled {
		if !strings.Contains(entry.searchName, normalized) &&
			!strings.Contains(entry.searchSummary, normalized) {
			continue
		}
		results = append(results, SearchResult{
			Name:      entry.Name,
			Signature: entry.signature,
			Summary:   entry.Summary,
		})
		if len(results) == catalog.maxSearchResults {
			break
		}
	}
	return results, nil
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
