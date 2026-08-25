package execution

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/meigma/codemode/internal/binding"
)

const executionStateLocal = "codemode.execution.state"

// executionOutcome carries one recovered top-level result without named returns.
type executionOutcome struct {
	// value is main's converted final value.
	value any

	// err is the classified execution failure.
	err error
}

// Execute runs source in a fresh restricted interpreter and converts only main's final value.
func (engine *Engine) Execute(
	ctx context.Context,
	source string,
	nativeCall NativeCall,
	limits Limits,
) (any, error) {
	outcome := executionOutcome{}
	func() {
		defer func() {
			if recover() != nil {
				outcome.value = nil
				outcome.err = ErrInternal
			}
		}()
		outcome.value, outcome.err = execute(ctx, source, engine.predeclared, nativeCall, limits)
	}()
	return outcome.value, outcome.err
}

// execute performs one execution after the panic-recovery boundary is installed.
func execute(
	ctx context.Context,
	source string,
	predeclared starlark.StringDict,
	nativeCall NativeCall,
	limits Limits,
) (any, error) {
	if ctx == nil || predeclared == nil || nativeCall == nil {
		return nil, ErrInternal
	}
	if len(source) > limits.MaxSourceBytes {
		return nil, ErrResourceLimit
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return nil, contextErr
	}

	runCtx, cancel := context.WithTimeout(ctx, limits.MaxExecutionTime)
	defer cancel()
	if contextErr := contextFailure(runCtx); contextErr != nil {
		return nil, contextErr
	}

	state := &executionState{
		phase: phaseLoading,
		nativeCalls: checkedCounter{
			maximum: limits.MaxNativeCalls,
		},
		call: wrapNativeCall(runCtx, nativeCall, limits),
	}
	thread := newThread(state, limits.MaxExecutionSteps)
	stopCancellation := watchCancellation(runCtx, thread)
	defer stopCancellation()

	globals, executionErr := starlark.ExecFileOptions(
		&syntax.FileOptions{},
		thread,
		"<codemode>",
		source,
		predeclared,
	)
	if executionErr != nil {
		return nil, classifyRuntimeError(runCtx, state, executionErr)
	}
	mainFunction, mainErr := requireMain(globals)
	if mainErr != nil {
		return nil, mainErr
	}
	if phaseErr := state.beginMain(); phaseErr != nil {
		return nil, phaseErr
	}
	finalValue, callErr := starlark.Call(thread, mainFunction, nil, nil)
	state.finishMain()
	if callErr != nil {
		return nil, classifyRuntimeError(runCtx, state, callErr)
	}
	if contextErr := contextFailure(runCtx); contextErr != nil {
		return nil, contextErr
	}

	converted, conversionErr := binding.ConvertFinal(
		finalValue,
		limits.MaxValueDepth,
		limits.MaxResultBytes,
	)
	if conversionErr != nil {
		if errors.Is(conversionErr, binding.ErrValueLimit) {
			return nil, fmt.Errorf("%w: %w", ErrResourceLimit, conversionErr)
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidProgram, conversionErr)
	}
	if contextErr := contextFailure(runCtx); contextErr != nil {
		return nil, contextErr
	}
	return converted, nil
}

// wrapNativeCall captures request context and conversion limits for one execution.
func wrapNativeCall(ctx context.Context, nativeCall NativeCall, limits Limits) nativeInvoker {
	return func(id string, arguments map[string]any) (starlark.Value, error) {
		if contextErr := contextFailure(ctx); contextErr != nil {
			return nil, contextErr
		}
		result, err := nativeCall(id, arguments)
		if err != nil {
			return nil, err
		}
		converted, conversionErr := binding.ToStarlark(result, limits.MaxValueDepth, limits.MaxResultBytes)
		if conversionErr != nil {
			if errors.Is(conversionErr, binding.ErrValueLimit) {
				return nil, fmt.Errorf("%w: %w", ErrResourceLimit, conversionErr)
			}
			return nil, fmt.Errorf("%w: %w", ErrInternal, conversionErr)
		}
		return converted, nil
	}
}

