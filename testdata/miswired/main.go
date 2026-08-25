package main

import (
	"fmt"
	"os"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
)

// main intentionally omits ServeWorkerAndExit to exercise Build's startup diagnostic.
func main() {
	builder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     codemode.DefaultLimits(),
	})
	if _, err := builder.Build(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
