package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/execution"
)

const (
	// workerMarkerName is the private environment key that selects worker mode.
	workerMarkerName = "CODEMODE_WORKER"

	// workerMarkerValue is the only accepted private marker value.
	workerMarkerValue = "1"

	// probeTimeout is the fixed probe exchange deadline.
	probeTimeout = 5 * time.Second

	// stderrCap is the number of probe-stderr bytes retained for diagnostics.
	stderrCap = 4096

	// probeAdvice is the host-wiring clause appended to generic probe failures.
	probeAdvice = "call codemode.ServeWorkerAndExit() as the first statement of main and of TestMain in test binaries, before flag parsing or constructing credentials, clients, authorizers, handlers, or a Server"
)

const (
	// causeStdoutClosed is the zero-exit missing-ack probe cause.
	causeStdoutClosed = "stdout closed before probe_ack"

	// causeNonProtocol is the non-frame stdout probe cause.
	causeNonProtocol = "stdout contained non-protocol data before probe_ack"

	// causeMalformedAck is the invalid-acknowledgement probe cause.
	causeMalformedAck = "received malformed probe_ack"

	// causeDataAfterAck is the trailing-bytes probe cause.
	causeDataAfterAck = "received data after probe_ack"

	// causeDeadline is the fixed five-second probe timeout cause.
	causeDeadline = "deadline exceeded after 5s"

	// causeVersionPrefix begins a protocol-version mismatch cause.
	causeVersionPrefix = "protocol version mismatch: child reported "

	// causeVersionSuffix ends a protocol-version mismatch cause.
	causeVersionSuffix = ", parent requires 1"

	// causeExitStatusFmt is the nonzero-exit probe cause.
	causeExitStatusFmt = "child exited with status %d"

	// causeUnsuccessful prefixes a non-code unsuccessful process state.
	causeUnsuccessful = "child exited unsuccessfully: "

	// causeProbeIOPrefix prefixes a pipe or exchange I/O failure.
	causeProbeIOPrefix = "probe I/O failed: "

	// probeFailedPrefix begins the generic probe-failure template.
	probeFailedPrefix = "CodeMode worker probe failed: "

	// probeResolvePrefix begins an executable-resolution failure.
	probeResolvePrefix = "CodeMode worker probe could not resolve the current executable: "

	// probeStartPrefix begins an executable-start failure.
	probeStartPrefix = "CodeMode worker probe could not start the current executable: "

	// stderrSuffixPlain prefixes a non-truncated quoted stderr excerpt.
	stderrSuffixPlain = "; worker stderr: "

	// stderrSuffixTrunc prefixes a truncated quoted stderr excerpt.
	stderrSuffixTrunc = "; worker stderr (truncated to 4096 bytes): "
)

// Limits is the parent-owned immutable worker budget set.
type Limits struct {
	// MaxSourceBytes bounds submitted source length.
	MaxSourceBytes int

	// MaxExecutionSteps bounds interpreter work.
	MaxExecutionSteps uint64

	// MaxExecutionTime bounds elapsed execution time including queue wait.
	MaxExecutionTime time.Duration

	// MaxNativeCalls bounds attempted native invocations.
	MaxNativeCalls uint64

	// MaxValueDepth bounds recursive converted-value nesting.
	MaxValueDepth int

	// MaxValueBytes bounds type-preserving encoded value size.
	MaxValueBytes int

	// MaxIntermediateValueBytes bounds cumulative successful native-result
	// value-body bytes for one Execute call. It is independent of MaxValueBytes
	// and is never projected to the child.
	MaxIntermediateValueBytes int

	// MaxConcurrentExecutions bounds live worker children.
	MaxConcurrentExecutions int
}

// Dispatch invokes one authoritative parent capability operation.
//
// remainingIntermediateBytes is the unused native-result value-body budget for
// this execution. handleNative passes min(MaxValueBytes, remainingIntermediate);
// the dispatcher uses that as the ConvertOutput node and materialization limit.
// Exact encoded-byte debit stays in writeNativeResult.
type Dispatch func(context.Context, authz.Subject, string, map[string]any, int) (any, error)

