package binding

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// TestBindAsBuildsTypedInputAndCanonicalArguments proves one plan traversal produces both projections.
func TestBindAsBuildsTypedInputAndCanonicalArguments(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	bound, canonical, err := BindAs[representativeInput](plan, nil, []starlark.Tuple{
		keyword("org", starlark.String("meigma")),
		keyword("limit", starlark.MakeInt64(25)),
	})

	require.NoError(t, err)
	require.NotNil(t, bound.Limit)
	assert.Equal(t, "meigma", bound.Org)
	assert.Equal(t, int64(25), *bound.Limit)
	assert.Equal(t, map[string]any{"org": "meigma", "limit": int64(25)}, canonical)
}

// TestBindAsNormalizesOmittedAndExplicitNone proves both optional forms produce nil and omit the canonical key.
func TestBindAsNormalizesOmittedAndExplicitNone(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	tests := []struct {
		// name identifies the optional argument form.
		name string

		// kwargs contains the Starlark keyword arguments.
		kwargs []starlark.Tuple
	}{
		{name: "omitted", kwargs: []starlark.Tuple{keyword("org", starlark.String("meigma"))}},
		{name: "explicit None", kwargs: []starlark.Tuple{
			keyword("org", starlark.String("meigma")),
			keyword("limit", starlark.None),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bound, canonical, err := BindAs[representativeInput](plan, nil, tt.kwargs)

			require.NoError(t, err)
			assert.Nil(t, bound.Limit)
			assert.Equal(t, map[string]any{"org": "meigma"}, canonical)
		})
	}
}

// TestBindReturnsFreshCanonicalMaps proves one invocation cannot mutate another authorization input.
func TestBindReturnsFreshCanonicalMaps(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	kwargs := []starlark.Tuple{keyword("org", starlark.String("meigma"))}

	_, first, err := plan.Bind(nil, kwargs)
	require.NoError(t, err)
	first["org"] = "mutated"
	_, second, err := plan.Bind(nil, kwargs)
	require.NoError(t, err)

	assert.Equal(t, "meigma", second["org"])
}

// TestBindRejectsMalformedArguments proves invalid calls fail before authorization can observe them.
func TestBindRejectsMalformedArguments(t *testing.T) {
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
			_, _, err := plan.Bind(tt.args, tt.kwargs)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidArguments)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestBindAsRejectsTypeDrift proves callers cannot reuse a plan with a different typed input.
func TestBindAsRejectsTypeDrift(t *testing.T) {
	plan, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)

	_, _, err = BindAs[struct{}](plan, nil, nil)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPlan)
}

// keyword constructs one Starlark keyword tuple.
func keyword(name string, value starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(name), value}
}
