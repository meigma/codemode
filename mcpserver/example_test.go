package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/mcpserver"
)

// Example_officialTransport connects official MCP sessions and calls execute.
//
// This in-memory sample has one process-owned identity, matching a single-user
// stdio host. Multi-user hosts must resolve each request from authenticated,
// host-owned context instead. authz.AllowAll is deliberate in this sample;
// production hosts normally supply policy.
func Example_officialTransport() {
	// lookupInput is the records.lookup argument contract.
	type lookupInput struct {
		// Key is the required record identifier.
		Key string `json:"key"`

		// Limit is the optional result bound.
		Limit *int64 `json:"limit,omitempty"`
	}

	// lookupOutput is the records.lookup handler result.
	type lookupOutput struct {
		// Key is the looked-up record identifier.
		Key string `json:"key"`

		// Count is the resolved optional limit, or zero when omitted.
		Count int64 `json:"count"`
	}

	builder := codemode.New(codemode.Options{Authorizer: authz.AllowAll()})
	codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
		Name:    "records.lookup",
		Summary: "Look up one record by key.",
		Handler: func(_ context.Context, _ authz.Subject, input lookupInput) (lookupOutput, error) {
			count := int64(0)
			if input.Limit != nil {
				count = *input.Limit
			}
			return lookupOutput{Key: input.Key, Count: count}, nil
		},
	})

	root, err := builder.Build()
	if err != nil {
		panic(err)
	}

	mcpServer, err := mcpserver.New(root, mcpserver.StaticSubject(authz.Subject{ID: "example-user"}))
	if err != nil {
		panic(err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		panic(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "example-client", Version: "1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		panic(err)
	}

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "execute",
		Arguments: map[string]any{
			"source": `
def main():
    return records.lookup(key="alpha", limit=2)
`,
		},
	})
	if err != nil {
		panic(err)
	}
	if result.IsError {
		panic(fmt.Sprint(result.Content))
	}

	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))

	if err := clientSession.Close(); err != nil {
		panic(err)
	}
	if err := serverSession.Close(); err != nil {
		panic(err)
	}
	// Output:
	// {"result":{"count":2,"key":"alpha"}}
}
