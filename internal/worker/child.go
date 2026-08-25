package worker

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/meigma/codemode/internal/execution"
)

// errNativeAbort is the private interpreter-unwind sentinel returned when the
// parent answers a native call with native_abort.
//
// It does not wrap or equal any execution.Err* sentinel, so Engine's default
// classifyRuntimeError branch preserves it through multi-%w wrapping.
var errNativeAbort = errors.New("native abort")

// errChildService classifies a child protocol or internal service failure that
// must not become a final_error frame.
var errChildService = errors.New("worker service failure")

// IsWorker reports whether the current process carries the private worker marker.
func IsWorker() bool {
	return workerMode()
}

// Serve serves exactly one request from [os.Stdin] and [os.Stdout] and returns process status 0/1.
func Serve() int {
	if err := serve(os.Stdin, os.Stdout); err != nil {
		return 1
	}
	return 0
}

// serve handles exactly one probe or execution request from r and w.
func serve(r io.Reader, w io.Writer) error {
	frame, err := readInitialFrame(r)
	if err != nil {
		return err
	}
	switch typed := frame.(type) {
	case probeFrame:
		return serveProbe(w)
	case execFrame:
		return serveExec(r, w, typed)
	default:
		return errIllegalState
	}
}

// readInitialFrame reads the trusted first parent payload without a precomputed cap.
func readInitialFrame(r io.Reader) (any, error) {
	payload, err := readFrame(r, math.MaxUint32)
	if err != nil {
		return nil, err
	}
	return decodePayload(payload)
}

// serveProbe writes the matching protocol-1 acknowledgement.
func serveProbe(w io.Writer) error {
	payload, err := encodeProbeAck()
	if err != nil {
		return err
	}
	return writeFrame(w, payload, probeCap)
}

// serveExec validates one execution request, runs it, and writes the terminal frame.
func serveExec(r io.Reader, w io.Writer, frame execFrame) error {
	if err := validateChildLimits(frame.Limits); err != nil {
		return err
	}
	if err := validateManifest(frame.Manifest); err != nil {
		return err
	}
	parentCap, err := parentPayloadCap(frame.Limits.MaxValueBytes)
	if err != nil {
		return err
	}
	childCap, err := childPayloadCap(frame.Limits.MaxValueBytes, frame.Manifest)
	if err != nil {
		return err
	}
	conn := &childConn{
		r:             r,
		w:             w,
		parentReadCap: parentCap,
		writeCap:      childCap,
		kind:          connKindExec,
		state:         stateReady,
		limits:        frame.Limits,
		hasLimits:     true,
	}
	if len(frame.Source) > frame.Limits.MaxSourceBytes {
		return conn.writeFinalError(finalErrorResourceLimit)
	}
	engine, err := execution.New(manifestBindings(frame.Manifest))
	if err != nil {
		return err
	}
	result, err := engine.Execute(
		frame.Source,
		nativeForwarder(conn),
		executionLimits(frame.Limits),
	)
	if err != nil {
		return writeExecutionError(conn, err)
	}
	return writeFinalResult(conn, result)
}

// manifestBindings converts advertised manifest entries into Engine bindings.
func manifestBindings(manifest []manifestEntry) []execution.CapabilityBinding {
	bindings := make([]execution.CapabilityBinding, len(manifest))
	for index, entry := range manifest {
		bindings[index] = execution.CapabilityBinding{
			ID:    entry.ID,
			Name:  entry.Name,
			Input: entry.Input,
		}
	}
	return bindings
}

// executionLimits projects child-visible budgets onto one Engine execution.
func executionLimits(limits childLimits) execution.Limits {
	return execution.Limits{
		MaxSourceBytes:    limits.MaxSourceBytes,
		MaxExecutionSteps: limits.MaxExecutionSteps,
		MaxNativeCalls:    limits.MaxNativeCalls,
		MaxValueDepth:     limits.MaxValueDepth,
		MaxValueBytes:     limits.MaxValueBytes,
	}
}

// nativeForwarder sends one native_call and waits for the parent reply.
func nativeForwarder(conn *childConn) execution.NativeCall {
	return func(id string, arguments map[string]any) (any, error) {
		if err := conn.writeNativeCall(id, arguments); err != nil {
			if errors.Is(err, errInvalidValue) || errors.Is(err, errFrameTooLarge) {
				return nil, execution.ErrResourceLimit
			}
			return nil, fmt.Errorf("%w: %w", errChildService, err)
		}
		frame, err := conn.read()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errChildService, err)
		}
		switch typed := frame.(type) {
		case nativeResultFrame:
			return typed.Result, nil
		case nativeAbortFrame:
			return nil, errNativeAbort
		default:
			return nil, errChildService
		}
	}
}

// writeExecutionError maps an Engine failure onto abort suppression or a final_error.
func writeExecutionError(conn *childConn, err error) error {
	if errors.Is(err, errNativeAbort) {
		return nil
	}
	if errors.Is(err, errChildService) {
		return err
	}
	return conn.writeFinalError(finalErrorFrom(err))
}

// writeFinalResult writes a successful value or classifies a legal value-byte overflow.
func writeFinalResult(conn *childConn, result any) error {
	if err := conn.writeFinal(result); err != nil {
		if code, ok := finalWriteCode(err); ok {
			return conn.writeFinalError(code)
		}
		return err
	}
	return nil
}

// finalWriteCode maps a failed final write onto a child-owned terminal code.
func finalWriteCode(err error) (finalErrorCode, bool) {
	switch {
	case errors.Is(err, errFrameTooLarge):
		return finalErrorResourceLimit, true
	case errors.Is(err, errInvalidValue), errors.Is(err, errInvalidNumber):
		return finalErrorInvalidProgram, true
	default:
		return "", false
	}
}

// finalErrorFrom maps a classified Engine error onto a child-owned terminal code.
func finalErrorFrom(err error) finalErrorCode {
	switch {
	case errors.Is(err, execution.ErrInvalidProgram):
		return finalErrorInvalidProgram
	case errors.Is(err, execution.ErrInvalidArguments):
		return finalErrorInvalidArguments
	case errors.Is(err, execution.ErrResourceLimit):
		return finalErrorResourceLimit
	default:
		return finalErrorInternal
	}
}
