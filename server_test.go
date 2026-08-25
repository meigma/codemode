package codemode_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	authzmocks "github.com/meigma/codemode/authz/mocks"
	"github.com/meigma/codemode/authz/rego"
)

// TestServerSearchAndDescribeExposeOnlyEnabledCapabilities proves public discovery uses the filtered catalog.
func TestServerSearchAndDescribeExposeOnlyEnabledCapabilities(t *testing.T) {
	builder := codemode.New(codemode.Options{
		Authorizer:           authz.AllowAll(),
		DisabledCapabilities: []codemode.CapabilityID{"cap.disabled"},
		Limits:               codemode.DefaultLimits(),
	})
	require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.alpha", "records.alpha")))
	require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.disabled", "records.disabled")))
	server, err := builder.Build()
	require.NoError(t, err)

	results, err := server.Search("record")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "records.alpha", results[0].Name)
	description, err := server.Describe("records.alpha")
	require.NoError(t, err)
	assert.Equal(t, "records.alpha", description.Name)
	_, err = server.Describe("records.disabled")
	require.ErrorIs(t, err, codemode.ErrNotFound)
}

// TestServerSearchProjectsQueryLimits proves internal discovery details do not escape the public taxonomy.
func TestServerSearchProjectsQueryLimits(t *testing.T) {
	limits := codemode.DefaultLimits()
	limits.MaxSearchQueryBytes = 4
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: limits})
	server, err := builder.Build()
	require.NoError(t, err)

	_, err = server.Search("oversized")

	require.ErrorIs(t, err, codemode.ErrResourceLimit)
	assert.Equal(t, codemode.ErrResourceLimit.Error(), err.Error())
}

// TestServerExecuteAuthorizesCanonicalArgumentsBeforeDispatch proves the complete native-call ordering.
func TestServerExecuteAuthorizesCanonicalArgumentsBeforeDispatch(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	events := make([]string, 0, 2)
	authorizer.EXPECT().Authorize(mock.Anything, mock.MatchedBy(func(input authz.AuthorizationInput) bool {
		return input.Subject.ID == "subject-1" &&
			input.CapabilityID == "cap.lookup" &&
			input.CapabilityName == "records.lookup" &&
			assert.ObjectsAreEqual(map[string]any{"value": "alpha"}, input.Arguments)
	})).Run(func(context.Context, authz.AuthorizationInput) {
		events = append(events, "authorize")
	}).Return(nil).Once()
	capability := validBuilderCapability("cap.lookup", "records.lookup")
	capability.Handler = func(_ context.Context, _ authz.Subject, input builderInput) (builderOutput, error) {
		events = append(events, "handler")
		return builderOutput(input), nil
	}
	server := buildTestServer(t, authorizer, codemode.DefaultLimits(), capability)

	result, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup(value="alpha")
`)

	require.NoError(t, err)
	assert.Equal(t, []string{"authorize", "handler"}, events)
	assert.Equal(t, map[string]any{"value": "alpha"}, result)
}

// TestServerExecuteRejectsArgumentsBeforeAuthorization proves malformed calls cannot reach policy or handlers.
func TestServerExecuteRejectsArgumentsBeforeAuthorization(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	var handlerCalls atomic.Int64
	capability := validBuilderCapability("cap.lookup", "records.lookup")
	capability.Handler = func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
		handlerCalls.Add(1)
		return builderOutput{}, nil
	}
	server := buildTestServer(t, authorizer, codemode.DefaultLimits(), capability)

	_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup()
`)

	require.ErrorIs(t, err, codemode.ErrInvalidArguments)
	assert.Zero(t, handlerCalls.Load())
}

// TestServerExecuteRejectsTopLevelNativeCalls proves loading code cannot bypass the running-phase guard.
func TestServerExecuteRejectsTopLevelNativeCalls(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	var handlerCalls atomic.Int64
	capability := validBuilderCapability("cap.lookup", "records.lookup")
	capability.Handler = func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
		handlerCalls.Add(1)
		return builderOutput{}, nil
	}
	server := buildTestServer(t, authorizer, codemode.DefaultLimits(), capability)

	_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
value = records.lookup(value="alpha")
def main():
    return value
