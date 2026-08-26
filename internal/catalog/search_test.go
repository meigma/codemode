package catalog

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchRanksTokensAndHonorsBounds proves ranked discovery, eligibility, and truncation.
func TestSearchRanksTokensAndHonorsBounds(t *testing.T) {
	tests := []struct {
		name          string
		registrations []Registration
		options       Options
		query         string
		wantNames     []string
		wantTruncated bool
		wantEmpty     bool
	}{
		{
			name: "sql does not match mysql infix and ranks snowflake first",
			registrations: []Registration{
				validRegistration("cap.mysql.users", "mysql.users.list", "List MySQL users"),
				validRegistration("cap.mysql.queries", "mysql.queries.execute", "Execute a MySQL query"),
				validRegistration("cap.snowflake", "snowflake.queries.execute", "Execute a Snowflake SQL query"),
			},
			options:   testOptions(),
			query:     "sql",
			wantNames: []string{"snowflake.queries.execute"},
		},
		{
			name: "mysql query matches mysql without emitting sql tokens",
			registrations: []Registration{
				validRegistration("cap.mysql.users", "mysql.users.list", "List MySQL users"),
				validRegistration("cap.snowflake", "snowflake.queries.execute", "Execute a Snowflake SQL query"),
			},
			options:   testOptions(),
			query:     "mysql",
			wantNames: []string{"mysql.users.list"},
		},
		{
			name: "acronyms split before a camel tail",
			registrations: []Registration{
				validRegistration("cap.xml", "records.parseXMLFile", "Parse an XML file"),
			},
			options:   testOptions(),
			query:     "xml file",
			wantNames: []string{"records.parseXMLFile"},
		},
		{
			name: "GitHub stays one token",
			registrations: []Registration{
				validRegistration("cap.review", "github.pulls.createReview", "Create a GitHub pull review"),
			},
			options:   testOptions(),
			query:     "hub",
			wantEmpty: true,
		},
		{
			name: "compound task query matches without a contiguous phrase",
			registrations: []Registration{
				validRegistration("cap.create", "github.issues.create", "Create a repository issue"),
				validRegistration("cap.merge", "github.pulls.merge", "Merge a pull request"),
			},
			options:   testOptions(),
			query:     "create github issue",
			wantNames: []string{"github.issues.create"},
		},
		{
			name: "camel and dotted names tokenize on boundaries",
			registrations: []Registration{
				validRegistration("cap.review", "github.pulls.createReview", "Create a pull review"),
			},
			options:   testOptions(),
			query:     "create review",
			wantNames: []string{"github.pulls.createReview"},
		},
		{
			name: "prefix tokens match longer document tokens",
			registrations: []Registration{
				validRegistration("cap.review", "github.pulls.createReview", "Create a pull review"),
			},
			options:   testOptions(),
			query:     "crea",
			wantNames: []string{"github.pulls.createReview"},
		},
		{
			name: "common exact list beats rare prefix listener in the same field",
			registrations: []Registration{
				withSummary(validRegistration("cap.alpha", "records.alpha", "Alpha record"), "List records"),
				withSummary(validRegistration("cap.zeta", "records.zeta", "Zeta record"), "List listener records"),
			},
			options:   testOptions(),
			query:     "list",
			wantNames: []string{"records.alpha", "records.zeta"},
		},
		{
			name: "search terms are required for otherwise undiscoverable vocabulary",
			registrations: []Registration{
				withSearchTerms(
					validRegistration("cap.create", "github.issues.create", "Create a repository issue"),
					"open ticket",
					"file bug report",
				),
				validRegistration("cap.merge", "github.pulls.merge", "Merge a pull request"),
			},
			options:   testOptions(),
			query:     "ticket",
			wantNames: []string{"github.issues.create"},
		},
		{
			name: "name outranks search terms, summary, and description",
			registrations: []Registration{
				withDescription(
					validRegistration("cap.desc", "other.describe", "Unrelated summary"),
					"Contains the widget token in description.",
				),
				withSummary(
					validRegistration("cap.summary", "other.summarize", "Handles a widget"),
					"Handles a widget",
				),
				withSearchTerms(validRegistration("cap.terms", "other.terms", "Unrelated summary"), "widget"),
				validRegistration("cap.name", "widget.alpha", "Unrelated summary"),
			},
			options:   testOptions(),
			query:     "widget",
			wantNames: []string{"widget.alpha", "other.terms", "other.summarize", "other.describe"},
		},
		{
			name: "unrelated two-token queries stay empty",
			registrations: []Registration{
				validRegistration("cap.deploy", "apps.deploy", "Deploy an application"),
			},
			options:   testOptions(),
			query:     "deploy rocket",
			wantEmpty: true,
		},
		{
			name: "exact normalized name is first even when another document scores",
			registrations: []Registration{
				validRegistration("cap.generic", "records.lookup", "Lookup records"),
				validRegistration("cap.exact", "records.alpha", "Alpha record"),
			},
			options:   testOptions(),
			query:     "records.alpha",
			wantNames: []string{"records.alpha"},
		},
		{
			name: "equal scores sort by exact dotted name",
			registrations: []Registration{
				validRegistration("cap.zeta", "records.zeta", "Shared token"),
				validRegistration("cap.alpha", "records.alpha", "Shared token"),
			},
			options:   testOptions(),
			query:     "shared",
			wantNames: []string{"records.alpha", "records.zeta"},
		},
		{
			name: "disabled metadata cannot match or occupy truncation slots",
			registrations: []Registration{
				validRegistration("cap.alpha", "records.alpha", "Alpha record"),
				validRegistration("cap.beta", "records.beta", "Beta record"),
				validRegistration("cap.disabled", "records.disabled", "Disabled record"),
			},
			options:       testOptions("cap.disabled"),
			query:         "record",
			wantNames:     []string{"records.alpha", "records.beta"},
			wantTruncated: false,
		},
		{
			name: "connector and blank queries return non-nil empty results",
			registrations: []Registration{
				validRegistration("cap.alpha", "records.alpha", "Alpha record"),
			},
			options:   testOptions(),
			query:     "the and of",
			wantEmpty: true,
		},
		{
			name: "count limit sets truncated",
			registrations: []Registration{
				validRegistration("cap.zeta", "records.zeta", "Alpha Zeta record"),
				validRegistration("cap.beta", "records.beta", "Alpha Beta record"),
				validRegistration("cap.alpha", "records.alpha", "Alpha primary record"),
			},
			options: func() Options {
				options := testOptions()
				options.MaxSearchResults = 2
				return options
			}(),
			query:         "alpha",
			wantNames:     []string{"records.alpha", "records.beta"},
			wantTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			built, err := Build(tt.registrations, tt.options)
			require.NoError(t, err)

			response, err := built.Search(tt.query)

			require.NoError(t, err)
			require.NotNil(t, response.Results)
			if tt.wantEmpty {
				assert.Empty(t, response.Results)
				assert.False(t, response.Truncated)
				return
			}
			require.Equal(t, tt.wantNames, searchNames(response))
			assert.Equal(t, tt.wantTruncated, response.Truncated)
		})
	}
}

