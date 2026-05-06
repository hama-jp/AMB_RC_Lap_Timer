// Package mock provides an in-memory Source that emits synthetic byte frames
// at a configurable interval. It exists so the gateway can be exercised on a
// developer laptop without the real AMB decoder
// (docs/gateway-technical-decision.md §"実機なしでの開発方針").
//
// Two modes:
//
//   - Multi-transponder (default via New): emits frames for several
//     transponders staggered across a configurable lap time, so the SPA
//     experiences a realistic race with multiple cars on track. Use this
//     for in-person demos and Field Test α-2 dry-runs.
//
//   - Legacy single-transponder (Transponders empty/length 1, Interval set):
//     emits at a fixed cadence with one ID. Test paths construct
//     `&Source{Interval: 0}` directly to use this; behavior is unchanged
//     from the pre-multi version.
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
	// legacyMockTransponder is the single-ID used when Transponders is empty
	// (test default). New() does NOT use this; demo mode uses the visible
	// 1/2/3 IDs from DefaultTransponders so the SPA settings field can be
	// set to a small number.
	legacyMockTransponder = 0x12345678

	defaultLapMs    = 18_000 // ~18s laps
	defaultJitterMs = 2_000  // ±2s
)

// DefaultTransponders is the multi-mode roster used by New(). The IDs match
// `tools/fieldtest/tcp-emitter` defaults so an operator who set
// `settings.transponder=1` in the SPA sees frames whether they're driving
// the in-process mock or the standalone tcp-emitter.
var DefaultTransponders = []uint32{1, 2, 3}

// Source is an in-memory bytestream emitter.
type Source struct {
	// Interval controls single-transponder cadence (legacy mode, when
	// len(Transponders) <= 1). Zero means "emit immediately"; useful in tests.
	// Ignored in multi-transponder mode (LapMs / JitterMs win).
	Interval time.Duration
	// Transponders is the rotating roster. Empty defaults to a single
	// legacyMockTransponder so existing test constructors keep working.
	// New() seeds DefaultTransponders for multi-mode demos.
	Transponders []uint32
	// LapMs is the per-transponder mean lap time (ms). Zero falls back to
	// defaultLapMs. Only consulted in multi-transponder mode.
	LapMs int
	// JitterMs is the ±jitter on each lap time (ms). Zero falls back to
	// defaultJitterMs. Only consulted in multi-transponder mode.
	JitterMs int
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
	counter uint32 // legacy single-transponder counter

	// multi-transponder state, populated in init() when len(Transponders) > 1.
	bootTime time.Time
	schedule []ponderSchedule
}

// ponderSchedule tracks one transponder's emission timetable.
type ponderSchedule struct {
	transponder   uint32
	nextEmit      time.Time // wall-clock time when this ponder should emit next
	passingNumber uint32    // increments per emit; matches PASSING_NUMBER field
}

// New returns a multi-transponder mock with realistic ~18 s laps.
// Operators can set the SPA's settings.transponder to one of
// DefaultTransponders (1, 2, 3) to see lap-times for that ID.
func New() *Source {
	return &Source{
		Transponders: DefaultTransponders,
		LapMs:        defaultLapMs,
		JitterMs:     defaultJitterMs,
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
		// Default to legacy single-ponder roster when caller didn't set one.
		if len(s.Transponders) == 0 {
			s.Transponders = []uint32{legacyMockTransponder}
		}
		// Stagger the ponders across one lap so they don't all cross the
		// loop simultaneously: with N ponders and Tlap ms per lap, each
		// ponder's first emit is at bootTime + (i * Tlap / N).
		if len(s.Transponders) > 1 {
			s.bootTime = s.Now()
			lap := time.Duration(s.lapMs()) * time.Millisecond
			stride := lap / time.Duration(len(s.Transponders))
			s.schedule = make([]ponderSchedule, len(s.Transponders))
			for i, t := range s.Transponders {
				s.schedule[i] = ponderSchedule{
					transponder: t,
					nextEmit:    s.bootTime.Add(time.Duration(i) * stride),
				}
			}
		}
	})
}

func (s *Source) lapMs() int {
	if s.LapMs > 0 {
		return s.LapMs
	}
	return defaultLapMs
}

// jitterMs returns the configured jitter. Zero is a valid "no jitter" value
// and is honoured as-is — the default is only applied via New().
func (s *Source) jitterMs() int {
	return s.JitterMs
}

// Read returns the next synthetic frame.
func (s *Source) Read(ctx context.Context) ([]byte, error) {
	s.init()
	select {
	case <-s.closed:
		return nil, io.EOF
	default:
	}
	if len(s.Transponders) > 1 {
		return s.readMulti(ctx)
	}
	return s.readLegacy(ctx)
}

// readLegacy is the pre-multi path: one transponder, fixed Interval.
func (s *Source) readLegacy(ctx context.Context) ([]byte, error) {
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
	rtcUs := uint64(s.counter) * 1500 * 1000
	s.counter++
	return s.buildFrame(s.Transponders[0], s.counter, rtcUs), nil
}

// readMulti picks the soonest-due ponder, sleeps until its time, emits a
// frame, and reschedules that ponder by lapMs ± jitter.
func (s *Source) readMulti(ctx context.Context) ([]byte, error) {
	next := 0
	for i := 1; i < len(s.schedule); i++ {
		if s.schedule[i].nextEmit.Before(s.schedule[next].nextEmit) {
			next = i
		}
	}
	if d := s.schedule[next].nextEmit.Sub(s.Now()); d > 0 {
		if err := s.Sleep(ctx, d); err != nil {
			return nil, err
		}
	}
	select {
	case <-s.closed:
		return nil, io.EOF
	default:
	}
	p := &s.schedule[next]
	p.passingNumber++
	// RTC_TIME is "μs since virtual decoder boot"; matches real AMB semantics
	// (docs/protocol-p3.md §7.3) so the SPA's lap calculation works without
	// a special case.
	elapsedUs := uint64(s.Now().Sub(s.bootTime).Microseconds())
	frame := s.buildFrame(p.transponder, p.passingNumber, elapsedUs)
	// Reschedule: nextEmit += lapMs ± jitter. Anchor on the previous
	// nextEmit (NOT s.Now()) so accumulated sleep skew doesn't bias the
	// long-run cadence.
	jitter := 0
	if j := s.jitterMs(); j > 0 {
		jitter = s.Rand.Intn(2*j+1) - j
	}
	p.nextEmit = p.nextEmit.Add(time.Duration(s.lapMs()+jitter) * time.Millisecond)
	return frame, nil
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

// buildFrame assembles one wire-encoded PASSING frame for the given identity.
func (s *Source) buildFrame(transponder, passingNumber uint32, rtcTimeUs uint64) []byte {
	body := make([]byte, 0, 34)
	body = appendTLV32(body, p3frame.PassingPassingNumber, passingNumber)
	body = appendTLV32(body, p3frame.PassingTransponder, transponder)
	body = appendTLV64(body, p3frame.PassingRTCTime, rtcTimeUs)
	body = appendTLV16(body, p3frame.PassingStrength, uint16(s.Rand.Intn(64)+32))
	body = appendTLV16(body, p3frame.PassingHits, uint16(s.Rand.Intn(8)))
	body = appendTLV16(body, p3frame.PassingFlags, 0)

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
