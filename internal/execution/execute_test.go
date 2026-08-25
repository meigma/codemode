package execution_test

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/execution"
)

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
			_, err := engine.Execute(tt.source, nativeCall, defaultExecutionLimits())

			require.ErrorIs(t, err, execution.ErrInvalidProgram)
		})
	}
	assert.Zero(t, nativeCalls.Load())
}

// TestExecuteBindsThenDispatches proves canonical validation precedes the native port.
func TestExecuteBindsThenDispatches(t *testing.T) {
	tests := []struct {
		// name identifies the bound call.
		name string

		// engine constructs the execution engine under test.
		engine func(*testing.T) *execution.Engine

		// source is the Starlark program that performs one native call.
		source string

		// wantArguments is the canonical map forwarded to the native port.
		wantArguments map[string]any
	}{
		{
			name:   "required string",
			engine: buildEngine,
			source: `def main(): return records.lookup(value="alpha")`,
			wantArguments: map[string]any{
				"value": "alpha",
			},
		},
		{
			name:   "all eight scalar forms",
			engine: buildWidenedEngine,
			source: `
def main():
    return records.lookup(org="meigma", count=3, active=True, score=1.5, label="beta", limit=25, enabled=True, weight=2.5)
`,
			wantArguments: map[string]any{
				"org":     "meigma",
				"count":   int64(3),
				"active":  true,
				"score":   1.5,
				"label":   "beta",
				"limit":   int64(25),
				"enabled": true,
				"weight":  2.5,
			},
		},
		{
			name:   "omitted optional scalars",
			engine: buildWidenedEngine,
			source: `def main(): return records.lookup(org="meigma", count=3, active=False, score=0.0)`,
			wantArguments: map[string]any{
				"org":    "meigma",
				"count":  int64(3),
				"active": false,
				"score":  0.0,
			},
		},
		{
			name:   "explicit None optional scalars",
			engine: buildWidenedEngine,
			source: `def main(): return records.lookup(org="meigma", count=3, active=True, score=1.5, label=None, limit=None, enabled=None, weight=None)`,
			wantArguments: map[string]any{
				"org":    "meigma",
				"count":  int64(3),
				"active": true,
				"score":  1.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotID string
			var gotArguments map[string]any
			nativeCall := func(id string, arguments map[string]any) (any, error) {
				gotID = id
				gotArguments = arguments
				return map[string]any{"value": "alpha"}, nil
			}
			result, err := tt.engine(t).Execute(tt.source, nativeCall, defaultExecutionLimits())

			require.NoError(t, err)
			assert.Equal(t, "cap.lookup", gotID)
			assert.Equal(t, tt.wantArguments, gotArguments)
			assert.Equal(t, map[string]any{"value": "alpha"}, result)
		})
	}
}

