package authz_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/authz"
)

// TestAllowAllRequiresExplicitConstruction proves the unrestricted policy is explicit and always permits.
func TestAllowAllRequiresExplicitConstruction(t *testing.T) {
	var authorizer authz.Authorizer = authz.AllowAll()
	input := authz.AuthorizationInput{
		Subject:        authz.Subject{ID: "subject-1"},
		CapabilityID:   "records.lookup",
		CapabilityName: "records.lookup",
		Arguments:      map[string]any{"id": "alpha"},
	}

	require.NoError(t, authorizer.Authorize(t.Context(), input))
}

// TestDeniedClassificationKeepsTrustedDetailWrapped proves callers can classify denials without string matching.
func TestDeniedClassificationKeepsTrustedDetailWrapped(t *testing.T) {
	trustedErr := fmt.Errorf("account suspended: %w", authz.ErrDenied)

	require.ErrorIs(t, trustedErr, authz.ErrDenied)
	assert.NotEqual(t, authz.ErrDenied.Error(), trustedErr.Error())
}

// TestAuthorizerRemainsContextAware proves the port accepts request cancellation state.
func TestAuthorizerRemainsContextAware(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	authorizer := contextAuthorizer{}

	require.ErrorIs(t, authorizer.Authorize(ctx, authz.AuthorizationInput{}), context.Canceled)
}

// contextAuthorizer returns the request context error for contract testing.
type contextAuthorizer struct{}

// Authorize returns the current request context error.
func (contextAuthorizer) Authorize(ctx context.Context, _ authz.AuthorizationInput) error {
	return ctx.Err()
}
