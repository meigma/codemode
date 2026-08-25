package worker

import (
	"os"
	"testing"
)

// TestMain installs the production worker branch so this test binary can be
// re-executed with no test arguments.
func TestMain(m *testing.M) {
	if IsWorker() {
		os.Exit(Serve())
	}
	os.Exit(m.Run())
}
