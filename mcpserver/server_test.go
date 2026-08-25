package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/authz/rego"
	"github.com/meigma/codemode/mcpserver"
	"github.com/meigma/codemode/mcpserver/mocks"
)

// TestNewRejectsMissingDependencies proves construction fails closed for nil and typed-nil ports.
func TestNewRejectsMissingDependencies(t *testing.T) {
	tests := []struct {
		// name identifies the rejected construction.
		name string

		// service is the Service passed to New.
		service mcpserver.Service

		// resolver is the InvocationResolver passed to New.
		resolver mcpserver.InvocationResolver
	}{
		{
			name:     "nil service",
			service:  nil,
			resolver: mocks.NewMockInvocationResolver(t),
		},
		{
			name:     "typed-nil service",
			service:  (*mocks.MockService)(nil),
			resolver: mocks.NewMockInvocationResolver(t),
		},
		{
			name:     "nil resolver",
			service:  mocks.NewMockService(t),
			resolver: nil,
		},
		{
			name:     "typed-nil resolver",
			service:  mocks.NewMockService(t),
			resolver: (*mocks.MockInvocationResolver)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := mcpserver.New(tt.service, tt.resolver)

			require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
			assert.Nil(t, server)
		})
	}
}

// TestNewRegistersExactlyThreeTools proves the adapter exposes only the three official tools,
// lists authoring guidance on each description, and advertises non-null output schemas.
func TestNewRegistersExactlyThreeTools(t *testing.T) {
	session := newTestSession(t, mocks.NewMockService(t), mocks.NewMockInvocationResolver(t))

	listed, err := session.client.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 3)
	assert.Equal(t, []string{"describe_api", "execute", "search_api"}, toolNames(listed.Tools))

	tests := []struct {
		// name is the official listed tool.
		name string

		// cues are required authoring phrases on the listed description.
		cues []string

		// assertOutput inspects the advertised outputSchema.
		assertOutput func(*testing.T, map[string]any)
	}{
		{
			name:         "search_api",
			cues:         []string{"short literal substring", "shorter term"},
			assertOutput: requireSearchAPIOutputSchema,
		},
		{
			name:         "describe_api",
			cues:         []string{"exact name returned by search_api", "without whitespace or case changes"},
			assertOutput: requireDescribeAPIOutputSchema,
		},
		{
			name: "execute",
			cues: []string{
				"def main():",
				"zero arguments",
				"inside main",
				"confirmed through search_api and describe_api",
				"final result",
			},
			assertOutput: requireExecuteOutputSchema,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			description := listedDescription(t, listed.Tools, tt.name)
			for _, cue := range tt.cues {
				assert.Contains(t, description, cue)
			}
			tt.assertOutput(t, listedOutputSchema(t, listed.Tools, tt.name))
		})
	}
}

// TestSDKRejectsMalformedArgumentsBeforeResolution proves schema validation owns malformed tool input.
func TestSDKRejectsMalformedArgumentsBeforeResolution(t *testing.T) {
	tests := []struct {
		// name identifies the malformed argument shape.
		name string

		// tool is the official tool called through the SDK.
		tool string

		// arguments are rejected before the typed handler runs.
		arguments map[string]any
	}{
		{name: "search missing query", tool: "search_api", arguments: map[string]any{}},
		{name: "search wrong query type", tool: "search_api", arguments: map[string]any{"query": 1}},
		{
			name:      "search unexpected field",
			tool:      "search_api",
			arguments: map[string]any{"query": "lookup", "subject": "attacker"},
		},
		{name: "describe missing name", tool: "describe_api", arguments: map[string]any{}},
		{name: "describe wrong name type", tool: "describe_api", arguments: map[string]any{"name": true}},
		{name: "execute missing source", tool: "execute", arguments: map[string]any{}},
		{name: "execute wrong source type", tool: "execute", arguments: map[string]any{"source": []string{"program"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := mocks.NewMockService(t)
			resolver := mocks.NewMockInvocationResolver(t)
			session := newTestSession(t, service, resolver)

			result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.arguments,
			})

			require.NoError(t, err)
			require.Empty(t, resolver.Calls)
			require.Empty(t, service.Calls)
			requireToolValidationError(t, result)
		})
	}
}

