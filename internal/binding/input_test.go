package binding

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// TestBindShapeAndBindValueAgreeOnSupportedCalls proves child shape binding and parent re-binding share one matrix.
func TestBindShapeAndBindValueAgreeOnSupportedCalls(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	limit := int64(25)

	tests := []struct {
		// name identifies the supported call shape.
		name string

		// kwargs contains the Starlark keyword arguments.
		kwargs []starlark.Tuple

		// want is the exact registered input reconstructed by the parent.
		want representativeInput

		// canonical is the fresh JSON-shaped authorization map.
		canonical map[string]any
	}{
		{
			name: "required string and optional integer",
			kwargs: []starlark.Tuple{
				keyword("org", starlark.String("meigma")),
				keyword("limit", starlark.MakeInt64(limit)),
			},
			want:      representativeInput{Org: "meigma", Limit: &limit},
			canonical: map[string]any{"org": "meigma", "limit": int64(25)},
		},
		{
			name:      "omitted optional integer",
			kwargs:    []starlark.Tuple{keyword("org", starlark.String("meigma"))},
			want:      representativeInput{Org: "meigma"},
			canonical: map[string]any{"org": "meigma"},
		},
		{
			name: "explicit None optional integer",
			kwargs: []starlark.Tuple{
				keyword("org", starlark.String("meigma")),
				keyword("limit", starlark.None),
			},
			want:      representativeInput{Org: "meigma"},
			canonical: map[string]any{"org": "meigma"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child, err := BindShape(plan.InputShape(), nil, tt.kwargs)
			require.NoError(t, err)
			assert.Equal(t, tt.canonical, child)

			bound, canonical, err := plan.BindValue(child)
			require.NoError(t, err)
			typed, ok := bound.(representativeInput)
			require.True(t, ok, "parent must reconstruct the exact registered input type")
			assert.Equal(t, tt.want, typed)
			assert.Equal(t, tt.canonical, canonical)
			child["org"] = "mutated-child"
			assert.Equal(t, tt.canonical, canonical)
		})
	}
}

// TestBindShapeHandlesAnEmptyInputShape proves a zero-field manifest binds no arguments.
func TestBindShapeHandlesAnEmptyInputShape(t *testing.T) {
	plan, err := CompileFor[struct{}, representativeOutput]()
	require.NoError(t, err)

	child, err := BindShape(plan.InputShape(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, child)

	bound, canonical, err := plan.BindValue(child)
	require.NoError(t, err)
	assert.Equal(t, struct{}{}, bound)
	assert.Empty(t, canonical)
}

// TestBindValueReturnsFreshCanonicalMaps proves mutating the decoded child map cannot change parent results.
func TestBindValueReturnsFreshCanonicalMaps(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	decoded := map[string]any{"org": "meigma", "limit": int64(25)}

	bound, canonical, err := plan.BindValue(decoded)
	require.NoError(t, err)
	decoded["org"] = "mutated"
	decoded["limit"] = int64(99)
	canonical["org"] = "mutated-canonical"

	typed, ok := bound.(representativeInput)
	require.True(t, ok)
	require.NotNil(t, typed.Limit)
	assert.Equal(t, "meigma", typed.Org)
	assert.Equal(t, int64(25), *typed.Limit)

	_, second, err := plan.BindValue(map[string]any{"org": "meigma", "limit": int64(25)})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"org": "meigma", "limit": int64(25)}, second)
}

// TestBindShapeRejectsMalformedArguments proves child-side validation stays a caller argument error.
func TestBindShapeRejectsMalformedArguments(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	overflow := new(big.Int).Lsh(big.NewInt(1), 80)

	tests := []struct {
		// name identifies the malformed call.
		name string

		// args contains positional arguments.
		args starlark.Tuple

		// kwargs contains keyword arguments.
		kwargs []starlark.Tuple

		// contains is the expected safe diagnostic fragment.
		contains string
	}{
		{name: "positional", args: starlark.Tuple{starlark.String("meigma")}, contains: "positional"},
		{name: "missing required", contains: "missing required"},
		{name: "duplicate", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("one")),
			keyword("org", starlark.String("two")),
		}, contains: "duplicate"},
		{name: "unknown", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("other", starlark.String("value")),
		}, contains: "unknown"},
		{name: "mistyped required string", kwargs: []starlark.Tuple{
			keyword("org", starlark.MakeInt(1)),
		}, contains: "must be a string"},
		{name: "mistyped optional integer", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("limit", starlark.String("many")),
		}, contains: "integer or None"},
		{name: "overflowing optional integer", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("limit", starlark.MakeBigInt(overflow)),
		}, contains: "overflows int64"},
		{name: "malformed keyword", kwargs: []starlark.Tuple{{starlark.String("org")}}, contains: "malformed keyword"},
		{
			name:     "non-string keyword",
			kwargs:   []starlark.Tuple{{starlark.MakeInt(1), starlark.String("value")}},
			contains: "keyword name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BindShape(plan.InputShape(), tt.args, tt.kwargs)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidArguments)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestBindValueRejectsNormalizedWireMismatches proves parent re-binding never accepts a decoded child map as-is.
func TestBindValueRejectsNormalizedWireMismatches(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	tests := []struct {
		// name identifies the rejected decoded map.
		name string

		// arguments is the decoded child map presented to the parent.
		arguments map[string]any

		// contains is the expected safe diagnostic fragment.
		contains string
	}{
		{name: "missing required", arguments: map[string]any{}, contains: "missing required"},
		{name: "unknown", arguments: map[string]any{"org": "meigma", "other": "value"}, contains: "unknown"},
		{name: "mistyped required string", arguments: map[string]any{"org": int64(1)}, contains: "must be a string"},
		{
			name:      "float64 where int64 is required",
			arguments: map[string]any{"org": "meigma", "limit": float64(25)},
			contains:  "integer or None",
		},
		{
			name:      "json.Number",
			arguments: map[string]any{"org": "meigma", "limit": json.Number("25")},
			contains:  "integer or None",
		},
		{
			name:      "unsupported Go integer",
			arguments: map[string]any{"org": "meigma", "limit": int(25)},
			contains:  "integer or None",
		},
		{
			name:      "unsupported Go unsigned integer",
			arguments: map[string]any{"org": "meigma", "limit": uint64(25)},
			contains:  "integer or None",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := plan.BindValue(tt.arguments)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidArguments)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// keyword constructs one Starlark keyword tuple.
func keyword(name string, value starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), value}
}
