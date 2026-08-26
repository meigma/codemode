package codemode_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
)

// TestDefaultLimitsAreValid proves every exact default budget is positive.
func TestDefaultLimitsAreValid(t *testing.T) {
	limits := codemode.DefaultLimits()

	require.NoError(t, limits.Validate())
	assert.Equal(t, 65_536, limits.MaxSourceBytes)
	assert.Equal(t, uint64(1_000_000), limits.MaxExecutionSteps)
	assert.Equal(t, 5*time.Second, limits.MaxExecutionTime)
	assert.Equal(t, uint64(100), limits.MaxNativeCalls)
	assert.Equal(t, 32, limits.MaxValueDepth)
	assert.Equal(t, 1_048_576, limits.MaxValueBytes)
	assert.Equal(t, 8_388_608, limits.MaxIntermediateValueBytes)
	assert.Equal(t, 256, limits.MaxSearchQueryBytes)
	assert.Equal(t, 20, limits.MaxSearchResults)
	assert.Equal(t, 8, limits.MaxConcurrentExecutions)
}

// TestBuildDefaultsZeroLimitFields proves zero values are bounded defaults and
// a partial override does not require restating unrelated budgets.
func TestBuildDefaultsZeroLimitFields(t *testing.T) {
	defaultBuilder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})

	defaultServer, err := defaultBuilder.Build()

	require.NoError(t, err)
	require.NotNil(t, defaultServer)

	partialBuilder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     codemode.Limits{MaxSearchResults: 1},
	})
	codemode.Register(partialBuilder, validBuilderCapability("cap.one", "records.one"))
	codemode.Register(partialBuilder, validBuilderCapability("cap.two", "records.two"))

	partialServer, err := partialBuilder.Build()

	require.NoError(t, err)
	response, err := partialServer.Search("records")
	require.NoError(t, err)
	require.NotNil(t, response.Results)
	assert.Len(t, response.Results, 1)
	assert.True(t, response.Truncated)
}

// TestLimitsRejectNonPositiveValues proves zero never selects an unlimited budget.
func TestLimitsRejectNonPositiveValues(t *testing.T) {
	tests := []struct {
		// name identifies the rejected limit.
		name string

		// mutate makes one limit invalid.
		mutate func(*codemode.Limits)

		// field is the expected diagnostic field name.
		field string
	}{
		{
			name:   "source bytes",
			mutate: func(limits *codemode.Limits) { limits.MaxSourceBytes = 0 },
			field:  "MaxSourceBytes",
		},
		{
			name:   "execution steps",
			mutate: func(limits *codemode.Limits) { limits.MaxExecutionSteps = 0 },
			field:  "MaxExecutionSteps",
		},
		{
			name:   "execution time",
			mutate: func(limits *codemode.Limits) { limits.MaxExecutionTime = 0 },
			field:  "MaxExecutionTime",
		},
		{
			name:   "native calls",
			mutate: func(limits *codemode.Limits) { limits.MaxNativeCalls = 0 },
			field:  "MaxNativeCalls",
		},
		{
			name:   "value depth",
			mutate: func(limits *codemode.Limits) { limits.MaxValueDepth = 0 },
			field:  "MaxValueDepth",
		},
		{
			name:   "value bytes",
			mutate: func(limits *codemode.Limits) { limits.MaxValueBytes = 0 },
			field:  "MaxValueBytes",
		},
		{
			name:   "intermediate value bytes",
			mutate: func(limits *codemode.Limits) { limits.MaxIntermediateValueBytes = 0 },
			field:  "MaxIntermediateValueBytes",
		},
		{
			name:   "search query bytes",
			mutate: func(limits *codemode.Limits) { limits.MaxSearchQueryBytes = 0 },
			field:  "MaxSearchQueryBytes",
		},
		{
			name:   "search results",
			mutate: func(limits *codemode.Limits) { limits.MaxSearchResults = 0 },
			field:  "MaxSearchResults",
		},
		{
			name:   "concurrent executions",
			mutate: func(limits *codemode.Limits) { limits.MaxConcurrentExecutions = 0 },
			field:  "MaxConcurrentExecutions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := codemode.DefaultLimits()
			tt.mutate(&limits)

			err := limits.Validate()
			require.Error(t, err)
			require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
			assert.Contains(t, err.Error(), tt.field)
		})
	}
}
