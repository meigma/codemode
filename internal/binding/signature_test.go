package binding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSignatureUsesTheCompiledInputPlan proves documentation cannot drift from binding behavior.
func TestSignatureUsesTheCompiledInputPlan(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	signature := plan.Signature("records.lookup")

	assert.Equal(t, "records.lookup(*, org: str, limit: int | None) -> representativeOutput", signature)
}

// TestSignatureHandlesAnEmptyInputStruct proves zero-argument capabilities omit the keyword-only marker.
func TestSignatureHandlesAnEmptyInputStruct(t *testing.T) {
	plan, err := CompileFor[struct{}, representativeOutput]()
	require.NoError(t, err)

	assert.Equal(t, "records.status() -> representativeOutput", plan.Signature("records.status"))
}
