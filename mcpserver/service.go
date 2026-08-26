package mcpserver

import (
	"context"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
)

// Service is the inbound adapter's view of an immutable CodeMode server.
//
// The root *codemode.Server implements this port. The adapter does not re-enforce
// catalog bounds, hidden-capability filtering, or execution restrictions.
type Service interface {
	// Search returns a bounded relevance-ranked scan of enabled capabilities.
	Search(query string) (codemode.SearchResponse, error)

	// Describe returns one exact enabled capability description or a not-found error.
	Describe(name codemode.CapabilityName) (codemode.Description, error)

	// Execute runs one bounded program for a trusted authenticated subject and returns only main's final value.
	Execute(ctx context.Context, subject authz.Subject, program codemode.Program) (any, error)
}
