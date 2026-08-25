package binding

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

var (
	// ErrInvalidPlan classifies an input or output type that cannot form a restricted binding plan.
	ErrInvalidPlan = errors.New("invalid binding plan")

	// ErrInvalidArguments classifies Starlark arguments rejected before authorization.
	ErrInvalidArguments = errors.New("invalid capability arguments")

	// ErrUnsupportedValue classifies a Go or Starlark value outside the supported conversion surface.
	ErrUnsupportedValue = errors.New("unsupported value")

	// ErrValueLimit classifies converted output that exceeds a configured depth or byte limit.
	ErrValueLimit = errors.New("converted value limit exceeded")
)

// outputArenaHint is the initial compiled-output arena capacity for typical capability graphs.
const outputArenaHint = 8

// fieldKind identifies one supported input conversion.
type fieldKind uint8

const (
	fieldString fieldKind = iota + 1
	fieldInt64
	fieldBool
	fieldFloat64
	fieldOptionalString
	fieldOptionalInt64
	fieldOptionalBool
	fieldOptionalFloat64
)

// outputNodeKind identifies one compiled output conversion node.
type outputNodeKind uint8

const (
	outputNodeString outputNodeKind = iota + 1
	outputNodeInt
	outputNodeUint
	outputNodeBool
	outputNodeFloat
	outputNodeBytes
	outputNodeList
	outputNodeMap
	outputNodeStruct
	outputNodePointer
)

// inputField is one immutable input-field conversion step.
type inputField struct {
	// name is the Starlark keyword and canonical authorization key.
	name string

	// index is the direct field index in the non-embedded input struct.
	index int

	// kind determines validation and direct conversion behavior.
	kind fieldKind

	// required reports whether omission is invalid.
	required bool
}

// outputNode is one immutable compiled output conversion node.
type outputNode struct {
	// kind selects the conversion and notation strategy.
	kind outputNodeKind

	// elem is the child node index for pointers, lists, and maps.
	elem int

	// fields are declaration-ordered struct members.
	fields []outputStructField

	// notation is the exact model-facing type string for this node.
	notation string
}

// outputStructField is one compiled exported struct member.
type outputStructField struct {
	// name is the JSON and Starlark field name.
	name string

	// index is the direct field index on the struct type.
	index int

	// node is the compiled field type.
	node int

	// omitempty omits a nil pointer from the converted object.
	omitempty bool
}

// outputCompiler builds a flat immutable node arena with cycle detection.
type outputCompiler struct {
	// nodes is the arena under construction.
	nodes []outputNode

	// done maps a completed type onto its arena index.
	done map[reflect.Type]int

	// active is the stack of types currently being compiled.
	active map[reflect.Type]struct{}

	// jsonMarshaler is the json.Marshaler interface type used for method-set checks.
	jsonMarshaler reflect.Type

	// textMarshaler is the encoding.TextMarshaler interface type used for method-set checks.
	textMarshaler reflect.Type
}

// Plan is an immutable compiled input, output, signature, and canonical-argument plan.
type Plan struct {
	// inputType is the exact non-pointer Go input struct type.
	inputType reflect.Type

	// outputType is the exact non-pointer Go output struct type.
	outputType reflect.Type

	// inputFields preserves declaration order for binding and signatures.
	inputFields []inputField

	// inputByName maps each accepted keyword to its field-plan index.
	inputByName map[string]int

	// outputRoot is the arena index of the compiled root struct.
	outputRoot int

	// outputNodes is the immutable compiled output type arena.
	outputNodes []outputNode
}

// CompileFor compiles the exact generic input and output types once.
func CompileFor[Input, Output any]() (*Plan, error) {
	return Compile(reflect.TypeFor[Input](), reflect.TypeFor[Output]())
}

// Compile creates an immutable restricted plan for exact input and output struct types.
func Compile(inputType reflect.Type, outputType reflect.Type) (*Plan, error) {
	inputFields, inputByName, err := compileInput(inputType)
	if err != nil {
		return nil, err
	}
	root, nodes, err := compileOutput(outputType)
	if err != nil {
		return nil, err
	}
	return &Plan{
		inputType:   inputType,
		outputType:  outputType,
		inputFields: inputFields,
		inputByName: inputByName,
		outputRoot:  root,
		outputNodes: nodes,
	}, nil
}

// InputType returns the exact Go input type compiled into the plan.
func (plan *Plan) InputType() reflect.Type {
	return plan.inputType
}

