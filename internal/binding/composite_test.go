package binding

import (
	"encoding/json"
	"maps"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// compositeInput exercises every composite input form in one capability.
type compositeInput struct {
	// Tags is a required list of strings.
	Tags []string `json:"tags"`

	// Filter is a required nested struct.
	Filter compositeFilter `json:"filter"`

	// Labels is a required string-keyed map.
	Labels map[string]int64 `json:"labels"`

	// Window is an optional nested struct with non-identifier member names.
	Window *compositeWindow `json:"window"`

	// Payload is a byte slice bound as a list of bounded integers.
	Payload []byte `json:"payload"`

	// Scores is a list of optional float32 values.
	Scores []*float32 `json:"scores"`
}

// compositeFilter is a nested input struct with a narrow integer and an optional float.
type compositeFilter struct {
	// Owner is a required nested string.
	Owner string `json:"owner"`

	// Level is a required unsigned 8-bit integer.
	Level uint8 `json:"level"`

	// MinScore is an optional nested float.
	MinScore *float64 `json:"min_score"`
}

// compositeWindow uses a Starlark keyword and a quoted name as nested members.
type compositeWindow struct {
	// From is named after a reserved Starlark word.
	From string `json:"from"`

	// UntilMS needs quoting in notation and bracket paths in diagnostics.
	UntilMS int32 `json:"until-ms"`
}

// compositeSignature is the exact generated signature for compositeInput.
const compositeSignature = "records.query(*, tags: list[str], " +
	"filter: {owner: str, level: int, min_score: float | None}, labels: dict[str, int], " +
	`window: {from: str, "until-ms": int} | None, payload: list[int], scores: list[float | None])`

// compositeKeywords returns a valid composite call with selected arguments replaced.
func compositeKeywords(overrides map[string]starlark.Value) []starlark.Tuple {
	filter := starlark.NewDict(2)
	_ = filter.SetKey(starlark.String("owner"), starlark.String("ops"))
	_ = filter.SetKey(starlark.String("level"), starlark.MakeInt(7))
	values := map[string]starlark.Value{
		"tags":    starlark.NewList(nil),
		"filter":  filter,
		"labels":  starlark.NewDict(0),
		"payload": starlark.NewList(nil),
		"scores":  starlark.NewList(nil),
	}
	maps.Copy(values, overrides)
	kwargs := make([]starlark.Tuple, 0, len(values))
	for _, name := range []string{"tags", "filter", "labels", "window", "payload", "scores"} {
		if value, ok := values[name]; ok {
			kwargs = append(kwargs, keyword(name, value))
		}
	}
	return kwargs
}

// starlarkDict builds a dict from alternating string keys and values.
func starlarkDict(t *testing.T, pairs ...any) *starlark.Dict {
	t.Helper()
	dict := starlark.NewDict(len(pairs) / 2)
	for index := 0; index < len(pairs); index += 2 {
		key, ok := pairs[index].(starlark.Value)
		if !ok {
			key = starlark.String(pairs[index].(string))
		}
		require.NoError(t, dict.SetKey(key, pairs[index+1].(starlark.Value)))
	}
	return dict
}

// TestCompositeInputDiscoveryNotation proves signatures and shapes describe nested inputs exactly.
func TestCompositeInputDiscoveryNotation(t *testing.T) {
	plan, err := CompileFor[compositeInput, representativeOutput]()
	require.NoError(t, err)

	assert.Equal(t, compositeSignature, plan.Signature("records.query"))
	assert.Equal(t, []FieldShape{
		{Name: "tags", Type: "list[str]", Required: true},
		{Name: "filter", Type: "{owner: str, level: int, min_score: float | None}", Required: true},
		{Name: "labels", Type: "dict[str, int]", Required: true},
		{Name: "window", Type: `{from: str, "until-ms": int} | None`, Required: false},
		{Name: "payload", Type: "list[int]", Required: true},
		{Name: "scores", Type: "list[float | None]", Required: true},
	}, plan.InputShape())
}

// TestCompositeOutputNamesNeedOnlyBeJSONNames proves output members may use keywords and quoted names.
func TestCompositeOutputNamesNeedOnlyBeJSONNames(t *testing.T) {
	plan, err := CompileFor[representativeInput, struct {
		// From is named after a reserved Starlark word.
		From string `json:"from"`

		// Window carries a non-identifier nested name.
		Window compositeWindow `json:"time-window"`
	}]()
	require.NoError(t, err)

	assert.Equal(t, []FieldShape{
		{Name: "from", Type: "str", Required: true},
		{Name: "time-window", Type: `{from: str, "until-ms": int}`, Required: true},
	}, plan.OutputShape())
}

// TestCompositeInputsBindAndRebind proves worker binding and parent re-binding agree on nested values.
func TestCompositeInputsBindAndRebind(t *testing.T) {
	plan, err := CompileFor[compositeInput, representativeOutput]()
	require.NoError(t, err)
	minScore := 2.0
	firstScore, thirdScore := float32(1.5), float32(2)

	tests := []struct {
		// name identifies the call.
		name string

		// kwargs is the Starlark call.
		kwargs []starlark.Tuple

		// want is the exact typed input the handler receives.
		want compositeInput

		// canonical is the exact authorization argument map.
		canonical map[string]any
	}{
		{
			name: "every form populated",
			kwargs: compositeKeywords(map[string]starlark.Value{
				"tags": starlark.Tuple{starlark.String("a"), starlark.String("b")},
				"filter": starlarkDict(t,
					"owner", starlark.String("ops"),
					"level", starlark.MakeInt(255),
					"min_score", starlark.MakeInt(2),
				),
				"labels":  starlarkDict(t, "x", starlark.MakeInt(1)),
				"window":  starlarkDict(t, "from", starlark.String("monday"), "until-ms", starlark.MakeInt(-5)),
				"payload": starlark.NewList([]starlark.Value{starlark.MakeInt(0), starlark.MakeInt(255)}),
				"scores": starlark.NewList([]starlark.Value{
					starlark.Float(1.5), starlark.None, starlark.MakeInt(2),
				}),
			}),
			want: compositeInput{
				Tags:    []string{"a", "b"},
				Filter:  compositeFilter{Owner: "ops", Level: 255, MinScore: &minScore},
				Labels:  map[string]int64{"x": 1},
				Window:  &compositeWindow{From: "monday", UntilMS: -5},
				Payload: []byte{0, 255},
				Scores:  []*float32{&firstScore, nil, &thirdScore},
			},
			canonical: map[string]any{
				"tags":    []any{"a", "b"},
				"filter":  map[string]any{"owner": "ops", "level": int64(255), "min_score": 2.0},
				"labels":  map[string]any{"x": int64(1)},
				"window":  map[string]any{"from": "monday", "until-ms": int64(-5)},
				"payload": []any{int64(0), int64(255)},
				"scores":  []any{1.5, nil, 2.0},
			},
		},
		{
			name: "optional members omitted or None",
			kwargs: compositeKeywords(map[string]starlark.Value{
				"filter": starlarkDict(t,
					"owner", starlark.String("ops"),
					"level", starlark.MakeInt(0),
					"min_score", starlark.None,
				),
				"window": starlark.None,
			}),
			want: compositeInput{
				Tags:    []string{},
				Filter:  compositeFilter{Owner: "ops"},
				Labels:  map[string]int64{},
				Payload: []byte{},
				Scores:  []*float32{},
			},
			canonical: map[string]any{
				"tags":    []any{},
				"filter":  map[string]any{"owner": "ops", "level": int64(0)},
				"labels":  map[string]any{},
				"payload": []any{},
				"scores":  []any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child, err := plan.InputSchema().Bind(nil, tt.kwargs)
			require.NoError(t, err)
			assert.Equal(t, tt.canonical, child)

			bound, canonical, err := plan.BindValue(child)
			require.NoError(t, err)
			assert.Equal(t, tt.want, bound)
			assert.Equal(t, tt.canonical, canonical)

			child["filter"].(map[string]any)["owner"] = "mutated"
			child["tags"] = []any{"mutated"}
			assert.Equal(t, tt.canonical, canonical, "canonical arguments must share no container with the child map")
		})
	}
}

// TestCompositeInputDiagnostics proves every nested rejection names the exact argument path.
func TestCompositeInputDiagnostics(t *testing.T) {
	plan, err := CompileFor[compositeInput, representativeOutput]()
	require.NoError(t, err)
	filter := func(pairs ...any) *starlark.Dict {
		return starlarkDict(t, append([]any{"owner", starlark.String("o"), "level", starlark.MakeInt(1)}, pairs...)...)
	}

	tests := []struct {
		// name identifies the rejected call.
		name string

		// overrides replaces arguments in the valid base call.
		overrides map[string]starlark.Value

		// want is the exact model-facing error text.
		want string
	}{
		{name: "nested type", overrides: map[string]starlark.Value{
			"filter": starlarkDict(t, "owner", starlark.MakeInt(1), "level", starlark.MakeInt(1)),
		}, want: `argument "filter.owner" must be a string`},
		{name: "list element", overrides: map[string]starlark.Value{
			"tags": starlark.NewList([]starlark.Value{starlark.String("a"), starlark.MakeInt(2)}),
		}, want: `argument "tags[1]" must be a string`},
		{name: "string for list", overrides: map[string]starlark.Value{
			"tags": starlark.String("ab"),
		}, want: `argument "tags" must be a list`},
		{name: "list for struct", overrides: map[string]starlark.Value{
			"filter": starlark.NewList(nil),
		}, want: `argument "filter" must be a dict`},
		{name: "unknown nested key", overrides: map[string]starlark.Value{
			"filter": filter("bogus", starlark.MakeInt(1)),
		}, want: `unknown argument "filter.bogus"`},
		{name: "missing nested key", overrides: map[string]starlark.Value{
			"filter": starlarkDict(t, "owner", starlark.String("o")),
		}, want: `missing required argument "filter.level"`},
		{name: "missing quoted nested key", overrides: map[string]starlark.Value{
			"window": starlarkDict(t, "from", starlark.String("m")),
		}, want: `missing required argument "window['until-ms']"`},
		{name: "map value", overrides: map[string]starlark.Value{
			"labels": starlarkDict(t, "env", starlark.None),
		}, want: `argument "labels['env']" must be an integer`},
		{name: "non-string map key", overrides: map[string]starlark.Value{
			"labels": starlarkDict(t, starlark.MakeInt(1), starlark.MakeInt(2)),
		}, want: `argument "labels" keys must be strings`},
		{name: "optional nested mismatch", overrides: map[string]starlark.Value{
			"filter": filter("min_score", starlark.String("x")),
		}, want: `argument "filter.min_score" must be a float or None`},
		{name: "optional struct mismatch", overrides: map[string]starlark.Value{
			"window": starlark.NewList(nil),
		}, want: `argument "window" must be a dict or None`},
		{name: "unsigned range", overrides: map[string]starlark.Value{
			"filter": starlarkDict(t, "owner", starlark.String("o"), "level", starlark.MakeInt(256)),
		}, want: `argument "filter.level" must be between 0 and 255`},
		{name: "byte range", overrides: map[string]starlark.Value{
			"payload": starlark.NewList([]starlark.Value{starlark.MakeInt(-1)}),
		}, want: `argument "payload[0]" must be between 0 and 255`},
		{name: "signed range", overrides: map[string]starlark.Value{
			"window": starlarkDict(t, "from", starlark.String("m"), "until-ms", starlark.MakeInt64(math.MaxInt32+1)),
		}, want: `argument "window['until-ms']" must be between -2147483648 and 2147483647`},
		{name: "float32 range", overrides: map[string]starlark.Value{
			"scores": starlark.NewList([]starlark.Value{starlark.Float(1e39)}),
		}, want: `argument "scores[0]" overflows float32`},
		{name: "non-finite float", overrides: map[string]starlark.Value{
			"filter": filter("min_score", starlark.Float(math.Inf(1))),
		}, want: `argument "filter.min_score" is not finite`},
		{name: "float for unsigned", overrides: map[string]starlark.Value{
			"payload": starlark.NewList([]starlark.Value{starlark.Float(1)}),
		}, want: `argument "payload[0]" must be an integer`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := plan.InputSchema().Bind(nil, compositeKeywords(tt.overrides))

			require.ErrorIs(t, err, ErrInvalidArguments)
			assert.EqualError(t, err, "invalid capability arguments: "+tt.want)
		})
	}
}

