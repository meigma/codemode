package worker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/binding"
)

const (
	testWriteCap uint32 = 4096
	testReadCap  uint32 = 4096
)

// TestFrameWriteAndReadRoundTrip proves a length-prefixed payload survives an exact cap.
func TestFrameWriteAndReadRoundTrip(t *testing.T) {
	payload, err := encodeProbeAck()
	require.NoError(t, err)
	require.Len(t, payload, int(probeCap), "probe_ack must be the compact 32-byte handshake")

	var buf bytes.Buffer
	require.NoError(t, writeFrame(&buf, payload, probeCap))
	require.Equal(t, 4+int(probeCap), buf.Len())
	assert.Equal(t, uint32(len(payload)), binary.BigEndian.Uint32(buf.Bytes()[:4]))

	got, err := readFrame(&buf, probeCap)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// TestFrameRejectsInvalidLengths proves caps are checked before a payload is accepted.
func TestFrameRejectsInvalidLengths(t *testing.T) {
	valid, err := encodeProbe()
	require.NoError(t, err)

	tests := []struct {
		// name identifies the framing failure.
		name string

		// raw is the complete byte stream presented to the reader.
		raw []byte

		// cap is the state-specific payload cap.
		cap uint32

		// target is the expected protocol error.
		target error
	}{
		{name: "zero length", raw: prefixed(t, nil), cap: probeCap, target: errEmptyPayload},
		{name: "truncated prefix", raw: []byte{0x00, 0x00}, cap: probeCap, target: errTruncatedFrame},
		{
			name:   "truncated payload",
			raw:    append(u32(uint32(len(valid))), valid[:len(valid)/2]...),
			cap:    probeCap,
			target: errTruncatedFrame,
		},
		{
			name:   "over-cap length",
			raw:    prefixed(t, bytes.Repeat([]byte("a"), int(probeCap)+1)),
			cap:    probeCap,
			target: errFrameTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readFrame(bytes.NewReader(tt.raw), tt.cap)

			require.Error(t, err)
			require.ErrorIs(t, err, tt.target)
		})
	}
}

// TestFrameWriteRejectsOverCapPayloads proves writers measure the encoded payload before prefixing.
func TestFrameWriteRejectsOverCapPayloads(t *testing.T) {
	var buf bytes.Buffer
	err := writeFrame(&buf, bytes.Repeat([]byte("x"), int(probeCap)+1), probeCap)

	require.Error(t, err)
	require.ErrorIs(t, err, errFrameTooLarge)
	assert.Zero(t, buf.Len(), "an over-cap write must not emit a prefix")
}

// TestFrameRejectsInvalidPayloads proves UTF-8 is checked before a strict single-object JSON decode.
func TestFrameRejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		// name identifies the decode failure.
		name string

		// payload is the unframed JSON object.
		payload string

		// target is the expected protocol error.
		target error
	}{
		{name: "invalid UTF-8", payload: "{\xff\"type\":\"probe\",\"version\":1}", target: errInvalidUTF8},
		{name: "malformed JSON", payload: `{"type":"probe","version":`, target: errMalformedJSON},
		{name: "trailing document", payload: `{"type":"probe","version":1}{}`, target: errTrailingJSON},
		{name: "unknown type", payload: `{"type":"nope","version":1}`, target: errUnknownType},
		{name: "unknown field", payload: `{"type":"probe","version":1,"extra":true}`, target: errUnknownField},
		{name: "wrong version", payload: `{"type":"probe","version":2}`, target: errInvalidValue},
		{name: "missing version", payload: `{"type":"probe"}`, target: errInvalidValue},
		{name: "null version", payload: `{"type":"probe","version":null}`, target: errInvalidValue},
		{name: "version on later frame", payload: `{"type":"native_abort","version":1}`, target: errUnknownField},
		{
			name:    "unknown final code",
			payload: `{"type":"final_error","code":"permission_denied"}`,
			target:  errInvalidValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, tt.name != "invalid UTF-8" || !utf8.ValidString(tt.payload))
			_, err := decodeFramed(t, tt.payload)

			require.Error(t, err)
			require.ErrorIs(t, err, tt.target)
			if tt.name == "wrong version" {
				assert.Contains(t, err.Error(), "2")
			}
		})
	}
}

