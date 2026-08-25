package binding

import (
	"fmt"
	"strings"
)

const (
	unsupportedTypeSignature = "unsupported"

	// requiredStringType is the exact InputShape notation for a required string.
	requiredStringType = "str"

	// optionalIntegerType is the exact InputShape notation for an optional integer.
	optionalIntegerType = "int | None"
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
	shape := make([]FieldShape, len(plan.outputFields))
	for index, field := range plan.outputFields {
		shape[index] = FieldShape{
			Name:     field.name,
			Type:     outputKindSignature(field.kind),
			Required: true,
		}
	}
	return shape
}

// ValidateInputShape reports whether fields is a combination Plan.InputShape can produce.
//
// A required string is exactly Type "str" and Required true. An optional integer
// is exactly Type "int | None" and Required false. An empty shape is valid.
// Names must be unique Starlark identifiers. Any other pair, including the
// literal "unsupported" notation, is rejected.
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
	return isRequiredStringShape(field) || isOptionalIntegerShape(field)
}

// isRequiredStringShape reports whether field is a required string descriptor.
func isRequiredStringShape(field FieldShape) bool {
	return field.Type == requiredStringType && field.Required
}

// isOptionalIntegerShape reports whether field is an optional integer descriptor.
func isOptionalIntegerShape(field FieldShape) bool {
	return field.Type == optionalIntegerType && !field.Required
}

// inputKindSignature returns the model-facing notation for one supported input conversion.
func inputKindSignature(kind fieldKind) string {
	switch kind {
	case fieldString:
		return requiredStringType
	case fieldOptionalInt64:
		return optionalIntegerType
	case fieldInt64, fieldBool, fieldFloat64:
		return unsupportedTypeSignature
	}
	return unsupportedTypeSignature
}

// outputKindSignature returns the model-facing notation for one supported output conversion.
func outputKindSignature(kind fieldKind) string {
	switch kind {
	case fieldString:
		return "str"
	case fieldInt64:
		return "int"
	case fieldBool:
		return "bool"
	case fieldFloat64:
		return "float"
	case fieldOptionalInt64:
		return unsupportedTypeSignature
	}
	return unsupportedTypeSignature
}
