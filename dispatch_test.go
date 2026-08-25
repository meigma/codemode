package codemode

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/authz"
	authzmocks "github.com/meigma/codemode/authz/mocks"
	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/catalog"
	"github.com/meigma/codemode/internal/execution"
	"github.com/meigma/codemode/internal/worker"
)

// dispatchInput is the representative dispatcher test input.
type dispatchInput struct {
	// Value is one required capability argument.
	Value string `json:"value"`
}

// dispatchOutput is the representative dispatcher test output.
type dispatchOutput struct {
	// Value is one handler result field.
	Value string `json:"value"`
}

// dispatchOutcome carries one asynchronous dispatcher result.
type dispatchOutcome struct {
	// value is the converted native result.
	value any

	// err is the classified dispatcher failure.
	err error
}

// TestDispatchBindsAuthorizesThenInvokes proves parent re-binding precedes policy and handlers.
func TestDispatchBindsAuthorizesThenInvokes(t *testing.T) {
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
	subject := newDispatchSubject(t, authorizer, func(_ context.Context, _ authz.Subject, input any) (any, error) {
		events = append(events, "handler")
		typed, ok := input.(dispatchInput)
		if !ok {
			return nil, catalog.ErrInputTypeMismatch
		}
		return dispatchOutput(typed), nil
	})

	result, err := subject.dispatch.dispatch(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		"cap.lookup",
		map[string]any{"value": "alpha"},
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"authorize", "handler"}, events)
	assert.Equal(t, map[string]any{"value": "alpha"}, result)
}

// TestDispatchTranslatesEveryBindValueFailureInternally proves decoded maps never reach policy.
func TestDispatchTranslatesEveryBindValueFailureInternally(t *testing.T) {
	tests := []struct {
		// name identifies the rejected decoded map.
		name string

		// arguments is the decoded child map presented to dispatch.
		arguments map[string]any
	}{
		{name: "missing required", arguments: map[string]any{}},
		{name: "unknown", arguments: map[string]any{"value": "alpha", "other": "extra"}},
		{name: "mistyped required string", arguments: map[string]any{"value": int64(1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := authzmocks.NewMockAuthorizer(t)
			var handlerCalls atomic.Int64
			subject := newDispatchSubject(t, authorizer, func(context.Context, authz.Subject, any) (any, error) {
				handlerCalls.Add(1)
				return dispatchOutput{}, nil
			})

			_, err := subject.dispatch.dispatch(
				t.Context(),
				authz.Subject{ID: "subject-1"},
				"cap.lookup",
				tt.arguments,
			)

			require.ErrorIs(t, err, worker.ErrProtocol)
			require.NotErrorIs(t, err, execution.ErrInvalidArguments)
			assert.Zero(t, handlerCalls.Load())
		})
	}
}

// TestDispatchRejectsUnknownIDsBeforeAuthorization proves disabled and unknown IDs stay internal.
func TestDispatchRejectsUnknownIDsBeforeAuthorization(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	var handlerCalls atomic.Int64
	subject := newDispatchSubject(t, authorizer, func(context.Context, authz.Subject, any) (any, error) {
		handlerCalls.Add(1)
		return dispatchOutput{}, nil
	})

	_, err := subject.dispatch.dispatch(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		"cap.missing",
		map[string]any{"value": "alpha"},
	)

	require.ErrorIs(t, err, worker.ErrProtocol)
	assert.Zero(t, handlerCalls.Load())
}

// TestDispatchClassifiesPolicyAndHandlerFailures proves policy and handler failures map to native classifications.
func TestDispatchClassifiesPolicyAndHandlerFailures(t *testing.T) {
	tests := []struct {
		// name identifies the classified failure.
		name string

		// configure installs one generated mock expectation.
		configure func(*authzmocks.MockAuthorizer)

		// invoke is the native handler under test.
		invoke catalog.Invoker

		// target is the expected dispatcher classification.
		target error
	}{
		{
			name: "recognized denial",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(authz.ErrDenied).Once()
			},
			invoke: func(context.Context, authz.Subject, any) (any, error) {
				return dispatchOutput{}, nil
			},
			target: execution.ErrPermissionDenied,
		},
		{
			name: "policy error",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().
					Authorize(mock.Anything, mock.Anything).
					Return(errors.New("trusted policy detail")).
					Once()
			},
			invoke: func(context.Context, authz.Subject, any) (any, error) {
				return dispatchOutput{}, nil
			},
			target: execution.ErrPolicyFailure,
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
			invoke: func(context.Context, authz.Subject, any) (any, error) {
				return dispatchOutput{}, nil
			},
			target: execution.ErrPolicyFailure,
		},
		{
			name: "handler error",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(nil).Once()
			},
			invoke: func(context.Context, authz.Subject, any) (any, error) {
				return dispatchOutput{}, errors.New("trusted handler detail")
			},
			target: execution.ErrCapabilityFailure,
		},
		{
			name: "handler panic",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(nil).Once()
			},
			invoke: func(context.Context, authz.Subject, any) (any, error) {
				panic("trusted handler panic")
			},
			target: execution.ErrInternal,
		},
		{
			name: "input type mismatch",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(nil).Once()
			},
			invoke: func(context.Context, authz.Subject, any) (any, error) {
				return nil, catalog.ErrInputTypeMismatch
			},
			target: execution.ErrInternal,
		},
		{
			name: "output type drift",
			configure: func(authorizer *authzmocks.MockAuthorizer) {
				authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(nil).Once()
			},
			invoke: func(context.Context, authz.Subject, any) (any, error) {
				return struct {
					// Name is an intentionally incompatible output field.
					Name string
				}{Name: "alpha"}, nil
			},
			target: execution.ErrCapabilityFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := authzmocks.NewMockAuthorizer(t)
			tt.configure(authorizer)
			subject := newDispatchSubject(t, authorizer, tt.invoke)

			_, err := subject.dispatch.dispatch(
				t.Context(),
				authz.Subject{ID: "subject-1"},
				"cap.lookup",
				map[string]any{"value": "alpha"},
			)

			require.ErrorIs(t, err, tt.target)
		})
	}
}

