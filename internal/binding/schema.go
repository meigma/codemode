package binding

import (
	"fmt"
	"math"
	"reflect"

	"go.starlark.net/starlark"
)

// keywordTupleSize is the length of one Starlark keyword argument tuple.
const keywordTupleSize = 2

// maxIntegerBits is the widest Go integer; unsigned 64-bit inputs cap at [math.MaxInt64] on the wire.
const maxIntegerBits = 64

// InputKind identifies one node kind in a process-neutral input schema.
type InputKind string

const (
	// InputString accepts a Starlark str.
	InputString InputKind = "str"

	// InputInt accepts a Starlark int within the node's inclusive IntMin..IntMax range.
	InputInt InputKind = "int"

	// InputBool accepts a Starlark bool.
	InputBool InputKind = "bool"

	// InputFloat accepts a finite Starlark float or int; Float32 narrows the range.
	InputFloat InputKind = "float"

	// InputList accepts a Starlark list or tuple of Elem values.
	InputList InputKind = "list"

	// InputMap accepts a Starlark dict with str keys and Elem values.
	InputMap InputKind = "dict"

	// InputStruct accepts a Starlark dict whose keys are a subset of the declared
	// fields and include every required field. The root struct binds keyword arguments.
	InputStruct InputKind = "struct"

	// InputOptional accepts None or an Elem value; a struct member may also be omitted.
	InputOptional InputKind = "optional"
)

// InputSchema is the process-neutral input description the worker uses to
// bind keyword arguments without access to Go types.
//
// Nodes form a post-order arena: every Elem and field Node index refers to an
// earlier node, which makes the graph acyclic by construction. Root is a
// struct node whose fields are the keyword arguments.
type InputSchema struct {
	// Root is the arena index of the keyword-argument struct.
	Root int `json:"root"`

	// Nodes is the post-order node arena.
	Nodes []InputNode `json:"nodes"`
}

// InputNode is one schema node.
type InputNode struct {
	// Kind selects the accepted Starlark value shape.
	Kind InputKind `json:"kind"`

	// Elem is the element node for list, dict, and optional kinds.
	Elem int `json:"elem,omitempty"`

	// Fields are the declaration-ordered members of a struct node.
	Fields []InputField `json:"fields,omitempty"`

	// IntMin is the inclusive lower bound of an int node.
	IntMin int64 `json:"int_min,omitempty"`

	// IntMax is the inclusive upper bound of an int node.
	IntMax int64 `json:"int_max,omitempty"`

	// Float32 reports that a float node must fit the float32 range.
	Float32 bool `json:"float32,omitempty"`
}

// InputField is one named struct member in an input schema.
type InputField struct {
	// Name is the exact Starlark keyword or dict key.
	Name string `json:"name"`

	// Node is the arena index of the field's type.
	Node int `json:"node"`
}

// projectInputSchema derives the process-neutral schema from a compiled input arena.
//
// The schema is index-aligned with nodes so parent re-binding can pair each
// schema node with its Go type.
func projectInputSchema(root int, nodes []typeNode) InputSchema {
	schema := InputSchema{Root: root, Nodes: make([]InputNode, len(nodes))}
	for index, node := range nodes {
		schema.Nodes[index] = projectInputNode(node)
	}
	return schema
}

// projectInputNode maps one compiled input node onto its schema node.
func projectInputNode(node typeNode) InputNode {
	switch node.kind {
	case nodeString:
		return InputNode{Kind: InputString}
	case nodeBool:
		return InputNode{Kind: InputBool}
	case nodeInt:
		bits := node.typ.Bits()
		return InputNode{
			Kind:   InputInt,
			IntMin: math.MinInt64 >> (maxIntegerBits - bits),
			IntMax: math.MaxInt64 >> (maxIntegerBits - bits),
		}
	case nodeUint:
		bits := node.typ.Bits()
		if bits == maxIntegerBits {
			return InputNode{Kind: InputInt, IntMax: math.MaxInt64}
		}
		return InputNode{Kind: InputInt, IntMax: 1<<bits - 1}
	case nodeFloat:
		return InputNode{Kind: InputFloat, Float32: node.typ.Kind() == reflect.Float32}
	case nodeList:
		return InputNode{Kind: InputList, Elem: node.elem}
	case nodeMap:
		return InputNode{Kind: InputMap, Elem: node.elem}
	case nodePointer:
		return InputNode{Kind: InputOptional, Elem: node.elem}
	case nodeStruct:
		fields := make([]InputField, len(node.fields))
		for index, field := range node.fields {
			fields[index] = InputField{Name: field.name, Node: field.node}
		}
		return InputNode{Kind: InputStruct, Fields: fields}
	case nodeBytes:
	}
	return InputNode{}
}

// Clone returns a deep copy that shares no slices with schema.
func (schema InputSchema) Clone() InputSchema {
	nodes := make([]InputNode, len(schema.Nodes))
	for index, node := range schema.Nodes {
		nodes[index] = node
		if node.Fields != nil {
			nodes[index].Fields = append([]InputField(nil), node.Fields...)
		}
	}
	return InputSchema{Root: schema.Root, Nodes: nodes}
}

