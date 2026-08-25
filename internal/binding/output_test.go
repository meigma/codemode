package binding

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertOutputUsesCompiledFields proves handler output conversion follows the immutable output plan.
func TestConvertOutputUsesCompiledFields(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	output := representativeOutput{Name: "alpha", Count: 3, Active: true, Score: 1.5}

	converted, err := plan.ConvertOutput(output)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"name":   "alpha",
		"count":  int64(3),
		"active": true,
		"score":  1.5,
	}, converted)
}

// TestConvertOutputPreservesExactKinds proves handler output becomes process-neutral data without type erasure.
func TestConvertOutputPreservesExactKinds(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	output := representativeOutput{Name: "alpha", Count: 3, Active: true, Score: 1.0}

	converted, err := plan.ConvertOutput(output)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"name":   "alpha",
		"count":  int64(3),
		"active": true,
		"score":  1.0,
	}, converted)
	assert.IsType(t, int64(0), converted["count"])
	assert.IsType(t, float64(0), converted["score"])
}

// TestConvertOutputRejectsTypeDriftAndNonFiniteFloats proves outputs cannot bypass their compiled shape.
func TestConvertOutputRejectsTypeDriftAndNonFiniteFloats(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	tests := []struct {
		// name identifies the invalid handler output.
		name string

		// output is the value presented to the compiled output plan.
		output any
	}{
		{name: "wrong type", output: struct {
			// Name is an intentionally incompatible output field.
			Name string
		}{Name: "alpha"}},
		{name: "pointer", output: &representativeOutput{}},
		{name: "NaN", output: representativeOutput{Score: math.NaN()}},
		{name: "positive infinity", output: representativeOutput{Score: math.Inf(1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := plan.ConvertOutput(tt.output)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrUnsupportedValue)
		})
	}
}