// ErrProtocol lets the authoritative root dispatcher report an impossible child ID or re-bind mismatch.
var ErrProtocol = errors.New("worker protocol violation")

// errProbeWorkerMode rejects recursive server construction from a worker child.
//
//nolint:staticcheck // ST1005: Build names the exported Builder.Build method in the mandated diagnostic.
var errProbeWorkerMode = errors.New(
	"Build ran in CodeMode worker mode; call codemode.ServeWorkerAndExit() as the first statement of main and of TestMain in test binaries, before flag parsing or constructing credentials, clients, authorizers, handlers, or a Server",
)

// Runner owns immutable manifest, budgets, caps, dispatch, and live-child permits.
type Runner struct {
	// manifest is the deep-copied child capability advertisement.
	manifest []manifestEntry

	// limits is the parent-owned budget set.
	limits Limits

	// childLimits are the child-visible budgets excluding elapsed time and concurrency.
	childLimits childLimits

	// writeCap is the largest legal parent-originated execution payload.
	writeCap uint32

	// readCap is the largest legal child-originated execution payload.
	readCap uint32

	// dispatch is the captured parent capability callback.
	dispatch Dispatch

	// sem bounds live children by MaxConcurrentExecutions.
	sem chan struct{}
}

// callbackResult is one buffered native-dispatch completion.
type callbackResult struct {
	// value is the successful native result.
	value any

	// err is the accepted callback error, if any.
	err error
}

// execOutcome is the exchange goroutine's terminal report.
type execOutcome struct {
	// value is the successful final value.
	value any

	// err is the exchange error to return after cleanup.
	err error

	// retained is a parent-owned callback error that wins cleanup errors.
	retained error

	// kill requests process kill before Wait.
	kill bool
}

// childSession owns one spawned worker and its cleanup.
type childSession struct {
	// cmd is the running worker command.
	cmd *exec.Cmd

	// stdin is the parent write end of the protocol pipe.
	stdin io.WriteCloser

	// stdout is the parent read end of the protocol pipe.
	stdout io.ReadCloser

	// waitOnce guarantees exactly one Wait.
	waitOnce sync.Once

	// waitErr is the retained Wait result.
	waitErr error
}

// stderrCapture consumes all writes and retains the first stderrCap bytes.
type stderrCapture struct {
	// buf holds the retained prefix.
	buf []byte

	// overflow reports whether more than stderrCap bytes arrived.
	overflow bool
}

// NewRunner constructs an immutable runner after validating budgets, manifest, and frame caps.
func NewRunner(bindings []execution.CapabilityBinding, limits Limits, dispatch Dispatch) (*Runner, error) {
	if dispatch == nil {
		return nil, errors.New("nil dispatch")
	}
	if limits.MaxExecutionTime <= 0 || limits.MaxConcurrentExecutions <= 0 || limits.MaxIntermediateValueBytes <= 0 {
		return nil, errInvalidLimits
	}
	child := childLimits{
		MaxSourceBytes:    limits.MaxSourceBytes,
		MaxExecutionSteps: limits.MaxExecutionSteps,
		MaxNativeCalls:    limits.MaxNativeCalls,
		MaxValueDepth:     limits.MaxValueDepth,
		MaxValueBytes:     limits.MaxValueBytes,
	}
	if err := validateChildLimits(child); err != nil {
		return nil, err
	}
	manifest := copyManifest(bindings)
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	if _, err := execPayloadCap(child.MaxSourceBytes, manifest); err != nil {
		return nil, err
	}
	writeCap, err := parentPayloadCap(child.MaxValueBytes)
	if err != nil {
		return nil, err
	}
	readCap, err := childPayloadCap(child.MaxValueBytes, manifest)
	if err != nil {
		return nil, err
	}
	return &Runner{
		manifest:    manifest,
		limits:      limits,
		childLimits: child,
		writeCap:    writeCap,
		readCap:     readCap,
		dispatch:    dispatch,
		sem:         make(chan struct{}, limits.MaxConcurrentExecutions),
	}, nil
}

