// Package rego implements authz.Authorizer with one prepared in-process Rego decision.
//
// New accepts trusted in-memory module source and one ground data reference such as
// data.codemode.authz.allow. It compiles the modules with Rego v1 semantics, removes
// every OPA-declared nondeterministic builtin, sets AllowNet to a non-nil empty
// deny-all host list as defense in depth, treats builtin errors as fatal, and
// disables print statements. Construction is synchronous. The returned Authorizer
// is immutable and safe for concurrent Authorize calls.
//
// Removing those builtins takes away runtime network-capable builtins, including
// http.send, plus DNS, runtime, random, time, and UUID builtins. AllowNet is not
// the mechanism that removes http.send. New installs no schema set and no schema
// resolver. Metadata schema["https://example.invalid/schema.json"] is accepted
// but ignored: there is no validation and no fetch. Metadata with an external
// $ref: "https://example.invalid/schema.json" asks OPA to load a remote schema
// and is rejected because remote reference loading is disabled.
//
// Authorize projects only subject.id, capability.id, capability.name, and the
// borrowed canonical arguments map. A ground decision is either undefined or
// yields one value, which must be Boolean. Boolean true allows the call, and
// Boolean false returns authz.ErrDenied. Undefined, non-Boolean, evaluation,
// and builtin failures are ordinary policy errors. A canceled or expired
// context returns ctx.Err().
//
// Write a total Boolean decision with default allow := false. Treat unmatched
// input as an intentional denial, not an undefined result. Module source is
// trusted deployment configuration. These restrictions reduce evaluator
// capabilities; they do not isolate CPU, heap, processes, or tenants.
package rego
