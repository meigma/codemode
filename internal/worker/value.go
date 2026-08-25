package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/meigma/codemode/internal/binding"
)

// encodeNormalizedValue encodes one process-neutral value with sorted keys.
func encodeNormalizedValue(value any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeNormalizedValue(&buf, value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeNormalizedValue converts one JSON value into the normalized domain.
func decodeNormalizedValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, errMalformedJSON
	}
	if !utf8.Valid(raw) {
		return nil, errInvalidUTF8
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, classifyJSONErr(err)
	}
	if err := requireEOF(dec); err != nil {
		return nil, err
	}
	return normalizeValue(value)
}

// writeNormalizedValue writes one supported value in canonical JSON form.
func writeNormalizedValue(buf *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if typed {
			buf.WriteString("true")
			return nil
		}
		buf.WriteString("false")
		return nil
	case string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return errInvalidValue
		}
		buf.Write(encoded)
		return nil
	case int64:
		buf.WriteString(strconv.FormatInt(typed, 10))
		return nil
	case float64:
		return writeFloat(buf, typed)
	case []any:
		return writeNormalizedList(buf, typed)
	case map[string]any:
		return writeNormalizedObject(buf, typed)
	default:
		return errInvalidValue
	}
}

// writeNormalizedList writes one process-neutral list.
func writeNormalizedList(buf *bytes.Buffer, value []any) error {
	buf.WriteByte('[')
	for index, item := range value {
		if index > 0 {
			buf.WriteByte(',')
		}
		if err := writeNormalizedValue(buf, item); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// writeNormalizedObject writes one process-neutral object with sorted keys.
func writeNormalizedObject(buf *bytes.Buffer, value map[string]any) error {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	buf.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			buf.WriteByte(',')
		}
		encoded, err := json.Marshal(key)
		if err != nil {
			return errInvalidValue
		}
		buf.Write(encoded)
		buf.WriteByte(':')
		if err := writeNormalizedValue(buf, value[key]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// writeFloat writes one finite float in shortest round-trippable spelling.
func writeFloat(buf *bytes.Buffer, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return errInvalidNumber
	}
	encoded := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(encoded, ".eE") {
		encoded += ".0"
	}
	buf.WriteString(encoded)
	return nil
}

// normalizeValue converts decoded JSON into the process-neutral domain.
func normalizeValue(value any) (any, error) {
	switch typed := value.(type) {
	case nil, bool, string:
		return typed, nil
	case json.Number:
		return decodeNumber(typed)
	case []any:
		normalized := make([]any, len(typed))
		for index, item := range typed {
			converted, err := normalizeValue(item)
			if err != nil {
				return nil, err
			}
			normalized[index] = converted
		}
		return normalized, nil
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			converted, err := normalizeValue(item)
			if err != nil {
				return nil, err
			}
			normalized[key] = converted
		}
		return normalized, nil
	default:
		return nil, errInvalidValue
	}
}

// decodeNumber preserves int64 tokens and finite fractional or exponent floats.
func decodeNumber(number json.Number) (any, error) {
	token := number.String()
	if strings.ContainsAny(token, ".eE") {
		parsed, err := strconv.ParseFloat(token, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, errInvalidNumber
		}
		return parsed, nil
	}
	parsed, err := strconv.ParseInt(token, 10, 64)
	if err != nil {
		return nil, errInvalidNumber
	}
	return parsed, nil
}

// validateBoundedValue enforces the shared type, depth, and encoded-size limits.
func validateBoundedValue(value any, limits childLimits) error {
	if err := binding.ValidateValue(value, limits.MaxValueDepth, limits.MaxValueBytes); err != nil {
		return fmt.Errorf("%w: %w", errInvalidValue, err)
	}
	encoded, err := encodeNormalizedValue(value)
	if err != nil {
		return err
	}
	if len(encoded) > limits.MaxValueBytes {
		return errFrameTooLarge
	}
	return nil
}
