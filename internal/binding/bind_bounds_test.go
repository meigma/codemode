package binding

import (
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// cubeInput takes a three-level nested list.
type cubeInput struct {
	// Cube is a list of lists of integer lists.
	Cube [][][]int64 `json:"cube"`
}

// groupedInput takes integer lists under model-chosen keys.
type groupedInput struct {
	// Groups maps names to integer lists.
	Groups map[string][]int64 `json:"groups"`
}

// ratioInput takes float32 values directly and inside a list.
type ratioInput struct {
	// Ratio is a required float32.
	Ratio float32 `json:"ratio"`

	// Ratios is a list of float32 values.
	Ratios []float32 `json:"ratios"`
}

// measureAllocations reports heap bytes allocated while run executes.
func measureAllocations(t *testing.T, run func()) uint64 {
	t.Helper()
	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	run()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// starlarkGlobal evaluates source and returns the named global.
func starlarkGlobal(t *testing.T, source string, name string) starlark.Value {
	t.Helper()
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, &starlark.Thread{}, "test.star", source, nil)
	require.NoError(t, err)
	return globals[name]
}

// TestBindChargesAliasedContainersBeforeMaterializing proves repeated references
// to one Starlark list cannot expand past the node budget.
func TestBindChargesAliasedContainersBeforeMaterializing(t *testing.T) {
	plan, err := CompileFor[cubeInput, representativeOutput]()
	require.NoError(t, err)
	cube := starlarkGlobal(t, "a = [0] * 300\nb = [a] * 300\nc = [b] * 300\n", "c")
	schema := plan.InputSchema()

	var bindErr error
	allocated := measureAllocations(t, func() {
		_, bindErr = schema.Bind(nil, []starlark.Tuple{keyword("cube", cube)}, testValueDepth, 64*1024)
	})

	require.ErrorIs(t, bindErr, ErrValueLimit)
	assert.Less(t, allocated, uint64(8<<20), "27 million logical elements must fail before being copied")
}

// TestBindEnforcesArgumentDepth proves the argument map counts as depth 1.
func TestBindEnforcesArgumentDepth(t *testing.T) {
	plan, err := CompileFor[cubeInput, representativeOutput]()
	require.NoError(t, err)
	cube := starlarkGlobal(t, "c = [[[1]]]\n", "c")
	schema := plan.InputSchema()

	_, err = schema.Bind(nil, []starlark.Tuple{keyword("cube", cube)}, 4, testValueNodes)
	require.ErrorIs(t, err, ErrValueLimit)

	canonical, err := schema.Bind(nil, []starlark.Tuple{keyword("cube", cube)}, 5, testValueNodes)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"cube": []any{[]any{[]any{int64(1)}}}}, canonical)
}

// TestBindingCostIsLinearInKeyLength proves long model-chosen keys are not
// copied once per nested element on either side of the worker boundary.
func TestBindingCostIsLinearInKeyLength(t *testing.T) {
	plan, err := CompileFor[groupedInput, representativeOutput]()
	require.NoError(t, err)
	key := strings.Repeat("k", 60_000)
	items := make([]starlark.Value, 1_000)
	neutral := make([]any, len(items))
	for index := range items {
		items[index] = starlark.MakeInt(index)
		neutral[index] = int64(index)
	}
	groups := starlark.NewDict(1)
	require.NoError(t, groups.SetKey(starlark.String(key), starlark.NewList(items)))
	schema := plan.InputSchema()

	var bindErr error
	childAllocated := measureAllocations(t, func() {
		_, bindErr = schema.Bind(nil, []starlark.Tuple{keyword("groups", groups)}, testValueDepth, testValueNodes)
	})
	require.NoError(t, bindErr)

	var rebindErr error
	parentAllocated := measureAllocations(t, func() {
		_, _, rebindErr = plan.BindValue(map[string]any{"groups": map[string]any{key: neutral}})
	})
	require.NoError(t, rebindErr)

	const linearBound = 1 << 20
	assert.Less(t, childAllocated, uint64(linearBound))
	assert.Less(t, parentAllocated, uint64(linearBound))
}

// TestFloat32InputsCanonicalizeToTheHandlerValue proves policy sees the rounded value the handler receives.
func TestFloat32InputsCanonicalizeToTheHandlerValue(t *testing.T) {
	plan, err := CompileFor[ratioInput, representativeOutput]()
	require.NoError(t, err)

	child, err := plan.InputSchema().Bind(nil, []starlark.Tuple{
		keyword("ratio", starlark.Float(0.1)),
		keyword("ratios", starlark.NewList([]starlark.Value{starlark.Float(1e-50), starlark.MakeInt(16_777_217)})),
	}, testValueDepth, testValueNodes)
	require.NoError(t, err)

	bound, canonical, err := plan.BindValue(child)
	require.NoError(t, err)
	typed, ok := bound.(ratioInput)
	require.True(t, ok)
	assert.Equal(t, ratioInput{Ratio: 0.1, Ratios: []float32{0, 16_777_216}}, typed)
	assert.Equal(t, map[string]any{
		"ratio":  float64(typed.Ratio),
		"ratios": []any{float64(typed.Ratios[0]), float64(typed.Ratios[1])},
	}, canonical)
	assert.Equal(t, child, canonical, "worker and parent canonicalization must agree")
}