// TestBindValueRejectsCompositeWireMismatches proves the parent re-checks every nested value.
func TestBindValueRejectsCompositeWireMismatches(t *testing.T) {
	plan, err := CompileFor[compositeInput, representativeOutput]()
	require.NoError(t, err)
	valid := func(overrides map[string]any) map[string]any {
		arguments := map[string]any{
			"tags":    []any{},
			"filter":  map[string]any{"owner": "o", "level": int64(1)},
			"labels":  map[string]any{},
			"payload": []any{},
			"scores":  []any{},
		}
		maps.Copy(arguments, overrides)
		return arguments
	}

	tests := []struct {
		// name identifies the forged normalized map.
		name string

		// arguments is the normalized map received from the worker.
		arguments map[string]any

		// contains is the expected diagnostic fragment.
		contains string
	}{
		{name: "nested type", arguments: valid(map[string]any{
			"filter": map[string]any{"owner": int64(1), "level": int64(1)},
		}), contains: `"filter.owner" must be a string`},
		{name: "nested range", arguments: valid(map[string]any{
			"filter": map[string]any{"owner": "o", "level": int64(300)},
		}), contains: `"filter.level" must be between 0 and 255`},
		{name: "integer for float on the wire", arguments: valid(map[string]any{
			"filter": map[string]any{"owner": "o", "level": int64(1), "min_score": int64(2)},
		}), contains: `"filter.min_score" must be a float or None`},
		{name: "unknown nested key", arguments: valid(map[string]any{
			"filter": map[string]any{"owner": "o", "level": int64(1), "bogus": true},
		}), contains: `unknown argument "filter.bogus"`},
		{name: "missing nested key", arguments: valid(map[string]any{
			"filter": map[string]any{"owner": "o"},
		}), contains: `missing required argument "filter.level"`},
		{name: "non-neutral Go list", arguments: valid(map[string]any{
			"tags": []string{"a"},
		}), contains: `"tags" must be a list`},
		{name: "optional element type", arguments: valid(map[string]any{
			"scores": []any{"x"},
		}), contains: `"scores[0]" must be a float or None`},
		{name: "map value type", arguments: valid(map[string]any{
			"labels": map[string]any{"x": "y"},
		}), contains: `"labels['x']" must be an integer`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := plan.BindValue(tt.arguments)

			require.ErrorIs(t, err, ErrInvalidArguments)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestInputSchemaSurvivesTheWorkerManifestEncoding proves JSON framing preserves every schema field.
func TestInputSchemaSurvivesTheWorkerManifestEncoding(t *testing.T) {
	plan, err := CompileFor[compositeInput, representativeOutput]()
	require.NoError(t, err)
	schema := plan.InputSchema()

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	var decoded InputSchema
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	assert.Equal(t, schema, decoded)
	require.NoError(t, decoded.Validate())
	_, err = decoded.Bind(nil, compositeKeywords(map[string]starlark.Value{
		"payload": starlark.NewList([]starlark.Value{starlark.MakeInt(256)}),
	}))
	require.EqualError(t, err, `invalid capability arguments: argument "payload[0]" must be between 0 and 255`)
}

// TestInputSchemaValidateAcceptsCompiledSchemas proves every compiled schema validates.
func TestInputSchemaValidateAcceptsCompiledSchemas(t *testing.T) {
	representative, err := CompileFor[representativeInput, representativeOutput]()
	require.NoError(t, err)
	empty, err := CompileFor[struct{}, representativeOutput]()
	require.NoError(t, err)
	widened, err := CompileFor[widenedInput, representativeOutput]()
	require.NoError(t, err)
	composite, err := CompileFor[compositeInput, representativeOutput]()
	require.NoError(t, err)

	for _, plan := range []*Plan{representative, empty, widened, composite} {
		require.NoError(t, plan.InputSchema().Validate())
	}
}

// TestInputSchemaValidateRejectsMalformedArenas proves a forged manifest cannot smuggle an invalid schema.
func TestInputSchemaValidateRejectsMalformedArenas(t *testing.T) {
	str := InputNode{Kind: InputString}
	root := func(fields ...InputField) InputNode { return InputNode{Kind: InputStruct, Fields: fields} }

	tests := []struct {
		// name identifies the malformed schema.
		name string

		// schema is the candidate arena.
		schema InputSchema

		// contains is the expected diagnostic fragment.
		contains string
	}{
		{name: "root out of range", schema: InputSchema{Root: 1, Nodes: []InputNode{root()}},
			contains: "root must be a struct"},
		{name: "root not a struct", schema: InputSchema{Nodes: []InputNode{str}},
			contains: "root must be a struct"},
		{name: "forward element", schema: InputSchema{Root: 2, Nodes: []InputNode{
			{Kind: InputList, Elem: 1}, str, root(InputField{Name: "a", Node: 0}),
		}}, contains: "invalid element"},
		{name: "self element", schema: InputSchema{Root: 1, Nodes: []InputNode{
			{Kind: InputOptional}, root(InputField{Name: "a", Node: 0}),
		}}, contains: "invalid element"},
		{name: "optional wraps optional", schema: InputSchema{Root: 3, Nodes: []InputNode{
			str, {Kind: InputOptional}, {Kind: InputOptional, Elem: 1}, root(InputField{Name: "a", Node: 2}),
		}}, contains: "nests optional"},
		{name: "unknown kind", schema: InputSchema{Root: 1, Nodes: []InputNode{
			{Kind: "tuple"}, root(InputField{Name: "a", Node: 0}),
		}}, contains: "unknown kind"},
		{name: "scalar with children", schema: InputSchema{Root: 1, Nodes: []InputNode{
			{Kind: InputString, Fields: []InputField{}}, root(InputField{Name: "a", Node: 0}),
		}}, contains: "has children"},
		{name: "bounds on a string", schema: InputSchema{Root: 1, Nodes: []InputNode{
			{Kind: InputString, IntMax: 1}, root(InputField{Name: "a", Node: 0}),
		}}, contains: "integer bounds"},
		{name: "empty integer range", schema: InputSchema{Root: 1, Nodes: []InputNode{
			{Kind: InputInt, IntMin: 5, IntMax: 1}, root(InputField{Name: "a", Node: 0}),
		}}, contains: "empty integer range"},
		{name: "float32 flag on an integer", schema: InputSchema{Root: 1, Nodes: []InputNode{
			{Kind: InputInt, Float32: true}, root(InputField{Name: "a", Node: 0}),
		}}, contains: "float32 flag"},
		{name: "keyword root name", schema: InputSchema{Root: 1, Nodes: []InputNode{
			str, root(InputField{Name: "from", Node: 0}),
		}}, contains: "Starlark identifier"},
		{name: "invalid nested name", schema: InputSchema{Root: 2, Nodes: []InputNode{
			str, root(InputField{Name: "a'b", Node: 0}), root(InputField{Name: "a", Node: 1}),
		}}, contains: "not a valid JSON field name"},
		{name: "duplicate name", schema: InputSchema{Root: 1, Nodes: []InputNode{
			str, root(InputField{Name: "a", Node: 0}, InputField{Name: "a", Node: 0}),
		}}, contains: "duplicate input name"},
		{name: "forward field node", schema: InputSchema{Root: 0, Nodes: []InputNode{
			root(InputField{Name: "a", Node: 1}), str,
		}}, contains: "invalid node"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schema.Validate()

			require.ErrorIs(t, err, ErrInvalidPlan)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}
