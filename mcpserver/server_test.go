package mcpserver_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
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

// TestNewRegistersExactlyThreeTools proves the adapter exposes only the three official tools.
func TestNewRegistersExactlyThreeTools(t *testing.T) {
	session := newTestSession(t, mocks.NewMockService(t), mocks.NewMockInvocationResolver(t))

	listed, err := session.client.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 3)
	assert.Equal(t, []string{"describe_api", "execute", "search_api"}, toolNames(listed.Tools))
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
			{Name: "records.lookup", Signature: "records.lookup() -> object", Summary: "lookup"},
		}, nil
	}).Once()
	service.EXPECT().
		Describe(codemode.CapabilityName("records.lookup")).
		RunAndReturn(func(codemode.CapabilityName) (codemode.Description, error) {
			events = append(events, "describe")
			return codemode.Description{
				Name:      "records.lookup",
				Signature: "records.lookup() -> object",
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
		"signature":   "records.lookup() -> object",
		"summary":     "lookup",
		"description": "",
		"input":       nil,
		"output":      nil,
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