// TestFrameRequiredFieldsDistinguishNull proves explicit JSON null is not treated as an omitted field.
func TestFrameRequiredFieldsDistinguishNull(t *testing.T) {
	t.Run("explicit null result is legal", func(t *testing.T) {
		decoded, err := decodeFramed(t, `{"type":"final","result":null}`)
		require.NoError(t, err)
		frame, ok := decoded.(finalFrame)
		require.True(t, ok)
		assert.Equal(t, frameTypeFinal, frame.Type)
		assert.Nil(t, frame.Result)
	})

	t.Run("absent result is rejected", func(t *testing.T) {
		_, err := decodeFramed(t, `{"type":"final"}`)
		require.Error(t, err)
		require.ErrorIs(t, err, errInvalidValue)
	})

	t.Run("explicit null native result is legal", func(t *testing.T) {
		decoded, err := decodeFramed(t, `{"type":"native_result","result":null}`)
		require.NoError(t, err)
		frame, ok := decoded.(nativeResultFrame)
		require.True(t, ok)
		assert.Nil(t, frame.Result)
	})

	t.Run("absent native result is rejected", func(t *testing.T) {
		_, err := decodeFramed(t, `{"type":"native_result"}`)
		require.Error(t, err)
		require.ErrorIs(t, err, errInvalidValue)
	})

	t.Run("absent native arguments is rejected", func(t *testing.T) {
		_, err := decodeFramed(t, `{"type":"native_call","capability_id":"cap.lookup"}`)
		require.Error(t, err)
		require.ErrorIs(t, err, errInvalidValue)
	})

	t.Run("absent final error code is rejected", func(t *testing.T) {
		_, err := decodeFramed(t, `{"type":"final_error"}`)
		require.Error(t, err)
		require.ErrorIs(t, err, errInvalidValue)
	})
}

// TestFrameEncodersProduceCompactDiscriminators proves every protocol type is privately named.
func TestFrameEncodersProduceCompactDiscriminators(t *testing.T) {
	exec := validExecFrame()
	nativeArgs := map[string]any{"org": "meigma"}

	tests := []struct {
		// name identifies the encoder.
		name string

		// encode produces one compact payload.
		encode func() ([]byte, error)

		// typ is the frozen discriminator.
		typ string
	}{
		{name: "probe", encode: encodeProbe, typ: frameTypeProbe},
		{name: "probe_ack", encode: encodeProbeAck, typ: frameTypeProbeAck},
		{name: "exec", encode: func() ([]byte, error) { return encodeExec(exec) }, typ: frameTypeExec},
		{
			name:   "native_call",
			encode: func() ([]byte, error) { return encodeNativeCall("cap.lookup", nativeArgs) },
			typ:    frameTypeNativeCall,
		},
		{
			name:   "native_result",
			encode: func() ([]byte, error) { return encodeNativeResult(nil) },
			typ:    frameTypeNativeResult,
		},
		{name: "native_abort", encode: encodeNativeAbort, typ: frameTypeNativeAbort},
		{name: "final", encode: func() ([]byte, error) { return encodeFinal("ok") }, typ: frameTypeFinal},
		{
			name:   "final_error",
			encode: func() ([]byte, error) { return encodeFinalError(finalErrorInvalidProgram, "") },
			typ:    frameTypeFinalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.encode()
			require.NoError(t, err)
			assert.True(t, json.Valid(payload))
			assert.Contains(t, string(payload), `"type":"`+tt.typ+`"`)

			decoded, err := decodePayload(payload)
			require.NoError(t, err)
			assertFrameType(t, decoded, tt.typ)
		})
	}
}

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

func TestProtocolProbeSucceeds(t *testing.T) {
	parent, child, closePipes := newProbePair(t)

	var wg sync.WaitGroup
	wg.Go(func() {
		defer closePipes()
		frame, err := child.read()
		if err != nil {
			t.Errorf("child.read() error = %v", err)
			return
		}
		assertFrameType(t, frame, frameTypeProbe)
		if err := child.writeProbeAck(); err != nil {
			t.Errorf("child.writeProbeAck() error = %v", err)
		}
	})

	require.NoError(t, parent.writeProbe())
	ack, err := parent.read()
	require.NoError(t, err)
	assertFrameType(t, ack, frameTypeProbeAck)
	wg.Wait()

	_, err = parent.read()
	require.Error(t, err)
	require.ErrorIs(t, err, errIllegalState)
}

