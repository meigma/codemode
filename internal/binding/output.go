package binding

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"go.starlark.net/starlark"
)

// ConvertOutput converts the plan's exact handler output directly to a Starlark object.
func (plan *Plan) ConvertOutput(output any) (starlark.Value, error) {
	if plan == nil {
		return nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}
	value := reflect.ValueOf(output)
	if !value.IsValid() || value.Type() != plan.outputType {
		return nil, fmt.Errorf("%w: handler output type does not match the compiled plan", ErrUnsupportedValue)
	}

	converted := starlark.NewDict(len(plan.outputFields))
	for _, field := range plan.outputFields {
		item, err := convertOutputField(field, value.Field(field.index))
		if err != nil {
			return nil, err
		}
		if err := converted.SetKey(starlark.String(field.name), item); err != nil {
			return nil, fmt.Errorf("%w: output field %q: %w", ErrUnsupportedValue, field.name, err)
		}
	}
	return converted, nil
}

// ConvertFinal converts a final Starlark value to MCP-safe JSON data under depth and byte limits.
func ConvertFinal(value starlark.Value, maxDepth int, maxBytes int) (any, error) {
	if maxDepth <= 0 || maxBytes <= 0 {
		return nil, fmt.Errorf("%w: conversion limits must be positive", ErrValueLimit)
	}
	converter := finalConverter{
		maxDepth:       maxDepth,
		remainingNodes: maxBytes,
		active:         make(map[visitKey]struct{}),
	}
	converted, err := converter.convert(value, 1)
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

// visitKey identifies an active pointer container during cycle detection.
type visitKey struct {
	// kind distinguishes list and dictionary pointer identities.
	kind byte

	// pointer is the active container's Go pointer identity.
	pointer uintptr
}

// finalConverter carries immutable depth configuration and path-local conversion state.
type finalConverter struct {
	// maxDepth is the maximum accepted nesting depth.
	maxDepth int

	// remainingNodes bounds materialization before exact JSON byte measurement.
	remainingNodes int

	// active contains containers on the current recursive path.
	active map[visitKey]struct{}
}

// convert recursively converts one supported Starlark value.
func (converter *finalConverter) convert(value starlark.Value, depth int) (any, error) {
	if isNilStarlarkValue(value) {
		return nil, fmt.Errorf("%w: nil Starlark value", ErrUnsupportedValue)
	}
	if depth > converter.maxDepth {
		return nil, fmt.Errorf("%w: value exceeds depth %d", ErrValueLimit, converter.maxDepth)
	}
	if converter.remainingNodes == 0 {
		return nil, fmt.Errorf("%w: value exceeds byte-derived node budget", ErrValueLimit)
	}
	converter.remainingNodes--

	switch value := value.(type) {
	case starlark.NoneType:
		return nil, nil //nolint:nilnil // Starlark None intentionally maps to JSON null.
	case starlark.Bool:
		return bool(value), nil
	case starlark.String:
		return string(value), nil
	case starlark.Int:
		return convertInteger(value)
	case starlark.Float:
		return convertFloat(value)
	case starlark.Tuple:
		return converter.convertTuple(value, depth)
	case *starlark.List:
		return converter.convertList(value, depth)
	case *starlark.Dict:
		return converter.convertDict(value, depth)
	default:
		return nil, fmt.Errorf("%w: Starlark type %s", ErrUnsupportedValue, value.Type())
	}
}

// isNilStarlarkValue reports whether an interface contains no value or a typed nil pointer.
func isNilStarlarkValue(value starlark.Value) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

// convertOutputField converts one field according to its compiled output kind.
func convertOutputField(field outputField, value reflect.Value) (starlark.Value, error) {
	switch field.kind {
	case fieldString:
		return starlark.String(value.String()), nil
	case fieldInt64:
		return starlark.MakeInt64(value.Int()), nil
	case fieldBool:
		return starlark.Bool(value.Bool()), nil
	case fieldFloat64:
		float := value.Float()
		if math.IsNaN(float) || math.IsInf(float, 0) {
			return nil, fmt.Errorf("%w: output field %q is not finite", ErrUnsupportedValue, field.name)
		}
		return starlark.Float(float), nil
	case fieldOptionalInt64:
		return nil, fmt.Errorf("%w: output field %q has an invalid compiled kind", ErrInvalidPlan, field.name)
	}
	return nil, fmt.Errorf("%w: output field %q has an unknown compiled kind", ErrInvalidPlan, field.name)
}

// convertInteger converts a Starlark integer when it fits the MCP signed integer surface.
func convertInteger(value starlark.Int) (any, error) {
	integer, ok := value.Int64()
	if !ok {
		return nil, fmt.Errorf("%w: integer overflows int64", ErrUnsupportedValue)
	}
	return integer, nil
}

// convertFloat converts a Starlark float when it is finite.
func convertFloat(value starlark.Float) (any, error) {
	float := float64(value)
	if math.IsNaN(float) || math.IsInf(float, 0) {
		return nil, fmt.Errorf("%w: float is not finite", ErrUnsupportedValue)
	}
	return float, nil
}

// convertTuple recursively converts an immutable sequence.
func (converter *finalConverter) convertTuple(value starlark.Tuple, depth int) ([]any, error) {
	items := make([]any, len(value))
	for index, item := range value {
		converted, err := converter.convert(item, depth+1)
		if err != nil {
			return nil, err
		}
		items[index] = converted
	}
	return items, nil
}

// convertList recursively converts a mutable sequence while rejecting active cycles.
func (converter *finalConverter) convertList(value *starlark.List, depth int) ([]any, error) {
	key, err := converter.enter('l', value)
	if err != nil {
		return nil, err
	}
	defer converter.leave(key)

	items := make([]any, 0, value.Len())
	iterator := value.Iterate()
	defer iterator.Done()
	var item starlark.Value
	for iterator.Next(&item) {
		converted, err := converter.convert(item, depth+1)
		if err != nil {
			return nil, err
		}
		items = append(items, converted)
	}
	return items, nil
}

// convertDict recursively converts a mutable dictionary with string keys while rejecting active cycles.
func (converter *finalConverter) convertDict(value *starlark.Dict, depth int) (map[string]any, error) {
	key, err := converter.enter('d', value)
	if err != nil {
		return nil, err
	}
	defer converter.leave(key)

	object := make(map[string]any, value.Len())
	for _, item := range value.Items() {
		name, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("%w: dictionary key must be a string", ErrUnsupportedValue)
		}
		converted, err := converter.convert(item[1], depth+1)
		if err != nil {
			return nil, err
		}
		object[name] = converted
	}
	return object, nil
}

// enter adds a pointer container to the active recursion path.
func (converter *finalConverter) enter(kind byte, value any) (visitKey, error) {
	key := visitKey{kind: kind, pointer: reflect.ValueOf(value).Pointer()}
	if _, exists := converter.active[key]; exists {
		return visitKey{}, fmt.Errorf("%w: cyclic value", ErrUnsupportedValue)
	}
	converter.active[key] = struct{}{}
	return key, nil
}

// leave removes a pointer container from the active recursion path.
func (converter *finalConverter) leave(key visitKey) {
	delete(converter.active, key)
}
