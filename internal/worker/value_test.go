package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/binding"
)

// TestValueCodecPreservesNumericIdentity proves tokens keep int64 and finite float64 apart.
func TestValueCodecPreservesNumericIdentity(t *testing.T) {
	negativeZero := math.Copysign(0, -1)

	tests := []struct {
		// name identifies the numeric identity.
		name string

		// value is the normalized Go value.
		value any

		// json is the exact type-preserving spelling.
		json string
	}{
		{name: "minimum int64", value: int64(math.MinInt64), json: "-9223372036854775808"},
		{name: "maximum int64", value: int64(math.MaxInt64), json: "9223372036854775807"},
		{name: "integer zero", value: int64(0), json: "0"},
		{name: "integral float", value: 1.0, json: "1.0"},
		{name: "fractional float", value: 1.25, json: "1.25"},
		{name: "exponent float", value: 1e20, json: strconv.FormatFloat(1e20, 'g', -1, 64)},
		{name: "negative zero", value: negativeZero, json: "-0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeNormalizedValue(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.json, string(encoded))

			decoded, err := decodeNormalizedValue(json.RawMessage(encoded))
			require.NoError(t, err)
			assert.Equal(t, tt.value, decoded)
			assert.IsType(t, tt.value, decoded)
			if tt.name == "negative zero" {
				got, ok := decoded.(float64)
				require.True(t, ok)
				assert.True(t, math.Signbit(got))
			}
		})
	}
}

// TestValueCodecDecodesTokensBySpelling proves decoder rules follow the token, not Go defaults.
func TestValueCodecDecodesTokensBySpelling(t *testing.T) {
	tests := []struct {
		// name identifies the token class.
		name string

		// token is the raw JSON number.
		token string

		// want is the normalized value.
		want any
	}{
		{name: "integer token", token: "7", want: int64(7)},
		{name: "fraction token", token: "1.0", want: 1.0},
		{name: "exponent token", token: "1e2", want: 100.0},
		{name: "negative exponent", token: "-2.5E+1", want: -25.0},
		{name: "negative zero token", token: "-0.0", want: math.Copysign(0, -1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeNormalizedValue(json.RawMessage(tt.token))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.IsType(t, tt.want, got)
			_, isNumber := got.(json.Number)
			assert.False(t, isNumber, "json.Number must not escape the codec")
		})
	}
}

// TestValueCodecSortsObjectKeys proves maps have a deterministic wire order.
func TestValueCodecSortsObjectKeys(t *testing.T) {
	encoded, err := encodeNormalizedValue(map[string]any{
		"zeta": int64(1),
		"alpha": map[string]any{
			"n": int64(2),
			"a": true,
		},
	})
	require.NoError(t, err)
	assert.True(t, bytes.Equal(encoded, []byte(`{"alpha":{"a":true,"n":2},"zeta":1}`)))
}

// TestValueCodecRejectsUnsupportedValues proves [json.Number] never leaves the codec.
func TestValueCodecRejectsUnsupportedValues(t *testing.T) {
	encodeTests := []struct {
		// name identifies the rejected Go value.
		name string

		// value is presented to the encoder.
		value any

		// target is the expected protocol error.
		target error
	}{
		{name: "json.Number", value: json.Number("1"), target: errInvalidValue},
		{name: "int", value: 1, target: errInvalidValue},
		{name: "uint64", value: uint64(1), target: errInvalidValue},
		{name: "float32", value: float32(1), target: errInvalidValue},
		{name: "NaN", value: math.NaN(), target: errInvalidNumber},
		{name: "infinity", value: math.Inf(1), target: errInvalidNumber},
		{name: "negative infinity", value: math.Inf(-1), target: errInvalidNumber},
		{name: "string map", value: map[string]string{"k": "v"}, target: errInvalidValue},
	}

	for _, tt := range encodeTests {
		t.Run("encode "+tt.name, func(t *testing.T) {
			_, err := encodeNormalizedValue(tt.value)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.target)
		})
	}

	decodeTests := []struct {
		// name identifies the rejected token.
		name string

		// token is raw JSON.
		token string
	}{
		{name: "integer overflow", token: "9223372036854775808"},
		{name: "integer underflow", token: "-9223372036854775809"},
		{name: "NaN token", token: "NaN"},
		{name: "Infinity token", token: "Infinity"},
		{name: "non-string key", token: `{1:true}`},
	}

	for _, tt := range decodeTests {
		t.Run("decode "+tt.name, func(t *testing.T) {
			_, err := decodeNormalizedValue(json.RawMessage(tt.token))
			require.Error(t, err)
			assert.True(
				t,
				errors.Is(err, errInvalidNumber) || errors.Is(err, errInvalidValue) || errors.Is(err, errMalformedJSON),
			)
		})
	}
}

// TestValueCodecUsesBindingValidation proves value domain checks are not reimplemented here.
func TestValueCodecUsesBindingValidation(t *testing.T) {
	value := map[string]any{"org": "meigma", "limit": int64(25), "nested": []any{nil, true, 1.0}}
	require.NoError(t, binding.ValidateValue(value, 4, 1024))

	encoded, err := encodeNormalizedValue(value)
	require.NoError(t, err)
	decoded, err := decodeNormalizedValue(encoded)
	require.NoError(t, err)
	assert.Equal(t, value, decoded)
	require.NoError(t, binding.ValidateValue(decoded, 4, 1024))
}
