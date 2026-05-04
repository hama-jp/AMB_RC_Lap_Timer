package mock

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"testing"
	"time"
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
	if frame[0] != 0x8E {
		t.Errorf("expected SOR=0x8E at byte 0, got 0x%02X", frame[0])
	}
	if frame[len(frame)-1] != 0x8F {
		t.Errorf("expected EOR=0x8F at last byte, got 0x%02X", frame[len(frame)-1])
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
