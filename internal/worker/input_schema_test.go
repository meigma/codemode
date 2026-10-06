package worker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/execution"
)

// valueInput is the single required string input of the runner lookup capability.
type valueInput struct {
	// Value is the required string argument.
	Value string `json:"value"`
}

// lookupInput is the frame-test lookup input with one optional member.
type lookupInput struct {
	// Org is the required string argument.
	Org string `json:"org"`

	// Limit is the optional signed integer argument.
	Limit *int64 `json:"limit,omitempty"`
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

// TestExecFrameRoundTripsCompositeInputSchema proves the manifest carries the full schema arena.
func TestExecFrameRoundTripsCompositeInputSchema(t *testing.T) {
	exec := validExecFrame()
	exec.Manifest = []manifestEntry{{
		ID:    "cap.search",
		Name:  "records.search",
		Input: mustInputSchema[searchInput](),
	}}
	payload, err := encodeExec(exec)
	require.NoError(t, err)

	decoded, err := decodePayload(payload)

	require.NoError(t, err)
	got, ok := decoded.(execFrame)
	require.True(t, ok)
	assert.Equal(t, exec.Manifest, got.Manifest)
}

// TestRunnerNewRejectsMalformedInputSchemas proves the parent validates schemas before any spawn.
func TestRunnerNewRejectsMalformedInputSchemas(t *testing.T) {
	for _, malformed := range malformedInputSchemas() {
		t.Run(malformed.name, func(t *testing.T) {
			capability := lookupBinding()
			capability.Input = malformed.schema

			_, err := NewRunner([]execution.CapabilityBinding{capability}, testLimits(), nopDispatch())

			require.ErrorIs(t, err, errInvalidManifest)
		})
	}
}

// TestRunnerCarriesCompositeArguments proves nested arguments cross the worker boundary canonically.
func TestRunnerCarriesCompositeArguments(t *testing.T) {
	var gotID string
	var gotArguments map[string]any
	runner := newSearchRunner(t, func(
		_ context.Context,
		_ authz.Subject,
		id string,
		arguments map[string]any,
		_ int,
	) (any, error) {
		gotID = id
		gotArguments = arguments
		return "ok", nil
	})

	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, "def main():\n"+
		"    return records.search(tags=[\"alpha\", \"beta\"], filter={\"owner\": \"meigma\", \"label\": None},"+
		" counts={\"open\": 3})\n")

	require.NoError(t, err)
	assert.Equal(t, "ok", got)
	assert.Equal(t, "cap.search", gotID)
	assert.Equal(t, map[string]any{
		"tags":   []any{"alpha", "beta"},
		"filter": map[string]any{"owner": "meigma"},
		"counts": map[string]any{"open": int64(3)},
	}, gotArguments)
}

// TestRunnerReportsCompositeArgumentPath proves nested type errors keep their path as safe detail.
func TestRunnerReportsCompositeArgumentPath(t *testing.T) {
	var calls int
	runner := newSearchRunner(t, func(context.Context, authz.Subject, string, map[string]any, int) (any, error) {
		calls++
		return "ok", nil
	})

	_, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, "def main():\n"+
		"    return records.search(tags=[\"alpha\"], filter={\"owner\": 1}, counts={})\n")

	require.ErrorIs(t, err, execution.ErrInvalidArguments)
	assert.Equal(t, execution.ErrInvalidArguments.Error(), err.Error())
	detail, ok := execution.SafeDetail(err)
	require.True(t, ok)
	assert.Equal(t, `argument "filter.owner" must be a string`, detail)
	assert.Zero(t, calls)
}

// newSearchRunner constructs a runner exposing the composite records.search capability.
func newSearchRunner(t *testing.T, dispatch Dispatch) *Runner {
	t.Helper()
	runner, err := NewRunner([]execution.CapabilityBinding{{
		ID:    "cap.search",
		Name:  "records.search",
		Input: mustInputSchema[searchInput](),
	}}, testLimits(), dispatch)
	require.NoError(t, err)
	return runner
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
