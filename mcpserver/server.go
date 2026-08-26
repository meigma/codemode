package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/execution"
)

// searchInput is the exact search_api tool argument object.
type searchInput struct {
	// Query is the bounded capability-search string.
	Query string `json:"query"`
}

// describeInput is the exact describe_api tool argument object.
type describeInput struct {
	// Name is the exact model-facing capability name to describe.
	Name string `json:"name"`
}

// executeInput is the exact execute tool argument object.
type executeInput struct {
	// Source is the bounded Starlark program text.
	Source string `json:"source"`
}

// executeOutput is the exact successful execute payload.
type executeOutput struct {
	// Result is main's final converted value.
	Result any `json:"result"`
}

// operationResult carries one recovered tool operation outcome without named returns.
type operationResult[Value any] struct {
	// value is the successful operation output.
	value Value

	// err is the safe operation failure.
	err error
}

// adapter binds a Service and InvocationResolver to the three official MCP tools.
type adapter struct {
	// service is the required CodeMode application port.
	service Service

	// resolver is the required trusted-subject port.
	resolver InvocationResolver
}

// New constructs an official MCP server that exposes exactly search_api, describe_api, and execute.
//
// New rejects a nil or typed-nil Service or InvocationResolver. The returned server has no
// generic downstream MCP forwarding path. Client request metadata is untrusted and ignored.
func New(service Service, resolver InvocationResolver) (*mcp.Server, error) {
	if isNil(service) {
		return nil, fmt.Errorf("%w: service is required", codemode.ErrInvalidRegistration)
	}
	if isNil(resolver) {
		return nil, fmt.Errorf("%w: invocation resolver is required", codemode.ErrInvalidRegistration)
	}

	searchOutputSchema, schemaErr := jsonschema.For[[]codemode.SearchResult](nil)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: search_api output schema: %w", codemode.ErrInvalidRegistration, schemaErr)
	}
	schemaErr = requireNonNullArray(searchOutputSchema)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: search_api output schema: %w", codemode.ErrInvalidRegistration, schemaErr)
	}
	schemaErr = requireResolvedSchema(searchOutputSchema)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: search_api output schema: %w", codemode.ErrInvalidRegistration, schemaErr)
	}

	describeOutputSchema, schemaErr := jsonschema.For[codemode.Description](nil)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: describe_api output schema: %w", codemode.ErrInvalidRegistration, schemaErr)
	}
	if describeOutputSchema == nil || describeOutputSchema.Properties == nil {
		return nil, fmt.Errorf("%w: describe_api output schema: missing properties", codemode.ErrInvalidRegistration)
	}
	schemaErr = requireNonNullArray(describeOutputSchema.Properties["input"])
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: describe_api output schema: input: %w", codemode.ErrInvalidRegistration, schemaErr)
	}
	schemaErr = requireNonNullArray(describeOutputSchema.Properties["output"])
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: describe_api output schema: output: %w", codemode.ErrInvalidRegistration, schemaErr)
	}
	schemaErr = requireResolvedSchema(describeOutputSchema)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: describe_api output schema: %w", codemode.ErrInvalidRegistration, schemaErr)
	}

	executeOutputSchema, schemaErr := jsonschema.For[executeOutput](nil)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: execute output schema: %w", codemode.ErrInvalidRegistration, schemaErr)
	}
	schemaErr = requireResolvedSchema(executeOutputSchema)
	if schemaErr != nil {
		return nil, fmt.Errorf("%w: execute output schema: %w", codemode.ErrInvalidRegistration, schemaErr)
	}

	bound := &adapter{service: service, resolver: resolver}
	server := mcp.NewServer(&mcp.Implementation{Name: "codemode", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:         "search_api",
		Description:  "Search enabled names and summaries with a short literal substring. Retry an empty result with a shorter term.",
		OutputSchema: searchOutputSchema,
	}, bound.search)
	mcp.AddTool(server, &mcp.Tool{
		Name:         "describe_api",
		Description:  "Describe one enabled capability by the exact name returned by search_api, without whitespace or case changes.",
		OutputSchema: describeOutputSchema,
	}, bound.describe)
	mcp.AddTool(server, &mcp.Tool{
		Name:         "execute",
		Description:  "Execute one Starlark program that defines def main(): with zero arguments, calls only names confirmed through search_api and describe_api inside main, and returns main's final result. Starlark is not Python: sum, import, while, and f-strings are unavailable; load is disabled; print is discarded.",
		OutputSchema: executeOutputSchema,
	}, bound.execute)
	return server, nil
}

// search resolves a trusted subject and then searches enabled capabilities.
func (bound *adapter) search(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input searchInput,
) (*mcp.CallToolResult, []codemode.SearchResult, error) {
	outcome := runToolOperation(func() ([]codemode.SearchResult, error) {
		if _, err := resolveSubject(ctx, bound.resolver); err != nil {
			return nil, err
		}
		return bound.service.Search(input.Query)
	})
	if outcome.err == nil {
		outcome.value = nonNilSlice(outcome.value)
	}
	return nil, outcome.value, outcome.err
}

