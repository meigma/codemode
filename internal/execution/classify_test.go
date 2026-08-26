package execution

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/syntax"
)

// TestClassifyRuntimeErrorOmitsParserInternalError proves parser bug text stays coarse.
func TestClassifyRuntimeErrorOmitsParserInternalError(t *testing.T) {
	filename := "<codemode>"
	err := classifyRuntimeError(&executionState{}, syntax.Error{
		Pos: syntax.MakePosition(&filename, 1, 1),
		Msg: "internal error: parser panic",
	})

	require.ErrorIs(t, err, ErrInvalidProgram)
	assert.Equal(t, ErrInvalidProgram.Error(), err.Error())
	_, ok := SafeDetail(err)
	assert.False(t, ok)
	assert.NotContains(t, err.Error(), "internal error:")
	assert.NotContains(t, err.Error(), "parser panic")
}