// newThread constructs one restricted Starlark thread and stores its trusted execution state.
func newThread(state *executionState, maxSteps uint64) *starlark.Thread {
	thread := &starlark.Thread{
		Name: "codemode",
		Print: func(*starlark.Thread, string) {
			// Program output is intentionally discarded; only main's final value may escape.
		},
		Load: func(*starlark.Thread, string) (starlark.StringDict, error) {
			return nil, fmt.Errorf("%w: module loading is disabled", ErrInvalidProgram)
		},
	}
	thread.SetLocal(executionStateLocal, state)
	thread.OnMaxSteps = func(thread *starlark.Thread) {
		state.stepLimited = true
		thread.Cancel("execution step limit exceeded")
	}
	thread.SetMaxExecutionSteps(maxSteps)
	return thread
}

// watchCancellation interrupts Starlark evaluation when the request or elapsed deadline ends.
func watchCancellation(ctx context.Context, thread *starlark.Thread) func() {
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			thread.Cancel("execution canceled")
		case <-stopped:
		}
	}()
	return func() {
		close(stopped)
	}
}

// requireMain returns the exact zero-argument main function required by the execution contract.
func requireMain(globals starlark.StringDict) (*starlark.Function, error) {
	value, ok := globals["main"]
	if !ok {
		return nil, fmt.Errorf("%w: main is missing", ErrInvalidProgram)
	}
	mainFunction, ok := value.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("%w: main must be a function", ErrInvalidProgram)
	}
	if mainFunction.NumParams() != 0 || mainFunction.NumKwonlyParams() != 0 ||
		mainFunction.HasVarargs() || mainFunction.HasKwargs() {
		return nil, fmt.Errorf("%w: main must accept no arguments", ErrInvalidProgram)
	}
	return mainFunction, nil
}

// callCapability binds one native invocation and forwards a fresh canonical map.
func callCapability(
	thread *starlark.Thread,
	id string,
	input []binding.FieldShape,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	state, ok := thread.Local(executionStateLocal).(*executionState)
	if !ok || state == nil {
		return nil, ErrInternal
	}
	if state.phase != phaseRunning {
		return nil, ErrInvalidProgram
	}
	if counterErr := state.nativeCalls.increment(); counterErr != nil {
		return nil, counterErr
	}
	canonical, bindingErr := binding.BindShape(input, args, kwargs)
	if bindingErr != nil {
		if errors.Is(bindingErr, binding.ErrInvalidArguments) {
			return nil, fmt.Errorf("%w: %w", ErrInvalidArguments, bindingErr)
		}
		return nil, fmt.Errorf("%w: %w", ErrInternal, bindingErr)
	}
	return state.call(id, canonical)
}

// classifyRuntimeError prefers request and budget state before classifying evaluator causes.
func classifyRuntimeError(ctx context.Context, state *executionState, err error) error {
	if state.stepLimited {
		return ErrResourceLimit
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return contextErr
	}
	switch {
	case errors.Is(err, ErrInvalidArguments):
		return ErrInvalidArguments
	case errors.Is(err, ErrPermissionDenied):
		return ErrPermissionDenied
	case errors.Is(err, ErrPolicyFailure):
		return ErrPolicyFailure
	case errors.Is(err, ErrResourceLimit):
		return ErrResourceLimit
	case errors.Is(err, ErrCapabilityFailure):
		return ErrCapabilityFailure
	case errors.Is(err, ErrInternal):
		return ErrInternal
	case errors.Is(err, ErrInvalidProgram):
		return ErrInvalidProgram
	default:
		return fmt.Errorf("%w: %w", ErrInvalidProgram, err)
	}
}

// contextFailure projects request cancellation directly and deadlines as resource exhaustion.
func contextFailure(ctx context.Context) error {
	switch err := ctx.Err(); {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", ErrResourceLimit, context.DeadlineExceeded)
	default:
		return nil
	}
}
