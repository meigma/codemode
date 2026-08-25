package worker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const protocolVersion = 1

const (
	frameTypeProbe        = "probe"
	frameTypeProbeAck     = "probe_ack"
	frameTypeExec         = "exec"
	frameTypeNativeCall   = "native_call"
	frameTypeNativeResult = "native_result"
	frameTypeNativeAbort  = "native_abort"
	frameTypeFinal        = "final"
	frameTypeFinalError   = "final_error"
)

// finalErrorCode is one child-owned terminal failure class.
type finalErrorCode string

const (
	finalErrorInvalidProgram   finalErrorCode = "invalid_program"
	finalErrorInvalidArguments finalErrorCode = "invalid_arguments"
	finalErrorResourceLimit    finalErrorCode = "resource_limit"
	finalErrorInternal         finalErrorCode = "internal"
)

// knownFinalError reports whether code is one child-owned terminal class.
func knownFinalError(code finalErrorCode) bool {
	switch code {
	case finalErrorInvalidProgram, finalErrorInvalidArguments, finalErrorResourceLimit, finalErrorInternal:
		return true
	default:
		return false
	}
}

const (
	// probeCap is the exact compact {"type":"probe_ack","version":1} size.
	probeCap uint32 = 32

	framePrefixSize = 4
)

var (
	errEmptyPayload    = errors.New("empty payload")
	errTruncatedFrame  = errors.New("truncated frame")
	errFrameTooLarge   = errors.New("frame too large")
	errInvalidUTF8     = errors.New("invalid utf-8")
	errMalformedJSON   = errors.New("malformed json")
	errTrailingJSON    = errors.New("trailing json")
	errUnknownType     = errors.New("unknown frame type")
	errUnknownField    = errors.New("unknown field")
	errInvalidNumber   = errors.New("invalid number")
	errIllegalState    = errors.New("illegal protocol state")
	errInvalidValue    = errors.New("invalid value")
	errInvalidLimits   = errors.New("invalid limits")
	errInvalidManifest = errors.New("invalid manifest")
)

const (
	nativeCallPrefix   = `{"type":"native_call","capability_id":`
	nativeCallMid      = `,"arguments":`
	nativeCallSuffix   = `}`
	finalPrefix        = `{"type":"final","result":`
	finalSuffix        = `}`
	finalErrorLongest  = `{"type":"final_error","code":"invalid_arguments"}`
	nativeResultPrefix = `{"type":"native_result","result":`
	nativeResultSuffix = `}`
	nativeAbortPayload = `{"type":"native_abort"}`
	emptyJSONString    = `""`
)

// probeFrame is the parent's first and only probe request.
type probeFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Version is the private protocol version.
	Version int `json:"version"`
}

// probeAckFrame is the child's successful probe reply.
type probeAckFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Version is the private protocol version.
	Version int `json:"version"`
}

// execFrame is the parent's first execution request.
type execFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Version is the private protocol version.
	Version int `json:"version"`

	// Source is the Starlark program.
	Source string `json:"source"`

	// Limits are the child-visible execution budgets.
	Limits childLimits `json:"limits"`

	// Manifest lists enabled capabilities for this child.
	Manifest []manifestEntry `json:"manifest"`
}

// nativeCallFrame is one child-originated capability invocation.
type nativeCallFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// CapabilityID is the enabled capability identity.
	CapabilityID string `json:"capability_id"`

	// Arguments is the fresh canonical argument map.
	Arguments map[string]any `json:"arguments"`
}

// nativeResultFrame is the parent's successful native response.
type nativeResultFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Result is one normalized process-neutral value.
	Result any `json:"result"`
}

