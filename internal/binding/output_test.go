package binding

import (
	"math"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// generousOutputDepth is a depth budget that never binds representative values.
	generousOutputDepth = 16

	// generousOutputNodes is a node budget that never binds representative values.
	generousOutputNodes = 1024
)

// TestConvertOutputUsesCompiledFields proves handler output conversion follows the immutable output plan.
func TestConvertOutputUsesCompiledFields(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	output := representativeOutput{Name: "alpha", Count: 3, Active: true, Score: 1.5}

	converted, err := plan.ConvertOutput(output, generousOutputDepth, generousOutputNodes)
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

	converted, err := plan.ConvertOutput(output, generousOutputDepth, generousOutputNodes)
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
			_, err := plan.ConvertOutput(tt.output, generousOutputDepth, generousOutputNodes)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrUnsupportedValue)
		})
	}
}

// TestConvertOutputAcceptsTheCompositeUniverse proves nested values become process-neutral data.
func TestConvertOutputAcceptsTheCompositeUniverse(t *testing.T) {
	note := "keep"
	extra := "more"
	plan, err := CompileFor[representativeInput, compositeOutput]()
	require.NoError(t, err)
	output := compositeOutput{
		Items:   []nestedItem{{Title: "alpha", Score: 1.5}},
		Tags:    [2]string{"a", "b"},
		ByID:    map[string]nestedItem{"z": {Title: "zeta", Score: 2.5}},
		Note:    &note,
		Extra:   &extra,
		Payload: []byte{1, 2},
	}

	converted, err := plan.ConvertOutput(output, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"items":   []any{map[string]any{"title": "alpha", "score": 1.5}},
		"tags":    []any{"a", "b"},
		"by_id":   map[string]any{"z": map[string]any{"title": "zeta", "score": 2.5}},
		"note":    "keep",
		"extra":   "more",
		"payload": []any{int64(1), int64(2)},
	}, converted)
}

// TestConvertOutputNormalizesEveryNumericKind proves integers become int64 and floats become float64.
func TestConvertOutputNormalizesEveryNumericKind(t *testing.T) {
	plan, err := CompileFor[representativeInput, numericOutput]()
	require.NoError(t, err)
	output := numericOutput{
		I: 1, I8: 2, I16: 3, I32: 4, I64: 5,
		U: 6, U8: 7, U16: 8, U32: 9, U64: 10,
		F32: 1.5, F64: 2.5,
	}

	converted, err := plan.ConvertOutput(output, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"i":   int64(1),
		"i8":  int64(2),
		"i16": int64(3),
		"i32": int64(4),
		"i64": int64(5),
		"u":   int64(6),
		"u8":  int64(7),
		"u16": int64(8),
		"u32": int64(9),
		"u64": int64(10),
		"f32": float64(float32(1.5)),
		"f64": 2.5,
	}, converted)
	assert.IsType(t, int64(0), converted["u64"])
	assert.IsType(t, float64(0), converted["f32"])
}

// TestConvertOutputAcceptsNamedAliasesAndByteSequences proves aliases and bytes stay in the integer list surface.
func TestConvertOutputAcceptsNamedAliasesAndByteSequences(t *testing.T) {
	aliasPlan, err := CompileFor[representativeInput, aliasedOutput]()
	require.NoError(t, err)
	converted, err := aliasPlan.ConvertOutput(aliasedOutput{
		Name:   "alpha",
		Count:  3,
		Active: true,
		Score:  1.5,
	}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"name":   "alpha",
		"count":  int64(3),
		"active": true,
		"score":  1.5,
	}, converted)

	bytesPlan, err := CompileFor[representativeInput, bytesOutput]()
	require.NoError(t, err)
	converted, err = bytesPlan.ConvertOutput(bytesOutput{
		Payload: []byte{255},
		Fixed:   [2]byte{1, 2},
	}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"payload": []any{int64(255)},
		"fixed":   []any{int64(1), int64(2)},
	}, converted)

	keyPlan, err := CompileFor[representativeInput, aliasKeyOutput]()
	require.NoError(t, err)
	converted, err = keyPlan.ConvertOutput(aliasKeyOutput{
		ByName: map[namedString]int64{"beta": 9},
	}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"by_name": map[string]any{"beta": int64(9)}}, converted)
}

// TestConvertOutputDistinguishesNilAndEmptyContainers proves nil becomes None and empty stays empty.
func TestConvertOutputDistinguishesNilAndEmptyContainers(t *testing.T) {
	plan, err := CompileFor[representativeInput, compositeOutput]()
	require.NoError(t, err)

	converted, err := plan.ConvertOutput(compositeOutput{
		Items:   nil,
		ByID:    nil,
		Payload: nil,
	}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)
	assert.Nil(t, converted["items"])
	assert.Equal(t, []any{"", ""}, converted["tags"])
	assert.Nil(t, converted["by_id"])
	assert.Nil(t, converted["note"])
	_, hasExtra := converted["extra"]
	assert.False(t, hasExtra)
	assert.Nil(t, converted["payload"])

	converted, err = plan.ConvertOutput(compositeOutput{
		Items:   []nestedItem{},
		ByID:    map[string]nestedItem{},
		Payload: []byte{},
	}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)
	assert.Equal(t, []any{}, converted["items"])
	assert.Equal(t, map[string]any{}, converted["by_id"])
	assert.Equal(t, []any{}, converted["payload"])
}

