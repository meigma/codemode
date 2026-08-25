package worker

import (
	"bytes"
	"io"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, parent.writeNativeAbort())
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
