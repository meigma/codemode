package binding

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// representativeInput is the restricted MVP input shape.
type representativeInput struct {
	// Org is the required string argument.
	Org string `json:"org"`

	// Limit is the optional signed integer argument.
	Limit *int64 `json:"limit,omitempty"`
}

// representativeOutput is a simple supported result structure.
type representativeOutput struct {
	// Name is a string output field.
	Name string `json:"name"`

	// Count is a signed integer output field.
	Count int64 `json:"count"`

	// Active is a Boolean output field.
	Active bool `json:"active"`

	// Score is a finite floating-point output field.
	Score float64 `json:"score"`
}

// embeddedInputBase supplies a field that must not be promoted implicitly.
type embeddedInputBase struct {
	// Value is an embedded candidate argument.
	Value string `json:"value"`
}

// embeddedInput contains an unsupported anonymous field.
type embeddedInput struct {
	// embeddedInputBase is intentionally anonymous to exercise rejection.
	embeddedInputBase
}

// TestCompileAcceptsRepresentativeTypes proves the restricted MVP shape compiles once.
func TestCompileAcceptsRepresentativeTypes(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()

	require.NoError(t, err)
	assert.Equal(t, reflect.TypeFor[representativeInput](), plan.InputType())
	assert.Equal(t, reflect.TypeFor[representativeOutput](), plan.OutputType())
}

// TestCompileRejectsUnsupportedInputShapes proves malformed binding plans fail during registration.
func TestCompileRejectsUnsupportedInputShapes(t *testing.T) {
	tests := []struct {
		// name identifies the invalid input shape.
		name string

		// inputType is compiled as a capability input.
		inputType reflect.Type

		// contains is the expected diagnostic fragment.
		contains string
	}{
		{name: "pointer input", inputType: reflect.TypeFor[*representativeInput](), contains: "non-pointer struct"},
		{name: "required integer", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value int64 `json:"value"`
		}{}), contains: "unsupported type"},
		{name: "wrong optional pointer", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value *string `json:"value,omitempty"`
		}{}), contains: "unsupported type"},
		{name: "embedded field", inputType: reflect.TypeFor[embeddedInput](), contains: "embedded fields"},
		{name: "unexported field", inputType: reflect.TypeOf(struct {
			// value is intentionally unexported.
			value string
		}{}), contains: "must be exported"},
		{name: "duplicate JSON name", inputType: duplicateJSONNameType(), contains: "duplicate input name"},
		{name: "invalid identifier", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"not-valid"`
		}{}), contains: "Starlark identifier"},
		{name: "reserved identifier", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"for"`
		}{}), contains: "Starlark identifier"},
		{name: "future reserved identifier", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"from"`
		}{}), contains: "Starlark identifier"},
		{name: "non-ASCII digit", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"a١"`
		}{}), contains: "Starlark identifier"},
		{name: "ignored field", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"-"`
		}{}), contains: "ignored JSON fields"},
		{name: "unknown tag option", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"value,string"`
		}{}), contains: "unsupported JSON tag option"},
		{name: "unknown struct tag", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"value" xml:"value"`
		}{}), contains: "only one json struct tag"},
		{name: "required omitempty", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"value,omitempty"`
		}{}), contains: "cannot use omitempty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.inputType, reflect.TypeFor[representativeOutput]())

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidPlan)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestCompileRejectsUnsupportedOutputShapes proves outputs cannot smuggle pointers or unsupported kinds.
func TestCompileRejectsUnsupportedOutputShapes(t *testing.T) {
	tests := []struct {
		// name identifies the invalid output shape.
		name string

		// outputType is compiled as a capability output.
		outputType reflect.Type

		// contains is the expected diagnostic fragment.
		contains string
	}{
		{name: "pointer output", outputType: reflect.TypeFor[*representativeOutput](), contains: "non-pointer struct"},
		{name: "pointer field", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value *int64 `json:"value"`
		}{}), contains: "unsupported type"},
		{name: "output omitempty", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"value,omitempty"`
		}{}), contains: "cannot use omitempty"},
		{name: "duplicate JSON name", outputType: duplicateJSONNameType(), contains: "duplicate output name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(reflect.TypeFor[representativeInput](), tt.outputType)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidPlan)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// duplicateJSONNameType constructs a type that intentionally violates unique JSON field naming.
func duplicateJSONNameType() reflect.Type {
	stringType := reflect.TypeFor[string]()
	return reflect.StructOf([]reflect.StructField{
		{Name: "First", Type: stringType, Tag: `json:"value"`},
		{Name: "Second", Type: stringType, Tag: `json:"value"`},
	})
}
