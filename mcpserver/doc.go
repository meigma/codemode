// Package mcpserver exposes CodeMode as exactly three official MCP tools.
//
// The adapter resolves a trusted subject through the required
// InvocationResolver before every operation, ignores untrusted client metadata,
// and projects failures to stable coarse tool text. StaticSubject supports
// single-user process-owned identity; multi-user hosts must resolve each
// authenticated request separately. The adapter does not proxy arbitrary
// downstream MCP tools.
package mcpserver