// TestProtocolAcceptsChildResourceLimit proves final_error(resource_limit) is a legal child terminal report.
func TestProtocolAcceptsChildResourceLimit(t *testing.T) {
	parent, child, closePipes := newExecPair(t, validExecFrame())

	var wg sync.WaitGroup
	wg.Go(func() {
		defer closePipes()
		_, err := child.read()
		if err != nil {
			t.Errorf("child.read() error = %v", err)
			return
		}
		if err := child.writeFinalError(finalErrorResourceLimit, ""); err != nil {
			t.Errorf("child.writeFinalError() error = %v", err)
		}
	})

	require.NoError(t, parent.writeExec(validExecFrame()))
	frame, err := parent.read()
	require.NoError(t, err)
	got, ok := frame.(finalErrorFrame)
	require.True(t, ok)
	assert.Equal(t, finalErrorResourceLimit, got.Code)
	wg.Wait()
}

func TestProtocolExecWithNativeCalls(t *testing.T) {
	parent, child, closePipes := newExecPair(t, validExecFrame())

	var wg sync.WaitGroup
	wg.Go(func() {
		defer closePipes()
		frame, err := child.read()
		if err != nil {
			t.Errorf("child.read() error = %v", err)
			return
		}
		assertFrameType(t, frame, frameTypeExec)

		for _, result := range []any{"one", "two"} {
			err := child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"})
			if err != nil {
				t.Errorf("child.writeNativeCall() error = %v", err)
				return
			}
			reply, err := child.read()
			if err != nil {
				t.Errorf("child.read() error = %v", err)
				return
			}
			got, ok := reply.(nativeResultFrame)
			if !ok {
				t.Errorf("child.read() type = %T, want nativeResultFrame", reply)
				return
			}
			assert.Equal(t, result, got.Result)
		}
		if err := child.writeFinal(map[string]any{"ok": true}); err != nil {
			t.Errorf("child.writeFinal() error = %v", err)
		}
	})

	require.NoError(t, parent.writeExec(validExecFrame()))
	for _, result := range []any{"one", "two"} {
		call, err := parent.read()
		require.NoError(t, err)
		got, ok := call.(nativeCallFrame)
		require.True(t, ok)
		assert.Equal(t, "cap.lookup", got.CapabilityID)
		assert.Equal(t, map[string]any{"org": "meigma"}, got.Arguments)
		require.NoError(t, parent.writeNativeResult(result))
	}
	final, err := parent.read()
	require.NoError(t, err)
	got, ok := final.(finalFrame)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"ok": true}, got.Result)
	wg.Wait()
}

// TestProtocolTerminalFinalErrorAndAbort proves both terminal tails close the exchange.
func TestProtocolTerminalFinalErrorAndAbort(t *testing.T) {
	t.Run("child final_error", func(t *testing.T) {
		parent, child, closePipes := newExecPair(t, validExecFrame())

		var wg sync.WaitGroup
		wg.Go(func() {
			defer closePipes()
			_, err := child.read()
			if err != nil {
				t.Errorf("child.read() error = %v", err)
				return
			}
			if err := child.writeFinalError(finalErrorInvalidArguments, ""); err != nil {
				t.Errorf("child.writeFinalError() error = %v", err)
			}
		})

		require.NoError(t, parent.writeExec(validExecFrame()))
		frame, err := parent.read()
		require.NoError(t, err)
		got, ok := frame.(finalErrorFrame)
		require.True(t, ok)
		assert.Equal(t, finalErrorInvalidArguments, got.Code)
		wg.Wait()
	})

	t.Run("parent abort", func(t *testing.T) {
		parent, child, closePipes := newExecPair(t, validExecFrame())

		var wg sync.WaitGroup
		wg.Go(func() {
			defer closePipes()
			_, err := child.read()
			if err != nil {
				t.Errorf("child.read() error = %v", err)
				return
			}
			err = child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"})
			if err != nil {
				t.Errorf("child.writeNativeCall() error = %v", err)
				return
			}
			reply, err := child.read()
			if err != nil {
				t.Errorf("child.read() error = %v", err)
				return
			}
			assertFrameType(t, reply, frameTypeNativeAbort)
		})

		require.NoError(t, parent.writeExec(validExecFrame()))
		_, err := parent.read()
		require.NoError(t, err)
		require.NoError(t, parent.writeNativeAbort())
		wg.Wait()
	})
}