// nativeAbortFrame is the parent's terminal native failure.
type nativeAbortFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`
}

// finalFrame is the child's successful terminal result.
type finalFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Result is one normalized process-neutral value.
	Result any `json:"result"`
}

// finalErrorFrame is the child's terminal classified failure.
type finalErrorFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Code is one child-owned final error class.
	Code finalErrorCode `json:"code"`
}

type connKind uint8

const (
	connKindProbe connKind = iota
	connKindExec
)

type connState uint8

const (
	stateInit connState = iota
	stateAwaitPeer
	stateReady
	stateAwaitNative
	stateDone
)

// parentConn is the parent side of one probe or execution exchange.
type parentConn struct {
	// w receives parent-originated frames.
	w io.Writer

	// r supplies child-originated frames.
	r io.Reader

	// writeCap bounds parent-originated payloads after the handshake.
	writeCap uint32

	// readCap bounds untrusted child-originated payloads.
	readCap uint32

	// kind selects probe or execution rules.
	kind connKind

	// state is the current protocol phase.
	state connState

	// nativeCalls is the number of accepted native_call frames.
	nativeCalls uint64

	// maxNativeCalls caps accepted native_call frames.
	maxNativeCalls uint64

	// limits are the child budgets from the written exec frame.
	limits childLimits

	// hasLimits reports whether exec limits have been stored.
	hasLimits bool

	// remainingIntermediate is the unused native-result value-body budget.
	remainingIntermediate int
}

// childConn is the child side of one probe or execution exchange.
type childConn struct {
	// r supplies parent-originated frames.
	r io.Reader

	// w receives child-originated frames.
	w io.Writer

	// execReadCap bounds the initial trusted exec payload.
	execReadCap uint32

	// parentReadCap bounds later parent-originated payloads.
	parentReadCap uint32

	// writeCap bounds child-originated payloads.
	writeCap uint32

	// kind selects probe or execution rules.
	kind connKind

	// state is the current protocol phase.
	state connState

	// nativeCalls is the number of written native_call frames.
	nativeCalls uint64

	// limits are the child budgets from the received exec frame.
	limits childLimits

	// hasLimits reports whether exec limits have been stored.
	hasLimits bool
}

// writeFrame writes one length-prefixed payload after checking the bound.
func writeFrame(w io.Writer, payload []byte, maxPayload uint32) error {
	if w == nil {
		return errIllegalState
	}
	if len(payload) == 0 {
		return errEmptyPayload
	}
	if err := rejectOverCap(len(payload), maxPayload); err != nil {
		return err
	}
	length, err := uint32FromInt(len(payload))
	if err != nil {
		return err
	}
	var prefix [framePrefixSize]byte
	binary.BigEndian.PutUint32(prefix[:], length)
	if err := writeFull(w, prefix[:]); err != nil {
		return err
	}
	return writeFull(w, payload)
}

// readFrame reads one length-prefixed payload after checking the bound.
func readFrame(r io.Reader, maxPayload uint32) ([]byte, error) {
	if r == nil {
		return nil, errIllegalState
	}
	var prefix [framePrefixSize]byte
	if err := readFull(r, prefix[:]); err != nil {
		return nil, classifyReadErr(err)
	}
	length := binary.BigEndian.Uint32(prefix[:])
	if length == 0 {
		return nil, errEmptyPayload
	}
	if length > maxPayload {
		return nil, errFrameTooLarge
	}
	payload := make([]byte, length)
	if err := readFull(r, payload); err != nil {
		return nil, classifyReadErr(err)
	}
	return payload, nil
}

// encodeProbe encodes the compact protocol-1 probe request.
func encodeProbe() ([]byte, error) {
	return marshalFrame(probeFrame{Type: frameTypeProbe, Version: protocolVersion})
}

// encodeProbeAck encodes the compact protocol-1 probe acknowledgement.
func encodeProbeAck() ([]byte, error) {
	return marshalFrame(probeAckFrame{Type: frameTypeProbeAck, Version: protocolVersion})
}

// encodeExec encodes one execution request after validating limits and manifest.
func encodeExec(frame execFrame) ([]byte, error) {
	if err := validateChildLimits(frame.Limits); err != nil {
		return nil, err
	}
	if err := validateManifest(frame.Manifest); err != nil {
		return nil, err
	}
	if frame.Manifest == nil {
		frame.Manifest = []manifestEntry{}
	}
	frame.Type = frameTypeExec
	frame.Version = protocolVersion
	return marshalFrame(frame)
}

// encodeNativeCall encodes one child-originated native invocation.
func encodeNativeCall(capabilityID string, arguments map[string]any) ([]byte, error) {
	if arguments == nil {
		return nil, errInvalidValue
	}
	encodedArgs, err := encodeNormalizedValue(arguments)
	if err != nil {
		return nil, err
	}
	return encodeNativeCallBytes(capabilityID, encodedArgs)
}

// encodeNativeCallBytes wraps one already-encoded argument body in a native_call frame.
func encodeNativeCallBytes(capabilityID string, encodedArgs []byte) ([]byte, error) {
	if capabilityID == "" {
		return nil, errInvalidValue
	}
	encodedID, err := json.Marshal(capabilityID)
	if err != nil {
		return nil, errInvalidValue
	}
	var buf bytes.Buffer
	buf.Grow(len(nativeCallPrefix) + len(encodedID) + len(nativeCallMid) + len(encodedArgs) + len(nativeCallSuffix))
	buf.WriteString(nativeCallPrefix)
	buf.Write(encodedID)
	buf.WriteString(nativeCallMid)
	buf.Write(encodedArgs)
	buf.WriteString(nativeCallSuffix)
	return buf.Bytes(), nil
}

// encodeNativeResult encodes one parent-originated native success.
func encodeNativeResult(result any) ([]byte, error) {
	encoded, err := encodeNormalizedValue(result)
	if err != nil {
		return nil, err
	}
	return encodeNativeResultBytes(encoded), nil
}

// encodeNativeResultBytes wraps one already-encoded value body in a native_result frame.
func encodeNativeResultBytes(encoded []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(nativeResultPrefix) + len(encoded) + len(nativeResultSuffix))
	buf.WriteString(nativeResultPrefix)
	buf.Write(encoded)
	buf.WriteString(nativeResultSuffix)
	return buf.Bytes()
}

// encodeNativeAbort encodes the payload-free parent abort frame.
func encodeNativeAbort() ([]byte, error) {
	return []byte(nativeAbortPayload), nil
}

// encodeFinal encodes one successful terminal child result.
func encodeFinal(result any) ([]byte, error) {
	encoded, err := encodeNormalizedValue(result)
	if err != nil {
		return nil, err
	}
	return encodeFinalBytes(encoded), nil
}

// encodeFinalBytes wraps one already-encoded value body in a final frame.
func encodeFinalBytes(encoded []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(finalPrefix) + len(encoded) + len(finalSuffix))
	buf.WriteString(finalPrefix)
	buf.Write(encoded)
	buf.WriteString(finalSuffix)
	return buf.Bytes()
}

// encodeFinalError encodes one child-owned terminal failure.
func encodeFinalError(code finalErrorCode) ([]byte, error) {
	if !knownFinalError(code) {
		return nil, errInvalidValue
	}
	return marshalFrame(finalErrorFrame{Type: frameTypeFinalError, Code: code})
}

// decodePayload decodes exactly one UTF-8 JSON frame object.
func decodePayload(payload []byte) (any, error) {
	if !utf8.Valid(payload) {
		return nil, errInvalidUTF8
	}
	previewType, err := decodeType(payload)
	if err != nil {
		return nil, err
	}
	switch previewType {
	case frameTypeProbe:
		frame, err := decodeVersioned(payload, frameTypeProbe)
		if err != nil {
			return nil, err
		}
		return frame, nil
	case frameTypeProbeAck:
		frame, err := decodeVersioned(payload, frameTypeProbeAck)
		if err != nil {
			return nil, err
		}
		return probeAckFrame(frame), nil
	case frameTypeExec:
		return decodeExec(payload)
	case frameTypeNativeCall:
		return decodeNativeCall(payload)
	case frameTypeNativeResult:
		result, err := decodeResult(payload, frameTypeNativeResult)
		if err != nil {
			return nil, err
		}
		return nativeResultFrame{Type: frameTypeNativeResult, Result: result}, nil
	case frameTypeNativeAbort:
		if err := decodeStrict(payload, &nativeAbortFrame{}); err != nil {
			return nil, err
		}
		return nativeAbortFrame{Type: frameTypeNativeAbort}, nil
	case frameTypeFinal:
		result, err := decodeResult(payload, frameTypeFinal)
		if err != nil {
			return nil, err
		}
		return finalFrame{Type: frameTypeFinal, Result: result}, nil
	case frameTypeFinalError:
		return decodeFinalError(payload)
	case "":
		return nil, errMalformedJSON
	default:
		return nil, errUnknownType
	}
}

// newParentProbeConn constructs the parent side of a probe exchange.
func newParentProbeConn(w io.Writer, r io.Reader) *parentConn {
	return &parentConn{
		w:        w,
		r:        r,
		writeCap: probeCap,
		readCap:  probeCap,
		kind:     connKindProbe,
		state:    stateInit,
	}
}

// newChildProbeConn constructs the child side of a probe exchange.
func newChildProbeConn(r io.Reader, w io.Writer) *childConn {
	return &childConn{
		r:             r,
		w:             w,
		execReadCap:   probeCap,
		parentReadCap: probeCap,
		writeCap:      probeCap,
		kind:          connKindProbe,
		state:         stateInit,
	}
}

// newParentExecConn constructs the parent side of an execution exchange.
func newParentExecConn(w io.Writer, r io.Reader, writeCap uint32, readCap uint32, intermediateBudget int) *parentConn {
	return &parentConn{
		w:                     w,
		r:                     r,
		writeCap:              writeCap,
		readCap:               readCap,
		kind:                  connKindExec,
		state:                 stateInit,
		remainingIntermediate: intermediateBudget,
	}
}

// newChildExecConn constructs the child side of an execution exchange.
func newChildExecConn(r io.Reader, w io.Writer, execReadCap uint32, parentReadCap uint32, writeCap uint32) *childConn {
	return &childConn{
		r:             r,
		w:             w,
		execReadCap:   execReadCap,
		parentReadCap: parentReadCap,
		writeCap:      writeCap,
		kind:          connKindExec,
		state:         stateInit,
	}
}

// writeProbe writes the first and only parent probe request.
func (c *parentConn) writeProbe() error {
	if c == nil || c.kind != connKindProbe || c.state != stateInit {
		return errIllegalState
	}
	payload, err := encodeProbe()
	if err != nil {
		return err
	}
	if err := writeFrame(c.w, payload, c.writeCap); err != nil {
		return err
	}
	c.state = stateAwaitPeer
	return nil
}

// writeExec writes the first parent execution request.
func (c *parentConn) writeExec(frame execFrame) error {
	if c == nil || c.kind != connKindExec || c.state != stateInit {
		return errIllegalState
	}
	payload, err := encodeExec(frame)
	if err != nil {
		return err
	}
	maxPayload, err := execPayloadCap(frame.Limits.MaxSourceBytes, frame.Manifest)
	if err != nil {
		return err
	}
	if err := writeFrame(c.w, payload, maxPayload); err != nil {
		return err
	}
	c.limits = frame.Limits
	c.hasLimits = true
	c.maxNativeCalls = frame.Limits.MaxNativeCalls
	c.state = stateReady
	return nil
}

// writeNativeResult writes the response to the outstanding native call.
func (c *parentConn) writeNativeResult(result any) error {
	if c == nil || c.kind != connKindExec || c.state != stateAwaitNative || !c.hasLimits {
		return errIllegalState
	}
	encoded, err := encodeBoundedValue(result, c.limits)
	if err != nil {
		return err
	}
	if len(encoded) > c.remainingIntermediate {
		return errFrameTooLarge
	}
	payload := encodeNativeResultBytes(encoded)
	if err := writeFrame(c.w, payload, c.writeCap); err != nil {
		return err
	}
	c.remainingIntermediate -= len(encoded)
	c.state = stateReady
	return nil
}

// writeNativeAbort writes the terminal parent-owned native failure.
func (c *parentConn) writeNativeAbort() error {
	if c == nil || c.kind != connKindExec || c.state != stateAwaitNative {
		return errIllegalState
	}
	payload, err := encodeNativeAbort()
	if err != nil {
		return err
	}
	if err := writeFrame(c.w, payload, c.writeCap); err != nil {
		return err
	}
	c.state = stateDone
	return nil
}

// read consumes the next legal child frame.
func (c *parentConn) read() (any, error) {
	if c == nil || c.state != stateAwaitPeer && c.state != stateReady {
		return nil, errIllegalState
	}
	payload, err := readFrame(c.r, c.readCap)
	if err != nil {
		return nil, err
	}
	frame, err := decodePayload(payload)
	if err != nil {
		return nil, err
	}
	switch typed := frame.(type) {
	case probeAckFrame:
		return c.acceptProbeAck(typed)
	case nativeCallFrame:
		return c.acceptNativeCall(typed)
	case finalFrame:
		return c.acceptFinal(typed)
	case finalErrorFrame:
		return c.acceptFinalError(typed)
	default:
		return nil, errIllegalState
	}
}

// acceptProbeAck records a successful probe acknowledgement.
func (c *parentConn) acceptProbeAck(frame probeAckFrame) (any, error) {
	if c.kind != connKindProbe || c.state != stateAwaitPeer || frame.Version != protocolVersion {
		return nil, errIllegalState
	}
	c.state = stateDone
	return frame, nil
}

// acceptNativeCall records one in-budget child native invocation.
func (c *parentConn) acceptNativeCall(frame nativeCallFrame) (any, error) {
	if c.kind != connKindExec || c.state != stateReady || !c.hasLimits {
		return nil, errIllegalState
	}
	if c.nativeCalls >= c.maxNativeCalls {
		return nil, errIllegalState
	}
	if err := validateBoundedValue(frame.Arguments, c.limits); err != nil {
		return nil, err
	}
	c.nativeCalls++
	c.state = stateAwaitNative
	return frame, nil
}

// acceptFinal records a successful child terminal result.
func (c *parentConn) acceptFinal(frame finalFrame) (any, error) {
	if c.kind != connKindExec || c.state != stateReady || !c.hasLimits {
		return nil, errIllegalState
	}
	if err := validateBoundedValue(frame.Result, c.limits); err != nil {
		return nil, err
	}
	c.state = stateDone
	return frame, nil
}

// acceptFinalError records a legal child-owned terminal failure.
func (c *parentConn) acceptFinalError(frame finalErrorFrame) (any, error) {
	if c.kind != connKindExec || c.state != stateReady {
		return nil, errIllegalState
	}
	c.state = stateDone
	return frame, nil
}

// writeProbeAck writes the child's successful probe reply.
func (c *childConn) writeProbeAck() error {
	if c == nil || c.kind != connKindProbe || c.state != stateReady {
		return errIllegalState
	}
	payload, err := encodeProbeAck()
	if err != nil {
		return err
	}
	if err := writeFrame(c.w, payload, c.writeCap); err != nil {
		return err
	}
	c.state = stateDone
	return nil
}

// writeNativeCall writes one native invocation and waits for a parent reply.
func (c *childConn) writeNativeCall(capabilityID string, arguments map[string]any) error {
	if c == nil || c.kind != connKindExec || c.state != stateReady || !c.hasLimits {
		return errIllegalState
	}
	if c.nativeCalls >= c.limits.MaxNativeCalls {
		return errIllegalState
	}
	if arguments == nil {
		return errInvalidValue
	}
	encodedArgs, err := encodeBoundedValue(arguments, c.limits)
	if err != nil {
		return err
	}
	payload, err := encodeNativeCallBytes(capabilityID, encodedArgs)
	if err != nil {
		return err
	}
	if err := writeFrame(c.w, payload, c.writeCap); err != nil {
		return err
	}
	c.nativeCalls++
	c.state = stateAwaitNative
	return nil
}

// writeFinal writes the successful terminal child result.
func (c *childConn) writeFinal(result any) error {
	if c == nil || c.kind != connKindExec || c.state != stateReady || !c.hasLimits {
		return errIllegalState
	}
	encoded, err := encodeBoundedValue(result, c.limits)
	if err != nil {
		return err
	}
	payload := encodeFinalBytes(encoded)
	if err := writeFrame(c.w, payload, c.writeCap); err != nil {
		return err
	}
	c.state = stateDone
	return nil
}

// writeFinalError writes one child-owned terminal failure.
func (c *childConn) writeFinalError(code finalErrorCode) error {
	if c == nil || c.kind != connKindExec || c.state != stateReady {
		return errIllegalState
	}
	payload, err := encodeFinalError(code)
	if err != nil {
		return err
	}
	if err := writeFrame(c.w, payload, c.writeCap); err != nil {
		return err
	}
	c.state = stateDone
	return nil
}

// read consumes the next legal parent frame.
func (c *childConn) read() (any, error) {
	if c == nil {
		return nil, errIllegalState
	}
	maxPayload, err := c.readLimit()
	if err != nil {
		return nil, err
	}
	payload, err := readFrame(c.r, maxPayload)
	if err != nil {
		return nil, err
	}
	frame, err := decodePayload(payload)
	if err != nil {
		return nil, err
	}
	switch typed := frame.(type) {
	case probeFrame:
		return c.acceptProbe(typed)
	case execFrame:
		return c.acceptExec(typed)
	case nativeResultFrame:
		return c.acceptNativeResult(typed)
	case nativeAbortFrame:
		return c.acceptNativeAbort(typed)
	default:
		return nil, errIllegalState
	}
}

// readLimit returns the state-specific parent payload bound.
func (c *childConn) readLimit() (uint32, error) {
	switch {
	case c.kind == connKindProbe && c.state == stateInit:
		return c.execReadCap, nil
	case c.kind == connKindExec && c.state == stateInit:
		return c.execReadCap, nil
	case c.kind == connKindExec && c.state == stateAwaitNative:
		return c.parentReadCap, nil
	default:
		return 0, errIllegalState
	}
}

// acceptProbe records the first probe request.
func (c *childConn) acceptProbe(frame probeFrame) (any, error) {
	if c.kind != connKindProbe || c.state != stateInit || frame.Version != protocolVersion {
		return nil, errIllegalState
	}
	c.state = stateReady
	return frame, nil
}

// acceptExec records the first execution request and stores child limits.
func (c *childConn) acceptExec(frame execFrame) (any, error) {
	if c.kind != connKindExec || c.state != stateInit {
		return nil, errIllegalState
	}
	c.limits = frame.Limits
	c.hasLimits = true
	c.state = stateReady
	return frame, nil
}

// acceptNativeResult records the response to the outstanding native call.
func (c *childConn) acceptNativeResult(frame nativeResultFrame) (any, error) {
	if c.kind != connKindExec || c.state != stateAwaitNative || !c.hasLimits {
		return nil, errIllegalState
	}
	if err := validateBoundedValue(frame.Result, c.limits); err != nil {
		return nil, err
	}
	c.state = stateReady
	return frame, nil
}

// acceptNativeAbort records the terminal parent-owned native failure.
func (c *childConn) acceptNativeAbort(frame nativeAbortFrame) (any, error) {
	if c.kind != connKindExec || c.state != stateAwaitNative {
		return nil, errIllegalState
	}
	c.state = stateDone
	return frame, nil
}

// classifyReadErr maps stream failures onto truncated-frame protocol errors.
func classifyReadErr(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errTruncatedFrame
	}
	return err
}

// writeFull writes every byte or fails.
func writeFull(w io.Writer, payload []byte) error {
	written, err := w.Write(payload)
	if err != nil {
		return err
	}
	if written != len(payload) {
		return io.ErrShortWrite
	}
	return nil
}

// readFull reads every requested byte or fails.
func readFull(r io.Reader, payload []byte) error {
	_, err := io.ReadFull(r, payload)
	return err
}

// rejectOverCap rejects a payload that cannot fit the uint32 prefix or bound.
func rejectOverCap(length int, maxPayload uint32) error {
	if length < 0 {
		return errFrameTooLarge
	}
	converted, err := uint32FromInt(length)
	if err != nil {
		return err
	}
	if converted > maxPayload {
		return errFrameTooLarge
	}
	return nil
}

// marshalFrame encodes one concrete frame without a trailing newline.
func marshalFrame(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, errMalformedJSON
	}
	if len(payload) == 0 {
		return nil, errEmptyPayload
	}
	return payload, nil
}
