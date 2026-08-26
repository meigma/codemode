package catalog

import (
	"context"
	"slices"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/binding"
)

// Invoker is a type-erased adapter around one registered typed capability handler.
type Invoker func(context.Context, authz.Subject, any) (any, error)

// Registration contains one capability before validation and static filtering.
type Registration struct {
	// ID is the stable deployment and policy identity.
	ID string

	// Name is the dotted Starlark and model-facing capability name.
	Name string

	// Summary is the compact description used by capability search.
	Summary string

	// Description is the full exact-description text.
	Description string

	// SearchTerms contains alternative task vocabulary used only for discovery.
	SearchTerms []string

	// Plan is the caller-compiled input, output, canonical-argument, and signature plan.
	Plan *binding.Plan

	// Invoke calls the registered typed handler through its registration-time adapter.
	Invoke Invoker
}

// Options controls one immutable catalog build.
type Options struct {
	// DisabledCapabilities lists stable IDs removed from every live catalog surface.
	DisabledCapabilities []string

	// MaxSearchQueryBytes bounds search input before normalization.
	MaxSearchQueryBytes int

	// MaxSearchResults bounds the deterministic result list.
	MaxSearchResults int
}

// Entry is one validated, enabled capability with its compiled binding plan.
type Entry struct {
	// ID is the stable deployment and policy identity.
	ID string

	// Name is the dotted Starlark and model-facing capability name.
	Name string

	// Summary is the compact search description.
	Summary string

	// Description is the full exact-description text.
	Description string

	// Plan is the immutable input, output, canonical-argument, and signature plan.
	Plan *binding.Plan

	// Invoke calls the registered typed handler through its registration-time adapter.
	Invoke Invoker

	// signature is generated once from the immutable binding plan.
	signature string

	// inputShape is the registration-time compiled model-facing input shape.
	inputShape []binding.FieldShape

	// outputShape is the registration-time compiled model-facing output shape.
	outputShape []binding.FieldShape
}

// NamespaceBinding maps one enabled capability to its immutable namespace path.
type NamespaceBinding struct {
	// Segments contains the namespace path before the final function name.
	Segments []string

	// Function is the final Starlark function segment.
	Function string

	// Capability is the enabled catalog entry represented by this function.
	Capability Entry
}

// Catalog is an immutable, statically filtered capability catalog.
type Catalog struct {
	// enabled is the single name-sorted source for every live surface.
	enabled []Entry

	// byName maps exact enabled names to indexes in enabled.
	byName map[string]int

	// byID maps exact enabled stable IDs to indexes in enabled.
	byID map[string]int

	// namespaces is derived only from enabled and remains name-sorted.
	namespaces []NamespaceBinding

	// search is the immutable enabled-document search index.
	search searchIndex

	// maxSearchQueryBytes is the positive search input budget.
	maxSearchQueryBytes int

	// maxSearchResults is the positive search output budget.
	maxSearchResults int
}

// Entries returns a copy of the name-sorted enabled entries.
func (catalog *Catalog) Entries() []Entry {
	return slices.Clone(catalog.enabled)
}

// Lookup returns one enabled capability by exact model-facing name.
func (catalog *Catalog) Lookup(name string) (Entry, bool) {
	index, ok := catalog.byName[name]
	if !ok {
		return Entry{}, false
	}
	return catalog.enabled[index], true
}

// LookupID returns one enabled capability by exact stable ID.
func (catalog *Catalog) LookupID(id string) (Entry, bool) {
	index, ok := catalog.byID[id]
	if !ok {
		return Entry{}, false
	}
	return catalog.enabled[index], true
}

// NamespaceBindings returns deep copies of namespace data derived from enabled entries.
func (catalog *Catalog) NamespaceBindings() []NamespaceBinding {
	bindings := make([]NamespaceBinding, len(catalog.namespaces))
	for index, namespace := range catalog.namespaces {
		bindings[index] = namespace
		bindings[index].Segments = slices.Clone(namespace.Segments)
	}
	return bindings
}
