package binding

import (
	"encoding/json"
	"math"
	"math/big"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// TestFromStarlarkAcceptsTheNeutralValueSurface proves every allowed Starlark category becomes a process-neutral value.
func TestFromStarlarkAcceptsTheNeutralValueSurface(t *testing.T) {
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

	converted, err := FromStarlark(value, 4, 1024)

	require.NoError(t, err)
	assert.Equal(t, []any{
		"text",
		int64(7),
		1.25,
		[]any{false},
		map[string]any{"flag": true, "none": nil},
	}, converted)
}

// TestToStarlarkPreservesNumericIdentity proves int64 and finite float64 survive both conversion directions.
func TestToStarlarkPreservesNumericIdentity(t *testing.T) {
	tests := []struct {
		// name identifies the numeric identity under test.
		name string

		// value is the process-neutral source.
		value any
	}{
		{name: "minimum int64", value: int64(math.MinInt64)},
		{name: "maximum int64", value: int64(math.MaxInt64)},
		{name: "integral float", value: 1.0},
		{name: "fractional float", value: 1.25},
		{name: "nested numeric object", value: map[string]any{
			"count": int64(3),
			"score": 1.0,
			"items": []any{int64(0), 2.5},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starlarkValue, err := ToStarlark(tt.value, 4, 1024)
			require.NoError(t, err)

			converted, err := FromStarlark(starlarkValue, 4, 1024)
			require.NoError(t, err)
			assert.Equal(t, tt.value, converted)
			assertNeutralTypes(t, converted)
		})
	}
}

// TestValidateValueAcceptsNeutralValues proves validation accepts the shared process-neutral matrix.
func TestValidateValueAcceptsNeutralValues(t *testing.T) {
	tests := []struct {
		// name identifies the accepted value.
		name string

		// value is the process-neutral source.
		value any
	}{
		{name: "nil", value: nil},
		{name: "bool", value: true},
		{name: "string", value: "text"},
		{name: "int64", value: int64(7)},
		{name: "float64", value: 1.25},
		{name: "empty list", value: []any{}},
		{name: "empty object", value: map[string]any{}},
		{name: "nested values", value: []any{
			nil,
			false,
			"text",
			int64(1),
			1.5,
			[]any{int64(2)},
			map[string]any{"inner": true},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, ValidateValue(tt.value, 4, 1024))
		})
	}
}

// TestValueConversionRejectsUnsupportedValues proves executable, numeric, and unsafe values stay classified.
func TestValueConversionRejectsUnsupportedValues(t *testing.T) {
	overflow := new(big.Int).Lsh(big.NewInt(1), 80)
	nonStringKey := starlark.NewDict(1)
	require.NoError(t, nonStringKey.SetKey(starlark.MakeInt(1), starlark.String("value")))
	var nilList *starlark.List
	var nilDict *starlark.Dict

	starlarkTests := []struct {
		// name identifies the unsafe Starlark value.
		name string

		// value is the Starlark source presented for conversion.
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

	for _, tt := range starlarkTests {
		t.Run("from "+tt.name, func(t *testing.T) {
			_, err := FromStarlark(tt.value, 4, 1024)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrUnsupportedValue)
		})
	}

	goTests := []struct {
		// name identifies the unsafe Go value.
		name string

		// value is the process-neutral impostor presented for conversion.
		value any
	}{
		{name: "json.Number", value: json.Number("1")},
		{name: "int", value: 1},
		{name: "uint64", value: uint64(1)},
		{name: "float32", value: float32(1)},
		{name: "NaN", value: math.NaN()},
		{name: "infinity", value: math.Inf(1)},
		{name: "string map", value: map[string]string{"key": "value"}},
		{name: "typed slice", value: []string{"value"}},
	}

	for _, tt := range goTests {
		t.Run("validate "+tt.name, func(t *testing.T) {
			err := ValidateValue(tt.value, 4, 1024)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrUnsupportedValue)
		})
		t.Run("to "+tt.name, func(t *testing.T) {
			_, err := ToStarlark(tt.value, 4, 1024)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrUnsupportedValue)
		})
	}
}

