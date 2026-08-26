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
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})
	codemode.Register(builder, validBuilderCapability("cap.one", "records.one"))

	server, err := builder.Build()

	require.NoError(t, err)
	require.NotNil(t, server)
	_, err = builder.Build()
	require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
	assert.Panics(t, func() {
		codemode.Register(builder, validBuilderCapability("cap.two", "records.two"))
	})
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
	codemode.Register(builder, validBuilderCapability("cap.disabled", "records.disabled"))

	server, err := builder.Build()

	require.NoError(t, err)
	response, err := server.Search("disabled")
	require.NoError(t, err)
	require.NotNil(t, response.Results)
	assert.Empty(t, response.Results)
	assert.False(t, response.Truncated)
}

// TestBuilderCopiesSearchTerms proves caller search-term mutation cannot alter discovery.
func TestBuilderCopiesSearchTerms(t *testing.T) {
	terms := []string{"open ticket"}
	capability := validBuilderCapability("cap.alpha", "records.alpha")
	capability.SearchTerms = terms
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})
	codemode.Register(builder, capability)
	terms[0] = "mutated after register"

	server, err := builder.Build()
	require.NoError(t, err)

	response, err := server.Search("ticket")
	require.NoError(t, err)
	require.NotNil(t, response.Results)
	require.Len(t, response.Results, 1)
	assert.Equal(t, "records.alpha", response.Results[0].Name)
	assert.False(t, response.Truncated)

	mutated, err := server.Search("mutated")
	require.NoError(t, err)
	require.NotNil(t, mutated.Results)
	assert.Empty(t, mutated.Results)
	assert.False(t, mutated.Truncated)
}

// TestBuilderRejectsNilAuthorizersAndNegativeLimits proves required construction policy fails closed.
func TestBuilderRejectsNilAuthorizersAndNegativeLimits(t *testing.T) {
	var typedNil *nilPolicy
	negativeLimits := codemode.Limits{MaxSourceBytes: -1}
	tests := []struct {
		// name identifies the invalid server options.
		name string

		// options contains the construction input.
		options codemode.Options
	}{
		{name: "nil authorizer", options: codemode.Options{}},
		{name: "typed nil authorizer", options: codemode.Options{Authorizer: typedNil}},
		{
			name:    "negative limit",
			options: codemode.Options{Authorizer: authz.AllowAll(), Limits: negativeLimits},
		},
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

// TestBuildJoinsRegistrationFailures proves one build reports every invalid capability by name.
func TestBuildJoinsRegistrationFailures(t *testing.T) {
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})
	invalidMetadata := validBuilderCapability("cap.metadata", "records.metadata")
	invalidMetadata.Summary = ""
	nilHandler := validBuilderCapability("cap.nil", "records.nil")
	nilHandler.Handler = nil

	codemode.Register(builder, invalidMetadata)
	codemode.Register(builder, nilHandler)
	codemode.Register(builder, codemode.Capability[invalidBuilderInput, builderOutput]{
		ID:          "cap.unsupported",
		Name:        "records.unsupported",
		Summary:     "Unsupported input.",
		Description: "Exercises compile-before-erasure validation.",
		Handler: func(context.Context, authz.Subject, invalidBuilderInput) (builderOutput, error) {
			return builderOutput{}, nil
		},
	})

	server, err := builder.Build()

	require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
	assert.Nil(t, server)
	require.ErrorContains(t, err, `capability "records.metadata"`)
	require.ErrorContains(t, err, `capability "records.nil"`)
	require.ErrorContains(t, err, `capability "records.unsupported"`)
	assert.Panics(t, func() {
		codemode.Register[builderInput, builderOutput](nil, validBuilderCapability(
			"cap.nil_builder",
			"records.nil_builder",
		))
	})
}

// TestBuildRejectsUnsupportedNestedOutputs proves representative unsupportable
// output graphs are reported during the consolidated build.
func TestBuildRejectsUnsupportedNestedOutputs(t *testing.T) {
	tests := []struct {
		// name identifies the unsupported nested output.
		name string

		// register attempts one invalid public registration.
		register func(*codemode.Builder)
	}{
		{
			name: "interface any",
			register: func(builder *codemode.Builder) {
				codemode.Register(builder, codemode.Capability[builderInput, interfaceBuilderOutput]{
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
			register: func(builder *codemode.Builder) {
				codemode.Register(builder, codemode.Capability[builderInput, rawMessageBuilderOutput]{
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
			register: func(builder *codemode.Builder) {
				codemode.Register(builder, codemode.Capability[builderInput, cyclicBuilderOutput]{
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
			register: func(builder *codemode.Builder) {
				codemode.Register(builder, codemode.Capability[builderInput, marshalerBuilderOutput]{
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
			register: func(builder *codemode.Builder) {
				codemode.Register(builder, codemode.Capability[builderInput, mapKeyBuilderOutput]{
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
			builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})

			tt.register(builder)
			codemode.Register(builder, validBuilderCapability("cap.retained", "records.retained"))
			server, err := builder.Build()

			require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
			assert.Nil(t, server)
		})
	}
}

// TestBuildJoinsDuplicateRegistrationFailures proves duplicate identities are deferred and complete.
func TestBuildJoinsDuplicateRegistrationFailures(t *testing.T) {
	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})
	codemode.Register(builder, validBuilderCapability("cap.one", "records.one"))
	codemode.Register(builder, validBuilderCapability("cap.one", "records.two"))
	codemode.Register(builder, validBuilderCapability("cap.two", "records.one"))

	server, err := builder.Build()

	require.ErrorIs(t, err, codemode.ErrInvalidRegistration)
	assert.Nil(t, server)
	require.ErrorContains(t, err, `capability "records.two"`)
	require.ErrorContains(t, err, `capability "records.one"`)
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
		codemode.Register(builder, validBuilderCapability("cap.lookup", "records.lookup"))
		codemode.Register(builder, validBuilderCapability("cap.detail", "records.lookup.detail"))

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
	codemode.Register(builder,
		validBuilderCapability(
			codemode.CapabilityID("cap."+strings.Repeat("x", 128)),
			"records.lookup",
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
