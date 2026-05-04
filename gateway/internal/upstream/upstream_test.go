package upstream

import (
	"math/rand"
	"testing"
	"time"
)

func TestBackoff_Schedule_NoJitter(t *testing.T) {
	b := &Backoff{
		Initial: time.Second,
		Max:     30 * time.Second,
		Jitter:  0,
	}
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{-1, 1 * time.Second},
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // 32s saturated to 30s
		{6, 30 * time.Second},
		{100, 30 * time.Second}, // overflow saturated
	}
	for _, tc := range cases {
		got := b.Next(tc.attempt)
		if got != tc.want {
			t.Errorf("Next(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestBackoff_Jitter_BoundsAndMean(t *testing.T) {
	b := &Backoff{
		Initial: time.Second,
		Max:     30 * time.Second,
		Jitter:  0.2,
		Rand:    rand.New(rand.NewSource(1)),
	}
	const N = 5000
	var sum time.Duration
	for i := 0; i < N; i++ {
		d := b.Next(0) // base=1s, ±20% → [800ms, 1200ms]
		if d < 800*time.Millisecond || d > 1200*time.Millisecond {
			t.Fatalf("jitter out of [800ms,1200ms]: got %v", d)
		}
		sum += d
	}
	mean := sum / N
	// With N=5000 the sample mean should be very close to 1s; allow ±10ms.
	if mean < 990*time.Millisecond || mean > 1010*time.Millisecond {
		t.Errorf("jittered mean off: got %v want ~1s", mean)
	}
}

func TestBackoff_Jitter_AtCap(t *testing.T) {
	b := &Backoff{
		Initial: time.Second,
		Max:     30 * time.Second,
		Jitter:  0.2,
		Rand:    rand.New(rand.NewSource(2)),
	}
	// Once saturated at Max, jitter still applies on top of Max.
	for i := 0; i < 100; i++ {
		d := b.Next(50) // way beyond cap
		// Bounds: 30s ± 20% = [24s, 36s]. Negative would clamp to 0 (won't happen here).
		if d < 24*time.Second || d > 36*time.Second {
			t.Fatalf("cap-jitter out of [24s,36s]: got %v", d)
		}
	}
}

func TestNewBackoff_DefaultsRand(t *testing.T) {
	b := NewBackoff(time.Second, 30*time.Second, 0.2)
	if b.Rand == nil {
		t.Fatal("NewBackoff did not seed Rand")
	}
}

func TestBackoff_NoJitter_NeverNegative(t *testing.T) {
	b := &Backoff{Initial: 0, Max: 0, Jitter: 0}
	for i := 0; i < 5; i++ {
		if d := b.Next(i); d < 0 {
			t.Fatalf("negative delay: %v", d)
		}
	}
}
