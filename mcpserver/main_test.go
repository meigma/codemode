package mcpserver_test

import (
	"os"
	"testing"

	"github.com/meigma/codemode"
)

// TestMain serves worker mode before running MCP package tests.
func TestMain(m *testing.M) {
	codemode.ServeWorkerAndExit()
	os.Exit(m.Run())
}
