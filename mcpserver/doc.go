// Package mcpserver exposes CodeMode as exactly three official MCP tools.
//
// The adapter resolves a trusted subject from host-owned Go context before every
// operation, ignores untrusted client metadata, and projects failures to stable
// coarse tool text. It does not proxy arbitrary downstream MCP tools.
package mcpserver
