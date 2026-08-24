package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/mcpserver"
)

const (
	// trustedSubjectID is the authenticated identity installed only in server context.
	trustedSubjectID authz.SubjectID = "subject-trusted"

	// attackerSubjectID is a misleading identity supplied only in untrusted client metadata.
	attackerSubjectID authz.SubjectID = "subject-attacker"

	// credentialCanary is a unique secret that must never appear in client-visible MCP payloads.
	credentialCanary = "credential-canary-7f3a91c2e8b64d0f"

	// discardedPrint is top-level printed text that must not escape execute results.
	discardedPrint = "print-must-not-escape"

	// discardedIntermediate is a top-level Starlark value that must not escape execute results.
	discardedIntermediate = "intermediate-must-not-escape"

	// allowedLookupKey is the enabled lookup argument used by the successful program.
	allowedLookupKey = "alpha"

	// deniedLookupKey is the enabled lookup argument rejected by the test authorizer.
	deniedLookupKey = "forbidden"

	// allowedLookupLimit is the optional integer argument used by the successful program.
	allowedLookupLimit int64 = 2
)

// invocationContextKey is the typed trusted-context key for e2e identity.
type invocationContextKey struct{}

// invocationIdentity is the host-owned subject and credential canary.
type invocationIdentity struct {
	// Subject is the trusted authenticated caller.
	Subject authz.Subject

	// Canary is a unique credential that must never cross MCP.
	Canary string
}

// lookupInput is the enabled records.lookup argument contract.
type lookupInput struct {
	// Key is the required record identifier.
	Key string `json:"key"`

	// Limit is the optional result bound.
	Limit *int64 `json:"limit,omitempty"`
}

// lookupResult is the deterministic records.lookup handler output.
type lookupResult struct {
	// Key is the looked-up record identifier.
	Key string `json:"key"`

	// Count is the resolved optional limit.
	Count int64 `json:"count"`
}

// StatusResult is an exported handler output whose Go identifier must not cross MCP.
type StatusResult struct {
	// State is the current status value.
	State string `json:"state"`
}

// executeEnvelope is the exact structured execute payload.
type executeEnvelope struct {
	// Result is main's final converted value.
	Result lookupResult `json:"result"`
}

// contextResolver reads the trusted subject from typed server context.
type contextResolver struct{}

// Resolve returns the subject installed by the host in trusted context.
func (contextResolver) Resolve(ctx context.Context) (authz.Subject, error) {
	identity, ok := invocationIdentityFrom(ctx)
	if !ok || identity.Subject.ID == "" {
		return authz.Subject{}, codemode.ErrUnauthenticated
	}
	return identity.Subject, nil
}

// recordingAuthorizer records canonical authorization inputs and denies one key.
type recordingAuthorizer struct {
	// mu protects recorded authorization inputs.
	mu sync.Mutex

	// calls is the ordered clone of observed authorization inputs.
	calls []authz.AuthorizationInput
}

// Authorize records one canonical input and denies the reserved lookup key.
func (authorizer *recordingAuthorizer) Authorize(_ context.Context, input authz.AuthorizationInput) error {
	authorizer.mu.Lock()
	defer authorizer.mu.Unlock()
	authorizer.calls = append(authorizer.calls, cloneAuthorizationInput(input))
	if key, _ := input.Arguments["key"].(string); key == deniedLookupKey {
		return fmt.Errorf("trusted denial detail: %w", authz.ErrDenied)
	}
	return nil
}

// snapshot returns a copy of recorded authorization inputs.
func (authorizer *recordingAuthorizer) snapshot() []authz.AuthorizationInput {
	authorizer.mu.Lock()
	defer authorizer.mu.Unlock()
	return append([]authz.AuthorizationInput(nil), authorizer.calls...)
}

// lookupRecorder is the enabled capability handler that records trusted dispatch.
type lookupRecorder struct {
	// canary is the credential that must remain visible only in trusted context.
	canary string

	// mu protects recorded handler observations.
	mu sync.Mutex

	// calls is the number of handler dispatches.
	calls int

	// subjects are the trusted subjects observed at dispatch.
	subjects []authz.Subject

	// inputs are cloned typed arguments observed at dispatch.
	inputs []lookupInput

	// sawCanary reports whether every dispatch could read the trusted canary.
	sawCanary bool
}

