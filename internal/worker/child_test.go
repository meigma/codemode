package worker

import (
	"bytes"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"

	"github.com/meigma/codemode/internal/execution"
)

// TestServeProbe proves one probe request is acknowledged and serve returns nil.
func TestServeProbe(t *testing.T) {
	payload, err := encodeProbe()
	require.NoError(t, err)

	var in bytes.Buffer
	var out bytes.Buffer
	require.NoError(t, writeFrame(&in, payload, probeCap))

	require.NoError(t, serve(&in, &out))

	got, err := readFrame(&out, probeCap)
	require.NoError(t, err)
	decoded, err := decodePayload(got)
	require.NoError(t, err)
	assertFrameType(t, decoded, frameTypeProbeAck)
	assert.Empty(t, out.Bytes())
}

// TestServeExecValidation proves source length and child budgets are checked
// before Engine construction.
func TestServeExecValidation(t *testing.T) {
	t.Run("oversized source", func(t *testing.T) {
		payload := []byte(
			`{"type":"exec","version":1,"source":"def main():\n    return 1\n",` +
				`"limits":{"max_source_bytes":8,"max_execution_steps":1000,` +
				`"max_native_calls":8,"max_value_depth":8,"max_value_bytes":256},` +
				`"manifest":[]}`,
		)

		var in bytes.Buffer
		var out bytes.Buffer
		require.NoError(t, writeFrame(&in, payload, math.MaxUint32))

		require.NoError(t, serve(&in, &out))

		got, err := readFrame(&out, math.MaxUint32)
		require.NoError(t, err)
		decoded, err := decodePayload(got)
		require.NoError(t, err)
		frame, ok := decoded.(finalErrorFrame)
		require.True(t, ok)
		assert.Equal(t, finalErrorResourceLimit, frame.Code)
		assert.Empty(t, out.Bytes())
	})

	t.Run("zero child limit", func(t *testing.T) {
		payload := []byte(
			`{"type":"exec","version":1,"source":"def main():\n    return 1\n",` +
				`"limits":{"max_source_bytes":0,"max_execution_steps":1000,` +
				`"max_native_calls":8,"max_value_depth":8,"max_value_bytes":256},` +
				`"manifest":[]}`,
		)

		var in bytes.Buffer
		var out bytes.Buffer
		require.NoError(t, writeFrame(&in, payload, math.MaxUint32))

		err := serve(&in, &out)
		require.Error(t, err)
		require.ErrorIs(t, err, errInvalidLimits)
		assert.Zero(t, out.Len())
	})
}

// TestServeProtocolFailure proves a non-handshake first frame is a status-1 service error.
func TestServeProtocolFailure(t *testing.T) {
	payload := []byte(`{"type":"native_abort"}`)

	var in bytes.Buffer
	var out bytes.Buffer
	require.NoError(t, writeFrame(&in, payload, math.MaxUint32))

	err := serve(&in, &out)
	require.Error(t, err)
	require.ErrorIs(t, err, errIllegalState)
	assert.Zero(t, out.Len())
}

// TestServeManifestRejection proves an illegal advertised shape never starts Engine.
func TestServeManifestRejection(t *testing.T) {
	payload := []byte(
		`{"type":"exec","version":1,"source":"def main():\n    return 1\n",` +
			`"limits":{"max_source_bytes":256,"max_execution_steps":1000,` +
			`"max_native_calls":8,"max_value_depth":8,"max_value_bytes":256},` +
			`"manifest":[{"id":"cap.lookup","name":"records.lookup",` +
			`"input":[{"name":"org","type":"str","required":false}]}]}`,
	)

	var in bytes.Buffer
	var out bytes.Buffer
	require.NoError(t, writeFrame(&in, payload, math.MaxUint32))

	err := serve(&in, &out)
	require.Error(t, err)
	require.ErrorIs(t, err, errInvalidManifest)
	assert.Zero(t, out.Len())
}

// TestServeNativeRoundTrip proves one Engine native call is forwarded and returned.
func TestServeNativeRoundTrip(t *testing.T) {
	exec := validExecFrame()
	exec.Source = "def main():\n    return records.lookup(org=\"acme\")\n"

	parent, done, closePipes := startServeExec(t, exec)
	defer closePipes()
	require.NoError(t, parent.writeExec(exec))

	frame, err := parent.read()
	require.NoError(t, err)
	call, ok := frame.(nativeCallFrame)
	require.True(t, ok)
	assert.Equal(t, "cap.lookup", call.CapabilityID)
	assert.Equal(t, map[string]any{"org": "acme"}, call.Arguments)

	require.NoError(t, parent.writeNativeResult("ok"))
	frame, err = parent.read()
	require.NoError(t, err)
	final, ok := frame.(finalFrame)
	require.True(t, ok)
	assert.Equal(t, "ok", final.Result)
	require.NoError(t, <-done)
}

