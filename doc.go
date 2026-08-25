// Package codemode builds immutable catalogs of typed Go capabilities and executes
// bounded, authorized Starlark programs against them.
//
// # Host wiring
//
// A final binary must enter worker mode before flag parsing or ordinary host
// setup:
//
//	func main() {
//		codemode.ServeWorkerAndExit()
//
//		// Parse flags and construct credentials, clients, authorizers, handlers,
//		// the CodeMode Server, and the host transport here.
//	}
//
// A test binary that calls [Builder.Build] must do the same:
//
//	func TestMain(m *testing.M) {
//		codemode.ServeWorkerAndExit()
//		os.Exit(m.Run())
//	}
//
// [ServeWorkerAndExit] must be the first statement of main and TestMain. A
// library that embeds CodeMode cannot satisfy this requirement for an
// application it does not own; it must tell downstream users to install the
// call in their final binary and in every test binary that calls
// [Builder.Build].
package codemode
