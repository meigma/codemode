package mcpserver_test

import (
	"context"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/mcpserver"
)

// queryInput is a composite capability input with lists, nested structs, maps, and optionals.
type queryInput struct {
	// Tags is a required list of strings.
	Tags []string `json:"tags"`

	// Filter is a required nested struct.
	Filter queryFilter `json:"filter"`

	// Labels is a required string-keyed map.
	Labels map[string]string `json:"labels"`

	// Window is an optional nested struct whose members need not be identifiers.
	Window *queryWindow `json:"window"`
}

// queryFilter is a nested input struct.
type queryFilter struct {
	// Owner is a required nested string.
	Owner string `json:"owner"`

	// States is a required nested list.
	States []string `json:"states"`

	// MinScore is an optional nested float.
	MinScore *float64 `json:"min_score"`
}

// queryWindow uses a Starlark keyword and a hyphenated name as nested members.
type queryWindow struct {
	// From is named after a reserved Starlark word.
	From string `json:"from"`

	// Until carries a non-identifier name.
	Until string `json:"until-day"`
}

// queryResult echoes what the handler received.
type queryResult struct {
	// Owner echoes the nested filter owner.
	Owner string `json:"owner"`

	// TagCount is the number of received tags.
	TagCount int64 `json:"tag_count"`
}

// queryRecorder records typed composite handler inputs.
type queryRecorder struct {
	// mu protects inputs.
	mu sync.Mutex

	// inputs are the typed inputs observed at dispatch.
	inputs []queryInput
}

// invoke records one typed input and echoes part of it.
func (recorder *queryRecorder) invoke(_ context.Context, _ authz.Subject, input queryInput) (queryResult, error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.inputs = append(recorder.inputs, input)
	return queryResult{Owner: input.Filter.Owner, TagCount: int64(len(input.Tags))}, nil
}

// snapshot returns a copy of recorded inputs.
func (recorder *queryRecorder) snapshot() []queryInput {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]queryInput(nil), recorder.inputs...)
}

// TestActualMCPCompositeInputProgram proves composite arguments travel from a
// model program through the worker, parent re-binding, and authorization to
// the typed handler, and that malformed nested arguments return path-precise
// diagnostics without reaching policy.
func TestActualMCPCompositeInputProgram(t *testing.T) {
	authorizer := &recordingAuthorizer{}
	recorder := &queryRecorder{}
	builder := codemode.New(codemode.Options{Authorizer: authorizer, Limits: codemode.DefaultLimits()})
	codemode.Register(builder, codemode.Capability[queryInput, queryResult]{
		ID:      "records.entry.query",
		Name:    "records.query",
		Summary: "Query records with a composite filter.",
		Handler: recorder.invoke,
	})
	root, err := builder.Build()
	require.NoError(t, err)
	mcpServer, err := mcpserver.New(root, contextResolver{}, mcpserver.Options{})
	require.NoError(t, err)

	trustedCtx := withInvocationIdentity(t.Context(), invocationIdentity{
		Subject: authz.Subject{ID: trustedSubjectID},
		Canary:  credentialCanary,
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(trustedCtx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "codemode-e2e", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	described, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe_api",
		Arguments: map[string]any{"name": "records.query"},
	})
	require.NoError(t, err)
	assertSuccessfulTool(t, described)
	description := decodeStructured[codemode.Description](t, described)
	assert.Equal(t,
		"records.query(*, tags: list[str], filter: {owner: str, states: list[str], min_score: float | None}, "+
			`labels: dict[str, str], window: {from: str, "until-day": str} | None)`,
		description.Signature)
	require.Len(t, description.Input, 4)
	assert.Equal(t, `{from: str, "until-day": str} | None`, description.Input[3].Type)
	assert.False(t, description.Input[3].Required)

	execute := func(source string) *mcp.CallToolResult {
		t.Helper()
		result, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "execute",
			Arguments: map[string]any{"source": source},
		})
		require.NoError(t, callErr)
		return result
	}

	first := execute(`
def main():
    return records.query(
        tags = ["a", "b"],
        filter = {"owner": "ops", "states": ("open", "triaged")},
        labels = {"env": "prod"},
    )
`)
	assertSuccessfulTool(t, first)
	assert.Equal(t, queryResult{Owner: "ops", TagCount: 2}, decodeStructured[struct {
		// Result is main's final converted value.
		Result queryResult `json:"result"`
	}](t, first).Result)

	second := execute(`
def main():
    return records.query(
        tags = [],
        filter = {"owner": "sec", "states": [], "min_score": 1},
        labels = {},
        window = {"from": "2026-01-01", "until-day": "2026-02-01"},
    )
`)
	assertSuccessfulTool(t, second)

	minScore := 1.0
	assert.Equal(t, []queryInput{
		{
			Tags:   []string{"a", "b"},
			Filter: queryFilter{Owner: "ops", States: []string{"open", "triaged"}},
			Labels: map[string]string{"env": "prod"},
		},
		{
			Tags:   []string{},
			Filter: queryFilter{Owner: "sec", States: []string{}, MinScore: &minScore},
			Labels: map[string]string{},
			Window: &queryWindow{From: "2026-01-01", Until: "2026-02-01"},
		},
	}, recorder.snapshot())

	authorizations := authorizer.snapshot()
	require.Len(t, authorizations, 2)
	assert.Equal(t, map[string]any{
		"tags":   []any{"a", "b"},
		"filter": map[string]any{"owner": "ops", "states": []any{"open", "triaged"}},
		"labels": map[string]any{"env": "prod"},
	}, authorizations[0].Arguments)
	assert.Equal(t, map[string]any{
		"tags":   []any{},
		"filter": map[string]any{"owner": "sec", "states": []any{}, "min_score": 1.0},
		"labels": map[string]any{},
		"window": map[string]any{"from": "2026-01-01", "until-day": "2026-02-01"},
	}, authorizations[1].Arguments)

	failures := []struct {
		// call is the malformed capability call.
		call string

		// want is the exact model-facing tool error.
		want string
	}{
		{
			call: `records.query(tags=[], filter={"owner": 1, "states": []}, labels={})`,
			want: `invalid capability arguments: argument "filter.owner" must be a string`,
		},
		{
			call: `records.query(tags=["a", 2], filter={"owner": "o", "states": []}, labels={})`,
			want: `invalid capability arguments: argument "tags[1]" must be a string`,
		},
		{
			call: `records.query(tags=[], filter={"owner": "o", "states": [], "bogus": 1}, labels={})`,
			want: `invalid capability arguments: unknown argument "filter.bogus"`,
		},
		{
			call: `records.query(tags=[], filter={"owner": "o", "states": []}, labels={"env": None})`,
			want: `invalid capability arguments: argument "labels['env']" must be a string`,
		},
		{
			call: `records.query(tags=[], filter={"owner": "o", "states": []}, labels={}, window={"from": "x"})`,
			want: `invalid capability arguments: missing required argument "window['until-day']"`,
		},
	}
	for _, failure := range failures {
		assertToolError(t, execute("def main():\n    return "+failure.call+"\n"), failure.want)
	}
	assert.Len(t, authorizer.snapshot(), 2, "rejected calls must never reach authorization")
	assert.Len(t, recorder.snapshot(), 2, "rejected calls must never reach the handler")
}
