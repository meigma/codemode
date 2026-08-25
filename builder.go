package codemode

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/catalog"
)

// Options configures one immutable CodeMode server build.
type Options struct {
	// Authorizer decides whether each validated native capability call may dispatch.
	Authorizer authz.Authorizer

	// DisabledCapabilities lists stable capability IDs removed from every live server surface.
	DisabledCapabilities []CapabilityID

	// Limits contains execution, conversion, and discovery budgets. Build
	// replaces each zero-valued field with the corresponding DefaultLimits value.
	Limits Limits
}

// Builder collects capability registrations for one immutable Server.
//
// A Builder is single-threaded and one-shot. Its first Build call closes registration even when
// validation fails. Construct another Builder to change configuration or capability visibility.
type Builder struct {
	// authorizer is the required authorization port retained by the eventual server.
	authorizer authz.Authorizer

	// disabledCapabilities is a private copy of the static deployment filter.
	disabledCapabilities []string

	// limits is the value-copied server budget configuration.
	limits Limits

	// registrations contains validated, precompiled capability registrations until Build.
	registrations []catalog.Registration

	// registrationErrors contains capability-specific failures deferred to Build.
	registrationErrors []error

	// built reports whether Build has closed this builder.
	built bool
}

// New creates a mutable one-shot Builder and copies caller-owned option slices.
//
// The final binary must call ServeWorkerAndExit as the first statement of main
// before it calls New or performs ordinary host setup. Test binaries that call
// Build must make the same call as the first statement of TestMain.
func New(options Options) *Builder {
	disabled := make([]string, len(options.DisabledCapabilities))
	for index, id := range options.DisabledCapabilities {
		disabled[index] = string(id)
	}
	return &Builder{
		authorizer:           options.Authorizer,
		disabledCapabilities: disabled,
		limits:               options.Limits,
	}
}

// Register compiles and retains one typed capability without erasing its
// binding contract first.
//
// Capability-specific failures are accumulated and returned together by Build.
// Register panics when builder is nil or already closed because no future Build
// call can report those lifecycle violations.
func Register[Input, Output any](builder *Builder, capability Capability[Input, Output]) {
	if builder == nil {
		panic(fmt.Errorf("%w: nil builder", ErrInvalidRegistration))
	}
	if builder.built {
		panic(fmt.Errorf("%w: builder is already closed", ErrInvalidRegistration))
	}
	if capability.Handler == nil {
		builder.recordRegistrationError(capability.Name, errors.New("handler must not be nil"))
		return
	}

	plan, err := binding.CompileFor[Input, Output]()
	if err != nil {
		builder.recordRegistrationError(capability.Name, err)
		return
	}
	id := capability.ID
	if id == "" {
		id = CapabilityID(capability.Name)
	}
	description := capability.Description
	if description == "" {
		description = capability.Summary
	}
	handler := capability.Handler
	registration := catalog.Registration{
		ID:          string(id),
		Name:        string(capability.Name),
		Summary:     capability.Summary,
		Description: description,
		Plan:        plan,
		Invoke: func(ctx context.Context, subject authz.Subject, input any) (any, error) {
			typed, ok := input.(Input)
			if !ok {
				return nil, catalog.ErrInputTypeMismatch
			}
			return handler(ctx, subject, typed)
		},
	}
	if err := catalog.ValidateRegistration(registration); err != nil {
		builder.recordRegistrationError(capability.Name, err)
		return
	}
	for _, existing := range builder.registrations {
		if existing.ID == registration.ID {
			builder.recordRegistrationError(
				capability.Name,
				fmt.Errorf("duplicate capability ID %q", registration.ID),
			)
			return
		}
		if existing.Name == registration.Name {
			builder.recordRegistrationError(
				capability.Name,
				fmt.Errorf("duplicate capability name %q", registration.Name),
			)
			return
		}
	}
	builder.registrations = append(builder.registrations, registration)
}

// recordRegistrationError retains one capability-specific programmer error.
func (builder *Builder) recordRegistrationError(name CapabilityName, err error) {
	builder.registrationErrors = append(builder.registrationErrors, fmt.Errorf(
		"%w: capability %q: %w",
		ErrInvalidRegistration,
		name,
		err,
	))
}

// Build closes the Builder and returns an immutable concurrency-safe Server
// after full validation and a same-executable worker probe.
//
// Build allows up to five seconds for the probe exchange, then kills and reaps
// the probe child; operating-system spawn and kill/reap overhead can extend the
// call beyond that exchange deadline. Build has no context and the probe
// deadline is not configurable.
//
// The final binary must call ServeWorkerAndExit as the first statement of main,
// and a test binary that calls Build must do the same in TestMain. The probe
// detects an absent or nonfunctional worker entry, but it cannot detect ordinary
// host work that completes silently before ServeWorkerAndExit is called.
func (builder *Builder) Build() (*Server, error) {
	if builder == nil {
		return nil, fmt.Errorf("%w: nil builder", ErrInvalidRegistration)
	}
	if builder.built {
		return nil, fmt.Errorf("%w: builder is already closed", ErrInvalidRegistration)
	}
	builder.built = true
	registrations := slices.Clone(builder.registrations)
	builder.registrations = nil
	buildErrors := slices.Clone(builder.registrationErrors)
	builder.registrationErrors = nil
	limits := builder.limits.withDefaults()

	if isNilAuthorizer(builder.authorizer) {
		buildErrors = append(
			buildErrors,
			fmt.Errorf("%w: Authorizer must not be nil", ErrInvalidRegistration),
		)
	}
	if err := limits.Validate(); err != nil {
		buildErrors = append(buildErrors, err)
	}
	if len(buildErrors) > 0 {
		return nil, errors.Join(buildErrors...)
	}
	capabilityCatalog, err := catalog.Build(registrations, catalog.Options{
		DisabledCapabilities: slices.Clone(builder.disabledCapabilities),
		MaxSearchQueryBytes:  limits.MaxSearchQueryBytes,
		MaxSearchResults:     limits.MaxSearchResults,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRegistration, err)
	}
	server, err := newServer(capabilityCatalog, builder.authorizer, limits)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRegistration, err)
	}
	return server, nil
}

// isNilAuthorizer reports whether an authorization interface is nil or contains a typed nil value.
func isNilAuthorizer(authorizer authz.Authorizer) bool {
	if authorizer == nil {
		return true
	}
	value := reflect.ValueOf(authorizer)
	kind := value.Kind()
	nilable := kind == reflect.Chan || kind == reflect.Func || kind == reflect.Map ||
		kind == reflect.Pointer || kind == reflect.Slice
	return nilable && value.IsNil()
}
