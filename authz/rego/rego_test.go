package rego_test

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/authz/rego"
)

const (
	// allowDecision is the ground data reference used by the adapter tests.
	allowDecision = "data.codemode.authz.allow"

	// authorizationModule is the deterministic filename for test policies.
	authorizationModule = "authorization.rego"
)

// TestNewRejectsInvalidConstruction proves constructor validation fails closed.
func TestNewRejectsInvalidConstruction(t *testing.T) {
	validModules := map[string]string{authorizationModule: totalAllowPolicy()}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		// name identifies the invalid constructor case.
		name string
		// ctx is the constructor context.
		ctx context.Context
		// decision is the requested Rego decision path.
		decision string
		// modules are the in-memory policy modules.
		modules map[string]string
		// want is a required constructor error, when classification matters.
		want error
		// contains is a required error substring for ordinary constructor failures.
		contains string
	}{
		{
			name:     "nil context",
			decision: allowDecision,
			modules:  validModules,
			contains: "context is required",
		},
		{
			name:     "cancelled context",
			ctx:      cancelled,
			decision: allowDecision,
			modules:  validModules,
			want:     context.Canceled,
		},
		{
			name:     "no modules",
			ctx:      t.Context(),
			decision: allowDecision,
			contains: "at least one module is required",
		},
		{
			name:     "blank filename",
			ctx:      t.Context(),
			decision: allowDecision,
			modules:  map[string]string{"   ": totalAllowPolicy()},
			contains: "module filename must not be blank",
		},
		{
			name:     "unparseable decision",
			ctx:      t.Context(),
			decision: "data.codemode.authz.",
			modules:  validModules,
			contains: "decision",
		},
		{
			name:     "non-ground decision",
			ctx:      t.Context(),
			decision: "data.codemode.authz[x]",
			modules:  validModules,
			contains: "must be a ground data reference",
		},
		{
			name:     "input-rooted decision",
			ctx:      t.Context(),
			decision: "input.allow",
			modules:  validModules,
			contains: "must be a ground data reference",
		},
		{
			name:     "invalid module syntax",
			ctx:      t.Context(),
			decision: allowDecision,
			modules:  map[string]string{authorizationModule: "package not valid"},
			contains: "prepare decision",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer, err := rego.New(tt.ctx, tt.decision, tt.modules)
			require.Error(t, err, "expected constructor validation to fail")
			assert.Nil(t, authorizer, "expected no authorizer after a constructor failure")
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want, "expected the original context error")
				return
			}
			assert.Contains(t, err.Error(), tt.contains, "expected a constructor diagnostic")
		})
	}
}

// TestNewPreparesAValidPolicy proves a total Boolean policy is ready after construction.
func TestNewPreparesAValidPolicy(t *testing.T) {
	authorizer := mustNew(t, totalAllowPolicy())

	require.NoError(
		t,
		authorizer.Authorize(t.Context(), matchingInput()),
		"expected a prepared total policy to allow a matching input",
	)
}

// TestAuthorizeProjectsTrustedInput proves the Rego document maps only the trusted fields.
func TestAuthorizeProjectsTrustedInput(t *testing.T) {
	authorizer := mustNew(t, `
package codemode.authz

default allow := false

allow if {
	input.subject.id == "subject-1"
	input.capability.id == "records.entry.lookup"
	input.capability.name == "records.lookup"
	input.arguments.key == "alpha"
	input.arguments.limit == 42
}
`)

	require.NoError(
		t,
		authorizer.Authorize(t.Context(), matchingInput()),
		"expected the exact trusted projection to allow",
	)

	denied := matchingInput()
	denied.CapabilityID = "records.lookup"
	denied.CapabilityName = "records.entry.lookup"
	require.ErrorIs(
		t,
		authorizer.Authorize(t.Context(), denied),
		authz.ErrDenied,
		"expected capability id and name to remain distinct",
	)
}

