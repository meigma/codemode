package binding

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// ConvertOutput converts the plan's exact handler output to a process-neutral object.
//
// maxDepth and maxNodes must be positive. The root struct is depth 1. Pointers
// add no depth. Nested structs, lists, and maps add one depth through their
// value node. Destination maps, lists, and sorted map-key slices are allocated
// only after their child counts fit the remaining materialization budget.
func (plan *Plan) ConvertOutput(output any, maxDepth int, maxNodes int) (map[string]any, error) {
	if plan == nil {
		return nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}
	converter, err := newValueConverter(maxDepth, maxNodes)
	if err != nil {
		return nil, err
	}
	value := reflect.ValueOf(output)
	if !value.IsValid() || value.Type() != plan.outputType {
		return nil, fmt.Errorf("%w: handler output type does not match the compiled plan", ErrUnsupportedValue)
	}
	converted, err := plan.convertNode(plan.outputRoot, value, "output", 1, &converter)
	if err != nil {
		return nil, err
	}
	object, ok := converted.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: handler output type does not match the compiled plan", ErrUnsupportedValue)
	}
	return object, nil
}

// convertNode converts one compiled node to a process-neutral value.
func (plan *Plan) convertNode(
	index int,
	value reflect.Value,
	path string,
	depth int,
	converter *valueConverter,
) (any, error) {
	node := plan.outputNodes[index]
	switch node.kind {
	case outputNodePointer:
		return plan.convertPointer(node, value, path, depth, converter)
	case outputNodeString:
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return value.String(), nil
	case outputNodeInt:
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return value.Int(), nil
	case outputNodeUint:
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return convertOutputUint(value, path)
	case outputNodeBool:
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return value.Bool(), nil
	case outputNodeFloat:
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return convertOutputFloat(value, path)
	case outputNodeBytes:
		return convertOutputBytes(value, path, depth, converter)
	case outputNodeList:
		return plan.convertList(node, value, path, depth, converter)
	case outputNodeMap:
		return plan.convertMap(node, value, path, depth, converter)
	case outputNodeStruct:
		return plan.convertStruct(node, value, path, depth, converter)
	default:
		return nil, fmt.Errorf("%w: %s has an unknown compiled kind", ErrInvalidPlan, path)
	}
}

// convertPointer follows a pointer without adding depth and maps nil to None.
func (plan *Plan) convertPointer(
	node outputNode,
	value reflect.Value,
	path string,
	depth int,
	converter *valueConverter,
) (any, error) {
	if value.IsNil() {
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // A nil pointer is process-neutral None.
	}
	return plan.convertNode(node.elem, value.Elem(), path, depth, converter)
}

// convertStruct materializes included fields after preflighting their count.
func (plan *Plan) convertStruct(
	node outputNode,
	value reflect.Value,
	path string,
	depth int,
	converter *valueConverter,
) (any, error) {
	if err := converter.consumeNode(depth); err != nil {
		return nil, err
	}
	included := 0
	for _, field := range node.fields {
		if omitOutputField(field, value.Field(field.index)) {
			continue
		}
		included++
	}
	if _, err := converter.containerLength(included); err != nil {
		return nil, err
	}
	object := make(map[string]any, included)
	for _, field := range node.fields {
		fieldValue := value.Field(field.index)
		if omitOutputField(field, fieldValue) {
			continue
		}
		converted, err := plan.convertNode(
			field.node,
			fieldValue,
			outputRuntimeField(path, field.name),
			depth+1,
			converter,
		)
		if err != nil {
			return nil, err
		}
		object[field.name] = converted
	}
	return object, nil
}

// convertList materializes array and non-nil slice elements after preflight.
func (plan *Plan) convertList(
	node outputNode,
	value reflect.Value,
	path string,
	depth int,
	converter *valueConverter,
) (any, error) {
	if value.Kind() == reflect.Slice && value.IsNil() {
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // A nil slice is process-neutral None.
	}
	if err := converter.consumeNode(depth); err != nil {
		return nil, err
	}
	length, err := converter.containerLength(value.Len())
	if err != nil {
		return nil, err
	}
	items := make([]any, length)
	for index := range length {
		converted, err := plan.convertNode(
			node.elem,
			value.Index(index),
			outputRuntimeIndex(path, index),
			depth+1,
			converter,
		)
		if err != nil {
			return nil, err
		}
		items[index] = converted
	}
	return items, nil
}

// convertMap materializes a string-keyed map in sorted key order after preflight.
func (plan *Plan) convertMap(
	node outputNode,
	value reflect.Value,
	path string,
	depth int,
	converter *valueConverter,
) (any, error) {
	if value.IsNil() {
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // A nil map is process-neutral None.
	}
	if err := converter.consumeNode(depth); err != nil {
		return nil, err
	}
	length, err := converter.containerLength(value.Len())
	if err != nil {
		return nil, err
	}
	keys := value.MapKeys()
	slices.SortFunc(keys, compareMapKeys)
	object := make(map[string]any, length)
	for _, key := range keys {
		name := key.String()
		converted, err := plan.convertNode(
			node.elem,
			value.MapIndex(key),
			outputRuntimeKey(path, name),
			depth+1,
			converter,
		)
		if err != nil {
			return nil, err
		}
		object[name] = converted
	}
	return object, nil
}

// convertOutputBytes materializes a byte slice or array as a list of integers.
func convertOutputBytes(value reflect.Value, path string, depth int, converter *valueConverter) (any, error) {
	if value.Kind() == reflect.Slice && value.IsNil() {
		if err := converter.consumeNode(depth); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // A nil byte slice is process-neutral None.
	}
	if err := converter.consumeNode(depth); err != nil {
		return nil, err
	}
	length, err := converter.containerLength(value.Len())
	if err != nil {
		return nil, err
	}
	items := make([]any, length)
	for index := range length {
		if err := converter.consumeNode(depth + 1); err != nil {
			return nil, fmt.Errorf("%w: %s[%d]", err, path, index)
		}
		item, err := convertOutputUint(value.Index(index), path)
		if err != nil {
			return nil, err
		}
		items[index] = item
	}
	return items, nil
}

// convertOutputUint normalizes an unsigned integer when it fits int64.
func convertOutputUint(value reflect.Value, path string) (int64, error) {
	integer := value.Uint()
	if integer > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("%w: %s overflows int64", ErrUnsupportedValue, path)
	}
	return int64(integer), nil
}

// convertOutputFloat normalizes a floating-point value when it is finite.
func convertOutputFloat(value reflect.Value, path string) (float64, error) {
	float := value.Float()
	if !isFiniteFloat(float) {
		return 0, fmt.Errorf("%w: %s is not finite", ErrUnsupportedValue, path)
	}
	return float, nil
}

// omitOutputField reports whether a nil pointer+omitempty field is excluded.
func omitOutputField(field outputStructField, value reflect.Value) bool {
	return field.omitempty && value.Kind() == reflect.Pointer && value.IsNil()
}

// compareMapKeys orders reflect map keys by their string form.
func compareMapKeys(left reflect.Value, right reflect.Value) int {
	return strings.Compare(left.String(), right.String())
}

// outputRuntimeField joins a runtime path with a struct field name.
func outputRuntimeField(path string, name string) string {
	return path + "." + name
}

// outputRuntimeIndex joins a runtime path with a list index.
func outputRuntimeIndex(path string, index int) string {
	return path + "[" + strconv.Itoa(index) + "]"
}

// outputRuntimeKey joins a runtime path with a quoted map key.
func outputRuntimeKey(path string, key string) string {
	return path + "[" + strconv.Quote(key) + "]"
}