`)

	require.ErrorIs(t, err, codemode.ErrInvalidProgram)
	assert.Zero(t, handlerCalls.Load())
}

// TestServerExecuteFailsClosedBeforeHandlerDispatch proves denial and policy failures have no handler side effects.
func TestServerExecuteFailsClosedBeforeHandlerDispatch(t *testing.T) {
	tests := []struct {
		// name identifies the policy failure.
		name string

		// configure installs one generated mock expectation.
		configure func(*authzmocks.MockAuthorizer)

		// target is the expected public classification.
		target error
	}{
		{
			name: "recognized denial",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(authz.ErrDenied).Once()
			},
			target: codemode.ErrPermissionDenied,
		},
		{
			name: "policy error",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().
					Authorize(mock.Anything, mock.Anything).
					Return(errors.New("trusted policy detail")).
					Once()
			},
			target: codemode.ErrPolicyFailure,
		},
		{
			name: "policy cancellation",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(context.Canceled).Once()
			},
			target: codemode.ErrPolicyFailure,
		},
		{
			name: "policy panic",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().
					Authorize(mock.Anything, mock.Anything).
					Run(func(context.Context, authz.AuthorizationInput) {
						panic("trusted panic detail")
					}).
					Return(nil).
					Once()
			},
			target: codemode.ErrPolicyFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := authzmocks.NewMockAuthorizer(t)
			tt.configure(authorizer)
			var handlerCalls atomic.Int64
			capability := validBuilderCapability("cap.lookup", "records.lookup")
			capability.Handler = func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
				handlerCalls.Add(1)
				return builderOutput{}, nil
			}
			server := buildTestServer(t, authorizer, codemode.DefaultLimits(), capability)

			_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup(value="alpha")
`)

			require.ErrorIs(t, err, tt.target)
			assert.Equal(t, tt.target.Error(), err.Error())
			assert.Zero(t, handlerCalls.Load())
		})
	}
}

// TestServerExecuteProjectsRegoDecisionFailuresWithoutTrustedDetail proves
// undefined and non-Boolean ground decisions stay coarse at the root boundary.
func TestServerExecuteProjectsRegoDecisionFailuresWithoutTrustedDetail(t *testing.T) {
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
			authorizer := mustRegoAuthorizer(t, tt.module)
			var handlerCalls atomic.Int64
			capability := validBuilderCapability("cap.lookup", "records.lookup")
			capability.Handler = func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
				handlerCalls.Add(1)
				return builderOutput{}, nil
			}
			server := buildTestServer(t, authorizer, codemode.DefaultLimits(), capability)

			_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup(value="alpha")
`)

			require.ErrorIs(t, err, codemode.ErrPolicyFailure)
			assert.Equal(t, codemode.ErrPolicyFailure.Error(), err.Error())
			assertNoRegoDiagnostics(t, err.Error())
			assert.Zero(t, handlerCalls.Load())
		})
	}
}

// TestServerExecuteProjectsHandlerFailuresWithoutTrustedDetail proves native failures remain opaque.
func TestServerExecuteProjectsHandlerFailuresWithoutTrustedDetail(t *testing.T) {
	tests := []struct {
		// name identifies the handler failure.
		name string

		// handler is the failing native implementation.
		handler codemode.Handler[builderInput, builderOutput]

		// target is the expected public classification.
		target error
	}{
		{
			name: "handler error",
			handler: func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
				return builderOutput{}, errors.New("trusted handler detail")
			},
			target: codemode.ErrCapabilityFailure,
		},
		{
			name: "handler panic",
			handler: func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
				panic("trusted handler panic")
			},
			target: codemode.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability := validBuilderCapability("cap.lookup", "records.lookup")
			capability.Handler = tt.handler
			server := buildTestServer(t, authz.AllowAll(), codemode.DefaultLimits(), capability)

			_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup(value="alpha")
`)

			require.ErrorIs(t, err, tt.target)
			assert.Equal(t, tt.target.Error(), err.Error())
		})
	}
}

