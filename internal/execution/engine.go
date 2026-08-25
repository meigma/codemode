package execution

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"github.com/meigma/codemode/internal/binding"
)

const minimumDottedSegments = 2

// CapabilityBinding is one process-neutral capability exposed to the interpreter.
type CapabilityBinding struct {
	// ID is the stable capability identity passed to NativeCall.
	ID string

	// Name is the dotted Starlark path used to assemble the frozen namespace.
	Name string

	// Input is the exact compiled input shape used to bind keyword arguments.
	Input []binding.FieldShape
}

// NativeCall invokes one capability with a fresh canonical argument map.
//
// It returns one normalized process-neutral value or a classified error.
type NativeCall func(id string, arguments map[string]any) (any, error)

// Engine owns the frozen capability namespace shared by fresh execution threads.
type Engine struct {
	// predeclared contains immutable namespaced capability builtins.
	predeclared starlark.StringDict
}

// New compiles one immutable execution engine from process-neutral capability bindings.
func New(bindings []CapabilityBinding) (*Engine, error) {
	copied := copyBindings(bindings)
	if err := validateBindings(copied); err != nil {
		return nil, err
	}
	predeclared, err := buildPredeclared(copied)
	if err != nil {
		return nil, err
	}
	return &Engine{predeclared: predeclared}, nil
}

// copyBindings deep-copies caller-owned binding slices so Engine stays immutable.
func copyBindings(bindings []CapabilityBinding) []CapabilityBinding {
	copied := make([]CapabilityBinding, len(bindings))
	for index, capability := range bindings {
		input := make([]binding.FieldShape, len(capability.Input))
		copy(input, capability.Input)
		copied[index] = CapabilityBinding{
			ID:    capability.ID,
			Name:  capability.Name,
			Input: input,
		}
	}
	return copied
}

// validateBindings rejects empty or colliding identities and invalid input shapes.
func validateBindings(bindings []CapabilityBinding) error {
	ids := make(map[string]struct{}, len(bindings))
	names := make(map[string]struct{}, len(bindings))
	for _, capability := range bindings {
		if capability.ID == "" || capability.ID != strings.TrimSpace(capability.ID) {
			return fmt.Errorf("%w: empty capability ID", ErrInternal)
		}
		if _, duplicate := ids[capability.ID]; duplicate {
			return fmt.Errorf("%w: duplicate capability ID", ErrInternal)
		}
		if !isDottedName(capability.Name) {
			return fmt.Errorf("%w: capability name %q is not dotted", ErrInternal, capability.Name)
		}
		if _, duplicate := names[capability.Name]; duplicate {
			return fmt.Errorf("%w: duplicate namespace function", ErrInternal)
		}
		if err := binding.ValidateInputShape(capability.Input); err != nil {
			return fmt.Errorf("%w: %w", ErrInternal, err)
		}
		ids[capability.ID] = struct{}{}
		names[capability.Name] = struct{}{}
	}
	return validateNamespaceCollisions(names)
}

// validateNamespaceCollisions rejects a capability function that is also a namespace.
func validateNamespaceCollisions(names map[string]struct{}) error {
	for name := range names {
		segments := strings.Split(name, ".")
		for end := 1; end < len(segments); end++ {
			prefix := strings.Join(segments[:end], ".")
			if _, collides := names[prefix]; collides {
				return fmt.Errorf("%w: capability %q is also a namespace of %q", ErrInternal, prefix, name)
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

// splitCapabilityName returns the namespace path and function for a validated dotted name.
func splitCapabilityName(name string) ([]string, string) {
	segments := strings.Split(name, ".")
	return segments[:len(segments)-1], segments[len(segments)-1]
}