// TestToolsResolveSubjectBeforeServiceWork proves every operation authenticates before catalog or execution work.
func TestToolsResolveSubjectBeforeServiceWork(t *testing.T) {
	events := make([]string, 0, 6)
	service := mocks.NewMockService(t)
	resolver := mocks.NewMockInvocationResolver(t)
	resolver.EXPECT().Resolve(mock.Anything).RunAndReturn(func(context.Context) (authz.Subject, error) {
		events = append(events, "resolve")
		return authz.Subject{ID: "subject-1"}, nil
	}).Times(3)
	service.EXPECT().Search("lookup").RunAndReturn(func(string) ([]codemode.SearchResult, error) {
		events = append(events, "search")
		return []codemode.SearchResult{
			{Name: "records.lookup", Signature: "records.lookup()", Summary: "lookup"},
		}, nil
	}).Once()
	service.EXPECT().
		Describe(codemode.CapabilityName("records.lookup")).
		RunAndReturn(func(codemode.CapabilityName) (codemode.Description, error) {
			events = append(events, "describe")
			return codemode.Description{
				Name:      "records.lookup",
				Signature: "records.lookup()",
				Summary:   "lookup",
			}, nil
		}).
		Once()
	service.EXPECT().Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("source")).RunAndReturn(
		func(context.Context, authz.Subject, codemode.Program) (any, error) {
			events = append(events, "execute")
			return "ok", nil
		},
	).Once()
	session := newTestSession(t, service, resolver)

	searchResult, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "search_api",
		Arguments: map[string]any{"query": "lookup"},
	})
	require.NoError(t, err)
	require.False(t, searchResult.IsError)
	describeResult, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "describe_api",
		Arguments: map[string]any{"name": "records.lookup"},
	})
	require.NoError(t, err)
	require.False(t, describeResult.IsError)
	assert.Equal(t, map[string]any{
		"name":        "records.lookup",
		"signature":   "records.lookup()",
		"summary":     "lookup",
		"description": "",
		"input":       []any{},
		"output":      []any{},
	}, describeResult.StructuredContent)
	executeResult, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "execute",
		Arguments: map[string]any{"source": "source"},
	})
	require.NoError(t, err)
	require.False(t, executeResult.IsError)
	assert.Equal(t, []string{"resolve", "search", "resolve", "describe", "resolve", "execute"}, events)
	assert.Equal(t, map[string]any{"result": "ok"}, executeResult.StructuredContent)
}

// TestToolsIgnoreUntrustedClientMetadata proves request _meta cannot affect subject resolution.
func TestToolsIgnoreUntrustedClientMetadata(t *testing.T) {
	service := mocks.NewMockService(t)
	resolver := mocks.NewMockInvocationResolver(t)
	resolver.EXPECT().Resolve(mock.Anything).Return(authz.Subject{ID: "subject-1"}, nil).Once()
	service.EXPECT().Search("lookup").Return([]codemode.SearchResult{}, nil).Once()
	session := newTestSession(t, service, resolver)

	result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
		Meta:      mcp.Meta{"subject": "attacker", "credential": "canary"},
		Name:      "search_api",
		Arguments: map[string]any{"query": "lookup"},
	})

	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Equal(t, []any{}, result.StructuredContent)
}

