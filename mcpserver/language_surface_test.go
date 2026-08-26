package mcpserver_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/internal/universe"
)

const (
	// languageSurfaceFence opens the exact documented name block.
	languageSurfaceFence = "```language-surface"

	// languageSurfaceClose ends the exact documented name block.
	languageSurfaceClose = "```"
)

// TestLanguageSurfaceReferenceMatchesUniverse proves the MCP reference
// language-surface block matches the canonical universe name queries exactly.
func TestLanguageSurfaceReferenceMatchesUniverse(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "expected the test file path")
	docPath := filepath.Join(filepath.Dir(thisFile), "..", "docs", "docs", "reference", "mcp-tools.md")
	raw, err := os.ReadFile(docPath)
	require.NoError(t, err, "expected to read the MCP tool reference")

	block, found := extractLanguageSurfaceBlock(string(raw))
	require.True(t, found, "expected a language-surface fence in mcp-tools.md")
	assert.Equal(t, expectedLanguageSurfaceBlock(), block, "documented language surface must match internal/universe")
}

// expectedLanguageSurfaceBlock formats the exact documented name lists.
func expectedLanguageSurfaceBlock() string {
	return strings.Join([]string{
		"top-level: " + strings.Join(universe.TopLevelNames(), ", "),
		"json: " + strings.Join(universe.JSONMemberNames(), ", "),
		"math: " + strings.Join(universe.MathMemberNames(), ", "),
	}, "\n")
}

// extractLanguageSurfaceBlock returns the fenced language-surface body.
func extractLanguageSurfaceBlock(markdown string) (string, bool) {
	start := strings.Index(markdown, languageSurfaceFence)
	if start < 0 {
		return "", false
	}
	bodyStart := start + len(languageSurfaceFence)
	if bodyStart < len(markdown) && markdown[bodyStart] == '\n' {
		bodyStart++
	}
	end := strings.Index(markdown[bodyStart:], languageSurfaceClose)
	if end < 0 {
		return "", false
	}
	return strings.TrimSuffix(markdown[bodyStart:bodyStart+end], "\n"), true
}
