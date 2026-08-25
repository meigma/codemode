package codemode

import (
	"fmt"
	"time"
)

const (
	defaultMaxSourceBytes          = 64 * 1024
	defaultMaxExecutionSteps       = 1_000_000
	defaultMaxExecutionTime        = 5 * time.Second
	defaultMaxNativeCalls          = 100
	defaultMaxValueDepth           = 32
	defaultMaxValueBytes           = 1024 * 1024
	defaultMaxSearchQueryBytes     = 256
	defaultMaxSearchResults        = 20
	defaultMaxConcurrentExecutions = 8
)

// Limits bounds one execution and the model-facing catalog search surface.
type Limits struct {
	// MaxSourceBytes is the maximum accepted Starlark source size in bytes.
	MaxSourceBytes int

	// MaxExecutionSteps is the maximum number of Starlark bytecode steps.
	MaxExecutionSteps uint64

	// MaxExecutionTime is the maximum elapsed execution budget. The budget starts
	// before waiting for a worker slot and covers spawn, protocol exchange,
	// Starlark execution, and parent dispatch. Killing and reaping can add
	// operating-system overhead.
	MaxExecutionTime time.Duration

	// MaxNativeCalls is the maximum number of attempted native capability calls.
	MaxNativeCalls uint64

	// MaxValueDepth is the maximum nesting depth of any JSON-shaped value crossing
	// the worker boundary, including arguments, native results, and the final value.
	MaxValueDepth int

	// MaxValueBytes is the maximum encoded size of any JSON-shaped value crossing
	// the worker boundary, including arguments, native results, and the final value.
	// Size is measured by CodeMode's type-preserving JSON value encoder.
	MaxValueBytes int

	// MaxSearchQueryBytes is the maximum capability-search query size in bytes.
	MaxSearchQueryBytes int

	// MaxSearchResults is the maximum number of capability-search results.
	MaxSearchResults int

	// MaxConcurrentExecutions is the maximum number of concurrent spawn attempts
	// and live execution-worker children. Waiting for a slot consumes
	// MaxExecutionTime and remains cancelable through the request context.
	MaxConcurrentExecutions int
}

// DefaultLimits returns positive development defaults for every supported budget.
func DefaultLimits() Limits {
	return Limits{
		MaxSourceBytes:          defaultMaxSourceBytes,
		MaxExecutionSteps:       defaultMaxExecutionSteps,
		MaxExecutionTime:        defaultMaxExecutionTime,
		MaxNativeCalls:          defaultMaxNativeCalls,
		MaxValueDepth:           defaultMaxValueDepth,
		MaxValueBytes:           defaultMaxValueBytes,
		MaxSearchQueryBytes:     defaultMaxSearchQueryBytes,
		MaxSearchResults:        defaultMaxSearchResults,
		MaxConcurrentExecutions: defaultMaxConcurrentExecutions,
	}
}

// Validate rejects zero and negative limits; zero never means unlimited.
func (limits Limits) Validate() error {
	switch {
	case limits.MaxSourceBytes <= 0:
		return fmt.Errorf("%w: MaxSourceBytes must be positive", ErrInvalidRegistration)
	case limits.MaxExecutionSteps == 0:
		return fmt.Errorf("%w: MaxExecutionSteps must be positive", ErrInvalidRegistration)
	case limits.MaxExecutionTime <= 0:
		return fmt.Errorf("%w: MaxExecutionTime must be positive", ErrInvalidRegistration)
	case limits.MaxNativeCalls == 0:
		return fmt.Errorf("%w: MaxNativeCalls must be positive", ErrInvalidRegistration)
	case limits.MaxValueDepth <= 0:
		return fmt.Errorf("%w: MaxValueDepth must be positive", ErrInvalidRegistration)
	case limits.MaxValueBytes <= 0:
		return fmt.Errorf("%w: MaxValueBytes must be positive", ErrInvalidRegistration)
	case limits.MaxSearchQueryBytes <= 0:
		return fmt.Errorf("%w: MaxSearchQueryBytes must be positive", ErrInvalidRegistration)
	case limits.MaxSearchResults <= 0:
		return fmt.Errorf("%w: MaxSearchResults must be positive", ErrInvalidRegistration)
	case limits.MaxConcurrentExecutions <= 0:
		return fmt.Errorf("%w: MaxConcurrentExecutions must be positive", ErrInvalidRegistration)
	default:
		return nil
	}
}
