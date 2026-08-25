package codemode_test

import (
	"context"
	"errors"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/codemode"
	"github.com/meigma/codemode/authz"
)

const busyWorkerProgram = `
def main():
    total = 0
    for item in range(1000000000):
        total += item
    return total
`

// TestServerExecuteWorkerHandlerRemainsInParent proves native handlers do not enter the child.
func TestServerExecuteWorkerHandlerRemainsInParent(t *testing.T) {
	capability := validBuilderCapability("cap.pid", "records.pid")
	capability.Handler = func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
		return builderOutput{Value: strconv.Itoa(os.Getpid())}, nil
	}
	server := buildTestServer(t, authz.AllowAll(), codemode.DefaultLimits(), capability)

	result, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.pid(value="ignored")
`)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": strconv.Itoa(os.Getpid())}, result)
	assert.False(t, codemode.IsWorker())
}

// TestServerExecuteWorkerNumberPreservation proves exact wire numeric kinds and values survive re-exec.
func TestServerExecuteWorkerNumberPreservation(t *testing.T) {
	server := buildWorkerOnlyServer(t, codemode.DefaultLimits())
	tests := []struct {
		// name identifies the numeric contract.
		name string

		// source returns one exact Starlark number.
		source codemode.Program

		// check validates the decoded Go value.
		check func(*testing.T, any)
	}{
		{
			name:   "max int64",
			source: `def main(): return 9223372036854775807`,
			check: func(t *testing.T, value any) {
				assert.Equal(t, int64(math.MaxInt64), value)
			},
		},
		{
			name:   "integral float",
			source: `def main(): return 1.0`,
			check: func(t *testing.T, value any) {
				floatValue, ok := value.(float64)
				require.True(t, ok)
				assert.InDelta(t, float64(1), floatValue, 0)
			},
		},
		{
			name:   "negative zero",
			source: `def main(): return -0.0`,
			check: func(t *testing.T, value any) {
				floatValue, ok := value.(float64)
				require.True(t, ok)
				assert.True(t, math.Signbit(floatValue))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, tt.source)

			require.NoError(t, err)
			tt.check(t, value)
		})
	}
}

// TestServerExecuteWorkerIssue12Deadline proves the worker deadline hard-preempts monolithic list built-ins.
func TestServerExecuteWorkerIssue12Deadline(t *testing.T) {
	limits := codemode.DefaultLimits()
	limits.MaxExecutionTime = 20 * time.Millisecond
	limits.MaxExecutionSteps = math.MaxUint64
	server := buildWorkerOnlyServer(t, limits)
	tests := []struct {
		// name identifies the monolithic built-in case.
		name string

		// source is the Starlark program for that case.
		source codemode.Program
	}{
		{name: "list", source: `def main(): return list(range(8000000))`},
		{name: "extend", source: `
def main():
    values = []
    values.extend(range(8000000))
    return values
`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := time.Now()
			value, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, tt.source)

			assert.Nil(t, value)
			require.ErrorIs(t, err, codemode.ErrResourceLimit)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Less(t, time.Since(started), 2*time.Second)
		})
	}
}

// TestServerExecuteWorkerCancellation kills and reaps a running interpreter child.
func TestServerExecuteWorkerCancellation(t *testing.T) {
	limits := codemode.DefaultLimits()
	limits.MaxExecutionSteps = math.MaxUint64
	server := buildWorkerOnlyServer(t, limits)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := server.Execute(ctx, authz.Subject{ID: "subject-1"}, busyWorkerProgram)
		result <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	require.ErrorIs(t, <-result, context.Canceled)
	value, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `def main(): return "reaped"`)
	require.NoError(t, err)
	assert.Equal(t, "reaped", value)
}

// TestServerExecuteWorkerSemaphore proves a canceled queued call never reaches parent dispatch.
func TestServerExecuteWorkerSemaphore(t *testing.T) {
	limits := codemode.DefaultLimits()
	limits.MaxConcurrentExecutions = 1
	limits.MaxExecutionTime = 2 * time.Second
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var handlerCalls atomic.Int64
	capability := validBuilderCapability("cap.queue", "records.queue")
	capability.Handler = func(context.Context, authz.Subject, builderInput) (builderOutput, error) {
		handlerCalls.Add(1)
		close(handlerStarted)
		<-releaseHandler
		return builderOutput{Value: "first"}, nil
	}
	server := buildTestServer(t, authz.AllowAll(), limits, capability)
	first := make(chan error, 1)
	go func() {
		_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.queue(value="first")
`)
		first <- err
	}()
	<-handlerStarted

	queuedCtx, cancelQueued := context.WithCancel(t.Context())
	queued := make(chan error, 1)
	go func() {
		_, err := server.Execute(queuedCtx, authz.Subject{ID: "subject-1"}, `
def main():
    return records.queue(value="queued")
`)
		queued <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancelQueued()

	require.ErrorIs(t, <-queued, context.Canceled)
	assert.Equal(t, int64(1), handlerCalls.Load())
	close(releaseHandler)
	require.NoError(t, <-first)
}

