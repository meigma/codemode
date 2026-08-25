package binding

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ExportedOutput is a named exported result structure used only in signature tests.
type ExportedOutput struct {
	// Name is a string output field.
	Name string `json:"name"`
}

// unexportedOutput is a named unexported result structure used only in signature tests.
type unexportedOutput struct {
	// Name is a string output field.
	Name string `json:"name"`
}

// TestSignatureUsesTheCompiledInputPlan proves documentation cannot drift from binding behavior.
func TestSignatureUsesTheCompiledInputPlan(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	signature := plan.Signature("records.lookup")

	assert.Equal(t, "records.lookup(*, org: str, limit: int | None)", signature)
}

// TestSignatureUsesWidenedScalarInputs proves all eight compiled input forms appear in declaration order.
func TestSignatureUsesWidenedScalarInputs(t *testing.T) {
	plan, err := CompileFor[widenedInput, representativeOutput]()
	require.NoError(t, err)

	assert.Equal(
		t,
		"records.lookup(*, org: str, count: int, active: bool, score: float, label: str | None, limit: int | None, enabled: bool | None, weight: float | None)",
		plan.Signature("records.lookup"),
	)
	assert.Equal(t, []FieldShape{
		{Name: "org", Type: "str", Required: true},
		{Name: "count", Type: "int", Required: true},
		{Name: "active", Type: "bool", Required: true},
		{Name: "score", Type: "float", Required: true},
		{Name: "label", Type: "str | None", Required: false},
		{Name: "limit", Type: "int | None", Required: false},
		{Name: "enabled", Type: "bool | None", Required: false},
		{Name: "weight", Type: "float | None", Required: false},
	}, plan.InputShape())
}

// TestSignatureHandlesAnEmptyInputStruct proves zero-argument capabilities omit the keyword-only marker.
func TestSignatureHandlesAnEmptyInputStruct(t *testing.T) {
	plan, err := CompileFor[struct{}, representativeOutput]()
	require.NoError(t, err)

	assert.Equal(t, "records.status()", plan.Signature("records.status"))
}

// TestSignatureOmitsHostOutputTypeNames proves export status cannot change the invocation-only signature.
func TestSignatureOmitsHostOutputTypeNames(t *testing.T) {
	const want = "records.lookup(*, org: str, limit: int | None)"

	tests := []struct {
		// name identifies the output type export status.
		name string

		// outputType is compiled as the capability result type.
		outputType reflect.Type

		// outputName is the host Go identifier that must stay out of the signature.
		outputName string
	}{
		{name: "exported output", outputType: reflect.TypeFor[ExportedOutput](), outputName: "ExportedOutput"},
		{name: "unexported output", outputType: reflect.TypeFor[unexportedOutput](), outputName: "unexportedOutput"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Compile(reflect.TypeFor[representativeInput](), tt.outputType)
			require.NoError(t, err)

			signature := plan.Signature("records.lookup")

			assert.Equal(t, want, signature)
			assert.Equal(t, tt.outputName, plan.OutputType().Name())
			assert.NotContains(t, signature, tt.outputName)
			assert.NotContains(t, signature, "->")
		})
	}
}

// TestValidateInputShapeAcceptsCompiledAndEmptyShapes proves only InputShape combinations are valid.
func TestValidateInputShapeAcceptsCompiledAndEmptyShapes(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	empty, err := CompileFor[struct{}, representativeOutput]()
	require.NoError(t, err)

	tests := []struct {
		// name identifies the accepted descriptor.
		name string

		// fields is the candidate input shape.
		fields []FieldShape
	}{
		{name: "compiled representative", fields: plan.InputShape()},
		{name: "empty compiled shape", fields: empty.InputShape()},
		{name: "nil empty shape", fields: nil},
		{
			name: "required string",
			fields: []FieldShape{{
				Name:     "org",
				Type:     "str",
				Required: true,
			}},
		},
		{
			name: "required integer",
			fields: []FieldShape{{
				Name:     "count",
				Type:     "int",
				Required: true,
			}},
		},
		{
			name: "required bool",
			fields: []FieldShape{{
				Name:     "active",
				Type:     "bool",
				Required: true,
			}},
		},
		{
			name: "required float",
			fields: []FieldShape{{
				Name:     "score",
				Type:     "float",
				Required: true,
			}},
		},
		{
			name: "optional string",
			fields: []FieldShape{{
				Name:     "label",
				Type:     "str | None",
				Required: false,
			}},
		},
		{
			name: "optional integer",
			fields: []FieldShape{{
				Name:     "limit",
				Type:     "int | None",
				Required: false,
			}},
		},
		{
			name: "optional bool",
			fields: []FieldShape{{
				Name:     "enabled",
				Type:     "bool | None",
				Required: false,
			}},
		},
		{
			name: "optional float",
			fields: []FieldShape{{
				Name:     "weight",
				Type:     "float | None",
				Required: false,
			}},
		},
		{name: "compiled widened", fields: mustCompileWidenedInputShape(t)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, ValidateInputShape(tt.fields))
		})
	}
}

// TestValidateInputShapeRejectsNonCompiledPairs proves the child cannot invent another descriptor matrix.
func TestValidateInputShapeRejectsNonCompiledPairs(t *testing.T) {
	tests := []struct {
		// name identifies the rejected descriptor.
		name string

		// fields is the invalid input shape.
		fields []FieldShape

		// contains is the expected diagnostic fragment.
		contains string
	}{
		{
			name: "optional string Type/Required mismatch",
			fields: []FieldShape{{
				Name:     "org",
				Type:     "str",
				Required: false,
			}},
			contains: "unsupported shape",
		},
		{
			name: "required optional-integer Type/Required mismatch",
			fields: []FieldShape{{
				Name:     "limit",
				Type:     "int | None",
				Required: true,
			}},
			contains: "unsupported shape",
		},
		{
			name: "required bool marked optional",
			fields: []FieldShape{{
				Name:     "active",
				Type:     "bool",
				Required: false,
			}},
			contains: "unsupported shape",
		},
		{
			name: "optional float marked required",
			fields: []FieldShape{{
				Name:     "weight",
				Type:     "float | None",
				Required: true,
			}},
			contains: "unsupported shape",
		},
		{
			name: "unsupported notation",
			fields: []FieldShape{{
				Name:     "value",
				Type:     "unsupported",
				Required: true,
			}},
			contains: "unsupported shape",
		},
		{
			name: "duplicate name",
			fields: []FieldShape{
				{Name: "org", Type: "str", Required: true},
				{Name: "org", Type: "int | None", Required: false},
			},
			contains: "duplicate input name",
		},
		{
			name: "invalid identifier",
			fields: []FieldShape{{
				Name:     "not-valid",
				Type:     "str",
				Required: true,
			}},
			contains: "Starlark identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputShape(tt.fields)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidPlan)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// mustCompileWidenedInputShape returns the compiled eight-form input shape.
func mustCompileWidenedInputShape(t *testing.T) []FieldShape {
	t.Helper()
	plan, err := CompileFor[widenedInput, representativeOutput]()
	require.NoError(t, err)
	return plan.InputShape()
}
