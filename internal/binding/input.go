package binding

import (
	"fmt"
	"reflect"
)

// argumentRebinder carries one parent-side re-binding's plan and diagnostic path.
type argumentRebinder struct {
	// plan supplies the schema and index-aligned Go type arena.
	plan *Plan

	// path is the active diagnostic path, rendered only on error.
	path argumentPath
}

// BindValue reconstructs the exact registered Go input and a fresh canonical map.
//
// arguments is the normalized map produced by worker-side binding. The parent
// re-checks every value against the compiled schema, builds the typed input by
// reflection, and returns a canonical copy that shares no container with
// arguments. An optional struct member that is absent or nil stays a nil
// pointer and is absent from the canonical map. Work is linear in the size of
// arguments: diagnostic paths are rendered only on error. Invalid maps are
// classified with ErrInvalidArguments.
func (plan *Plan) BindValue(arguments map[string]any) (any, map[string]any, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("%w: nil plan", ErrInvalidPlan)
	}
	rebinder := argumentRebinder{plan: plan, path: newArgumentPath()}
	input := reflect.New(plan.inputType).Elem()
	canonical, err := rebinder.rebindStruct(plan.inputSchema.Root, input, arguments)
	if err != nil {
		return nil, nil, err
	}
	return input.Interface(), canonical, nil
}

// rebindStruct writes a normalized object onto a struct target and returns its canonical copy.
func (rebinder *argumentRebinder) rebindStruct(
	index int,
	target reflect.Value,
	object map[string]any,
) (map[string]any, error) {
	node := rebinder.plan.inputSchema.Nodes[index]
	goFields := rebinder.plan.inputNodes[index].fields
	canonical := make(map[string]any, len(node.Fields))
	for position, field := range node.Fields {
		value, present := object[field.Name]
		if rebinder.plan.inputSchema.Nodes[field.Node].Kind == InputOptional && (!present || value == nil) {
			continue
		}
		rebinder.path.push(pathSegment{kind: segmentMember, name: field.Name})
		if !present {
			return nil, fmt.Errorf("%w: missing required argument %q", ErrInvalidArguments, rebinder.path.String())
		}
		converted, err := rebinder.rebindValue(field.Node, target.Field(goFields[position].index), value, false)
		if err != nil {
			return nil, err
		}
		rebinder.path.pop()
		canonical[field.Name] = converted
	}
	if len(object) > len(canonical) {
		for name := range object {
			if fieldPosition(node, name) < 0 {
				rebinder.path.push(pathSegment{kind: segmentMember, name: name})
				return nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, rebinder.path.String())
			}
		}
	}
	return canonical, nil
}

// rebindValue writes one normalized value onto target and returns its canonical copy.
//
// nullable reports that an enclosing optional node also accepts nil, which
// only changes the type-mismatch wording.
func (rebinder *argumentRebinder) rebindValue(index int, target reflect.Value, value any, nullable bool) (any, error) {
	node := rebinder.plan.inputSchema.Nodes[index]
	switch node.Kind {
	case InputString:
		if text, ok := value.(string); ok {
			target.SetString(text)
			return text, nil
		}
	case InputInt:
		if integer, ok := value.(int64); ok {
			return rebinder.rebindInteger(node, target, integer)
		}
	case InputBool:
		if boolean, ok := value.(bool); ok {
			target.SetBool(boolean)
			return boolean, nil
		}
	case InputFloat:
		if number, ok := value.(float64); ok {
			checked, err := checkFloat(node, number, &rebinder.path)
			if err != nil {
				return nil, err
			}
			target.SetFloat(checked)
			return checked, nil
		}
	case InputOptional:
		return rebinder.rebindOptional(node, target, value)
	case InputList:
		if items, ok := value.([]any); ok {
			return rebinder.rebindList(node, target, items)
		}
	case InputMap:
		if object, ok := value.(map[string]any); ok {
			return rebinder.rebindMap(node, target, object)
		}
	case InputStruct:
		if object, ok := value.(map[string]any); ok {
			return rebinder.rebindStruct(index, target, object)
		}
	}
	return nil, typeError(node.Kind, &rebinder.path, nullable)
}

// rebindInteger range-checks and stores a signed or unsigned integer.
func (rebinder *argumentRebinder) rebindInteger(node InputNode, target reflect.Value, value int64) (any, error) {
	checked, err := checkInteger(node, value, &rebinder.path)
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
func (rebinder *argumentRebinder) rebindOptional(node InputNode, target reflect.Value, value any) (any, error) {
	if value == nil {
		return nil, nil //nolint:nilnil // None maps to JSON null inside lists and dicts.
	}
	pointer := reflect.New(target.Type().Elem())
	converted, err := rebinder.rebindValue(node.Elem, pointer.Elem(), value, true)
	if err != nil {
		return nil, err
	}
	target.Set(pointer)
	return converted, nil
}

// rebindList writes a normalized list onto a slice target.
func (rebinder *argumentRebinder) rebindList(node InputNode, target reflect.Value, items []any) (any, error) {
	slice := reflect.MakeSlice(target.Type(), len(items), len(items))
	canonical := make([]any, len(items))
	for index, item := range items {
		rebinder.path.push(pathSegment{kind: segmentIndex, index: index})
		converted, err := rebinder.rebindValue(node.Elem, slice.Index(index), item, false)
		if err != nil {
			return nil, err
		}
		rebinder.path.pop()
		canonical[index] = converted
	}
	target.Set(slice)
	return canonical, nil
}

// rebindMap writes a normalized string-keyed object onto a map target.
func (rebinder *argumentRebinder) rebindMap(node InputNode, target reflect.Value, object map[string]any) (any, error) {
	mapType := target.Type()
	result := reflect.MakeMapWithSize(mapType, len(object))
	canonical := make(map[string]any, len(object))
	for key, item := range object {
		rebinder.path.push(pathSegment{kind: segmentKey, name: key})
		element := reflect.New(mapType.Elem()).Elem()
		converted, err := rebinder.rebindValue(node.Elem, element, item, false)
		if err != nil {
			return nil, err
		}
		rebinder.path.pop()
		result.SetMapIndex(reflect.ValueOf(key).Convert(mapType.Key()), element)
		canonical[key] = converted
	}
	target.Set(result)
	return canonical, nil
}