// TestProtocolRejectsIllegalStateTransitions proves the state machine has no call IDs or extra frames.
func TestProtocolRejectsIllegalStateTransitions(t *testing.T) {
	tests := []struct {
		// name identifies the illegal transition.
		name string

		// run drives one side into the forbidden state.
		run func(t *testing.T)
	}{
		{
			name: "native result without outstanding call",
			run: func(t *testing.T) {
				parent, child := newBufferedExecPair(t, validExecFrame())
				require.NoError(t, parent.writeExec(validExecFrame()))
				_, err := child.read()
				require.NoError(t, err)
				err = parent.writeNativeResult("late")
				require.Error(t, err)
				require.ErrorIs(t, err, errIllegalState)
			},
		},
		{
			name: "second outstanding native call",
			run: func(t *testing.T) {
				parent, child := newBufferedExecPair(t, validExecFrame())
				require.NoError(t, parent.writeExec(validExecFrame()))
				_, err := child.read()
				require.NoError(t, err)
				require.NoError(t, child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"}))
				err = child.writeNativeCall("cap.other", map[string]any{"org": "other"})
				require.Error(t, err)
				require.ErrorIs(t, err, errIllegalState)
			},
		},
		{
			name: "frame after final",
			run: func(t *testing.T) {
				parent, child := newBufferedExecPair(t, validExecFrame())
				require.NoError(t, parent.writeExec(validExecFrame()))
				_, err := child.read()
				require.NoError(t, err)
				require.NoError(t, child.writeFinal(nil))
				_, err = parent.read()
				require.NoError(t, err)
				err = child.writeFinalError(finalErrorInternal, "")
				require.Error(t, err)
				require.ErrorIs(t, err, errIllegalState)
			},
		},
		{
			name: "frame after abort",
			run: func(t *testing.T) {
				parent, child := newBufferedExecPair(t, validExecFrame())
				require.NoError(t, parent.writeExec(validExecFrame()))
				_, err := child.read()
				require.NoError(t, err)
				require.NoError(t, child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"}))
				_, err = parent.read()
				require.NoError(t, err)
				require.NoError(t, parent.writeNativeAbort())
				_, err = child.read()
				require.NoError(t, err)
				err = child.writeFinalError(finalErrorInternal, "")
				require.Error(t, err)
				require.ErrorIs(t, err, errIllegalState)
			},
		},
		{
			name: "probe frame on exec connection",
			run: func(t *testing.T) {
				parent, _ := newBufferedExecPair(t, validExecFrame())
				err := parent.writeProbe()
				require.Error(t, err)
				require.ErrorIs(t, err, errIllegalState)
			},
		},
		{
			name: "native call beyond MaxNativeCalls",
			run: func(t *testing.T) {
				exec := validExecFrame()
				exec.Limits.MaxNativeCalls = 1
				parent, child := newBufferedExecPair(t, exec)
				require.NoError(t, parent.writeExec(exec))
				_, err := child.read()
				require.NoError(t, err)
				require.NoError(t, child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"}))
				_, err = parent.read()
				require.NoError(t, err)
				require.NoError(t, parent.writeNativeResult("ok"))
				_, err = child.read()
				require.NoError(t, err)
				err = child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"})
				require.Error(t, err)
				require.ErrorIs(t, err, errIllegalState)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

// TestProtocolChildUsesExecCapThenParentCap proves the first child read is independently bounded.
func TestProtocolChildUsesExecCapThenParentCap(t *testing.T) {
	exec := validExecFrame()
	execPayload, err := encodeExec(exec)
	require.NoError(t, err)
	execCap, err := execPayloadCap(exec.Limits.MaxSourceBytes, exec.Manifest)
	require.NoError(t, err)
	parentCap, err := parentPayloadCap(exec.Limits.MaxValueBytes)
	require.NoError(t, err)
	require.Greater(t, execCap, parentCap)

	var toChild bytes.Buffer
	child := newChildExecConn(&toChild, io.Discard, execCap, parentCap, testWriteCap)
	require.NoError(t, writeFrame(&toChild, execPayload, execCap))
	frame, err := child.read()
	require.NoError(t, err)
	assertFrameType(t, frame, frameTypeExec)
	require.NoError(t, child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"}))

	oversized := []byte(`{"type":"native_abort","pad":"` + strings.Repeat("a", int(parentCap)) + `"}`)
	require.NoError(t, writeFrame(&toChild, oversized, execCap))
	_, err = child.read()
	require.Error(t, err)
	require.ErrorIs(t, err, errFrameTooLarge)
}

// TestProtocolParentReadsChildNativeCallAboveParentCap proves parentConn uses the
// child read cap. A swapped newParentExecConn write/read pair would reject this
// native_call, whose payload sits strictly above parentCap and at or below childCap.
func TestProtocolParentReadsChildNativeCallAboveParentCap(t *testing.T) {
	exec := validExecFrame()
	parentCap, err := parentPayloadCap(exec.Limits.MaxValueBytes)
	require.NoError(t, err)
	childCap, err := childPayloadCap(exec.Limits.MaxValueBytes, exec.Manifest)
	require.NoError(t, err)
	require.Greater(t, childCap, parentCap)

	arguments := map[string]any{"org": strings.Repeat("a", 217)}
	payload, err := encodeNativeCall("cap.lookup", arguments)
	require.NoError(t, err)
	require.Greater(t, uint32(len(payload)), parentCap)
	require.LessOrEqual(t, uint32(len(payload)), childCap)

	parent, child := newBufferedExecPair(t, exec)
	require.NoError(t, parent.writeExec(exec))
	_, err = child.read()
	require.NoError(t, err)
	require.NoError(t, child.writeNativeCall("cap.lookup", arguments))

	frame, err := parent.read()
	require.NoError(t, err)
	call, ok := frame.(nativeCallFrame)
	require.True(t, ok)
	assert.Equal(t, "cap.lookup", call.CapabilityID)
	assert.Equal(t, arguments, call.Arguments)
}

// TestFrameLimitsValidateChildLimits proves zero never means unlimited.
func TestFrameLimitsValidateChildLimits(t *testing.T) {
	valid := validExecFrame().Limits

	tests := []struct {
		// name identifies the rejected limit.
		name string

		// mutate changes one positive field.
		mutate func(*childLimits)
	}{
		{name: "zero source", mutate: func(limits *childLimits) { limits.MaxSourceBytes = 0 }},
		{name: "negative source", mutate: func(limits *childLimits) { limits.MaxSourceBytes = -1 }},
		{name: "zero steps", mutate: func(limits *childLimits) { limits.MaxExecutionSteps = 0 }},
		{name: "zero native calls", mutate: func(limits *childLimits) { limits.MaxNativeCalls = 0 }},
		{name: "zero depth", mutate: func(limits *childLimits) { limits.MaxValueDepth = 0 }},
		{name: "zero value bytes", mutate: func(limits *childLimits) { limits.MaxValueBytes = 0 }},
	}

	require.NoError(t, validateChildLimits(valid))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := valid
			tt.mutate(&limits)
			err := validateChildLimits(limits)
			require.Error(t, err)
			require.ErrorIs(t, err, errInvalidLimits)
		})
	}
}

// TestFrameLimitsValidateManifest proves the child cannot invent IDs, names, or field shapes.
func TestFrameLimitsValidateManifest(t *testing.T) {
	valid := validManifest()

	tests := []struct {
		// name identifies the rejected manifest.
		name string

		// manifest is the candidate child descriptor.
		manifest []manifestEntry
	}{
		{name: "blank ID", manifest: []manifestEntry{{ID: "", Name: "records.lookup", Input: valid[0].Input}}},
		{
			name:     "whitespace ID",
			manifest: []manifestEntry{{ID: " cap.lookup", Name: "records.lookup", Input: valid[0].Input}},
		},
		{name: "duplicate ID", manifest: []manifestEntry{
			{ID: "cap.lookup", Name: "records.lookup", Input: valid[0].Input},
			{ID: "cap.lookup", Name: "records.other", Input: valid[0].Input},
		}},
		{name: "undotted name", manifest: []manifestEntry{{ID: "cap.lookup", Name: "records", Input: valid[0].Input}}},
		{
			name:     "invalid name",
			manifest: []manifestEntry{{ID: "cap.lookup", Name: "records.not-valid", Input: valid[0].Input}},
		},
		{
			name:     "reserved name segment",
			manifest: []manifestEntry{{ID: "cap.lookup", Name: "records.import", Input: valid[0].Input}},
		},
		{name: "duplicate name", manifest: []manifestEntry{
			{ID: "cap.one", Name: "records.lookup", Input: valid[0].Input},
			{ID: "cap.two", Name: "records.lookup", Input: valid[0].Input},
		}},
		{name: "namespace collision", manifest: []manifestEntry{
			{ID: "cap.lookup", Name: "records.lookup", Input: valid[0].Input},
			{ID: "cap.detail", Name: "records.lookup.detail", Input: valid[0].Input},
		}},
		{name: "unsupported input shape", manifest: []manifestEntry{{
			ID:   "cap.lookup",
			Name: "records.lookup",
			Input: []binding.FieldShape{{
				Name:     "org",
				Type:     "str",
				Required: false,
			}},
		}}},
	}

	require.NoError(t, validateManifest(nil))
	require.NoError(t, validateManifest(valid))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManifest(tt.manifest)
			require.Error(t, err)
			require.ErrorIs(t, err, errInvalidManifest)
		})
	}
}

// TestFrameLimitsCheckedCaps proves payload caps fit uint32 and follow envelope math.
func TestFrameLimitsCheckedCaps(t *testing.T) {
	manifest := validManifest()
	maxValueBytes := 64

	childCap, err := childPayloadCap(maxValueBytes, manifest)
	require.NoError(t, err)
	parentCap, err := parentPayloadCap(maxValueBytes)
	require.NoError(t, err)
	execCap, err := execPayloadCap(32, manifest)
	require.NoError(t, err)

	abort, err := encodeNativeAbort()
	require.NoError(t, err)
	finalError, err := encodeFinalError(finalErrorInvalidArguments, "")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, childCap, uint32(len(finalError)))
	assert.GreaterOrEqual(t, parentCap, uint32(len(abort)))
	assert.Greater(t, childCap, uint32(maxValueBytes))
	assert.Greater(t, parentCap, uint32(maxValueBytes))
	assert.Greater(t, execCap, uint32(len(manifestJSON(t, manifest))))

	escapedDetail, err := encodeFinalError(finalErrorInvalidArguments, strings.Repeat("\"", maxDiagnosticBytes))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, childCap, uint32(len(escapedDetail)))
	wantFinalError, err := finalErrorPayloadCap()
	require.NoError(t, err)
	assert.Equal(t, wantFinalError, childCap)

	largeValueBytes := maxDiagnosticBytes * jsonStringEscapeMax
	baseCap, err := childPayloadCap(largeValueBytes, manifest)
	require.NoError(t, err)

	longer := []manifestEntry{{
		ID:    strings.Repeat("a", 40),
		Name:  "records.lookup",
		Input: manifest[0].Input,
	}}
	require.NoError(t, validateManifest(longer))
	longerCap, err := childPayloadCap(largeValueBytes, longer)
	require.NoError(t, err)
	assert.Greater(t, longerCap, baseCap)

	escaped := []manifestEntry{{
		ID:    strings.Repeat("\"", 40),
		Name:  "records.lookup",
		Input: manifest[0].Input,
	}}
	require.NoError(t, validateManifest(escaped))
	escapedCap, err := childPayloadCap(largeValueBytes, escaped)
	require.NoError(t, err)
	assert.Greater(t, escapedCap, longerCap)

	expanded, err := execPayloadCap(8, manifest)
	require.NoError(t, err)
	unexpanded, err := execPayloadCap(1, manifest)
	require.NoError(t, err)
	assert.Greater(t, expanded, unexpanded)
}

