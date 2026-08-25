package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode/authz"
	"github.com/meigma/codemode/internal/binding"
	"github.com/meigma/codemode/internal/execution"
)

const (
	testAdvice = "call codemode.ServeWorkerAndExit() as the first statement of main and of TestMain in test binaries, before flag parsing or constructing credentials, clients, authorizers, handlers, or a Server"

	trivialSource = "def main():\n    return 1\n"

	nativeSource = "def main():\n    return records.lookup(value=\"alpha\")\n"
)

// testLimits returns positive budgets used by runner tests.
func testLimits() Limits {
	return Limits{
		MaxSourceBytes:          64 * 1024,
		MaxExecutionSteps:       1_000_000,
		MaxExecutionTime:        5 * time.Second,
		MaxNativeCalls:          100,
		MaxValueDepth:           32,
		MaxValueBytes:           1024 * 1024,
		MaxConcurrentExecutions: 2,
	}
}

// lookupBinding is one valid capability used by runner tests.
func lookupBinding() execution.CapabilityBinding {
	return execution.CapabilityBinding{
		ID:   "cap.lookup",
		Name: "records.lookup",
		Input: []binding.FieldShape{
			{Name: "value", Type: "str", Required: true},
		},
	}
}

// nopDispatch rejects every native call as an ordinary capability failure.
func nopDispatch() Dispatch {
	return func(context.Context, authz.Subject, string, map[string]any) (any, error) {
		return nil, execution.ErrCapabilityFailure
	}
}

// newTestRunner constructs a runner with the lookup capability.
func newTestRunner(t *testing.T, limits Limits, dispatch Dispatch) *Runner {
	t.Helper()
	runner, err := NewRunner([]execution.CapabilityBinding{lookupBinding()}, limits, dispatch)
	require.NoError(t, err)
	return runner
}

// TestProbeFailedTemplateComponents proves the generic probe message parts.
func TestProbeFailedTemplateComponents(t *testing.T) {
	err := probeFailedError(causeStdoutClosed, nil)

	require.Error(t, err)
	assert.Equal(t, probeFailedPrefix+causeStdoutClosed+"; "+testAdvice, err.Error())
}

// TestProbeResolutionAndStartPrefixes proves executable failures stay distinct.
func TestProbeResolutionAndStartPrefixes(t *testing.T) {
	assert.Equal(t, "CodeMode worker probe could not resolve the current executable: ", probeResolvePrefix)
	assert.Equal(t, "CodeMode worker probe could not start the current executable: ", probeStartPrefix)
	assert.Equal(t, "Build ran in CodeMode worker mode; "+testAdvice, errProbeWorkerMode.Error())
}

