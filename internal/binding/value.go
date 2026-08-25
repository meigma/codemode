package binding

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"

	"go.starlark.net/starlark"
)

// ValidateValue reports whether value is a process-neutral MCP value within the supplied limits.
//
// Accepted values are nil, bool, string, int64, finite float64, []any, and
// map[string]any, recursively. [json.Number] and other numeric types are rejected.
// maxDepth and maxNodes must be positive. Active cycles are rejected.
func ValidateValue(value any, maxDepth int, maxNodes int) error {
	converter, err := newValueConverter(maxDepth, maxNodes)
	if err != nil {
		return err
	}
	return converter.validate(value, 1)
}

// FromStarlark converts a Starlark value into a process-neutral MCP value.
//
// maxDepth and maxNodes must be positive. Active cycles are rejected, and
// destination containers are allocated only after their child counts fit the
// remaining materialization budget.
func FromStarlark(value starlark.Value, maxDepth int, maxNodes int) (any, error) {
	converter, err := newValueConverter(maxDepth, maxNodes)
	if err != nil {
		return nil, err
	}
	return converter.fromStarlark(value, 1)
}

// ToStarlark converts a process-neutral MCP value into a Starlark value.
//
// maxDepth and maxNodes must be positive. Active cycles are rejected, and
// destination containers are allocated only after their child counts fit the
// remaining materialization budget.
func ToStarlark(value any, maxDepth int, maxNodes int) (starlark.Value, error) {
	converter, err := newValueConverter(maxDepth, maxNodes)
	if err != nil {
		return nil, err
	}
	return converter.toStarlark(value, 1)
}

// visitKey identifies an active pointer container during cycle detection.
type visitKey struct {
	// kind distinguishes list, dictionary, slice, and map pointer identities.
	kind byte

	// pointer is the active container's Go pointer identity.
	pointer uintptr

	// length distinguishes visible slice ranges that share a backing array.
	length int
}

// valueConverter carries immutable depth configuration and path-local conversion state.
type valueConverter struct {
	// maxDepth is the maximum accepted nesting depth.
	maxDepth int

	// remainingNodes is the remaining materialization budget.
	remainingNodes int

	// active contains containers on the current recursive path.
	active map[visitKey]struct{}
}

// newValueConverter constructs conversion state after rejecting non-positive limits.
func newValueConverter(maxDepth int, maxNodes int) (valueConverter, error) {
	if maxDepth <= 0 || maxNodes <= 0 {
		return valueConverter{}, fmt.Errorf("%w: conversion limits must be positive", ErrValueLimit)
	}
	return valueConverter{
		maxDepth:       maxDepth,
		remainingNodes: maxNodes,
		active:         make(map[visitKey]struct{}),
	}, nil
}

// consumeNode accounts for one visited value against depth and materialization limits.
func (converter *valueConverter) consumeNode(depth int) error {
	if depth > converter.maxDepth {
		return fmt.Errorf("%w: value exceeds depth %d", ErrValueLimit, converter.maxDepth)
	}
	if converter.remainingNodes == 0 {
		return fmt.Errorf("%w: value exceeds byte-derived node budget", ErrValueLimit)
	}
	converter.remainingNodes--
	return nil
}

// containerLength returns length when that many child nodes fit the remaining budget.
func (converter *valueConverter) containerLength(length int) (int, error) {
	if length > converter.remainingNodes {
		return 0, fmt.Errorf("%w: value exceeds byte-derived node budget", ErrValueLimit)
	}
	return length, nil
}

// enter adds a pointer container to the active recursion path.
func (converter *valueConverter) enter(kind byte, value any) (visitKey, error) {
	return converter.enterKey(visitKey{kind: kind, pointer: reflect.ValueOf(value).Pointer()})
}

// leave removes a pointer container from the active recursion path.
func (converter *valueConverter) leave(key visitKey) {
	delete(converter.active, key)
}