// TestAuthorizeTreatsAbsentOptionalArgumentsAsMissing proves omitted arguments are not present as null.
func TestAuthorizeTreatsAbsentOptionalArgumentsAsMissing(t *testing.T) {
	authorizer := mustNew(t, `
package codemode.authz

default allow := false

allow if {
	input.subject.id == "subject-1"
	input.capability.id == "records.entry.lookup"
	not input.arguments.limit
}
`)

	input := matchingInput()
	input.Arguments = map[string]any{"key": "alpha"}
	require.NoError(t, authorizer.Authorize(t.Context(), input), "expected an omitted optional argument to be absent")
}

// TestAuthorizeDecodesDirectBooleanResults proves true, false, undefined, and non-Boolean shapes.
func TestAuthorizeDecodesDirectBooleanResults(t *testing.T) {
	tests := []struct {
		// name identifies the decision shape.
		name string
		// module is the in-memory policy source.
		module string
		// input is the authorization request.
		input authz.AuthorizationInput
		// want is the expected classified error, if any.
		want error
		// policyFailure reports whether the error must not be a denial.
		policyFailure bool
		// message is the exact policy-failure diagnostic.
		message string
	}{
		{
			name:   "true allows",
			module: totalAllowPolicy(),
			input:  matchingInput(),
		},
		{
			name:   "false denies",
			module: totalAllowPolicy(),
			input:  deniedInput(),
			want:   authz.ErrDenied,
		},
		{
			name:          "undefined is a policy failure",
			module:        undefinedAllowPolicy(),
			input:         matchingInput(),
			policyFailure: true,
			message:       "rego: decision is undefined",
		},
		{
			name:          "non-boolean is a policy failure",
			module:        `package codemode.authz` + "\n\nallow := \"yes\"\n",
			input:         matchingInput(),
			policyFailure: true,
			message:       "rego: decision must be boolean",
		},
		{
			name:          "set result is a policy failure",
			module:        `package codemode.authz` + "\n\nallow contains x if x := {true, false}[_]\n",
			input:         matchingInput(),
			policyFailure: true,
			message:       "rego: decision must be boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := mustNew(t, tt.module)
			err := authorizer.Authorize(t.Context(), tt.input)
			if tt.want == nil && !tt.policyFailure {
				require.NoError(t, err, "expected the decision to allow")
				return
			}
			require.Error(t, err, "expected the decision to fail closed")
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want, "expected the classified denial")
				return
			}
			require.NotErrorIs(t, err, authz.ErrDenied, "expected a broken decision to remain a policy failure")
			assert.Equal(t, tt.message, err.Error(), "expected the distinct decision diagnostic")
		})
	}
}

// TestAuthorizeTreatsFatalBuiltinErrorsAsPolicyFailures proves a failing builtin cannot fall through to allow.
func TestAuthorizeTreatsFatalBuiltinErrorsAsPolicyFailures(t *testing.T) {
	authorizer := mustNew(t, `
package codemode.authz

default allow := false

allow if to_number("not-a-number") == 1

allow if true
`)

	err := authorizer.Authorize(t.Context(), matchingInput())
	require.Error(t, err, "expected a fatal builtin error")
	require.NotErrorIs(t, err, authz.ErrDenied, "expected a builtin error to remain a policy failure")
}

// TestAuthorizeRejectsNilReceiverAndContext proves nil values fail closed without panicking.
func TestAuthorizeRejectsNilReceiverAndContext(t *testing.T) {
	var authorizer *rego.Authorizer
	err := authorizer.Authorize(t.Context(), matchingInput())
	require.Error(t, err, "expected a nil authorizer to fail closed")
	require.NotErrorIs(t, err, authz.ErrDenied, "expected a nil authorizer to remain a policy failure")

	prepared := mustNew(t, totalAllowPolicy())
	var nilContext context.Context
	err = prepared.Authorize(nilContext, matchingInput())
	require.Error(t, err, "expected a nil context to fail closed")
	require.NotErrorIs(t, err, authz.ErrDenied, "expected a nil context to remain a policy failure")
}