// Validate reports whether schema is a well-formed acyclic arena with a struct root.
//
// Root members must be Starlark identifiers; nested members must be valid JSON
// field names. Callers validate once when loading a schema; Bind trusts it.
func (schema InputSchema) Validate() error {
	if schema.Root < 0 || schema.Root >= len(schema.Nodes) || schema.Nodes[schema.Root].Kind != InputStruct {
		return fmt.Errorf("%w: input schema root must be a struct node", ErrInvalidPlan)
	}
	for _, field := range schema.Nodes[schema.Root].Fields {
		if !ValidIdentifier(field.Name) {
			return fmt.Errorf("%w: input field %q is not a Starlark identifier", ErrInvalidPlan, field.Name)
		}
	}
	for index, node := range schema.Nodes {
		if err := validateInputNode(schema.Nodes, index, node); err != nil {
			return err
		}
	}
	return nil
}

// validateInputNode checks one node's kind, bounds, back-references, and member names.
func validateInputNode(nodes []InputNode, index int, node InputNode) error {
	if node.Kind != InputInt && (node.IntMin != 0 || node.IntMax != 0) {
		return fmt.Errorf("%w: input node %d has integer bounds", ErrInvalidPlan, index)
	}
	if node.Kind != InputFloat && node.Float32 {
		return fmt.Errorf("%w: input node %d has a float32 flag", ErrInvalidPlan, index)
	}
	switch node.Kind {
	case InputString, InputBool, InputFloat, InputInt:
		if node.Elem != 0 || node.Fields != nil {
			return fmt.Errorf("%w: scalar input node %d has children", ErrInvalidPlan, index)
		}
		if node.Kind == InputInt && node.IntMin > node.IntMax {
			return fmt.Errorf("%w: input node %d has an empty integer range", ErrInvalidPlan, index)
		}
	case InputList, InputMap, InputOptional:
		if node.Elem < 0 || node.Elem >= index || node.Fields != nil {
			return fmt.Errorf("%w: input node %d has an invalid element", ErrInvalidPlan, index)
		}
		if node.Kind == InputOptional && nodes[node.Elem].Kind == InputOptional {
			return fmt.Errorf("%w: input node %d nests optional values", ErrInvalidPlan, index)
		}
	case InputStruct:
		return validateInputStruct(index, node)
	default:
		return fmt.Errorf("%w: input node %d has unknown kind %q", ErrInvalidPlan, index, node.Kind)
	}
	return nil
}

// validateInputStruct checks struct member names, uniqueness, and back-references.
func validateInputStruct(index int, node InputNode) error {
	if node.Elem != 0 {
		return fmt.Errorf("%w: struct input node %d has an element", ErrInvalidPlan, index)
	}
	seen := make(map[string]struct{}, len(node.Fields))
	for _, field := range node.Fields {
		if !validJSONName(field.Name) {
			return fmt.Errorf("%w: input field %q is not a valid JSON field name", ErrInvalidPlan, field.Name)
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return fmt.Errorf("%w: duplicate input name %q", ErrInvalidPlan, field.Name)
		}
		if field.Node < 0 || field.Node >= index {
			return fmt.Errorf("%w: input field %q has an invalid node", ErrInvalidPlan, field.Name)
		}
		seen[field.Name] = struct{}{}
	}
	return nil
}

// Bind binds keyword-only Starlark arguments against a validated schema.
//
// It returns only a fresh canonical JSON-shaped map. Explicit Starlark None and
// omission both leave an optional member absent at every struct level. Callers
// must not treat the result as the authoritative authorization map; parent
// re-binding constructs that.
func (schema InputSchema) Bind(args starlark.Tuple, kwargs []starlark.Tuple) (map[string]any, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("%w: positional arguments are not accepted", ErrInvalidArguments)
	}
	for _, keyword := range kwargs {
		if len(keyword) != keywordTupleSize {
			return nil, fmt.Errorf("%w: malformed keyword argument", ErrInvalidArguments)
		}
	}
	return schema.bindStruct(schema.Nodes[schema.Root], kwargs, "")
}

// bindStruct binds name/value pairs against a struct node's declared fields.
func (schema InputSchema) bindStruct(node InputNode, entries []starlark.Tuple, path string) (map[string]any, error) {
	converted := make(map[string]any, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name, ok := entry[0].(starlark.String)
		if !ok && path == "" {
			return nil, fmt.Errorf("%w: keyword name must be a string", ErrInvalidArguments)
		}
		if !ok {
			return nil, fmt.Errorf("%w: argument %q keys must be strings", ErrInvalidArguments, path)
		}
		member := memberPath(path, string(name))
		field, known := lookupInputField(node, string(name))
		if !known {
			return nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidArguments, member)
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate argument %q", ErrInvalidArguments, member)
		}
		seen[field.Name] = struct{}{}
		if schema.Nodes[field.Node].Kind == InputOptional && entry[1] == starlark.None {
			continue
		}
		value, err := schema.bindValue(field.Node, entry[1], member, false)
		if err != nil {
			return nil, err
		}
		converted[field.Name] = value
	}
	for _, field := range node.Fields {
		if schema.Nodes[field.Node].Kind == InputOptional {
			continue
		}
		if _, ok := seen[field.Name]; !ok {
			return nil, fmt.Errorf(
				"%w: missing required argument %q",
				ErrInvalidArguments,
				memberPath(path, field.Name),
			)
		}
	}
	return converted, nil
}

