package binding

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	// ErrInvalidPlan classifies an input or output type that cannot form a restricted binding plan.
	ErrInvalidPlan = errors.New("invalid binding plan")

	// ErrInvalidArguments classifies Starlark arguments rejected before authorization.
	ErrInvalidArguments = errors.New("invalid capability arguments")

	// ErrUnsupportedValue classifies a Go or Starlark value outside the supported conversion surface.
	ErrUnsupportedValue = errors.New("unsupported value")

	// ErrValueLimit classifies converted output that exceeds a configured depth or byte limit.
	ErrValueLimit = errors.New("converted value limit exceeded")
)

// Plan is an immutable compiled input, output, signature, and canonical-argument plan.
type Plan struct {
	// inputType is the exact non-pointer Go input struct type.
	inputType reflect.Type

	// outputType is the exact non-pointer Go output struct type.
	outputType reflect.Type

	// inputNodes is the compiled input type arena, index-aligned with inputSchema.Nodes.
	inputNodes []typeNode

	// inputSchema is the process-neutral projection of inputNodes shared with the worker.
	inputSchema InputSchema

	// outputRoot is the arena index of the compiled root struct.
	outputRoot int

	// outputNodes is the immutable compiled output type arena.
	outputNodes []typeNode
}

// CompileFor compiles the exact generic input and output types once.
func CompileFor[Input, Output any]() (*Plan, error) {
	return Compile(reflect.TypeFor[Input](), reflect.TypeFor[Output]())
}

// Compile creates an immutable restricted plan for exact input and output struct types.
func Compile(inputType reflect.Type, outputType reflect.Type) (*Plan, error) {
	if inputType == nil || inputType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: input must be a non-pointer struct", ErrInvalidPlan)
	}
	inputRoot, inputNodes, err := compileType(directionInput, inputType)
	if err != nil {
		return nil, err
	}
	if outputType == nil || outputType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: output must be a non-pointer struct", ErrInvalidPlan)
	}
	outputRoot, outputNodes, err := compileType(directionOutput, outputType)
	if err != nil {
		return nil, err
	}
	return &Plan{
		inputType:   inputType,
		outputType:  outputType,
		inputNodes:  inputNodes,
		inputSchema: projectInputSchema(inputRoot, inputNodes),
		outputRoot:  outputRoot,
		outputNodes: outputNodes,
	}, nil
}

// InputType returns the exact Go input type compiled into the plan.
func (plan *Plan) InputType() reflect.Type {
	return plan.inputType
}

// OutputType returns the exact Go output type compiled into the plan.
func (plan *Plan) OutputType() reflect.Type {
	return plan.outputType
}

// InputSchema returns a fresh process-neutral schema for worker-side binding.
func (plan *Plan) InputSchema() InputSchema {
	return plan.inputSchema.Clone()
}
