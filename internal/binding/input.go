package binding

import (
	"fmt"
	"reflect"
)

// BindValue reconstructs the exact registered Go input and a fresh canonical map.
//
// arguments is the normalized map produced by worker-side binding. The parent
// re-checks every value against the compiled schema, builds the typed input by
// reflection, and returns a canonical copy that shares no container with
// arguments. An optional struct member that is absent or nil stays a nil
// pointer and is absent from the canonical map. Invalid maps are classified
// with ErrInvalidArguments.
func (plan *Plan) BindValue(arguments map[string]any) (any, map[string]any, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}
	input := reflect.New(plan.inputType).Elem()
	canonical, err := plan.rebindStruct(plan.inputSchema.Root, input, arguments, "")
	if err != nil {
		return nil, nil, err
	}
	return input.Interface(), canonical, nil
}

// rebindStruct writes a normalized object onto a struct target and returns its canonical copy.
func (plan *Plan) rebindStruct(
	index int,
	target reflect.Value,
	object map[string]any,
	path string,
) (map[string]any, error) {
	node := plan.inputSchema.Nodes[index]
	goFields := plan.inputNodes[index].fields
	canonical := make(map[string]any, len(node.Fields))
	for position, field := range node.Fields {
		member := memberPath(path, field.Name)
		value, present := object[field.Name]
		if plan.inputSchema.Nodes[field.Node].Kind == InputOptional && (!present || value == nil) {
			continue
		}
		if !present {
			return nil, fmt.Errorf("%w: missing required argument %q", ErrInvalidArguments, member)
		}
		converted, err := plan.rebindValue(field.Node, target.Field(goFields[position].index), value, member, false)
		if err != nil {
			return nil, err
		}
		canonical[field.Name] = converted
	}
	if len(object) > len(canonical) {
		for name := range object {
			if _, known := lookupInputField(node, name); !known {
				return nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, memberPath(path, name))
			}
		}
	}
	return canonical, nil
}

// rebindValue writes one normalized value onto target and returns its canonical copy.
//
// nullable reports that an enclosing optional node also accepts nil, which
// only changes the type-mismatch wording.
func (plan *Plan) rebindValue(index int, target reflect.Value, value any, path string, nullable bool) (any, error) {
	node := plan.inputSchema.Nodes[index]
	switch node.Kind {
	case InputString:
		if text, ok := value.(string); ok {
			target.SetString(text)
			return text, nil
		}
	case InputInt:
		if integer, ok := value.(int64); ok {
			return rebindInteger(node, target, integer, path)
		}
	case InputBool:
		if boolean, ok := value.(bool); ok {
			target.SetBool(boolean)
			return boolean, nil
		}
	case InputFloat:
		if number, ok := value.(float64); ok {
			checked, err := checkFloat(node, number, path)
			if err != nil {
				return nil, err
			}
			target.SetFloat(checked)
			return checked, nil
		}
	case InputOptional:
		return plan.rebindOptional(node, target, value, path)
	case InputList:
		if items, ok := value.([]any); ok {
			return plan.rebindList(node, target, items, path)
		}
	case InputMap:
		if object, ok := value.(map[string]any); ok {
			return plan.rebindMap(node, target, object, path)
		}
	case InputStruct:
		if object, ok := value.(map[string]any); ok {
			return plan.rebindStruct(index, target, object, path)
		}
	}
	return nil, typeError(node.Kind, path, nullable)
}

// rebindInteger range-checks and stores a signed or unsigned integer.
func rebindInteger(node InputNode, target reflect.Value, value int64, path string) (any, error) {
	checked, err := checkInteger(node, value, path)
	if err != nil {
		return nil, err
	}
	if target.CanUint() {
		target.SetUint(uint64(checked)) //nolint:gosec // checkInteger bounds unsigned nodes to 0..IntMax.
	} else {
		target.SetInt(checked)
	}
	return checked, nil
}

// rebindOptional stores nil or a freshly allocated pointer to the element value.
func (plan *Plan) rebindOptional(node InputNode, target reflect.Value, value any, path string) (any, error) {
	if value == nil {
		return nil, nil //nolint:nilnil // None maps to JSON null inside lists and dicts.
	}
	pointer := reflect.New(target.Type().Elem())
	converted, err := plan.rebindValue(node.Elem, pointer.Elem(), value, path, true)
	if err != nil {
		return nil, err
	}
	target.Set(pointer)
	return converted, nil
}

// rebindList writes a normalized list onto a slice target.
func (plan *Plan) rebindList(node InputNode, target reflect.Value, items []any, path string) (any, error) {
	slice := reflect.MakeSlice(target.Type(), len(items), len(items))
	canonical := make([]any, len(items))
	for index, item := range items {
		converted, err := plan.rebindValue(node.Elem, slice.Index(index), item, indexPath(path, index), false)
		if err != nil {
			return nil, err
		}
		canonical[index] = converted
	}
	target.Set(slice)
	return canonical, nil
}

// rebindMap writes a normalized string-keyed object onto a map target.
func (plan *Plan) rebindMap(node InputNode, target reflect.Value, object map[string]any, path string) (any, error) {
	mapType := target.Type()
	result := reflect.MakeMapWithSize(mapType, len(object))
	canonical := make(map[string]any, len(object))
	for key, item := range object {
		element := reflect.New(mapType.Elem()).Elem()
		converted, err := plan.rebindValue(node.Elem, element, item, path+keySegment(key), false)
		if err != nil {
			return nil, err
		}
		result.SetMapIndex(reflect.ValueOf(key).Convert(mapType.Key()), element)
		canonical[key] = converted
	}
	target.Set(result)
	return canonical, nil
}