// TestDispatchReturnsFreshCanonicalMaps proves decoded child maps never become authorization input.
func TestDispatchReturnsFreshCanonicalMaps(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	decoded := map[string]any{"value": "alpha"}
	var authorized map[string]any
	authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Run(
		func(_ context.Context, input authz.AuthorizationInput) {
			authorized = input.Arguments
			input.Arguments["value"] = "mutated-policy"
			decoded["value"] = "mutated-decoded"
		},
	).Return(nil).Once()
	var received dispatchInput
	subject := newDispatchSubject(t, authorizer, func(_ context.Context, _ authz.Subject, input any) (any, error) {
		typed, ok := input.(dispatchInput)
		if !ok {
			return nil, catalog.ErrInputTypeMismatch
		}
		received = typed
		return dispatchOutput(typed), nil
	})

	result, err := subject.dispatch.dispatch(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		"cap.lookup",
		decoded,
	)

	require.NoError(t, err)
	require.NotNil(t, authorized)
	assert.Equal(t, "mutated-policy", authorized["value"])
	assert.Equal(t, "mutated-decoded", decoded["value"])
	assert.Equal(t, "alpha", received.Value)
	assert.Equal(t, map[string]any{"value": "alpha"}, result)
}

// TestDispatchCancellationAfterAllowPreventsInvoke proves a stale allow cannot reach the handler.
func TestDispatchCancellationAfterAllowPreventsInvoke(t *testing.T) {
	tests := []struct {
		// name identifies the cancellation source.
		name string

		// newContext creates the dispatch context, cleanup, and cancellation trigger.
		newContext func(*testing.T) (context.Context, context.CancelFunc, func())

		// targets are the required dispatcher error classifications.
		targets []error
	}{
		{
			name: "request cancellation",
			newContext: func(t *testing.T) (context.Context, context.CancelFunc, func()) {
				ctx, cancel := context.WithCancel(t.Context())
				return ctx, cancel, cancel
			},
			targets: []error{context.Canceled},
		},
		{
			name: "elapsed deadline",
			newContext: func(t *testing.T) (context.Context, context.CancelFunc, func()) {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
				return ctx, cancel, func() {
					<-ctx.Done()
				}
			},
			targets: []error{execution.ErrResourceLimit, context.DeadlineExceeded},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel, trigger := tt.newContext(t)
			defer cancel()
			authorizationStarted := make(chan struct{})
			releaseAuthorization := make(chan struct{})
			authorizer := authzmocks.NewMockAuthorizer(t)
			authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Run(
				func(context.Context, authz.AuthorizationInput) {
					close(authorizationStarted)
					<-releaseAuthorization
				},
			).Return(nil).Once()
			var handlerCalls atomic.Int64
			subject := newDispatchSubject(t, authorizer, func(context.Context, authz.Subject, any) (any, error) {
				handlerCalls.Add(1)
				return dispatchOutput{}, nil
			})
			result := make(chan dispatchOutcome, 1)
			go func() {
				value, err := subject.dispatch.dispatch(
					ctx,
					authz.Subject{ID: "subject-1"},
					"cap.lookup",
					map[string]any{"value": "alpha"},
				)
				result <- dispatchOutcome{value: value, err: err}
			}()

			<-authorizationStarted
			trigger()
			close(releaseAuthorization)
			outcome := <-result

			assert.Nil(t, outcome.value)
			for _, target := range tt.targets {
				require.ErrorIs(t, outcome.err, target)
			}
			assert.Zero(t, handlerCalls.Load())
		})
	}
}

