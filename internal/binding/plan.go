package binding

import (
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

// fieldKind identifies one supported direct conversion.
type fieldKind uint8

const (
	fieldString fieldKind = iota + 1
	fieldOptionalInt64
	fieldInt64
	fieldBool
	fieldFloat64
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

// outputField is one immutable output-field conversion step.
type outputField struct {
	// name is the string key returned to Starlark.
	name string

	// index is the direct field index in the non-embedded output struct.
	index int

	// kind determines direct conversion behavior.
	kind fieldKind
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

	// outputFields preserves declaration order for deterministic conversion.
	outputFields []outputField
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
	outputFields, err := compileOutput(outputType)
	if err != nil {
		return nil, err
	}
	return &Plan{
		inputType:    inputType,
		outputType:   outputType,
		inputFields:  inputFields,
		inputByName:  inputByName,
		outputFields: outputFields,
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
		switch {
		case field.Type.Kind() == reflect.String:
			if options.omitempty {
				return nil, nil, fmt.Errorf("%w: required input %q cannot use omitempty", ErrInvalidPlan, name)
			}
			compiled.kind = fieldString
			compiled.required = true
		case field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Int64:
			compiled.kind = fieldOptionalInt64
		default:
			return nil, nil, fmt.Errorf("%w: input field %q has unsupported type %s", ErrInvalidPlan, name, field.Type)
		}
		byName[name] = len(fields)
		fields = append(fields, compiled)
	}
	return fields, byName, nil
}

// compileOutput validates and compiles the restricted output struct.
func compileOutput(outputType reflect.Type) ([]outputField, error) {
	if outputType == nil || outputType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: output must be a non-pointer struct", ErrInvalidPlan)
	}
	fields := make([]outputField, 0, outputType.NumField())
	seen := make(map[string]struct{}, outputType.NumField())
	for index := range outputType.NumField() {
		field := outputType.Field(index)
		name, options, err := compileFieldName(field)
		if err != nil {
			return nil, fmt.Errorf("%w: output field %s: %w", ErrInvalidPlan, field.Name, err)
		}
		if options.omitempty {
			return nil, fmt.Errorf("%w: output field %q cannot use omitempty", ErrInvalidPlan, name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("%w: duplicate output name %q", ErrInvalidPlan, name)
		}

		compiled := outputField{name: name, index: index}
		switch field.Type.Kind() { //nolint:exhaustive // Unsupported reflect kinds share registration-time rejection.
		case reflect.String:
			compiled.kind = fieldString
		case reflect.Int64:
			compiled.kind = fieldInt64
		case reflect.Bool:
			compiled.kind = fieldBool
		case reflect.Float64:
			compiled.kind = fieldFloat64
		default:
			return nil, fmt.Errorf("%w: output field %q has unsupported type %s", ErrInvalidPlan, name, field.Type)
		}
		seen[name] = struct{}{}
		fields = append(fields, compiled)
	}
	return fields, nil
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