// Probe runs one five-second same-executable handshake and returns a worker-level error.
func (r *Runner) Probe() error {
	if r == nil {
		return errors.New(probeFailedPrefix + causeProbeIOPrefix + "nil runner" + "; " + probeAdvice)
	}
	if workerMode() {
		return errProbeWorkerMode
	}
	executable, err := os.Executable()
	if err != nil {
		return errors.New(probeResolvePrefix + err.Error())
	}
	capture := &stderrCapture{}
	cmd := workerCmd(executable, capture)
	stdin, stdout, err := startWorker(cmd)
	if err != nil {
		return errors.New(probeStartPrefix + err.Error())
	}
	session := &childSession{cmd: cmd, stdin: stdin, stdout: stdout}
	runCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runProbeExchange(stdin, stdout)
	}()
	var (
		exchangeErr error
		deadline    bool
	)
	select {
	case exchangeErr = <-done:
	case <-runCtx.Done():
		deadline = true
	}
	waitErr := session.finish(deadline || exchangeErr != nil, func() {
		if deadline {
			<-done
		}
	})
	if deadline {
		return probeFailedError(causeDeadline, capture)
	}
	cause := classifyProbeFailure(waitErr, exchangeErr)
	if cause == "" {
		return nil
	}
	return probeFailedError(cause, capture)
}

// Execute runs source in a fresh worker and returns the decoded final value.
func (r *Runner) Execute(ctx context.Context, subject authz.Subject, source string) (any, error) {
	if r == nil || ctx == nil || r.dispatch == nil {
		return nil, execution.ErrInternal
	}
	if len(source) > r.limits.MaxSourceBytes {
		return nil, execution.ErrResourceLimit
	}
	runCtx, cancel := context.WithTimeout(ctx, r.limits.MaxExecutionTime)
	defer cancel()
	select {
	case r.sem <- struct{}{}:
	case <-runCtx.Done():
		return nil, classifyRunContext(runCtx)
	}
	held := true
	release := func() {
		if held {
			<-r.sem
			held = false
		}
	}
	if workerMode() {
		release()
		return nil, execution.ErrInternal
	}
	select {
	case <-runCtx.Done():
		release()
		return nil, classifyRunContext(runCtx)
	default:
	}
	executable, err := os.Executable()
	if err != nil {
		release()
		return nil, execution.ErrInternal
	}
	cmd := workerCmd(executable, io.Discard)
	stdin, stdout, err := startWorker(cmd)
	if err != nil {
		release()
		return nil, execution.ErrInternal
	}
	session := &childSession{cmd: cmd, stdin: stdin, stdout: stdout}
	done := make(chan execOutcome, 1)
	go func() {
		done <- r.runExecExchange(runCtx, stdin, stdout, subject, source)
	}()
	var out execOutcome
	selectedDone := false
	select {
	case out = <-done:
		selectedDone = true
	case <-runCtx.Done():
		out = execOutcome{err: classifyRunContext(runCtx), kill: true}
	}
	waitErr := session.finish(out.kill || !selectedDone, func() {
		if !selectedDone {
			<-done
		}
	})
	release()
	return interpretExec(runCtx, out, waitErr)
}

// copyManifest deep-copies ID, name, and input shapes into child manifest entries.
func copyManifest(bindings []execution.CapabilityBinding) []manifestEntry {
	entries := make([]manifestEntry, len(bindings))
	for index, capability := range bindings {
		input := make([]binding.FieldShape, len(capability.Input))
		copy(input, capability.Input)
		entries[index] = manifestEntry{
			ID:    capability.ID,
			Name:  capability.Name,
			Input: input,
		}
	}
	return entries
}

