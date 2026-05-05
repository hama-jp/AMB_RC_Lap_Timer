// Package p3frame implements the wire-format primitives of the AMB P3 protocol:
// frame splitting at 0x8F/0x8E boundaries, escape/unescape of body bytes,
// header parsing, and TLV traversal.
//
// The package is shared by gateway/cmd/analyze (read-only inspection) and
// gateway/cmd/anonymize (mutate then re-encode). It does NOT implement CRC
// validation — docs/protocol-p3.md §6 keeps that as an open item and the
// initial gateway scope skips verification.
//
// All public APIs operate on byte slices and never panic on truncated input;
// they return zero values or stop iteration instead so that callers can
// safely walk possibly-corrupt streams.
package p3frame

import "encoding/binary"

// Special wire bytes (docs/protocol-p3.md §2, §4).
const (
	SOR     = 0x8E // Start Of Record
	EOR     = 0x8F // End Of Record
	ESC     = 0x8D // Escape marker
	ESCBias = 0x20 // Escape bias: encoded byte = original + ESCBias
)

// HeaderSize is the size of the fixed header (including SOR) inside an
// unescaped frame, in bytes. See docs/protocol-p3.md §3.
const HeaderSize = 10

// TOR (Type Of Record) values used by the gateway. Only the ones we have
// actually seen on the wire or care about are enumerated here. See
// docs/protocol-p3.md §7.2 for the full list.
const (
	TORReset   = 0x0000
	TORPassing = 0x0001
	TORStatus  = 0x0002
	TORVersion = 0x0003
)

// PASSING field IDs (docs/protocol-p3.md §7.3).
const (
	PassingPassingNumber = 0x01
	PassingTransponder   = 0x03
	PassingRTCTime       = 0x04
	PassingStrength      = 0x05
	PassingHits          = 0x06
	PassingFlags         = 0x08
)

// GeneralDecoderID is the cross-TOR DECODER_ID field (docs §7.1).
const GeneralDecoderID = 0x81

// Header holds the fixed-size frame header decoded from the first 10 bytes
// of an unescaped frame (i.e., SOR + 9 header bytes).
type Header struct {
	Version     byte
	FrameLength uint16
	CRC         uint16
	Flags       uint16
	TOR         uint16
}

// Split walks data and returns each P3 frame found, where a frame is the
// byte range starting at SOR (0x8E) and ending at the next EOR (0x8F).
// Bytes before the first SOR or after the last EOR are silently dropped.
// The returned slices share storage with data; do not mutate them.
func Split(data []byte) [][]byte {
	var frames [][]byte
	start := -1
	for i := 0; i < len(data); i++ {
		b := data[i]
		if start == -1 {
			if b == SOR {
				start = i
			}
			continue
		}
		if b == EOR {
			frames = append(frames, data[start:i+1])
			start = -1
		}
	}
	return frames
}

// Unescape returns a copy of frame with the body's escape sequences decoded.
// The returned slice is bracketed by SOR/EOR (kept identical to the input).
// 0x8D <byte> sequences in the body become <byte - ESCBias>; bare 0x8D
// (truncated escape) is dropped, mirroring the gateway's lenient behaviour.
//
// Callers may mutate the returned slice (e.g. anonymize TLV values) before
// passing it to Escape.
func Unescape(frame []byte) []byte {
	if len(frame) < 2 {
		out := make([]byte, len(frame))
		copy(out, frame)
		return out
	}
	out := make([]byte, 0, len(frame))
	out = append(out, frame[0]) // SOR
	inner := frame[1 : len(frame)-1]
	esc := false
	for _, b := range inner {
		if esc {
			out = append(out, b-ESCBias)
			esc = false
			continue
		}
		if b == ESC {
			esc = true
			continue
		}
		out = append(out, b)
	}
	out = append(out, frame[len(frame)-1]) // EOR
	return out
}

// Escape is the inverse of Unescape. Given an unescaped frame
// (SOR + body + EOR), it produces the wire-encoded frame where any
// 0x8D / 0x8E / 0x8F byte in the body is encoded as 0x8D <byte+ESCBias>.
// The SOR at index 0 and EOR at the last index are preserved verbatim and
// are NOT escaped (they are the frame markers).
func Escape(unesc []byte) []byte {
	if len(unesc) < 2 {
		out := make([]byte, len(unesc))
		copy(out, unesc)
		return out
	}
	out := make([]byte, 0, len(unesc)+8)
	out = append(out, unesc[0]) // SOR
	for _, b := range unesc[1 : len(unesc)-1] {
		switch b {
		case ESC, SOR, EOR:
			out = append(out, ESC, b+ESCBias)
		default:
			out = append(out, b)
		}
	}
	out = append(out, unesc[len(unesc)-1]) // EOR
	return out
}

// ParseHeader extracts the header from an unescaped frame.
// Returns ok=false if unesc is shorter than HeaderSize.
func ParseHeader(unesc []byte) (Header, bool) {
	if len(unesc) < HeaderSize {
		return Header{}, false
	}
	return Header{
		Version:     unesc[1],
		FrameLength: binary.LittleEndian.Uint16(unesc[2:4]),
		CRC:         binary.LittleEndian.Uint16(unesc[4:6]),
		Flags:       binary.LittleEndian.Uint16(unesc[6:8]),
		TOR:         binary.LittleEndian.Uint16(unesc[8:10]),
	}, true
}

// WalkTLV iterates the TLV list inside an unescaped frame's body, i.e. the
// bytes between the fixed header and the trailing EOR. fn is invoked for
// each TLV; the val slice points into unesc, so writes to val[i] mutate the
// frame in place — this is the supported way to anonymize a field.
//
// fn returning false stops the walk early. A truncated TLV at the tail
// (length byte missing or value shorter than declared) is ignored silently.
func WalkTLV(unesc []byte, fn func(id, length byte, val []byte) bool) {
	if len(unesc) < HeaderSize+1 {
		return
	}
	body := unesc[HeaderSize : len(unesc)-1] // exclude trailing EOR
	for i := 0; i < len(body); {
		if i+2 > len(body) {
			return
		}
		id := body[i]
		flen := body[i+1]
		end := i + 2 + int(flen)
		if end > len(body) {
			return
		}
		val := body[i+2 : end]
		if !fn(id, flen, val) {
			return
		}
		i = end
	}
}
