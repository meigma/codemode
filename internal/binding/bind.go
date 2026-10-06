package binding

import (
	"fmt"
	"math"

	"go.starlark.net/starlark"
)

// keywordTupleSize is the length of one Starlark keyword argument tuple.
const keywordTupleSize = 2

// argumentBinder carries one worker-side call's schema, budget, and diagnostic path.
type argumentBinder struct {
	// schema is the validated input schema.
	schema InputSchema

	// converter enforces the depth and byte-derived node budget.
	converter valueConverter

	// path is the active diagnostic path, rendered only on error.
	path argumentPath
}

// Bind binds keyword-only Starlark arguments against a validated schema.
//
// It returns only a fresh canonical JSON-shaped map. Explicit Starlark None and
// omission both leave an optional member absent at every struct level. Callers
// must not treat the result as the authoritative authorization map; parent
// re-binding constructs that.
//
// maxDepth and maxNodes must be positive and bound the canonical map exactly
// as [FromStarlark] bounds a value: the argument map is depth 1, optional
// wrappers add no depth, and every container's child count is charged against
// the node budget before the container is materialized. Starlark values that
// alias one container many times therefore fail with [ErrValueLimit] instead
// of being copied. Argument mistakes wrap [ErrInvalidArguments].
func (schema InputSchema) Bind(
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	maxDepth int,
	maxNodes int,
) (map[string]any, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("%w: positional arguments are not accepted", ErrInvalidArguments)
	}
	for _, keyword := range kwargs {
		if len(keyword) != keywordTupleSize {
			return nil, fmt.Errorf("%w: malformed keyword argument", ErrInvalidArguments)
		}
	}
	converter, err := newValueConverter(maxDepth, maxNodes)
	if err != nil {
		return nil, err
	}
	binder := argumentBinder{schema: schema, converter: converter, path: newArgumentPath()}
	if err := binder.converter.consumeNode(1); err != nil {
		return nil, err
	}
	if _, err := binder.converter.containerLength(len(kwargs)); err != nil {
		return nil, err
	}
	return binder.bindStruct(schema.Nodes[schema.Root], kwargs, 1)
}

// bindStruct binds name/value pairs against a struct node's declared fields.
//
// The caller has already charged the struct node and its entry count.
func (binder *argumentBinder) bindStruct(node InputNode, entries []starlark.Tuple, depth int) (map[string]any, error) {
	converted := make(map[string]any, len(entries))
	seen := make([]bool, len(node.Fields))
	for _, entry := range entries {
		name, ok := entry[0].(starlark.String)
		if !ok {
			return nil, binder.keyError()
		}
		binder.path.push(pathSegment{kind: segmentMember, name: string(name)})
		position := fieldPosition(node, string(name))
		if position < 0 {
			return nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, binder.path.String())
		}
		if seen[position] {
			return nil, fmt.Errorf("%w: duplicate argument %q", ErrInvalidArguments, binder.path.String())
		}
		seen[position] = true
		field := node.Fields[position]
		if binder.schema.Nodes[field.Node].Kind != InputOptional || entry[1] != starlark.None {
			value, err := binder.bindValue(field.Node, entry[1], depth+1, false)
			if err != nil {
				return nil, err
			}
			converted[field.Name] = value
		}
		binder.path.pop()
	}
	for position, field := range node.Fields {
		if !seen[position] && binder.schema.Nodes[field.Node].Kind != InputOptional {
			binder.path.push(pathSegment{kind: segmentMember, name: field.Name})
			return nil, fmt.Errorf("%w: missing required argument %q", ErrInvalidArguments, binder.path.String())
		}
	}
	return converted, nil
}

// bindValue binds one Starlark value against the node at index.
//
// nullable reports that an enclosing optional node also accepts None, which
// only changes the type-mismatch wording.
func (binder *argumentBinder) bindValue(index int, value starlark.Value, depth int, nullable bool) (any, error) {
	node := binder.schema.Nodes[index]
	if node.Kind == InputOptional {
		if value == starlark.None {
			if err := binder.converter.consumeNode(depth); err != nil {
				return nil, err
			}
			return nil, nil //nolint:nilnil // None maps to JSON null inside lists and dicts.
		}
		return binder.bindValue(node.Elem, value, depth, true)
	}
	if err := binder.converter.consumeNode(depth); err != nil {
		return nil, err
	}
	switch node.Kind {
	case InputList:
		return binder.bindList(node, value, depth, nullable)
	case InputMap:
		return binder.bindMap(node, value, depth, nullable)
	case InputStruct:
		if dict, ok := value.(*starlark.Dict); ok {
			if _, err := binder.converter.containerLength(dict.Len()); err != nil {
				return nil, err
			}
			return binder.bindStruct(node, dict.Items(), depth)
		}
		return nil, typeError(node.Kind, &binder.path, nullable)
	case InputString, InputInt, InputBool, InputFloat, InputOptional:
	}
	return binder.bindScalar(node, value, nullable)
}

