package binding

import (
	"encoding/json"
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompileAcceptsCompositeOutputs proves the reflected output universe registers.
func TestCompileAcceptsCompositeOutputs(t *testing.T) {
	tests := []struct {
		// name identifies the accepted output type.
		name string

		// outputType is compiled as a capability output.
		outputType reflect.Type
	}{
		{name: "nested structs slices maps and pointers", outputType: reflect.TypeFor[compositeOutput]()},
		{name: "every integer and float kind", outputType: reflect.TypeFor[numericOutput]()},
		{name: "named scalar aliases", outputType: reflect.TypeFor[aliasedOutput]()},
		{name: "byte slice and array", outputType: reflect.TypeFor[bytesOutput]()},
		{name: "string-alias map keys", outputType: reflect.TypeFor[aliasKeyOutput]()},
		{name: "pointer without omitempty", outputType: reflect.TypeOf(struct {
			// Value is a required nullable integer.
			Value *int64 `json:"value"`
		}{})},
		{name: "pointer with omitempty", outputType: reflect.TypeOf(struct {
			// Value is an optional integer.
			Value *int64 `json:"value,omitempty"`
		}{})},
		{name: "optional nested struct", outputType: reflect.TypeOf(struct {
			// Item is an optional nested object.
			Item *nestedItem `json:"item,omitempty"`
		}{})},
		{name: "fixed string array", outputType: reflect.TypeOf(struct {
			// Tags is a fixed-length string array.
			Tags [2]string `json:"tags"`
		}{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Compile(reflect.TypeFor[representativeInput](), tt.outputType)

			require.NoError(t, err)
			assert.Equal(t, tt.outputType, plan.OutputType())
		})
	}
}

// TestCompileRejectsUnsupportableOutputGraphs proves unsupportable output graphs fail at registration.
func TestCompileRejectsUnsupportableOutputGraphs(t *testing.T) {
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
		{name: "interface field", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value any `json:"value"`
		}{}), contains: "value"},
		{name: "nested interface", outputType: reflect.TypeFor[nestedInterfaceOutput](), contains: "items/value"},
		{name: "raw message", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value json.RawMessage `json:"value"`
		}{}), contains: "value"},
		{name: "value json marshaler", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value valueJSONMarshaler `json:"value"`
		}{}), contains: "value"},
		{name: "pointer json marshaler", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value pointerJSONMarshaler `json:"value"`
		}{}), contains: "value"},
		{name: "value text marshaler", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value valueTextMarshaler `json:"value"`
		}{}), contains: "value"},
		{name: "pointer text marshaler", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value pointerTextMarshaler `json:"value"`
		}{}), contains: "value"},
		{name: "named uint8 marshaler slice", outputType: reflect.TypeOf(struct {
			// Value is a slice of named uint8 marshalers.
			Value []marshalerByte `json:"value"`
		}{}), contains: "value"},
		{name: "named uint8 marshaler array", outputType: reflect.TypeOf(struct {
			// Value is an array of named uint8 marshalers.
			Value [2]marshalerByte `json:"value"`
		}{}), contains: "value"},
		{name: "func field", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value func() `json:"value"`
		}{}), contains: "value"},
		{name: "chan field", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value chan string `json:"value"`
		}{}), contains: "value"},
		{name: "complex field", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value complex128 `json:"value"`
		}{}), contains: "value"},
		{name: "unsafe pointer", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value unsafe.Pointer `json:"value"`
		}{}), contains: "value"},
		{name: "uintptr field", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value uintptr `json:"value"`
		}{}), contains: "value"},
		{name: "non-string map key", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value map[int]string `json:"value"`
		}{}), contains: "unsupported map key type"},
		{name: "embedded field", outputType: reflect.TypeOf(struct {
			// nestedItem is intentionally anonymous to exercise rejection.
			nestedItem
		}{}), contains: "embedded fields"},
		{name: "unexported field", outputType: reflect.TypeOf(struct {
			// value is intentionally unexported.
			value string
		}{}), contains: "must be exported"},
		{name: "ignored field", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"-"`
		}{}), contains: "ignored JSON fields"},
		{name: "unknown tag option", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"value,string"`
		}{}), contains: "unsupported JSON tag option"},
		{name: "unknown struct tag", outputType: reflect.TypeOf(struct {
			// Value is the field under validation.
			Value string `json:"value" xml:"value"`
		}{}), contains: "only one json struct tag"},
		{name: "direct cycle", outputType: reflect.TypeFor[directCycleOutput](), contains: "next"},
		{name: "indirect cycle", outputType: reflect.TypeFor[indirectCycleBranch](), contains: "leaf"},
		{name: "cycle through slice", outputType: reflect.TypeFor[sliceCycleOutput](), contains: "items"},
		{name: "cycle through map", outputType: reflect.TypeFor[mapCycleOutput](), contains: "children"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(reflect.TypeFor[representativeInput](), tt.outputType)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidPlan)
			assert.Contains(t, err.Error(), tt.contains)
			switch tt.name {
			case "interface field", "nested interface", "raw message",
				"value json marshaler", "pointer json marshaler",
				"value text marshaler", "pointer text marshaler",
				"named uint8 marshaler slice", "named uint8 marshaler array",
				"func field", "chan field", "complex field", "unsafe pointer",
				"uintptr field":
				assert.Contains(t, err.Error(), "unsupported type")
			case "direct cycle":
				assert.Contains(t, err.Error(), "cycl")
				assert.Contains(t, err.Error(), "next")
			case "indirect cycle":
				assert.Contains(t, err.Error(), "cycl")
				assert.Contains(t, err.Error(), "leaf")
			case "cycle through slice":
				assert.Contains(t, err.Error(), "cycl")
				assert.Contains(t, err.Error(), "items")
			case "cycle through map":
				assert.Contains(t, err.Error(), "cycl")
				assert.Contains(t, err.Error(), "children")
			}
		})
	}
}