// TestValueConversionEnforcesPositiveDepthAndNodeLimits proves inclusive depth and node budgets are hard limits.
func TestValueConversionEnforcesPositiveDepthAndNodeLimits(t *testing.T) {
	converted, err := FromStarlark(starlark.String("value"), 1, 1024)
	require.NoError(t, err)
	assert.Equal(t, "value", converted)
	require.NoError(t, ValidateValue("value", 1, 1024))

	converted, err = FromStarlark(starlark.None, 1, 1024)
	require.NoError(t, err)
	assert.Nil(t, converted)
	require.NoError(t, ValidateValue(nil, 1, 1024))

	shallow := starlark.NewList([]starlark.Value{starlark.String("value")})
	converted, err = FromStarlark(shallow, 2, 1024)
	require.NoError(t, err)
	assert.Equal(t, []any{"value"}, converted)
	require.NoError(t, ValidateValue([]any{"value"}, 2, 1024))

	deepStarlark := starlark.NewList([]starlark.Value{
		starlark.NewList([]starlark.Value{starlark.String("value")}),
	})
	deepNeutral := []any{[]any{"value"}}

	_, err = FromStarlark(deepStarlark, 2, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "depth")

	err = ValidateValue(deepNeutral, 2, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "depth")

	_, err = ToStarlark(deepNeutral, 2, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "depth")

	_, err = FromStarlark(starlark.Tuple{starlark.String("a"), starlark.String("b")}, 2, 2)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "node budget")

	_, err = FromStarlark(starlark.None, 0, 10)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)

	_, err = ToStarlark(nil, 1, 0)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
}

// TestValueConversionRejectsCycles proves recursive containers fail before destination materialization.
func TestValueConversionRejectsCycles(t *testing.T) {
	list := starlark.NewList(nil)
	require.NoError(t, list.Append(list))

	_, err := FromStarlark(list, 10, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "cyclic")

	cyclicList := []any{nil}
	cyclicList[0] = cyclicList
	err = ValidateValue(cyclicList, 10, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "cyclic")

	_, err = ToStarlark(cyclicList, 10, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "cyclic")

	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	err = ValidateValue(cyclicMap, 10, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "cyclic")
}

// TestValueConversionAcceptsNestedReslices proves shared backing arrays with different lengths are not cycles.
func TestValueConversionAcceptsNestedReslices(t *testing.T) {
	parent := []any{"alpha", "beta", "gamma"}
	nested := []any{parent[1:], parent[:2], []any{}}

	require.NoError(t, ValidateValue(nested, 4, 16))

	converted, err := ToStarlark(nested, 4, 16)
	require.NoError(t, err)
	roundTrip, err := FromStarlark(converted, 4, 16)
	require.NoError(t, err)
	assert.Equal(t, []any{[]any{"beta", "gamma"}, []any{"alpha", "beta"}, []any{}}, roundTrip)
}

// TestValueConversionBoundsSharedSubstructure proves a compact DAG cannot expand beyond the node budget.
func TestValueConversionBoundsSharedSubstructure(t *testing.T) {
	var value starlark.Value = starlark.String("x")
	for range 10 {
		value = starlark.Tuple{value, value}
	}

	_, err := FromStarlark(value, 16, 64)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "node budget")

	var nested any = "x"
	for range 10 {
		nested = []any{nested, nested}
	}
	err = ValidateValue(nested, 16, 64)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "node budget")
}

// TestToStarlarkInsertsDictionaryKeysInSortedOrder proves object conversion is deterministic.
func TestToStarlarkInsertsDictionaryKeysInSortedOrder(t *testing.T) {
	converted, err := ToStarlark(map[string]any{
		"zeta":  int64(1),
		"alpha": int64(2),
		"mu":    int64(3),
	}, 2, 8)
	require.NoError(t, err)

	dictionary, ok := converted.(*starlark.Dict)
	require.True(t, ok)
	assert.Equal(t, []string{"alpha", "mu", "zeta"}, dictionaryKeys(t, dictionary))
}

