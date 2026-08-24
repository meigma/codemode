package execution_test

import (
	"context"
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
)

// testInput is the representative native capability input.
type testInput struct {
	// Value is one required capability argument.
	Value string `json:"value"`
}

// testOutput is the representative native capability output.
type testOutput struct {
	// Value is one capability result field.
	Value string `json:"value"`
}

// testExecutionResult carries one asynchronous execution outcome.
type testExecutionResult struct {
	// value is main's converted final value.
	value any

	// err is the classified execution failure.
	err error
}

// TestExecuteRequiresExactMainAndRejectsLoadingCalls proves entrypoint and phase validation precede side effects.
func TestExecuteRequiresExactMainAndRejectsLoadingCalls(t *testing.T) {
	var handlerCalls atomic.Int64
	capabilityCatalog := buildEngine(t, func(context.Context, authz.Subject, any) (any, error) {
		handlerCalls.Add(1)
		return testOutput{}, nil
	})
	tests := []struct {
		// name identifies the invalid program shape.
		name string

		// source is the submitted Starlark program.
		source string
	}{
		{name: "missing main", source: `value = 1`},
		{name: "main is not function", source: `main = 1`},
		{name: "main has argument", source: `def main(value): return value`},
		{name: "main has variadic arguments", source: `def main(*args): return args`},
		{name: "top-level native call", source: `
value = records.lookup(value="alpha")
def main():
    return value
`},
		{name: "module loading", source: `
load("module.star", "value")
def main():
    return value
`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := capabilityCatalog.Execute(
				t.Context(),
				authz.Subject{ID: "subject-1"},
				tt.source,
				authz.AllowAll(),
				defaultExecutionLimits(),
			)

			require.ErrorIs(t, err, execution.ErrInvalidProgram)
		})
	}
	assert.Zero(t, handlerCalls.Load())
}

// TestExecuteBindsAuthorizesThenDispatches proves canonical validation and policy ordering.
func TestExecuteBindsAuthorizesThenDispatches(t *testing.T) {
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
	capabilityCatalog := buildEngine(t, func(_ context.Context, _ authz.Subject, input any) (any, error) {
		events = append(events, "handler")
		typed, ok := input.(testInput)
		if !ok {
			return nil, catalog.ErrInputTypeMismatch
		}
		return testOutput(typed), nil
	})

	result, err := capabilityCatalog.Execute(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		`def main(): return records.lookup(value="alpha")`,
		authorizer,
		defaultExecutionLimits(),
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"authorize", "handler"}, events)
	assert.Equal(t, map[string]any{"value": "alpha"}, result)
}

// TestExecuteRejectsMalformedArgumentsBeforePolicy proves invalid calls reach neither policy nor handler.
func TestExecuteRejectsMalformedArgumentsBeforePolicy(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	var handlerCalls atomic.Int64
	capabilityCatalog := buildEngine(t, func(context.Context, authz.Subject, any) (any, error) {
		handlerCalls.Add(1)
		return testOutput{}, nil
	})

	_, err := capabilityCatalog.Execute(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		`def main(): return records.lookup()`,
		authorizer,
		defaultExecutionLimits(),
	)

	require.ErrorIs(t, err, execution.ErrInvalidArguments)
	assert.Zero(t, handlerCalls.Load())
}

// TestExecuteRejectsDuplicateKeywordSyntaxAsInvalidProgram proves repeated keywords fail at parse time before policy.
func TestExecuteRejectsDuplicateKeywordSyntaxAsInvalidProgram(t *testing.T) {
	authorizer := authzmocks.NewMockAuthorizer(t)
	var handlerCalls atomic.Int64
	capabilityCatalog := buildEngine(t, func(context.Context, authz.Subject, any) (any, error) {
		handlerCalls.Add(1)
		return testOutput{}, nil
	})

	_, err := capabilityCatalog.Execute(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		`def main(): return records.lookup(value="alpha", value="beta")`,
		authorizer,
		defaultExecutionLimits(),
	)

	require.ErrorIs(t, err, execution.ErrInvalidProgram)
	assert.Zero(t, handlerCalls.Load())
}

