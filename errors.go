package codemode

import (
	"errors"
	"unicode"
	"unicode/utf8"

	"github.com/meigma/codemode/internal/execution"
)

var (
	// ErrInvalidRegistration classifies invalid capability registration, limits, or server construction.
	ErrInvalidRegistration = errors.New("invalid registration")

	// ErrUnauthenticated classifies failure to resolve a trusted invocation subject.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrNotFound classifies an unavailable or disabled capability.
	ErrNotFound = errors.New("capability not found")

	// ErrInvalidProgram classifies invalid Starlark source or entrypoint behavior.
	ErrInvalidProgram = errors.New("invalid program")

	// ErrInvalidArguments classifies capability arguments rejected before authorization.
	ErrInvalidArguments = errors.New("invalid capability arguments")

	// ErrPermissionDenied classifies a recognized authorization denial.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrPolicyFailure classifies an authorization evaluation failure.
	ErrPolicyFailure = errors.New("authorization policy failure")

	// ErrResourceLimit classifies a configured execution or conversion limit.
	ErrResourceLimit = errors.New("resource limit exceeded")

	// ErrCapabilityFailure classifies a native capability handler failure.
	ErrCapabilityFailure = errors.New("capability failed")

	// ErrInternal classifies an unexpected framework failure.
	ErrInternal = errors.New("internal failure")
)

// AgentError carries a message the handler author has chosen to expose to the agent.
//
// Return it directly or wrap it using %w. Only Message is
// exposed, never surrounding error text. CodeMode replaces non-printable runes
// with spaces and truncates the message to 256 UTF-8 bytes, including a trailing
// "..." when truncated. Empty messages leave the capability failure bare.
// The host must not put secrets or other sensitive data in Message.
type AgentError struct {
	// Message is the handler-authored, agent-facing failure explanation.
	Message string
}

// Error returns the handler-authored message before sanitization.
func (err *AgentError) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

// sanitizeAgentMessage bounds work and output independently of handler message size.
func sanitizeAgentMessage(message string) string {
	var buffer [execution.MaxAgentErrorBytes]byte
	output := buffer[:0]
	for _, char := range message {
		if !unicode.IsPrint(char) {
			char = ' '
		}
		if len(output)+utf8.RuneLen(char) > len(buffer) {
			end := len(buffer) - len("...")
			for end < len(output) && !utf8.RuneStart(output[end]) {
				end--
			}
			return string(append(output[:end], "..."...))
		}
		output = utf8.AppendRune(output, char)
	}
	return string(output)
}