// TestFrameNativeResultChargesValueBodyOnly proves the envelope is excluded from accounting.
func TestFrameNativeResultChargesValueBodyOnly(t *testing.T) {
	exec := validExecFrame()
	result := map[string]any{"zeta": int64(1), "alpha": true}
	body, err := encodeNormalizedValue(result)
	require.NoError(t, err)
	frame, err := encodeNativeResult(result)
	require.NoError(t, err)
	assert.Equal(t, encodeNativeResultBytes(body), frame)
	assert.Len(t, frame, len(nativeResultPrefix)+len(body)+len(nativeResultSuffix))
	assert.Greater(t, len(frame), len(body))

	parent, child := newBufferedExecPair(t, exec)
	parent.remainingIntermediate = len(body)
	require.NoError(t, parent.writeExec(exec))
	_, err = child.read()
	require.NoError(t, err)
	require.NoError(t, child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"}))
	_, err = parent.read()
	require.NoError(t, err)
	require.NoError(t, parent.writeNativeResult(result))
	assert.Zero(t, parent.remainingIntermediate)
	reply, err := child.read()
	require.NoError(t, err)
	got, ok := reply.(nativeResultFrame)
	require.True(t, ok)
	assert.Equal(t, result, got.Result)
	require.NoError(t, child.writeNativeCall("cap.lookup", map[string]any{"org": "meigma"}))
	_, err = parent.read()
	require.NoError(t, err)
	err = parent.writeNativeResult(result)
	require.ErrorIs(t, err, errFrameTooLarge)
}

// TestWriteNativeCallRejectsNilArguments proves a nil map never encodes as JSON null.
func TestWriteNativeCallRejectsNilArguments(t *testing.T) {
	exec := validExecFrame()
	parent, child := newBufferedExecPair(t, exec)
	require.NoError(t, parent.writeExec(exec))
	_, err := child.read()
	require.NoError(t, err)

	err = child.writeNativeCall("cap.lookup", nil)

	require.ErrorIs(t, err, errInvalidValue)
}

// TestFrameLimitsRejectUint32Overflow proves checked addition never wraps the prefix.
func TestFrameLimitsRejectUint32Overflow(t *testing.T) {
	manifest := validManifest()

	_, err := childPayloadCap(math.MaxInt, manifest)
	require.Error(t, err)
	require.ErrorIs(t, err, errInvalidLimits)

	_, err = parentPayloadCap(math.MaxInt)
	require.Error(t, err)
	require.ErrorIs(t, err, errInvalidLimits)

	_, err = execPayloadCap(math.MaxInt, manifest)
	require.Error(t, err)
	require.ErrorIs(t, err, errInvalidLimits)
}

// TestFrameLimitsOversizedValueIsProtocolViolation proves over-cap values are not resource-limit reports.
func TestFrameLimitsOversizedValueIsProtocolViolation(t *testing.T) {
	exec := validExecFrame()
	exec.Limits.MaxValueBytes = 8
	parent, child := newBufferedExecPair(t, exec)
	require.NoError(t, parent.writeExec(exec))
	_, err := child.read()
	require.NoError(t, err)

	oversized := strings.Repeat("a", 32)
	err = child.writeFinal(oversized)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidValue) || errors.Is(err, errFrameTooLarge))
	assert.NotErrorIs(t, err, errIllegalState)
}