// TestSearchOmitsOversizedTrailingResults proves byte packing truncates without reordering.
func TestSearchOmitsOversizedTrailingResults(t *testing.T) {
	summary := "token " + strings.Repeat("x", 40_000)
	built, err := Build([]Registration{
		withSummary(validRegistration("cap.alpha", "records.alpha", "token"), summary),
		withSummary(validRegistration("cap.beta", "records.beta", "token"), summary),
	}, testOptions())
	require.NoError(t, err)

	response, err := built.Search("token")

	require.NoError(t, err)
	require.NotNil(t, response.Results)
	require.Equal(t, []string{"records.alpha"}, searchNames(response))
	assert.True(t, response.Truncated)
}

// TestSearchRejectsQueryTokenOverflow proves distinct token excess uses the search-limit sentinel.
func TestSearchRejectsQueryTokenOverflow(t *testing.T) {
	options := testOptions()
	options.MaxSearchQueryBytes = 256
	built, err := Build([]Registration{
		validRegistration("cap.alpha", "records.alpha", "Alpha record"),
	}, options)
	require.NoError(t, err)

	_, err = built.Search(
		"alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec",
	)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrSearchQueryLimit)
	assert.NotContains(t, err.Error(), "bravo")
}

// TestDescribeIgnoresSearchVocabulary proves exact lookup is unchanged by discovery metadata.
func TestDescribeIgnoresSearchVocabulary(t *testing.T) {
	built, err := Build([]Registration{
		withSearchTerms(validRegistration("cap.alpha", "records.alpha", "Alpha record"), "open ticket"),
	}, testOptions())
	require.NoError(t, err)

	_, foundTerm := built.Describe("open ticket")
	_, foundCase := built.Describe("Records.alpha")
	_, foundPrefix := built.Describe("records.alp")
	description, found := built.Describe("records.alpha")

	assert.False(t, foundTerm)
	assert.False(t, foundCase)
	assert.False(t, foundPrefix)
	require.True(t, found)
	assert.Equal(t, "records.alpha", description.Name)
	assert.Equal(t, "Alpha record", description.Summary)
}

