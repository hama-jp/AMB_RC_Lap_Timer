package frame

import (
	"encoding/binary"
	"testing"
)

// unescape reverses the body escape rules so the test can read header/TLV
// fields straight out of the buffer (mirrors gateway/internal/p3frame).
func unescape(t *testing.T, frame []byte) []byte {
	t.Helper()
	if len(frame) < 2 || frame[0] != SOR || frame[len(frame)-1] != EOR {
		t.Fatalf("bad framing: head=0x%02X tail=0x%02X len=%d",
			frame[0], frame[len(frame)-1], len(frame))
	}
	out := []byte{frame[0]}
	esc := false
	for _, b := range frame[1 : len(frame)-1] {
		if esc {
			out = append(out, b-0x20)
			esc = false
			continue
		}
		if b == ESC {
			esc = true
			continue
		}
		out = append(out, b)
	}
	out = append(out, frame[len(frame)-1])
	return out
}

func TestBuildPassing_FrameLengthMatches(t *testing.T) {
	wire := BuildPassing(PassingArgs{
		PassingNumber: 0x001185E3,
		Transponder:   0x00000001,
		RTCTimeUs:     1_777_985_972_473_000,
		Strength:      170,
		Hits:          200,
		Flags:         0,
		DecoderID:     0x00041D17,
	})
	unesc := unescape(t, wire)

	flen := binary.LittleEndian.Uint16(unesc[2:4])
	if int(flen) != len(unesc) {
		t.Errorf("FrameLength=%d, len(unescaped)=%d (must match per protocol-p3 §9 #1)",
			flen, len(unesc))
	}

	tor := binary.LittleEndian.Uint16(unesc[8:10])
	if tor != 0x0001 {
		t.Errorf("TOR=0x%04X, want PASSING 0x0001", tor)
	}
}

func TestBuildStatus_FrameLengthMatches(t *testing.T) {
	wire := BuildStatus(StatusArgs{
		Noise:        0x0006,
		Temperature:  0x001B,
		InputVoltage: 0x79,
		GPS:          0x00,
		DecoderID:    0x00041D17,
	})
	unesc := unescape(t, wire)

	flen := binary.LittleEndian.Uint16(unesc[2:4])
	if int(flen) != len(unesc) {
		t.Errorf("FrameLength=%d, len(unescaped)=%d", flen, len(unesc))
	}

	tor := binary.LittleEndian.Uint16(unesc[8:10])
	if tor != 0x0002 {
		t.Errorf("TOR=0x%04X, want STATUS 0x0002", tor)
	}
}

func TestBuildPassing_EscapesBodyBytes(t *testing.T) {
	// A transponder ID containing 0x8E must round-trip through escape.
	wire := BuildPassing(PassingArgs{
		PassingNumber: 1,
		Transponder:   0x8E8F8D01, // hits all three special bytes
		RTCTimeUs:     0,
		Strength:      0,
		Hits:          0,
		Flags:         0,
		DecoderID:     0,
	})
	// Wire form must keep exactly one SOR (head) and one EOR (tail).
	headEOR := 0
	for i, b := range wire {
		if b == EOR && i != len(wire)-1 {
			headEOR++
		}
	}
	if headEOR != 0 {
		t.Errorf("EOR appears in body %d times — escape failed", headEOR)
	}
	unesc := unescape(t, wire)
	flen := binary.LittleEndian.Uint16(unesc[2:4])
	if int(flen) != len(unesc) {
		t.Errorf("FrameLength=%d, len(unescaped)=%d", flen, len(unesc))
	}
}
