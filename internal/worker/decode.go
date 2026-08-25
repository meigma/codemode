package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// rawTypedFrame is the discriminator-only preview used before concrete decode.
type rawTypedFrame struct {
	// Type is the discriminator used to select a concrete frame.
	Type string `json:"type"`
}

// rawVersionFrame is the versioned handshake object for probe and probe_ack.
type rawVersionFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Version is the optional protocol version token.
	Version *int `json:"version"`
}

// rawExecFrame is the raw exec object used to distinguish absent from null fields.
type rawExecFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Version is the protocol version.
	Version *int `json:"version"`

	// Source is the Starlark program token.
	Source *string `json:"source"`

	// Limits is the raw child-limit object.
	Limits json.RawMessage `json:"limits"`

	// Manifest is the raw capability array.
	Manifest json.RawMessage `json:"manifest"`
}

// rawNativeCallFrame is the raw native_call object used to require arguments.
type rawNativeCallFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// CapabilityID is the capability identity token.
	CapabilityID *string `json:"capability_id"`

	// Arguments is the raw argument object.
	Arguments json.RawMessage `json:"arguments"`
}

// rawResultFrame is the raw result object used to require a present result token.
type rawResultFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Result is the raw normalized value token.
	Result json.RawMessage `json:"result"`
}

// rawFinalErrorFrame is the raw final_error object used to require a code.
type rawFinalErrorFrame struct {
	// Type is the frame discriminator.
	Type string `json:"type"`

	// Code is the child-owned error class.
	Code *finalErrorCode `json:"code"`
}

// decodeType reads only the type discriminator without rejecting unknown fields.
func decodeType(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errMalformedJSON
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var preview rawTypedFrame
	if err := dec.Decode(&preview); err != nil {
		return "", classifyJSONErr(err)
	}
	if err := requireEOF(dec); err != nil {
		return "", err
	}
	return preview.Type, nil
}

// decodeVersioned decodes probe or probe_ack and requires protocol version 1.
func decodeVersioned(payload []byte, wantType string) (probeFrame, error) {
	var raw rawVersionFrame
	if err := decodeStrict(payload, &raw); err != nil {
		return probeFrame{}, err
	}
	if raw.Type != wantType {
		return probeFrame{}, errMalformedJSON
	}
	if err := requireProtocolVersion(raw.Version); err != nil {
		return probeFrame{}, err
	}
	return probeFrame{Type: wantType, Version: protocolVersion}, nil
}

// decodeExec decodes one versioned execution request.
func decodeExec(payload []byte) (execFrame, error) {
	var raw rawExecFrame
	if err := decodeStrict(payload, &raw); err != nil {
		return execFrame{}, err
	}
	if raw.Type != frameTypeExec {
		return execFrame{}, errMalformedJSON
	}
	if err := requireProtocolVersion(raw.Version); err != nil {
		return execFrame{}, err
	}
	if raw.Source == nil || isJSONNull(raw.Limits) || isJSONNull(raw.Manifest) {
		return execFrame{}, errInvalidValue
	}
	var limits childLimits
	if err := decodeStrict(raw.Limits, &limits); err != nil {
		return execFrame{}, err
	}
	if err := validateChildLimits(limits); err != nil {
		return execFrame{}, err
	}
	var manifest []manifestEntry
	if err := decodeStrict(raw.Manifest, &manifest); err != nil {
		return execFrame{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return execFrame{}, err
	}
	return execFrame{
		Type:     frameTypeExec,
		Version:  protocolVersion,
		Source:   *raw.Source,
		Limits:   limits,
		Manifest: manifest,
	}, nil
}

// decodeNativeCall decodes one native invocation with a required argument object.
func decodeNativeCall(payload []byte) (nativeCallFrame, error) {
	var raw rawNativeCallFrame
	if err := decodeStrict(payload, &raw); err != nil {
		return nativeCallFrame{}, err
	}
	if raw.Type != frameTypeNativeCall {
		return nativeCallFrame{}, errMalformedJSON
	}
	if raw.CapabilityID == nil || *raw.CapabilityID == "" || isJSONNull(raw.Arguments) {
		return nativeCallFrame{}, errInvalidValue
	}
	value, err := decodeNormalizedValue(raw.Arguments)
	if err != nil {
		return nativeCallFrame{}, err
	}
	arguments, ok := value.(map[string]any)
	if !ok {
		return nativeCallFrame{}, errInvalidValue
	}
	return nativeCallFrame{
		Type:         frameTypeNativeCall,
		CapabilityID: *raw.CapabilityID,
		Arguments:    arguments,
	}, nil
}

// decodeResult decodes a required result token, including explicit null.
func decodeResult(payload []byte, wantType string) (any, error) {
	var raw rawResultFrame
	if err := decodeStrict(payload, &raw); err != nil {
		return nil, err
	}
	if raw.Type != wantType {
		return nil, errMalformedJSON
	}
	if raw.Result == nil {
		return nil, errInvalidValue
	}
	return decodeNormalizedValue(raw.Result)
}

// decodeFinalError decodes one known child-owned terminal code.
func decodeFinalError(payload []byte) (finalErrorFrame, error) {
	var raw rawFinalErrorFrame
	if err := decodeStrict(payload, &raw); err != nil {
		return finalErrorFrame{}, err
	}
	if raw.Type != frameTypeFinalError {
		return finalErrorFrame{}, errMalformedJSON
	}
	if raw.Code == nil || !knownFinalError(*raw.Code) {
		return finalErrorFrame{}, errInvalidValue
	}
	return finalErrorFrame{Type: frameTypeFinalError, Code: *raw.Code}, nil
}

// requireProtocolVersion rejects a missing or mismatched protocol version.
func requireProtocolVersion(version *int) error {
	const mismatch = "%w: protocol version mismatch: child reported %s, parent requires %d"
	if version == nil {
		return fmt.Errorf(mismatch, errInvalidValue, "<nil>", protocolVersion)
	}
	if *version != protocolVersion {
		return fmt.Errorf(mismatch, errInvalidValue, strconv.Itoa(*version), protocolVersion)
	}
	return nil
}

// decodeStrict decodes exactly one JSON value and rejects unknown fields.
func decodeStrict(payload []byte, dest any) error {
	if len(payload) == 0 {
		return errMalformedJSON
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return classifyJSONErr(err)
	}
	return requireEOF(dec)
}

// requireEOF rejects a trailing JSON value after the one legal object.
func requireEOF(dec *json.Decoder) error {
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return errTrailingJSON
		}
		return errTrailingJSON
	}
	return nil
}

// classifyJSONErr maps encoding/json failures onto protocol sentinels.
func classifyJSONErr(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.Contains(message, "unknown field") {
		return errUnknownField
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return errMalformedJSON
	}
	var unmarshalType *json.UnmarshalTypeError
	if errors.As(err, &unmarshalType) {
		return errMalformedJSON
	}
	return errMalformedJSON
}

// isJSONNull reports whether raw is absent or an explicit JSON null token.
func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(raw, []byte("null"))
}
