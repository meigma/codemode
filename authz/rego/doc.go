// Package rego implements authz.Authorizer with one prepared in-process Rego decision.
//
// New accepts trusted in-memory module source and one ground data reference such as
// data.codemode.authz.allow. It compiles the modules with Rego v1 semantics, removes
// every OPA-declared nondeterministic builtin, sets the allowed network hosts to an
// explicit empty set, treats builtin errors as fatal, and disables print statements.
// Construction is synchronous. The returned Authorizer is immutable and safe for
// concurrent Authorize calls.
//
// Authorize projects only subject.id, capability.id, capability.name, and the
// borrowed canonical arguments map. Exactly one Boolean true allows the call.
// Exactly one Boolean false returns authz.ErrDenied. Undefined, non-Boolean,
// multi-result, evaluation, and builtin failures are ordinary policy errors.
// A canceled or expired context returns ctx.Err().
//
// Write a total Boolean decision with default allow := false. Treat unmatched
// input as an intentional denial, not an undefined result. Module source is
// trusted deployment configuration. These restrictions reduce evaluator
// capabilities; they do not isolate CPU, heap, processes, or tenants.
package rego
