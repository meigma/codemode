package binding

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

const keywordTupleSize = 2

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
// map. Invalid normalized maps are classified with ErrInvalidArguments.
func (plan *Plan) BindValue(arguments map[string]any) (any, map[string]any, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}

	input := reflect.New(plan.inputType).Elem()
	canonical := make(map[string]any, len(plan.inputFields))
	for _, field := range plan.inputFields {
		value, present := arguments[field.name]
		converted, omitted, err := convertCanonicalArgument(field, present, value)
		if err != nil {
			return nil, nil, err
		}
		if omitted {
			continue
		}
		if err := assignInputField(input.Field(field.index), field, converted); err != nil {
			return nil, nil, err
		}
		canonical[field.name] = converted
	}
	for name := range arguments {
		if _, known := plan.inputByName[name]; !known {
			return nil, nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, name)
		}
	}
	return input.Interface(), canonical, nil
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
		return convertStarlarkString(name, value, false)
	case isOptionalStringShape(field):
		if value == starlark.None {
			return nil, true, nil
		}
		return convertStarlarkString(name, value, true)
	case isRequiredIntegerShape(field):
		return convertStarlarkInteger(name, value, false)
	case isOptionalIntegerShape(field):
		if value == starlark.None {
			return nil, true, nil
		}
		return convertStarlarkInteger(name, value, true)
	case isRequiredBoolShape(field):
		return convertStarlarkBool(name, value, false)
	case isOptionalBoolShape(field):
		if value == starlark.None {
			return nil, true, nil
		}
		return convertStarlarkBool(name, value, true)
	case isRequiredFloatShape(field):
		return convertStarlarkFloat(name, value, false)
	case isOptionalFloatShape(field):
		if value == starlark.None {
			return nil, true, nil
		}
		return convertStarlarkFloat(name, value, true)
	default:
		return nil, false, fmt.Errorf("%w: argument %q has an unsupported shape", ErrInvalidPlan, name)
	}
}

// convertCanonicalArgument converts one normalized parent argument against a compiled field.
func convertCanonicalArgument(field inputField, present bool, value any) (any, bool, error) {
	if field.required && !present {
		return nil, false, fmt.Errorf("%w: missing required argument %q", ErrInvalidArguments, field.name)
	}
	if !field.required && (!present || value == nil) {
		return nil, true, nil
	}
	switch field.kind {
	case fieldString:
		return convertCanonicalString(field.name, value, false)
	case fieldOptionalString:
		return convertCanonicalString(field.name, value, true)
	case fieldInt64:
		return convertCanonicalInteger(field.name, value, false)
	case fieldOptionalInt64:
		return convertCanonicalInteger(field.name, value, true)
	case fieldBool:
		return convertCanonicalBool(field.name, value, false)
	case fieldOptionalBool:
		return convertCanonicalBool(field.name, value, true)
	case fieldFloat64:
		return convertCanonicalFloat(field.name, value, false)
	case fieldOptionalFloat64:
		return convertCanonicalFloat(field.name, value, true)
	default:
		return nil, false, fmt.Errorf("%w: argument %q has an unknown compiled kind", ErrInvalidPlan, field.name)
	}
}

// assignInputField writes one converted scalar onto the reconstructed Go input.
func assignInputField(target reflect.Value, field inputField, converted any) error {
	switch field.kind {
	case fieldString, fieldOptionalString:
		value, err := compiledScalar[string](converted, field.name)
		if err != nil {
			return err
		}
		return writeScalar(target, field.required, value, target.SetString)
	case fieldInt64, fieldOptionalInt64:
		value, err := compiledScalar[int64](converted, field.name)
		if err != nil {
			return err
		}
		return writeScalar(target, field.required, value, target.SetInt)
	case fieldBool, fieldOptionalBool:
		value, err := compiledScalar[bool](converted, field.name)
		if err != nil {
			return err
		}
		return writeScalar(target, field.required, value, target.SetBool)
	case fieldFloat64, fieldOptionalFloat64:
		value, err := compiledScalar[float64](converted, field.name)
		if err != nil {
			return err
		}
		return writeScalar(target, field.required, value, target.SetFloat)
	default:
		return fmt.Errorf("%w: argument %q has an unknown compiled kind", ErrInvalidPlan, field.name)
	}
}