// TestExecuteRejectsMalformedArgumentsBeforeNativeCall proves invalid calls never reach the native port.
func TestExecuteRejectsMalformedArgumentsBeforeNativeCall(t *testing.T) {
	tests := []struct {
		// name identifies the malformed call.
		name string

		// engine constructs the execution engine under test.
		engine func(*testing.T) *execution.Engine

		// source is the Starlark program that performs one native call.
		source string
	}{
		{
			name:   "missing required string",
			engine: buildEngine,
			source: `def main(): return records.lookup()`,
		},
		{
			name:   "missing required integer",
			engine: buildWidenedEngine,
			source: `def main(): return records.lookup(org="meigma", active=True, score=1.5)`,
		},
		{
			name:   "integer supplied as float",
			engine: buildWidenedEngine,
			source: `def main(): return records.lookup(org="meigma", count=3.0, active=True, score=1.5)`,
		},
		{
			name:   "overflowing integer",
			engine: buildWidenedEngine,
			source: `def main(): return records.lookup(org="meigma", count=9223372036854775808, active=True, score=1.5)`,
		},
		{
			name:   "NaN float",
			engine: buildWidenedEngine,
			source: `def main(): return records.lookup(org="meigma", count=3, active=True, score=float("nan"))`,
		},
		{
			name:   "infinity float",
			engine: buildWidenedEngine,
			source: `def main(): return records.lookup(org="meigma", count=3, active=True, score=float("inf"))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nativeCalls atomic.Int64
			_, err := tt.engine(t).Execute(tt.source, countingNativeCall(&nativeCalls), defaultExecutionLimits())

			require.ErrorIs(t, err, execution.ErrInvalidArguments)
			assert.Zero(t, nativeCalls.Load())
		})
	}
}

// TestExecuteRejectsDuplicateKeywordSyntaxAsInvalidProgram proves repeated keywords fail at parse time.
func TestExecuteRejectsDuplicateKeywordSyntaxAsInvalidProgram(t *testing.T) {
	var nativeCalls atomic.Int64
	_, err := buildEngine(
		t,
	).Execute(`def main(): return records.lookup(value="alpha", value="beta")`, countingNativeCall(&nativeCalls), defaultExecutionLimits())

	require.ErrorIs(t, err, execution.ErrInvalidProgram)
	assert.Zero(t, nativeCalls.Load())
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

	first, err := engine.Execute(source, echoNativeCall(), limits)
	require.NoError(t, err)
	second, err := engine.Execute(source, echoNativeCall(), limits)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "visible"}, first)
	assert.Equal(t, map[string]any{"value": "visible"}, second)
}

// TestExecuteEnforcesEveryRuntimeBudget proves source, steps, calls, depth, and materialization bounds fail safely.
func TestExecuteEnforcesEveryRuntimeBudget(t *testing.T) {
	engine := buildEngine(t)
	var nativeCalls atomic.Int64
	countingCall := countingNativeCall(&nativeCalls)

	t.Run("source bytes", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxSourceBytes = 8
		_, err := engine.Execute(`def main(): return None`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("execution steps", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxExecutionSteps = 20
		_, err := engine.Execute(`
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
		_, err := engine.Execute(`
def main():
    records.lookup(value="first")
    return records.lookup(value="second")
`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
		assert.Equal(t, int64(1), nativeCalls.Load())
	})

	t.Run("value depth", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxValueDepth = 2
		_, err := engine.Execute(`def main(): return [[["deep"]]]`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("value materialization", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxValueBytes = 4
		_, err := engine.Execute(`def main(): return [1, 2, 3, 4, 5]`, countingCall, limits)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})

	t.Run("native result depth", func(t *testing.T) {
		limits := defaultExecutionLimits()
		limits.MaxValueDepth = 2
		_, err := engine.Execute(
			`def main(): return records.lookup(value="alpha")`,
			func(string, map[string]any) (any, error) {
				return []any{[]any{[]any{"deep"}}}, nil
			},
			limits,
		)
		require.ErrorIs(t, err, execution.ErrResourceLimit)
	})
}

// TestExecuteRejectsUnsupportedFinalValues proves terminal values remain classified.
func TestExecuteRejectsUnsupportedFinalValues(t *testing.T) {
	engine := buildEngine(t)

	_, err := engine.Execute(`def main(): return main`, echoNativeCall(), defaultExecutionLimits())
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
			_, err := buildEngine(
				t,
			).Execute(`def main(): return records.lookup(value="alpha")`, func(string, map[string]any) (any, error) {
				return tt.result, tt.err
			}, defaultExecutionLimits())

			require.ErrorIs(t, err, tt.target)
		})
	}
}

// TestExecuteRecoversNativeCallPanic proves a request-port panic stays inside the interpreter boundary.
func TestExecuteRecoversNativeCallPanic(t *testing.T) {
	_, err := buildEngine(
		t,
	).Execute(`def main(): return records.lookup(value="alpha")`, func(string, map[string]any) (any, error) {
		panic("native boom")
	}, defaultExecutionLimits())

	require.ErrorIs(t, err, execution.ErrInternal)
}

// TestExecutePreservesSortedNativeDictionaryKeys proves ToStarlark insertion order is observable.
func TestExecutePreservesSortedNativeDictionaryKeys(t *testing.T) {
	result, err := buildEngine(t).Execute(`
def main():
    result = records.lookup(value="alpha")
    keys = []
    for key in result:
        keys.append(key)
    return keys
`, func(string, map[string]any) (any, error) {
		return map[string]any{
			"zeta":  int64(1),
			"alpha": int64(2),
			"mu":    int64(3),
		}, nil
	}, defaultExecutionLimits())

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

// buildWidenedEngine creates one engine exposing all eight scalar input forms.
func buildWidenedEngine(t *testing.T) *execution.Engine {
	t.Helper()
	engine, err := execution.New([]execution.CapabilityBinding{widenedLookupBinding()})
	require.NoError(t, err)
	return engine
}

// widenedLookupBinding returns records.lookup with every supported scalar input form.
func widenedLookupBinding() execution.CapabilityBinding {
	return execution.CapabilityBinding{
		ID:   "cap.lookup",
		Name: "records.lookup",
		Input: []binding.FieldShape{
			{Name: "org", Type: "str", Required: true},
			{Name: "count", Type: "int", Required: true},
			{Name: "active", Type: "bool", Required: true},
			{Name: "score", Type: "float", Required: true},
			{Name: "label", Type: "str | None", Required: false},
			{Name: "limit", Type: "int | None", Required: false},
			{Name: "enabled", Type: "bool | None", Required: false},
			{Name: "weight", Type: "float | None", Required: false},
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
		MaxNativeCalls:    10,
		MaxValueDepth:     16,
		MaxValueBytes:     64 * 1024,
	}
}
