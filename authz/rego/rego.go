package rego

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	oparego "github.com/open-policy-agent/opa/v1/rego"

	"github.com/meigma/codemode/authz"
)

// Authorizer evaluates one prepared in-process Rego authorization decision.
type Authorizer struct {
	// prepared is the immutable compiled query shared across concurrent Authorize calls.
	prepared oparego.PreparedEvalQuery
}

// policyInput is the exact trusted document projected into Rego.
type policyInput struct {
	// Subject is the trusted authenticated identity.
	Subject policySubject `json:"subject"`

	// Capability identifies the validated native call.
	Capability policyCapability `json:"capability"`

	// Arguments is the borrowed canonical argument map.
	Arguments map[string]any `json:"arguments"`
}

// policySubject is the trusted identity projection.
type policySubject struct {
	// ID is the trusted non-secret subject identity.
	ID string `json:"id"`
}

// policyCapability is the validated capability projection.
type policyCapability struct {
	// ID is the stable policy identity.
	ID string `json:"id"`

	// Name is the model-facing dotted capability name.
	Name string `json:"name"`
}

// New validates a ground data decision, compiles the supplied in-memory Rego
// modules, and returns an authorizer ready for concurrent use.
func New(ctx context.Context, decision string, modules map[string]string) (*Authorizer, error) {
	if ctx == nil {
		return nil, errors.New("rego: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(modules) == 0 {
		return nil, errors.New("rego: at least one module is required")
	}

	ref, err := ast.ParseRef(decision)
	if err != nil {
		return nil, fmt.Errorf("rego: decision %q: %w", decision, err)
	}
	if !ref.IsGround() || !ref.HasPrefix(ast.DefaultRootRef) {
		return nil, fmt.Errorf("rego: decision %q must be a ground data reference", decision)
	}

	filenames := make([]string, 0, len(modules))
	for filename := range modules {
		if strings.TrimSpace(filename) == "" {
			return nil, errors.New("rego: module filename must not be blank")
		}
		filenames = append(filenames, filename)
	}
	slices.Sort(filenames)

	capabilities := restrictedCapabilities()
	options := []func(*oparego.Rego){
		oparego.Query(ref.String()),
		oparego.SetRegoVersion(ast.RegoV1),
		oparego.Capabilities(capabilities),
		oparego.StrictBuiltinErrors(true),
		oparego.EnablePrintStatements(false),
	}
	options = slices.Grow(options, len(filenames))
	for _, filename := range filenames {
		parsed, parseErr := ast.ParseModuleWithOpts(filename, modules[filename], ast.ParserOptions{
			ProcessAnnotation: true,
			RegoVersion:       ast.RegoV1,
			Capabilities:      capabilities,
		})
		if parseErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, fmt.Errorf("rego: prepare decision: %w", parseErr)
		}
		options = append(options, oparego.ParsedModule(parsed))
	}

	prepared, err := oparego.New(options...).PrepareForEval(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("rego: prepare decision: %w", err)
	}
	return &Authorizer{prepared: prepared}, nil
}

// Authorize implements authz.Authorizer.
func (authorizer *Authorizer) Authorize(ctx context.Context, input authz.AuthorizationInput) error {
	if authorizer == nil {
		return errors.New("rego: authorizer is required")
	}
	if ctx == nil {
		return errors.New("rego: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	results, err := authorizer.prepared.Eval(ctx, oparego.EvalInput(policyInput{
		Subject: policySubject{ID: string(input.Subject.ID)},
		Capability: policyCapability{
			ID:   input.CapabilityID,
			Name: input.CapabilityName,
		},
		Arguments: input.Arguments,
	}))
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return fmt.Errorf("rego: evaluate decision: %w", err)
	}

	if len(results) == 0 {
		return errors.New("rego: decision is undefined")
	}

	allowed, ok := oparego.ResultValue[bool](results)
	if !ok {
		return errors.New("rego: decision must be boolean")
	}
	if !allowed {
		return authz.ErrDenied
	}
	return nil
}

// restrictedCapabilities returns Rego-v1 capabilities without nondeterministic
// builtins and with no allowed network hosts.
func restrictedCapabilities() *ast.Capabilities {
	capabilities := ast.CapabilitiesForThisVersion(ast.CapabilitiesRegoVersion(ast.RegoV1))
	filtered := capabilities.Builtins[:0]
	for _, builtin := range capabilities.Builtins {
		if builtin.IsNondeterministic() {
			continue
		}
		filtered = append(filtered, builtin)
	}
	capabilities.Builtins = filtered
	capabilities.AllowNet = []string{}
	return capabilities
}
