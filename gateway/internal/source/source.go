// Package source defines the abstraction over the byte stream that feeds the
// gateway: real TCP from an AMB decoder, an in-memory mock, or (later) a
// replay file. Concrete implementations live in subpackages.
//
// docs/architecture.md §3.3 lists the modes; this MVP (#1) ships real and
// mock only.
package source

import "context"

// Source produces raw bytes from an upstream. Implementations decide how to
// recover from transient errors (e.g., real reconnects with backoff). Read
// returns a non-nil error only when the source is permanently done — this
// includes context cancellation and io.EOF for sources that can complete.
type Source interface {
	// Read blocks until at least one byte is available and returns a fresh
	// slice owned by the caller. Returning a zero-length slice with a nil
	// error is allowed but discouraged.
	Read(ctx context.Context) ([]byte, error)

	// Close releases any resources. After Close, Read must return an error.
	Close() error
}
