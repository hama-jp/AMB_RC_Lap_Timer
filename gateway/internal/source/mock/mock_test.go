package mock

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/p3frame"
)

func TestMockSource_FrameShape(t *testing.T) {
	s := &Source{
		Interval: 0, // no waiting
		Rand:     rand.New(rand.NewSource(1)),
	}
	defer s.Close()

	frame, err := s.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(frame) < 12 {
		t.Fatalf("frame too short: %d", len(frame))
	}
	if frame[0] != p3frame.SOR {
		t.Errorf("expected SOR=0x8E at byte 0, got 0x%02X", frame[0])
	}
	if frame[len(frame)-1] != p3frame.EOR {
		t.Errorf("expected EOR=0x8F at last byte, got 0x%02X", frame[len(frame)-1])
	}
}

func TestMockSource_EmitsValidPassingFrames(t *testing.T) {
	s := &Source{
		Interval: 0,
		Rand:     rand.New(rand.NewSource(1)),
	}
	defer s.Close()

	frame, err := s.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	unescaped := p3frame.Unescape(frame)
	header, ok := p3frame.ParseHeader(unescaped)
	if !ok {
		t.Fatalf("ParseHeader failed")
	}
	if header.TOR != p3frame.TORPassing {
		t.Fatalf("TOR: got 0x%04x want 0x%04x", header.TOR, p3frame.TORPassing)
	}
	if int(header.FrameLength) != len(unescaped) {
		t.Fatalf("FrameLength: got %d want %d", header.FrameLength, len(unescaped))
	}

	fields := map[byte][]byte{}
	p3frame.WalkTLV(unescaped, func(id, _ byte, val []byte) bool {
		fields[id] = val
		return true
	})

	if got := binary.LittleEndian.Uint32(fields[p3frame.PassingPassingNumber]); got != 1 {
		t.Errorf("PASSING_NUMBER: got %d want 1", got)
	}
	if got := binary.LittleEndian.Uint32(fields[p3frame.PassingTransponder]); got != legacyMockTransponder {
		t.Errorf("TRANSPONDER: got 0x%08x want 0x%08x", got, legacyMockTransponder)
	}
	if got := binary.LittleEndian.Uint64(fields[p3frame.PassingRTCTime]); got != 0 {
		t.Errorf("RTC_TIME: got %d want 0", got)
	}
}

func TestMockSource_DistinctFrames(t *testing.T) {
	s := &Source{Interval: 0, Rand: rand.New(rand.NewSource(1))}
	defer s.Close()
	a, _ := s.Read(context.Background())
	b, _ := s.Read(context.Background())
	if string(a) == string(b) {
		t.Errorf("expected counter to differ between frames, got identical")
	}
}

func TestMockSource_ContextCancel_DuringSleep(t *testing.T) {
	s := &Source{Interval: 5 * time.Second, Rand: rand.New(rand.NewSource(1))}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	_, err := s.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestMockSource_CloseThenRead_ReturnsEOF(t *testing.T) {
	s := &Source{Interval: 0, Rand: rand.New(rand.NewSource(1))}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := s.Read(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestMockSource_Close_Idempotent(t *testing.T) {
	s := New()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// fakeClock is a Now/Sleep pair for deterministic multi-ponder tests:
// every Sleep call advances the clock by exactly d so the schedule is
// played out without real wall-clock waits.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.now = c.now.Add(d)
	return nil
}

func TestMockSource_Multi_RotatesAcrossTransponders(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)}
	s := &Source{
		Transponders: []uint32{10, 20, 30},
		LapMs:        30_000,
		JitterMs:     0,
		Rand:         rand.New(rand.NewSource(1)),
		Now:          clock.Now,
		Sleep:        clock.Sleep,
	}
	defer s.Close()

	got := make([]uint32, 0, 3)
	for i := 0; i < 3; i++ {
		frame, err := s.Read(context.Background())
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
		got = append(got, transponderOf(t, frame))
	}
	want := []uint32{10, 20, 30}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("frame %d transponder: got %d want %d (full got=%v)", i, got[i], id, got)
		}
	}
}

