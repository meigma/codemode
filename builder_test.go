package codemode_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
)

// builderInput is the representative builder test input.
type builderInput struct {
	// Value is one required capability argument.
	Value string `json:"value"`
}

// builderOutput is the representative builder test output.
type builderOutput struct {
	// Value is one handler result field.
	Value string `json:"value"`
}

// invalidBuilderInput contains a type outside the restricted binder contract.
type invalidBuilderInput struct {
	// Count is intentionally unsupported as a required integer.
	Count int64 `json:"count"`
}

// nilPolicy is a typed-nil authorization implementation used to test Build validation.
type nilPolicy struct{}

// Authorize permits an invocation when the receiver is non-nil.
func (*nilPolicy) Authorize(context.Context, authz.AuthorizationInput) error {
	return nil
}

// TestBuilderBuildsOnceAndClosesRegistration proves the mutable lifecycle ends at the first Build call.
func TestBuilderBuildsOnceAndClosesRegistration(t *testing.T) {
	builder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     codemode.DefaultLimits(),
	})
	require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.one", "records.one")))

	server, err := builder.Build()

	require.NoError(t, err)
	require.NotNil(t, server)
	_, err = builder.Build()
	require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
	err = codemode.Register(builder, validBuilderCapability("cap.two", "records.two"))
	require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
}

// TestBuilderCopiesStaticFilteringOptions proves caller slice mutation cannot alter retained configuration.
func TestBuilderCopiesStaticFilteringOptions(t *testing.T) {
	disabled := []codemode.CapabilityID{"cap.disabled"}
	builder := codemode.New(codemode.Options{
		Authorizer:           authz.AllowAll(),
		DisabledCapabilities: disabled,
		Limits:               codemode.DefaultLimits(),
	})
	disabled[0] = "cap.unknown"
	require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.disabled", "records.disabled")))

	server, err := builder.Build()

	require.NoError(t, err)
	results, err := server.Search("disabled")
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestBuilderRejectsNilAuthorizersAndInvalidLimits proves required construction policy fails closed.
func TestBuilderRejectsNilAuthorizersAndInvalidLimits(t *testing.T) {
	var typedNil *nilPolicy
	tests := []struct {
		// name identifies the invalid server options.
		name string

		// options contains the construction input.
		options codemode.Options
	}{
		{name: "nil authorizer", options: codemode.Options{Limits: codemode.DefaultLimits()}},
		{
			name:    "typed nil authorizer",
			options: codemode.Options{Authorizer: typedNil, Limits: codemode.DefaultLimits()},
		},
		{name: "zero limits", options: codemode.Options{Authorizer: authz.AllowAll()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := codemode.New(tt.options)

			server, err := builder.Build()

			require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
			assert.Nil(t, server)
			_, secondErr := builder.Build()
			require.ErrorIs(t, secondErr, codemode.ErrInvalidRegistration)
		})
	}
}

// TestRegisterRejectsInvalidContractsBeforeRetention proves malformed capabilities never enter the builder.
func TestRegisterRejectsInvalidContractsBeforeRetention(t *testing.T) {
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: codemode.DefaultLimits()})
	invalidMetadata := validBuilderCapability("", "records.valid")
	nilHandler := validBuilderCapability("cap.nil", "records.nil")
	nilHandler.Handler = nil

	require.ErrorIs(t, codemode.Register(builder, invalidMetadata), codemode.ErrInvalidRegistration)
	require.ErrorIs(t, codemode.Register(builder, nilHandler), codemode.ErrInvalidRegistration)
	require.ErrorIs(t, codemode.Register(builder, codemode.Capability[invalidBuilderInput, builderOutput]{
		ID:          "cap.unsupported",
		Name:        "records.unsupported",
		Summary:     "Unsupported input.",
		Description: "Exercises compile-before-erasure validation.",
		Handler: func(context.Context, authz.Subject, invalidBuilderInput) (builderOutput, error) {
			return builderOutput{}, nil
		},
	}), codemode.ErrInvalidRegistration)
	require.ErrorIs(t, codemode.Register[builderInput, builderOutput](nil, validBuilderCapability(
		"cap.nil_builder",
		"records.nil_builder",
	)), codemode.ErrInvalidRegistration)
}

// TestRegisterRejectsObviousDuplicates proves duplicate stable and model-facing identities fail immediately.
func TestRegisterRejectsObviousDuplicates(t *testing.T) {
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: codemode.DefaultLimits()})
	require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.one", "records.one")))

	require.ErrorIs(
		t,
		codemode.Register(builder, validBuilderCapability("cap.one", "records.two")),
		codemode.ErrInvalidRegistration,
	)
	require.ErrorIs(
		t,
		codemode.Register(builder, validBuilderCapability("cap.two", "records.one")),
		codemode.ErrInvalidRegistration,
	)
}

// TestBuildRejectsWholeCatalogFailures proves filtering and namespace checks run before a server escapes.
func TestBuildRejectsWholeCatalogFailures(t *testing.T) {
	t.Run("unknown disabled capability", func(t *testing.T) {
		builder := codemode.New(codemode.Options{
			Authorizer:           authz.AllowAll(),
			DisabledCapabilities: []codemode.CapabilityID{"cap.unknown"},
			Limits:               codemode.DefaultLimits(),
		})

		server, err := builder.Build()

		require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
		assert.Nil(t, server)
	})

	t.Run("function namespace collision", func(t *testing.T) {
		builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: codemode.DefaultLimits()})
		require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.lookup", "records.lookup")))
		require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.detail", "records.lookup.detail")))

		server, err := builder.Build()

		require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
		assert.Nil(t, server)
	})
}

// validBuilderCapability constructs one valid representative registration.
func validBuilderCapability(
	id codemode.CapabilityID,
	name codemode.CapabilityName,
) codemode.Capability[builderInput, builderOutput] {
	return codemode.Capability[builderInput, builderOutput]{
		ID:          id,
		Name:        name,
		Summary:     "Return one record.",
		Description: "Returns the supplied record value.",
		Handler: func(_ context.Context, _ authz.Subject, input builderInput) (builderOutput, error) {
			return builderOutput(input), nil
		},
	}
}
