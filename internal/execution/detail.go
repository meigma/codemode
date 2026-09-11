package execution

import "errors"

// MaxAgentErrorBytes is the maximum UTF-8 size of an approved agent-visible
// capability-failure suffix, including a trailing ASCII ellipsis when truncated.
const MaxAgentErrorBytes = 256

// safeDetailError attaches one model-derived diagnostic suffix without changing the coarse error text.
type safeDetailError struct {
	// cause is the coarse classified sentinel.
	cause error

	// detail is the model-derived suffix excluded from Error.
	detail string
}

// Error returns only the coarse cause text.
func (err *safeDetailError) Error() string {
	return err.cause.Error()
}

// Unwrap returns the coarse cause.
func (err *safeDetailError) Unwrap() error {
	return err.cause
}

// WithSafeDetail attaches detail to cause without changing cause.Error.
//
// Empty detail returns cause unchanged. Callers must pass only model-derived
// suffixes or sanitized explicitly handler-authored detail; host-derived text
// must not be attached.
func WithSafeDetail(cause error, detail string) error {
	if detail == "" {
		return cause
	}
	return &safeDetailError{cause: cause, detail: detail}
}

// SafeDetail reports the approved suffix attached to err, if any.
//
// Extraction follows the error chain with [errors.As].
func SafeDetail(err error) (string, bool) {
	var wrapped *safeDetailError
	if !errors.As(err, &wrapped) {
		return "", false
	}
	return wrapped.detail, true
}
