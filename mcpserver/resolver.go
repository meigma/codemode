package mcpserver

import (
	"context"

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
