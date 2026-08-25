package binding

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// representativeInput is a supported capability input used by binding tests.
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

// orgName is a named string alias accepted as a required or optional input.
type orgName string

// itemCount is a named int64 alias accepted as a required or optional input.
type itemCount int64

// flag is a named bool alias accepted as a required or optional input.
type flag bool

// score is a named float64 alias accepted as a required or optional input.
type score float64

// widenedInput is the eight-form scalar input used by widened binding tests.
type widenedInput struct {
	// Org is the required string argument.
	Org string `json:"org"`

	// Count is the required signed integer argument.
	Count int64 `json:"count"`

	// Active is the required Boolean argument.
	Active bool `json:"active"`

	// Score is the required finite floating-point argument.
	Score float64 `json:"score"`

	// Label is the optional string argument.
	Label *string `json:"label,omitempty"`

	// Limit is the optional signed integer argument.
	Limit *int64 `json:"limit,omitempty"`

	// Enabled is the optional Boolean argument.
	Enabled *bool `json:"enabled"`

	// Weight is the optional finite floating-point argument.
	Weight *float64 `json:"weight,omitempty"`
}

// aliasedInput is the eight-form scalar input using named underlying kinds.
type aliasedInput struct {
	// Org is the required named string argument.
	Org orgName `json:"org"`

	// Count is the required named integer argument.
	Count itemCount `json:"count"`

	// Active is the required named Boolean argument.
	Active flag `json:"active"`

	// Score is the required named floating-point argument.
	Score score `json:"score"`

	// Label is the optional named string argument.
	Label *orgName `json:"label,omitempty"`

	// Limit is the optional named integer argument.
	Limit *itemCount `json:"limit,omitempty"`

	// Enabled is the optional named Boolean argument.
	Enabled *flag `json:"enabled"`

	// Weight is the optional named floating-point argument.
	Weight *score `json:"weight,omitempty"`
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

// TestCompileAcceptsRepresentativeTypes proves a supported input and output compile once.
func TestCompileAcceptsRepresentativeTypes(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()

	require.NoError(t, err)
	assert.Equal(t, reflect.TypeFor[representativeInput](), plan.InputType())
	assert.Equal(t, reflect.TypeFor[representativeOutput](), plan.OutputType())
}

// TestCompileAcceptsWidenedScalarInputs proves all eight scalar forms and named aliases compile.
func TestCompileAcceptsWidenedScalarInputs(t *testing.T) {
	tests := []struct {
		// name identifies the accepted input type.
		name string

		// inputType is compiled as a capability input.
		inputType reflect.Type
	}{
		{name: "eight scalar forms", inputType: reflect.TypeFor[widenedInput]()},
		{name: "named aliases", inputType: reflect.TypeFor[aliasedInput]()},
		{name: "required integer", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value int64 `json:"value"`
		}{})},
		{name: "required bool", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value bool `json:"value"`
		}{})},
		{name: "required float", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value float64 `json:"value"`
		}{})},
		{name: "optional string without omitempty", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value *string `json:"value"`
		}{})},
		{name: "optional integer without omitempty", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value *int64 `json:"value"`
		}{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Compile(tt.inputType, reflect.TypeFor[representativeOutput]())

			require.NoError(t, err)
			assert.Equal(t, tt.inputType, plan.InputType())
		})
	}
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
		{name: "unsupported integer width", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value int32 `json:"value"`
		}{}), contains: "unsupported type"},
		{name: "unsupported unsigned integer", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value uint64 `json:"value"`
		}{}), contains: "unsupported type"},
		{name: "unsupported optional integer width", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value *int `json:"value,omitempty"`
		}{}), contains: "unsupported type"},
		{name: "unsupported float width", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value float32 `json:"value"`
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
		{name: "required integer omitempty", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value int64 `json:"value,omitempty"`
		}{}), contains: "cannot use omitempty"},
		{name: "required bool omitempty", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value bool `json:"value,omitempty"`
		}{}), contains: "cannot use omitempty"},
		{name: "required float omitempty", inputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value float64 `json:"value,omitempty"`
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

// TestCompileRejectsUnsupportedOutputShapes proves root outputs stay non-pointer structs
// and non-pointer omitempty remains rejected.
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
		{name: "non-pointer omitempty", outputType: reflect.TypeOf(struct {
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
