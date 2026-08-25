package mcpserver

import (
	"context"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
)

// InvocationResolver resolves the trusted invocation subject from host-owned Go context.
//
// Implementations must read only typed trusted context established by middleware or
// process composition. They must not derive identity from program data, tool
// arguments, or untrusted request metadata.
type InvocationResolver interface {
	// Resolve returns the authenticated subject for the current request.
	//
	// A resolver failure or empty subject ID stops the request before discovery or
	// execution. Resolve must not return credential material.
	Resolve(ctx context.Context) (authz.Subject, error)
}

// staticSubjectResolver returns one process-owned identity for every invocation.
type staticSubjectResolver struct {
	// subject is the fixed trusted identity supplied by the host.
	subject authz.Subject
}

// Resolve returns the fixed trusted subject.
func (resolver staticSubjectResolver) Resolve(context.Context) (authz.Subject, error) {
	if resolver.subject.ID == "" {
		return authz.Subject{}, codemode.ErrUnauthenticated
	}
	return resolver.subject, nil
}

// contextSubjectResolver reads identity from authz-owned trusted context.
type contextSubjectResolver struct{}

// Resolve returns a non-empty subject installed with authz.WithSubject.
func (contextSubjectResolver) Resolve(ctx context.Context) (authz.Subject, error) {
	subject, ok := authz.SubjectFromContext(ctx)
	if !ok || subject.ID == "" {
		return authz.Subject{}, codemode.ErrUnauthenticated
	}
	return subject, nil
}

// StaticSubject returns a resolver that uses subject for every invocation.
//
// StaticSubject is suitable only when process ownership is the authentication
// boundary, such as a single-user stdio server. Multi-user hosts must resolve a
// distinct authenticated subject for each request and must not use
// StaticSubject.
func StaticSubject(subject authz.Subject) InvocationResolver {
	return staticSubjectResolver{subject: subject}
}

// ContextSubject returns a resolver for subjects installed with authz.WithSubject.
//
// Authentication middleware remains responsible for validating credentials
// and installing the subject. Client-controlled tool arguments, program source,
// and MCP request metadata cannot set or replace the stored subject.
func ContextSubject() InvocationResolver {
	return contextSubjectResolver{}
}