// fromStarlark recursively converts one supported Starlark value.
func (converter *valueConverter) fromStarlark(value starlark.Value, depth int) (any, error) {
	if isNilStarlarkValue(value) {
		return nil, fmt.Errorf("%w: nil Starlark value", ErrUnsupportedValue)
	}
	if err := converter.consumeNode(depth); err != nil {
		return nil, err
	}

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
		return converter.fromStarlarkTuple(value, depth)
	case *starlark.List:
		return converter.fromStarlarkList(value, depth)
	case *starlark.Dict:
		return converter.fromStarlarkDict(value, depth)
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
	if !isFiniteFloat(float) {
		return nil, fmt.Errorf("%w: float is not finite", ErrUnsupportedValue)
	}
	return float, nil
}

// isFiniteFloat reports whether value is a finite floating-point number.
func isFiniteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// fromStarlarkTuple recursively converts an immutable sequence.
func (converter *valueConverter) fromStarlarkTuple(value starlark.Tuple, depth int) ([]any, error) {
	length, err := converter.containerLength(len(value))
	if err != nil {
		return nil, err
	}
	items := make([]any, length)
	for index, item := range value {
		converted, err := converter.fromStarlark(item, depth+1)
		if err != nil {
			return nil, err
		}
		items[index] = converted
	}
	return items, nil
}

// fromStarlarkList recursively converts a mutable sequence while rejecting active cycles.
func (converter *valueConverter) fromStarlarkList(value *starlark.List, depth int) ([]any, error) {
	key, err := converter.enter('l', value)
	if err != nil {
		return nil, err
	}
	defer converter.leave(key)

	length, err := converter.containerLength(value.Len())
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, length)
	iterator := value.Iterate()
	defer iterator.Done()
	var item starlark.Value
	for iterator.Next(&item) {
		converted, err := converter.fromStarlark(item, depth+1)
		if err != nil {
			return nil, err
		}
		items = append(items, converted)
	}
	return items, nil
}

// fromStarlarkDict recursively converts a mutable dictionary with string keys while rejecting active cycles.
func (converter *valueConverter) fromStarlarkDict(value *starlark.Dict, depth int) (map[string]any, error) {
	key, err := converter.enter('d', value)
	if err != nil {
		return nil, err
	}
	defer converter.leave(key)

	length, err := converter.containerLength(value.Len())
	if err != nil {
		return nil, err
	}
	object := make(map[string]any, length)
	iterator := value.Iterate()
	defer iterator.Done()
	var dictKey starlark.Value
	for iterator.Next(&dictKey) {
		name, ok := starlark.AsString(dictKey)
		if !ok {
			return nil, fmt.Errorf("%w: dictionary key must be a string", ErrUnsupportedValue)
		}
		item, found, err := value.Get(dictKey)
		if err != nil {
			return nil, fmt.Errorf("%w: dictionary lookup: %w", ErrUnsupportedValue, err)
		}
		if !found {
			return nil, fmt.Errorf("%w: missing dictionary value", ErrUnsupportedValue)
		}
		converted, err := converter.fromStarlark(item, depth+1)
		if err != nil {
			return nil, err
		}
		object[name] = converted
	}
	return object, nil
}

// validate recursively checks one process-neutral value.
func (converter *valueConverter) validate(value any, depth int) error {
	if err := converter.consumeNode(depth); err != nil {
		return err
	}
	switch typed := value.(type) {
	case nil, bool, string, int64:
		return nil
	case float64:
		if !isFiniteFloat(typed) {
			return fmt.Errorf("%w: float is not finite", ErrUnsupportedValue)
		}
		return nil
	case []any:
		return converter.validateSlice(typed, depth)
	case map[string]any:
		return converter.validateMap(typed, depth)
	case json.Number:
		return fmt.Errorf("%w: Go type %T", ErrUnsupportedValue, typed)
	default:
		return fmt.Errorf("%w: Go type %T", ErrUnsupportedValue, value)
	}
}

