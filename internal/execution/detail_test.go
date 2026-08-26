package execution_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/execution"
)

// TestWithSafeDetailPreservesCoarseCause proves the wrapper never changes Error or [errors.Is].
func TestWithSafeDetailPreservesCoarseCause(t *testing.T) {
	t.Run("empty detail returns cause", func(t *testing.T) {
		got := execution.WithSafeDetail(execution.ErrInvalidProgram, "")

		assert.Equal(t, execution.ErrInvalidProgram, got)
		_, ok := execution.SafeDetail(got)
		assert.False(t, ok)
	})

	t.Run("Error stays coarse", func(t *testing.T) {
		const detail = "<codemode>:1:1: got '='"
		got := execution.WithSafeDetail(execution.ErrInvalidProgram, detail)

		require.ErrorIs(t, got, execution.ErrInvalidProgram)
		assert.Equal(t, execution.ErrInvalidProgram.Error(), got.Error())
		gotDetail, ok := execution.SafeDetail(got)
		require.True(t, ok)
		assert.Equal(t, detail, gotDetail)
	})

	t.Run("extracts through genuine wrapper", func(t *testing.T) {
		const detail = `unknown argument "keu"`
		got := fmt.Errorf("worker: %w", execution.WithSafeDetail(execution.ErrInvalidArguments, detail))

		require.ErrorIs(t, got, execution.ErrInvalidArguments)
		gotDetail, ok := execution.SafeDetail(got)
		require.True(t, ok)
		assert.Equal(t, detail, gotDetail)
	})
}

// TestExecuteAttachesApprovedProgramDiagnostics proves parse and resolve suffixes stay model-derived.
func TestExecuteAttachesApprovedProgramDiagnostics(t *testing.T) {
	engine := buildEngine(t)
	tests := []struct {
		// name identifies the invalid program.
		name string

		// source is the submitted Starlark program.
		source string

		// contains is the required model-derived suffix fragment.
		contains string
	}{
		{
			name:     "ordinary syntax",
			source:   "def main():\n    return =\n",
			contains: "<codemode>:",
		},
		{
			name:     "undefined filter",
			source:   "def main():\n    return filter([1, 2])\n",
			contains: "undefined: filter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.Execute(tt.source, echoNativeCall(), defaultExecutionLimits())

			require.ErrorIs(t, err, execution.ErrInvalidProgram)
			assert.Equal(t, execution.ErrInvalidProgram.Error(), err.Error())
			detail, ok := execution.SafeDetail(err)
			require.True(t, ok)
			assert.Contains(t, detail, tt.contains)
			assert.Contains(t, detail, "<codemode>:")
			assert.NotContains(t, err.Error(), detail)
		})
	}
}

// TestExecuteAttachesOneBindingDiagnostic proves BindShape suffixes omit the coarse prefix.
func TestExecuteAttachesOneBindingDiagnostic(t *testing.T) {
	var nativeCalls atomic.Int64
	_, err := buildEngine(t).Execute(
		`def main(): return records.lookup(keu="alpha")`,
		countingNativeCall(&nativeCalls),
		defaultExecutionLimits(),
	)

	require.ErrorIs(t, err, execution.ErrInvalidArguments)
	assert.Equal(t, execution.ErrInvalidArguments.Error(), err.Error())
	detail, ok := execution.SafeDetail(err)
	require.True(t, ok)
	assert.Equal(t, `unknown argument "keu"`, detail)
	assert.NotContains(t, detail, execution.ErrInvalidArguments.Error())
	assert.Zero(t, nativeCalls.Load())
}

// TestExecuteKeepsGenericRuntimeErrorsCoarse proves evaluator messages stay hidden.
func TestExecuteKeepsGenericRuntimeErrorsCoarse(t *testing.T) {
	_, err := buildEngine(t).Execute(
		"def main():\n    fail(\"db password rejected\")\n",
		echoNativeCall(),
		defaultExecutionLimits(),
	)

	require.ErrorIs(t, err, execution.ErrInvalidProgram)
	assert.Equal(t, execution.ErrInvalidProgram.Error(), err.Error())
	_, ok := execution.SafeDetail(err)
	assert.False(t, ok)
	assert.NotContains(t, err.Error(), "db password rejected")
	assert.NotContains(t, err.Error(), "fail")
}