// OutputType returns the exact Go output type compiled into the plan.
func (plan *Plan) OutputType() reflect.Type {
	return plan.outputType
}

// compileInput validates and compiles the restricted input struct.
func compileInput(inputType reflect.Type) ([]inputField, map[string]int, error) {
	if inputType == nil || inputType.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("%w: input must be a non-pointer struct", ErrInvalidPlan)
	}
	fields := make([]inputField, 0, inputType.NumField())
	byName := make(map[string]int, inputType.NumField())
	for index := range inputType.NumField() {
		field := inputType.Field(index)
		name, options, err := compileFieldName(field)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: input field %s: %w", ErrInvalidPlan, field.Name, err)
		}
		if _, exists := byName[name]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate input name %q", ErrInvalidPlan, name)
		}

		compiled := inputField{name: name, index: index}
		kind, required, ok := compileInputKind(field.Type)
		if !ok {
			return nil, nil, fmt.Errorf("%w: input field %q has unsupported type %s", ErrInvalidPlan, name, field.Type)
		}
		if required && options.omitempty {
			return nil, nil, fmt.Errorf("%w: required input %q cannot use omitempty", ErrInvalidPlan, name)
		}
		compiled.kind = kind
		compiled.required = required
		byName[name] = len(fields)
		fields = append(fields, compiled)
	}
	return fields, byName, nil
}

// compileInputKind maps one input field type onto a supported scalar conversion.
//
// Named aliases are accepted by underlying kind. Pointers to the four scalar
// kinds are optional; non-pointer scalars are required.
func compileInputKind(fieldType reflect.Type) (fieldKind, bool, bool) {
	if fieldType.Kind() == reflect.Pointer {
		elementKind := fieldType.Elem().Kind()
		switch elementKind { //nolint:exhaustive // Unsupported pointer element kinds share registration-time rejection.
		case reflect.String:
			return fieldOptionalString, false, true
		case reflect.Int64:
			return fieldOptionalInt64, false, true
		case reflect.Bool:
			return fieldOptionalBool, false, true
		case reflect.Float64:
			return fieldOptionalFloat64, false, true
		default:
			return 0, false, false
		}
	}
	switch fieldType.Kind() { //nolint:exhaustive // Unsupported reflect kinds share registration-time rejection.
	case reflect.String:
		return fieldString, true, true
	case reflect.Int64:
		return fieldInt64, true, true
	case reflect.Bool:
		return fieldBool, true, true
	case reflect.Float64:
		return fieldFloat64, true, true
	default:
		return 0, false, false
	}
}

// compileOutput validates and compiles the restricted output struct into an arena.
func compileOutput(outputType reflect.Type) (int, []outputNode, error) {
	if outputType == nil || outputType.Kind() != reflect.Struct {
		return 0, nil, fmt.Errorf("%w: output must be a non-pointer struct", ErrInvalidPlan)
	}
	compiler := outputCompiler{
		nodes:         make([]outputNode, 0, outputArenaHint),
		done:          make(map[reflect.Type]int),
		active:        make(map[reflect.Type]struct{}),
		jsonMarshaler: reflect.TypeFor[json.Marshaler](),
		textMarshaler: reflect.TypeFor[encoding.TextMarshaler](),
	}
	root, err := compiler.compile(outputType, "")
	if err != nil {
		return 0, nil, err
	}
	return root, compiler.nodes, nil
}

