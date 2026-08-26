package execution

import (
	"fmt"
	"maps"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/meigma/codemode/internal/universe"
)

// namespaceNode is one temporary registration-time path used to assemble frozen modules.
type namespaceNode struct {
	// children contains nested namespace segments.
	children map[string]*namespaceNode

	// functions contains native capability leaves.
	functions starlark.StringDict
}

// buildPredeclared merges the fixed language surface with frozen capability namespaces.
func buildPredeclared(bindings []CapabilityBinding) (starlark.StringDict, error) {
	root := newNamespaceNode()
	for _, capability := range bindings {
		segments, function := splitCapabilityName(capability.Name)
		node := root
		for _, segment := range segments {
			child, ok := node.children[segment]
			if !ok {
				child = newNamespaceNode()
				node.children[segment] = child
			}
			node = child
		}
		if _, duplicate := node.functions[function]; duplicate {
			return nil, fmt.Errorf("%w: duplicate namespace function", ErrInternal)
		}
		id := capability.ID
		input := capability.Input
		name := capability.Name
		node.functions[function] = starlark.NewBuiltin(name, func(
			thread *starlark.Thread,
			_ *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			return callCapability(thread, id, input, args, kwargs)
		})
	}

	predeclared := universe.Predeclared()
	for name, node := range root.children {
		if _, exists := predeclared[name]; exists {
			return nil, fmt.Errorf("%w: capability root %q collides with the fixed language surface", ErrInternal, name)
		}
		predeclared[name] = freezeModule(name, node)
	}
	predeclared.Freeze()
	return predeclared, nil
}

// newNamespaceNode creates an empty mutable namespace assembly node.
func newNamespaceNode() *namespaceNode {
	return &namespaceNode{
		children:  make(map[string]*namespaceNode),
		functions: make(starlark.StringDict),
	}
}

// freezeModule recursively converts one assembly node to an immutable Starlark module.
func freezeModule(name string, node *namespaceNode) *starlarkstruct.Module {
	members := make(starlark.StringDict, len(node.children)+len(node.functions))
	maps.Copy(members, node.functions)
	for segment, child := range node.children {
		members[segment] = freezeModule(name+"."+segment, child)
	}
	module := &starlarkstruct.Module{Name: name, Members: members}
	module.Freeze()
	return module
}
