package codemode

import (
	"context"

	"github.com/meigma/codemode/authz"
)

// CapabilityID is a stable deployment and authorization identity for a capability.
type CapabilityID string

// CapabilityName is the dotted name exposed to Starlark programs and model-facing discovery.
type CapabilityName string

// Handler executes one capability with trusted subject identity and typed input.
type Handler[Input, Output any] func(context.Context, authz.Subject, Input) (Output, error)

// Capability describes one typed native operation available to CodeMode.
type Capability[Input, Output any] struct {
	// ID is the stable identity used by deployment filtering and authorization policy.
	ID CapabilityID

	// Name is the dotted Starlark name exposed to programs and discovery.
	Name CapabilityName

	// Summary is a compact description used by capability search.
	Summary string

	// Description explains the capability behavior for exact description requests.
	Description string

	// Handler executes the capability after binding and authorization succeed.
	Handler Handler[Input, Output]
}