// TestServerExecuteEnforcesSourceCallStepAndValueLimits proves each public budget fails safely.
func TestServerExecuteEnforcesSourceCallStepAndValueLimits(t *testing.T) {
	t.Run("source bytes", func(t *testing.T) {
		limits := codemode.DefaultLimits()
		limits.MaxSourceBytes = 8
		server := buildTestServer(
			t,
			authz.AllowAll(),
			limits,
			validBuilderCapability("cap.lookup", "records.lookup"),
		)

		_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `def main(): return None`)

		require.ErrorIs(t, err, codemode.ErrResourceLimit)
	})

	t.Run("native calls", func(t *testing.T) {
		limits := codemode.DefaultLimits()
		limits.MaxNativeCalls = 1
		var handlerCalls atomic.Int64
		capability := validBuilderCapability("cap.lookup", "records.lookup")
		capability.Handler = func(_ context.Context, _ authz.Subject, input builderInput) (builderOutput, error) {
			handlerCalls.Add(1)
			return builderOutput(input), nil
		}
		server := buildTestServer(t, authz.AllowAll(), limits, capability)

		_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    records.lookup(value="first")
    return records.lookup(value="second")
`)

		require.ErrorIs(t, err, codemode.ErrResourceLimit)
		assert.Equal(t, int64(1), handlerCalls.Load())
	})

	t.Run("execution steps", func(t *testing.T) {
		limits := codemode.DefaultLimits()
		limits.MaxExecutionSteps = 20
		server := buildTestServer(
			t,
			authz.AllowAll(),
			limits,
			validBuilderCapability("cap.lookup", "records.lookup"),
		)

		_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    total = 0
    for item in range(1000000):
        total += item
    return total
`)

		require.ErrorIs(t, err, codemode.ErrResourceLimit)
	})

	t.Run("value bytes", func(t *testing.T) {
		limits := codemode.DefaultLimits()
		limits.MaxValueBytes = 4
		server := buildTestServer(
			t,
			authz.AllowAll(),
			limits,
			validBuilderCapability("cap.lookup", "records.lookup"),
		)

		_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup(value="oversized")
`)

		require.ErrorIs(t, err, codemode.ErrResourceLimit)
	})
}

// TestServerExecuteEnforcesAggregateIntermediateLimit proves aggregate exhaustion
// projects only ErrResourceLimit and stays independent of MaxValueBytes.
func TestServerExecuteEnforcesAggregateIntermediateLimit(t *testing.T) {
	const result = "xx"
	body := `{"value":"` + result + `"}`
	limits := codemode.DefaultLimits()
	limits.MaxValueBytes = 64
	limits.MaxIntermediateValueBytes = len(body)*2 - 1
	var handlerCalls atomic.Int64
	capability := validBuilderCapability("cap.lookup", "records.lookup")
	capability.Handler = func(_ context.Context, _ authz.Subject, _ builderInput) (builderOutput, error) {
		handlerCalls.Add(1)
		return builderOutput{Value: result}, nil
	}
	server := buildTestServer(t, authz.AllowAll(), limits, capability)

	_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    records.lookup(value="first")
    return records.lookup(value="second")
`)

	require.ErrorIs(t, err, codemode.ErrResourceLimit)
	assert.Equal(t, codemode.ErrResourceLimit, err)
	assert.Equal(t, int64(2), handlerCalls.Load())
}

// compositeProjectionInput is the empty input used by public composite projections.
type compositeProjectionInput struct{}

// nanProjectionOutput covers nested non-finite floating-point results.
type nanProjectionOutput struct {
	// Score is a finite floating-point field.
	Score float64 `json:"score"`
}

// overflowProjectionOutput covers unsigned values above MaxInt64.
type overflowProjectionOutput struct {
	// Count is an unsigned 64-bit integer.
	Count uint64 `json:"count"`
}

// nestedProjectionItem is one nested object used for depth and budget cases.
type nestedProjectionItem struct {
	// ID is the nested row identifier.
	ID string `json:"id"`
}

// nestedProjectionOutput is a composite result whose nesting exceeds MaxValueDepth 2.
type nestedProjectionOutput struct {
	// Items is the compiled list of nested objects.
	Items []nestedProjectionItem `json:"items"`
}

// TestServerExecuteProjectsCompositeOutputFailures proves nested NaN, Inf, and
// unsigned overflow become the bare public capability-failure sentinel.
func TestServerExecuteProjectsCompositeOutputFailures(t *testing.T) {
	tests := []struct {
		// name identifies the invalid handler output.
		name string

		// capabilityName is the Starlark capability invoked by main.
		capabilityName string

		// register retains one capability that returns an invalid composite value.
		register func(*codemode.Builder) error
	}{
		{
			name:           "nested NaN",
			capabilityName: "records.nan",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(builder, codemode.Capability[compositeProjectionInput, nanProjectionOutput]{
					ID:          "cap.nan",
					Name:        "records.nan",
					Summary:     "Return a NaN score.",
					Description: "Projects non-finite floats as capability failure.",
					Handler: func(context.Context, authz.Subject, compositeProjectionInput) (nanProjectionOutput, error) {
						return nanProjectionOutput{Score: math.NaN()}, nil
					},
				})
			},
		},
		{
			name:           "nested Inf",
			capabilityName: "records.inf",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(builder, codemode.Capability[compositeProjectionInput, nanProjectionOutput]{
					ID:          "cap.inf",
					Name:        "records.inf",
					Summary:     "Return an infinite score.",
					Description: "Projects non-finite floats as capability failure.",
					Handler: func(context.Context, authz.Subject, compositeProjectionInput) (nanProjectionOutput, error) {
						return nanProjectionOutput{Score: math.Inf(1)}, nil
					},
				})
			},
		},
		{
			name:           "uint overflow",
			capabilityName: "records.overflow",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(
					builder,
					codemode.Capability[compositeProjectionInput, overflowProjectionOutput]{
						ID:          "cap.overflow",
						Name:        "records.overflow",
						Summary:     "Return an overflowing unsigned count.",
						Description: "Projects unsigned overflow as capability failure.",
						Handler: func(context.Context, authz.Subject, compositeProjectionInput) (overflowProjectionOutput, error) {
							return overflowProjectionOutput{Count: uint64(1) << 63}, nil
						},
					},
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: codemode.DefaultLimits()})
			require.NoError(t, tt.register(builder))
			server, err := builder.Build()
			require.NoError(t, err)

			_, err = server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, codemode.Program(`
def main():
    return `+tt.capabilityName+`()
`))

			require.ErrorIs(t, err, codemode.ErrCapabilityFailure)
			assert.Equal(t, codemode.ErrCapabilityFailure, err)
		})
	}
}