// describe resolves a trusted subject and then describes one enabled capability.
func (bound *adapter) describe(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input describeInput,
) (*mcp.CallToolResult, codemode.Description, error) {
	outcome := runToolOperation(func() (codemode.Description, error) {
		if _, err := resolveSubject(ctx, bound.resolver); err != nil {
			return codemode.Description{}, err
		}
		return bound.service.Describe(codemode.CapabilityName(input.Name))
	})
	if outcome.err == nil {
		outcome.value.Input = nonNilSlice(outcome.value.Input)
		outcome.value.Output = nonNilSlice(outcome.value.Output)
	}
	return nil, outcome.value, outcome.err
}

// execute resolves a trusted subject and then runs one bounded program.
func (bound *adapter) execute(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input executeInput,
) (*mcp.CallToolResult, executeOutput, error) {
	outcome := runToolOperation(func() (executeOutput, error) {
		subject, err := resolveSubject(ctx, bound.resolver)
		if err != nil {
			return executeOutput{}, err
		}
		value, err := bound.service.Execute(ctx, subject, codemode.Program(input.Source))
		if err != nil {
			return executeOutput{}, err
		}
		return executeOutput{Result: value}, nil
	})
	return nil, outcome.value, outcome.err
}

// resolveSubject returns a non-empty trusted subject or a coarse unauthenticated error.
func resolveSubject(ctx context.Context, resolver InvocationResolver) (authz.Subject, error) {
	subject, err := resolver.Resolve(ctx)
	if err != nil {
		return authz.Subject{}, codemode.ErrUnauthenticated
	}
	if subject.ID == "" {
		return authz.Subject{}, codemode.ErrUnauthenticated
	}
	return subject, nil
}

// projectToolError removes wrapped service detail and maps failures to fixed client-visible sentinels.
// Approved SafeDetail for invalid program and invalid capability arguments is formatted as
// "<sentinel>: <detail>". Arbitrary custom-Service wrapper text remains hidden.
func projectToolError(err error) error {
	if detail, ok := execution.SafeDetail(err); ok {
		switch {
		case errors.Is(err, codemode.ErrInvalidProgram):
			return fmt.Errorf("%w: %s", codemode.ErrInvalidProgram, detail)
		case errors.Is(err, codemode.ErrInvalidArguments):
			return fmt.Errorf("%w: %s", codemode.ErrInvalidArguments, detail)
		}
	}
	switch {
	case errors.Is(err, codemode.ErrResourceLimit):
		return codemode.ErrResourceLimit
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, codemode.ErrUnauthenticated):
		return codemode.ErrUnauthenticated
	case errors.Is(err, codemode.ErrNotFound):
		return codemode.ErrNotFound
	case errors.Is(err, codemode.ErrInvalidProgram):
		return codemode.ErrInvalidProgram
	case errors.Is(err, codemode.ErrInvalidArguments):
		return codemode.ErrInvalidArguments
	case errors.Is(err, codemode.ErrPermissionDenied):
		return codemode.ErrPermissionDenied
	case errors.Is(err, codemode.ErrPolicyFailure):
		return codemode.ErrPolicyFailure
	case errors.Is(err, codemode.ErrCapabilityFailure):
		return codemode.ErrCapabilityFailure
	default:
		return codemode.ErrInternal
	}
}

// runToolOperation recovers boundary panics and projects failures to coarse tool errors.
func runToolOperation[Value any](operation func() (Value, error)) operationResult[Value] {
	var outcome operationResult[Value]
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				outcome.err = codemode.ErrInternal
			}
		}()
		outcome.value, outcome.err = operation()
	}()
	if outcome.err != nil {
		outcome.err = projectToolError(outcome.err)
	}
	return outcome
}

// isNil reports whether value is a nil interface or a typed-nil dependency.
func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	kind := reflected.Kind()
	nilable := kind == reflect.Chan ||
		kind == reflect.Func ||
		kind == reflect.Interface ||
		kind == reflect.Map ||
		kind == reflect.Pointer ||
		kind == reflect.Slice
	return nilable && reflected.IsNil()
}

// nonNilSlice returns values, or an empty slice when values is nil.
func nonNilSlice[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

// jsonSchemaArrayType is the JSON Schema name for an array.
const jsonSchemaArrayType = "array"

// requireNonNullArray narrows a nullable JSON Schema slice node to type array.
func requireNonNullArray(schema *jsonschema.Schema) error {
	if schema == nil {
		return errors.New("schema is required")
	}
	if schema.Type == jsonSchemaArrayType && schema.Types == nil {
		return nil
	}
	hasArray := false
	hasNull := false
	for _, typ := range schema.Types {
		switch typ {
		case jsonSchemaArrayType:
			hasArray = true
		case "null":
			hasNull = true
		default:
			return fmt.Errorf("schema type includes %q", typ)
		}
	}
	if schema.Type != "" || !hasArray || !hasNull {
		return errors.New("schema must be a nullable array")
	}
	schema.Type = jsonSchemaArrayType
	schema.Types = nil
	return nil
}

// requireResolvedSchema reports whether schema can be used as a tool output schema.
func requireResolvedSchema(schema *jsonschema.Schema) error {
	if schema == nil {
		return errors.New("inferred schema is nil")
	}
	_, err := schema.Resolve(nil)
	return err
}
