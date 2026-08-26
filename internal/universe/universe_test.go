package universe_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/lib/math"
	"go.starlark.net/starlark"

	"github.com/meigma/codemode/internal/universe"
)

// TestTopLevelNamesExcludesSetAndIncludesStdlib proves the documented surface.
func TestTopLevelNamesExcludesSetAndIncludesStdlib(t *testing.T) {
	names := universe.TopLevelNames()

	assert.True(t, slices.IsSorted(names), "expected documented names to be sorted")
	assert.NotContains(t, names, "set", "set is dialect-gated and must not be documented")
	assert.NotContains(t, names, "time", "time is not a built-in module")
	assert.Contains(t, names, "abs")
	assert.Contains(t, names, "len")
	assert.Contains(t, names, "print")
	assert.Contains(t, names, "sum")
	assert.Contains(t, names, "json")
	assert.Contains(t, names, "math")

	names[0] = "mutated"
	assert.NotEqual(t, names[0], universe.TopLevelNames()[0], "expected TopLevelNames to copy")
}

// TestJSONMemberNamesAreExactlyTheSelectedSurface proves encode_indent is omitted.
func TestJSONMemberNamesAreExactlyTheSelectedSurface(t *testing.T) {
	assert.Equal(t, []string{"decode", "encode", "indent"}, universe.JSONMemberNames())

	names := universe.JSONMemberNames()
	names[0] = "mutated"
	assert.Equal(t, []string{"decode", "encode", "indent"}, universe.JSONMemberNames())
}

// TestMathMemberNamesMatchThePinnedModule proves math members stay complete and sorted.
func TestMathMemberNamesMatchThePinnedModule(t *testing.T) {
	names := universe.MathMemberNames()

	assert.True(t, slices.IsSorted(names), "expected math members to be sorted")
	assert.Equal(t, math.Module.Members.Keys(), names)
	assert.Contains(t, names, "sqrt")
	assert.Contains(t, names, "round")
	assert.Contains(t, names, "e")
	assert.Contains(t, names, "pi")

	names[0] = "mutated"
	assert.NotEqual(t, names[0], universe.MathMemberNames()[0], "expected MathMemberNames to copy")
}

// TestIsReservedRootCoversUniverseAndStdlibRoots proves nested leaves stay legal.
func TestIsReservedRootCoversUniverseAndStdlibRoots(t *testing.T) {
	tests := []struct {
		// name identifies the membership case.
		name string

		// root is the first dotted segment under test.
		root string

		// reserved is the expected membership.
		reserved bool
	}{
		{name: "sum", root: "sum", reserved: true},
		{name: "json", root: "json", reserved: true},
		{name: "math", root: "math", reserved: true},
		{name: "len", root: "len", reserved: true},
		{name: "set", root: "set", reserved: true},
		{name: "print", root: "print", reserved: true},
		{name: "None", root: "None", reserved: true},
		{name: "stats", root: "stats", reserved: false},
		{name: "time", root: "time", reserved: false},
		{name: "empty", root: "", reserved: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.reserved, universe.IsReservedRoot(tt.root))
		})
	}

	for name := range starlark.Universe {
		assert.Truef(t, universe.IsReservedRoot(name), "expected universe name %q to be reserved", name)
	}
}

// TestPredeclaredExposesOnlyTheFixedAdditions proves a fresh dictionary has no time or encode_indent.
func TestPredeclaredExposesOnlyTheFixedAdditions(t *testing.T) {
	predeclared := universe.Predeclared()

	require.ElementsMatch(t, []string{"json", "math", "sum"}, predeclared.Keys())
	_, hasTime := predeclared["time"]
	assert.False(t, hasTime, "expected no built-in time module")

	jsonModule, ok := predeclared["json"].(starlark.HasAttrs)
	require.True(t, ok)
	assert.Equal(t, []string{"decode", "encode", "indent"}, jsonModule.AttrNames())
	value, err := jsonModule.Attr("encode_indent")
	require.NoError(t, err)
	assert.Nil(t, value, "expected json.encode_indent to be absent")

	second := universe.Predeclared()
	second["extra"] = starlark.None
	_, exists := universe.Predeclared()["extra"]
	assert.False(t, exists, "expected Predeclared to return a fresh dictionary")
}
