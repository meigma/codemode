package execution

// Limits contains the prevalidated positive budgets for one execution.
type Limits struct {
	// MaxSourceBytes bounds the submitted Starlark source length.
	MaxSourceBytes int

	// MaxExecutionSteps bounds abstract Starlark interpreter work.
	MaxExecutionSteps uint64

	// MaxNativeCalls bounds attempted capability invocations.
	MaxNativeCalls uint64

	// MaxValueDepth bounds recursive crossing-value conversion.
	MaxValueDepth int

	// MaxValueBytes supplies the byte-derived materialization bound.
	MaxValueBytes int
}