// TestServerExecuteProjectsCompositeValueLimits proves composite depth and
// independent per-value/aggregate budgets project only the bare resource sentinel.
func TestServerExecuteProjectsCompositeValueLimits(t *testing.T) {
	t.Run("max value depth", func(t *testing.T) {
		limits := codemode.DefaultLimits()
		limits.MaxValueDepth = 2
		limits.MaxValueBytes = 1024
		limits.MaxIntermediateValueBytes = 1024
		builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: limits})
		require.NoError(
			t,
			codemode.Register(builder, codemode.Capability[compositeProjectionInput, nestedProjectionOutput]{
				ID:          "cap.depth",
				Name:        "records.depth",
				Summary:     "Return nested items.",
				Description: "Projects composite depth exhaustion as a resource limit.",
				Handler: func(context.Context, authz.Subject, compositeProjectionInput) (nestedProjectionOutput, error) {
					return nestedProjectionOutput{Items: []nestedProjectionItem{{ID: "a"}}}, nil
				},
			}),
		)
		server, err := builder.Build()
		require.NoError(t, err)

		_, err = server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.depth()
`)

		require.ErrorIs(t, err, codemode.ErrResourceLimit)
		assert.Equal(t, codemode.ErrResourceLimit, err)
	})

	t.Run("per-value bytes", func(t *testing.T) {
		const body = `{"items":[{"id":"xx"}]}`
		limits := codemode.DefaultLimits()
		limits.MaxValueBytes = len(body) - 1
		limits.MaxIntermediateValueBytes = 1024
		var handlerCalls atomic.Int64
		builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: limits})
		require.NoError(
			t,
			codemode.Register(builder, codemode.Capability[compositeProjectionInput, nestedProjectionOutput]{
				ID:          "cap.pervalue",
				Name:        "records.pervalue",
				Summary:     "Return one nested item.",
				Description: "Projects one oversized composite result independently of the aggregate budget.",
				Handler: func(context.Context, authz.Subject, compositeProjectionInput) (nestedProjectionOutput, error) {
					handlerCalls.Add(1)
					return nestedProjectionOutput{Items: []nestedProjectionItem{{ID: "xx"}}}, nil
				},
			}),
		)
		server, err := builder.Build()
		require.NoError(t, err)

		_, err = server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.pervalue()
`)

		require.ErrorIs(t, err, codemode.ErrResourceLimit)
		assert.Equal(t, codemode.ErrResourceLimit, err)
		assert.Equal(t, int64(1), handlerCalls.Load())
	})

	t.Run("aggregate intermediate bytes", func(t *testing.T) {
		const body = `{"items":[{"id":"xx"}]}`
		limits := codemode.DefaultLimits()
		limits.MaxValueBytes = 1024
		limits.MaxIntermediateValueBytes = len(body)*2 - 1
		var handlerCalls atomic.Int64
		builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: limits})
		require.NoError(
			t,
			codemode.Register(builder, codemode.Capability[compositeProjectionInput, nestedProjectionOutput]{
				ID:          "cap.aggregate",
				Name:        "records.aggregate",
				Summary:     "Return one nested item.",
				Description: "Projects composite aggregate exhaustion independently of MaxValueBytes.",
				Handler: func(context.Context, authz.Subject, compositeProjectionInput) (nestedProjectionOutput, error) {
					handlerCalls.Add(1)
					return nestedProjectionOutput{Items: []nestedProjectionItem{{ID: "xx"}}}, nil
				},
			}),
		)
		server, err := builder.Build()
		require.NoError(t, err)

		_, err = server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    records.aggregate()
    return records.aggregate()