// TestConvertOutputOmitsOptionalNilPointers proves omitempty is distinct from required nullability.
func TestConvertOutputOmitsOptionalNilPointers(t *testing.T) {
	plan, err := CompileFor[representativeInput, compositeOutput]()
	require.NoError(t, err)
	note := "keep"

	converted, err := plan.ConvertOutput(compositeOutput{Note: &note}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)
	assert.Equal(t, "keep", converted["note"])
	_, hasExtra := converted["extra"]
	assert.False(t, hasExtra)

	converted, err = plan.ConvertOutput(compositeOutput{}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)
	assert.Nil(t, converted["note"])
	_, hasNote := converted["note"]
	assert.True(t, hasNote)
	_, hasExtra = converted["extra"]
	assert.False(t, hasExtra)
}

// TestConvertOutputPreservesNilPointerElements proves list and map omission never applies to elements.
func TestConvertOutputPreservesNilPointerElements(t *testing.T) {
	plan, err := CompileFor[representativeInput, pointerElementOutput]()
	require.NoError(t, err)
	count := int64(7)

	converted, err := plan.ConvertOutput(pointerElementOutput{
		Values: []*int64{&count, nil},
		ByID:   map[string]*nestedItem{"a": {Title: "alpha", Score: 1}, "b": nil},
	}, generousOutputDepth, generousOutputNodes)
	require.NoError(t, err)

	assert.Equal(t, []any{int64(7), nil}, converted["values"])
	byID, ok := converted["by_id"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"title": "alpha", "score": float64(1)}, byID["a"])
	assert.Nil(t, byID["b"])
}

// TestConvertOutputRejectsUnsignedOverflowAndNonFiniteFloats proves invalid numerics stay capability failures.
func TestConvertOutputRejectsUnsignedOverflowAndNonFiniteFloats(t *testing.T) {
	overflowPlan, err := CompileFor[representativeInput, overflowOutput]()
	require.NoError(t, err)
	_, err = overflowPlan.ConvertOutput(
		overflowOutput{Count: uint64(math.MaxInt64) + 1},
		generousOutputDepth,
		generousOutputNodes,
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "output.count")

	floatPlan, err := CompileFor[representativeInput, float32Output]()
	require.NoError(t, err)
	_, err = floatPlan.ConvertOutput(
		float32Output{Score: float32(math.NaN())},
		generousOutputDepth,
		generousOutputNodes,
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "output.score")

	_, err = floatPlan.ConvertOutput(
		float32Output{Score: float32(math.Inf(1))},
		generousOutputDepth,
		generousOutputNodes,
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)

	scalarPlan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	_, err = scalarPlan.ConvertOutput(
		representativeOutput{Score: math.Inf(-1)},
		generousOutputDepth,
		generousOutputNodes,
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
}

// TestConvertOutputRejectsWrongRootType proves only the compiled output type converts.
func TestConvertOutputRejectsWrongRootType(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	_, err = plan.ConvertOutput(compositeOutput{}, generousOutputDepth, generousOutputNodes)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "handler output type")
}

// TestConvertOutputSortsMapsAndUsesStableErrorPaths proves map keys sort before traversal.
func TestConvertOutputSortsMapsAndUsesStableErrorPaths(t *testing.T) {
	mapPlan, err := CompileFor[representativeInput, mapScoreOutput]()
	require.NoError(t, err)
	_, err = mapPlan.ConvertOutput(mapScoreOutput{
		ByID: map[string]float64{"z": 1, "a": math.NaN(), "m": 2},
	}, generousOutputDepth, generousOutputNodes)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), `output.by_id["a"]`)

	listPlan, err := CompileFor[representativeInput, listScoreOutput]()
	require.NoError(t, err)
	_, err = listPlan.ConvertOutput(listScoreOutput{
		Items: []nestedItem{
			{Title: "zero", Score: 1},
			{Title: "one", Score: 2},
			{Title: "two", Score: 3},
			{Title: "three", Score: math.Inf(1)},
		},
	}, generousOutputDepth, generousOutputNodes)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedValue)
	assert.Contains(t, err.Error(), "output.items[3].score")
}

