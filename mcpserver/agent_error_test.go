package mcpserver_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	authzmocks "github.com/meigma/codemode/authz/mocks"
	"github.com/meigma/codemode/mcpserver"
)

const (
	// failProgram invokes the on-demand failure capability.
	failProgram = `
def main():
    return records.fail()
`

	// scoreProgram invokes the non-finite conversion capability.
	scoreProgram = `
def main():
    return records.score()
`

	// agentErrorByteCap is the contracted AgentError suffix bound, including a trailing "...".
	agentErrorByteCap = 256

	// namedResourceDetail is a handler-authored named-resource failure.
	namedResourceDetail = `instance "web" not found in sandbox "demo"`

	// wrappedAgentDetail is the only text a wrapped AgentError may expose.
	wrappedAgentDetail = "x"

	// panicAgentDetail must not cross MCP after a recovered handler panic.
	panicAgentDetail = `panic-instance "web" not found`

	// policyAgentDetail must not cross MCP after a policy AgentError.
	policyAgentDetail = `policy-instance "web" not found`

	// fourByteRune is a printable 4-byte UTF-8 scalar used at the truncation boundary.
	fourByteRune = "\U00010000"

	// hiddenFourByteTail is text after a 4-byte rune that truncation must drop.
	hiddenFourByteTail = "hidden-4byte-tail"

	// hiddenSplitTail is text after a rune that would be split at byte 253.
	hiddenSplitTail = "hidden-split-tail"

	// hiddenTenKiBTail is text at the end of an oversized message that truncation must drop.
	hiddenTenKiBTail = "hidden-10kib-tail"
)

// nanScoreOutput is a handler result that conversion rejects as non-finite.
type nanScoreOutput struct {
	// Score is a floating-point field that may be NaN.
	Score float64 `json:"score"`
}

