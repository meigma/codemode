package binding

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// TestConvertOutputUsesCompiledFields proves handler output conversion follows the immutable output plan.
func TestConvertOutputUsesCompiledFields(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	output := representativeOutput{Name: "alpha", Count: 3, Active: true, Score: 1.5}

	value, err := plan.ConvertOutput(output)
	require.NoError(t, err)
	converted, err := ConvertFinal(value, 4, 1024)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"name":   "alpha",
		"count":  int64(3),
		"active": true,
		"score":  1.5,
	}, converted)
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

// TestConvertFinalAcceptsTheMCPValueSurface proves all supported final value categories convert safely.
func TestConvertFinalAcceptsTheMCPValueSurface(t *testing.T) {
	dictionary := starlark.NewDict(2)
	require.NoError(t, dictionary.SetKey(starlark.String("flag"), starlark.True))
	require.NoError(t, dictionary.SetKey(starlark.String("none"), starlark.None))
	value := starlark.Tuple{
		starlark.String("text"),
		starlark.MakeInt64(7),
		starlark.Float(1.25),
		starlark.NewList([]starlark.Value{starlark.False}),
		dictionary,
	}

	converted, err := ConvertFinal(value, 4, 1024)

	require.NoError(t, err)
	assert.Equal(t, []any{
		"text",
		int64(7),
		1.25,
		[]any{false},
		map[string]any{"flag": true, "none": nil},
	}, converted)
}

// TestConvertFinalRejectsUnsupportedAndOverflowingValues proves executable and unsafe values never cross MCP.
func TestConvertFinalRejectsUnsupportedAndOverflowingValues(t *testing.T) {
	overflow := new(big.Int).Lsh(big.NewInt(1), 80)
	nonStringKey := starlark.NewDict(1)
	require.NoError(t, nonStringKey.SetKey(starlark.MakeInt(1), starlark.String("value")))
	var nilList *starlark.List
	var nilDict *starlark.Dict

	tests := []struct {
		// name identifies the unsafe final value.
		name string

		// value is the Starlark value presented for final conversion.
		value starlark.Value
	}{
		{name: "integer overflow", value: starlark.MakeBigInt(overflow)},
		{name: "NaN", value: starlark.Float(math.NaN())},
		{name: "infinity", value: starlark.Float(math.Inf(1))},
		{name: "non-string dictionary key", value: nonStringKey},
		{name: "builtin", value: starlark.NewBuiltin("run", nil)},
		{name: "bytes", value: starlark.Bytes("secret")},
		{name: "nil interface", value: nil},
		{name: "typed nil list", value: nilList},
		{name: "typed nil dictionary", value: nilDict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ConvertFinal(tt.value, 4, 1024)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrUnsupportedValue)
		})
	}
}

// TestConvertFinalEnforcesDepthAndEncodedSize proves inclusive nesting depth and both conversion budgets are positive hard limits.
func TestConvertFinalEnforcesDepthAndEncodedSize(t *testing.T) {
	converted, err := ConvertFinal(starlark.String("value"), 1, 1024)
	require.NoError(t, err)
	assert.Equal(t, "value", converted)

	converted, err = ConvertFinal(starlark.None, 1, 1024)
	require.NoError(t, err)
	assert.Nil(t, converted)

	shallow := starlark.NewList([]starlark.Value{starlark.String("value")})
	converted, err = ConvertFinal(shallow, 2, 1024)
	require.NoError(t, err)
	assert.Equal(t, []any{"value"}, converted)

	deep := starlark.NewList([]starlark.Value{
		starlark.NewList([]starlark.Value{starlark.String("value")}),
	})

	_, err = ConvertFinal(deep, 2, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "depth")

	_, err = ConvertFinal(starlark.String("long result"), 2, 4)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "bytes")

	_, err = ConvertFinal(starlark.None, 0, 10)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
}

// TestConvertFinalRejectsCycles proves recursive containers fail before JSON encoding.
func TestConvertFinalRejectsCycles(t *testing.T) {
	list := starlark.NewList(nil)
	require.NoError(t, list.Append(list))

	_, err := ConvertFinal(list, 10, 1024)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "cyclic")
}

// TestConvertFinalBoundsSharedSubstructure proves a compact DAG cannot expand beyond the result budget.
func TestConvertFinalBoundsSharedSubstructure(t *testing.T) {
	var value starlark.Value = starlark.String("x")
	for range 10 {
		value = starlark.Tuple{value, value}
	}

	_, err := ConvertFinal(value, 16, 64)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "node budget")
}
