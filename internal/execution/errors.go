package execution

import "errors"

var (
	// ErrInvalidProgram classifies invalid source, entrypoint behavior, or loading-phase native calls.
	ErrInvalidProgram = errors.New("invalid program")

	// ErrInvalidArguments classifies native arguments rejected before the native port.
	ErrInvalidArguments = errors.New("invalid capability arguments")

	// ErrPermissionDenied classifies a recognized authorization denial.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrPolicyFailure classifies an authorization evaluation error or panic.
	ErrPolicyFailure = errors.New("authorization policy failure")

	// ErrResourceLimit classifies source, step, native-call, depth, or value-size exhaustion.
	ErrResourceLimit = errors.New("resource limit exceeded")

	// ErrCapabilityFailure classifies a native handler or handler-result failure.
	ErrCapabilityFailure = errors.New("capability failed")

	// ErrInternal classifies an unexpected runtime or contract failure.
	ErrInternal = errors.New("internal failure")
)