// TestConvertOutputEnforcesDepthAndNodeLimits proves root depth is 1 and scalar fields sit at depth 2.
func TestConvertOutputEnforcesDepthAndNodeLimits(t *testing.T) {
	scalarPlan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	_, err = scalarPlan.ConvertOutput(representativeOutput{Name: "alpha"}, 1, generousOutputNodes)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "depth")

	_, err = scalarPlan.ConvertOutput(representativeOutput{Name: "alpha"}, 2, generousOutputNodes)
	require.NoError(t, err)

	nestedPlan, err := CompileFor[representativeInput, nestedDepthOutput]()
	require.NoError(t, err)
	nested := nestedDepthOutput{Item: nestedItem{Title: "alpha"}}
	_, err = nestedPlan.ConvertOutput(nested, 2, generousOutputNodes)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "depth")

	_, err = nestedPlan.ConvertOutput(nested, 3, generousOutputNodes)
	require.NoError(t, err)

	listPlan, err := CompileFor[representativeInput, listScoreOutput]()
	require.NoError(t, err)
	_, err = listPlan.ConvertOutput(listScoreOutput{
		Items: []nestedItem{{Title: "a"}, {Title: "b"}},
	}, generousOutputDepth, 2)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "node budget")

	_, err = scalarPlan.ConvertOutput(representativeOutput{}, 0, generousOutputNodes)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)

	_, err = scalarPlan.ConvertOutput(representativeOutput{}, generousOutputDepth, 0)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
}

// TestConvertOutputCountsIncludedOptionalFieldsBeforeAllocation proves omitempty inclusion is preflighted.
func TestConvertOutputCountsIncludedOptionalFieldsBeforeAllocation(t *testing.T) {
	plan, err := CompileFor[representativeInput, optionalHeavyOutput]()
	require.NoError(t, err)
	a, b, c := "a", "b", "c"

	_, err = plan.ConvertOutput(optionalHeavyOutput{}, 1, 1)
	require.NoError(t, err)

	_, err = plan.ConvertOutput(optionalHeavyOutput{A: &a, B: &b, C: &c}, 2, 3)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrValueLimit)
	assert.Contains(t, err.Error(), "node budget")
}

// TestConvertOutputRejectsOversizedContainersBeforeAllocation proves reflected containers
// are rejected before proportional destination materialization.
func TestConvertOutputRejectsOversizedContainersBeforeAllocation(t *testing.T) {
	const (
		sourceLen     = 128_000
		maxDepth      = 4
		maxNodes      = 1024
		maxAllocBytes = 1 << 20
	)

	items := make([]string, sourceLen)
	mapped := make(map[string]string, sourceLen)
	structs := make([]nestedItem, sourceLen)
	for index := range sourceLen {
		items[index] = "x"
		mapped[strconv.Itoa(index)] = "x"
		structs[index] = nestedItem{Title: "x"}
	}

	tests := []struct {
		// name identifies the oversized reflected container.
		name string

		// compile builds the plan under test.
		compile func(*testing.T) *Plan

		// output is the oversized handler value.
		output any
	}{
		{
			name: "slice",
			compile: func(t *testing.T) *Plan {
				t.Helper()
				plan, err := CompileFor[representativeInput, hugeSliceOutput]()
				require.NoError(t, err)
				return plan
			},
			output: hugeSliceOutput{Items: items},
		},
		{
			name: "array",
			compile: func(t *testing.T) *Plan {
				t.Helper()
				plan, err := CompileFor[representativeInput, hugeArrayOutput]()
				require.NoError(t, err)
				return plan
			},
			output: hugeArrayOutput{},
		},
		{
			name: "map",
			compile: func(t *testing.T) *Plan {
				t.Helper()
				plan, err := CompileFor[representativeInput, hugeMapOutput]()
				require.NoError(t, err)
				return plan
			},
			output: hugeMapOutput{Items: mapped},
		},
		{
			name: "struct list",
			compile: func(t *testing.T) *Plan {
				t.Helper()
				plan, err := CompileFor[representativeInput, hugeStructOutput]()
				require.NoError(t, err)
				return plan
			},
			output: hugeStructOutput{Items: structs},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := tt.compile(t)
			allocated, err := measureConvertOutputGrowth(t, plan, tt.output, maxDepth, maxNodes)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrValueLimit)
			assert.Contains(t, err.Error(), "value exceeds byte-derived node budget")
			assert.LessOrEqual(
				t,
				allocated,
				uint64(maxAllocBytes),
				"ConvertOutput allocated %d bytes converting an oversized %s; destination materialization must not scale with source length %d",
				allocated,
				tt.name,
				sourceLen,
			)
			runtime.KeepAlive(tt.output)
		})
	}
}

// measureConvertOutputGrowth reports heap bytes allocated by ConvertOutput.
func measureConvertOutputGrowth(t *testing.T, plan *Plan, output any, maxDepth int, maxNodes int) (uint64, error) {
	t.Helper()

	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := plan.ConvertOutput(output, maxDepth, maxNodes)
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(output)
	return after.TotalAlloc - before.TotalAlloc, err
}