// TestFrameDecodeNormalizesNestedValues proves UseNumber results never escape decodePayload.
func TestFrameDecodeNormalizesNestedValues(t *testing.T) {
	payload := `{"type":"final","result":{"count":1,"score":1.0,"items":[2,3.0]}}`
	decoded, err := decodeFramed(t, payload)
	require.NoError(t, err)
	frame, ok := decoded.(finalFrame)
	require.True(t, ok)
	result, ok := frame.Result.(map[string]any)
	require.True(t, ok)
	assert.IsType(t, int64(0), result["count"])
	assert.IsType(t, float64(0), result["score"])
	items, ok := result["items"].([]any)
	require.True(t, ok)
	assert.IsType(t, int64(0), items[0])
	assert.IsType(t, float64(0), items[1])
}

func validExecFrame() execFrame {
	return execFrame{
		Type:    frameTypeExec,
		Version: protocolVersion,
		Source:  "def main():\n    return 1\n",
		Limits: childLimits{
			MaxSourceBytes:    256,
			MaxExecutionSteps: 1000,
			MaxNativeCalls:    8,
			MaxValueDepth:     8,
			MaxValueBytes:     256,
		},
		Manifest: validManifest(),
	}
}

func validManifest() []manifestEntry {
	return []manifestEntry{{
		ID:   "cap.lookup",
		Name: "records.lookup",
		Input: []binding.FieldShape{
			{Name: "org", Type: "str", Required: true},
			{Name: "limit", Type: "int | None", Required: false},
		},
	}}
}

