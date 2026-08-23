package execution

import (
	"fmt"
	"maps"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/meigma/codemode/internal/catalog"
)

// namespaceNode is one temporary registration-time path used to assemble frozen modules.
type namespaceNode struct {
	// children contains nested namespace segments.
	children map[string]*namespaceNode

	// functions contains native capability leaves.
	functions starlark.StringDict
}

// buildPredeclared creates the frozen capability namespace for one immutable Engine.
func buildPredeclared(capabilityCatalog *catalog.Catalog) (starlark.StringDict, error) {
	root := newNamespaceNode()
	for _, namespaceBinding := range capabilityCatalog.NamespaceBindings() {
		node := root
		for _, segment := range namespaceBinding.Segments {
			child, ok := node.children[segment]
			if !ok {
				child = newNamespaceNode()
				node.children[segment] = child
			}
			node = child
		}
		if _, duplicate := node.functions[namespaceBinding.Function]; duplicate {
			return nil, fmt.Errorf("%w: duplicate namespace function", ErrInternal)
		}
		entry := namespaceBinding.Capability
		node.functions[namespaceBinding.Function] = starlark.NewBuiltin(entry.Name, func(
			thread *starlark.Thread,
			_ *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			return callCapability(thread, entry, args, kwargs)
		})
	}

	predeclared := make(starlark.StringDict, len(root.children))
	for name, node := range root.children {
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