// TestToolsSerializeSuccessfulEmptySlicesAsArrays proves successful nil and empty
// service slices serialize as JSON arrays, not null, including the text mirror.
func TestToolsSerializeSuccessfulEmptySlicesAsArrays(t *testing.T) {
	tests := []struct {
		// name identifies the successful slice serialization.
		name string

		// tool is the official tool under test.
		tool string

		// arguments are the valid tool arguments.
		arguments map[string]any

		// configure installs one generated service expectation.
		configure func(*mocks.MockService)

		// want is the successful structured content and JSON text mirror.
		want any
	}{
		{
			name:      "search nil results",
			tool:      "search_api",
			arguments: map[string]any{"query": "lookup"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().Search("lookup").Return(nil, nil).Once()
			},
			want: []any{},
		},
		{
			name:      "search empty results",
			tool:      "search_api",
			arguments: map[string]any{"query": "lookup"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().Search("lookup").Return([]codemode.SearchResult{}, nil).Once()
			},
			want: []any{},
		},
		{
			name:      "search populated results",
			tool:      "search_api",
			arguments: map[string]any{"query": "lookup"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().Search("lookup").Return([]codemode.SearchResult{
					{Name: "records.lookup", Signature: "records.lookup()", Summary: "lookup"},
				}, nil).Once()
			},
			want: []any{
				map[string]any{
					"name":      "records.lookup",
					"signature": "records.lookup()",
					"summary":   "lookup",
				},
			},
		},
		{
			name:      "describe nil field slices",
			tool:      "describe_api",
			arguments: map[string]any{"name": "records.lookup"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Describe(codemode.CapabilityName("records.lookup")).
					Return(codemode.Description{
						Name:      "records.lookup",
						Signature: "records.lookup()",
						Summary:   "lookup",
					}, nil).
					Once()
			},
			want: map[string]any{
				"name":        "records.lookup",
				"signature":   "records.lookup()",
				"summary":     "lookup",
				"description": "",
				"input":       []any{},
				"output":      []any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := mocks.NewMockService(t)
			resolver := mocks.NewMockInvocationResolver(t)
			resolver.EXPECT().Resolve(mock.Anything).Return(authz.Subject{ID: "subject-1"}, nil).Once()
			tt.configure(service)
			session := newTestSession(t, service, resolver)

			result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.arguments,
			})

			require.NoError(t, err)
			requireSuccessfulStructuredValue(t, result, tt.want)
		})
	}
}

// TestToolsProjectResolverFailures proves resolver errors and empty subjects never reach the service.
func TestToolsProjectResolverFailures(t *testing.T) {
	tests := []struct {
		// name identifies the authentication failure.
		name string

		// resolve returns the resolver outcome.
		resolve func(context.Context) (authz.Subject, error)
	}{
		{
			name: "resolver error",
			resolve: func(context.Context) (authz.Subject, error) {
				return authz.Subject{}, errors.New("trusted identity detail")
			},
		},
		{
			name: "empty subject",
			resolve: func(context.Context) (authz.Subject, error) {
				return authz.Subject{}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := mocks.NewMockService(t)
			resolver := mocks.NewMockInvocationResolver(t)
			resolver.EXPECT().Resolve(mock.Anything).RunAndReturn(tt.resolve).Once()
			session := newTestSession(t, service, resolver)

			result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "search_api",
				Arguments: map[string]any{"query": "lookup"},
			})

			require.NoError(t, err)
			requireToolError(t, result, codemode.ErrUnauthenticated.Error())
		})
	}
}