func newProbePair(t *testing.T) (*parentConn, *childConn, func()) {
	t.Helper()
	childReader, parentWriter := io.Pipe()
	parentReader, childWriter := io.Pipe()
	closePipes := func() {
		_ = childReader.Close()
		_ = parentWriter.Close()
		_ = parentReader.Close()
		_ = childWriter.Close()
	}
	t.Cleanup(closePipes)
	return newParentProbeConn(parentWriter, parentReader), newChildProbeConn(childReader, childWriter), closePipes
}

func newExecPair(t *testing.T, exec execFrame) (*parentConn, *childConn, func()) {
	t.Helper()
	execCap, err := execPayloadCap(exec.Limits.MaxSourceBytes, exec.Manifest)
	require.NoError(t, err)
	parentCap, err := parentPayloadCap(exec.Limits.MaxValueBytes)
	require.NoError(t, err)
	childCap, err := childPayloadCap(exec.Limits.MaxValueBytes, exec.Manifest)
	require.NoError(t, err)

	childReader, parentWriter := io.Pipe()
	parentReader, childWriter := io.Pipe()
	closePipes := func() {
		_ = childReader.Close()
		_ = parentWriter.Close()
		_ = parentReader.Close()
		_ = childWriter.Close()
	}
	t.Cleanup(closePipes)
	return newParentExecConn(parentWriter, parentReader, parentCap, childCap, exec.Limits.MaxValueBytes),
		newChildExecConn(childReader, childWriter, execCap, parentCap, childCap),
		closePipes
}

