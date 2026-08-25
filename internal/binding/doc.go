// Package binding compiles restricted Go input and output types into immutable
// conversion plans and owns process-neutral value conversion.
//
// Typed handler values, Starlark values, and JSON-shaped maps share one allowed
// matrix: nil, bool, string, int64, finite float64, []any, and map[string]any.
// ValidateValue, FromStarlark, and ToStarlark enforce that matrix plus positive
// depth and materialization limits. json.Number and other numeric types are
// rejected. Plan.InputShape remains the only descriptor source for compiled
// input fields.
package binding
