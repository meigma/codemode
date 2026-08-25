package binding

import (
	"fmt"
	"strings"
)

const (
	unsupportedTypeSignature = "unsupported"

	// stringType is the compact Starlark-facing notation for a string value.
	stringType = "str"

	// integerType is the compact Starlark-facing notation for a signed integer.
	integerType = "int"

	// boolType is the compact Starlark-facing notation for a Boolean value.
	boolType = "bool"

	// floatType is the compact Starlark-facing notation for a finite floating-point value.
	floatType = "float"

	// noneSuffix is the exact InputShape suffix for an optional scalar.
	noneSuffix = " | None"

	// optionalStringType is the exact InputShape notation for an optional string.
	optionalStringType = stringType + noneSuffix

	// optionalIntegerType is the exact InputShape notation for an optional integer.
	optionalIntegerType = integerType + noneSuffix

	// optionalBoolType is the exact InputShape notation for an optional Boolean.
	optionalBoolType = boolType + noneSuffix

	// optionalFloatType is the exact InputShape notation for an optional float.
	optionalFloatType = floatType + noneSuffix
)

// FieldShape is one model-facing field in a supported capability input or output structure.
type FieldShape struct {
	// Name is the field's exact Starlark and JSON name.
	Name string `json:"name"`

	// Type is the compact Starlark-facing value notation.
	Type string `json:"type"`

	// Required reports whether the caller or handler must provide the field.
	Required bool `json:"required"`
}

// Signature renders the compact keyword-only invocation form, ending at the closing parenthesis.
func (plan *Plan) Signature(capabilityName string) string {
	var signature strings.Builder
	signature.WriteString(capabilityName)
	signature.WriteByte('(')
	if len(plan.inputFields) > 0 {
		signature.WriteString("*, ")
		for index, field := range plan.inputFields {
			if index > 0 {
				signature.WriteString(", ")
			}
			signature.WriteString(field.name)
			signature.WriteString(": ")
			signature.WriteString(inputKindSignature(field.kind))
		}
	}
	signature.WriteByte(')')
	return signature.String()
}

// InputShape returns a fresh model-facing description of the compiled input fields.
func (plan *Plan) InputShape() []FieldShape {
	shape := make([]FieldShape, len(plan.inputFields))
	for index, field := range plan.inputFields {
		shape[index] = FieldShape{
			Name:     field.name,
			Type:     inputKindSignature(field.kind),
			Required: field.required,
		}
	}
	return shape
}

// OutputShape returns a fresh model-facing description of the compiled output fields.
func (plan *Plan) OutputShape() []FieldShape {
	root := plan.outputNodes[plan.outputRoot]
	shape := make([]FieldShape, len(root.fields))
	for index, field := range root.fields {
		shape[index] = outputFieldShape(plan, field)
	}
	return shape
}

// outputFieldShape renders one root field's flat discovery descriptor.
func outputFieldShape(plan *Plan, field outputStructField) FieldShape {
	if field.omitempty {
		return FieldShape{
			Name:     field.name,
			Type:     plan.outputNodes[plan.outputNodes[field.node].elem].notation,
			Required: false,
		}
	}
	return FieldShape{
		Name:     field.name,
		Type:     plan.outputNodes[field.node].notation,
		Required: true,
	}
}

// ValidateInputShape reports whether fields is a combination Plan.InputShape can produce.
//
// Required scalars are exactly Type "str", "int", "bool", or "float" with
// Required true. Optional scalars are exactly those types with " | None" and
// Required false. An empty shape is valid. Names must be unique Starlark
// identifiers. Any other pair, including the literal "unsupported" notation, is
// rejected.
func ValidateInputShape(fields []FieldShape) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !ValidIdentifier(field.Name) {
			return fmt.Errorf("%w: input field %q is not a Starlark identifier", ErrInvalidPlan, field.Name)
		}
		if _, exists := seen[field.Name]; exists {
			return fmt.Errorf("%w: duplicate input name %q", ErrInvalidPlan, field.Name)
		}
		if !isSupportedInputShape(field) {
			return fmt.Errorf("%w: input field %q has an unsupported shape", ErrInvalidPlan, field.Name)
		}
		seen[field.Name] = struct{}{}
	}
	return nil
}

// isSupportedInputShape reports whether field is an exact compiled input pair.
func isSupportedInputShape(field FieldShape) bool {
	return isRequiredStringShape(field) ||
		isRequiredIntegerShape(field) ||
		isRequiredBoolShape(field) ||
		isRequiredFloatShape(field) ||
		isOptionalStringShape(field) ||
		isOptionalIntegerShape(field) ||
		isOptionalBoolShape(field) ||
		isOptionalFloatShape(field)
}

// isRequiredStringShape reports whether field is a required string descriptor.
func isRequiredStringShape(field FieldShape) bool {
	return field.Type == stringType && field.Required
}

// isRequiredIntegerShape reports whether field is a required integer descriptor.
func isRequiredIntegerShape(field FieldShape) bool {
	return field.Type == integerType && field.Required
}

// isRequiredBoolShape reports whether field is a required Boolean descriptor.
func isRequiredBoolShape(field FieldShape) bool {
	return field.Type == boolType && field.Required
}

// isRequiredFloatShape reports whether field is a required float descriptor.
func isRequiredFloatShape(field FieldShape) bool {
	return field.Type == floatType && field.Required
}

// isOptionalStringShape reports whether field is an optional string descriptor.
func isOptionalStringShape(field FieldShape) bool {
	return field.Type == optionalStringType && !field.Required
}

// isOptionalIntegerShape reports whether field is an optional integer descriptor.
func isOptionalIntegerShape(field FieldShape) bool {
	return field.Type == optionalIntegerType && !field.Required
}

// isOptionalBoolShape reports whether field is an optional Boolean descriptor.
func isOptionalBoolShape(field FieldShape) bool {
	return field.Type == optionalBoolType && !field.Required
}

// isOptionalFloatShape reports whether field is an optional float descriptor.
func isOptionalFloatShape(field FieldShape) bool {
	return field.Type == optionalFloatType && !field.Required
}

// inputKindSignature returns the model-facing notation for one supported input conversion.
func inputKindSignature(kind fieldKind) string {
	switch kind {
	case fieldString:
		return stringType
	case fieldInt64:
		return integerType
	case fieldBool:
		return boolType
	case fieldFloat64:
		return floatType
	case fieldOptionalString:
		return optionalStringType
	case fieldOptionalInt64:
		return optionalIntegerType
	case fieldOptionalBool:
		return optionalBoolType
	case fieldOptionalFloat64:
		return optionalFloatType
	}
	return unsupportedTypeSignature
}