// TestServerExecuteWorkerAbort preserves a parent failure and releases the next execution.
func TestServerExecuteWorkerAbort(t *testing.T) {
	var calls atomic.Int64
	capability := validBuilderCapability("cap.abort", "records.abort")
	capability.Handler = func(_ context.Context, _ authz.Subject, input builderInput) (builderOutput, error) {
		if calls.Add(1) == 1 {
			return builderOutput{}, errors.New("trusted handler detail")
		}
		return builderOutput(input), nil
	}
	server := buildTestServer(t, authz.AllowAll(), codemode.DefaultLimits(), capability)

	_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.abort(value="first")
`)
	require.ErrorIs(t, err, codemode.ErrCapabilityFailure)
	value, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.abort(value="second")
`)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "second"}, value)
}

// TestServerExecuteWorkerParallelCap proves live workers never exceed the configured cap.
func TestServerExecuteWorkerParallelCap(t *testing.T) {
	limits := codemode.DefaultLimits()
	limits.MaxConcurrentExecutions = 2
	limits.MaxExecutionTime = 20 * time.Second
	var current atomic.Int32
	var peak atomic.Int32
	capability := validBuilderCapability("cap.parallel", "records.parallel")
	capability.Handler = func(_ context.Context, _ authz.Subject, input builderInput) (builderOutput, error) {
		active := current.Add(1)
		updatePeak(&peak, active)
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return builderOutput(input), nil
	}
	server := buildTestServer(t, authz.AllowAll(), limits, capability)

	var group sync.WaitGroup
	errors := make(chan error, 8)
	for index := range 8 {
		group.Go(func() {
			_, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, codemode.Program(`
def main():
    return records.parallel(value="`+strconv.Itoa(index)+`")
`))
			errors <- err
		})
	}
	group.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	assert.LessOrEqual(t, peak.Load(), int32(limits.MaxConcurrentExecutions))
}

// TestServerExecuteWorkerCancellationDuringDispatch proves cancellation does not wait for trusted Go.
func TestServerExecuteWorkerCancellationDuringDispatch(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var calls atomic.Int32
	capability := validBuilderCapability("cap.cancel", "records.cancel")
	capability.Handler = func(_ context.Context, _ authz.Subject, input builderInput) (builderOutput, error) {
		if calls.Add(1) == 1 {
			close(handlerStarted)
			<-releaseHandler
		}
		return builderOutput(input), nil
	}
	server := buildTestServer(t, authz.AllowAll(), codemode.DefaultLimits(), capability)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := server.Execute(ctx, authz.Subject{ID: "subject-1"}, `
def main():
    return records.cancel(value="first")
`)
		result <- err
	}()
	<-handlerStarted
	cancel()

	require.ErrorIs(t, <-result, context.Canceled)
	close(releaseHandler)
	value, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `
def main():
    return records.cancel(value="second")
`)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"value": "second"}, value)
}

// TestServerExecuteWorkerRepeatedKillReap proves repeated cancellation leaves later execution usable.
func TestServerExecuteWorkerRepeatedKillReap(t *testing.T) {
	limits := codemode.DefaultLimits()
	limits.MaxExecutionSteps = math.MaxUint64
	server := buildWorkerOnlyServer(t, limits)
	for range 6 {
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		go func() {
			_, err := server.Execute(ctx, authz.Subject{ID: "subject-1"}, busyWorkerProgram)
			result <- err
		}()
		time.Sleep(10 * time.Millisecond)
		cancel()
		require.ErrorIs(t, <-result, context.Canceled)
	}

	value, err := server.Execute(t.Context(), authz.Subject{ID: "subject-1"}, `def main(): return "ready"`)
	require.NoError(t, err)
	assert.Equal(t, "ready", value)
}

// buildWorkerOnlyServer returns a probed server without native capabilities.
func buildWorkerOnlyServer(t *testing.T, limits codemode.Limits) *codemode.Server {
	t.Helper()
	builder := codemode.New(codemode.Options{
		Authorizer: authz.AllowAll(),
		Limits:     limits,
	})
	server, err := builder.Build()
	require.NoError(t, err)
	return server
}

// updatePeak records value when it is greater than the current concurrent peak.
func updatePeak(peak *atomic.Int32, value int32) {
	for {
		current := peak.Load()
		if value <= current || peak.CompareAndSwap(current, value) {
			return
		}
	}
}
