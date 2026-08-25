package binding

import (
	"encoding/json"
	"maps"
	"math"
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

// TestBindShapeAndBindValueAgreeOnWidenedScalars proves all eight forms share child and parent behavior.
func TestBindShapeAndBindValueAgreeOnWidenedScalars(t *testing.T) {
	plan, err := CompileFor[widenedInput, representativeOutput]()
	require.NoError(t, err)
	label := "beta"
	limit := int64(25)
	enabled := true
	weight := 2.5

	tests := []struct {
		// name identifies the supported call shape.
		name string

		// kwargs contains the Starlark keyword arguments.
		kwargs []starlark.Tuple

		// want is the exact registered input reconstructed by the parent.
		want widenedInput

		// canonical is the fresh JSON-shaped authorization map.
		canonical map[string]any
	}{
		{
			name: "all eight present values",
			kwargs: []starlark.Tuple{
				keyword("org", starlark.String("meigma")),
				keyword("count", starlark.MakeInt64(3)),
				keyword("active", starlark.True),
				keyword("score", starlark.Float(1.5)),
				keyword("label", starlark.String(label)),
				keyword("limit", starlark.MakeInt64(limit)),
				keyword("enabled", starlark.True),
				keyword("weight", starlark.Float(weight)),
			},
			want: widenedInput{
				Org:     "meigma",
				Count:   3,
				Active:  true,
				Score:   1.5,
				Label:   &label,
				Limit:   &limit,
				Enabled: &enabled,
				Weight:  &weight,
			},
			canonical: map[string]any{
				"org":     "meigma",
				"count":   int64(3),
				"active":  true,
				"score":   1.5,
				"label":   "beta",
				"limit":   int64(25),
				"enabled": true,
				"weight":  2.5,
			},
		},
		{
			name: "omitted optional scalars",
			kwargs: []starlark.Tuple{
				keyword("org", starlark.String("meigma")),
				keyword("count", starlark.MakeInt64(3)),
				keyword("active", starlark.False),
				keyword("score", starlark.Float(0)),
			},
			want: widenedInput{Org: "meigma", Count: 3, Score: 0},
			canonical: map[string]any{
				"org":    "meigma",
				"count":  int64(3),
				"active": false,
				"score":  float64(0),
			},
		},
		{
			name: "explicit None optional scalars",
			kwargs: []starlark.Tuple{
				keyword("org", starlark.String("meigma")),
				keyword("count", starlark.MakeInt64(3)),
				keyword("active", starlark.True),
				keyword("score", starlark.Float(1.5)),
				keyword("label", starlark.None),
				keyword("limit", starlark.None),
				keyword("enabled", starlark.None),
				keyword("weight", starlark.None),
			},
			want: widenedInput{Org: "meigma", Count: 3, Active: true, Score: 1.5},
			canonical: map[string]any{
				"org":    "meigma",
				"count":  int64(3),
				"active": true,
				"score":  1.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child, err := BindShape(plan.InputShape(), nil, tt.kwargs)
			require.NoError(t, err)
			assert.Equal(t, tt.canonical, child)

			bound, canonical, err := plan.BindValue(child)
			require.NoError(t, err)
			typed, ok := bound.(widenedInput)
			require.True(t, ok, "parent must reconstruct the exact registered input type")
			assert.Equal(t, tt.want, typed)
			assert.Equal(t, tt.canonical, canonical)
			child["org"] = "mutated-child"
			assert.Equal(t, tt.canonical, canonical)
			_, hasLabel := canonical["label"]
			_, hasLimit := canonical["limit"]
			_, hasEnabled := canonical["enabled"]
			_, hasWeight := canonical["weight"]
			if tt.want.Label == nil {
				assert.False(t, hasLabel)
				assert.Nil(t, typed.Label)
			}
			if tt.want.Limit == nil {
				assert.False(t, hasLimit)
				assert.Nil(t, typed.Limit)
			}
			if tt.want.Enabled == nil {
				assert.False(t, hasEnabled)
				assert.Nil(t, typed.Enabled)
			}
			if tt.want.Weight == nil {
				assert.False(t, hasWeight)
				assert.Nil(t, typed.Weight)
			}
		})
	}
}

// TestBindShapeAndBindValueAgreeOnNamedAliases proves aliases bind by underlying kind.
func TestBindShapeAndBindValueAgreeOnNamedAliases(t *testing.T) {
	plan, err := CompileFor[aliasedInput, representativeOutput]()
	require.NoError(t, err)
	label := orgName("beta")
	limit := itemCount(25)
	enabled := flag(true)
	weight := score(2.5)

	child, err := BindShape(plan.InputShape(), nil, []starlark.Tuple{
		keyword("org", starlark.String("meigma")),
		keyword("count", starlark.MakeInt64(3)),
		keyword("active", starlark.True),
		keyword("score", starlark.Float(1.5)),
		keyword("label", starlark.String("beta")),
		keyword("limit", starlark.MakeInt64(25)),
		keyword("enabled", starlark.True),
		keyword("weight", starlark.Float(2.5)),
	})
	require.NoError(t, err)

	bound, canonical, err := plan.BindValue(child)
	require.NoError(t, err)
	typed, ok := bound.(aliasedInput)
	require.True(t, ok, "parent must reconstruct the exact named-alias input type")
	assert.Equal(t, aliasedInput{
		Org:     "meigma",
		Count:   3,
		Active:  true,
		Score:   1.5,
		Label:   &label,
		Limit:   &limit,
		Enabled: &enabled,
		Weight:  &weight,
	}, typed)
	assert.Equal(t, map[string]any{
		"org":     "meigma",
		"count":   int64(3),
		"active":  true,
		"score":   1.5,
		"label":   "beta",
		"limit":   int64(25),
		"enabled": true,
		"weight":  2.5,
	}, canonical)
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

// TestBindShapeRejectsWidenedScalarMismatches proves category, range, and finiteness fail before authorization.
func TestBindShapeRejectsWidenedScalarMismatches(t *testing.T) {
	plan, err := CompileFor[widenedInput, representativeOutput]()
	require.NoError(t, err)
	overflow := new(big.Int).Lsh(big.NewInt(1), 80)
	required := []starlark.Tuple{
		keyword("org", starlark.String("meigma")),
		keyword("count", starlark.MakeInt64(3)),
		keyword("active", starlark.True),
		keyword("score", starlark.Float(1.5)),
	}

	tests := []struct {
		// name identifies the malformed call.
		name string

		// kwargs contains keyword arguments.
		kwargs []starlark.Tuple

		// contains is the expected safe diagnostic fragment.
		contains string
	}{
		{name: "missing required integer", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("active", starlark.True),
			keyword("score", starlark.Float(1.5)),
		}, contains: "missing required"},
		{name: "integer supplied as float", kwargs: append(append([]starlark.Tuple{}, required[:2]...),
			keyword("active", starlark.True),
			keyword("score", starlark.MakeInt64(1)),
		), contains: "must be a float"},
		{name: "float supplied as integer", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("count", starlark.Float(3)),
			keyword("active", starlark.True),
			keyword("score", starlark.Float(1.5)),
		}, contains: "must be an integer"},
		{name: "bool supplied as integer", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("count", starlark.True),
			keyword("active", starlark.True),
			keyword("score", starlark.Float(1.5)),
		}, contains: "must be an integer"},
		{name: "overflowing required integer", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("count", starlark.MakeBigInt(overflow)),
			keyword("active", starlark.True),
			keyword("score", starlark.Float(1.5)),
		}, contains: "overflows int64"},
		{name: "NaN float", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("count", starlark.MakeInt64(3)),
			keyword("active", starlark.True),
			keyword("score", starlark.Float(math.NaN())),
		}, contains: "not finite"},
		{name: "positive infinity", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("count", starlark.MakeInt64(3)),
			keyword("active", starlark.True),
			keyword("score", starlark.Float(math.Inf(1))),
		}, contains: "not finite"},
		{name: "negative infinity", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("count", starlark.MakeInt64(3)),
			keyword("active", starlark.True),
			keyword("score", starlark.Float(math.Inf(-1))),
		}, contains: "not finite"},
		{name: "optional float NaN", kwargs: append(append([]starlark.Tuple{}, required...),
			keyword("weight", starlark.Float(math.NaN())),
		), contains: "not finite"},
		{name: "optional integer overflow", kwargs: append(append([]starlark.Tuple{}, required...),
			keyword("limit", starlark.MakeBigInt(overflow)),
		), contains: "overflows int64"},
		{name: "optional bool as integer", kwargs: append(append([]starlark.Tuple{}, required...),
			keyword("enabled", starlark.MakeInt64(1)),
		), contains: "bool or None"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BindShape(plan.InputShape(), nil, tt.kwargs)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidArguments)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestBindValueRejectsWidenedNormalizedMismatches proves parent re-binding rejects coerced or non-finite values.
