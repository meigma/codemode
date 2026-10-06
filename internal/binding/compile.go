package binding

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// arenaHint is the initial compiled-arena capacity for typical capability graphs.
const arenaHint = 8

// direction selects the per-direction rules of the shared type compiler.
type direction uint8

const (
	// directionInput compiles a type that CodeMode builds from model arguments.
	directionInput direction = iota + 1

	// directionOutput compiles a type that a handler returns to the model.
	directionOutput
)

// label names the direction in registration diagnostics.
func (dir direction) label() string {
	if dir == directionInput {
		return "input"
	}
	return "output"
}

// nodeKind identifies one compiled type node.
type nodeKind uint8

const (
	nodeString nodeKind = iota + 1
	nodeInt
	nodeUint
	nodeBool
	nodeFloat
	nodeBytes
	nodeList
	nodeMap
	nodeStruct
	nodePointer
)

// typeNode is one immutable compiled type node.
type typeNode struct {
	// kind selects the conversion and notation strategy.
	kind nodeKind

	// typ is the exact Go type the node describes.
	typ reflect.Type

	// elem is the child node index for pointers, lists, and maps.
	elem int

	// fields are declaration-ordered struct members.
	fields []structField

	// notation is the exact model-facing type string for this node.
	notation string
}

// structField is one compiled exported struct member.
type structField struct {
	// name is the JSON and Starlark field name.
	name string

	// index is the direct field index on the struct type.
	index int

	// node is the compiled field type.
	node int

	// omitempty omits a nil pointer from converted output.
	omitempty bool
}

// typeCompiler builds a flat post-order node arena with cycle detection.
type typeCompiler struct {
	// dir selects input or output rules.
	dir direction

	// nodes is the arena under construction.
	nodes []typeNode

	// done maps a completed type onto its arena index.
	done map[reflect.Type]int

	// active is the stack of types currently being compiled.
	active map[reflect.Type]struct{}

	// forbidden are the custom codec interfaces this direction rejects.
	forbidden [2]reflect.Type
}

// compileType compiles root and every reachable type for dir.
//
// Children are appended before their parents, so every elem and field node
// index is lower than the index of the node that references it.
func compileType(dir direction, root reflect.Type) (int, []typeNode, error) {
	compiler := typeCompiler{
		dir:    dir,
		nodes:  make([]typeNode, 0, arenaHint),
		done:   make(map[reflect.Type]int),
		active: make(map[reflect.Type]struct{}),
	}
	if dir == directionInput {
		compiler.forbidden = [2]reflect.Type{
			reflect.TypeFor[json.Unmarshaler](),
			reflect.TypeFor[encoding.TextUnmarshaler](),
		}
	} else {
		compiler.forbidden = [2]reflect.Type{
			reflect.TypeFor[json.Marshaler](),
			reflect.TypeFor[encoding.TextMarshaler](),
		}
	}
	index, err := compiler.compile(root, "")
	if err != nil {
		return 0, nil, err
	}
	return index, compiler.nodes, nil
}

// compile returns the arena index for typ, reusing completed nodes and rejecting cycles.
func (compiler *typeCompiler) compile(typ reflect.Type, path string) (int, error) {
	if typ == nil {
		return 0, fmt.Errorf("%w: %s field %q has unsupported type <nil>", ErrInvalidPlan, compiler.dir.label(), path)
	}
	if index, ok := compiler.done[typ]; ok {
		return index, nil
	}
	if _, exists := compiler.active[typ]; exists {
		return 0, fmt.Errorf("%w: cyclic type at %q", ErrInvalidPlan, path)
	}
	compiler.active[typ] = struct{}{}
	defer delete(compiler.active, typ)

	index, err := compiler.compileNew(typ, path)
	if err != nil {
		return 0, err
	}
	compiler.done[typ] = index
	return index, nil
}