// TestToolsProjectStableServiceErrors proves known sentinels and unknown errors become coarse tool text.
func TestToolsProjectStableServiceErrors(t *testing.T) {
	tests := []struct {
		// name identifies the projected failure.
		name string

		// tool is the official tool under test.
		tool string

		// arguments are the valid tool arguments.
		arguments map[string]any

		// configure installs one generated service expectation.
		configure func(*mocks.MockService)

		// want is the exact coarse tool error text.
		want string
	}{
		{
			name:      "not found",
			tool:      "describe_api",
			arguments: map[string]any{"name": "records.hidden"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Describe(codemode.CapabilityName("records.hidden")).
					Return(codemode.Description{}, fmt.Errorf("trusted catalog: %w", codemode.ErrNotFound)).
					Once()
			},
			want: codemode.ErrNotFound.Error(),
		},
		{
			name:      "invalid program",
			tool:      "execute",
			arguments: map[string]any{"source": "broken"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("broken")).
					Return(nil, fmt.Errorf("trusted parse: %w", codemode.ErrInvalidProgram)).
					Once()
			},
			want: codemode.ErrInvalidProgram.Error(),
		},
		{
			name:      "invalid arguments",
			tool:      "execute",
			arguments: map[string]any{"source": "args"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("args")).
					Return(nil, fmt.Errorf("trusted args: %w", codemode.ErrInvalidArguments)).
					Once()
			},
			want: codemode.ErrInvalidArguments.Error(),
		},
		{
			name:      "permission denied",
			tool:      "execute",
			arguments: map[string]any{"source": "denied"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("denied")).
					Return(nil, fmt.Errorf("trusted denial: %w", codemode.ErrPermissionDenied)).
					Once()
			},
			want: codemode.ErrPermissionDenied.Error(),
		},
		{
			name:      "policy failure",
			tool:      "execute",
			arguments: map[string]any{"source": "policy"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("policy")).
					Return(nil, fmt.Errorf("trusted policy: %w", codemode.ErrPolicyFailure)).
					Once()
			},
			want: codemode.ErrPolicyFailure.Error(),
		},
		{
			name:      "resource limit",
			tool:      "search_api",
			arguments: map[string]any{"query": "oversized"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Search("oversized").
					Return(nil, fmt.Errorf("trusted budget: %w", codemode.ErrResourceLimit)).
					Once()
			},
			want: codemode.ErrResourceLimit.Error(),
		},
		{
			name:      "capability failure",
			tool:      "execute",
			arguments: map[string]any{"source": "handler"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("handler")).
					Return(nil, fmt.Errorf("trusted handler: %w", codemode.ErrCapabilityFailure)).
					Once()
			},
			want: codemode.ErrCapabilityFailure.Error(),
		},
		{
			name:      "canceled",
			tool:      "execute",
			arguments: map[string]any{"source": "canceled"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("canceled")).
					Return(nil, context.Canceled).
					Once()
			},
			want: context.Canceled.Error(),
		},
		{
			name:      "elapsed resource limit",
			tool:      "execute",
			arguments: map[string]any{"source": "deadline"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().
					Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("deadline")).
					Return(nil, fmt.Errorf("%w: %w", codemode.ErrResourceLimit, context.DeadlineExceeded)).
					Once()
			},
			want: codemode.ErrResourceLimit.Error(),
		},
		{
			name:      "unknown internal",
			tool:      "search_api",
			arguments: map[string]any{"query": "lookup"},
			configure: func(service *mocks.MockService) {
				service.EXPECT().Search("lookup").Return(nil, errors.New("trusted stack dump")).Once()
			},
			want: codemode.ErrInternal.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := mocks.NewMockService(t)
			resolver := mocks.NewMockInvocationResolver(t)
			resolver.EXPECT().Resolve(mock.Anything).Return(authz.Subject{ID: "subject-1"}, nil).Once()
			tt.configure(service)
			session := newTestSession(t, service, resolver)

			result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.arguments,
			})

			require.NoError(t, err)
			requireToolError(t, result, tt.want)
		})
	}
}

// TestToolsProjectRegoDecisionFailuresWithoutTrustedDetail proves undefined and
// non-Boolean ground decisions stay coarse at the official MCP execute boundary.
func TestToolsProjectRegoDecisionFailuresWithoutTrustedDetail(t *testing.T) {
	tests := []struct {
		// name identifies the broken ground decision.
		name string

		// module is the in-memory Rego source that produces that decision.
		module string
	}{
		{
			name:   "undefined ground decision",
			module: undefinedRegoPolicy(),
		},
		{
			name:   "non-boolean ground decision",
			module: nonBooleanRegoPolicy(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handlerCalls atomic.Int64
			server := buildRegoPolicyServer(t, tt.module, &handlerCalls)
			resolver := mocks.NewMockInvocationResolver(t)
			resolver.EXPECT().Resolve(mock.Anything).Return(authz.Subject{ID: "subject-1"}, nil).Once()
			session := newTestSession(t, server, resolver)

			result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "execute",
				Arguments: map[string]any{
					"source": `
def main():
    return records.lookup(key="alpha")
`,
				},
			})

			require.NoError(t, err)
			requireOpaquePolicyToolError(t, result)
			assert.Zero(t, handlerCalls.Load())
		})
	}
}

