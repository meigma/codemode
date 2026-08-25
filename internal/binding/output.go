package binding

import (
	"encoding/json"
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// ConvertOutput converts the plan's exact handler output to a process-neutral object.
func (plan *Plan) ConvertOutput(output any) (map[string]any, error) {
	if plan == nil {
		return nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}
	value := reflect.ValueOf(output)
	if !value.IsValid() || value.Type() != plan.outputType {
		return nil, fmt.Errorf("%w: handler output type does not match the compiled plan", ErrUnsupportedValue)
	}

	converted := make(map[string]any, len(plan.outputFields))
	for _, field := range plan.outputFields {
		item, err := convertOutputField(field, value.Field(field.index))
		if err != nil {
			return nil, err
		}
		converted[field.name] = item
	}
	return converted, nil
}

// ConvertFinal converts a final Starlark value to MCP-safe JSON data under depth and byte limits.
//
// Exact JSON byte measurement stays in this compatibility wrapper until the
// worker codec becomes the only encoded-size owner.
func ConvertFinal(value starlark.Value, maxDepth int, maxBytes int) (any, error) {
	converted, err := FromStarlark(value, maxDepth, maxBytes)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(converted)
	if err != nil {
		return nil, fmt.Errorf("%w: encode final value: %w", ErrUnsupportedValue, err)
	}
	if len(encoded) > maxBytes {
		return nil, fmt.Errorf("%w: final result is %d bytes; maximum is %d", ErrValueLimit, len(encoded), maxBytes)
	}
	return converted, nil
}

// convertOutputField converts one field according to its compiled output kind.
func convertOutputField(field outputField, value reflect.Value) (any, error) {
	switch field.kind {
	case fieldString:
		return value.String(), nil
	case fieldInt64:
		return value.Int(), nil
	case fieldBool:
		return value.Bool(), nil
	case fieldFloat64:
		float := value.Float()
		if !isFiniteFloat(float) {
			return nil, fmt.Errorf("%w: output field %q is not finite", ErrUnsupportedValue, field.name)
		}
		return float, nil
	case fieldOptionalInt64:
		return nil, fmt.Errorf("%w: output field %q has an invalid compiled kind", ErrInvalidPlan, field.name)
	}
	return nil, fmt.Errorf("%w: output field %q has an unknown compiled kind", ErrInvalidPlan, field.name)
}
