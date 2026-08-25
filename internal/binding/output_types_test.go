package binding

// namedString is a named string alias accepted as an output scalar or map key.
type namedString string

// namedInt is a named signed integer alias accepted as an output scalar.
type namedInt int64

// namedBool is a named Boolean alias accepted as an output scalar.
type namedBool bool

// namedFloat is a named floating-point alias accepted as an output scalar.
type namedFloat float64

// nestedItem is a nested exported result used by composite output tests.
type nestedItem struct {
	// Title is a nested string field.
	Title string `json:"title"`

	// Score is a nested finite floating-point field.
	Score float64 `json:"score"`
}

// compositeOutput exercises nested structs, lists, maps, pointers, and bytes.
type compositeOutput struct {
	// Items is a list of nested objects.
	Items []nestedItem `json:"items"`

	// Tags is a fixed-length string array.
	Tags [2]string `json:"tags"`

	// ByID is a string-keyed object map.
	ByID map[string]nestedItem `json:"by_id"`

	// Note is a required nullable string.
	Note *string `json:"note"`

	// Extra is an optional string omitted when nil.
	Extra *string `json:"extra,omitempty"`

	// Payload is a byte slice converted as an integer list.
	Payload []byte `json:"payload"`
}

// numericOutput covers every accepted integer and floating-point kind.
type numericOutput struct {
	// I is a platform signed integer.
	I int `json:"i"`

	// I8 is an 8-bit signed integer.
	I8 int8 `json:"i8"`

	// I16 is a 16-bit signed integer.
	I16 int16 `json:"i16"`

	// I32 is a 32-bit signed integer.
	I32 int32 `json:"i32"`

	// I64 is a 64-bit signed integer.
	I64 int64 `json:"i64"`

	// U is a platform unsigned integer.
	U uint `json:"u"`

	// U8 is an 8-bit unsigned integer.
	U8 uint8 `json:"u8"`

	// U16 is a 16-bit unsigned integer.
	U16 uint16 `json:"u16"`

	// U32 is a 32-bit unsigned integer.
	U32 uint32 `json:"u32"`

	// U64 is a 64-bit unsigned integer.
	U64 uint64 `json:"u64"`

	// F32 is a 32-bit floating-point value.
	F32 float32 `json:"f32"`

	// F64 is a 64-bit floating-point value.
	F64 float64 `json:"f64"`
}

// aliasedOutput covers named aliases of accepted scalar kinds.
type aliasedOutput struct {
	// Name is a named string.
	Name namedString `json:"name"`

	// Count is a named integer.
	Count namedInt `json:"count"`

	// Active is a named Boolean.
	Active namedBool `json:"active"`

	// Score is a named float.
	Score namedFloat `json:"score"`
}

// bytesOutput covers byte slices and arrays as integer lists.
type bytesOutput struct {
	// Payload is a byte slice.
	Payload []byte `json:"payload"`

	// Fixed is a byte array.
	Fixed [2]byte `json:"fixed"`
}

// aliasKeyOutput covers maps whose keys are named string aliases.
type aliasKeyOutput struct {
	// ByName maps named string keys onto integers.
	ByName map[namedString]int64 `json:"by_name"`
}

// notationOutput locks exact discovery Type grammar.
type notationOutput struct {
	// Items is a list of nested objects.
	Items []nestedItem `json:"items"`

	// ByID is a string-keyed object map.
	ByID map[string]nestedItem `json:"by_id"`

	// Tags is a fixed-length string array.
	Tags [2]string `json:"tags"`

	// Note is a required nullable string.
	Note *string `json:"note"`

	// Extra is an optional string.
	Extra *string `json:"extra,omitempty"`

	// Nested carries a nested optional field.
	Nested nestedOptional `json:"nested"`

	// Values is a nullable list of nullable strings.
	Values *[]*string `json:"values"`

	// Alias is a named-string-key Boolean map.
	Alias map[namedString]bool `json:"alias"`

	// Payload is a byte slice.
	Payload []byte `json:"payload"`
}

// nestedOptional is a nested object with one optional field.
type nestedOptional struct {
	// Value is a required nested string.
	Value string `json:"value"`

	// Detail is an optional nested string.
	Detail *string `json:"detail,omitempty"`
}

// noneDedupOutput proves stacked pointers append None once.
type noneDedupOutput struct {
	// Value is a double pointer to a string.
	Value **string `json:"value"`
}

// pointerElementOutput covers pointer elements inside lists and maps.
type pointerElementOutput struct {
	// Values is a list of nullable integers.
	Values []*int64 `json:"values"`

	// ByID is a map of nullable nested objects.
	ByID map[string]*nestedItem `json:"by_id"`
}