// validateSlice recursively checks a process-neutral list while rejecting active cycles.
func (converter *valueConverter) validateSlice(value []any, depth int) error {
	if err := converter.enterGoContainer('s', value); err != nil {
		return err
	}
	defer converter.leaveGoContainer('s', value)

	if _, err := converter.containerLength(len(value)); err != nil {
		return err
	}
	for _, item := range value {
		if err := converter.validate(item, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// validateMap recursively checks a process-neutral object while rejecting active cycles.
func (converter *valueConverter) validateMap(value map[string]any, depth int) error {
	if err := converter.enterGoContainer('m', value); err != nil {
		return err
	}
	defer converter.leaveGoContainer('m', value)

	if _, err := converter.containerLength(len(value)); err != nil {
		return err
	}
	for _, item := range value {
		if err := converter.validate(item, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// toStarlark recursively converts one process-neutral value.
func (converter *valueConverter) toStarlark(value any, depth int) (starlark.Value, error) {
	if err := converter.consumeNode(depth); err != nil {
		return nil, err
	}
	switch typed := value.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(typed), nil
	case string:
		return starlark.String(typed), nil
	case int64:
		return starlark.MakeInt64(typed), nil
	case float64:
		if !isFiniteFloat(typed) {
			return nil, fmt.Errorf("%w: float is not finite", ErrUnsupportedValue)
		}
		return starlark.Float(typed), nil
	case []any:
		return converter.toStarlarkList(typed, depth)
	case map[string]any:
		return converter.toStarlarkDict(typed, depth)
	case json.Number:
		return nil, fmt.Errorf("%w: Go type %T", ErrUnsupportedValue, typed)
	default:
		return nil, fmt.Errorf("%w: Go type %T", ErrUnsupportedValue, value)
	}
}

// toStarlarkList recursively converts a process-neutral list while rejecting active cycles.
func (converter *valueConverter) toStarlarkList(value []any, depth int) (starlark.Value, error) {
	if err := converter.enterGoContainer('s', value); err != nil {
		return nil, err
	}
	defer converter.leaveGoContainer('s', value)

	length, err := converter.containerLength(len(value))
	if err != nil {
		return nil, err
	}
	items := make([]starlark.Value, length)
	for index, item := range value {
		converted, err := converter.toStarlark(item, depth+1)
		if err != nil {
			return nil, err
		}
		items[index] = converted
	}
	return starlark.NewList(items), nil
}

// toStarlarkDict recursively converts a process-neutral object while rejecting active cycles.
func (converter *valueConverter) toStarlarkDict(value map[string]any, depth int) (starlark.Value, error) {
	if err := converter.enterGoContainer('m', value); err != nil {
		return nil, err
	}
	defer converter.leaveGoContainer('m', value)

	length, err := converter.containerLength(len(value))
	if err != nil {
		return nil, err
	}
	object := starlark.NewDict(length)
	for _, name := range slices.Sorted(maps.Keys(value)) {
		converted, err := converter.toStarlark(value[name], depth+1)
		if err != nil {
			return nil, err
		}
		if err := object.SetKey(starlark.String(name), converted); err != nil {
			return nil, fmt.Errorf("%w: dictionary key %q: %w", ErrUnsupportedValue, name, err)
		}
	}
	return object, nil
}

// enterGoContainer records a Go container on the active recursion path.
func (converter *valueConverter) enterGoContainer(kind byte, value any) error {
	key, ok := goContainerKey(kind, value)
	if !ok {
		return nil
	}
	_, err := converter.enterKey(key)
	return err
}

// leaveGoContainer removes a Go container from the active recursion path.
func (converter *valueConverter) leaveGoContainer(kind byte, value any) {
	key, ok := goContainerKey(kind, value)
	if !ok {
		return
	}
	converter.leave(key)
}

// enterKey adds a prepared container identity to the active recursion path.
func (converter *valueConverter) enterKey(key visitKey) (visitKey, error) {
	if _, exists := converter.active[key]; exists {
		return visitKey{}, fmt.Errorf("%w: cyclic value", ErrUnsupportedValue)
	}
	converter.active[key] = struct{}{}
	return key, nil
}

// goContainerKey reports the active-path identity for a non-empty Go container.
func goContainerKey(kind byte, value any) (visitKey, bool) {
	if value == nil {
		return visitKey{}, false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() { //nolint:exhaustive // Only Slice and Map have supported container identity.
	case reflect.Slice:
		if reflected.IsNil() || reflected.Len() == 0 {
			return visitKey{}, false
		}
		return visitKey{kind: kind, pointer: reflected.Pointer(), length: reflected.Len()}, true
	case reflect.Map:
		if reflected.IsNil() {
			return visitKey{}, false
		}
		return visitKey{kind: kind, pointer: reflected.Pointer()}, true
	default:
		return visitKey{}, false
	}
}
