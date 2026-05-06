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
	if got := binary.LittleEndian.Uint32(fields[p3frame.PassingTransponder]); got != mockTransponder {
		t.Errorf("TRANSPONDER: got 0x%08x want 0x%08x", got, mockTransponder)
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