// bindScalar binds one already-charged Starlark scalar against a scalar node.
func (binder *argumentBinder) bindScalar(node InputNode, value starlark.Value, nullable bool) (any, error) {
	switch node.Kind {
	case InputString:
		if text, ok := value.(starlark.String); ok {
			return string(text), nil
		}
	case InputInt:
		if integer, ok := value.(starlark.Int); ok {
			converted, fits := integer.Int64()
			if !fits {
				return nil, integerRangeError(node, &binder.path)
			}
			return checkInteger(node, converted, &binder.path)
		}
	case InputBool:
		if boolean, ok := value.(starlark.Bool); ok {
			return bool(boolean), nil
		}
	case InputFloat:
		switch number := value.(type) {
		case starlark.Float:
			return checkFloat(node, float64(number), &binder.path)
		case starlark.Int:
			return checkFloat(node, float64(number.Float()), &binder.path)
		}
	case InputList, InputMap, InputStruct, InputOptional:
	}
	return nil, typeError(node.Kind, &binder.path, nullable)
}

// bindList binds a Starlark list or tuple element by element after charging its length.
func (binder *argumentBinder) bindList(node InputNode, value starlark.Value, depth int, nullable bool) (any, error) {
	var sequence starlark.Indexable
	switch typed := value.(type) {
	case *starlark.List:
		sequence = typed
	case starlark.Tuple:
		sequence = typed
	default:
		return nil, typeError(InputList, &binder.path, nullable)
	}
	length, err := binder.converter.containerLength(sequence.Len())
	if err != nil {
		return nil, err
	}
	converted := make([]any, length)
	for index := range converted {
		binder.path.push(pathSegment{kind: segmentIndex, index: index})
		element, err := binder.bindValue(node.Elem, sequence.Index(index), depth+1, false)
		if err != nil {
			return nil, err
		}
		binder.path.pop()
		converted[index] = element
	}
	return converted, nil
}

// bindMap binds a str-keyed Starlark dict value by value after charging its length.
func (binder *argumentBinder) bindMap(node InputNode, value starlark.Value, depth int, nullable bool) (any, error) {
	dict, ok := value.(*starlark.Dict)
	if !ok {
		return nil, typeError(InputMap, &binder.path, nullable)
	}
	length, err := binder.converter.containerLength(dict.Len())
	if err != nil {
		return nil, err
	}
	converted := make(map[string]any, length)
	for _, item := range dict.Items() {
		key, isString := item[0].(starlark.String)
		if !isString {
			return nil, binder.keyError()
		}
		binder.path.push(pathSegment{kind: segmentKey, name: string(key)})
		element, err := binder.bindValue(node.Elem, item[1], depth+1, false)
		if err != nil {
			return nil, err
		}
		binder.path.pop()
		converted[string(key)] = element
	}
	return converted, nil
}

// keyError reports a non-string keyword name or dict key at the current path.
func (binder *argumentBinder) keyError() error {
	if len(binder.path.segments) == 0 {
		return fmt.Errorf("%w: keyword name must be a string", ErrInvalidArguments)
	}
	return fmt.Errorf("%w: argument %q keys must be strings", ErrInvalidArguments, binder.path.String())
}

// checkInteger enforces an int node's inclusive range.
func checkInteger(node InputNode, value int64, path *argumentPath) (int64, error) {
	if value < node.IntMin || value > node.IntMax {
		return 0, integerRangeError(node, path)
	}
	return value, nil
}

// integerRangeError describes an integer outside the node's range.
func integerRangeError(node InputNode, path *argumentPath) error {
	if node.IntMin == math.MinInt64 && node.IntMax == math.MaxInt64 {
		return fmt.Errorf("%w: argument %q overflows int64", ErrInvalidArguments, path.String())
	}
	return fmt.Errorf("%w: argument %q must be between %d and %d",
		ErrInvalidArguments, path.String(), node.IntMin, node.IntMax)
}

// checkFloat enforces finiteness and the float32 range, then canonicalizes.
//
// A float32 node returns the value rounded to float32, so authorization sees
// exactly the number the handler receives.
func checkFloat(node InputNode, value float64, path *argumentPath) (float64, error) {
	if !isFiniteFloat(value) {
		return 0, fmt.Errorf("%w: argument %q is not finite", ErrInvalidArguments, path.String())
	}
	if !node.Float32 {
		return value, nil
	}
	if math.Abs(value) > math.MaxFloat32 {
		return 0, fmt.Errorf("%w: argument %q overflows float32", ErrInvalidArguments, path.String())
	}
	return float64(float32(value)), nil
}

// typeError describes the value a node expected.
func typeError(kind InputKind, path *argumentPath, nullable bool) error {
	if nullable {
		return fmt.Errorf("%w: argument %q must be %s or None", ErrInvalidArguments, path.String(), expectedValue(kind))
	}
	return fmt.Errorf("%w: argument %q must be %s", ErrInvalidArguments, path.String(), expectedValue(kind))
}

// expectedValue names the accepted Starlark value for kind.
func expectedValue(kind InputKind) string {
	switch kind {
	case InputString:
		return "a string"
	case InputInt:
		return "an integer"
	case InputBool:
		return "a bool"
	case InputFloat:
		return "a float"
	case InputList:
		return "a list"
	case InputMap, InputStruct:
		return "a dict"
	case InputOptional:
	}
	return "a supported value"
}
