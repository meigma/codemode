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
	// ID is the stable identity used by deployment filtering and authorization
	// policy. An empty ID defaults to Name. Set ID explicitly before writing
	// policy or deployment filters against this capability.
	ID CapabilityID

	// Name is the dotted Starlark name exposed to programs and discovery.
	// The first segment must not collide with a reserved Starlark universe root.
	Name CapabilityName

	// Summary is a compact description used by capability search.
	Summary string

	// Description explains the capability behavior for exact description
	// requests. An empty Description defaults to Summary.
	Description string

	// SearchTerms contains alternative task vocabulary used only for discovery.
	// Terms are not callable aliases and are not accepted by Describe or Execute.
	// They are not returned in search results, but callers can infer indexed
	// vocabulary by probing. Do not put secrets, policy facts, credentials,
	// tenant identifiers, or sensitive examples in search terms.
	SearchTerms []string

	// Handler executes the capability after binding and authorization succeed.
	Handler Handler[Input, Output]
}
