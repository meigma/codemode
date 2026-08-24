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

	// authorizer is called once before each valid native handler dispatch.
	authorizer authz.Authorizer

	// limits is the immutable execution budget configuration.
	limits execution.Limits
}

// newServer compiles a Server from fully validated retained state.
func newServer(
	capabilityCatalog *catalog.Catalog,
	authorizer authz.Authorizer,
	limits Limits,
) (*Server, error) {
	engine, err := execution.New(capabilityCatalog)
	if err != nil {
		return nil, err
	}
	return &Server{
		catalog:    capabilityCatalog,
		engine:     engine,
		authorizer: authorizer,
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
	if server == nil || server.engine == nil || server.authorizer == nil {
		return nil, ErrInternal
	}
	if ctx == nil {
		return nil, ErrInternal
	}
	if subject.ID == "" {
		return nil, ErrUnauthenticated
	}
	result, err := server.engine.Execute(ctx, subject, string(program), server.authorizer, server.limits)
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