// workerMode reports whether the private worker marker is set.
func workerMode() bool {
	return os.Getenv(workerMarkerName) == workerMarkerValue
}

// workerCmd builds the marker-only same-executable command with zero arguments.
func workerCmd(executable string, stderr io.Writer) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), executable)
	cmd.Args = []string{executable}
	cmd.Env = []string{workerMarkerName + "=" + workerMarkerValue}
	cmd.Stderr = stderr
	return cmd
}

// startWorker attaches anonymous stdin/stdout pipes and starts cmd.
func startWorker(cmd *exec.Cmd) (io.WriteCloser, io.ReadCloser, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, err
	}
	return stdin, stdout, nil
}

// Write consumes p and retains bytes up to stderrCap.
func (c *stderrCapture) Write(p []byte) (int, error) {
	if c == nil {
		return len(p), nil
	}
	if !c.overflow {
		room := stderrCap - len(c.buf)
		if len(p) > room {
			if room > 0 {
				c.buf = append(c.buf, p[:room]...)
			}
			c.overflow = true
		} else {
			c.buf = append(c.buf, p...)
		}
	}
	return len(p), nil
}

// bytes returns the retained stderr prefix.
func (c *stderrCapture) bytes() []byte {
	if c == nil {
		return nil
	}
	return c.buf
}

// truncated reports whether the capture discarded bytes beyond stderrCap.
func (c *stderrCapture) truncated() bool {
	return c != nil && c.overflow
}

// finish closes pipes, optionally kills, joins exchange, and waits once.
func (s *childSession) finish(kill bool, join func()) error {
	if s == nil {
		return nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
		s.stdin = nil
	}
	if kill && s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.stdout != nil {
		_ = s.stdout.Close()
		s.stdout = nil
	}
	if join != nil {
		join()
	}
	return s.wait()
}

// wait reaps the child exactly once.
func (s *childSession) wait() error {
	s.waitOnce.Do(func() {
		if s.cmd != nil {
			s.waitErr = s.cmd.Wait()
		}
	})
	return s.waitErr
}

// runProbeExchange writes probe, requires probe_ack, and requires protocol EOF.
func runProbeExchange(stdin io.Writer, stdout io.Reader) error {
	conn := newParentProbeConn(stdin, stdout)
	if err := conn.writeProbe(); err != nil {
		return fmt.Errorf("%s%w", causeProbeIOPrefix, err)
	}
	payload, err := readFrame(stdout, probeCap)
	if err != nil {
		return classifyProbeRead(err)
	}
	if cause := classifyProbePayload(payload); cause != "" {
		return errors.New(cause)
	}
	return readProtocolEOF(stdout)
}

// classifyProbeRead maps a probe stdout read failure onto a stable cause error.
func classifyProbeRead(err error) error {
	switch {
	case errors.Is(err, errTruncatedFrame), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New(causeStdoutClosed)
	case errors.Is(err, errEmptyPayload),
		errors.Is(err, errFrameTooLarge),
		errors.Is(err, errInvalidUTF8),
		errors.Is(err, errUnknownType):
		return errors.New(causeNonProtocol)
	default:
		return fmt.Errorf("%s%w", causeProbeIOPrefix, err)
	}
}

// classifyProbePayload maps one complete probe payload onto a stable cause.
func classifyProbePayload(payload []byte) string {
	if !utf8.Valid(payload) {
		return causeNonProtocol
	}
	preview, err := decodeType(payload)
	if err != nil {
		return causeNonProtocol
	}
	if preview != frameTypeProbeAck {
		return causeNonProtocol
	}
	var raw rawVersionFrame
	if err := decodeStrict(payload, &raw); err != nil {
		return causeMalformedAck
	}
	if raw.Version == nil {
		return causeMalformedAck
	}
	if *raw.Version != protocolVersion {
		return causeVersionPrefix + strconv.Itoa(*raw.Version) + causeVersionSuffix
	}
	return ""
}

