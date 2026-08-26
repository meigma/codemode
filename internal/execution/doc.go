// Package execution runs one restricted, bounded Starlark program at a time.
//
// Engine is compiled from the fixed language surface merged with
// process-neutral capability bindings. Each Execute call receives a
// request-specific NativeCall and converts only main's final value.
package execution
