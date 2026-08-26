package catalog

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/binding"
)

// testInput is the catalog test capability input.
type testInput struct {
	// Org is one required query value.
	Org string `json:"org"`

	// Limit is one optional bound.
	Limit *int64 `json:"limit,omitempty"`
}

// testOutput is the catalog test capability output.
type testOutput struct {
	// Name is one result value.
	Name string `json:"name"`
}

// ExportedOutput is a registered exported result type whose identifier must stay host-local.
type ExportedOutput struct {
	// Name is one result value.
	Name string `json:"name"`
}

// NamedItem is a nested catalog result whose Go identifier must stay host-local.
type NamedItem struct {
	// ID is the nested row identifier.
	ID string `json:"id"`

	// Active reports whether the row survives filtering.
	Active bool `json:"active"`

	// Score is the nested finite floating-point field.
	Score float64 `json:"score"`
}

// compositeTestOutput is a list-valued catalog result used to lock derived notation.
type compositeTestOutput struct {
	// Items is the compiled list of nested objects.
	Items []NamedItem `json:"items"`
}

// TestBuildValidatesEveryRegistrationBeforeFiltering proves disabled entries cannot hide invalid contracts.
func TestBuildValidatesEveryRegistrationBeforeFiltering(t *testing.T) {
	registration := validRegistration("cap.bad", "records.bad", "Bad record")
	registration.Plan = nil

	_, err := Build([]Registration{registration}, testOptions("cap.bad"))

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRegistration)
	assert.Contains(t, err.Error(), "plan")
}

// TestBuildRejectsInvalidMetadataDuplicatesAndNamespaceCollisions proves initialization fails closed.
func TestBuildRejectsInvalidMetadataDuplicatesAndNamespaceCollisions(t *testing.T) {
	valid := validRegistration("cap.one", "records.one", "First record")
	tests := []struct {
		// name identifies the invalid catalog.
		name string

		// registrations contains the candidate capabilities.
		registrations []Registration

		// options configures filtering and search.
		options Options

		// target is the expected classified error.
		target error
	}{
		{
			name:          "nil handler",
			registrations: []Registration{withoutHandler(valid)},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "blank ID",
			registrations: []Registration{withID(valid, "")},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "undotted name",
			registrations: []Registration{withName(valid, "records")},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "invalid name",
			registrations: []Registration{withName(valid, "records.not-valid")},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "reserved name segment",
			registrations: []Registration{withName(valid, "records.import")},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "non-ASCII digit",
			registrations: []Registration{withName(valid, "records.a١")},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "blank summary",
			registrations: []Registration{withSummary(valid, "")},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "blank description",
			registrations: []Registration{withDescription(valid, "")},
			options:       testOptions(),
			target:        ErrInvalidRegistration,
		},
		{name: "duplicate ID", registrations: []Registration{
			valid,
			validRegistration("cap.one", "records.two", "Second record"),
		}, options: testOptions(), target: ErrInvalidRegistration},
		{name: "duplicate name", registrations: []Registration{
			valid,
			validRegistration("cap.two", "records.one", "Second record"),
		}, options: testOptions(), target: ErrInvalidRegistration},
		{name: "namespace collision", registrations: []Registration{
			validRegistration("cap.lookup", "records.lookup", "Lookup records"),
			validRegistration("cap.detail", "records.lookup.detail", "Lookup detail"),
		}, options: testOptions(), target: ErrInvalidRegistration},
		{
			name:          "unknown disabled ID",
			registrations: []Registration{valid},
			options:       testOptions("cap.unknown"),
			target:        ErrUnknownDisabledCapability,
		},
		{
			name:          "duplicate disabled ID",
			registrations: []Registration{valid},
			options:       testOptions("cap.one", "cap.one"),
			target:        ErrInvalidRegistration,
		},
		{
			name:          "invalid query limit",
			registrations: []Registration{valid},
			options:       Options{MaxSearchResults: 1},
			target:        ErrInvalidRegistration,
		},
		{
			name:          "invalid result limit",
			registrations: []Registration{valid},
			options:       Options{MaxSearchQueryBytes: 10},
			target:        ErrInvalidRegistration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Build(tt.registrations, tt.options)

			require.Error(t, err)
			require.ErrorIs(t, err, tt.target)
		})
	}
}