// compile returns the arena index for typ, reusing completed nodes and rejecting cycles.
func (compiler *outputCompiler) compile(typ reflect.Type, path string) (int, error) {
	if typ == nil {
		return 0, fmt.Errorf("%w: output field %q has unsupported type <nil>", ErrInvalidPlan, path)
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
func (compiler *outputCompiler) compileNew(typ reflect.Type, path string) (int, error) {
	if compiler.implementsMarshaler(typ) {
		return 0, unsupportedOutputType(path, typ)
	}
	switch typ.Kind() { //nolint:exhaustive // Unsupported reflect kinds share registration-time rejection.
	case reflect.String:
		return compiler.append(outputNode{kind: outputNodeString, notation: stringType}), nil
	case reflect.Bool:
		return compiler.append(outputNode{kind: outputNodeBool, notation: boolType}), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return compiler.append(outputNode{kind: outputNodeInt, notation: integerType}), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return compiler.append(outputNode{kind: outputNodeUint, notation: integerType}), nil
	case reflect.Float32, reflect.Float64:
		return compiler.append(outputNode{kind: outputNodeFloat, notation: floatType}), nil
	case reflect.Pointer:
		return compiler.compilePointer(typ, path)
	case reflect.Slice:
		return compiler.compileSlice(typ, path)
	case reflect.Array:
		return compiler.compileArray(typ, path)
	case reflect.Map:
		return compiler.compileMap(typ, path)
	case reflect.Struct:
		return compiler.compileStruct(typ, path)
	default:
		return 0, unsupportedOutputType(path, typ)
	}
}

// compilePointer compiles a pointer to a supported node.
func (compiler *outputCompiler) compilePointer(typ reflect.Type, path string) (int, error) {
	elem, err := compiler.compile(typ.Elem(), path)
	if err != nil {
		return 0, err
	}
	return compiler.append(outputNode{
		kind:     outputNodePointer,
		elem:     elem,
		notation: pointerNotation(compiler.nodes[elem].notation),
	}), nil
}

// compileSlice compiles a slice, treating uint8 elements as integer byte lists.
func (compiler *outputCompiler) compileSlice(typ reflect.Type, path string) (int, error) {
	if typ.Elem().Kind() == reflect.Uint8 && !compiler.implementsMarshaler(typ.Elem()) {
		return compiler.append(outputNode{kind: outputNodeBytes, notation: listNotation(integerType)}), nil
	}
	elem, err := compiler.compile(typ.Elem(), path)
	if err != nil {
		return 0, err
	}
	return compiler.append(outputNode{
		kind:     outputNodeList,
		elem:     elem,
		notation: listNotation(compiler.nodes[elem].notation),
	}), nil
}

// compileArray compiles a fixed array, treating uint8 elements as integer byte lists.
func (compiler *outputCompiler) compileArray(typ reflect.Type, path string) (int, error) {
	if typ.Elem().Kind() == reflect.Uint8 && !compiler.implementsMarshaler(typ.Elem()) {
		return compiler.append(outputNode{kind: outputNodeBytes, notation: listNotation(integerType)}), nil
	}
	elem, err := compiler.compile(typ.Elem(), path)
	if err != nil {
		return 0, err
	}
	return compiler.append(outputNode{
		kind:     outputNodeList,
		elem:     elem,
		notation: listNotation(compiler.nodes[elem].notation),
	}), nil
}

// compileMap compiles a string-keyed map.
func (compiler *outputCompiler) compileMap(typ reflect.Type, path string) (int, error) {
	keyType := typ.Key()
	if compiler.implementsMarshaler(keyType) || keyType.Kind() != reflect.String {
		if path == "" {
			return 0, fmt.Errorf("%w: output type %s has unsupported map key type %s", ErrInvalidPlan, typ, keyType)
		}
		return 0, fmt.Errorf("%w: output field %q has unsupported map key type %s", ErrInvalidPlan, path, keyType)
	}
	elem, err := compiler.compile(typ.Elem(), path)
	if err != nil {
		return 0, err
	}
	return compiler.append(outputNode{
		kind:     outputNodeMap,
		elem:     elem,
		notation: mapNotation(compiler.nodes[elem].notation),
	}), nil
}

// compileStruct compiles exported non-embedded fields in declaration order.
func (compiler *outputCompiler) compileStruct(typ reflect.Type, path string) (int, error) {
	fields := make([]outputStructField, 0, typ.NumField())
	seen := make(map[string]struct{}, typ.NumField())
	for index := range typ.NumField() {
		field := typ.Field(index)
		name, options, err := compileFieldName(field)
		if err != nil {
			return 0, fmt.Errorf("%w: output field %s: %w", ErrInvalidPlan, outputFieldPath(path, field.Name), err)
		}
		fieldPath := outputFieldPath(path, name)
		if options.omitempty && field.Type.Kind() != reflect.Pointer {
			return 0, fmt.Errorf("%w: output field %q cannot use omitempty", ErrInvalidPlan, fieldPath)
		}
		if _, exists := seen[name]; exists {
			return 0, fmt.Errorf("%w: duplicate output name %q", ErrInvalidPlan, fieldPath)
		}
		node, err := compiler.compile(field.Type, fieldPath)
		if err != nil {
			return 0, err
		}
		seen[name] = struct{}{}
		fields = append(fields, outputStructField{
			name:      name,
			index:     index,
			node:      node,
			omitempty: options.omitempty,
		})
	}
	return compiler.append(outputNode{
		kind:     outputNodeStruct,
		fields:   fields,
		notation: compiler.structNotation(fields),
	}), nil
}

// append stores node and returns its arena index.
func (compiler *outputCompiler) append(node outputNode) int {
	compiler.nodes = append(compiler.nodes, node)
	return len(compiler.nodes) - 1
}

// structNotation renders a declaration-ordered struct literal.
func (compiler *outputCompiler) structNotation(fields []outputStructField) string {
	var notation strings.Builder
	notation.WriteByte('{')
	for index, field := range fields {
		if index > 0 {
			notation.WriteString(", ")
		}
		notation.WriteString(field.name)
		if field.omitempty {
			notation.WriteByte('?')
			notation.WriteString(": ")
			notation.WriteString(compiler.nodes[compiler.nodes[field.node].elem].notation)
			continue
		}
		notation.WriteString(": ")
		notation.WriteString(compiler.nodes[field.node].notation)
	}
	notation.WriteByte('}')
	return notation.String()
}

// implementsMarshaler reports whether typ or *typ implements a forbidden marshaler.
func (compiler *outputCompiler) implementsMarshaler(typ reflect.Type) bool {
	if typ == nil {
		return false
	}
	if typ.Implements(compiler.jsonMarshaler) || typ.Implements(compiler.textMarshaler) {
		return true
	}
	if typ.Kind() == reflect.Pointer {
		return false
	}
	pointer := reflect.PointerTo(typ)
	return pointer.Implements(compiler.jsonMarshaler) || pointer.Implements(compiler.textMarshaler)
}

// unsupportedOutputType classifies a rejected output type at path.
func unsupportedOutputType(path string, typ reflect.Type) error {
	if path == "" {
		return fmt.Errorf("%w: output type %s is unsupported", ErrInvalidPlan, typ)
	}
	return fmt.Errorf("%w: output field %q has unsupported type %s", ErrInvalidPlan, path, typ)
}

// outputFieldPath joins a parent compile path with a JSON field name.
func outputFieldPath(parent string, name string) string {
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

// tagOptions contains the supported JSON tag option state.
type tagOptions struct {
	// omitempty reports whether the field may be omitted when absent.
	omitempty bool
}

// compileFieldName validates field visibility, embedding, tags, and Starlark identifier syntax.
func compileFieldName(field reflect.StructField) (string, tagOptions, error) {
	if field.Anonymous {
		return "", tagOptions{}, errors.New("embedded fields are not supported")
	}
	if field.PkgPath != "" {
		return "", tagOptions{}, errors.New("field must be exported")
	}

	name := field.Name
	var options tagOptions
	if field.Tag != "" {
		jsonTag, ok := field.Tag.Lookup("json")
		if !ok || string(field.Tag) != fmt.Sprintf("json:%q", jsonTag) {
			return "", tagOptions{}, errors.New("only one json struct tag is supported")
		}
		parts := strings.Split(jsonTag, ",")
		if parts[0] == "-" {
			return "", tagOptions{}, errors.New("ignored JSON fields are not supported")
		}
		if parts[0] != "" {
			name = parts[0]
		}
		for _, option := range parts[1:] {
			switch option {
			case "omitempty":
				if options.omitempty {
					return "", tagOptions{}, errors.New("duplicate omitempty option")
				}
				options.omitempty = true
			case "":
				return "", tagOptions{}, errors.New("empty JSON tag option")
			default:
				return "", tagOptions{}, fmt.Errorf("unsupported JSON tag option %q", option)
			}
		}
	}
	if !ValidIdentifier(name) {
		return "", tagOptions{}, fmt.Errorf("JSON name %q is not a Starlark identifier", name)
	}
	return name, options, nil
}

// ValidIdentifier reports whether name is accepted as an identifier by the pinned Starlark scanner.
func ValidIdentifier(name string) bool {
	if name == "" || isKeyword(name) {
		return false
	}
	for index, character := range name {
		if character == '_' || unicode.IsLetter(character) ||
			index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

// isKeyword reports whether name is reserved by Starlark syntax.
func isKeyword(name string) bool {
	switch name {
	case "False", "None", "True",
		"and", "as", "async", "await", "break", "class", "continue", "def", "del", "elif", "else",
		"except", "finally", "for", "from", "global", "if", "import", "in", "is", "lambda", "load",
		"nonlocal", "not", "or", "pass", "raise", "return", "try", "while", "with", "yield":
		return true
	default:
		return false
	}
}
