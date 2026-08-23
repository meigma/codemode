package execution

import "time"

// Limits contains the prevalidated positive budgets for one execution.
type Limits struct {
	// MaxSourceBytes bounds the submitted Starlark source length.
	MaxSourceBytes int

	// MaxExecutionSteps bounds abstract Starlark interpreter work.
	MaxExecutionSteps uint64

	// MaxExecutionTime bounds elapsed execution time.
	MaxExecutionTime time.Duration

	// MaxNativeCalls bounds attempted capability invocations.
	MaxNativeCalls uint64

	// MaxValueDepth bounds recursive final-result conversion.
	MaxValueDepth int

	// MaxResultBytes bounds encoded final-result size.
	MaxResultBytes int
}
