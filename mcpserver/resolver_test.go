package mcpserver_test

import (
	"context"
	"errors"
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

// TestStockResolversResolveTrustedSubjects proves fixed and context-owned
// identities succeed while missing or empty identities fail closed.
func TestStockResolversResolveTrustedSubjects(t *testing.T) {
	subject := authz.Subject{ID: "subject-1"}
	tests := []struct {
		// name identifies the resolver behavior.
		name string

		// resolver is the stock resolver under test.
		resolver mcpserver.InvocationResolver

		// ctx is the host-owned invocation context.
		ctx context.Context

		// want is the expected trusted identity.
		want authz.Subject

		// wantError reports whether resolution must fail closed.
		wantError bool
	}{
		{
			name:     "static subject",
			resolver: mcpserver.StaticSubject(subject),
			ctx:      context.Background(),
			want:     subject,
		},
		{
			name:      "empty static subject",
			resolver:  mcpserver.StaticSubject(authz.Subject{}),
			ctx:       context.Background(),
			wantError: true,
		},
		{
			name:     "context subject",
			resolver: mcpserver.ContextSubject(),
			ctx:      authz.WithSubject(context.Background(), subject),
			want:     subject,
		},
		{
			name:      "missing context subject",
			resolver:  mcpserver.ContextSubject(),
			ctx:       context.Background(),
			wantError: true,
		},
		{
			name:      "empty context subject",
			resolver:  mcpserver.ContextSubject(),
			ctx:       authz.WithSubject(context.Background(), authz.Subject{}),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.resolver.Resolve(tt.ctx)
			if tt.wantError {
				require.ErrorIs(t, err, codemode.ErrUnauthenticated)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolverFailureStopsEveryToolBeforeServiceWork proves authentication precedes all three operations.
func TestResolverFailureStopsEveryToolBeforeServiceWork(t *testing.T) {
	tools := []struct {
		// name is the official tool that must fail closed.
		name string

		// arguments are valid tool arguments that would otherwise reach the service.
		arguments map[string]any
	}{
		{name: "search_api", arguments: map[string]any{"query": "lookup"}},
		{name: "describe_api", arguments: map[string]any{"name": "records.lookup"}},
		{name: "execute", arguments: map[string]any{"source": "source"}},
	}

	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			service := mocks.NewMockService(t)
			resolver := mocks.NewMockInvocationResolver(t)
			resolver.EXPECT().
				Resolve(mock.Anything).
				Return(authz.Subject{}, errors.New("trusted identity store")).
				Once()
			session := newTestSession(t, service, resolver)

			result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tool.name,
				Arguments: tool.arguments,
			})

			require.NoError(t, err)
			requireToolError(t, result, codemode.ErrUnauthenticated.Error())
		})
	}
}

// TestEmptySubjectStopsEveryToolBeforeServiceWork proves a blank identity is unauthenticated.
func TestEmptySubjectStopsEveryToolBeforeServiceWork(t *testing.T) {
	service := mocks.NewMockService(t)
	resolver := mcpserver.StaticSubject(authz.Subject{})
	session := newTestSession(t, service, resolver)

	result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "execute",
		Arguments: map[string]any{"source": "source"},
	})

	require.NoError(t, err)
	requireToolError(t, result, codemode.ErrUnauthenticated.Error())
}

// TestResolvedSubjectIsPassedToExecute proves execute uses the trusted subject, not request metadata.
func TestResolvedSubjectIsPassedToExecute(t *testing.T) {
	service := mocks.NewMockService(t)
	resolver := mocks.NewMockInvocationResolver(t)
	resolver.EXPECT().Resolve(mock.Anything).Return(authz.Subject{ID: "subject-1"}, nil).Once()
	service.EXPECT().
		Execute(mock.Anything, authz.Subject{ID: "subject-1"}, codemode.Program("source")).
		Return("ok", nil).
		Once()
	session := newTestSession(t, service, resolver)

	result, err := session.client.CallTool(t.Context(), &mcp.CallToolParams{
		Meta:      mcp.Meta{"subject": "attacker"},
		Name:      "execute",
		Arguments: map[string]any{"source": "source"},
	})

	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Equal(t, map[string]any{"result": "ok"}, result.StructuredContent)
}