// TestBuildFiltersOnceAndDerivesEverySurface proves disabled capabilities have no live representation.
func TestBuildFiltersOnceAndDerivesEverySurface(t *testing.T) {
	catalog, err := Build([]Registration{
		validRegistration("cap.zeta", "teams.zeta", "Zeta team"),
		validRegistration("cap.disabled", "records.disabled", "Disabled record"),
		validRegistration("cap.alpha", "records.alpha", "Alpha record"),
		{
			ID:          "cap.composite",
			Name:        "records.composite",
			Summary:     "Composite record",
			Description: "Composite record full description.",
			Plan:        mustCompileCompositePlan(),
			Invoke: func(context.Context, authz.Subject, any) (any, error) {
				return compositeTestOutput{}, nil
			},
		},
	}, testOptions("cap.disabled"))
	require.NoError(t, err)

	entries := catalog.Entries()
	require.Len(t, entries, 3)
	assert.Equal(t, []string{"records.alpha", "records.composite", "teams.zeta"}, []string{
		entries[0].Name,
		entries[1].Name,
		entries[2].Name,
	})
	_, foundByName := catalog.Lookup("records.disabled")
	_, foundByID := catalog.LookupID("cap.disabled")
	_, foundDisabledDescription := catalog.Describe("records.disabled")
	disabledSearch, err := catalog.Search("disabled")
	require.NoError(t, err)
	assert.False(t, foundByName)
	assert.False(t, foundByID)
	assert.False(t, foundDisabledDescription)
	assert.Empty(t, disabledSearch.Results)
	assert.False(t, disabledSearch.Truncated)

	recordSearch, err := catalog.Search("record")
	require.NoError(t, err)
	require.Len(t, recordSearch.Results, 2)
	assert.Equal(t, []string{"records.alpha", "records.composite"}, []string{
		recordSearch.Results[0].Name,
		recordSearch.Results[1].Name,
	})
	assert.False(t, recordSearch.Truncated)

	bindings := catalog.NamespaceBindings()
	require.Len(t, bindings, 3)
	assert.Equal(t, []string{"records"}, bindings[0].Segments)
	assert.Equal(t, "alpha", bindings[0].Function)
	assert.Same(t, entries[0].Plan, bindings[0].Capability.Plan)

	description, foundDescription := catalog.Describe("records.alpha")
	require.True(t, foundDescription)
	assert.Equal(t, "Alpha record", description.Summary)
	assert.Equal(t, "Alpha record full description.", description.Description)
	assert.Equal(t, "records.alpha(*, org: str, limit: int | None)", description.Signature)
	require.Len(t, description.Input, 2)
	assert.Equal(t, "org", description.Input[0].Name)
	assert.Equal(t, "str", description.Input[0].Type)
	assert.True(t, description.Input[0].Required)
	assert.Equal(t, "limit", description.Input[1].Name)
	assert.Equal(t, "int | None", description.Input[1].Type)
	assert.False(t, description.Input[1].Required)
	require.Len(t, description.Output, 1)
	assert.Equal(t, "name", description.Output[0].Name)
	assert.Equal(t, "str", description.Output[0].Type)
	assert.True(t, description.Output[0].Required)

	compositeDescription, foundComposite := catalog.Describe("records.composite")
	require.True(t, foundComposite)
	assert.Equal(t, "records.composite(*, org: str, limit: int | None)", compositeDescription.Signature)
	require.Len(t, compositeDescription.Output, 1)
	assert.Equal(t, "items", compositeDescription.Output[0].Name)
	assert.Equal(t, "list[{id: str, active: bool, score: float}]", compositeDescription.Output[0].Type)
	assert.True(t, compositeDescription.Output[0].Required)
	assertDescriptionOmitsOutputTypeNames(t, compositeDescription, "NamedItem", "compositeTestOutput")
	compositeDescription.Input[0].Name = "mutated"
	compositeDescription.Input[0].Type = "mutated"
	compositeDescription.Output[0].Name = "mutated"
	compositeDescription.Output[0].Type = "mutated"
	freshComposite, foundComposite := catalog.Describe("records.composite")
	require.True(t, foundComposite)
	assert.Equal(t, "org", freshComposite.Input[0].Name)
	assert.Equal(t, "str", freshComposite.Input[0].Type)
	assert.Equal(t, "items", freshComposite.Output[0].Name)
	assert.Equal(t, "list[{id: str, active: bool, score: float}]", freshComposite.Output[0].Type)

	description.Input[0].Name = "mutated"
	freshDescription, foundDescription := catalog.Describe("records.alpha")
	require.True(t, foundDescription)
	assert.Equal(t, "org", freshDescription.Input[0].Name)

	entries[0].Name = "mutated"
	bindings[0].Segments[0] = "mutated"
	assert.Equal(t, "records.alpha", catalog.Entries()[0].Name)
	assert.Equal(t, []string{"records"}, catalog.NamespaceBindings()[0].Segments)
}

