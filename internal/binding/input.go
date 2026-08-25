package binding

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

const keywordTupleSize = 2

// BindAs binds keyword arguments to the plan's exact generic input type and canonical arguments.
func BindAs[Input any](plan *Plan, args starlark.Tuple, kwargs []starlark.Tuple) (Input, map[string]any, error) {
	var zero Input
	if plan == nil || plan.inputType != reflect.TypeFor[Input]() {
		return zero, nil, fmt.Errorf("%w: requested input type does not match the compiled plan", ErrInvalidPlan)
	}
	bound, canonical, err := plan.Bind(args, kwargs)
	if err != nil {
		return zero, nil, err
	}
	typed, ok := bound.(Input)
	if !ok {
		return zero, nil, fmt.Errorf("%w: bound input does not match its compiled type", ErrInvalidPlan)
	}
	return typed, canonical, nil
}

// Bind validates keyword-only Starlark arguments and constructs an exact Go input plus fresh canonical map.
func (plan *Plan) Bind(args starlark.Tuple, kwargs []starlark.Tuple) (any, map[string]any, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}
	if len(args) != 0 {
		return nil, nil, fmt.Errorf("%w: positional arguments are not accepted", ErrInvalidArguments)
	}

	input := reflect.New(plan.inputType).Elem()
	canonical := make(map[string]any, len(kwargs))
	seen := make(map[string]struct{}, len(kwargs))
	for _, keyword := range kwargs {
		name, err := plan.bindKeyword(input, canonical, keyword)
		if err != nil {
			return nil, nil, err
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate argument %q", ErrInvalidArguments, name)
		}
		seen[name] = struct{}{}
	}
	if err := plan.requireArguments(seen); err != nil {
		return nil, nil, err
	}
	return input.Interface(), canonical, nil
}

// BindShape binds keyword-only Starlark arguments against a validated input shape.
//
// It returns only a fresh canonical JSON-shaped map. Explicit Starlark None and
// omission both leave an optional key absent. Callers must not treat the result
// as the authoritative authorization map; parent re-binding constructs that.
func BindShape(fields []FieldShape, args starlark.Tuple, kwargs []starlark.Tuple) (map[string]any, error) {
	if err := ValidateInputShape(fields); err != nil {
		return nil, err
	}
	if len(args) != 0 {
		return nil, fmt.Errorf("%w: positional arguments are not accepted", ErrInvalidArguments)
	}

	byName := make(map[string]FieldShape, len(fields))
	for _, field := range fields {
		byName[field.Name] = field
	}
	canonical := make(map[string]any, len(kwargs))
	seen := make(map[string]struct{}, len(kwargs))
	for _, keyword := range kwargs {
		name, value, err := keywordArgument(keyword)
		if err != nil {
			return nil, err
		}
		field, known := byName[name]
		if !known {
			return nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate argument %q", ErrInvalidArguments, name)
		}
		seen[name] = struct{}{}
		converted, omitted, err := convertShapeArgument(field, name, value)
		if err != nil {
			return nil, err
		}
		if !omitted {
			canonical[name] = converted
		}
	}
	for _, field := range fields {
		if !field.Required {
			continue
		}
		if _, ok := seen[field.Name]; !ok {
			return nil, fmt.Errorf("%w: missing required argument %q", ErrInvalidArguments, field.Name)
		}
	}
	return canonical, nil
}

// BindValue reconstructs the exact registered Go input and a fresh canonical map.
//
// The decoded child map is never returned or retained. A present optional key
// with a nil value is treated as explicit None and omitted from the canonical
// map. Invalid normalized maps are classified with ErrInvalidArguments so
// Increment 2 can translate every BindValue failure to an internal/protocol
// error rather than a caller argument error.
func (plan *Plan) BindValue(arguments map[string]any) (any, map[string]any, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}

	input := reflect.New(plan.inputType).Elem()
	canonical := make(map[string]any, len(plan.inputFields))
	for _, field := range plan.inputFields {
		value, present := arguments[field.name]
		switch field.kind {
		case fieldString:
			if !present {
				return nil, nil, fmt.Errorf("%w: missing required argument %q", ErrInvalidArguments, field.name)
			}
			converted, ok := value.(string)
			if !ok {
				return nil, nil, fmt.Errorf("%w: argument %q must be a string", ErrInvalidArguments, field.name)
			}
			input.Field(field.index).SetString(converted)
			canonical[field.name] = converted
		case fieldOptionalInt64:
			if !present || value == nil {
				continue
			}
			converted, ok := value.(int64)
			if !ok {
				return nil, nil, fmt.Errorf(
					"%w: argument %q must be an integer or None",
					ErrInvalidArguments,
					field.name,
				)
			}
			pointer := reflect.New(input.Field(field.index).Type().Elem())
			pointer.Elem().SetInt(converted)
			input.Field(field.index).Set(pointer)
			canonical[field.name] = converted
		case fieldInt64, fieldBool, fieldFloat64:
			return nil, nil, fmt.Errorf("%w: argument %q has an invalid compiled kind", ErrInvalidPlan, field.name)
		default:
			return nil, nil, fmt.Errorf("%w: argument %q has an unknown compiled kind", ErrInvalidPlan, field.name)
		}
	}
	for name := range arguments {
		if _, known := plan.inputByName[name]; !known {
			return nil, nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, name)
		}
	}
	return input.Interface(), canonical, nil
}