// invoke records trusted handler inputs and returns a deterministic lookup result.
func (recorder *lookupRecorder) invoke(
	ctx context.Context,
	subject authz.Subject,
	input lookupInput,
) (lookupResult, error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.calls++
	recorder.subjects = append(recorder.subjects, subject)
	recorder.inputs = append(recorder.inputs, cloneLookupInput(input))
	identity, ok := invocationIdentityFrom(ctx)
	recorder.sawCanary = (recorder.calls == 1 || recorder.sawCanary) &&
		ok && identity.Canary == recorder.canary && identity.Subject.ID == subject.ID
	count := int64(0)
	if input.Limit != nil {
		count = *input.Limit
	}
	return lookupResult{Key: input.Key, Count: count}, nil
}

// snapshot returns a copy of recorded handler observations.
func (recorder *lookupRecorder) snapshot() (int, []authz.Subject, []lookupInput, bool) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.calls,
		append([]authz.Subject(nil), recorder.subjects...),
		append([]lookupInput(nil), recorder.inputs...),
		recorder.sawCanary
}

// TestActualMCPSecureLoop proves the official in-memory MCP boundary preserves the secure loop.
func TestActualMCPSecureLoop(t *testing.T) {
	authorizer := &recordingAuthorizer{}
	lookup := &lookupRecorder{canary: credentialCanary}
	var hiddenCalls atomic.Int64

	builder := codemode.New(codemode.Options{
		Authorizer:           authorizer,
		DisabledCapabilities: []codemode.CapabilityID{"records.entry.hidden"},
		Limits:               codemode.DefaultLimits(),
	})
	require.NoError(t, codemode.Register(builder, codemode.Capability[lookupInput, lookupResult]{
		ID:          "records.entry.lookup",
		Name:        "records.lookup",
		Summary:     "Look up one record by key.",
		Description: "Returns one deterministic record for the supplied key.",
		Handler:     lookup.invoke,
	}))
	require.NoError(t, codemode.Register(builder, codemode.Capability[lookupInput, lookupResult]{
		ID:          "records.entry.hidden",
		Name:        "records.hidden",
		Summary:     "Look up one hidden record by key.",
		Description: "Must remain absent from every live MCP surface.",
		Handler: func(context.Context, authz.Subject, lookupInput) (lookupResult, error) {
			hiddenCalls.Add(1)
			return lookupResult{}, errors.New("disabled capability invoked")
		},
	}))
	require.NoError(t, codemode.Register(builder, codemode.Capability[struct{}, StatusResult]{
		ID:          "health.entry.status",
		Name:        "health.status",
		Summary:     "Report current health.",
		Description: "Returns one deterministic health status object.",
		Handler: func(context.Context, authz.Subject, struct{}) (StatusResult, error) {
			return StatusResult{State: "ok"}, nil
		},
	}))
	root, err := builder.Build()
	require.NoError(t, err)

	mcpServer, err := mcpserver.New(root, contextResolver{})
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

	listed, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	assertNoCanary(t, listed)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"search_api", "describe_api", "execute"}, names)

	searched, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "search_api",
		Arguments: map[string]any{"query": "record"},
	})
	require.NoError(t, err)
	assertSuccessfulTool(t, searched)
	assertNoCanary(t, searched)
	searchResults := decodeStructured[[]codemode.SearchResult](t, searched)
	require.Len(t, searchResults, 1)
	assert.Equal(t, "records.lookup", searchResults[0].Name)
	assert.Equal(t, "records.lookup(*, key: str, limit: int | None)", searchResults[0].Signature)
	assert.Equal(t, "Look up one record by key.", searchResults[0].Summary)
	assertDiscoveryOmitsGoTypeNames(t, searched, "lookupResult", "StatusResult")

	described, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe_api",
		Arguments: map[string]any{"name": "records.lookup"},
	})
	require.NoError(t, err)
	assertSuccessfulTool(t, described)
	assertNoCanary(t, described)
	description := decodeStructured[codemode.Description](t, described)
	assert.Equal(t, "records.lookup", description.Name)
	assert.Equal(t, "records.lookup(*, key: str, limit: int | None)", description.Signature)
	require.Len(t, description.Input, 2)
	assert.Equal(t, "key", description.Input[0].Name)
	assert.Equal(t, "str", description.Input[0].Type)
	assert.True(t, description.Input[0].Required)
	assert.Equal(t, "limit", description.Input[1].Name)
	assert.Equal(t, "int | None", description.Input[1].Type)
	assert.False(t, description.Input[1].Required)
	require.Len(t, description.Output, 2)
	assert.Equal(t, "key", description.Output[0].Name)
	assert.Equal(t, "str", description.Output[0].Type)
	assert.True(t, description.Output[0].Required)
	assert.Equal(t, "count", description.Output[1].Name)
	assert.Equal(t, "int", description.Output[1].Type)
	assert.True(t, description.Output[1].Required)
	assertDiscoveryOmitsGoTypeNames(t, described, "lookupResult", "StatusResult")

	statusSearch, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "search_api",
		Arguments: map[string]any{"query": "health"},
	})
	require.NoError(t, err)
	assertSuccessfulTool(t, statusSearch)
	assertNoCanary(t, statusSearch)
	statusResults := decodeStructured[[]codemode.SearchResult](t, statusSearch)
	require.Len(t, statusResults, 1)
	assert.Equal(t, "health.status", statusResults[0].Name)
	assert.Equal(t, "health.status()", statusResults[0].Signature)
	assertDiscoveryOmitsGoTypeNames(t, statusSearch, "lookupResult", "StatusResult")

	statusDescribed, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe_api",
		Arguments: map[string]any{"name": "health.status"},
	})
	require.NoError(t, err)
	assertSuccessfulTool(t, statusDescribed)
	assertNoCanary(t, statusDescribed)
	statusDescription := decodeStructured[codemode.Description](t, statusDescribed)
	assert.Equal(t, "health.status", statusDescription.Name)
	assert.Equal(t, "health.status()", statusDescription.Signature)
	assert.Empty(t, statusDescription.Input)
	require.Len(t, statusDescription.Output, 1)
	assert.Equal(t, "state", statusDescription.Output[0].Name)
	assertDiscoveryOmitsGoTypeNames(t, statusDescribed, "lookupResult", "StatusResult")

	hidden, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe_api",
		Arguments: map[string]any{"name": "records.hidden"},
	})
	require.NoError(t, err)
	assertToolError(t, hidden, codemode.ErrNotFound.Error())
	assertNoCanary(t, hidden)

	allowed, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "execute",
		Arguments: map[string]any{
			"source": `
print("` + discardedPrint + `")
intermediate = "` + discardedIntermediate + `"
def main():
    return records.lookup(key="` + allowedLookupKey + `", limit=` + strconv.FormatInt(allowedLookupLimit, 10) + `)
`,
		},
	})
	require.NoError(t, err)
	assertSuccessfulTool(t, allowed)
	assertNoCanary(t, allowed)
	assertNotContainsText(t, allowed, discardedPrint, discardedIntermediate)
	wantAllowed := executeEnvelope{Result: lookupResult{Key: allowedLookupKey, Count: allowedLookupLimit}}
	assert.Equal(t, wantAllowed, decodeStructured[executeEnvelope](t, allowed))
	assertExactExecuteEnvelope(t, allowed.StructuredContent)
	assertMirroredExecuteContent(t, allowed, wantAllowed)

	handlerCalls, subjects, inputs, sawCanary := lookup.snapshot()
	require.Equal(t, 1, handlerCalls)
	require.Equal(t, []authz.Subject{{ID: trustedSubjectID}}, subjects)
	require.Len(t, inputs, 1)
	assert.Equal(t, allowedLookupKey, inputs[0].Key)
	require.NotNil(t, inputs[0].Limit)
	assert.Equal(t, allowedLookupLimit, *inputs[0].Limit)
	assert.True(t, sawCanary, "enabled handler must observe the trusted context canary")

	authorizations := authorizer.snapshot()
	require.Len(t, authorizations, 1)
	assert.Equal(t, authz.Subject{ID: trustedSubjectID}, authorizations[0].Subject)
	assert.Equal(t, "records.entry.lookup", authorizations[0].CapabilityID)
	assert.Equal(t, "records.lookup", authorizations[0].CapabilityName)
	assert.Equal(t, map[string]any{"key": allowedLookupKey, "limit": allowedLookupLimit}, authorizations[0].Arguments)

	denied, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Meta: mcp.Meta{
			"subject_id": string(attackerSubjectID),
			"subject":    map[string]any{"id": string(attackerSubjectID)},
			"canary":     "forged-canary",
		},
		Name: "execute",
		Arguments: map[string]any{
			"source": `
def main():
    return records.lookup(key="` + deniedLookupKey + `", limit=1)
`,
		},
	})
	require.NoError(t, err)
	assertToolError(t, denied, codemode.ErrPermissionDenied.Error())
	assertNoCanary(t, denied)

	handlerCalls, _, _, _ = lookup.snapshot()
	assert.Equal(t, 1, handlerCalls, "denied native calls must not dispatch the handler")
	assert.Zero(t, hiddenCalls.Load(), "disabled capabilities must never dispatch")

	authorizations = authorizer.snapshot()
	require.Len(t, authorizations, 2)
	assert.Equal(t, authz.Subject{ID: trustedSubjectID}, authorizations[1].Subject)
	assert.Equal(t, "records.entry.lookup", authorizations[1].CapabilityID)
	assert.Equal(t, "records.lookup", authorizations[1].CapabilityName)
	assert.Equal(t, map[string]any{"key": deniedLookupKey, "limit": int64(1)}, authorizations[1].Arguments)
	assert.NotEqual(t, string(attackerSubjectID), string(authorizations[1].Subject.ID))
}

