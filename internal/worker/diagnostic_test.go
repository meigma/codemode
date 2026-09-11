package worker

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/execution"
)

// TestEncodeFinalErrorRoundTripsApprovedDetail proves legal suffixes survive encode and decode.
func TestEncodeFinalErrorRoundTripsApprovedDetail(t *testing.T) {
	tests := []struct {
		// name identifies the approved detail class.
		name string

		// code is the child-owned terminal class.
		code finalErrorCode

		// detail is the model-derived suffix.
		detail string
	}{
		{
			name:   "invalid program",
			code:   finalErrorInvalidProgram,
			detail: "<codemode>:3:7: got '=', want primary expression",
		},
		{
			name:   "invalid arguments",
			code:   finalErrorInvalidArguments,
			detail: `unknown argument "keu"`,
		},
		{
			name:   "max budget",
			code:   finalErrorInvalidProgram,
			detail: strings.Repeat("x", maxDiagnosticBytes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := encodeFinalError(tt.code, tt.detail)
			require.NoError(t, err)
			assert.Contains(t, string(payload), `"detail":`)

			decoded, err := decodePayload(payload)
			require.NoError(t, err)
			got, ok := decoded.(finalErrorFrame)
			require.True(t, ok)
			assert.Equal(t, tt.code, got.Code)
			assert.Equal(t, tt.detail, got.Detail)
		})
	}
}

// TestDecodeFinalErrorRejectsInvalidDetail proves empty, oversized, and illegal-code detail fail closed.
func TestDecodeFinalErrorRejectsInvalidDetail(t *testing.T) {
	tests := []struct {
		// name identifies the illegal detail.
		name string

		// payload is the unframed JSON object.
		payload string
	}{
		{
			name:    "empty detail",
			payload: `{"type":"final_error","code":"invalid_program","detail":""}`,
		},
		{
			name:    "null detail",
			payload: `{"type":"final_error","code":"invalid_program","detail":null}`,
		},
		{
			name: "oversized detail",
			payload: `{"type":"final_error","code":"invalid_program","detail":"` + strings.Repeat(
				"a",
				maxDiagnosticBytes+1,
			) + `"}`,
		},
		{
			name:    "illegal resource_limit detail",
			payload: `{"type":"final_error","code":"resource_limit","detail":"hidden"}`,
		},
		{
			name:    "illegal internal detail",
			payload: `{"type":"final_error","code":"internal","detail":"hidden"}`,
		},
		{
			name:    "null detail on illegal code",
			payload: `{"type":"final_error","code":"internal","detail":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodePayload([]byte(tt.payload))

			require.Error(t, err)
			require.ErrorIs(t, err, errInvalidValue)
		})
	}
}

// TestEncodeNativeAbortRoundTripsApprovedDetail proves legal suffixes survive encode and decode.
func TestEncodeNativeAbortRoundTripsApprovedDetail(t *testing.T) {
	tests := []struct {
		// name identifies the approved abort suffix.
		name string

		// detail is the parent-approved capability-failure suffix.
		detail string
	}{
		{
			name:   "quoted resource name",
			detail: `instance "web" not found in sandbox "demo"`,
		},
		{
			name:   "max budget",
			detail: strings.Repeat("x", execution.MaxAgentErrorBytes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := encodeNativeAbort(tt.detail)
			require.NoError(t, err)
			assert.Contains(t, string(payload), `"detail":`)
			assert.NotContains(t, string(payload), "db password")

			decoded, err := decodePayload(payload)
			require.NoError(t, err)
			got, ok := decoded.(nativeAbortFrame)
			require.True(t, ok)
			assert.Equal(t, tt.detail, got.Detail)
		})
	}
}

// TestEncodeNativeAbortDropsIllegalDetail proves empty and illegal suffixes stay payload-free.
func TestEncodeNativeAbortDropsIllegalDetail(t *testing.T) {
	tests := []struct {
		// name identifies the omitted detail.
		name string

		// detail is rejected before encode.
		detail string
	}{
		{name: "empty", detail: ""},
		{name: "newline", detail: "a\nb"},
		{name: "control", detail: "a\x01b"},
		{name: "oversized", detail: strings.Repeat("a", execution.MaxAgentErrorBytes+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := encodeNativeAbort(tt.detail)
			require.NoError(t, err)
			assert.JSONEq(t, nativeAbortPayload, string(payload))
		})
	}
}

// TestDecodeNativeAbortRejectsInvalidDetail proves malformed abort suffixes fail closed.
func TestDecodeNativeAbortRejectsInvalidDetail(t *testing.T) {
	tests := []struct {
		// name identifies the illegal abort payload.
		name string

		// payload is the unframed JSON object.
		payload string

		// target is the expected protocol error.
		target error
	}{
		{
			name:    "empty detail",
			payload: `{"type":"native_abort","detail":""}`,
			target:  errInvalidValue,
		},
		{
			name:    "null detail",
			payload: `{"type":"native_abort","detail":null}`,
			target:  errInvalidValue,
		},
		{
			name:    "non-string detail",
			payload: `{"type":"native_abort","detail":1}`,
			target:  errInvalidValue,
		},
		{
			name:    "newline detail",
			payload: `{"type":"native_abort","detail":"a\u000ab"}`,
			target:  errInvalidValue,
		},
		{
			name:    "control detail",
			payload: `{"type":"native_abort","detail":"a\u0001b"}`,
			target:  errInvalidValue,
		},
		{
			name: "oversized detail",
			payload: `{"type":"native_abort","detail":"` + strings.Repeat(
				"a",
				execution.MaxAgentErrorBytes+1,
			) + `"}`,
			target: errInvalidValue,
		},
		{
			name:    "unknown field",
			payload: `{"type":"native_abort","detail":"x","cause":"db password rejected"}`,
			target:  errUnknownField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodePayload([]byte(tt.payload))

			require.Error(t, err)
			require.ErrorIs(t, err, tt.target)
		})
	}
}