func TestBindValueRejectsWidenedNormalizedMismatches(t *testing.T) {
	plan, err := CompileFor[widenedInput, representativeOutput]()
	require.NoError(t, err)
	required := map[string]any{
		"org":    "meigma",
		"count":  int64(3),
		"active": true,
		"score":  1.5,
	}

	tests := []struct {
		// name identifies the rejected decoded map.
		name string

		// arguments is the decoded child map presented to the parent.
		arguments map[string]any

		// contains is the expected safe diagnostic fragment.
		contains string
	}{
		{name: "missing required bool", arguments: map[string]any{
			"org":   "meigma",
			"count": int64(3),
			"score": 1.5,
		}, contains: "missing required"},
		{name: "float64 where int64 is required", arguments: map[string]any{
			"org":    "meigma",
			"count":  float64(3),
			"active": true,
			"score":  1.5,
		}, contains: "must be an integer"},
		{name: "int64 where float64 is required", arguments: map[string]any{
			"org":    "meigma",
			"count":  int64(3),
			"active": true,
			"score":  int64(1),
		}, contains: "must be a float"},
		{name: "bool where int64 is required", arguments: map[string]any{
			"org":    "meigma",
			"count":  true,
			"active": true,
			"score":  1.5,
		}, contains: "must be an integer"},
		{name: "json.Number float", arguments: map[string]any{
			"org":    "meigma",
			"count":  int64(3),
			"active": true,
			"score":  json.Number("1.5"),
		}, contains: "must be a float"},
		{name: "NaN float", arguments: map[string]any{
			"org":    "meigma",
			"count":  int64(3),
			"active": true,
			"score":  math.NaN(),
		}, contains: "not finite"},
		{name: "positive infinity", arguments: map[string]any{
			"org":    "meigma",
			"count":  int64(3),
			"active": true,
			"score":  math.Inf(1),
		}, contains: "not finite"},
		{
			name:      "optional float infinity",
			arguments: withArgument(required, "weight", math.Inf(-1)),
			contains:  "not finite",
		},
		{
			name:      "optional integer json.Number",
			arguments: withArgument(required, "limit", json.Number("25")),
			contains:  "integer or None",
		},
		{name: "unknown extra key", arguments: withArgument(required, "other", "value"), contains: "unknown"},
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

// withArgument returns a copy of base with one extra or replaced key.
func withArgument(base map[string]any, name string, value any) map[string]any {
	copied := make(map[string]any, len(base)+1)
	maps.Copy(copied, base)
	copied[name] = value
	return copied
}

// keyword constructs one Starlark keyword tuple.
func keyword(name string, value starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), value}
}