// TestDispatchClassifiesParentOutputLimits proves converted handler values are bounded before transport.
func TestDispatchClassifiesParentOutputLimits(t *testing.T) {
	tests := []struct {
		// name identifies the exhausted parent output budget.
		name string

		// configure makes one output budget reject the representative result.
		configure func(*dispatcher)
	}{
		{
			name: "depth",
			configure: func(dispatch *dispatcher) {
				dispatch.maxValueDepth = 1
			},
		},
		{
			name: "materialization",
			configure: func(dispatch *dispatcher) {
				dispatch.maxValueBytes = 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := authzmocks.NewMockAuthorizer(t)
			authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(nil).Once()
			subject := newDispatchSubject(
				t,
				authorizer,
				func(context.Context, authz.Subject, any) (any, error) {
					return dispatchOutput{Value: "alpha"}, nil
				},
			)
			tt.configure(subject.dispatch)

			_, err := subject.dispatch.dispatch(
				t.Context(),
				authz.Subject{ID: "subject-1"},
				"cap.lookup",
				map[string]any{"value": "alpha"},
			)

			require.ErrorIs(t, err, execution.ErrResourceLimit)
		})
	}
}

// TestDispatchCancellationDuringHandlerReturnsPromptly proves late trusted Go work cannot hold dispatch.
func TestDispatchCancellationDuringHandlerReturnsPromptly(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	authorizer.EXPECT().Authorize(mock.Anything, mock.Anything).Return(nil).Once()
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	subject := newDispatchSubject(
		t,
		authorizer,
		func(context.Context, authz.Subject, any) (any, error) {
			close(handlerStarted)
			<-releaseHandler
			return dispatchOutput{Value: "late"}, nil
		},
	)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan dispatchOutcome, 1)
	go func() {
		value, err := subject.dispatch.dispatch(
			ctx,
			authz.Subject{ID: "subject-1"},
			"cap.lookup",
			map[string]any{"value": "alpha"},
		)
		result <- dispatchOutcome{value: value, err: err}
	}()

	<-handlerStarted
	cancel()
	select {
	case outcome := <-result:
		assert.Nil(t, outcome.value)
		require.ErrorIs(t, outcome.err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("dispatch waited for a canceled handler")
	}
	close(releaseHandler)
}

// dispatchSubject carries one catalog-backed dispatcher under test.
type dispatchSubject struct {
	// dispatch is the unexported native-call owner.
	dispatch *dispatcher
}

// newDispatchSubject builds one catalog and dispatcher around a function-fake handler.
func newDispatchSubject(t *testing.T, authorizer authz.Authorizer, invoke catalog.Invoker) *dispatchSubject {
	t.Helper()
	plan, err := binding.CompileFor[dispatchInput, dispatchOutput]()
	require.NoError(t, err)
	capabilityCatalog, err := catalog.Build([]catalog.Registration{{
		ID:          "cap.lookup",
		Name:        "records.lookup",
		Summary:     "Return one record.",
		Description: "Returns the supplied record value.",
		Plan:        plan,
		Invoke:      invoke,
	}}, catalog.Options{
		MaxSearchQueryBytes: 256,
		MaxSearchResults:    20,
	})
	require.NoError(t, err)
	return &dispatchSubject{
		dispatch: newDispatcher(capabilityCatalog, authorizer, 16, 64*1024),
	}
}