// withInvocationIdentity stores trusted identity on the server-owned context.
func withInvocationIdentity(ctx context.Context, identity invocationIdentity) context.Context {
	return context.WithValue(ctx, invocationContextKey{}, identity)
}

// invocationIdentityFrom reads trusted identity from the server-owned context.
func invocationIdentityFrom(ctx context.Context) (invocationIdentity, bool) {
	identity, ok := ctx.Value(invocationContextKey{}).(invocationIdentity)
	return identity, ok
}

// cloneAuthorizationInput copies one authorization input and its canonical argument map.
func cloneAuthorizationInput(input authz.AuthorizationInput) authz.AuthorizationInput {
	cloned := input
	if input.Arguments != nil {
		cloned.Arguments = make(map[string]any, len(input.Arguments))
		maps.Copy(cloned.Arguments, input.Arguments)
	}
	return cloned
}

// cloneLookupInput copies one typed lookup input including the optional limit.
func cloneLookupInput(input lookupInput) lookupInput {
	cloned := input
	if input.Limit != nil {
		limit := *input.Limit
		cloned.Limit = &limit
	}
	return cloned
}

// decodeStructured decodes MCP structured content into a typed value.
func decodeStructured[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var decoded T
	require.NoError(t, json.Unmarshal(raw, &decoded), "structured content %s", raw)
	return decoded
}

