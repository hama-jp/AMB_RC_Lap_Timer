// Package mock provides an in-memory Source that emits synthetic byte frames
// at a configurable interval. It exists so the gateway can be exercised on a
// developer laptop without the real AMB decoder
// (docs/gateway-technical-decision.md §"実機なしでの開発方針").
//
// The frame layout is intentionally only "P3-shaped" (SOR 0x8E … EOR 0x8F)
// so a captured `--record` output looks plausible under xxd; it is NOT a
// valid P3 record. Phase #2 (TS parser) is the consumer of real captured data.
package mock

import (
	"context"
	"encoding/binary"
	"io"
	"math/rand"
	"sync"
	"time"
)

const (
	sor = 0x8E
	eor = 0x8F
)

// Source is an in-memory bytestream emitter.
type Source struct {
	// Interval between emitted frames. 0 means "emit immediately"; useful in tests.
	Interval time.Duration
	// Rand seeds the synthetic field values. Optional; defaults to a
	// time-seeded PRNG on first use.
	Rand *rand.Rand
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
	// Sleep waits for d, returning early if ctx is cancelled.
	// Defaults to a context-aware time.NewTimer.
	Sleep func(ctx context.Context, d time.Duration) error

	once    sync.Once
	closed  chan struct{}
	closeMu sync.Mutex
	counter uint32
}

// New returns a mock Source with sensible defaults
// (1.5 s interval matching the project's typical PASSING cadence).
func New() *Source {
	return &Source{
		Interval: 1500 * time.Millisecond,
	}
}

func (s *Source) init() {
	s.once.Do(func() {
		s.closed = make(chan struct{})
		if s.Rand == nil {
			s.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		if s.Now == nil {
			s.Now = time.Now
		}
		if s.Sleep == nil {
			s.Sleep = ctxSleep
		}
	})
}

// Read returns the next synthetic frame.
func (s *Source) Read(ctx context.Context) ([]byte, error) {
	s.init()
	if s.Interval > 0 {
		if err := s.Sleep(ctx, s.Interval); err != nil {
			return nil, err
		}
	}
	select {
	case <-s.closed:
		return nil, io.EOF
	default:
	}
	return s.frame(), nil
}

// Close stops the source. Subsequent Read calls return io.EOF.
func (s *Source) Close() error {
	s.init()
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	select {
	case <-s.closed:
		return nil
	default:
		close(s.closed)
		return nil
	}
}

// frame builds a synthetic ~30-byte record:
//
//	0x8E | 9-byte header | TLV-shaped body | 0x8F
//
// The header's Frame Length field is filled in so a casual `xxd` glance
// shows believable structure, but the body is not a real P3 record.
func (s *Source) frame() []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint32(body[0:4], 0x12345678) // pretend transponder
	binary.LittleEndian.PutUint64(body[4:12], uint64(s.counter)*1500*1000)
	body[12] = byte(s.Rand.Intn(64) + 32) // pretend strength
	body[13] = byte(s.Rand.Intn(8))       // pretend hits
	body[14] = 0x00
	body[15] = 0x00
	s.counter++

	header := make([]byte, 9)
	// Pretend "Frame Length" in bytes 0..1 little-endian (SOR + header + body + EOR).
	binary.LittleEndian.PutUint16(header[0:2], uint16(1+len(header)+len(body)+1))
	// header[2..8] left as zero placeholders.

	out := make([]byte, 0, 1+len(header)+len(body)+1)
	out = append(out, sor)
	out = append(out, header...)
	out = append(out, body...)
	out = append(out, eor)
	return out
}

func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
