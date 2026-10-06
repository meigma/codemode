package execution_test

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/execution"
)

// valueInput is the single required string input of the representative capabilities.
type valueInput struct {
	// Value is the required string argument.
	Value string `json:"value"`
}

// widenedInput exposes every required and optional scalar input form.
type widenedInput struct {
	// Org is the required string argument.
	Org string `json:"org"`

	// Count is the required signed integer argument.
	Count int64 `json:"count"`

	// Active is the required Boolean argument.
	Active bool `json:"active"`

	// Score is the required finite floating-point argument.
	Score float64 `json:"score"`

	// Label is the optional string argument.
	Label *string `json:"label,omitempty"`

	// Limit is the optional signed integer argument.
	Limit *int64 `json:"limit,omitempty"`

	// Enabled is the optional Boolean argument.
	Enabled *bool `json:"enabled,omitempty"`

	// Weight is the optional finite floating-point argument.
	Weight *float64 `json:"weight,omitempty"`
}

// searchFilter is the nested struct argument of the composite search capability.
type searchFilter struct {
	// Owner is the required nested string member.
	Owner string `json:"owner"`

	// Label is the optional nested string member.
	Label *string `json:"label,omitempty"`
}

// searchInput combines a list, a nested struct, and a string-keyed dict.
type searchInput struct {
	// Tags is the required list of strings.
	Tags []string `json:"tags"`

	// Filter is the required nested struct.
	Filter searchFilter `json:"filter"`

	// Counts is the required string-keyed dict of integers.
	Counts map[string]int64 `json:"counts"`
}

// schemaOutput is the minimal output type paired with test inputs at compile time.
type schemaOutput struct {
	// Value is a string output field.
	Value string `json:"value"`
}

// malformedInputSchema is one structurally invalid input schema fixture.
type malformedInputSchema struct {
	// name identifies the structural defect.
	name string

	// schema is the invalid input schema.
	schema binding.InputSchema
}

// TestExecuteBindsCompositeArguments proves nested arguments reach the native port as canonical maps.
func TestExecuteBindsCompositeArguments(t *testing.T) {
	tests := []struct {
		// name identifies the bound composite call.
		name string

		// source is the Starlark program that performs one native call.
		source string

		// wantArguments is the canonical map forwarded to the native port.
		wantArguments map[string]any
	}{
		{
			name: "every nested member",
			source: `def main(): return records.search(` +
				`tags=["alpha", "beta"], filter={"owner": "meigma", "label": "beta"}, counts={"open": 3, "closed": 0})`,
			wantArguments: map[string]any{
				"tags":   []any{"alpha", "beta"},
				"filter": map[string]any{"owner": "meigma", "label": "beta"},
				"counts": map[string]any{"open": int64(3), "closed": int64(0)},
			},
		},
		{
			name: "tuple list and omitted nested optional",
			source: `def main(): return records.search(` +
				`tags=("alpha",), filter={"owner": "meigma"}, counts={"open": 1})`,
			wantArguments: map[string]any{
				"tags":   []any{"alpha"},
				"filter": map[string]any{"owner": "meigma"},
				"counts": map[string]any{"open": int64(1)},
			},
		},
		{
			name: "explicit None nested optional",
			source: `def main(): return records.search(` +
				`tags=["alpha"], filter={"owner": "meigma", "label": None}, counts={"open": 1})`,
			wantArguments: map[string]any{
				"tags":   []any{"alpha"},
				"filter": map[string]any{"owner": "meigma"},
				"counts": map[string]any{"open": int64(1)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotID string
			var gotArguments map[string]any
			nativeCall := func(id string, arguments map[string]any) (any, error) {
				gotID = id
				gotArguments = arguments
				return "ok", nil
			}
			result, err := buildSearchEngine(t).Execute(tt.source, nativeCall, defaultExecutionLimits())

			require.NoError(t, err)
			assert.Equal(t, "cap.search", gotID)
			assert.Equal(t, tt.wantArguments, gotArguments)
			assert.Equal(t, "ok", result)
		})
	}
}

// TestExecuteAttachesCompositeArgumentPaths proves nested binding failures carry the path as safe detail.
func TestExecuteAttachesCompositeArgumentPaths(t *testing.T) {
	tests := []struct {
		// name identifies the malformed composite call.
		name string

		// source is the Starlark program that performs one native call.
		source string

		// detail is the expected model-facing suffix.
		detail string
	}{
		{
			name:   "nested struct member type",
			source: `def main(): return records.search(tags=["alpha"], filter={"owner": 1}, counts={})`,
			detail: `argument "filter.owner" must be a string`,
		},
		{
			name:   "nested optional member type",
			source: `def main(): return records.search(tags=["alpha"], filter={"owner": "m", "label": 1}, counts={})`,
			detail: `argument "filter.label" must be a string or None`,
		},
		{
			name:   "list element type",
			source: `def main(): return records.search(tags=["alpha", 2], filter={"owner": "m"}, counts={})`,
			detail: `argument "tags[1]" must be a string`,
		},
		{
			name:   "unknown nested member",
			source: `def main(): return records.search(tags=[], filter={"owner": "m", "bogus": 1}, counts={})`,
			detail: `unknown argument "filter.bogus"`,
		},
		{
			name:   "missing nested member",
			source: `def main(): return records.search(tags=[], filter={}, counts={})`,
			detail: `missing required argument "filter.owner"`,
		},
		{
			name:   "dict value overflow",
			source: `def main(): return records.search(tags=[], filter={"owner": "m"}, counts={"open": 1 << 64})`,
			detail: `argument "counts['open']" overflows int64`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nativeCalls atomic.Int64
			_, err := buildSearchEngine(
				t,
			).Execute(tt.source, countingNativeCall(&nativeCalls), defaultExecutionLimits())

			require.ErrorIs(t, err, execution.ErrInvalidArguments)
			assert.Equal(t, execution.ErrInvalidArguments.Error(), err.Error())
			detail, ok := execution.SafeDetail(err)
			require.True(t, ok)
			assert.Equal(t, tt.detail, detail)
			assert.Zero(t, nativeCalls.Load())
		})
	}
}

