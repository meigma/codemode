// Package binding compiles restricted Go input and output types into immutable
// conversion plans and owns process-neutral value conversion.
//
// One shared type compiler builds a post-order node arena for each direction.
// Inputs and outputs accept the same recursive structural universe (scalars of
// every integer and float width, pointers, slices, string-keyed maps, and
// nested structs) with direction-specific exceptions: inputs reject arrays,
// pointer-to-pointer, and custom unmarshalers; outputs reject custom
// marshalers. Root input field names must be Starlark identifiers because they
// are keyword arguments; every other field name only needs to be a valid JSON
// field name.
//
// The input arena projects to InputSchema, a process-neutral description the
// worker validates once and uses to bind Starlark keyword arguments. The
// parent re-binds the worker's canonical map against the same arena to build
// the typed handler input and the authoritative canonical arguments.
//
// Typed handler values, Starlark values, and JSON-shaped maps share one allowed
// matrix: nil, bool, string, int64, finite float64, []any, and map[string]any.
// ValidateValue, FromStarlark, and ToStarlark enforce that matrix plus positive
// depth and materialization limits. [json.Number] and other numeric types are
// rejected. Plan.InputShape and Plan.OutputShape remain flat FieldShape slices
// whose Type strings carry nested list, dict, struct, and nullable notation.
package binding