// assertSuccessfulTool requires a non-error tool result.
func assertSuccessfulTool(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	require.NotNil(t, result)
	require.False(t, result.IsError, "tool call failed, content: %+v", result.Content)
}

// assertToolError requires a tool-level error whose only client-visible text is expected.
func assertToolError(t *testing.T, result *mcp.CallToolResult, expected string) {
	t.Helper()
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected a tool-level error")
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "tool error content must be text")
	assert.Equal(t, expected, text.Text)
}

// assertExactExecuteEnvelope requires structured execute output to be only {"result": ...}.
func assertExactExecuteEnvelope(t *testing.T, structured any) {
	t.Helper()
	raw, err := json.Marshal(structured)
	require.NoError(t, err)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.Len(t, envelope, 1)
	_, ok := envelope["result"]
	assert.True(t, ok, "execute structured content must wrap the final value under result")
}

// assertMirroredExecuteContent requires the SDK text mirror to contain only the execute payload.
func assertMirroredExecuteContent(
	t *testing.T,
	result *mcp.CallToolResult,
	expected executeEnvelope,
) {
	t.Helper()
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "successful execute content must be text")
	raw, err := json.Marshal(expected)
	require.NoError(t, err)
	assert.JSONEq(t, string(raw), text.Text)
}

// assertNoCanary requires serialized MCP payloads to omit the trusted credential canary.
func assertNoCanary(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), credentialCanary)
}

// assertNotContainsText requires tool content and structured output to omit leaked values.
func assertNotContainsText(t *testing.T, result *mcp.CallToolResult, forbidden ...string) {
	t.Helper()
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	payload := string(raw)
	for _, value := range forbidden {
		assert.NotContains(t, payload, value)
	}
}

// assertDiscoveryOmitsGoTypeNames requires search/describe structured content
// and its JSON text mirror to omit host Go output identifiers.
func assertDiscoveryOmitsGoTypeNames(t *testing.T, result *mcp.CallToolResult, forbidden ...string) {
	t.Helper()
	structured, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "successful discovery content must be text")
	for _, name := range forbidden {
		assert.NotContains(t, string(structured), name)
		assert.NotContains(t, text.Text, name)
	}
}
