package codemode_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
)

// TestDefaultLimitsAreValid proves every default budget is explicitly positive.
func TestDefaultLimitsAreValid(t *testing.T) {
	require.NoError(t, codemode.DefaultLimits().Validate())
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
			name:   "result bytes",
			mutate: func(limits *codemode.Limits) { limits.MaxResultBytes = 0 },
			field:  "MaxResultBytes",
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