// readProtocolEOF requires stdout to close without trailing bytes.
func readProtocolEOF(r io.Reader) error {
	var extra [1]byte
	n, err := r.Read(extra[:])
	if n > 0 {
		return errors.New(causeDataAfterAck)
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s%w", causeProbeIOPrefix, err)
	}
	return errors.New(causeDataAfterAck)
}

// classifyProbeFailure applies wait/exchange precedence after a completed probe.
func classifyProbeFailure(waitErr error, exchangeErr error) string {
	exchangeCause := probeExchangeCause(exchangeErr)
	if exchangeCause == causeStdoutClosed || exchangeCause == "" {
		if cause, ok := childExitCause(waitErr); ok {
			return cause
		}
		if exchangeCause == causeStdoutClosed {
			return causeStdoutClosed
		}
		if waitErr != nil {
			return causeUnsuccessful + waitErr.Error()
		}
		return ""
	}
	if exchangeCause == causeDataAfterAck {
		if cause, ok := childExitCause(waitErr); ok {
			return cause
		}
		return causeDataAfterAck
	}
	return exchangeCause
}

// probeExchangeCause extracts the stable cause text from a probe exchange error.
func probeExchangeCause(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	switch {
	case text == causeStdoutClosed,
		text == causeNonProtocol,
		text == causeMalformedAck,
		text == causeDataAfterAck,
		text == causeDeadline:
		return text
	case strings.HasPrefix(text, causeVersionPrefix):
		return text
	case strings.HasPrefix(text, causeProbeIOPrefix):
		return text
	default:
		return causeProbeIOPrefix + text
	}
}

// childExitCause reports a nonzero or unsuccessful process state.
func childExitCause(waitErr error) (string, bool) {
	if waitErr == nil {
		return "", false
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return fmt.Sprintf(causeExitStatusFmt, code), true
		}
		state := waitErr.Error()
		if exitErr.ProcessState != nil {
			state = exitErr.ProcessState.String()
		}
		return causeUnsuccessful + state, true
	}
	return causeUnsuccessful + waitErr.Error(), true
}

// probeFailedError builds the generic probe-failure message with an optional stderr suffix.
func probeFailedError(cause string, capture *stderrCapture) error {
	msg := probeFailedPrefix + cause + "; " + probeAdvice
	if capture != nil {
		msg += stderrSuffix(capture.bytes(), capture.truncated())
	}
	return errors.New(msg)
}

// stderrSuffix formats the quoted one-line probe-stderr excerpt.
func stderrSuffix(raw []byte, truncated bool) string {
	excerpt := strings.TrimSpace(string(toValidUTF8(raw)))
	if excerpt == "" {
		return ""
	}
	quoted := strconv.Quote(excerpt)
	if truncated {
		return stderrSuffixTrunc + quoted
	}
	return stderrSuffixPlain + quoted
}

// toValidUTF8 replaces invalid UTF-8 byte sequences with the replacement character.
func toValidUTF8(raw []byte) []byte {
	if utf8.Valid(raw) {
		return raw
	}
	return bytes.ToValidUTF8(raw, []byte("\uFFFD"))
}

// classifyRunContext projects cancellation and deadlines onto execution errors.
func classifyRunContext(runCtx context.Context) error {
	switch err := runCtx.Err(); {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", execution.ErrResourceLimit, context.DeadlineExceeded)
	default:
		return execution.ErrInternal
	}
}

// interpretExec applies context, retained-callback, and wait precedence.
func interpretExec(runCtx context.Context, out execOutcome, waitErr error) (any, error) {
	if out.retained != nil {
		return nil, out.retained
	}
	if runCtx.Err() != nil && (out.kill || out.err != nil || waitErr != nil) {
		return nil, classifyRunContext(runCtx)
	}
	if out.err != nil {
		return nil, out.err
	}
	if waitErr != nil {
		return nil, execution.ErrInternal
	}
	return out.value, nil
}