// TestServeFinalAndFinalErrorMapping proves Engine outcomes become child terminal codes.
func TestServeFinalAndFinalErrorMapping(t *testing.T) {
	tests := []struct {
		// name identifies the mapped execution outcome.
		name string

		// mutate adjusts the representative exec request.
		mutate func(*execFrame)

		// wantFinal is the successful result when code is empty.
		wantFinal any

		// code is the expected final_error class; empty means final.
		code finalErrorCode
	}{
		{
			name:      "successful final",
			mutate:    func(*execFrame) {},
			wantFinal: int64(1),
		},
		{
			name: "invalid program",
			mutate: func(exec *execFrame) {
				exec.Source = "def main():\n    return main\n"
			},
			code: finalErrorInvalidProgram,
		},
		{
			name: "invalid arguments",
			mutate: func(exec *execFrame) {
				exec.Source = "def main():\n    return records.lookup()\n"
			},
			code: finalErrorInvalidArguments,
		},
		{
			name: "resource limit",
			mutate: func(exec *execFrame) {
				exec.Limits.MaxValueDepth = 2
				exec.Source = "def main():\n    return [[[\"deep\"]]]\n"
			},
			code: finalErrorResourceLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := validExecFrame()
			tt.mutate(&exec)

			parent, done, closePipes := startServeExec(t, exec)
			defer closePipes()
			require.NoError(t, parent.writeExec(exec))

			frame, err := parent.read()
			require.NoError(t, err)
			if tt.code == "" {
				got, ok := frame.(finalFrame)
				require.True(t, ok)
				assert.Equal(t, tt.wantFinal, got.Result)
			} else {
				got, ok := frame.(finalErrorFrame)
				require.True(t, ok)
				assert.Equal(t, tt.code, got.Code)
			}
			require.NoError(t, <-done)
		})
	}
}

// TestFinalErrorFromExtractsApprovedSafeDetail proves only matching approved codes carry a suffix.
func TestFinalErrorFromExtractsApprovedSafeDetail(t *testing.T) {
	tests := []struct {
		// name identifies the mapped execution error.
		name string

		// err is the classified Engine failure.
		err error

		// code is the expected child-owned terminal class.
		code finalErrorCode

		// detail is the suffix written for approved codes.
		detail string
	}{
		{
			name:   "invalid program keeps suffix",
			err:    execution.WithSafeDetail(execution.ErrInvalidProgram, "<codemode>:1:1: undefined: filter"),
			code:   finalErrorInvalidProgram,
			detail: "<codemode>:1:1: undefined: filter",
		},
		{
			name:   "invalid arguments keeps suffix",
			err:    execution.WithSafeDetail(execution.ErrInvalidArguments, `unknown argument "keu"`),
			code:   finalErrorInvalidArguments,
			detail: `unknown argument "keu"`,
		},
		{
			name: "code-only fallback for empty detail",
			err:  execution.ErrInvalidProgram,
			code: finalErrorInvalidProgram,
		},
		{
			name: "code-only fallback for oversized detail",
			err:  execution.WithSafeDetail(execution.ErrInvalidProgram, strings.Repeat("a", maxDiagnosticBytes+1)),
			code: finalErrorInvalidProgram,
		},
		{
			name: "resource limit stays bare",
			err:  execution.WithSafeDetail(execution.ErrResourceLimit, "hidden"),
			code: finalErrorResourceLimit,
		},
		{
			name: "internal stays bare",
			err:  execution.WithSafeDetail(execution.ErrInternal, "hidden"),
			code: finalErrorInternal,
		},
		{
			name: "capability failure stays child-internal",
			err:  execution.WithSafeDetail(execution.ErrCapabilityFailure, `instance "web" not found`),
			code: finalErrorInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, detail := finalErrorFrom(tt.err)

			assert.Equal(t, tt.code, code)
			assert.Equal(t, tt.detail, detail)
		})
	}
}

// TestServeFinalWriteClassification proves only a legal value-byte overflow is
// resource_limit; unsupported final values stay invalid_program.
func TestServeFinalWriteClassification(t *testing.T) {
	code, ok := finalWriteCode(errFrameTooLarge)
	require.True(t, ok)
	assert.Equal(t, finalErrorResourceLimit, code)

	code, ok = finalWriteCode(errInvalidValue)
	require.True(t, ok)
	assert.Equal(t, finalErrorInvalidProgram, code)

	code, ok = finalWriteCode(errInvalidNumber)
	require.True(t, ok)
	assert.Equal(t, finalErrorInvalidProgram, code)

	_, ok = finalWriteCode(errIllegalState)
	assert.False(t, ok)
}

// TestNativeForwarderClassifiesBoundedWriteErrors proves oversized or nil
// argument maps fail as a resource limit before a native_call is written.
func TestNativeForwarderClassifiesBoundedWriteErrors(t *testing.T) {
	tests := []struct {
		// name identifies the rejected argument map.
		name string

		// arguments are presented to the native forwarder.
		arguments map[string]any
	}{
		{name: "nil arguments", arguments: nil},
		{name: "oversized arguments", arguments: map[string]any{"org": "meigma"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := validExecFrame()
			exec.Limits.MaxValueBytes = 8
			parent, child := newBufferedExecPair(t, exec)
			require.NoError(t, parent.writeExec(exec))
			_, err := child.read()
			require.NoError(t, err)
			written, ok := parent.r.(*bytes.Buffer)
			require.True(t, ok)
			require.Zero(t, written.Len())

			_, err = nativeForwarder(child)("cap.lookup", tt.arguments)

			require.ErrorIs(t, err, execution.ErrResourceLimit)
			require.NotErrorIs(t, err, errChildService)
			assert.Zero(t, written.Len())
		})
	}
}