// TestProbeStableCauses proves every documented cause text.
func TestProbeStableCauses(t *testing.T) {
	tests := []struct {
		// name identifies the cause.
		name string

		// got is the produced cause text.
		got string

		// want is the expected cause text.
		want string
	}{
		{name: "stdout closed", got: causeStdoutClosed, want: "stdout closed before probe_ack"},
		{name: "non-protocol", got: causeNonProtocol, want: "stdout contained non-protocol data before probe_ack"},
		{name: "malformed ack", got: causeMalformedAck, want: "received malformed probe_ack"},
		{name: "data after ack", got: causeDataAfterAck, want: "received data after probe_ack"},
		{name: "deadline", got: causeDeadline, want: "deadline exceeded after 5s"},
		{
			name: "version",
			got:  causeVersionPrefix + "2" + causeVersionSuffix,
			want: "protocol version mismatch: child reported 2, parent requires 1",
		},
		{name: "exit status", got: fmt.Sprintf(causeExitStatusFmt, 7), want: "child exited with status 7"},
		{
			name: "unsuccessful",
			got:  causeUnsuccessful + "signal: killed",
			want: "child exited unsuccessfully: signal: killed",
		},
		{name: "probe io", got: causeProbeIOPrefix + "broken pipe", want: "probe I/O failed: broken pipe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

// TestProbeStderrEmptyOmitsSuffix proves empty capture adds no suffix.
func TestProbeStderrEmptyOmitsSuffix(t *testing.T) {
	assert.Empty(t, stderrSuffix(nil, false))
	assert.Empty(t, stderrSuffix([]byte("   \n\t"), false))
	err := probeFailedError(causeDeadline, &stderrCapture{})
	assert.Equal(t, probeFailedPrefix+causeDeadline+"; "+testAdvice, err.Error())
}

// TestProbeStderrQuotedOneLine proves whitespace trim and strconv quoting.
func TestProbeStderrQuotedOneLine(t *testing.T) {
	suffix := stderrSuffix([]byte("  boom\nline  "), false)
	quoted := strings.TrimPrefix(suffix, stderrSuffixPlain)

	assert.Equal(t, strconv.Quote("boom\nline"), quoted)
	assert.NotContains(t, quoted, "\n")
}

// TestProbeStderrInvalidUTF8Replaced proves invalid UTF-8 becomes U+FFFD.
func TestProbeStderrInvalidUTF8Replaced(t *testing.T) {
	raw := []byte("bad\xfferr")
	require.False(t, utf8.Valid(raw))

	suffix := stderrSuffix(raw, false)

	assert.Equal(t, stderrSuffixPlain+strconv.Quote("bad\uFFFDerr"), suffix)
}

// TestProbeStderrTruncatedLabel proves the over-4096-byte formatter suffix.
func TestProbeStderrTruncatedLabel(t *testing.T) {
	capture := &stderrCapture{}
	payload := bytes.Repeat([]byte("a"), stderrCap+64)
	n, err := capture.Write(payload)
	require.NoError(t, err)
	assert.Equal(t, len(payload), n)
	require.True(t, capture.truncated())
	require.Len(t, capture.bytes(), stderrCap)

	suffix := stderrSuffix(capture.bytes(), capture.truncated())
	quoted := strconv.Quote(strings.Repeat("a", stderrCap))
	assert.Equal(t, stderrSuffixTrunc+quoted, suffix)
	assert.Contains(t, probeFailedError(causeDeadline, capture).Error(), stderrSuffixTrunc)
}

// TestProbeSuccessDiscardsCapture proves success does not format stderr.
func TestProbeSuccessDiscardsCapture(t *testing.T) {
	capture := &stderrCapture{}
	_, _ = capture.Write([]byte("secret"))

	assert.Empty(t, classifyProbeFailure(nil, nil))
	assert.Empty(t, stderrSuffix(nil, false))
}

// TestProbeNonzeroExitBeatsEOF proves exit status wins incidental stdout close.
func TestProbeNonzeroExitBeatsEOF(t *testing.T) {
	waitErr := &exec.ExitError{ProcessState: fakeExitState(t, 3)}
	cause := classifyProbeFailure(waitErr, errors.New(causeStdoutClosed))

	assert.Equal(t, "child exited with status 3", cause)
}

// TestProbeDeadlineCauseIsStable proves the deadline text used before kill artifacts.
func TestProbeDeadlineCauseIsStable(t *testing.T) {
	capture := &stderrCapture{}
	_, _ = capture.Write([]byte("killed"))
	err := probeFailedError(causeDeadline, capture)

	assert.Equal(
		t,
		probeFailedPrefix+causeDeadline+"; "+testAdvice+stderrSuffixPlain+strconv.Quote("killed"),
		err.Error(),
	)
	assert.NotContains(t, err.Error(), causeStdoutClosed)
	assert.NotContains(t, err.Error(), "signal")
}

// TestProbeClassifyPayloadCauses proves handshake payload classification.
func TestProbeClassifyPayloadCauses(t *testing.T) {
	ack, err := encodeProbeAck()
	require.NoError(t, err)
	assert.Empty(t, classifyProbePayload(ack))

	assert.Equal(t, causeNonProtocol, classifyProbePayload([]byte("not-json")))
	assert.Equal(t, causeNonProtocol, classifyProbePayload([]byte{0xff, 0xfe}))

	probe, err := encodeProbe()
	require.NoError(t, err)
	assert.Equal(t, causeNonProtocol, classifyProbePayload(probe))

	malformed := []byte(`{"type":"probe_ack"}`)
	assert.Equal(t, causeMalformedAck, classifyProbePayload(malformed))

	version := []byte(`{"type":"probe_ack","version":2}`)
	assert.Equal(t, "protocol version mismatch: child reported 2, parent requires 1", classifyProbePayload(version))
}

// TestRunnerNewValidatesBudgetsAndManifest proves construction rejects invalid state.
func TestRunnerNewValidatesBudgetsAndManifest(t *testing.T) {
	valid := testLimits()
	dispatch := nopDispatch()
	tests := []struct {
		// name identifies the invalid construction.
		name string

		// bindings are presented to NewRunner.
		bindings []execution.CapabilityBinding

		// limits are presented to NewRunner.
		limits Limits

		// dispatch is presented to NewRunner.
		dispatch Dispatch
	}{
		{name: "nil dispatch", bindings: nil, limits: valid, dispatch: nil},
		{name: "zero time", bindings: nil, limits: withTime(valid, 0), dispatch: dispatch},
		{name: "zero concurrency", bindings: nil, limits: withConcurrency(valid, 0), dispatch: dispatch},
		{name: "zero source", bindings: nil, limits: withSource(valid, 0), dispatch: dispatch},
		{
			name:     "empty id",
			bindings: []execution.CapabilityBinding{withID(lookupBinding(), "")},
			limits:   valid,
			dispatch: dispatch,
		},
		{
			name:     "undotted name",
			bindings: []execution.CapabilityBinding{withName(lookupBinding(), "records")},
			limits:   valid,
			dispatch: dispatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRunner(tt.bindings, tt.limits, tt.dispatch)

			require.Error(t, err)
		})
	}
}

// TestRunnerNewCopiesManifest proves caller slices are not retained.
func TestRunnerNewCopiesManifest(t *testing.T) {
	capability := lookupBinding()
	runner, err := NewRunner([]execution.CapabilityBinding{capability}, testLimits(), nopDispatch())
	require.NoError(t, err)

	capability.ID = "mutated"
	capability.Input[0].Name = "mutated"
	require.Len(t, runner.manifest, 1)
	assert.Equal(t, "cap.lookup", runner.manifest[0].ID)
	assert.Equal(t, "value", runner.manifest[0].Input[0].Name)
}

// TestRunnerNilStateAndOversizeSource proves nil runner, nil context, and oversized source fail before spawn.
func TestRunnerNilStateAndOversizeSource(t *testing.T) {
	var runner *Runner
	_, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.ErrorIs(t, err, execution.ErrInternal)

	live := newTestRunner(t, testLimits(), nopDispatch())
	var nilCtx context.Context
	_, err = live.Execute(nilCtx, authz.Subject{ID: "s"}, trivialSource)
	require.ErrorIs(t, err, execution.ErrInternal)

	limits := testLimits()
	limits.MaxSourceBytes = 4
	live = newTestRunner(t, limits, nopDispatch())
	_, err = live.Execute(context.Background(), authz.Subject{ID: "s"}, "def main():\n    return 1\n")
	require.ErrorIs(t, err, execution.ErrResourceLimit)
}

// TestRunnerZeroChildArgs proves marker-only launch drops go-test flags.
func TestRunnerZeroChildArgs(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := workerCmd(executable, nil)
	assert.Equal(t, []string{executable}, cmd.Args)
	assert.Equal(t, []string{workerMarkerName + "=" + workerMarkerValue}, cmd.Env)
	assert.Empty(t, cmd.ExtraFiles)

	runner := newTestRunner(t, testLimits(), nopDispatch())
	require.NoError(t, runner.Probe())
}

// TestRunnerProbeSuccess proves a real same-binary handshake.
func TestRunnerProbeSuccess(t *testing.T) {
	runner := newTestRunner(t, testLimits(), nopDispatch())

	require.NoError(t, runner.Probe())
}

// TestRunnerFinalValue proves a trivial worker execution returns the exact result.
func TestRunnerFinalValue(t *testing.T) {
	runner := newTestRunner(t, testLimits(), nopDispatch())

	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)

	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// TestRunnerNativeForwarding proves parent dispatch receives the child call.
func TestRunnerNativeForwarding(t *testing.T) {
	var (
		gotID   string
		gotArgs map[string]any
		parent  = os.Getpid()
	)
	dispatch := func(_ context.Context, _ authz.Subject, id string, arguments map[string]any) (any, error) {
		gotID = id
		gotArgs = arguments
		return map[string]any{"pid": int64(os.Getpid())}, nil
	}
	runner := newTestRunner(t, testLimits(), dispatch)

	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, nativeSource)

	require.NoError(t, err)
	assert.Equal(t, "cap.lookup", gotID)
	assert.Equal(t, map[string]any{"value": "alpha"}, gotArgs)
	assert.Equal(t, map[string]any{"pid": int64(parent)}, got)
}

// TestRunnerRetainedCallbackError proves ordinary dispatch errors abort and win cleanup.
func TestRunnerRetainedCallbackError(t *testing.T) {
	retained := execution.ErrPermissionDenied
	runner := newTestRunner(t, testLimits(), func(context.Context, authz.Subject, string, map[string]any) (any, error) {
		return nil, retained
	})

	_, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, nativeSource)

	require.ErrorIs(t, err, retained)
	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// TestRunnerProtocolViolationKills proves ErrProtocol skips native_abort and returns ErrInternal.
func TestRunnerProtocolViolationKills(t *testing.T) {
	runner := newTestRunner(t, testLimits(), func(context.Context, authz.Subject, string, map[string]any) (any, error) {
		return nil, fmt.Errorf("unknown id: %w", ErrProtocol)
	})

	_, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, nativeSource)

	require.ErrorIs(t, err, execution.ErrInternal)
	require.NotErrorIs(t, err, ErrProtocol)
	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// TestRunnerFinalErrorTrailingByteKills proves trailing output after final_error is internal and killable.
func TestRunnerFinalErrorTrailingByteKills(t *testing.T) {
	runner := newTestRunner(t, testLimits(), nopDispatch())
	var parentWrites bytes.Buffer
	var childOut bytes.Buffer
	payload, err := encodeFinalError(finalErrorInvalidProgram)
	require.NoError(t, err)
	require.NoError(t, writeFrame(&childOut, payload, runner.readCap))
	require.NoError(t, childOut.WriteByte('x'))

	out := runner.runExecExchange(
		context.Background(),
		&parentWrites,
		bytes.NewReader(childOut.Bytes()),
		authz.Subject{ID: "s"},
		trivialSource,
	)

	require.ErrorIs(t, out.err, execution.ErrInternal)
	assert.True(t, out.kill)
	assert.NoError(t, out.retained)
}

// TestRunnerProtocolViolationWritesNoAbort proves ErrProtocol kills without writing native_abort.
func TestRunnerProtocolViolationWritesNoAbort(t *testing.T) {
	runner := newTestRunner(t, testLimits(), func(context.Context, authz.Subject, string, map[string]any) (any, error) {
		return nil, fmt.Errorf("unknown id: %w", ErrProtocol)
	})
	var parentWrites bytes.Buffer
	var childOut bytes.Buffer
	payload, err := encodeNativeCall("cap.lookup", map[string]any{"value": "alpha"})
	require.NoError(t, err)
	require.NoError(t, writeFrame(&childOut, payload, runner.readCap))

	out := runner.runExecExchange(
		context.Background(),
		&parentWrites,
		bytes.NewReader(childOut.Bytes()),
		authz.Subject{ID: "s"},
		trivialSource,
	)

	require.ErrorIs(t, out.err, execution.ErrInternal)
	assert.True(t, out.kill)
	require.NoError(t, out.retained)
	assert.NotContains(t, parentWrites.String(), `"type":"native_abort"`)
	execCap, err := execPayloadCap(runner.childLimits.MaxSourceBytes, runner.manifest)
	require.NoError(t, err)
	frames := readPrefixedFrames(t, parentWrites.Bytes(), execCap)
	require.Len(t, frames, 1)
	decoded, err := decodePayload(frames[0])
	require.NoError(t, err)
	_, ok := decoded.(execFrame)
	require.True(t, ok)
}

// readPrefixedFrames consumes every complete length-prefixed payload.
func readPrefixedFrames(t *testing.T, raw []byte, maxPayload uint32) [][]byte {
	t.Helper()
	reader := bytes.NewReader(raw)
	frames := make([][]byte, 0, 1)
	for reader.Len() > 0 {
		payload, err := readFrame(reader, maxPayload)
		require.NoError(t, err)
		frames = append(frames, payload)
	}
	return frames
}

// TestRunnerQueueCancellationSpawnsNoChild proves runCtx-before-queue cancellation.
func TestRunnerQueueCancellationSpawnsNoChild(t *testing.T) {
	limits := testLimits()
	limits.MaxConcurrentExecutions = 1
	started := make(chan struct{})
	release := make(chan struct{})
	var live atomic.Int32
	dispatch := func(ctx context.Context, _ authz.Subject, _ string, _ map[string]any) (any, error) {
		live.Add(1)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		live.Add(-1)
		return "ok", nil
	}
	runner := newTestRunner(t, limits, dispatch)

	firstDone := make(chan error, 1)
	go func() {
		_, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, nativeSource)
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("first execution did not start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runner.Execute(ctx, authz.Subject{ID: "s"}, trivialSource)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), live.Load())

	close(release)
	select {
	case err := <-firstDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("first execution did not finish")
	}
}

// TestRunnerPermitReuseAfterKill proves a later execution works after kill/reap.
func TestRunnerPermitReuseAfterKill(t *testing.T) {
	started := make(chan struct{})
	dispatch := func(ctx context.Context, _ authz.Subject, _ string, _ map[string]any) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	runner := newTestRunner(t, testLimits(), dispatch)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runner.Execute(ctx, authz.Subject{ID: "s"}, nativeSource)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("execution did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("canceled execution did not return")
	}

	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// TestRunnerSequentialSuccessAfterFailure proves one runner survives a failed child.
func TestRunnerSequentialSuccessAfterFailure(t *testing.T) {
	runner := newTestRunner(t, testLimits(), nopDispatch())

	_, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, "def main():\n    return unknown()\n")
	require.ErrorIs(t, err, execution.ErrInvalidProgram)

	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// TestRunnerMarkerGuard proves Execute refuses nested spawn in worker mode.
func TestRunnerMarkerGuard(t *testing.T) {
	if workerMode() {
		t.Skip("already in worker mode")
	}
	t.Setenv(workerMarkerName, workerMarkerValue)
	runner := newTestRunner(t, testLimits(), nopDispatch())

	err := runner.Probe()
	require.ErrorIs(t, err, errProbeWorkerMode)

	_, err = runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.ErrorIs(t, err, execution.ErrInternal)
}

// TestRunnerParallelCap exercises concurrent executions at the configured cap.
func TestRunnerParallelCap(t *testing.T) {
	limits := testLimits()
	limits.MaxConcurrentExecutions = 2
	var current, peak atomic.Int32
	dispatch := func(context.Context, authz.Subject, string, map[string]any) (any, error) {
		n := current.Add(1)
		for {
			prev := peak.Load()
			if n <= prev || peak.CompareAndSwap(prev, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		current.Add(-1)
		return "ok", nil
	}
	runner := newTestRunner(t, limits, dispatch)

	var wg sync.WaitGroup
	errCh := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			_, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, nativeSource)
			errCh <- err
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	assert.LessOrEqual(t, peak.Load(), int32(limits.MaxConcurrentExecutions))
	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// TestRunnerCancellationDuringDispatch proves cancellation is not joined to the callback.
func TestRunnerCancellationDuringDispatch(t *testing.T) {
	started := make(chan struct{})
	dispatch := func(ctx context.Context, _ authz.Subject, _ string, _ map[string]any) (any, error) {
		close(started)
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		return nil, ctx.Err()
	}
	runner := newTestRunner(t, testLimits(), dispatch)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runner.Execute(ctx, authz.Subject{ID: "s"}, nativeSource)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("dispatch did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("execute did not return after cancel")
	}
	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// TestRunnerRepeatedCleanup proves repeated kill/reap cycles release permits.
func TestRunnerRepeatedCleanup(t *testing.T) {
	runner := newTestRunner(
		t,
		testLimits(),
		func(ctx context.Context, _ authz.Subject, _ string, _ map[string]any) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)
	for range 5 {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := runner.Execute(ctx, authz.Subject{ID: "s"}, nativeSource)
			done <- err
		}()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(10 * time.Second):
			t.Fatal("canceled execution did not return")
		}
	}
	got, err := runner.Execute(context.Background(), authz.Subject{ID: "s"}, trivialSource)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

// withTime returns a copy of limits with MaxExecutionTime replaced.
func withTime(limits Limits, value time.Duration) Limits {
	limits.MaxExecutionTime = value
	return limits
}

// withConcurrency returns a copy of limits with MaxConcurrentExecutions replaced.
func withConcurrency(limits Limits, value int) Limits {
	limits.MaxConcurrentExecutions = value
	return limits
}

// withSource returns a copy of limits with MaxSourceBytes replaced.
func withSource(limits Limits, value int) Limits {
	limits.MaxSourceBytes = value
	return limits
}

// withID returns a copy of capability with ID replaced.
func withID(capability execution.CapabilityBinding, id string) execution.CapabilityBinding {
	capability.ID = id
	return capability
}

// withName returns a copy of capability with Name replaced.
func withName(capability execution.CapabilityBinding, name string) execution.CapabilityBinding {
	capability.Name = name
	return capability
}

// fakeExitState runs a helper process that exits with code.
func fakeExitState(t *testing.T, code int) *os.ProcessState {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestProbeHelperExit")
	cmd.Env = append(os.Environ(), "WORKER_TEST_EXIT="+strconv.Itoa(code))
	err := cmd.Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	return exitErr.ProcessState
}

// TestProbeHelperExit is a helper process used to build a real ExitError.
func TestProbeHelperExit(_ *testing.T) {
	raw := os.Getenv("WORKER_TEST_EXIT")
	if raw == "" {
		return
	}
	code, err := strconv.Atoi(raw)
	if err != nil {
		os.Exit(1)
	}
	os.Exit(code)
}
