package authz

import (
	"context"
	"errors"
)

// SubjectID is a stable, non-secret authenticated identity.
type SubjectID string

// Subject identifies the authenticated caller used for authorization decisions.
type Subject struct {
	// ID is the caller's stable, non-secret identity.
	ID SubjectID
}

// subjectContextKey prevents collisions with context values owned by host packages.
type subjectContextKey struct{}

// WithSubject returns a child context containing a trusted authenticated subject.
//
// Hosts must derive subject from their authentication boundary before calling
// WithSubject. Tool arguments, program source, and MCP request metadata are not
// trusted identity sources.
func WithSubject(ctx context.Context, subject Subject) context.Context {
	return context.WithValue(ctx, subjectContextKey{}, subject)
}

// SubjectFromContext returns the trusted subject stored by WithSubject.
func SubjectFromContext(ctx context.Context) (Subject, bool) {
	subject, ok := ctx.Value(subjectContextKey{}).(Subject)
	return subject, ok
}

// AuthorizationInput contains the complete trusted input for one authorization decision.
type AuthorizationInput struct {
	// Subject is the authenticated identity resolved from trusted host context.
	Subject Subject

	// CapabilityID is the capability's stable policy identity.
	CapabilityID string

	// CapabilityName is the model-facing dotted capability name.
	CapabilityName string

	// Arguments is a fresh canonical JSON-shaped projection of validated arguments.
	Arguments map[string]any
}

// Authorizer decides whether a subject may perform one validated native invocation.
type Authorizer interface {
	// Authorize returns nil to allow the invocation, an error wrapping ErrDenied for a recognized denial,
	// or another error when policy evaluation fails.
	Authorize(context.Context, AuthorizationInput) error
}

// ErrDenied classifies a recognized authorization denial.
//
// An authorizer may wrap ErrDenied with trusted diagnostic context. Callers must not expose
// the wrapped error text to untrusted clients.
var ErrDenied = errors.New("authorization denied")

// AllowAllAuthorizer explicitly permits every authorization input.
type AllowAllAuthorizer struct{}

// AllowAll returns an explicit authorizer that permits every invocation.
func AllowAll() AllowAllAuthorizer {
	return AllowAllAuthorizer{}
}

// Authorize permits the invocation without inspecting its input.
func (AllowAllAuthorizer) Authorize(context.Context, AuthorizationInput) error {
	return nil
}