// TestServeEngineAbortSuppressesFinalError proves a real Engine native_abort
// exits successfully without a terminal child frame.
func TestServeEngineAbortSuppressesFinalError(t *testing.T) {
	exec := validExecFrame()
	exec.Source = "def main():\n    return records.lookup(org=\"acme\")\n"

	parent, done, closePipes := startServeExec(t, exec)
	defer closePipes()
	require.NoError(t, parent.writeExec(exec))

	frame, err := parent.read()
	require.NoError(t, err)
	_, ok := frame.(nativeCallFrame)
	require.True(t, ok)
	require.NoError(t, parent.writeNativeAbort(""))
	require.NoError(t, <-done)
}

// TestNativeForwarderAbortClassification proves approved suffixes are Starlark-visible
// and empty aborts stay the private sentinel.
func TestNativeForwarderAbortClassification(t *testing.T) {
	const detail = `instance "web" not found in sandbox "demo"`
	tests := []struct {
		// name identifies the abort payload.
		name string

		// detail is written on the parent abort frame.
		detail string

		// want is the classified unwind error.
		want error

		// text is the Starlark-visible error string.
		text string

		// suffix is the approved SafeDetail, if any.
		suffix string
	}{
		{
			name:   "approved suffix is Starlark-visible",
			detail: detail,
			want:   execution.ErrCapabilityFailure,
			text:   execution.ErrCapabilityFailure.Error() + ": " + detail,
			suffix: detail,
		},
		{
			name: "empty abort stays private sentinel",
			want: errNativeAbort,
			text: errNativeAbort.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := validExecFrame()
			parent, child, closePipes := newExecPair(t, exec)
			defer closePipes()
			handshake := make(chan error, 1)
			go func() { handshake <- parent.writeExec(exec) }()
			_, err := child.read()
			require.NoError(t, err)
			require.NoError(t, <-handshake)

			done := make(chan error, 1)
			go func() {
				builtin := starlark.NewBuiltin(
					"lookup",
					func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
						_, nativeErr := nativeForwarder(child)("cap.lookup", map[string]any{"org": "meigma"})
						return nil, nativeErr
					},
				)
				_, callErr := starlark.Call(&starlark.Thread{Name: "codemode"}, builtin, nil, nil)
				done <- callErr
			}()

			frame, err := parent.read()
			require.NoError(t, err)
			_, ok := frame.(nativeCallFrame)
			require.True(t, ok)
			require.NoError(t, parent.writeNativeAbort(tt.detail))

			err = <-done
			require.ErrorIs(t, err, tt.want)
			assert.Equal(t, tt.text, err.Error())
			var evalErr *starlark.EvalError
			require.ErrorAs(t, err, &evalErr)
			assert.Equal(t, tt.text, evalErr.Msg)
			got, ok := execution.SafeDetail(err)
			if tt.suffix == "" {
				assert.False(t, ok)
				require.NotErrorIs(t, err, execution.ErrCapabilityFailure)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.suffix, got)
			require.NotErrorIs(t, err, errNativeAbort)
			assert.NotContains(t, err.Error(), "db password")
		})
	}
}

// TestServeEngineAbortWithDetailSuppressesFinalError proves a detailed native_abort
// still exits without a child-owned terminal frame.
func TestServeEngineAbortWithDetailSuppressesFinalError(t *testing.T) {
	exec := validExecFrame()
	exec.Source = "def main():\n    return records.lookup(org=\"acme\")\n"

	parent, done, closePipes := startServeExec(t, exec)
	defer closePipes()
	require.NoError(t, parent.writeExec(exec))

	frame, err := parent.read()
	require.NoError(t, err)
	_, ok := frame.(nativeCallFrame)
	require.True(t, ok)
	require.NoError(t, parent.writeNativeAbort(`instance "web" not found`))
	require.NoError(t, <-done)
}

// startServeExec launches serve against one execution parentConn.
func startServeExec(t *testing.T, exec execFrame) (*parentConn, <-chan error, func()) {
	t.Helper()
	parentCap, err := parentPayloadCap(exec.Limits.MaxValueBytes)
	require.NoError(t, err)
	childCap, err := childPayloadCap(exec.Limits.MaxValueBytes, exec.Manifest)
	require.NoError(t, err)

	childReader, parentWriter := io.Pipe()
	parentReader, childWriter := io.Pipe()
	parent := newParentExecConn(parentWriter, parentReader, parentCap, childCap, exec.Limits.MaxValueBytes)
	done := make(chan error, 1)
	go func() {
		done <- serve(childReader, childWriter)
	}()
	closePipes := func() {
		_ = childReader.Close()
		_ = parentWriter.Close()
		_ = parentReader.Close()
		_ = childWriter.Close()
	}
	t.Cleanup(closePipes)
	return parent, done, closePipes
}