func decodeFramed(t *testing.T, payload string) (any, error) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, writeFrame(&buf, []byte(payload), testReadCap))
	raw, err := readFrame(&buf, testReadCap)
	if err != nil {
		return nil, err
	}
	return decodePayload(raw)
}

func newBufferedExecPair(t *testing.T, exec execFrame) (*parentConn, *childConn) {
	t.Helper()
	execCap, err := execPayloadCap(exec.Limits.MaxSourceBytes, exec.Manifest)
	require.NoError(t, err)
	parentCap, err := parentPayloadCap(exec.Limits.MaxValueBytes)
	require.NoError(t, err)
	childCap, err := childPayloadCap(exec.Limits.MaxValueBytes, exec.Manifest)
	require.NoError(t, err)

	var toChild bytes.Buffer
	var toParent bytes.Buffer
	return newParentExecConn(&toChild, &toParent, parentCap, childCap, exec.Limits.MaxValueBytes),
		newChildExecConn(&toChild, &toParent, execCap, parentCap, childCap)
}

func prefixed(t *testing.T, payload []byte) []byte {
	t.Helper()
	return append(u32(uint32(len(payload))), payload...)
}

func u32(value uint32) []byte {
	raw := make([]byte, 4)
	binary.BigEndian.PutUint32(raw, value)
	return raw
}

func assertFrameType(t *testing.T, frame any, want string) {
	t.Helper()
	switch got := frame.(type) {
	case probeFrame:
		assert.Equal(t, want, got.Type)
	case probeAckFrame:
		assert.Equal(t, want, got.Type)
	case execFrame:
		assert.Equal(t, want, got.Type)
	case nativeCallFrame:
		assert.Equal(t, want, got.Type)
	case nativeResultFrame:
		assert.Equal(t, want, got.Type)
	case nativeAbortFrame:
		assert.Equal(t, want, got.Type)
	case finalFrame:
		assert.Equal(t, want, got.Type)
	case finalErrorFrame:
		assert.Equal(t, want, got.Type)
	default:
		t.Fatalf("unexpected frame type %T", frame)
	}
}

func manifestJSON(t *testing.T, manifest []manifestEntry) []byte {
	t.Helper()
	encoded, err := json.Marshal(manifest)
	require.NoError(t, err)
	return encoded
}
