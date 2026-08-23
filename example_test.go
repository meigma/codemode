package codemode_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
)

// Example_registerAndExecute registers one typed capability and prints main's final value.
//
// authz.AllowAll is deliberate in this sample. Production hosts normally supply
// an Authorizer that inspects the trusted subject and canonical arguments.
func Example_registerAndExecute() {
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

	builder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     codemode.DefaultLimits(),
	})
	err := codemode.Register(builder, codemode.Capability[lookupInput, lookupOutput]{
		ID:          "records.entry.lookup",
		Name:        "records.lookup",
		Summary:     "Look up one record by key.",
		Description: "Returns one deterministic record for the supplied key.",
		Handler: func(_ context.Context, _ authz.Subject, input lookupInput) (lookupOutput, error) {
			count := int64(0)
			if input.Limit != nil {
				count = *input.Limit
			}
			return lookupOutput{Key: input.Key, Count: count}, nil
		},
	})
	if err != nil {
		panic(err)
	}

	server, err := builder.Build()
	if err != nil {
		panic(err)
	}

	result, err := server.Execute(context.Background(), authz.Subject{ID: "example-user"}, `
print("discarded")
def main():
    return records.lookup(key="alpha", limit=2)
`)
	if err != nil {
		panic(err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))
	// Output:
	// {"count":2,"key":"alpha"}
}
