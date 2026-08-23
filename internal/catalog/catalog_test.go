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
	}, testOptions("cap.disabled"))
	require.NoError(t, err)

	entries := catalog.Entries()
	require.Len(t, entries, 2)
	assert.Equal(t, []string{"records.alpha", "teams.zeta"}, []string{entries[0].Name, entries[1].Name})
	_, foundByName := catalog.Lookup("records.disabled")
	_, foundByID := catalog.LookupID("cap.disabled")
	_, foundDisabledDescription := catalog.Describe("records.disabled")
	disabledSearch, err := catalog.Search("disabled")
	require.NoError(t, err)
	assert.False(t, foundByName)
	assert.False(t, foundByID)
	assert.False(t, foundDisabledDescription)
	assert.Empty(t, disabledSearch)

	bindings := catalog.NamespaceBindings()
	require.Len(t, bindings, 2)
	assert.Equal(t, []string{"records"}, bindings[0].Segments)
	assert.Equal(t, "alpha", bindings[0].Function)
	assert.Same(t, entries[0].Plan, bindings[0].Capability.Plan)

	description, foundDescription := catalog.Describe("records.alpha")
	require.True(t, foundDescription)
	assert.Equal(t, "Alpha record", description.Summary)
	assert.Equal(t, "Alpha record full description.", description.Description)
	assert.Equal(t, "records.alpha(*, org: str, limit: int | None) -> testOutput", description.Signature)
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

	results, err := catalog.Search("  ALPHA  ")

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, []string{"records.alpha", "records.beta"}, []string{results[0].Name, results[1].Name})
	assert.Equal(t, "records.alpha(*, org: str, limit: int | None) -> testOutput", results[0].Signature)

	nonContiguous, err := catalog.Search("alpha record")
	require.NoError(t, err)
	assert.Empty(t, nonContiguous)

	crossBoundary, err := catalog.Search("alpha alpha")
	require.NoError(t, err)
	assert.Empty(t, crossBoundary)

	empty, err := catalog.Search("   ")
	require.NoError(t, err)
	assert.Empty(t, empty)

	_, err = catalog.Search("query exceeding the configured byte budget")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSearchQueryLimit)
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