// compileNew appends one newly compiled node for typ.
func (compiler *typeCompiler) compileNew(typ reflect.Type, path string) (int, error) {
	if compiler.implementsForbidden(typ) {
		return 0, compiler.unsupported(path, typ)
	}
	switch typ.Kind() { //nolint:exhaustive // Unsupported reflect kinds share registration-time rejection.
	case reflect.String:
		return compiler.append(typeNode{kind: nodeString, typ: typ, notation: stringType}), nil
	case reflect.Bool:
		return compiler.append(typeNode{kind: nodeBool, typ: typ, notation: boolType}), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return compiler.append(typeNode{kind: nodeInt, typ: typ, notation: integerType}), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return compiler.append(typeNode{kind: nodeUint, typ: typ, notation: integerType}), nil
	case reflect.Float32, reflect.Float64:
		return compiler.append(typeNode{kind: nodeFloat, typ: typ, notation: floatType}), nil
	case reflect.Pointer:
		return compiler.compilePointer(typ, path)
	case reflect.Slice:
		return compiler.compileSequence(typ, path)
	case reflect.Array:
		if compiler.dir == directionInput {
			return 0, compiler.unsupported(path, typ)
		}
		return compiler.compileSequence(typ, path)
	case reflect.Map:
		return compiler.compileMap(typ, path)
	case reflect.Struct:
		return compiler.compileStruct(typ, path)
	default:
		return 0, compiler.unsupported(path, typ)
	}
}

// compilePointer compiles a pointer to a supported node.
//
// Inputs reject pointer-to-pointer because one None cannot select a level.
func (compiler *typeCompiler) compilePointer(typ reflect.Type, path string) (int, error) {
	if compiler.dir == directionInput && typ.Elem().Kind() == reflect.Pointer {
		return 0, compiler.unsupported(path, typ)
	}
	elem, err := compiler.compile(typ.Elem(), path)
	if err != nil {
		return 0, err
	}
	return compiler.append(typeNode{
		kind:     nodePointer,
		typ:      typ,
		elem:     elem,
		notation: pointerNotation(compiler.nodes[elem].notation),
	}), nil
}

// compileSequence compiles a slice or array.
//
// Output byte sequences use the dedicated byte conversion; input byte slices
// are ordinary lists of bounded integers.
func (compiler *typeCompiler) compileSequence(typ reflect.Type, path string) (int, error) {
	if compiler.dir == directionOutput && typ.Elem().Kind() == reflect.Uint8 &&
		!compiler.implementsForbidden(typ.Elem()) {
		return compiler.append(typeNode{kind: nodeBytes, typ: typ, notation: listNotation(integerType)}), nil
	}
	elem, err := compiler.compile(typ.Elem(), path)
	if err != nil {
		return 0, err
	}
	return compiler.append(typeNode{
		kind:     nodeList,
		typ:      typ,
		elem:     elem,
		notation: listNotation(compiler.nodes[elem].notation),
	}), nil
}

// compileMap compiles a string-keyed map.
func (compiler *typeCompiler) compileMap(typ reflect.Type, path string) (int, error) {
	keyType := typ.Key()
	if compiler.implementsForbidden(keyType) || keyType.Kind() != reflect.String {
		if path == "" {
			return 0, fmt.Errorf("%w: %s type %s has unsupported map key type %s",
				ErrInvalidPlan, compiler.dir.label(), typ, keyType)
		}
		return 0, fmt.Errorf("%w: %s field %q has unsupported map key type %s",
			ErrInvalidPlan, compiler.dir.label(), path, keyType)
	}
	elem, err := compiler.compile(typ.Elem(), path)
	if err != nil {
		return 0, err
	}
	return compiler.append(typeNode{
		kind:     nodeMap,
		typ:      typ,
		elem:     elem,
		notation: mapNotation(compiler.nodes[elem].notation),
	}), nil
}

