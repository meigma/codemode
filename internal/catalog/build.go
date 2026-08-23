package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/meigma/codemode/internal/binding"
)

const minimumDottedSegments = 2

var (
	// ErrInvalidRegistration classifies malformed metadata, handlers, types, duplicates, or namespace collisions.
	ErrInvalidRegistration = errors.New("invalid catalog registration")

	// ErrUnknownDisabledCapability classifies a disabled ID absent from the validated registration set.
	ErrUnknownDisabledCapability = errors.New("unknown disabled capability")

	// ErrInputTypeMismatch classifies an erased handler argument that cannot be the compiled input type.
	ErrInputTypeMismatch = errors.New("capability input type mismatch")
)

// candidate is one fully validated registration before static filtering.
type candidate struct {
	// entry contains copied metadata, the supplied plan, and the handler adapter.
	entry Entry
}

// Build validates every registration, applies static filtering once, and returns an immutable catalog.
func Build(registrations []Registration, options Options) (*Catalog, error) {
	if options.MaxSearchQueryBytes <= 0 {
		return nil, fmt.Errorf("%w: MaxSearchQueryBytes must be positive", ErrInvalidRegistration)
	}
	if options.MaxSearchResults <= 0 {
		return nil, fmt.Errorf("%w: MaxSearchResults must be positive", ErrInvalidRegistration)
	}

	candidates := make([]candidate, 0, len(registrations))
	ids := make(map[string]struct{}, len(registrations))
	names := make(map[string]struct{}, len(registrations))
	for index, registration := range registrations {
		if err := ValidateRegistration(registration); err != nil {
			return nil, fmt.Errorf("%w: registration %d: %w", ErrInvalidRegistration, index, err)
		}
		if _, duplicate := ids[registration.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate ID %q", ErrInvalidRegistration, registration.ID)
		}
		if _, duplicate := names[registration.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate name %q", ErrInvalidRegistration, registration.Name)
		}
		ids[registration.ID] = struct{}{}
		names[registration.Name] = struct{}{}
		candidates = append(candidates, candidate{entry: Entry{
			ID:            registration.ID,
			Name:          registration.Name,
			Summary:       registration.Summary,
			Description:   registration.Description,
			Plan:          registration.Plan,
			Invoke:        registration.Invoke,
			signature:     registration.Plan.Signature(registration.Name),
			searchName:    strings.ToLower(registration.Name),
			searchSummary: strings.ToLower(registration.Summary),
			inputShape:    registration.Plan.InputShape(),
			outputShape:   registration.Plan.OutputShape(),
		}})
	}
	if err := validateNamespaceCollisions(names); err != nil {
		return nil, err
	}

	disabled, err := validateDisabled(options.DisabledCapabilities, ids)
	if err != nil {
		return nil, err
	}
	enabled := make([]Entry, 0, len(candidates)-len(disabled))
	for _, candidate := range candidates {
		if _, isDisabled := disabled[candidate.entry.ID]; isDisabled {
			continue
		}
		enabled = append(enabled, candidate.entry)
	}
	sort.Slice(enabled, func(left int, right int) bool {
		return enabled[left].Name < enabled[right].Name
	})

	catalog := &Catalog{
		enabled:             enabled,
		byName:              make(map[string]int, len(enabled)),
		byID:                make(map[string]int, len(enabled)),
		namespaces:          make([]NamespaceBinding, 0, len(enabled)),
		maxSearchQueryBytes: options.MaxSearchQueryBytes,
		maxSearchResults:    options.MaxSearchResults,
	}
	for index, entry := range enabled {
		catalog.byName[entry.Name] = index
		catalog.byID[entry.ID] = index
		segments := strings.Split(entry.Name, ".")
		catalog.namespaces = append(catalog.namespaces, NamespaceBinding{
			Segments:   append([]string(nil), segments[:len(segments)-1]...),
			Function:   segments[len(segments)-1],
			Capability: entry,
		})
	}
	return catalog, nil
}

// ValidateRegistration reports whether one copied registration has valid metadata, a compiled plan, and a handler.
func ValidateRegistration(registration Registration) error {
	if registration.ID == "" || registration.ID != strings.TrimSpace(registration.ID) {
		return errors.New("ID must be non-empty without surrounding whitespace")
	}
	if !isDottedName(registration.Name) {
		return fmt.Errorf("name %q must contain valid dotted Starlark identifiers", registration.Name)
	}
	if registration.Summary == "" || registration.Summary != strings.TrimSpace(registration.Summary) {
		return errors.New("summary must be non-empty without surrounding whitespace")
	}
	if registration.Description == "" || registration.Description != strings.TrimSpace(registration.Description) {
		return errors.New("description must be non-empty without surrounding whitespace")
	}
	if registration.Plan == nil {
		return errors.New("plan must not be nil")
	}
	if registration.Invoke == nil {
		return errors.New("handler must not be nil")
	}
	return nil
}

// validateDisabled verifies the static disabled set against all validated registrations.
func validateDisabled(configured []string, known map[string]struct{}) (map[string]struct{}, error) {
	disabled := make(map[string]struct{}, len(configured))
	for _, id := range configured {
		if _, duplicate := disabled[id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate configured ID %q", ErrInvalidRegistration, id)
		}
		if _, exists := known[id]; !exists {
			return nil, fmt.Errorf("%w: %q", ErrUnknownDisabledCapability, id)
		}
		disabled[id] = struct{}{}
	}
	return disabled, nil
}

// validateNamespaceCollisions rejects a capability function that is also a namespace.
func validateNamespaceCollisions(names map[string]struct{}) error {
	for name := range names {
		segments := strings.Split(name, ".")
		for end := 1; end < len(segments); end++ {
			prefix := strings.Join(segments[:end], ".")
			if _, collides := names[prefix]; collides {
				return fmt.Errorf("%w: capability %q is also a namespace of %q", ErrInvalidRegistration, prefix, name)
			}
		}
	}
	return nil
}

// isDottedName reports whether name contains at least one namespace and valid Starlark segments.
func isDottedName(name string) bool {
	segments := strings.Split(name, ".")
	if len(segments) < minimumDottedSegments {
		return false
	}
	for _, segment := range segments {
		if !binding.ValidIdentifier(segment) {
			return false
		}
	}
	return true
}
