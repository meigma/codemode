package binding

import "strings"

const unsupportedTypeSignature = "unsupported"

// FieldShape is one model-facing field in a supported capability input or output structure.
type FieldShape struct {
	// Name is the field's exact Starlark and JSON name.
	Name string `json:"name"`

	// Type is the compact Starlark-facing value notation.
	Type string `json:"type"`

	// Required reports whether the caller or handler must provide the field.
	Required bool `json:"required"`
}

// Signature renders the compact keyword-only model documentation from the immutable plan.
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
	signature.WriteString(") -> ")
	if plan.outputType.Name() == "" {
		signature.WriteString("object")
	} else {
		signature.WriteString(plan.outputType.Name())
	}
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

// inputKindSignature returns the model-facing notation for one supported input conversion.
func inputKindSignature(kind fieldKind) string {
	switch kind {
	case fieldString:
		return "str"
	case fieldOptionalInt64:
		return "int | None"
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
