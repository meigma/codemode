package binding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOutputShapeUsesDeterministicNestedNotation proves discovery Type strings stay flat and exact.
func TestOutputShapeUsesDeterministicNestedNotation(t *testing.T) {
	plan, err := CompileFor[representativeInput, notationOutput]()
	require.NoError(t, err)

	assert.Equal(t, []FieldShape{
		{Name: "items", Type: "list[{title: str, score: float}]", Required: true},
		{Name: "by_id", Type: "dict[str, {title: str, score: float}]", Required: true},
		{Name: "tags", Type: "list[str]", Required: true},
		{Name: "note", Type: "str | None", Required: true},
		{Name: "extra", Type: "str", Required: false},
		{Name: "nested", Type: "{value: str, detail?: str}", Required: true},
		{Name: "values", Type: "list[str | None] | None", Required: true},
		{Name: "alias", Type: "dict[str, bool]", Required: true},
		{Name: "payload", Type: "list[int]", Required: true},
	}, plan.OutputShape())
}

// TestOutputShapeDeduplicatesNoneAndPreservesDeclarationOrder proves stacked pointers append None once.
func TestOutputShapeDeduplicatesNoneAndPreservesDeclarationOrder(t *testing.T) {
	plan, err := CompileFor[representativeInput, noneDedupOutput]()
	require.NoError(t, err)

	assert.Equal(t, []FieldShape{
		{Name: "value", Type: "str | None", Required: true},
	}, plan.OutputShape())

	composite, err := CompileFor[representativeInput, compositeOutput]()
	require.NoError(t, err)
	assert.Equal(t, []FieldShape{
		{Name: "items", Type: "list[{title: str, score: float}]", Required: true},
		{Name: "tags", Type: "list[str]", Required: true},
		{Name: "by_id", Type: "dict[str, {title: str, score: float}]", Required: true},
		{Name: "note", Type: "str | None", Required: true},
		{Name: "extra", Type: "str", Required: false},
		{Name: "payload", Type: "list[int]", Required: true},
	}, composite.OutputShape())
}
