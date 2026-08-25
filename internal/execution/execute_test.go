package execution_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/execution"
)

// testExecutionResult carries one asynchronous execution outcome.
type testExecutionResult struct {
	// value is main's converted final value.
	value any

	// err is the classified execution failure.
	err error
}

// TestNewRejectsInvalidBindings proves Engine construction fails closed.
func TestNewRejectsInvalidBindings(t *testing.T) {
	valid := lookupBinding()
	tests := []struct {
		// name identifies the invalid binding set.
		name string

		// bindings are presented to New.
		bindings []execution.CapabilityBinding
	}{
		{name: "empty ID", bindings: []execution.CapabilityBinding{withID(valid, "")}},
		{name: "whitespace ID", bindings: []execution.CapabilityBinding{withID(valid, " cap.lookup")}},
		{name: "undotted name", bindings: []execution.CapabilityBinding{withName(valid, "records")}},
		{name: "invalid name", bindings: []execution.CapabilityBinding{withName(valid, "records.not-valid")}},
		{name: "reserved name segment", bindings: []execution.CapabilityBinding{withName(valid, "records.import")}},
		{name: "duplicate ID", bindings: []execution.CapabilityBinding{
			valid,
			withName(withID(valid, "cap.lookup"), "records.other"),
		}},
		{name: "duplicate name", bindings: []execution.CapabilityBinding{
			valid,
			withID(valid, "cap.other"),
		}},
		{name: "namespace collision", bindings: []execution.CapabilityBinding{
			valid,
			{
				ID:   "cap.detail",
				Name: "records.lookup.detail",
				Input: []binding.FieldShape{
					{Name: "value", Type: "str", Required: true},
				},
			},
		}},
		{name: "invalid input shape", bindings: []execution.CapabilityBinding{
			{
				ID:   "cap.lookup",
				Name: "records.lookup",
				Input: []binding.FieldShape{
					{Name: "value", Type: "unsupported", Required: true},
				},
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := execution.New(tt.bindings)

			require.ErrorIs(t, err, execution.ErrInternal)
		})
	}
}

// TestNewCopiesCallerBindings proves Engine construction does not retain caller slices.
func TestNewCopiesCallerBindings(t *testing.T) {
	capability := lookupBinding()
	engine, err := execution.New([]execution.CapabilityBinding{capability})
	require.NoError(t, err)
	capability.Input[0].Name = "mutated"

	result, err := engine.Execute(
		t.Context(),
		`def main(): return records.lookup(value="alpha")`,
		echoNativeCall(),
		defaultExecutionLimits(),
	)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "alpha"}, result)
}

// TestExecuteRequiresExactMainAndRejectsLoadingCalls proves entrypoint and phase validation precede side effects.
func TestExecuteRequiresExactMainAndRejectsLoadingCalls(t *testing.T) {
	engine := buildEngine(t)
	var nativeCalls atomic.Int64
	nativeCall := countingNativeCall(&nativeCalls)
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
			_, err := engine.Execute(t.Context(), tt.source, nativeCall, defaultExecutionLimits())

			require.ErrorIs(t, err, execution.ErrInvalidProgram)
		})
	}
	assert.Zero(t, nativeCalls.Load())
}

// TestExecuteBindsThenDispatches proves canonical validation precedes the native port.
func TestExecuteBindsThenDispatches(t *testing.T) {
	var gotID string
	var gotArguments map[string]any
	result, err := buildEngine(t).Execute(
		t.Context(),
		`def main(): return records.lookup(value="alpha")`,
		func(id string, arguments map[string]any) (any, error) {
			gotID = id
			gotArguments = arguments
			return map[string]any{"value": "alpha"}, nil
		},
		defaultExecutionLimits(),
	)

	require.NoError(t, err)
	assert.Equal(t, "cap.lookup", gotID)
	assert.Equal(t, map[string]any{"value": "alpha"}, gotArguments)
	assert.Equal(t, map[string]any{"value": "alpha"}, result)
}

// TestExecuteRejectsMalformedArgumentsBeforeNativeCall proves invalid calls never reach the native port.
func TestExecuteRejectsMalformedArgumentsBeforeNativeCall(t *testing.T) {
	var nativeCalls atomic.Int64
	_, err := buildEngine(t).Execute(
		t.Context(),
		`def main(): return records.lookup()`,
		countingNativeCall(&nativeCalls),
		defaultExecutionLimits(),
	)

	require.ErrorIs(t, err, execution.ErrInvalidArguments)
	assert.Zero(t, nativeCalls.Load())
}

