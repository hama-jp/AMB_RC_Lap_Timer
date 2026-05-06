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

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/p3frame"
)

const (
	mockTransponder = 0x12345678
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

// frame builds a valid wire-encoded PASSING frame so the SPA parser, lap
// calculation, and speech path can be smoke-tested without a real decoder.
func (s *Source) frame() []byte {
	body := make([]byte, 0, 34)
	body = appendTLV32(body, p3frame.PassingPassingNumber, s.counter+1)
	body = appendTLV32(body, p3frame.PassingTransponder, mockTransponder)
	body = appendTLV64(body, p3frame.PassingRTCTime, uint64(s.counter)*1500*1000)
	body = appendTLV16(body, p3frame.PassingStrength, uint16(s.Rand.Intn(64)+32))
	body = appendTLV16(body, p3frame.PassingHits, uint16(s.Rand.Intn(8)))
	body = appendTLV16(body, p3frame.PassingFlags, 0)
	s.counter++

	totalLen := p3frame.HeaderSize + len(body) + 1
	unescaped := make([]byte, totalLen)
	unescaped[0] = p3frame.SOR
	unescaped[1] = 0x02
	binary.LittleEndian.PutUint16(unescaped[2:4], uint16(totalLen))
	binary.LittleEndian.PutUint16(unescaped[8:10], p3frame.TORPassing)
	copy(unescaped[p3frame.HeaderSize:], body)
	unescaped[len(unescaped)-1] = p3frame.EOR
	return p3frame.Escape(unescaped)
}

func appendTLV16(body []byte, id byte, value uint16) []byte {
	body = append(body, id, 2)
	return binary.LittleEndian.AppendUint16(body, value)
}

func appendTLV32(body []byte, id byte, value uint32) []byte {
	body = append(body, id, 4)
	return binary.LittleEndian.AppendUint32(body, value)
}

func appendTLV64(body []byte, id byte, value uint64) []byte {
	body = append(body, id, 8)
	return binary.LittleEndian.AppendUint64(body, value)
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
