package codemode

import "errors"

var (
	// ErrInvalidRegistration classifies invalid capability registration, limits, or server construction.
	ErrInvalidRegistration = errors.New("invalid registration")

	// ErrUnauthenticated classifies failure to resolve a trusted invocation subject.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrNotFound classifies an unavailable or disabled capability.
	ErrNotFound = errors.New("capability not found")

	// ErrInvalidProgram classifies invalid Starlark source or entrypoint behavior.
	ErrInvalidProgram = errors.New("invalid program")

	// ErrInvalidArguments classifies capability arguments rejected before authorization.
	ErrInvalidArguments = errors.New("invalid capability arguments")

	// ErrPermissionDenied classifies a recognized authorization denial.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrPolicyFailure classifies an authorization evaluation failure.
	ErrPolicyFailure = errors.New("authorization policy failure")

	// ErrResourceLimit classifies a configured execution or conversion limit.
	ErrResourceLimit = errors.New("resource limit exceeded")

	// ErrCapabilityFailure classifies a native capability handler failure.
	ErrCapabilityFailure = errors.New("capability failed")

	// ErrInternal classifies an unexpected framework failure.
	ErrInternal = errors.New("internal failure")
)