// TestExecuteRejectsDuplicateKeywordSyntaxAsInvalidProgram proves repeated keywords fail at parse time.
func TestExecuteRejectsDuplicateKeywordSyntaxAsInvalidProgram(t *testing.T) {
	var nativeCalls atomic.Int64
	_, err := buildEngine(t).Execute(
		t.Context(),
		`def main(): return records.lookup(value="alpha", value="beta")`,
		countingNativeCall(&nativeCalls),
		defaultExecutionLimits(),
	)

	require.ErrorIs(t, err, execution.ErrInvalidProgram)
	assert.Zero(t, nativeCalls.Load())
}

// TestExecuteCancellationBetweenNativeCallsPreventsDispatch proves a live cancel cannot start a later native call.
func TestExecuteCancellationBetweenNativeCallsPreventsDispatch(t *testing.T) {
	engine := buildEngine(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var nativeCalls atomic.Int64

	value, err := engine.Execute(
		ctx,
		`
def main():
    records.lookup(value="first")
    return records.lookup(value="second")
`,
		func(string, map[string]any) (any, error) {
			nativeCalls.Add(1)
			cancel()
			return map[string]any{"value": "first"}, nil
		},
		defaultExecutionLimits(),
	)

	assert.Nil(t, value)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int64(1), nativeCalls.Load())
}

