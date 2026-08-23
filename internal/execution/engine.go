package execution

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/meigma/codemode/internal/catalog"
)

// Engine owns the frozen capability namespace shared by fresh execution threads.
type Engine struct {
	// predeclared contains immutable namespaced capability builtins.
	predeclared starlark.StringDict
}

// New compiles one immutable execution engine from a validated filtered catalog.
func New(capabilityCatalog *catalog.Catalog) (*Engine, error) {
	if capabilityCatalog == nil {
		return nil, fmt.Errorf("%w: nil catalog", ErrInternal)
	}
	predeclared, err := buildPredeclared(capabilityCatalog)
	if err != nil {
		return nil, err
	}
	return &Engine{predeclared: predeclared}, nil
}
