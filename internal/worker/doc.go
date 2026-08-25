// Package worker implements the private same-executable parent/child transport.
//
// Protocol types, frame codecs, and connection state machines are not public
// API. They exist only between a host process and a worker re-exec of the same
// binary.
package worker
