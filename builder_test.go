package codemode_test

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"strings"
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
	// Count is intentionally unsupported as a 32-bit integer.
	Count int32 `json:"count"`
}

// interfaceBuilderOutput is an unsupported interface/any nested output.
type interfaceBuilderOutput struct {
	// Value is intentionally an unconstrained interface.
	Value any `json:"value"`
}

// rawMessageBuilderOutput is an unsupported [json.RawMessage] nested output.
type rawMessageBuilderOutput struct {
	// Value is intentionally opaque JSON.
	Value json.RawMessage `json:"value"`
}

// cyclicBuilderOutput is an unsupported cyclic nested output.
type cyclicBuilderOutput struct {
	// Next is a self-referential pointer that must fail registration.
	Next *cyclicBuilderOutput `json:"next"`
}

// marshalerBuilderValue is a custom JSON marshaler used only as a rejected fixture.
type marshalerBuilderValue struct{}

// MarshalJSON exists so registration rejects custom marshalers.
func (marshalerBuilderValue) MarshalJSON() ([]byte, error) {
	return []byte(`""`), nil
}

// marshalerBuilderOutput is an unsupported custom-marshaler nested output.
type marshalerBuilderOutput struct {
	// Value is intentionally a custom JSON marshaler.
	Value marshalerBuilderValue `json:"value"`
}

// mapKeyBuilderOutput is an unsupported non-string map key nested output.
type mapKeyBuilderOutput struct {
	// Value is intentionally keyed by integers.
	Value map[int]string `json:"value"`
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

// TestRegisterRejectsUnsupportedNestedOutputsBeforeRetention proves representative
// unsupportable output graphs never enter the builder.
func TestRegisterRejectsUnsupportedNestedOutputsBeforeRetention(t *testing.T) {
	tests := []struct {
		// name identifies the unsupported nested output.
		name string

		// register attempts one invalid public registration.
		register func(*codemode.Builder) error
	}{
		{
			name: "interface any",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(builder, codemode.Capability[builderInput, interfaceBuilderOutput]{
					ID:          "cap.interface",
					Name:        "records.interface",
					Summary:     "Unsupported interface output.",
					Description: "Rejected before retention.",
					Handler: func(context.Context, authz.Subject, builderInput) (interfaceBuilderOutput, error) {
						return interfaceBuilderOutput{}, nil
					},
				})
			},
		},
		{
			name: "json.RawMessage",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(builder, codemode.Capability[builderInput, rawMessageBuilderOutput]{
					ID:          "cap.raw",
					Name:        "records.raw",
					Summary:     "Unsupported raw message output.",
					Description: "Rejected before retention.",
					Handler: func(context.Context, authz.Subject, builderInput) (rawMessageBuilderOutput, error) {
						return rawMessageBuilderOutput{}, nil
					},
				})
			},
		},
		{
			name: "cyclic type",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(builder, codemode.Capability[builderInput, cyclicBuilderOutput]{
					ID:          "cap.cycle",
					Name:        "records.cycle",
					Summary:     "Unsupported cyclic output.",
					Description: "Rejected before retention.",
					Handler: func(context.Context, authz.Subject, builderInput) (cyclicBuilderOutput, error) {
						return cyclicBuilderOutput{}, nil
					},
				})
			},
		},
		{
			name: "custom marshaler",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(builder, codemode.Capability[builderInput, marshalerBuilderOutput]{
					ID:          "cap.marshaler",
					Name:        "records.marshaler",
					Summary:     "Unsupported marshaler output.",
					Description: "Rejected before retention.",
					Handler: func(context.Context, authz.Subject, builderInput) (marshalerBuilderOutput, error) {
						return marshalerBuilderOutput{}, nil
					},
				})
			},
		},
		{
			name: "non-string map key",
			register: func(builder *codemode.Builder) error {
				return codemode.Register(builder, codemode.Capability[builderInput, mapKeyBuilderOutput]{
					ID:          "cap.mapkey",
					Name:        "records.mapkey",
					Summary:     "Unsupported map key output.",
					Description: "Rejected before retention.",
					Handler: func(context.Context, authz.Subject, builderInput) (mapKeyBuilderOutput, error) {
						return mapKeyBuilderOutput{}, nil
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll(), Limits: codemode.DefaultLimits()})

			err := tt.register(builder)

			require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
			require.NoError(t, codemode.Register(builder, validBuilderCapability("cap.retained", "records.retained")))
			server, buildErr := builder.Build()
			require.NoError(t, buildErr)
			results, searchErr := server.Search("unsupported")
			require.NoError(t, searchErr)
			assert.Empty(t, results)
		})
	}
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

// TestBuilderBuildsWorkerProbe proves Build completes the real same-binary handshake.
func TestBuilderBuildsWorkerProbe(t *testing.T) {
	builder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     codemode.DefaultLimits(),
	})

	server, err := builder.Build()

	require.NoError(t, err)
	require.NotNil(t, server)
}

// TestBuilderRejectsWorkerFrameOverflow proves catalog-dependent cap arithmetic fails registration.
func TestBuilderRejectsWorkerFrameOverflow(t *testing.T) {
	limits := codemode.DefaultLimits()
	limits.MaxValueBytes = int(math.MaxUint32) - 64
	builder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     limits,
	})
	require.NoError(t, codemode.Register(
		builder,
		validBuilderCapability(
			codemode.CapabilityID("cap."+strings.Repeat("x", 128)),
			"records.lookup",
		),
	))

	server, err := builder.Build()

	require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
	assert.Nil(t, server)
	assert.Equal(t, 1, strings.Count(err.Error(), "invalid registration:"))
}

// TestBuilderReportsMiswiredWorker proves a final binary without the worker entry gets bounded guidance.
func TestBuilderReportsMiswiredWorker(t *testing.T) {
	command := exec.CommandContext(t.Context(), "go", "run", "./testdata/miswired")

	output, err := command.CombinedOutput()

	require.Error(t, err)
	message := string(output)
	assert.Contains(t, message, "CodeMode worker probe failed: child exited with status 1")
	assert.Contains(t, message, "worker stderr:")
	assert.Contains(t, message, "Build ran in CodeMode worker mode")
	assert.Less(t, len(output), 12*1024)
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
