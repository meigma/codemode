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