`)

		require.ErrorIs(t, err, codemode.ErrResourceLimit)
		assert.Equal(t, codemode.ErrResourceLimit, err)
		assert.Equal(t, int64(2), handlerCalls.Load())
	})
}

// TestServerExecuteReturnsOnlyMainResult proves top-level values and printed text do not escape.
func TestServerExecuteReturnsOnlyMainResult(t *testing.T) {
	server := buildTestServer(
		t,
		authz.AllowAll(),
		codemode.DefaultLimits(),
		validBuilderCapability("cap.lookup", "records.lookup"),
	)

	result, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
print("discarded")
intermediate = "hidden"
def main():
    return records.lookup(value="visible")
`)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "visible"}, result)
}

// TestServerExecuteRequiresTrustedInvocationInputs proves nil contexts and empty subjects fail before execution.
func TestServerExecuteRequiresTrustedInvocationInputs(t *testing.T) {
	server := buildTestServer(
		t,
		authz.AllowAll(),
		codemode.DefaultLimits(),
		validBuilderCapability("cap.lookup", "records.lookup"),
	)

	//nolint:staticcheck // A nil context is the behavior under test.
	_, err := server.Execute(nil, authz.Subject{ID: "subject-1"}, `def main(): return None`)
	require.ErrorIs(t, err, codemode.ErrInternal)
	_, err = server.Execute(t.Context(), authz.Subject{}, `def main(): return None`)
	require.ErrorIs(t, err, codemode.ErrUnauthenticated)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = server.Execute(ctx, authz.Subject{ID: "subject-1"}, `def main(): return None`)
	require.ErrorIs(t, err, context.Canceled)
	deadlineCtx, deadlineCancel := context.WithTimeout(t.Context(), 0)
	defer deadlineCancel()
	_, err = server.Execute(deadlineCtx, authz.Subject{ID: "subject-1"}, `def main(): return None`)
	require.ErrorIs(t, err, codemode.ErrResourceLimit)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestServerExecuteElapsedBudgetAfterAllowPreventsHandler proves only MaxExecutionTime
// can expire a blocked allow before the handler starts.
func TestServerExecuteElapsedBudgetAfterAllowPreventsHandler(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	var handlerCalls atomic.Int64
	authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Run(
		func(ctx context.Context, _ authz.AuthorizationInput) {
			<-ctx.Done()
		},
	).Return(nil).Once()
	capability := validBuilderCapability("cap.lookup", "records.lookup")
	capability.Handler = func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
		handlerCalls.Add(1)
		return builderOutput{}, nil
	}
	limits := codemode.DefaultLimits()
	limits.MaxExecutionTime = 2 * time.Second
	server := buildTestServer(t, authorizer, limits, capability)

	_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup(value="alpha")
`)

	require.ErrorIs(t, err, codemode.ErrResourceLimit)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Zero(t, handlerCalls.Load())
}

// TestServerSupportsConcurrentDiscoveryAndExecution proves immutable server state is race-safe for parallel reads.
func TestServerSupportsConcurrentDiscoveryAndExecution(t *testing.T) {
	var handlerCalls atomic.Int64
	capability := validBuilderCapability("cap.lookup", "records.lookup")
	capability.Handler = func(_ context.Context, _ authz.Subject, input builderInput) (builderOutput, error) {
		handlerCalls.Add(1)
		return builderOutput(input), nil
	}
	server := buildTestServer(t, authz.AllowAll(), codemode.DefaultLimits(), capability)

	const workers = 32
	var workersDone sync.WaitGroup
	failures := make(chan error, workers)
	for range workers {
		workersDone.Go(func() {
			if _, err := server.Search("lookup"); err != nil {
				failures <- err
				return
			}
			if _, err := server.Describe("records.lookup"); err != nil {
				failures <- err
				return
			}
			_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.lookup(value="alpha")
`)
			if err != nil {
				failures <- err
			}
		})
	}
	workersDone.Wait()
	close(failures)

	for err := range failures {
		require.NoError(t, err)
	}
	assert.Equal(t, int64(workers), handlerCalls.Load())
}

// buildTestServer registers one capability and returns a successfully built server.
func buildTestServer(
	t *testing.T,
	authorizer authz.Authorizer,
	limits codemode.Limits,
	capability codemode.Capability[builderInput, builderOutput],
) *codemode.Server {
	t.Helper()
	builder := codemode.New(codemode.Options{Authorizer: authorizer, Limits: limits})
	require.NoError(t, codemode.Register(builder, capability))
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

// assertNoRegoDiagnostics requires public error text to omit trusted Rego detail.
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