// optionalHeavyOutput covers omitempty inclusion accounting.
type optionalHeavyOutput struct {
	// A is an optional string.
	A *string `json:"a,omitempty"`

	// B is an optional string.
	B *string `json:"b,omitempty"`

	// C is an optional string.
	C *string `json:"c,omitempty"`
}

// mapScoreOutput covers deterministic map traversal order.
type mapScoreOutput struct {
	// ByID maps identifiers onto scores.
	ByID map[string]float64 `json:"by_id"`
}

// listScoreOutput covers deterministic list index paths.
type listScoreOutput struct {
	// Items is a list of nested scored objects.
	Items []nestedItem `json:"items"`
}

// overflowOutput covers unsigned values above MaxInt64.
type overflowOutput struct {
	// Count is an unsigned 64-bit integer.
	Count uint64 `json:"count"`
}

// float32Output covers non-finite float32 values.
type float32Output struct {
	// Score is a 32-bit floating-point field.
	Score float32 `json:"score"`
}

// hugeSliceOutput is used by the allocation preflight regression.
type hugeSliceOutput struct {
	// Items is an oversized reflected slice.
	Items []string `json:"items"`
}

// hugeArrayOutput is used by the allocation preflight regression.
type hugeArrayOutput struct {
	// Items is an oversized reflected array.
	Items [128000]byte `json:"items"`
}

// hugeMapOutput is used by the allocation preflight regression.
type hugeMapOutput struct {
	// Items is an oversized reflected map.
	Items map[string]string `json:"items"`
}

// hugeStructOutput is used by the allocation preflight regression.
type hugeStructOutput struct {
	// Items is an oversized list of nested structs.
	Items []nestedItem `json:"items"`
}

// nestedDepthOutput is a one-level nested object used by depth-limit tests.
type nestedDepthOutput struct {
	// Item is a nested object that requires depth 2.
	Item nestedItem `json:"item"`
}

// nestedInterfaceOutput embeds an interface behind a list path.
type nestedInterfaceOutput struct {
	// Items is a list of objects with an interface field.
	Items []struct {
		// Value is the field under validation.
		Value any `json:"value"`
	} `json:"items"`
}

// directCycleOutput is a self-referential pointer graph.
type directCycleOutput struct {
	// Next continues the cycle.
	Next *directCycleOutput `json:"next"`
}

// indirectCycleBranch is the first type in an indirect cycle.
type indirectCycleBranch struct {
	// Leaf continues the cycle.
	Leaf *indirectCycleLeaf `json:"leaf"`
}

// indirectCycleLeaf is the second type in an indirect cycle.
type indirectCycleLeaf struct {
	// Branch continues the cycle.
	Branch *indirectCycleBranch `json:"branch"`
}

// sliceCycleOutput is a cycle through a slice element type.
type sliceCycleOutput struct {
	// Items continues the cycle.
	Items []sliceCycleOutput `json:"items"`
}

// mapCycleOutput is a cycle through a map value type.
type mapCycleOutput struct {
	// Children continues the cycle.
	Children map[string]*mapCycleOutput `json:"children"`
}

// valueJSONMarshaler implements [json.Marshaler] on the value method set.
type valueJSONMarshaler struct {
	// Value is an unused host field.
	Value string `json:"value"`
}

// MarshalJSON implements [json.Marshaler].
func (valueJSONMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// pointerJSONMarshaler implements [json.Marshaler] on the pointer method set.
type pointerJSONMarshaler struct {
	// Value is an unused host field.
	Value string `json:"value"`
}

// MarshalJSON implements [json.Marshaler].
func (*pointerJSONMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// valueTextMarshaler implements [encoding.TextMarshaler] on the value method set.
type valueTextMarshaler struct {
	// Value is an unused host field.
	Value string `json:"value"`
}

// MarshalText implements [encoding.TextMarshaler].
func (valueTextMarshaler) MarshalText() ([]byte, error) {
	return []byte("x"), nil
}

// pointerTextMarshaler implements [encoding.TextMarshaler] on the pointer method set.
type pointerTextMarshaler struct {
	// Value is an unused host field.
	Value string `json:"value"`
}

// MarshalText implements [encoding.TextMarshaler].
func (*pointerTextMarshaler) MarshalText() ([]byte, error) {
	return []byte("x"), nil
}

// marshalerByte is a named uint8 that implements [encoding.TextMarshaler].
type marshalerByte uint8

// MarshalText implements [encoding.TextMarshaler].
func (marshalerByte) MarshalText() ([]byte, error) {
	return []byte("x"), nil
}