// TestValueConversionRejectsOversizedContainersBeforeAllocation proves oversized
// sources are rejected before destination materialization.
func TestValueConversionRejectsOversizedContainersBeforeAllocation(t *testing.T) {
	const (
		sourceLen     = 128_000
		maxDepth      = 4
		maxNodes      = 1024
		maxAllocBytes = 1 << 20
	)

	item := starlark.String("x")
	tuple := make(starlark.Tuple, sourceLen)
	listItems := make([]starlark.Value, sourceLen)
	neutralList := make([]any, sourceLen)
	neutralMap := make(map[string]any, sourceLen)
	for index := range sourceLen {
		tuple[index] = item
		listItems[index] = item
		neutralList[index] = "x"
		neutralMap[strconv.Itoa(index)] = "x"
	}
	list := starlark.NewList(listItems)

	dictionary := starlark.NewDict(sourceLen)
	for index := range sourceLen {
		require.NoError(t, dictionary.SetKey(starlark.String(strconv.Itoa(index)), item))
	}

	fromTests := []struct {
		// name identifies the oversized Starlark container kind.
		name string

		// value is the prebuilt source presented for conversion.
		value starlark.Value
	}{
		{name: "tuple", value: tuple},
		{name: "list", value: list},
		{name: "dictionary", value: dictionary},
	}

	for _, tt := range fromTests {
		t.Run("from "+tt.name, func(t *testing.T) {
			allocated, err := measureFromStarlarkGrowth(t, tt.value, maxDepth, maxNodes)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrValueLimit)
			assert.Contains(t, err.Error(), "value exceeds byte-derived node budget")
			assert.LessOrEqual(
				t,
				allocated,
				uint64(maxAllocBytes),
				"FromStarlark allocated %d bytes converting an oversized %s; destination materialization must not scale with source length %d",
				allocated,
				tt.name,
				sourceLen,
			)
			runtime.KeepAlive(tt.value)
		})
	}

	toTests := []struct {
		// name identifies the oversized Go container kind.
		name string

		// value is the prebuilt source presented for conversion.
		value any
	}{
		{name: "list", value: neutralList},
		{name: "dictionary", value: neutralMap},
	}

	for _, tt := range toTests {
		t.Run("to "+tt.name, func(t *testing.T) {
			allocated, err := measureToStarlarkGrowth(t, tt.value, maxDepth, maxNodes)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrValueLimit)
			assert.Contains(t, err.Error(), "value exceeds byte-derived node budget")
			assert.LessOrEqual(
				t,
				allocated,
				uint64(maxAllocBytes),
				"ToStarlark allocated %d bytes converting an oversized %s; destination materialization must not scale with source length %d",
				allocated,
				tt.name,
				sourceLen,
			)
			runtime.KeepAlive(tt.value)
		})
	}

	runtime.KeepAlive(fromTests)
	runtime.KeepAlive(toTests)
}

// measureFromStarlarkGrowth reports heap bytes allocated by FromStarlark.
func measureFromStarlarkGrowth(t *testing.T, value starlark.Value, maxDepth int, maxNodes int) (uint64, error) {
	t.Helper()

	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := FromStarlark(value, maxDepth, maxNodes)
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(value)
	return after.TotalAlloc - before.TotalAlloc, err
}

// measureToStarlarkGrowth reports heap bytes allocated by ToStarlark.
func measureToStarlarkGrowth(t *testing.T, value any, maxDepth int, maxNodes int) (uint64, error) {
	t.Helper()

	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := ToStarlark(value, maxDepth, maxNodes)
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(value)
	return after.TotalAlloc - before.TotalAlloc, err
}

// dictionaryKeys returns the Starlark dictionary's insertion-ordered keys.
func dictionaryKeys(t *testing.T, dictionary *starlark.Dict) []string {
	t.Helper()

	keys := make([]string, 0, dictionary.Len())
	iterator := dictionary.Iterate()
	defer iterator.Done()
	var key starlark.Value
	for iterator.Next(&key) {
		name, ok := starlark.AsString(key)
		require.True(t, ok)
		keys = append(keys, name)
	}
	return keys
}

// assertNeutralTypes proves converted values keep the shared process-neutral Go types.
func assertNeutralTypes(t *testing.T, value any) {
	t.Helper()

	switch typed := value.(type) {
	case nil, bool, string, int64:
		return
	case float64:
		require.False(t, math.IsNaN(typed) || math.IsInf(typed, 0))
	case []any:
		for _, item := range typed {
			assertNeutralTypes(t, item)
		}
	case map[string]any:
		for _, item := range typed {
			assertNeutralTypes(t, item)
		}
	default:
		require.Failf(t, "unexpected process-neutral type", "got %T", value)
	}
}
