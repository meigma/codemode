package codemode

import (
	"context"
	"errors"
	"fmt"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/catalog"
	"github.com/meigma/codemode/internal/execution"
	"github.com/meigma/codemode/internal/worker"
)

// dispatcher is the unexported authoritative native-call owner.
type dispatcher struct {
	// catalog is the immutable statically filtered capability set.
	catalog *catalog.Catalog

	// authorizer is called once after parent re-binding succeeds.
	authorizer authz.Authorizer

	// maxValueDepth bounds every parent-produced native result.
	maxValueDepth int

	// maxValueBytes supplies the byte-derived materialization bound.
	maxValueBytes int
}

// newDispatcher constructs one request-neutral dispatcher from retained server state.
func newDispatcher(
	capabilityCatalog *catalog.Catalog,
	authorizer authz.Authorizer,
	maxValueDepth int,
	maxValueBytes int,
) *dispatcher {
	return &dispatcher{
		catalog:       capabilityCatalog,
		authorizer:    authorizer,
		maxValueDepth: maxValueDepth,
		maxValueBytes: maxValueBytes,
	}
}

// dispatch looks up an enabled ID, re-binds, authorizes, invokes, and converts typed output.
func (dispatch *dispatcher) dispatch(
	ctx context.Context,
	subject authz.Subject,
	id string,
	args map[string]any,
) (any, error) {
	if dispatch == nil || dispatch.catalog == nil || dispatch.authorizer == nil {
		return nil, execution.ErrInternal
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return nil, contextErr
	}
	entry, ok := dispatch.catalog.LookupID(id)
	if !ok || entry.Plan == nil || entry.Invoke == nil {
		return nil, worker.ErrProtocol
	}
	input, canonical, bindingErr := entry.Plan.BindValue(args)
	if bindingErr != nil {
		return nil, fmt.Errorf("%w: %w", worker.ErrProtocol, bindingErr)
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return nil, contextErr
	}
	policyDone := make(chan error, 1)
	go func() {
		policyDone <- authorize(ctx, dispatch.authorizer, subject, entry, canonical)
	}()
	select {
	case <-ctx.Done():
		return nil, contextFailure(ctx)
	case policyErr := <-policyDone:
		if policyErr != nil {
			return nil, policyErr
		}
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return nil, contextErr
	}
	invocationDone := make(chan invocationOutcome, 1)
	go func() {
		output, err := invoke(ctx, subject, entry, input)
		invocationDone <- invocationOutcome{output: output, err: err}
	}()
	var outcome invocationOutcome
	select {
	case <-ctx.Done():
		return nil, contextFailure(ctx)
	case outcome = <-invocationDone:
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return nil, contextErr
	}
	if outcome.err != nil {
		return nil, outcome.err
	}
	converted, conversionErr := entry.Plan.ConvertOutput(outcome.output)
	if conversionErr != nil {
		return nil, fmt.Errorf("%w: %w", execution.ErrCapabilityFailure, conversionErr)
	}
	if validationErr := binding.ValidateValue(
		converted,
		dispatch.maxValueDepth,
		dispatch.maxValueBytes,
	); validationErr != nil {
		if errors.Is(validationErr, binding.ErrValueLimit) {
			return nil, fmt.Errorf("%w: %w", execution.ErrResourceLimit, validationErr)
		}
		return nil, fmt.Errorf("%w: %w", execution.ErrCapabilityFailure, validationErr)
	}
	return converted, nil
}

// authorize calls policy once and converts denial, error, and panic to safe internal classes.
func authorize(
	ctx context.Context,
	authorizer authz.Authorizer,
	subject authz.Subject,
	entry catalog.Entry,
	arguments map[string]any,
) (err error) {
	defer func() {
		if recover() != nil {
			err = execution.ErrPolicyFailure
		}
	}()
	err = authorizer.Authorize(ctx, authz.AuthorizationInput{
		Subject:        subject,
		CapabilityID:   entry.ID,
		CapabilityName: entry.Name,
		Arguments:      arguments,
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, authz.ErrDenied) {
		return fmt.Errorf("%w: %w", execution.ErrPermissionDenied, err)
	}
	return fmt.Errorf("%w: %w", execution.ErrPolicyFailure, err)
}

// invocationOutcome carries one recovered native handler result without named returns.
type invocationOutcome struct {
	// output is the handler's typed result.
	output any

	// err is the handler or recovery failure.
	err error
}

// errRecoveredHandlerPanic distinguishes a recovered panic from an ordinary handler error.
var errRecoveredHandlerPanic = errors.New("recovered handler panic")

// invoke calls one typed handler and recovers handler panics at the native boundary.
func invoke(ctx context.Context, subject authz.Subject, entry catalog.Entry, input any) (any, error) {
	outcome := invocationOutcome{}
	func() {
		defer func() {
			if recover() != nil {
				outcome.output = nil
				outcome.err = errRecoveredHandlerPanic
			}
		}()
		outcome.output, outcome.err = entry.Invoke(ctx, subject, input)
	}()
	if outcome.err == nil {
		return outcome.output, nil
	}
	if errors.Is(outcome.err, errRecoveredHandlerPanic) {
		return nil, execution.ErrInternal
	}
	if errors.Is(outcome.err, catalog.ErrInputTypeMismatch) {
		return nil, fmt.Errorf("%w: %w", execution.ErrInternal, outcome.err)
	}
	return nil, fmt.Errorf("%w: %w", execution.ErrCapabilityFailure, outcome.err)
}

// contextFailure projects request cancellation directly and deadlines as resource exhaustion.
func contextFailure(ctx context.Context) error {
	if ctx == nil {
		return execution.ErrInternal
	}
	switch err := ctx.Err(); {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", execution.ErrResourceLimit, context.DeadlineExceeded)
	default:
		return nil
	}
}
