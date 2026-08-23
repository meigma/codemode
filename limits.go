package codemode

import (
	"fmt"
	"time"
)

const (
	defaultMaxSourceBytes      = 64 * 1024
	defaultMaxExecutionSteps   = 1_000_000
	defaultMaxExecutionTime    = 5 * time.Second
	defaultMaxNativeCalls      = 100
	defaultMaxValueDepth       = 32
	defaultMaxResultBytes      = 1024 * 1024
	defaultMaxSearchQueryBytes = 256
	defaultMaxSearchResults    = 20
)

// Limits bounds one execution and the model-facing catalog search surface.
type Limits struct {
	// MaxSourceBytes is the maximum accepted Starlark source size in bytes.
	MaxSourceBytes int

	// MaxExecutionSteps is the maximum number of Starlark bytecode steps.
	MaxExecutionSteps uint64

	// MaxExecutionTime is the maximum elapsed duration of one execution.
	MaxExecutionTime time.Duration

	// MaxNativeCalls is the maximum number of attempted native capability calls.
	MaxNativeCalls uint64

	// MaxValueDepth is the maximum converted-value nesting depth.
	MaxValueDepth int

	// MaxResultBytes is the maximum encoded final-result size in bytes.
	MaxResultBytes int

	// MaxSearchQueryBytes is the maximum capability-search query size in bytes.
	MaxSearchQueryBytes int

	// MaxSearchResults is the maximum number of capability-search results.
	MaxSearchResults int
}

// DefaultLimits returns positive development defaults for every supported budget.
func DefaultLimits() Limits {
	return Limits{
		MaxSourceBytes:      defaultMaxSourceBytes,
		MaxExecutionSteps:   defaultMaxExecutionSteps,
		MaxExecutionTime:    defaultMaxExecutionTime,
		MaxNativeCalls:      defaultMaxNativeCalls,
		MaxValueDepth:       defaultMaxValueDepth,
		MaxResultBytes:      defaultMaxResultBytes,
		MaxSearchQueryBytes: defaultMaxSearchQueryBytes,
		MaxSearchResults:    defaultMaxSearchResults,
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
	case limits.MaxResultBytes <= 0:
		return fmt.Errorf("%w: MaxResultBytes must be positive", ErrInvalidRegistration)
	case limits.MaxSearchQueryBytes <= 0:
		return fmt.Errorf("%w: MaxSearchQueryBytes must be positive", ErrInvalidRegistration)
	case limits.MaxSearchResults <= 0:
		return fmt.Errorf("%w: MaxSearchResults must be positive", ErrInvalidRegistration)
	default:
		return nil
	}
}