// TestSearchIsSortedNormalizedAndBounded proves deterministic discovery from the filtered catalog.
func TestSearchIsSortedNormalizedAndBounded(t *testing.T) {
	options := testOptions()
	options.MaxSearchResults = 2
	catalog, err := Build([]Registration{
		validRegistration("cap.zeta", "records.zeta", "Alpha Zeta record"),
		validRegistration("cap.beta", "records.beta", "Alpha Beta record"),
		validRegistration("cap.alpha", "records.alpha", "Alpha primary record"),
	}, options)
	require.NoError(t, err)

	response, err := catalog.Search("  ALPHA  ")

	require.NoError(t, err)
	require.Len(t, response.Results, 2)
	assert.Equal(t, []string{"records.alpha", "records.beta"}, []string{
		response.Results[0].Name,
		response.Results[1].Name,
	})
	assert.True(t, response.Truncated)
	assert.Equal(t, "records.alpha(*, org: str, limit: int | None)", response.Results[0].Signature)

	compound, err := catalog.Search("alpha record")
	require.NoError(t, err)
	require.NotEmpty(t, compound.Results)
	assert.Equal(t, "records.alpha", compound.Results[0].Name)

	empty, err := catalog.Search("   ")
	require.NoError(t, err)
	require.NotNil(t, empty.Results)
	assert.Empty(t, empty.Results)
	assert.False(t, empty.Truncated)

	_, err = catalog.Search("query exceeding the configured byte budget")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSearchQueryLimit)
}

// TestSearchAndDescribeOmitHostOutputTypeNames proves exported and unexported
// Go output identifiers never appear on catalog Search or Describe surfaces.
func TestSearchAndDescribeOmitHostOutputTypeNames(t *testing.T) {
	catalog, err := Build([]Registration{
		validRegistration("cap.alpha", "records.alpha", "Alpha record"),
		{
			ID:          "cap.status",
			Name:        "health.status",
			Summary:     "Health status",
			Description: "Health status full description.",
			Plan:        mustCompileExportedOutputPlan(),
			Invoke: func(context.Context, authz.Subject, any) (any, error) {
				return ExportedOutput{Name: "ok"}, nil
			},
		},
	}, testOptions())
	require.NoError(t, err)

	const unexportedName = "testOutput"
	const exportedName = "ExportedOutput"

	results, err := catalog.Search("alpha")
	require.NoError(t, err)
	require.Len(t, results.Results, 1)
	assert.Equal(t, "records.alpha(*, org: str, limit: int | None)", results.Results[0].Signature)
	assertSearchOmitsOutputTypeNames(t, results.Results[0], unexportedName, exportedName)

	description, found := catalog.Describe("records.alpha")
	require.True(t, found)
	assert.Equal(t, "records.alpha(*, org: str, limit: int | None)", description.Signature)
	require.Len(t, description.Output, 1)
	assert.Equal(t, "name", description.Output[0].Name)
	assert.Equal(t, "str", description.Output[0].Type)
	assertDescriptionOmitsOutputTypeNames(t, description, unexportedName, exportedName)

	statusResults, err := catalog.Search("health")
	require.NoError(t, err)
	require.Len(t, statusResults.Results, 1)
	assert.Equal(t, "health.status()", statusResults.Results[0].Signature)
	assertSearchOmitsOutputTypeNames(t, statusResults.Results[0], unexportedName, exportedName)

	statusDescription, found := catalog.Describe("health.status")
	require.True(t, found)
	assert.Equal(t, "health.status()", statusDescription.Signature)
	require.Len(t, statusDescription.Output, 1)
	assert.Equal(t, "name", statusDescription.Output[0].Name)
	assert.Equal(t, "str", statusDescription.Output[0].Type)
	assertDescriptionOmitsOutputTypeNames(t, statusDescription, unexportedName, exportedName)
}