// TestExecuteBoundsCompositeArgumentsBeforeNativeCall proves oversized or aliased
// arguments fail as resource limits before materialization or dispatch.
func TestExecuteBoundsCompositeArgumentsBeforeNativeCall(t *testing.T) {
	tests := []struct {
		// name identifies the exhausted budget.
		name string

		// source is the over-budget program.
		source string

		// limits adjusts the default execution limits.
		limits func(*execution.Limits)
	}{
		{
			name:   "list exceeds node budget",
			source: `def main(): return records.search(tags=["x"] * 100000, filter={"owner": "m"}, counts={})`,
			limits: func(*execution.Limits) {},
		},
		{
			name:   "nesting exceeds depth",
			source: `def main(): return records.search(tags=[], filter={"owner": "m"}, counts={"open": 1})`,
			limits: func(limits *execution.Limits) { limits.MaxValueDepth = 2 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := defaultExecutionLimits()
			tt.limits(&limits)
			var nativeCalls atomic.Int64

			_, err := buildSearchEngine(t).Execute(tt.source, countingNativeCall(&nativeCalls), limits)

			require.ErrorIs(t, err, execution.ErrResourceLimit)
			assert.Zero(t, nativeCalls.Load())
		})
	}
}

// buildSearchEngine creates one engine exposing the composite records.search capability.
func buildSearchEngine(t *testing.T) *execution.Engine {
	t.Helper()
	engine, err := execution.New([]execution.CapabilityBinding{{
		ID:    "cap.search",
		Name:  "records.search",
		Input: mustInputSchema[searchInput](),
	}})
	require.NoError(t, err)
	return engine
}

// mustInputSchema compiles In as a capability input and returns its private schema.
//
// It panics when In is not a supported input type, which is a fixture bug.
func mustInputSchema[In any]() binding.InputSchema {
	plan, err := binding.CompileFor[In, schemaOutput]()
	if err != nil {
		panic(err)
	}
	return plan.InputSchema()
}

// malformedInputSchemas returns hand-built schemas that no compiled plan can produce.
func malformedInputSchemas() []malformedInputSchema {
	str := binding.InputNode{Kind: binding.InputString}
	return []malformedInputSchema{
		{name: "empty schema", schema: binding.InputSchema{}},
		{name: "root out of range", schema: binding.InputSchema{
			Root:  1,
			Nodes: []binding.InputNode{{Kind: binding.InputStruct}},
		}},
		{name: "root not a struct", schema: binding.InputSchema{Nodes: []binding.InputNode{str}}},
		{name: "forward element index", schema: binding.InputSchema{Root: 2, Nodes: []binding.InputNode{
			{Kind: binding.InputList, Elem: 1},
			str,
			{Kind: binding.InputStruct, Fields: []binding.InputField{{Name: "tags", Node: 0}}},
		}}},
		{name: "self element index", schema: binding.InputSchema{Root: 1, Nodes: []binding.InputNode{
			{Kind: binding.InputList, Elem: 0},
			{Kind: binding.InputStruct, Fields: []binding.InputField{{Name: "tags", Node: 0}}},
		}}},
		{name: "forward field index", schema: binding.InputSchema{Root: 0, Nodes: []binding.InputNode{
			{Kind: binding.InputStruct, Fields: []binding.InputField{{Name: "value", Node: 1}}},
			str,
		}}},
		{name: "optional wrapping optional", schema: binding.InputSchema{Root: 3, Nodes: []binding.InputNode{
			str,
			{Kind: binding.InputOptional, Elem: 0},
			{Kind: binding.InputOptional, Elem: 1},
			{Kind: binding.InputStruct, Fields: []binding.InputField{{Name: "label", Node: 2}}},
		}}},
		{name: "unknown kind", schema: binding.InputSchema{Root: 1, Nodes: []binding.InputNode{
			{Kind: binding.InputKind("tuple")},
			{Kind: binding.InputStruct, Fields: []binding.InputField{{Name: "value", Node: 0}}},
		}}},
		{name: "invalid root field name", schema: binding.InputSchema{Root: 1, Nodes: []binding.InputNode{
			str,
			{Kind: binding.InputStruct, Fields: []binding.InputField{{Name: "not-valid", Node: 0}}},
		}}},
		{name: "keyword root field name", schema: binding.InputSchema{Root: 1, Nodes: []binding.InputNode{
			str,
			{Kind: binding.InputStruct, Fields: []binding.InputField{{Name: "from", Node: 0}}},
		}}},
		{name: "duplicate field name", schema: binding.InputSchema{Root: 2, Nodes: []binding.InputNode{
			str,
			str,
			{
				Kind:   binding.InputStruct,
				Fields: []binding.InputField{{Name: "value", Node: 0}, {Name: "value", Node: 1}},
			},
		}}},
	}
}