// compileStruct compiles exported non-embedded fields in declaration order.
//
// Root input field names are Starlark keyword arguments and must be
// identifiers. Every other field name is a dict key and only needs to be a
// valid JSON field name.
func (compiler *typeCompiler) compileStruct(typ reflect.Type, path string) (int, error) {
	label := compiler.dir.label()
	keywords := compiler.dir == directionInput && path == ""
	fields := make([]structField, 0, typ.NumField())
	seen := make(map[string]struct{}, typ.NumField())
	for index := range typ.NumField() {
		field := typ.Field(index)
		name, options, err := compileFieldName(field, keywords)
		if err != nil {
			return 0, fmt.Errorf("%w: %s field %s: %w", ErrInvalidPlan, label, fieldPath(path, field.Name), err)
		}
		namePath := fieldPath(path, name)
		if options.omitempty && field.Type.Kind() != reflect.Pointer {
			if compiler.dir == directionInput {
				return 0, fmt.Errorf("%w: required input %q cannot use omitempty", ErrInvalidPlan, namePath)
			}
			return 0, fmt.Errorf("%w: output field %q cannot use omitempty", ErrInvalidPlan, namePath)
		}
		if _, exists := seen[name]; exists {
			return 0, fmt.Errorf("%w: duplicate %s name %q", ErrInvalidPlan, label, namePath)
		}
		node, err := compiler.compile(field.Type, namePath)
		if err != nil {
			return 0, err
		}
		seen[name] = struct{}{}
		fields = append(fields, structField{
			name:      name,
			index:     index,
			node:      node,
			omitempty: options.omitempty,
		})
	}
	return compiler.append(typeNode{
		kind:     nodeStruct,
		typ:      typ,
		fields:   fields,
		notation: compiler.structNotation(fields),
	}), nil
}

// append stores node and returns its arena index.
func (compiler *typeCompiler) append(node typeNode) int {
	compiler.nodes = append(compiler.nodes, node)
	return len(compiler.nodes) - 1
}

// structNotation renders a declaration-ordered struct literal.
//
// Output omitempty pointers render as "name?: T"; every other optional field
// renders its pointer notation "name: T | None".
func (compiler *typeCompiler) structNotation(fields []structField) string {
	var notation strings.Builder
	notation.WriteByte('{')
	for index, field := range fields {
		if index > 0 {
			notation.WriteString(", ")
		}
		notation.WriteString(notationName(field.name))
		if compiler.dir == directionOutput && field.omitempty {
			notation.WriteString("?: ")
			notation.WriteString(compiler.nodes[compiler.nodes[field.node].elem].notation)
			continue
		}
		notation.WriteString(": ")
		notation.WriteString(compiler.nodes[field.node].notation)
	}
	notation.WriteByte('}')
	return notation.String()
}

// implementsForbidden reports whether typ or *typ implements a rejected codec interface.
func (compiler *typeCompiler) implementsForbidden(typ reflect.Type) bool {
	for _, forbidden := range compiler.forbidden {
		if typ.Implements(forbidden) {
			return true
		}
		if typ.Kind() != reflect.Pointer && reflect.PointerTo(typ).Implements(forbidden) {
			return true
		}
	}
	return false
}

// unsupported classifies a rejected type at path.
func (compiler *typeCompiler) unsupported(path string, typ reflect.Type) error {
	if path == "" {
		return fmt.Errorf("%w: %s type %s is unsupported", ErrInvalidPlan, compiler.dir.label(), typ)
	}
	return fmt.Errorf("%w: %s field %q has unsupported type %s", ErrInvalidPlan, compiler.dir.label(), path, typ)
}

// fieldPath joins a parent compile path with a field name.
func fieldPath(parent string, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

// pointerNotation appends one nullable suffix.
func pointerNotation(elem string) string {
	if strings.HasSuffix(elem, noneSuffix) {
		return elem
	}
	return elem + noneSuffix
}

// listNotation renders list[T].
func listNotation(elem string) string {
	return "list[" + elem + "]"
}

// mapNotation renders dict[str, T].
func mapNotation(elem string) string {
	return "dict[str, " + elem + "]"
}