// TestToolsSanitizePanics proves adapter recovery never leaks panic text.
func TestToolsSanitizePanics(t *testing.T) {
	tests := []struct {
		// name identifies the recovered panic source.
		name string

		// configure installs one generated expectation that panics.
		configure func(*mocks.MockService, *mocks.MockInvocationResolver)
	}{
		{
			name: "resolver panic",
			configure: func(_ *mocks.MockService, resolver *mocks.MockInvocationResolver) {
				resolver.EXPECT().Resolve(mock.Anything).Run(func(context.Context) {
					panic("trusted resolver panic")
				}).Return(authz.Subject{}, nil).Once()
			},
		},
		{
			name: "service panic",
			configure: func(service *mocks.MockService, resolver *mocks.MockInvocationResolver) {
				resolver.EXPECT().Resolve(mock.Anything).Return(authz.Subject{ID: "subject-1"}, nil).Once()
				service.EXPECT().Search("lookup").Run(func(string) {
					panic("trusted service panic")
				}).Return(nil, nil).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := mocks.NewMockService(t)
			resolver := mocks.NewMockInvocationResolver(t)
			tt.configure(service, resolver)
			session := newTestSession(t, service, resolver)

			result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "search_api",
				Arguments: map[string]any{"query": "lookup"},
			})

			require.NoError(t, err)
			requireToolError(t, result, codemode.ErrInternal.Error())
		})
	}
}

// testSession owns one official in-memory client connected to the adapter.
type testSession struct {
	// client is the official MCP client session.
	client *mcp.ClientSession
}

// newTestSession connects an official client to New through in-memory transports.
func newTestSession(t *testing.T, service mcpserver.Service, resolver mcpserver.InvocationResolver) *testSession {
	t.Helper()

	server, err := mcpserver.New(service, resolver)
	require.NoError(t, err)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = serverSession.Close()
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "mcpserver-test", Version: "1"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = clientSession.Close()
	})
	return &testSession{client: clientSession}
}

// toolNames returns the listed tool names in listing order.
func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, len(tools))
	for index, tool := range tools {
		names[index] = tool.Name
	}
	return names
}

// listedTool returns the tools/list record named name.
func listedTool(t *testing.T, tools []*mcp.Tool, name string) *mcp.Tool {
	t.Helper()

	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	require.FailNow(t, "expected listed tool "+name)
	return nil
}

// listedDescription returns the tools/list description for name.
func listedDescription(t *testing.T, tools []*mcp.Tool, name string) string {
	t.Helper()
	return listedTool(t, tools, name).Description
}

// listedOutputSchema returns the advertised tools/list outputSchema for name.
func listedOutputSchema(t *testing.T, tools []*mcp.Tool, name string) map[string]any {
	t.Helper()
	return requireJSONObject(t, listedTool(t, tools, name).OutputSchema)
}

// requireSearchAPIOutputSchema requires search_api to advertise a non-null SearchResult array.
func requireSearchAPIOutputSchema(t *testing.T, schema map[string]any) {
	t.Helper()
	requireNonNullJSONType(t, schema, "array")
	requireSearchResultItemSchema(t, schema["items"])
}

// requireDescribeAPIOutputSchema requires describe_api to advertise an object whose
// required input and output properties are non-null field-shape arrays.
func requireDescribeAPIOutputSchema(t *testing.T, schema map[string]any) {
	t.Helper()
	requireNonNullJSONType(t, schema, "object")
	requireRequiredNames(t, schema, "name", "signature", "summary", "description", "input", "output")
	properties := requireJSONObject(t, schema["properties"])
	requireNonNullJSONType(t, requireJSONObject(t, properties["name"]), "string")
	requireNonNullJSONType(t, requireJSONObject(t, properties["signature"]), "string")
	requireNonNullJSONType(t, requireJSONObject(t, properties["summary"]), "string")
	requireNonNullJSONType(t, requireJSONObject(t, properties["description"]), "string")
	input := requireJSONObject(t, properties["input"])
	requireNonNullJSONType(t, input, "array")
	requireFieldShapeItemSchema(t, input["items"])
	output := requireJSONObject(t, properties["output"])
	requireNonNullJSONType(t, output, "array")
	requireFieldShapeItemSchema(t, output["items"])
}