// TestExecuteCancellationAfterAuthorizationPreventsDispatch proves a stale allow cannot cross the handler boundary.
func TestExecuteCancellationAfterAuthorizationPreventsDispatch(t *testing.T) {
	tests := []struct {
		// name identifies the cancellation source.
		name string

		// newContext creates the execution context, cleanup, and cancellation trigger.
		newContext func(*testing.T) (context.Context, context.CancelFunc, func())

		// targets are the required execution error classifications.
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
				ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
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
			engine := buildEngine(t, func(context.Context, authz.Subject, any) (any, error) {
				handlerCalls.Add(1)
				return testOutput{}, nil
			})
			result := make(chan testExecutionResult, 1)
			go func() {
				value, err := engine.Execute(
					ctx,
					authz.Subject{ID: "subject-1"},
					`def main(): return records.lookup(value="alpha")`,
					authorizer,
					defaultExecutionLimits(),
				)
				result <- testExecutionResult{value: value, err: err}
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

// TestExecuteCancelsInFlightStarlark proves the watcher interrupts evaluation after execution begins.
func TestExecuteCancelsInFlightStarlark(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerReturned := make(chan struct{})
	engine := buildEngine(t, func(context.Context, authz.Subject, any) (any, error) {
		close(handlerStarted)
		<-releaseHandler
		close(handlerReturned)
		return testOutput{}, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	limits := defaultExecutionLimits()
	limits.MaxExecutionSteps = ^uint64(0)
	result := make(chan testExecutionResult, 1)
	go func() {
		value, err := engine.Execute(
			ctx,
			authz.Subject{ID: "subject-1"},
			`
def main():
    records.lookup(value="alpha")
    total = 0
    for item in range(1000000000):
        total += item
    return total
`,
			authz.AllowAll(),
			limits,
		)
		result <- testExecutionResult{value: value, err: err}
	}()

	<-handlerStarted
	close(releaseHandler)
	<-handlerReturned
	cancel()
	outcome := <-result

	assert.Nil(t, outcome.value)
	require.ErrorIs(t, outcome.err, context.Canceled)
}

// TestExecuteCreatesFreshStateAndReturnsOnlyMain proves executions share no globals or counters.
func TestExecuteCreatesFreshStateAndReturnsOnlyMain(t *testing.T) {
	capabilityCatalog := buildEngine(t, func(_ context.Context, _ authz.Subject, input any) (any, error) {
		typed, ok := input.(testInput)
		if !ok {
			return nil, catalog.ErrInputTypeMismatch
		}
		return testOutput(typed), nil
	})
	const source = `
hidden = "discarded"
print(hidden)
def main():
    return records.lookup(value="visible")
`
	limits := defaultExecutionLimits()
	limits.MaxNativeCalls = 1

	first, err := capabilityCatalog.Execute(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		source,
		authz.AllowAll(),
		limits,
	)
	require.NoError(t, err)
	second, err := capabilityCatalog.Execute(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		source,
		authz.AllowAll(),
		limits,
	)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "visible"}, first)
	assert.Equal(t, map[string]any{"value": "visible"}, second)
}

// TestExecuteEnforcesEveryRuntimeBudget proves source, steps, calls, time, depth, and bytes fail safely.
func TestExecuteEnforcesEveryRuntimeBudget(t *testing.T) {
	var handlerCalls atomic.Int64
	capabilityCatalog := buildEngine(t, func(_ context.Context, _ authz.Subject, input any) (any, error) {
		handlerCalls.Add(1)
		typed, ok := input.(testInput)
		if !ok {
			return nil, catalog.ErrInputTypeMismatch
		}
		return testOutput(typed), nil
	})

	t.Run("source bytes", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxSourceBytes = 8
		_, err := capabilityCatalog.Execute(
			t.Context(),
			authz.Subject{ID: "subject-1"},
			`def main(): return None`,
			authz.AllowAll(),
			limits,
		)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("execution steps", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxExecutionSteps = 20
		_, err := capabilityCatalog.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    total = 0
    for item in range(1000000):
        total += item
    return total
`, authz.AllowAll(), limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("native calls", func(t *testing.T) {
		handlerCalls.Store(0)
		limits := defaultExecutionLimits()
		limits.MaxNativeCalls = 1
		_, err := capabilityCatalog.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    records.lookup(value="first")
    return records.lookup(value="second")
`, authz.AllowAll(), limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
		assert.Equal(t, int64(1), handlerCalls.Load())
	})

	t.Run("elapsed time", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxExecutionTime = time.Nanosecond
		_, err := capabilityCatalog.Execute(
			t.Context(),
			authz.Subject{ID: "subject-1"},
			`def main(): return None`,
			authz.AllowAll(),
			limits,
		)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("value depth", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxValueDepth = 2
		_, err := capabilityCatalog.Execute(
			t.Context(),
			authz.Subject{ID: "subject-1"},
			`def main(): return [[["deep"]]]`,
			authz.AllowAll(),
			limits,
		)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("result bytes", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxResultBytes = 4
		_, err := capabilityCatalog.Execute(
			t.Context(),
			authz.Subject{ID: "subject-1"},
			`def main(): return "oversized"`,
			authz.AllowAll(),
			limits,
		)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})
}

// TestExecutePreservesCancellationAndRejectsUnsupportedFinalValues proves terminal failures remain classified.
func TestExecutePreservesCancellationAndRejectsUnsupportedFinalValues(t *testing.T) {
	capabilityCatalog := buildEngine(t, func(context.Context, authz.Subject, any) (any, error) {
		return testOutput{}, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := capabilityCatalog.Execute(
		ctx,
		authz.Subject{ID: "subject-1"},
		`def main(): return None`,
		authz.AllowAll(),
		defaultExecutionLimits(),
	)
	require.ErrorIs(t, err, context.Canceled)
	_, err = capabilityCatalog.Execute(
		t.Context(),
		authz.Subject{ID: "subject-1"},
		`def main(): return main`,
		authz.AllowAll(),
		defaultExecutionLimits(),
	)
	require.ErrorIs(t, err, execution.ErrInvalidProgram)
}

// buildEngine creates one immutable representative execution engine.
func buildEngine(t *testing.T, invoke catalog.Invoker) *execution.Engine {
	t.Helper()
	plan, err := binding.CompileFor[testInput, testOutput]()
	require.NoError(t, err)
	capabilityCatalog, err := catalog.Build([]catalog.Registration{
		{
			ID:          "cap.lookup",
			Name:        "records.lookup",
			Summary:     "Look up one record.",
			Description: "Returns one record by value.",
			Plan:        plan,
			Invoke:      invoke,
		},
	}, catalog.Options{MaxSearchQueryBytes: 1024, MaxSearchResults: 10})
	require.NoError(t, err)
	engine, err := execution.New(capabilityCatalog)
	require.NoError(t, err)
	return engine
}

// defaultExecutionLimits returns positive representative execution budgets.
func defaultExecutionLimits() execution.Limits {
	return execution.Limits{
		MaxSourceBytes:    64 * 1024,
		MaxExecutionSteps: 1_000_000,
		MaxExecutionTime:  5 * time.Second,
		MaxNativeCalls:    10,
		MaxValueDepth:     16,
		MaxResultBytes:    64 * 1024,
	}
}
