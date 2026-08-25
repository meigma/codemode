package execution

import (
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
		outcome.value, outcome.err = execute(source, engine.predeclared, nativeCall, limits)
	}()
	return outcome.value, outcome.err
}

// execute performs one execution after the panic-recovery boundary is installed.
func execute(
	source string,
	predeclared starlark.StringDict,
	nativeCall NativeCall,
	limits Limits,
) (any, error) {
	if predeclared == nil || nativeCall == nil {
		return nil, ErrInternal
	}
	if len(source) > limits.MaxSourceBytes {
		return nil, ErrResourceLimit
	}

	state := &executionState{
		phase: phaseLoading,
		nativeCalls: checkedCounter{
			maximum: limits.MaxNativeCalls,
		},
		call: wrapNativeCall(nativeCall, limits),
	}
	thread := newThread(state, limits.MaxExecutionSteps)

	globals, executionErr := starlark.ExecFileOptions(
		&syntax.FileOptions{},
		thread,
		"<codemode>",
		source,
		predeclared,
	)
	if executionErr != nil {
		return nil, classifyRuntimeError(state, executionErr)
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
		return nil, classifyRuntimeError(state, callErr)
	}

	converted, conversionErr := binding.FromStarlark(
		finalValue,
		limits.MaxValueDepth,
		limits.MaxValueBytes,
	)
	if conversionErr != nil {
		if errors.Is(conversionErr, binding.ErrValueLimit) {
			return nil, fmt.Errorf("%w: %w", ErrResourceLimit, conversionErr)
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidProgram, conversionErr)
	}
	return converted, nil
}

// wrapNativeCall captures conversion limits for one execution.
func wrapNativeCall(nativeCall NativeCall, limits Limits) nativeInvoker {
	return func(id string, arguments map[string]any) (starlark.Value, error) {
		result, err := nativeCall(id, arguments)
		if err != nil {
			return nil, err
		}
		converted, conversionErr := binding.ToStarlark(result, limits.MaxValueDepth, limits.MaxValueBytes)
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

// classifyRuntimeError prefers deterministic budget state before classifying evaluator causes.
func classifyRuntimeError(state *executionState, err error) error {
	if state.stepLimited {
		return ErrResourceLimit
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