// TestActualMCPAgentErrorDetails proves MCP execute surfaces sanitized handler-authored
// AgentError suffixes and keeps ordinary, panic, policy, and conversion failures hidden.
func TestActualMCPAgentErrorDetails(t *testing.T) {
	ascii253 := strings.Repeat("A", 253)
	fourByteMessage := ascii253 + fourByteRune + hiddenFourByteTail
	fourByteWant := "capability failed: " + ascii253 + "..."

	ascii251 := strings.Repeat("B", 251)
	splitMessage := ascii251 + fourByteRune + hiddenSplitTail
	splitWant := "capability failed: " + ascii251 + "..."

	tenKiBHead := "instance-web"
	tenKiBControls := "\n\r\t\x00\x1f"
	tenKiBMessage := tenKiBHead + tenKiBControls + strings.Repeat("x", 10*1024) + hiddenTenKiBTail
	tenKiBSanitized := tenKiBHead + strings.Repeat(
		" ",
		len(tenKiBControls),
	) + strings.Repeat(
		"x",
		10*1024,
	) + hiddenTenKiBTail
	tenKiBWant := "capability failed: " + tenKiBSanitized[:agentErrorByteCap-len("...")] + "..."

	tests := []struct {
		// name identifies the MCP execute failure.
		name string

		// policyError is the authorizer failure. Nil selects AllowAll.
		policyError error

		// register installs the capability under test.
		register func(*codemode.Builder)

		// source is the execute program.
		source string

		// want is the exact MCP tool error text.
		want string

		// forbidden must not appear in the MCP payload.
		forbidden []string
	}{
		{
			name: "direct named-resource AgentError",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, &codemode.AgentError{Message: namedResourceDetail}
			}),
			source: failProgram,
			want:   "capability failed: " + namedResourceDetail,
		},
		{
			name: "wrapped AgentError hides wrapper secret",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, fmt.Errorf(
					"lookup: %s: %w",
					handlerPasswordCanary,
					&codemode.AgentError{Message: wrappedAgentDetail},
				)
			}),
			source:    failProgram,
			want:      "capability failed: " + wrappedAgentDetail,
			forbidden: []string{handlerPasswordCanary, "lookup:"},
		},
		{
			name: "ordinary error stays bare",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, errors.New(handlerPasswordCanary)
			}),
			source:    failProgram,
			want:      codemode.ErrCapabilityFailure.Error(),
			forbidden: []string{handlerPasswordCanary},
		},
		{
			name: "sanitized 10KiB controls and newlines",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, &codemode.AgentError{Message: tenKiBMessage}
			}),
			source:    failProgram,
			want:      tenKiBWant,
			forbidden: []string{hiddenTenKiBTail},
		},
		{
			name: "truncates before 4-byte rune",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, &codemode.AgentError{Message: fourByteMessage}
			}),
			source:    failProgram,
			want:      fourByteWant,
			forbidden: []string{fourByteRune, hiddenFourByteTail},
		},
		{
			name: "truncation inside multibyte sequence",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, &codemode.AgentError{Message: splitMessage}
			}),
			source:    failProgram,
			want:      splitWant,
			forbidden: []string{fourByteRune, hiddenSplitTail},
		},
		{
			name: "invalid UTF-8 becomes replacement rune",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, &codemode.AgentError{Message: "ok\xffmore"}
			}),
			source: failProgram,
			want:   "capability failed: ok\uFFFDmore",
		},
		{
			name: "empty AgentError stays bare",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, &codemode.AgentError{}
			}),
			source: failProgram,
			want:   codemode.ErrCapabilityFailure.Error(),
		},
		{
			name: "typed-nil AgentError stays bare",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				var typedNil *codemode.AgentError
				return lookupResult{}, typedNil
			}),
			source: failProgram,
			want:   codemode.ErrCapabilityFailure.Error(),
		},
		{
			name: "panic AgentError stays hidden",
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				panic(&codemode.AgentError{Message: panicAgentDetail})
			}),
			source:    failProgram,
			want:      codemode.ErrInternal.Error(),
			forbidden: []string{panicAgentDetail},
		},
		{
			name: "policy AgentError stays hidden",
			policyError: fmt.Errorf(
				"%s: %w",
				handlerPasswordCanary,
				&codemode.AgentError{Message: policyAgentDetail},
			),
			register: registerRecordsFail(func(context.Context, authz.Subject, struct{}) (lookupResult, error) {
				return lookupResult{}, errors.New(handlerPasswordCanary)
			}),
			source: failProgram,
			want:   codemode.ErrPolicyFailure.Error(),
			forbidden: []string{
				policyAgentDetail,
				handlerPasswordCanary,
				namedResourceDetail,
			},
		},
		{
			name: "conversion failure stays bare",
			register: func(builder *codemode.Builder) {
				codemode.Register(builder, codemode.Capability[struct{}, nanScoreOutput]{
					ID:          "records.entry.score",
					Name:        "records.score",
					Summary:     "Return a score.",
					Description: "Returns one floating-point score.",
					Handler: func(context.Context, authz.Subject, struct{}) (nanScoreOutput, error) {
						return nanScoreOutput{Score: math.NaN()}, nil
					},
				})
			},
			source: scoreProgram,
			want:   codemode.ErrCapabilityFailure.Error(),
			forbidden: []string{
				"not finite",
				"unsupported value",
				"NaN",
				handlerPasswordCanary,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executeMCPProgram(t, tt.policyError, tt.register, tt.source)

			assertToolError(t, result, tt.want)
			assertNoCanary(t, result)
			assertNotContainsText(t, result, tt.forbidden...)
			require.LessOrEqual(t, agentErrorDetailBytes(t, result), agentErrorByteCap)
		})
	}
}

// executeMCPProgram builds a real CodeMode server and runs source over in-memory MCP.
func executeMCPProgram(
	t *testing.T,
	policyError error,
	register func(*codemode.Builder),
	source string,
) *mcp.CallToolResult {
	t.Helper()
	var authorizer authz.Authorizer = authz.AllowAll()
	if policyError != nil {
		policy := authzmocks.NewMockAuthorizer(t)
		policy.EXPECT().Authorize(mock.Anything, mock.Anything).Return(policyError).Once()
		authorizer = policy
	}
	builder := codemode.New(codemode.Options{
		Authorizer: authorizer,
		Limits:     codemode.DefaultLimits(),
	})
	register(builder)
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

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "execute",
		Arguments: map[string]any{"source": source},
	})
	require.NoError(t, err)
	return result
}

// registerRecordsFail installs records.fail with the supplied handler.
func registerRecordsFail(
	handler func(context.Context, authz.Subject, struct{}) (lookupResult, error),
) func(*codemode.Builder) {
	return func(builder *codemode.Builder) {
		codemode.Register(builder, codemode.Capability[struct{}, lookupResult]{
			ID:          "records.entry.fail",
			Name:        "records.fail",
			Summary:     "Fail on demand.",
			Description: "Returns one handler-authored failure.",
			Handler:     handler,
		})
	}
}

// agentErrorDetailBytes returns the MCP suffix length after "capability failed: ", or 0 when bare.
func agentErrorDetailBytes(t *testing.T, result *mcp.CallToolResult) int {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "tool error content must be text")
	detail, ok := strings.CutPrefix(text.Text, codemode.ErrCapabilityFailure.Error()+": ")
	if !ok {
		return 0
	}
	return len(detail)
}
