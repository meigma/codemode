package execution_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/execution"
	"github.com/meigma/codemode/internal/universe"
)

// TestNewRejectsReservedCapabilityRoots proves execution defense-in-depth uses the canonical predicate.
func TestNewRejectsReservedCapabilityRoots(t *testing.T) {
	valid := lookupBinding()
	tests := []struct {
		// name identifies the reserved root.
		name string

		// capability is the colliding dotted name.
		capability string
	}{
		{name: "sum", capability: "sum.x"},
		{name: "json", capability: "json.fetch"},
		{name: "math", capability: "math.add"},
		{name: "len", capability: "len.x"},
		{name: "set", capability: "set.x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, universe.IsReservedRoot(tt.name))
			_, err := execution.New([]execution.CapabilityBinding{withName(valid, tt.capability)})

			require.ErrorIs(t, err, execution.ErrInternal)
			assert.Contains(t, err.Error(), "reserved")
		})
	}
}

// TestNewAcceptsNonreservedNestedSum proves stats.sum remains a valid dynamic capability path.
func TestNewAcceptsNonreservedNestedSum(t *testing.T) {
	engine, err := execution.New([]execution.CapabilityBinding{statsSumBinding()})
	require.NoError(t, err)

	result, err := engine.Execute(
		`def main(): return stats.sum(value="nested")`,
		echoNativeCall(),
		defaultExecutionLimits(),
	)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "nested"}, result)
}

// TestExecuteExposesFixedStdlibWithoutHostConfiguration proves sum, json, and math are always on.
func TestExecuteExposesFixedStdlibWithoutHostConfiguration(t *testing.T) {
	engine, err := execution.New(nil)
	require.NoError(t, err)

	tests := []struct {
		// name identifies the language-surface behavior.
		name string

		// source is the submitted Starlark program.
		source string

		// want is the converted final value.
		want any
	}{
		{name: "sum integers", source: `def main(): return sum([1, 2, 3])`, want: int64(6)},
		{name: "sum floats", source: `def main(): return sum([1.5, 2.5])`, want: 4.0},
		{name: "sum mixed", source: `def main(): return sum([1, 2.5])`, want: 3.5},
		{name: "sum empty", source: `def main(): return sum([])`, want: int64(0)},
		{
			name:   "json round-trip",
			source: `def main(): return json.decode(json.encode({"a": 1}))["a"]`,
			want:   int64(1),
		},
		{name: "math sqrt", source: `def main(): return math.sqrt(4.0)`, want: 2.0},
		{name: "json members", source: `def main(): return dir(json)`, want: []any{"decode", "encode", "indent"}},
		{name: "json encode_indent absent", source: `def main(): return hasattr(json, "encode_indent")`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, execErr := engine.Execute(tt.source, unusedNativeCall(), defaultExecutionLimits())

			require.NoError(t, execErr)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestExecuteSumRejectsNonNumericAndArityErrors proves sum stays numeric-only with one argument.
func TestExecuteSumRejectsNonNumericAndArityErrors(t *testing.T) {
	engine, err := execution.New(nil)
	require.NoError(t, err)

	tests := []struct {
		// name identifies the invalid sum use.
		name string

		// source is the submitted Starlark program.
		source string
	}{
		{name: "string element", source: `def main(): return sum(["a"])`},
		{name: "bool element", source: `def main(): return sum([True])`},
		{name: "missing argument", source: `def main(): return sum()`},
		{name: "extra argument", source: `def main(): return sum([1], 0)`},
		{name: "non iterable", source: `def main(): return sum(1)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, execErr := engine.Execute(tt.source, unusedNativeCall(), defaultExecutionLimits())

			require.ErrorIs(t, execErr, execution.ErrInvalidProgram)
		})
	}
}

// TestExecuteRejectsBuiltInTimeAndDialectSet proves excluded names stay undefined.
func TestExecuteRejectsBuiltInTimeAndDialectSet(t *testing.T) {
	engine, err := execution.New(nil)
	require.NoError(t, err)

	tests := []struct {
		// name identifies the unavailable name.
		name string

		// source is the submitted Starlark program.
		source string
	}{
		{name: "time", source: `def main(): return time`},
		{name: "set", source: `def main(): return set`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, execErr := engine.Execute(tt.source, unusedNativeCall(), defaultExecutionLimits())

			require.ErrorIs(t, execErr, execution.ErrInvalidProgram)
		})
	}
}

// TestExecuteMergesDynamicNamespaceWithFixedStdlib proves capabilities coexist with sum.
func TestExecuteMergesDynamicNamespaceWithFixedStdlib(t *testing.T) {
	engine, err := execution.New([]execution.CapabilityBinding{statsSumBinding()})
	require.NoError(t, err)

	result, err := engine.Execute(`
def main():
    return [sum([1, 2, 3]), stats.sum(value="nested")]
`, echoNativeCall(), defaultExecutionLimits())

	require.NoError(t, err)
	assert.Equal(t, []any{int64(6), map[string]any{"value": "nested"}}, result)
}

// statsSumBinding returns a nonreserved nested sum capability.
func statsSumBinding() execution.CapabilityBinding {
	return execution.CapabilityBinding{
		ID:   "cap.stats.sum",
		Name: "stats.sum",
		Input: []binding.FieldShape{
			{Name: "value", Type: "str", Required: true},
		},
	}
}

// unusedNativeCall fails if a program reaches the native port.
func unusedNativeCall() execution.NativeCall {
	return func(string, map[string]any) (any, error) {
		panic("native call is not part of the fixed language surface")
	}
}