// TestCatalogSupportsConcurrentReadOnlyUse proves cloned views and immutable indexes are race-safe.
func TestCatalogSupportsConcurrentReadOnlyUse(t *testing.T) {
	catalog, err := Build([]Registration{
		validRegistration("cap.alpha", "records.alpha", "Alpha record"),
		validRegistration("cap.beta", "records.beta", "Beta record"),
	}, testOptions())
	require.NoError(t, err)

	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			for range 100 {
				entries := catalog.Entries()
				entries[0].Name = "local mutation"
				_, _ = catalog.Lookup("records.alpha")
				_, _ = catalog.LookupID("cap.beta")
				_, _ = catalog.Search("record")
				bindings := catalog.NamespaceBindings()
				bindings[0].Segments[0] = "local mutation"
			}
		})
	}
	workers.Wait()

	assert.Equal(t, "records.alpha", catalog.Entries()[0].Name)
}

// validRegistration constructs one valid test registration from the once-compiled plan.
func validRegistration(id string, name string, summary string) Registration {
	return Registration{
		ID:          id,
		Name:        name,
		Summary:     summary,
		Description: summary + " full description.",
		Plan:        mustCompileTestPlan(),
		Invoke: func(context.Context, authz.Subject, any) (any, error) {
			return testOutput{Name: name}, nil
		},
	}
}

// testOptions constructs valid bounded catalog options with optional disabled IDs.
func testOptions(disabled ...string) Options {
	return Options{
		DisabledCapabilities: disabled,
		MaxSearchQueryBytes:  32,
		MaxSearchResults:     10,
	}
}

// withoutHandler returns a copy without a handler.
func withoutHandler(registration Registration) Registration {
	registration.Invoke = nil
	return registration
}

// withID returns a copy with the selected stable ID.
func withID(registration Registration, id string) Registration {
	registration.ID = id
	return registration
}

// withName returns a copy with the selected capability name.
func withName(registration Registration, name string) Registration {
	registration.Name = name
	return registration
}

// withSummary returns a copy with the selected summary.
func withSummary(registration Registration, summary string) Registration {
	registration.Summary = summary
	return registration
}

// withDescription returns a copy with the selected description.
func withDescription(registration Registration, description string) Registration {
	registration.Description = description
	return registration
}

// mustCompileTestPlan compiles the catalog test input and output types.
func mustCompileTestPlan() *binding.Plan {
	plan, err := binding.CompileFor[testInput, testOutput]()
	if err != nil {
		panic(err)
	}
	return plan
}

// mustCompileExportedOutputPlan compiles an empty input with an exported output type.
func mustCompileExportedOutputPlan() *binding.Plan {
	plan, err := binding.CompileFor[struct{}, ExportedOutput]()
	if err != nil {
		panic(err)
	}
	return plan
}

// mustCompileCompositePlan compiles the catalog test input with a list-valued output.
func mustCompileCompositePlan() *binding.Plan {
	plan, err := binding.CompileFor[testInput, compositeTestOutput]()
	if err != nil {
		panic(err)
	}
	return plan
}

// assertSearchOmitsOutputTypeNames requires a Search result to omit host Go output identifiers.
func assertSearchOmitsOutputTypeNames(t *testing.T, result SearchResult, forbidden ...string) {
	t.Helper()
	for _, name := range forbidden {
		assert.NotContains(t, result.Name, name)
		assert.NotContains(t, result.Signature, name)
		assert.NotContains(t, result.Summary, name)
	}
}

// assertDescriptionOmitsOutputTypeNames requires a Describe result to omit host Go output identifiers.
func assertDescriptionOmitsOutputTypeNames(t *testing.T, description Description, forbidden ...string) {
	t.Helper()
	for _, name := range forbidden {
		assert.NotContains(t, description.Name, name)
		assert.NotContains(t, description.Signature, name)
		assert.NotContains(t, description.Summary, name)
		assert.NotContains(t, description.Description, name)
		for _, field := range description.Input {
			assert.NotContains(t, field.Name, name)
			assert.NotContains(t, field.Type, name)
		}
		for _, field := range description.Output {
			assert.NotContains(t, field.Name, name)
			assert.NotContains(t, field.Type, name)
		}
	}
}