// TestAuthorizePreservesContextErrors proves cancelled evaluation returns the original context error.
func TestAuthorizePreservesContextErrors(t *testing.T) {
	authorizer := mustNew(t, expensiveAllowPolicy())
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(
		t,
		authorizer.Authorize(cancelled, matchingInput()),
		context.Canceled,
		"expected a pre-cancelled context to be preserved",
	)

	deadline, cancelDeadline := context.WithTimeout(t.Context(), 5*time.Millisecond)
	t.Cleanup(cancelDeadline)
	err := authorizer.Authorize(deadline, matchingInput())
	require.ErrorIs(t, err, context.DeadlineExceeded, "expected a cancelled evaluation to preserve the context error")
	require.NotErrorIs(t, err, authz.ErrDenied, "expected cancellation not to become a denial")
}

// TestAuthorizeLeavesCanonicalArgumentsUnchanged proves evaluation borrows arguments read-only.
func TestAuthorizeLeavesCanonicalArgumentsUnchanged(t *testing.T) {
	authorizer := mustNew(t, totalAllowPolicy())
	input := matchingInput()
	original := maps.Clone(input.Arguments)

	require.NoError(t, authorizer.Authorize(t.Context(), input), "expected the matching input to allow")
	require.Equal(t, original, input.Arguments, "expected the canonical argument map to remain unchanged")
	require.IsType(t, int64(0), input.Arguments["limit"], "expected the int64 argument type to remain exact")
	assert.Equal(t, int64(42), input.Arguments["limit"], "expected the int64 argument value to remain exact")
}

// TestNewRejectsNondeterministicBuiltins proves every pinned nondeterministic builtin fails preparation.
func TestNewRejectsNondeterministicBuiltins(t *testing.T) {
	tests := []struct {
		// name is the removed builtin.
		name string
		// expression is a representative call of that builtin.
		expression string
	}{
		{name: "http.send", expression: `http.send({"method": "GET", "url": "https://example.invalid"})`},
		{name: "io.jwt.decode_verify", expression: `io.jwt.decode_verify("token", {})`},
		{name: "io.jwt.encode_sign", expression: `io.jwt.encode_sign({}, {}, {})`},
		{name: "io.jwt.encode_sign_raw", expression: `io.jwt.encode_sign_raw("{}", "{}", "{}")`},
		{name: "net.lookup_ip_addr", expression: `net.lookup_ip_addr("example.invalid")`},
		{name: "opa.runtime", expression: `opa.runtime()`},
		{name: "rand.intn", expression: `rand.intn("seed", 2)`},
		{name: "time.now_ns", expression: `time.now_ns()`},
		{name: "uuid.rfc4122", expression: `uuid.rfc4122("seed")`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := fmt.Sprintf("package codemode.authz\n\ndefault allow := false\n\nallow if %s\n", tt.expression)
			authorizer, err := rego.New(t.Context(), allowDecision, map[string]string{authorizationModule: module})
			require.Error(t, err, "expected preparation to reject %s", tt.name)
			assert.Nil(t, authorizer, "expected no authorizer when a nondeterministic builtin is used")
			assert.Contains(t, err.Error(), "prepare decision", "expected a preparation failure")
		})
	}
}

// TestNewRejectsMetadataWithExternalRef proves metadata containing an external $ref is rejected.
func TestNewRejectsMetadataWithExternalRef(t *testing.T) {
	module := `
package codemode.authz

# METADATA
# schemas:
# - input: {$ref: "https://example.invalid/schema.json"}
allow if {
	input == 42
}
`
	authorizer, err := rego.New(t.Context(), allowDecision, map[string]string{authorizationModule: module})
	require.Error(t, err, "expected an external schema $ref to fail preparation")
	assert.Nil(t, authorizer, "expected no authorizer when remote reference loading is disabled")
	assert.Contains(
		t,
		err.Error(),
		"remote reference loading disabled",
		"expected an external $ref to be rejected because remote reference loading is disabled",
	)
}

// TestNewAcceptsUnconfiguredSchemaAnnotation proves schema["https://example.invalid/schema.json"] is accepted and ignored.
func TestNewAcceptsUnconfiguredSchemaAnnotation(t *testing.T) {
	authorizer := mustNew(t, `
package codemode.authz

# METADATA
# schemas:
# - input: schema["https://example.invalid/schema.json"]
default allow := false

allow if {
	input.subject.id == "subject-1"
	input.capability.id == "records.entry.lookup"
	input.arguments.key == "alpha"
	input.arguments.limit == 42
}
`)

	require.NoError(
		t,
		authorizer.Authorize(t.Context(), matchingInput()),
		"expected an unconfigured schema annotation to be ignored",
	)
}