// requireExecuteOutputSchema requires execute to advertise a non-null result object.
func requireExecuteOutputSchema(t *testing.T, schema map[string]any) {
	t.Helper()
	requireNonNullJSONType(t, schema, "object")
	requireRequiredNames(t, schema, "result")
	properties := requireJSONObject(t, schema["properties"])
	_, ok := properties["result"]
	require.True(t, ok, "execute outputSchema must describe a result property")
}

// requireSearchResultItemSchema requires items to be the inferred SearchResult object.
func requireSearchResultItemSchema(t *testing.T, items any) {
	t.Helper()
	item := requireJSONObject(t, items)
	requireNonNullJSONType(t, item, "object")
	requireRequiredNames(t, item, "name", "signature", "summary")
	properties := requireJSONObject(t, item["properties"])
	requireNonNullJSONType(t, requireJSONObject(t, properties["name"]), "string")
	requireNonNullJSONType(t, requireJSONObject(t, properties["signature"]), "string")
	requireNonNullJSONType(t, requireJSONObject(t, properties["summary"]), "string")
}

// requireFieldShapeItemSchema requires items to be the inferred field-shape object.
func requireFieldShapeItemSchema(t *testing.T, items any) {
	t.Helper()
	item := requireJSONObject(t, items)
	requireNonNullJSONType(t, item, "object")
	requireRequiredNames(t, item, "name", "type", "required")
	properties := requireJSONObject(t, item["properties"])
	requireNonNullJSONType(t, requireJSONObject(t, properties["name"]), "string")
	requireNonNullJSONType(t, requireJSONObject(t, properties["type"]), "string")
	requireNonNullJSONType(t, requireJSONObject(t, properties["required"]), "boolean")
}

// requireJSONObject decodes value as a JSON object.
func requireJSONObject(t *testing.T, value any) map[string]any {
	t.Helper()
	require.NotNil(t, value, "expected a JSON object")
	if object, ok := value.(map[string]any); ok {
		return object
	}
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var object map[string]any
	require.NoError(t, json.Unmarshal(raw, &object), "expected JSON object, got %s", raw)
	return object
}

// requireNonNullJSONType requires schema.type to be exactly typ and to exclude null.
func requireNonNullJSONType(t *testing.T, schema map[string]any, typ string) {
	t.Helper()
	require.NotNil(t, schema, "expected a JSON Schema object")
	switch typed := schema["type"].(type) {
	case string:
		assert.Equal(t, typ, typed, "schema type must be %q without null", typ)
	case []any:
		assert.Equal(t, []any{typ}, typed, "schema type must be exactly %q without null", typ)
	default:
		require.Fail(t, "schema type must be present and exclude null", "got %T %[1]v", schema["type"])
	}
}

// requireRequiredNames requires schema.required to include every name.
func requireRequiredNames(t *testing.T, schema map[string]any, names ...string) {
	t.Helper()
	required, ok := schema["required"].([]any)
	require.True(t, ok, "schema required must be an array")
	got := make([]string, 0, len(required))
	for _, item := range required {
		name, ok := item.(string)
		require.True(t, ok, "schema required entries must be strings")
		got = append(got, name)
	}
	for _, name := range names {
		assert.Contains(t, got, name)
	}
}

// requireNonNullJSONArray requires value to decode as a JSON array, not null.
func requireNonNullJSONArray(t *testing.T, value any) {
	t.Helper()
	require.NotNil(t, value, "expected a non-null JSON array")
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NotEqual(t, "null", string(raw), "expected a non-null JSON array")
	var array []any
	require.NoError(t, json.Unmarshal(raw, &array), "expected JSON array, got %s", raw)
	require.NotNil(t, array, "expected a non-null JSON array")
}