// bindKeyword validates and converts one keyword into both typed and canonical representations.
func (plan *Plan) bindKeyword(input reflect.Value, canonical map[string]any, keyword starlark.Tuple) (string, error) {
	name, value, err := keywordArgument(keyword)
	if err != nil {
		return "", err
	}
	fieldIndex, known := plan.inputByName[name]
	if !known {
		return "", fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, name)
	}

	field := plan.inputFields[fieldIndex]
	switch field.kind {
	case fieldString:
		converted, ok := starlark.AsString(value)
		if !ok {
			return "", fmt.Errorf("%w: argument %q must be a string", ErrInvalidArguments, name)
		}
		input.Field(field.index).SetString(converted)
		canonical[name] = converted
	case fieldOptionalInt64:
		if value == starlark.None {
			return name, nil
		}
		integer, ok := value.(starlark.Int)
		if !ok {
			return "", fmt.Errorf("%w: argument %q must be an integer or None", ErrInvalidArguments, name)
		}
		converted, ok := integer.Int64()
		if !ok {
			return "", fmt.Errorf("%w: argument %q overflows int64", ErrInvalidArguments, name)
		}
		pointer := reflect.New(input.Field(field.index).Type().Elem())
		pointer.Elem().SetInt(converted)
		input.Field(field.index).Set(pointer)
		canonical[name] = converted
	case fieldInt64, fieldBool, fieldFloat64:
		return "", fmt.Errorf("%w: argument %q has an invalid compiled kind", ErrInvalidPlan, name)
	}
	return name, nil
}

// requireArguments verifies that every required compiled field was present.
func (plan *Plan) requireArguments(seen map[string]struct{}) error {
	for _, field := range plan.inputFields {
		if !field.required {
			continue
		}
		if _, ok := seen[field.name]; !ok {
			return fmt.Errorf("%w: missing required argument %q", ErrInvalidArguments, field.name)
		}
	}
	return nil
}

// keywordArgument extracts the string name and value from one keyword tuple.
func keywordArgument(keyword starlark.Tuple) (string, starlark.Value, error) {
	if len(keyword) != keywordTupleSize {
		return "", nil, fmt.Errorf("%w: malformed keyword argument", ErrInvalidArguments)
	}
	name, ok := starlark.AsString(keyword[0])
	if !ok {
		return "", nil, fmt.Errorf("%w: keyword name must be a string", ErrInvalidArguments)
	}
	return name, keyword[1], nil
}

// convertShapeArgument converts one Starlark argument against an exact field shape.
func convertShapeArgument(field FieldShape, name string, value starlark.Value) (any, bool, error) {
	switch {
	case isRequiredStringShape(field):
		converted, ok := starlark.AsString(value)
		if !ok {
			return nil, false, fmt.Errorf("%w: argument %q must be a string", ErrInvalidArguments, name)
		}
		return converted, false, nil
	case isOptionalIntegerShape(field):
		if value == starlark.None {
			return nil, true, nil
		}
		integer, ok := value.(starlark.Int)
		if !ok {
			return nil, false, fmt.Errorf("%w: argument %q must be an integer or None", ErrInvalidArguments, name)
		}
		converted, ok := integer.Int64()
		if !ok {
			return nil, false, fmt.Errorf("%w: argument %q overflows int64", ErrInvalidArguments, name)
		}
		return converted, false, nil
	default:
		return nil, false, fmt.Errorf("%w: argument %q has an unsupported shape", ErrInvalidPlan, name)
	}
}