// TestAuthorizeErasesDisabledPrintCalls proves print statements do not prevent a Boolean decision.
func TestAuthorizeErasesDisabledPrintCalls(t *testing.T) {
	authorizer := mustNew(t, `
package codemode.authz

default allow := false

allow if {
	print("must not execute")
	input.subject.id == "subject-1"
}
`)

	require.NoError(
		t,
		authorizer.Authorize(t.Context(), matchingInput()),
		"expected a disabled print call to be erased",
	)
}

// TestAuthorizerAllowsConcurrentUse proves one prepared query can evaluate mixed inputs concurrently.
func TestAuthorizerAllowsConcurrentUse(t *testing.T) {
	authorizer := mustNew(t, totalAllowPolicy())
	const workers = 16
	var started sync.WaitGroup
	var finished sync.WaitGroup
	start := make(chan struct{})
	started.Add(workers)
	finished.Add(workers)

	for index := range workers {
		go func(index int) {
			defer finished.Done()
			started.Done()
			<-start
			input := matchingInput()
			if index%2 == 1 {
				input = deniedInput()
			}
			err := authorizer.Authorize(t.Context(), input)
			if index%2 == 1 {
				assert.ErrorIs(t, err, authz.ErrDenied, "expected a denied concurrent input")
				return
			}
			assert.NoError(t, err, "expected an allowed concurrent input")
		}(index)
	}

	started.Wait()
	close(start)
	finished.Wait()
}

// Example constructs the adapter and assigns it to CodeMode options.
func Example() {
	authorizer, err := rego.New(
		context.Background(),
		"data.codemode.authz.allow",
		map[string]string{
			"authorization.rego": `package codemode.authz

default allow := false

allow if {
	input.subject.id == "example-user"
	input.capability.id == "records.entry.lookup"
	input.arguments.key != "forbidden"
}
`,
		},
	)
	if err != nil {
		panic(err)
	}

	_ = codemode.Options{
		Authorizer: authorizer,
		Limits:     codemode.DefaultLimits(),
	}
	fmt.Println("prepared")

	// Output: prepared
}

// mustNew constructs an authorizer from one module or fails the test.
func mustNew(t *testing.T, module string) *rego.Authorizer {
	t.Helper()
	authorizer, err := rego.New(t.Context(), allowDecision, map[string]string{authorizationModule: module})
	require.NoError(t, err, "expected a valid policy to prepare")
	require.NotNil(t, authorizer, "expected a prepared authorizer")
	return authorizer
}

// matchingInput returns the trusted input that the sample policy allows.
func matchingInput() authz.AuthorizationInput {
	return authz.AuthorizationInput{
		Subject:        authz.Subject{ID: "subject-1"},
		CapabilityID:   "records.entry.lookup",
		CapabilityName: "records.lookup",
		Arguments: map[string]any{
			"key":   "alpha",
			"limit": int64(42),
		},
	}
}

// deniedInput returns a trusted input that the sample policy denies.
func deniedInput() authz.AuthorizationInput {
	input := matchingInput()
	input.Subject.ID = "other-subject"
	return input
}

// totalAllowPolicy returns a default-deny Boolean decision over the trusted input.
func totalAllowPolicy() string {
	return `
package codemode.authz

default allow := false

allow if {
	input.subject.id == "subject-1"
	input.capability.id == "records.entry.lookup"
	input.arguments.key == "alpha"
	input.arguments.limit == 42
}
`
}

// undefinedAllowPolicy returns a partial decision with no default.
func undefinedAllowPolicy() string {
	return `
package codemode.authz

allow if input.subject.id == "nobody"
`
}

// expensiveAllowPolicy returns a deterministic policy that takes measurable evaluation time.
func expensiveAllowPolicy() string {
	return `
package codemode.authz

default allow := false

allow if {
	count(numbers.range(1, 200000)) == 200000
	input.subject.id == "subject-1"
}
`
}
