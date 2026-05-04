// Package upstream computes reconnect timings for the AMB TCP source.
//
// The actual TCP dialing lives in internal/source/real because that is where
// the byte stream is consumed; this package keeps only the time math, which
// is the part most worth unit-testing in isolation
// (docs/test-strategy.md §4.2 requires deterministic backoff tests).
package upstream

import (
	"math/rand"
	"time"
)

// Backoff produces reconnect delays following an exponential schedule with
// optional ±Jitter*delay random offset, capped at Max.
//
// Schedule for attempt N (zero-based, attempt=0 is the first retry):
//
//	delay(N) = min(Initial * 2^N, Max) +/- (Jitter * delay)
//
// docs/architecture.md §4.4 / docs/gateway-technical-decision.md §3 specify
// initial=1s, max=30s, jitter=0.2.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
	Jitter  float64    // 0..1, e.g., 0.2 for ±20%
	Rand    *rand.Rand // optional; if nil, time-seeded on first call
}

// NewBackoff returns a Backoff seeded from the current time.
func NewBackoff(initial, max time.Duration, jitter float64) *Backoff {
	return &Backoff{
		Initial: initial,
		Max:     max,
		Jitter:  jitter,
		Rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Next returns the delay before the (attempt+1)-th retry.
// Negative attempts are treated as zero. Overflow is saturated at Max.
func (b *Backoff) Next(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	base := b.Initial
	for i := 0; i < attempt; i++ {
		next := base * 2
		// Saturate at Max on cap or overflow.
		if next <= 0 || next > b.Max {
			base = b.Max
			break
		}
		base = next
	}
	if base > b.Max {
		base = b.Max
	}
	if b.Jitter > 0 {
		if b.Rand == nil {
			b.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		// Uniform offset in [-Jitter*base, +Jitter*base].
		delta := time.Duration((b.Rand.Float64()*2 - 1) * b.Jitter * float64(base))
		base += delta
	}
	if base < 0 {
		base = 0
	}
	return base
}
