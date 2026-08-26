// Package universe owns the fixed Starlark language surface.
//
// Every execution receives a fresh predeclared dictionary containing numeric
// sum, a filtered json module with decode, encode, and indent, and the pinned
// math module. Query helpers return copied sorted name lists for documentation
// and reserved-root checks. Capability namespaces merge only after
// [IsReservedRoot] rejects colliding roots. Nested leaves such as stats.sum
// remain legal. There is no built-in time module.
package universe