// compiledScalar reports an impossible compiled/value mismatch as an invalid plan.
func compiledScalar[T any](converted any, name string) (T, error) {
	typed, ok := converted.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%w: argument %q has an invalid compiled value", ErrInvalidPlan, name)
	}
	return typed, nil
}

// writeScalar stores a required scalar or a non-nil optional pointer.
func writeScalar[T any](target reflect.Value, required bool, value T, set func(T)) error {
	if required {
		set(value)
		return nil
	}
	assignOptionalScalar(target, value)
	return nil
}

// assignOptionalScalar stores a non-nil pointer to converted on target.
func assignOptionalScalar(target reflect.Value, converted any) {
	pointer := reflect.New(target.Type().Elem())
	pointer.Elem().Set(reflect.ValueOf(converted).Convert(target.Type().Elem()))
	target.Set(pointer)
}

// convertStarlarkString converts a Starlark string argument.
func convertStarlarkString(name string, value starlark.Value, optional bool) (any, bool, error) {
	converted, ok := starlark.AsString(value)
	if !ok {
		return nil, false, scalarArgumentError(name, "a string", optional)
	}
	return converted, false, nil
}

// convertStarlarkInteger converts a Starlark integer that fits signed 64-bit range.
func convertStarlarkInteger(name string, value starlark.Value, optional bool) (any, bool, error) {
	integer, ok := value.(starlark.Int)
	if !ok {
		return nil, false, scalarArgumentError(name, "an integer", optional)
	}
	converted, ok := integer.Int64()
	if !ok {
		return nil, false, fmt.Errorf("%w: argument %q overflows int64", ErrInvalidArguments, name)
	}
	return converted, false, nil
}

// convertStarlarkBool converts a Starlark Boolean argument.
func convertStarlarkBool(name string, value starlark.Value, optional bool) (any, bool, error) {
	boolean, ok := value.(starlark.Bool)
	if !ok {
		return nil, false, scalarArgumentError(name, "a bool", optional)
	}
	return bool(boolean), false, nil
}

// convertStarlarkFloat converts a finite Starlark floating-point argument.
func convertStarlarkFloat(name string, value starlark.Value, optional bool) (any, bool, error) {
	number, ok := value.(starlark.Float)
	if !ok {
		return nil, false, scalarArgumentError(name, "a float", optional)
	}
	converted := float64(number)
	if !isFiniteFloat(converted) {
		return nil, false, fmt.Errorf("%w: argument %q is not finite", ErrInvalidArguments, name)
	}
	return converted, false, nil
}

// convertCanonicalString converts a normalized string argument.
func convertCanonicalString(name string, value any, optional bool) (any, bool, error) {
	converted, ok := value.(string)
	if !ok {
		return nil, false, scalarArgumentError(name, "a string", optional)
	}
	return converted, false, nil
}

// convertCanonicalInteger converts a normalized signed 64-bit integer argument.
func convertCanonicalInteger(name string, value any, optional bool) (any, bool, error) {
	converted, ok := value.(int64)
	if !ok {
		return nil, false, scalarArgumentError(name, "an integer", optional)
	}
	return converted, false, nil
}

// convertCanonicalBool converts a normalized Boolean argument.
func convertCanonicalBool(name string, value any, optional bool) (any, bool, error) {
	converted, ok := value.(bool)
	if !ok {
		return nil, false, scalarArgumentError(name, "a bool", optional)
	}
	return converted, false, nil
}

// convertCanonicalFloat converts a normalized finite floating-point argument.
func convertCanonicalFloat(name string, value any, optional bool) (any, bool, error) {
	converted, ok := value.(float64)
	if !ok {
		return nil, false, scalarArgumentError(name, "a float", optional)
	}
	if !isFiniteFloat(converted) {
		return nil, false, fmt.Errorf("%w: argument %q is not finite", ErrInvalidArguments, name)
	}
	return converted, false, nil
}

// scalarArgumentError classifies a mistyped scalar before authorization.
func scalarArgumentError(name string, expected string, optional bool) error {
	if optional {
		return fmt.Errorf("%w: argument %q must be %s or None", ErrInvalidArguments, name, expected)
	}
	return fmt.Errorf("%w: argument %q must be %s", ErrInvalidArguments, name, expected)
}
