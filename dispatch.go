package codemode

import (
	"context"
	"errors"
	"fmt"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/catalog"
	"github.com/meigma/codemode/internal/execution"
)

// dispatcher is the unexported authoritative native-call owner.
type dispatcher struct {
	// catalog is the immutable statically filtered capability set.
	catalog *catalog.Catalog

	// authorizer is called once after parent re-binding succeeds.
	authorizer authz.Authorizer
}

// newDispatcher constructs one request-neutral dispatcher from retained server state.
func newDispatcher(capabilityCatalog *catalog.Catalog, authorizer authz.Authorizer) *dispatcher {
	return &dispatcher{
		catalog:    capabilityCatalog,
		authorizer: authorizer,
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
		return nil, execution.ErrInternal
	}
	input, canonical, bindingErr := entry.Plan.BindValue(args)
	if bindingErr != nil {
		return nil, fmt.Errorf("%w: %w", execution.ErrInternal, bindingErr)
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return nil, contextErr
	}
	if policyErr := authorize(ctx, dispatch.authorizer, subject, entry, canonical); policyErr != nil {
		return nil, policyErr
	}
	if contextErr := contextFailure(ctx); contextErr != nil {
		return nil, contextErr
	}
	output, invocationErr := invoke(ctx, subject, entry, input)
	if invocationErr != nil {
		return nil, invocationErr
	}
	converted, conversionErr := entry.Plan.ConvertOutput(output)
	if conversionErr != nil {
		return nil, fmt.Errorf("%w: %w", execution.ErrCapabilityFailure, conversionErr)
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