func TestMockSource_Multi_RtcTimeMonotonicPerPonder(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)}
	s := &Source{
		Transponders: []uint32{10, 20},
		LapMs:        20_000,
		JitterMs:     0,
		Rand:         rand.New(rand.NewSource(1)),
		Now:          clock.Now,
		Sleep:        clock.Sleep,
	}
	defer s.Close()

	// Two laps × two ponders = 4 frames. Order of crosses with stride
	// 10s and lap 20s: T+0 (id=10), T+10 (id=20), T+20 (id=10), T+30 (id=20).
	type record struct {
		transponder uint32
		rtcTimeUs   uint64
	}
	records := make([]record, 0, 4)
	for i := 0; i < 4; i++ {
		frame, err := s.Read(context.Background())
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
		records = append(records, record{
			transponder: transponderOf(t, frame),
			rtcTimeUs:   rtcTimeOf(t, frame),
		})
	}

	// Group by transponder and check RTC_TIME differences are exactly lap-ms.
	byID := map[uint32][]uint64{}
	for _, r := range records {
		byID[r.transponder] = append(byID[r.transponder], r.rtcTimeUs)
	}
	for id, times := range byID {
		if len(times) < 2 {
			t.Fatalf("ponder %d: only %d frames, want >=2", id, len(times))
		}
		diff := times[1] - times[0]
		const wantUs = 20_000_000
		if diff != wantUs {
			t.Errorf("ponder %d: lap diff = %d µs, want %d (no jitter)", id, diff, wantUs)
		}
	}
}

func TestMockSource_Multi_PassingNumberPerPonder(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)}
	s := &Source{
		Transponders: []uint32{10, 20},
		LapMs:        20_000,
		JitterMs:     0,
		Rand:         rand.New(rand.NewSource(1)),
		Now:          clock.Now,
		Sleep:        clock.Sleep,
	}
	defer s.Close()

	// Read 4 frames; expect each ponder to count from 1 independently.
	byID := map[uint32][]uint32{}
	for i := 0; i < 4; i++ {
		frame, err := s.Read(context.Background())
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
		id := transponderOf(t, frame)
		byID[id] = append(byID[id], passingNumberOf(t, frame))
	}
	for id, nums := range byID {
		if len(nums) != 2 || nums[0] != 1 || nums[1] != 2 {
			t.Errorf("ponder %d PASSING_NUMBER: got %v want [1 2]", id, nums)
		}
	}
}

func TestMockSource_Multi_JitterStaysWithinBounds(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)}
	s := &Source{
		Transponders: []uint32{10},
		// len==1 falls into legacy mode, so use 2 ponders to exercise
		// jitter path. With one ponder the jitter code is unreachable.
	}
	s.Transponders = []uint32{10, 20}
	s.LapMs = 20_000
	s.JitterMs = 1_000 // ±1s
	s.Rand = rand.New(rand.NewSource(42))
	s.Now = clock.Now
	s.Sleep = clock.Sleep
	defer s.Close()

	// Collect 10 RTC_TIMEs for ponder 10 and check successive lap diffs
	// stay within [lapMs-jitter, lapMs+jitter].
	var times []uint64
	for len(times) < 10 {
		frame, err := s.Read(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if transponderOf(t, frame) == 10 {
			times = append(times, rtcTimeOf(t, frame))
		}
	}
	const lapUs = 20_000_000
	const jitterUs = 1_000_000
	for i := 1; i < len(times); i++ {
		diff := int64(times[i]) - int64(times[i-1])
		if diff < lapUs-jitterUs || diff > lapUs+jitterUs {
			t.Errorf("lap %d→%d diff = %d µs, want within [%d, %d]",
				i-1, i, diff, lapUs-jitterUs, lapUs+jitterUs)
		}
	}
}

func TestMockSource_New_DefaultsToMultiMode(t *testing.T) {
	s := New()
	defer s.Close()
	if len(s.Transponders) != len(DefaultTransponders) {
		t.Errorf("Transponders len: got %d want %d (default multi-mode)",
			len(s.Transponders), len(DefaultTransponders))
	}
	if s.LapMs <= 0 {
		t.Errorf("LapMs: got %d, want a sensible default for demos", s.LapMs)
	}
}

// transponderOf decodes the TRANSPONDER TLV from a wire-encoded frame.
func transponderOf(t *testing.T, frame []byte) uint32 {
	t.Helper()
	val := tlvOf(t, frame, p3frame.PassingTransponder)
	return binary.LittleEndian.Uint32(val)
}

func rtcTimeOf(t *testing.T, frame []byte) uint64 {
	t.Helper()
	val := tlvOf(t, frame, p3frame.PassingRTCTime)
	return binary.LittleEndian.Uint64(val)
}

func passingNumberOf(t *testing.T, frame []byte) uint32 {
	t.Helper()
	val := tlvOf(t, frame, p3frame.PassingPassingNumber)
	return binary.LittleEndian.Uint32(val)
}

func tlvOf(t *testing.T, frame []byte, id byte) []byte {
	t.Helper()
	unesc := p3frame.Unescape(frame)
	var got []byte
	p3frame.WalkTLV(unesc, func(tlvID, _ byte, val []byte) bool {
		if tlvID == id {
			got = val
			return false
		}
		return true
	})
	if got == nil {
		t.Fatalf("TLV id=0x%02X not found in frame", id)
	}
	return got
}