// TestEncodeFinalErrorFallsBackToCodeOnly proves illegal detail is dropped before write.
func TestEncodeFinalErrorFallsBackToCodeOnly(t *testing.T) {
	tests := []struct {
		// name identifies the omitted detail.
		name string

		// code is the child-owned terminal class.
		code finalErrorCode

		// detail is the suffix that must not be written.
		detail string
	}{
		{name: "absent detail", code: finalErrorInvalidProgram, detail: ""},
		{name: "oversized detail", code: finalErrorInvalidProgram, detail: strings.Repeat("a", maxDiagnosticBytes+1)},
		{name: "illegal code detail", code: finalErrorResourceLimit, detail: "hidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := encodeFinalError(tt.code, tt.detail)
			require.NoError(t, err)
			assert.NotContains(t, string(payload), `"detail"`)

			decoded, err := decodePayload(payload)
			require.NoError(t, err)
			got, ok := decoded.(finalErrorFrame)
			require.True(t, ok)
			assert.Equal(t, tt.code, got.Code)
			assert.Empty(t, got.Detail)
		})
	}
}

// TestWriteAbortAttachesOnlyApprovedCapabilityDetail proves the abort frame
// carries SafeDetail only for classified capability failures and never raw causes.
func TestWriteAbortAttachesOnlyApprovedCapabilityDetail(t *testing.T) {
	const approved = `instance "web" not found in sandbox "demo"`
	tests := []struct {
		// name identifies the retained parent error.
		name string

		// err is retained by writeAbort.
		err error

		// detail is the approved suffix expected on the wire.
		detail string
	}{
		{
			name:   "approved capability suffix",
			err:    execution.WithSafeDetail(execution.ErrCapabilityFailure, approved),
			detail: approved,
		},
		{
			name:   "wrapped approved suffix omits cause",
			err:    fmt.Errorf("lookup: %w", execution.WithSafeDetail(execution.ErrCapabilityFailure, "x")),
			detail: "x",
		},
		{
			name: "ordinary capability failure stays bare",
			err:  execution.ErrCapabilityFailure,
		},
		{
			name: "permission denied stays bare",
			err:  execution.ErrPermissionDenied,
		},
		{
			name: "permission denied hides attached detail",
			err:  execution.WithSafeDetail(execution.ErrPermissionDenied, "secret"),
		},
		{
			name: "resource limit stays bare",
			err:  execution.ErrResourceLimit,
		},
		{
			name: "raw wrapper text does not traverse",
			err:  fmt.Errorf("%w: %w", execution.ErrCapabilityFailure, errors.New("db password rejected")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := validExecFrame()
			parent, child := newBufferedExecPair(t, exec)
			require.NoError(t, parent.writeExec(exec))
			_, err := child.read()
			require.NoError(t, err)
			require.NoError(t, child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"}))
			_, err = parent.read()
			require.NoError(t, err)

			written, ok := parent.w.(*bytes.Buffer)
			require.True(t, ok)
			written.Reset()

			out := writeAbort(parent, tt.err)
			require.Equal(t, tt.err, out.retained)

			payload, err := readFrame(written, parent.writeCap)
			require.NoError(t, err)
			assert.NotContains(t, string(payload), "db password")
			assert.NotContains(t, string(payload), "secret")
			if tt.detail == "" {
				assert.JSONEq(t, nativeAbortPayload, string(payload))
				assert.NotContains(t, string(payload), `"detail"`)
				return
			}
			decoded, err := decodePayload(payload)
			require.NoError(t, err)
			frame, ok := decoded.(nativeAbortFrame)
			require.True(t, ok)
			assert.Equal(t, tt.detail, frame.Detail)
			assert.NotContains(t, string(payload), "lookup:")
		})
	}
}
