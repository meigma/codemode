package codemode

import (
	"os"

	"github.com/meigma/codemode/internal/worker"
)

// IsWorker reports whether the current process was re-executed as a CodeMode worker.
//
// Most hosts should call ServeWorkerAndExit instead. A host that uses IsWorker
// directly must still serve worker mode before flag parsing or constructing
// credentials, clients, authorizers, handlers, or a Server, and must not fall
// through into ordinary host wiring.
func IsWorker() bool {
	return worker.IsWorker()
}

// ServeWorkerAndExit serves one CodeMode probe or execution request and
// terminates the process when the current process is a CodeMode worker. It
// returns immediately in an ordinary host process.
//
// Call ServeWorkerAndExit as the first statement of main, and of TestMain in
// every test binary that calls Builder.Build. The call must precede flag
// parsing and construction of credentials, service clients, authorizers,
// handlers, a Server, or a transport.
//
// In worker mode, ServeWorkerAndExit exits with status 0 after a successful
// exchange and status 1 after an internal worker or protocol failure. It does
// not return an error and writes no diagnostic. Standard output is reserved
// for protocol frames. In worker mode this function calls [os.Exit], so deferred
// functions do not run.
func ServeWorkerAndExit() {
	if !worker.IsWorker() {
		return
	}
	os.Exit(worker.Serve())
}
