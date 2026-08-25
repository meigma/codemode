package codemode

import (
	"context"
	"errors"
	"fmt"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/catalog"
	"github.com/meigma/codemode/internal/execution"
	"github.com/meigma/codemode/internal/worker"
)

// Program is one bounded Starlark source program executed by a Server.
type Program string

// SearchResult is one compact enabled-capability discovery record.
type SearchResult = catalog.SearchResult

// Description is one exact enabled-capability description and supported binding shape.
type Description = catalog.Description

// Server is an immutable, concurrency-safe capability catalog and Starlark
// execution service.
//
// Every Execute call runs Starlark in a fresh worker process and owns fresh
// budgets. An elapsed deadline kills and reaps that worker. Registered
// Authorizer and Handler implementations run in the parent, must honor their
// context, return promptly, and be safe for the caller's concurrency.
type Server struct {
	// catalog is the immutable statically filtered capability set.
	catalog *catalog.Catalog

	// runner owns fresh worker execution and parent dispatch.
	runner *worker.Runner
}

// newServer constructs and probes a Server from fully validated retained state.
func newServer(
	capabilityCatalog *catalog.Catalog,
	authorizer authz.Authorizer,
	limits Limits,
) (*Server, error) {
	dispatch := newDispatcher(
		capabilityCatalog,
		authorizer,
		limits.MaxValueDepth,
		limits.MaxValueBytes,
	)
	runner, err := worker.NewRunner(
		capabilityBindings(capabilityCatalog),
		worker.Limits{
			MaxSourceBytes:            limits.MaxSourceBytes,
			MaxExecutionSteps:         limits.MaxExecutionSteps,
			MaxExecutionTime:          limits.MaxExecutionTime,
			MaxNativeCalls:            limits.MaxNativeCalls,
			MaxValueDepth:             limits.MaxValueDepth,
			MaxValueBytes:             limits.MaxValueBytes,
			MaxIntermediateValueBytes: limits.MaxIntermediateValueBytes,
			MaxConcurrentExecutions:   limits.MaxConcurrentExecutions,
		},
		dispatch.dispatch,
	)
	if err != nil {
		return nil, err
	}
	if err := runner.Probe(); err != nil {
		return nil, err
	}
	return &Server{catalog: capabilityCatalog, runner: runner}, nil
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

// Execute runs one bounded program for a trusted authenticated subject and
// returns only main's final value.
//
// Execute re-executes the current binary for each call. The elapsed budget
// includes worker-slot waiting, process startup, protocol exchange, Starlark
// execution, and parent dispatch. Deadline or request cancellation kills and
// reaps the child, but CodeMode cannot forcibly stop parent-side Authorizer or
// Handler code that ignores its context.
func (server *Server) Execute(ctx context.Context, subject authz.Subject, program Program) (any, error) {
	if server == nil || server.runner == nil || ctx == nil {
		return nil, ErrInternal
	}
	if subject.ID == "" {
		return nil, ErrUnauthenticated
	}
	result, err := server.runner.Execute(ctx, subject, string(program))
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