// TestBuildRejectsSearchMetadataBounds proves registration, term, and result-size ceilings fail closed.
func TestBuildRejectsSearchMetadataBounds(t *testing.T) {
	valid := validRegistration("cap.one", "records.one", "First record")
	tests := []struct {
		name          string
		registrations []Registration
	}{
		{
			name: "too many search-term phrases",
			registrations: []Registration{
				withSearchTerms(valid, makePhrases(maxSearchTermPhrases+1)...),
			},
		},
		{
			name: "search-term aggregate bytes",
			registrations: []Registration{
				withSearchTerms(valid, strings.Repeat("token", maxSearchTermBytes)),
			},
		},
		{
			name: "blank search term",
			registrations: []Registration{
				withSearchTerms(valid, "open ticket", ""),
			},
		},
		{
			name: "surrounding whitespace search term",
			registrations: []Registration{
				withSearchTerms(valid, " open ticket"),
			},
		},
		{
			name: "single result exceeds response cap",
			registrations: []Registration{
				withSummary(valid, strings.Repeat("x", maxSearchResponseBytes)),
			},
		},
		{
			name:          "too many registrations",
			registrations: overflowRegistrations(maxSupportedRegistrations + 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Build(tt.registrations, testOptions())

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidRegistration)
		})
	}
}

// TestDisabledSearchTermsDoNotAffectRanking proves filtered vocabulary cannot change enabled order.
func TestDisabledSearchTermsDoNotAffectRanking(t *testing.T) {
	options := testOptions("cap.disabled")
	built, err := Build([]Registration{
		validRegistration("cap.alpha", "records.alpha", "Shared token"),
		validRegistration("cap.beta", "records.beta", "Shared token"),
		withSearchTerms(validRegistration("cap.disabled", "records.disabled", "Shared token"), "unique disabled term"),
	}, options)
	require.NoError(t, err)

	disabled, err := built.Search("unique")
	require.NoError(t, err)
	assert.Empty(t, disabled.Results)
	assert.False(t, disabled.Truncated)

	shared, err := built.Search("shared")
	require.NoError(t, err)
	require.Equal(t, []string{"records.alpha", "records.beta"}, searchNames(shared))
	assert.False(t, shared.Truncated)
}

// withSearchTerms returns a copy with explicit discovery phrases.
func withSearchTerms(registration Registration, terms ...string) Registration {
	registration.SearchTerms = terms
	return registration
}

// searchNames extracts ranked result names.
func searchNames(response SearchResponse) []string {
	names := make([]string, len(response.Results))
	for index, result := range response.Results {
		names[index] = result.Name
	}
	return names
}

// makePhrases returns count distinct short search-term phrases.
func makePhrases(count int) []string {
	phrases := make([]string, count)
	for index := range phrases {
		phrases[index] = "term" + strings.Repeat("x", index)
	}
	return phrases
}

// overflowRegistrations constructs count valid registrations with distinct names.
func overflowRegistrations(count int) []Registration {
	registrations := make([]Registration, count)
	for index := range registrations {
		suffix := strconv.Itoa(index)
		registrations[index] = validRegistration(
			"cap.item"+suffix,
			"records.item"+suffix,
			"Item record",
		)
	}
	return registrations
}