// requireJSONTextMirror requires one TextContent item that JSON-equals expected.
func requireJSONTextMirror(t *testing.T, result *mcp.CallToolResult, expected any) {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "successful content must be one JSON text item")
	raw, err := json.Marshal(expected)
	require.NoError(t, err)
	assert.JSONEq(t, string(raw), text.Text)
}

// requireSuccessfulStructuredValue requires a successful tool result whose structured
// content and JSON text mirror equal expected.
func requireSuccessfulStructuredValue(t *testing.T, result *mcp.CallToolResult, expected any) {
	t.Helper()
	require.NotNil(t, result)
	require.False(t, result.IsError, "tool call failed, content: %+v", result.Content)
	want, err := json.Marshal(expected)
	require.NoError(t, err)
	got, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
	requireJSONTextMirror(t, result, expected)
}

// requireToolValidationError asserts the SDK rejected malformed typed arguments.
func requireToolValidationError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()

	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.NotEmpty(t, result.Content)
}

// requireToolError asserts a successful protocol response carrying one coarse tool error.
func requireToolError(t *testing.T, result *mcp.CallToolResult, want string) {
	t.Helper()

	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, want, text.Text)
	assert.NotContains(t, text.Text, "trusted")
}

// policyLookupInput is the enabled records.lookup argument contract.
type policyLookupInput struct {
	// Key is the required record identifier.
	Key string `json:"key"`
}

// policyLookupResult is the unused handler output for a denied policy path.
type policyLookupResult struct {
	// Key is the looked-up record identifier.
	Key string `json:"key"`
}

// buildRegoPolicyServer registers one lookup capability in front of a real Rego authorizer.
func buildRegoPolicyServer(t *testing.T, module string, handlerCalls *atomic.Int64) *codemode.Server {
	t.Helper()
	authorizer := mustRegoAuthorizer(t, module)
	builder := codemode.New(codemode.Options{Authorizer: authorizer, Limits: codemode.DefaultLimits()})
	require.NoError(t, codemode.Register(builder, codemode.Capability[policyLookupInput, policyLookupResult]{
		ID:          "cap.lookup",
		Name:        "records.lookup",
		Summary:     "Return one record.",
		Description: "Returns the supplied record value.",
		Handler: func(context.Context, authz.Subject, policyLookupInput) (policyLookupResult, error) {
			handlerCalls.Add(1)
			return policyLookupResult{}, nil
		},
	}))
	server, err := builder.Build()
	require.NoError(t, err)
	return server
}

// mustRegoAuthorizer prepares one in-memory Rego authorizer or fails the test.
func mustRegoAuthorizer(t *testing.T, module string) *rego.Authorizer {
	t.Helper()
	authorizer, err := rego.New(t.Context(), "data.codemode.authz.allow", map[string]string{
		"authorization.rego": module,
	})
	require.NoError(t, err)
	return authorizer
}

// undefinedRegoPolicy returns a partial decision with no default.
func undefinedRegoPolicy() string {
	return `
package codemode.authz

allow if input.subject.id == "nobody"
`
}

// nonBooleanRegoPolicy returns a ground decision that is not Boolean.
func nonBooleanRegoPolicy() string {
	return `
package codemode.authz

allow := "yes"
`
}

// requireOpaquePolicyToolError asserts execute returned only the coarse policy-failure text.
func requireOpaquePolicyToolError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	requireToolError(t, result, codemode.ErrPolicyFailure.Error())
	assertNoRegoDiagnostics(t, fmt.Sprint(result.StructuredContent))
	for _, content := range result.Content {
		assertNoRegoDiagnostics(t, fmt.Sprint(content))
	}
}

// assertNoRegoDiagnostics requires text to omit trusted Rego diagnostic detail.
func assertNoRegoDiagnostics(t *testing.T, text string) {
	t.Helper()
	for _, leaked := range []string{
		"rego:",
		"data.codemode.authz",
		"authorization.rego",
		"decision is undefined",
		"decision must be boolean",
		"decision must be a single boolean",
		"evaluate decision",
		"builtin",
	} {
		assert.NotContains(t, text, leaked)
	}
}
