package binding

import (
	"strings"
)

const (
	// stringType is the compact Starlark-facing notation for a string value.
	stringType = "str"

	// integerType is the compact Starlark-facing notation for an integer of any width.
	integerType = "int"

	// boolType is the compact Starlark-facing notation for a Boolean value.
	boolType = "bool"

	// floatType is the compact Starlark-facing notation for a finite floating-point value.
	floatType = "float"

	// noneSuffix is the nullable notation suffix for optional values.
	noneSuffix = " | None"
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
	fields := plan.inputNodes[plan.inputSchema.Root].fields
	if len(fields) > 0 {
		signature.WriteString("*, ")
		for index, field := range fields {
			if index > 0 {
				signature.WriteString(", ")
			}
			signature.WriteString(field.name)
			signature.WriteString(": ")
			signature.WriteString(plan.inputNodes[field.node].notation)
		}
	}
	signature.WriteByte(')')
	return signature.String()
}

// InputShape returns a fresh model-facing description of the root input fields.
//
// Type carries the full nested notation; Required is false only for pointer fields.
func (plan *Plan) InputShape() []FieldShape {
	fields := plan.inputNodes[plan.inputSchema.Root].fields
	shape := make([]FieldShape, len(fields))
	for index, field := range fields {
		node := plan.inputNodes[field.node]
		shape[index] = FieldShape{
			Name:     field.name,
			Type:     node.notation,
			Required: node.kind != nodePointer,
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
func outputFieldShape(plan *Plan, field structField) FieldShape {
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
