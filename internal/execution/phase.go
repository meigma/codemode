package execution

import (
	"fmt"

	"go.starlark.net/starlark"
)

// executionPhase identifies whether native capability calls are currently permitted.
type executionPhase uint8

const (
	// phaseLoading executes top-level source while rejecting every native capability call.
	phaseLoading executionPhase = iota

	// phaseRunning permits native capability calls from the validated main function.
	phaseRunning

	// phaseDone rejects calls after the one main invocation completes.
	phaseDone
)

// checkedCounter bounds one monotonically increasing per-execution quantity.
type checkedCounter struct {
	// current is the number of accepted increments.
	current uint64

	// maximum is the largest accepted count.
	maximum uint64
}

// increment advances the counter or returns ErrResourceLimit before exceeding its maximum.
func (counter *checkedCounter) increment() error {
	if counter.current >= counter.maximum {
		return ErrResourceLimit
	}
	counter.current++
	return nil
}

// nativeInvoker is a Starlark-ready native call that already converted a successful result.
type nativeInvoker func(id string, arguments map[string]any) (starlark.Value, error)

// executionState contains request-local trusted state unavailable to Starlark programs.
type executionState struct {
	// phase guards the side-effectful native-call boundary.
	phase executionPhase

	// nativeCalls bounds attempted capability invocations.
	nativeCalls checkedCounter

	// stepLimited records interpreter cancellation caused by the configured step budget.
	stepLimited bool

	// call forwards a bound canonical map through the request-specific native port.
	call nativeInvoker
}

// beginMain transitions exactly once from loading to running.
func (state *executionState) beginMain() error {
	if state.phase != phaseLoading {
		return fmt.Errorf("%w: invalid execution phase", ErrInternal)
	}
	state.phase = phaseRunning
	return nil
}

// finishMain closes native dispatch for this execution.
func (state *executionState) finishMain() {
	state.phase = phaseDone
}
