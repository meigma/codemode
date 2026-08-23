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

// bindKeyword validates and converts one keyword into both typed and canonical representations.
func (plan *Plan) bindKeyword(input reflect.Value, canonical map[string]any, keyword starlark.Tuple) (string, error) {
	if len(keyword) != keywordTupleSize {
		return "", fmt.Errorf("%w: malformed keyword argument", ErrInvalidArguments)
	}
	name, ok := starlark.AsString(keyword[0])
	if !ok {
		return "", fmt.Errorf("%w: keyword name must be a string", ErrInvalidArguments)
	}
	fieldIndex, known := plan.inputByName[name]
	if !known {
		return "", fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, name)
	}

	field := plan.inputFields[fieldIndex]
	value := keyword[1]
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