// runExecExchange owns all execution frame I/O for one child.
func (r *Runner) runExecExchange(
	runCtx context.Context,
	stdin io.Writer,
	stdout io.Reader,
	subject authz.Subject,
	source string,
) execOutcome {
	conn := newParentExecConn(stdin, stdout, r.writeCap, r.readCap, r.limits.MaxIntermediateValueBytes)
	frame := execFrame{
		Type:     frameTypeExec,
		Version:  protocolVersion,
		Source:   source,
		Limits:   r.childLimits,
		Manifest: r.manifest,
	}
	if err := conn.writeExec(frame); err != nil {
		return execOutcome{err: execution.ErrInternal, kill: true}
	}
	for {
		msg, err := conn.read()
		if err != nil {
			return execOutcome{err: execution.ErrInternal, kill: true}
		}
		switch typed := msg.(type) {
		case nativeCallFrame:
			out := r.handleNative(runCtx, conn, subject, typed)
			if out.err != nil || out.retained != nil || out.kill {
				return out
			}
		case finalFrame:
			if err := readExecEOF(stdout); err != nil {
				return execOutcome{err: execution.ErrInternal, kill: true}
			}
			return execOutcome{value: typed.Result}
		case finalErrorFrame:
			if err := readExecEOF(stdout); err != nil {
				return execOutcome{err: execution.ErrInternal, kill: true}
			}
			return execOutcome{err: mapFinalError(typed.Code)}
		default:
			return execOutcome{err: execution.ErrInternal, kill: true}
		}
	}
}

// handleNative dispatches one child native_call against runCtx.
func (r *Runner) handleNative(
	runCtx context.Context,
	conn *parentConn,
	subject authz.Subject,
	frame nativeCallFrame,
) execOutcome {
	results := make(chan callbackResult, 1)
	go func() {
		value, err := r.dispatch(
			runCtx,
			subject,
			frame.CapabilityID,
			frame.Arguments,
			min(r.limits.MaxValueBytes, conn.remainingIntermediate),
		)
		results <- callbackResult{value: value, err: err}
	}()
	select {
	case <-runCtx.Done():
		return execOutcome{err: classifyRunContext(runCtx), kill: true}
	case result := <-results:
		if errors.Is(result.err, ErrProtocol) {
			return execOutcome{err: execution.ErrInternal, kill: true}
		}
		if result.err != nil {
			return writeAbort(conn, result.err)
		}
		if err := conn.writeNativeResult(result.value); err != nil {
			if errors.Is(err, errInvalidValue) || errors.Is(err, errFrameTooLarge) {
				return writeAbort(conn, execution.ErrResourceLimit)
			}
			return execOutcome{err: execution.ErrInternal, kill: true}
		}
		return execOutcome{}
	}
}

// writeAbort writes native_abort, requires protocol EOF, and retains parentErr.
func writeAbort(conn *parentConn, parentErr error) execOutcome {
	_ = conn.writeNativeAbort()
	_ = readExecEOF(conn.r)
	return execOutcome{retained: parentErr}
}

// readExecEOF requires a terminal execution stream to close without trailing bytes.
func readExecEOF(r io.Reader) error {
	var extra [1]byte
	n, err := r.Read(extra[:])
	if n > 0 {
		return errIllegalState
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errIllegalState
}

// mapFinalError projects a child-owned terminal code onto an execution sentinel.
func mapFinalError(code finalErrorCode) error {
	switch code {
	case finalErrorInvalidProgram:
		return execution.ErrInvalidProgram
	case finalErrorInvalidArguments:
		return execution.ErrInvalidArguments
	case finalErrorResourceLimit:
		return execution.ErrResourceLimit
	case finalErrorInternal:
		return execution.ErrInternal
	default:
		return execution.ErrInternal
	}
}
