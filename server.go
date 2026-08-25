package codemode

import (
	"context"
	"errors"
	"fmt"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/catalog"
	"github.com/meigma/codemode/internal/execution"
)

// Program is one bounded Starlark source program executed by a Server.
type Program string

// SearchResult is one compact enabled-capability discovery record.
type SearchResult = catalog.SearchResult

// Description is one exact enabled-capability description and supported binding shape.
type Description = catalog.Description

// Server is an immutable, concurrency-safe capability catalog and Starlark execution service.
//
// Every Execute call owns a fresh interpreter and budgets. The elapsed budget cancels Starlark
// evaluation; registered Authorizer and Handler implementations must honor their context, return
// promptly, and be safe for the caller's concurrency.
type Server struct {
	// catalog is the immutable statically filtered capability set.
	catalog *catalog.Catalog

	// engine owns the immutable precompiled native capability namespace.
	engine *execution.Engine

	// dispatcher is the authoritative native-call owner for this server.
	dispatcher *dispatcher

	// limits is the immutable execution budget configuration.
	limits execution.Limits
}

// newServer compiles a Server from fully validated retained state.
func newServer(
	capabilityCatalog *catalog.Catalog,
	authorizer authz.Authorizer,
	limits Limits,
) (*Server, error) {
	engine, err := execution.New(capabilityBindings(capabilityCatalog))
	if err != nil {
		return nil, err
	}
	return &Server{
		catalog:    capabilityCatalog,
		engine:     engine,
		dispatcher: newDispatcher(capabilityCatalog, authorizer),
		limits: execution.Limits{
			MaxSourceBytes:    limits.MaxSourceBytes,
			MaxExecutionSteps: limits.MaxExecutionSteps,
			MaxExecutionTime:  limits.MaxExecutionTime,
			MaxNativeCalls:    limits.MaxNativeCalls,
			MaxValueDepth:     limits.MaxValueDepth,
			MaxResultBytes:    limits.MaxResultBytes,
		},
	}, nil
}

// capabilityBindings derives process-neutral engine bindings from enabled catalog entries.
func capabilityBindings(capabilityCatalog *catalog.Catalog) []execution.CapabilityBinding {
	entries := capabilityCatalog.Entries()
	bindings := make([]execution.CapabilityBinding, len(entries))
	for index, entry := range entries {
		bindings[index] = execution.CapabilityBinding{
			ID:    entry.ID,
			Name:  entry.Name,
			Input: entry.Plan.InputShape(),
		}
	}
	return bindings
}

// Search returns a bounded name-sorted scan of enabled capability names and summaries.
func (server *Server) Search(query string) ([]SearchResult, error) {
	if server == nil || server.catalog == nil {
		return nil, ErrInternal
	}
	results, err := server.catalog.Search(query)
	if err != nil {
		if errors.Is(err, catalog.ErrSearchQueryLimit) {
			return nil, ErrResourceLimit
		}
		return nil, ErrInternal
	}
	return results, nil
}

// Describe returns one exact enabled capability description or ErrNotFound.
func (server *Server) Describe(name CapabilityName) (Description, error) {
	if server == nil || server.catalog == nil {
		return Description{}, ErrInternal
	}
	description, ok := server.catalog.Describe(string(name))
	if !ok {
		return Description{}, ErrNotFound
	}
	return description, nil
}

// Execute runs one bounded program for a trusted authenticated subject and returns only main's final value.
//
// The elapsed budget cancels Starlark evaluation. Authorizer and Handler implementations must
// honor ctx and return promptly; CodeMode does not detach or forcibly interrupt blocking Go code.
func (server *Server) Execute(ctx context.Context, subject authz.Subject, program Program) (any, error) {
	if server == nil || server.engine == nil || server.dispatcher == nil {
		return nil, ErrInternal
	}
	if ctx == nil {
		return nil, ErrInternal
	}
	if subject.ID == "" {
		return nil, ErrUnauthenticated
	}
	runCtx, cancel := context.WithTimeout(ctx, server.limits.MaxExecutionTime)
	defer cancel()
	if contextErr := contextFailure(runCtx); contextErr != nil {
		return nil, projectExecutionError(contextErr)
	}
	result, err := server.engine.Execute(
		runCtx,
		string(program),
		func(id string, args map[string]any) (any, error) {
			return server.dispatcher.dispatch(runCtx, subject, id, args)
		},
		server.limits,
	)
	if err != nil {
		return nil, projectExecutionError(err)
	}
	return result, nil
}

// projectExecutionError removes trusted execution causes at the root boundary.
// It preserves only safe sentinels and documented context cancellation and deadline wrapping.
func projectExecutionError(err error) error {
	switch {
	case errors.Is(err, execution.ErrInvalidProgram):
		return ErrInvalidProgram
	case errors.Is(err, execution.ErrInvalidArguments):
		return ErrInvalidArguments
	case errors.Is(err, execution.ErrPermissionDenied):
		return ErrPermissionDenied
	case errors.Is(err, execution.ErrPolicyFailure):
		return ErrPolicyFailure
	case errors.Is(err, execution.ErrResourceLimit):
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %w", ErrResourceLimit, context.DeadlineExceeded)
		}
		return ErrResourceLimit
	case errors.Is(err, execution.ErrCapabilityFailure):
		return ErrCapabilityFailure
	case errors.Is(err, execution.ErrInternal):
		return ErrInternal
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", ErrResourceLimit, context.DeadlineExceeded)
	default:
		return ErrInternal
	}
}