// TestExecuteCancelsInFlightStarlark proves the watcher interrupts evaluation after execution begins.
func TestExecuteCancelsInFlightStarlark(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerReturned := make(chan struct{})
	engine := buildEngine(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	limits := defaultExecutionLimits()
	limits.MaxExecutionSteps = ^uint64(0)
	result := make(chan testExecutionResult, 1)
	go func() {
		value, err := engine.Execute(
			ctx,
			`
def main():
    records.lookup(value="alpha")
    total = 0
    for item in range(1000000000):
        total += item
    return total
`,
			func(string, map[string]any) (any, error) {
				close(handlerStarted)
				<-releaseHandler
				close(handlerReturned)
				return map[string]any{"value": "alpha"}, nil
			},
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
	engine := buildEngine(t)
	const source = `
hidden = "discarded"
print(hidden)
def main():
    return records.lookup(value="visible")
`
	limits := defaultExecutionLimits()
	limits.MaxNativeCalls = 1

	first, err := engine.Execute(t.Context(), source, echoNativeCall(), limits)
	require.NoError(t, err)
	second, err := engine.Execute(t.Context(), source, echoNativeCall(), limits)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "visible"}, first)
	assert.Equal(t, map[string]any{"value": "visible"}, second)
}

// TestExecuteEnforcesEveryRuntimeBudget proves source, steps, calls, time, depth, and bytes fail safely.
func TestExecuteEnforcesEveryRuntimeBudget(t *testing.T) {
	engine := buildEngine(t)
	var nativeCalls atomic.Int64
	countingCall := countingNativeCall(&nativeCalls)

	t.Run("source bytes", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxSourceBytes = 8
		_, err := engine.Execute(t.Context(), `def main(): return None`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("execution steps", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxExecutionSteps = 20
		_, err := engine.Execute(t.Context(), `
def main():
    total = 0
    for item in range(1000000):
        total += item
    return total
`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("native calls", func(t *testing.T) {
		nativeCalls.Store(0)
		limits := defaultExecutionLimits()
		limits.MaxNativeCalls = 1
		_, err := engine.Execute(t.Context(), `
def main():
    records.lookup(value="first")
    return records.lookup(value="second")
`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
		assert.Equal(t, int64(1), nativeCalls.Load())
	})

	t.Run("elapsed time", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxExecutionTime = time.Nanosecond
		_, err := engine.Execute(t.Context(), `def main(): return None`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("value depth", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxValueDepth = 2
		_, err := engine.Execute(t.Context(), `def main(): return [[["deep"]]]`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("result bytes", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxResultBytes = 4
		_, err := engine.Execute(t.Context(), `def main(): return "oversized"`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("native result depth", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxValueDepth = 2
		_, err := engine.Execute(
			t.Context(),
			`def main(): return records.lookup(value="alpha")`,
			func(string, map[string]any) (any, error) {
				return []any{[]any{[]any{"deep"}}}, nil
			},
			limits,
		)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})
}

// TestExecutePreservesCancellationAndRejectsUnsupportedFinalValues proves terminal failures remain classified.
func TestExecutePreservesCancellationAndRejectsUnsupportedFinalValues(t *testing.T) {
	engine := buildEngine(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := engine.Execute(ctx, `def main(): return None`, echoNativeCall(), defaultExecutionLimits())
	require.ErrorIs(t, err, context.Canceled)
	_, err = engine.Execute(
		t.Context(),
		`def main(): return main`,
		echoNativeCall(),
		defaultExecutionLimits(),
	)
	require.ErrorIs(t, err, execution.ErrInvalidProgram)
}

// TestExecuteClassifiesNativePortErrors proves request-specific failures keep their public classes.
func TestExecuteClassifiesNativePortErrors(t *testing.T) {
	tests := []struct {
		// name identifies the classified native failure.
		name string

		// result is the successful native value when err is nil.
		result any

		// err is returned by the native port.
		err error

		// target is the expected classified execution error.
		target error
	}{
		{name: "permission denied", err: execution.ErrPermissionDenied, target: execution.ErrPermissionDenied},
		{name: "policy failure", err: execution.ErrPolicyFailure, target: execution.ErrPolicyFailure},
		{name: "capability failure", err: execution.ErrCapabilityFailure, target: execution.ErrCapabilityFailure},
		{name: "internal failure", err: execution.ErrInternal, target: execution.ErrInternal},
		{name: "unsupported result", result: struct{ Value string }{Value: "alpha"}, target: execution.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildEngine(t).Execute(
				t.Context(),
				`def main(): return records.lookup(value="alpha")`,
				func(string, map[string]any) (any, error) {
					return tt.result, tt.err
				},
				defaultExecutionLimits(),
			)

			require.ErrorIs(t, err, tt.target)
		})
	}
}

// TestExecuteRecoversNativeCallPanic proves a request-port panic stays inside the interpreter boundary.
func TestExecuteRecoversNativeCallPanic(t *testing.T) {
	_, err := buildEngine(t).Execute(
		t.Context(),
		`def main(): return records.lookup(value="alpha")`,
		func(string, map[string]any) (any, error) {
			panic("native boom")
		},
		defaultExecutionLimits(),
	)

	require.ErrorIs(t, err, execution.ErrInternal)
}

// TestExecutePreservesSortedNativeDictionaryKeys proves ToStarlark insertion order is observable.
func TestExecutePreservesSortedNativeDictionaryKeys(t *testing.T) {
	result, err := buildEngine(t).Execute(
		t.Context(),
		`
def main():
    result = records.lookup(value="alpha")
    keys = []
    for key in result:
        keys.append(key)
    return keys
`,
		func(string, map[string]any) (any, error) {
			return map[string]any{
				"zeta":  int64(1),
				"alpha": int64(2),
				"mu":    int64(3),
			}, nil
		},
		defaultExecutionLimits(),
	)

	require.NoError(t, err)
	assert.Equal(t, []any{"alpha", "mu", "zeta"}, result)
}

// buildEngine creates one immutable representative execution engine.
func buildEngine(t *testing.T) *execution.Engine {
	t.Helper()
	engine, err := execution.New([]execution.CapabilityBinding{lookupBinding()})
	require.NoError(t, err)
	return engine
}

// lookupBinding returns the representative records.lookup capability.
func lookupBinding() execution.CapabilityBinding {
	return execution.CapabilityBinding{
		ID:   "cap.lookup",
		Name: "records.lookup",
		Input: []binding.FieldShape{
			{Name: "value", Type: "str", Required: true},
		},
	}
}

// withID returns a copy of capability with a replacement ID.
func withID(capability execution.CapabilityBinding, id string) execution.CapabilityBinding {
	capability.ID = id
	return capability
}

// withName returns a copy of capability with a replacement dotted name.
func withName(capability execution.CapabilityBinding, name string) execution.CapabilityBinding {
	capability.Name = name
	return capability
}

// echoNativeCall returns the bound value field as a normalized object.
func echoNativeCall() execution.NativeCall {
	return func(_ string, arguments map[string]any) (any, error) {
		return map[string]any{"value": arguments["value"]}, nil
	}
}

// countingNativeCall increments counter then echoes the bound value field.
func countingNativeCall(counter *atomic.Int64) execution.NativeCall {
	echo := echoNativeCall()
	return func(id string, arguments map[string]any) (any, error) {
		counter.Add(1)
		return echo(id, arguments)
	}
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