// bindValue binds one Starlark value against the node at index.
//
// nullable reports that an enclosing optional node also accepts None, which
// only changes the type-mismatch wording.
func (schema InputSchema) bindValue(index int, value starlark.Value, path string, nullable bool) (any, error) {
	node := schema.Nodes[index]
	switch node.Kind {
	case InputString:
		if text, ok := value.(starlark.String); ok {
			return string(text), nil
		}
	case InputInt:
		if integer, ok := value.(starlark.Int); ok {
			converted, fits := integer.Int64()
			if !fits {
				return nil, integerRangeError(node, path)
			}
			return checkInteger(node, converted, path)
		}
	case InputBool:
		if boolean, ok := value.(starlark.Bool); ok {
			return bool(boolean), nil
		}
	case InputFloat:
		switch number := value.(type) {
		case starlark.Float:
			return checkFloat(node, float64(number), path)
		case starlark.Int:
			return checkFloat(node, float64(number.Float()), path)
		}
	case InputOptional:
		if value == starlark.None {
			return nil, nil //nolint:nilnil // None maps to JSON null inside lists and dicts.
		}
		return schema.bindValue(node.Elem, value, path, true)
	case InputList:
		return schema.bindList(node, value, path, nullable)
	case InputMap:
		return schema.bindMap(node, value, path, nullable)
	case InputStruct:
		if dict, ok := value.(*starlark.Dict); ok {
			return schema.bindStruct(node, dict.Items(), path)
		}
	}
	return nil, typeError(node.Kind, path, nullable)
}

// bindList binds a Starlark list or tuple element by element.
func (schema InputSchema) bindList(node InputNode, value starlark.Value, path string, nullable bool) (any, error) {
	var sequence starlark.Indexable
	switch typed := value.(type) {
	case *starlark.List:
		sequence = typed
	case starlark.Tuple:
		sequence = typed
	default:
		return nil, typeError(InputList, path, nullable)
	}
	converted := make([]any, sequence.Len())
	for index := range converted {
		element, err := schema.bindValue(node.Elem, sequence.Index(index), indexPath(path, index), false)
		if err != nil {
			return nil, err
		}
		converted[index] = element
	}
	return converted, nil
}

// bindMap binds a str-keyed Starlark dict value by value.
func (schema InputSchema) bindMap(node InputNode, value starlark.Value, path string, nullable bool) (any, error) {
	dict, ok := value.(*starlark.Dict)
	if !ok {
		return nil, typeError(InputMap, path, nullable)
	}
	converted := make(map[string]any, dict.Len())
	for _, item := range dict.Items() {
		key, isString := item[0].(starlark.String)
		if !isString {
			return nil, fmt.Errorf("%w: argument %q keys must be strings", ErrInvalidArguments, path)
		}
		element, err := schema.bindValue(node.Elem, item[1], path+keySegment(string(key)), false)
		if err != nil {
			return nil, err
		}
		converted[string(key)] = element
	}
	return converted, nil
}

// checkInteger enforces an int node's inclusive range.
func checkInteger(node InputNode, value int64, path string) (int64, error) {
	if value < node.IntMin || value > node.IntMax {
		return 0, integerRangeError(node, path)
	}
	return value, nil
}

// integerRangeError describes an integer outside the node's range.
func integerRangeError(node InputNode, path string) error {
	if node.IntMin == math.MinInt64 && node.IntMax == math.MaxInt64 {
		return fmt.Errorf("%w: argument %q overflows int64", ErrInvalidArguments, path)
	}
	return fmt.Errorf("%w: argument %q must be between %d and %d", ErrInvalidArguments, path, node.IntMin, node.IntMax)
}

// checkFloat enforces finiteness and the float32 range when required.
func checkFloat(node InputNode, value float64, path string) (float64, error) {
	if !isFiniteFloat(value) {
		return 0, fmt.Errorf("%w: argument %q is not finite", ErrInvalidArguments, path)
	}
	if node.Float32 && math.Abs(value) > math.MaxFloat32 {
		return 0, fmt.Errorf("%w: argument %q overflows float32", ErrInvalidArguments, path)
	}
	return value, nil
}

// typeError describes the value a node expected.
func typeError(kind InputKind, path string, nullable bool) error {
	if nullable {
		return fmt.Errorf("%w: argument %q must be %s or None", ErrInvalidArguments, path, expectedValue(kind))
	}
	return fmt.Errorf("%w: argument %q must be %s", ErrInvalidArguments, path, expectedValue(kind))
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

// lookupInputField finds name among a struct node's fields.
func lookupInputField(node InputNode, name string) (InputField, bool) {
	for _, field := range node.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return InputField{}, false
}
