package p3frame

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestSplit_MultipleFrames_LeadingTrailingGarbage(t *testing.T) {
	// 2 frames bracketed by garbage bytes outside SOR/EOR.
	data := []byte{
		0xAA, 0xBB, // pre-SOR garbage
		0x8E, 0x01, 0x02, 0x8F, // frame 1
		0xCC,                         // inter-frame garbage
		0x8E, 0x10, 0x20, 0x30, 0x8F, // frame 2
		0x99, // post-EOR garbage
	}
	got := Split(data)
	if len(got) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(got))
	}
	if !bytes.Equal(got[0], []byte{0x8E, 0x01, 0x02, 0x8F}) {
		t.Errorf("frame 1 wrong: % x", got[0])
	}
	if !bytes.Equal(got[1], []byte{0x8E, 0x10, 0x20, 0x30, 0x8F}) {
		t.Errorf("frame 2 wrong: % x", got[1])
	}
}

func TestSplit_NoFrames(t *testing.T) {
	if got := Split(nil); got != nil {
		t.Errorf("nil input: got %v", got)
	}
	if got := Split([]byte{0xAA, 0xBB}); got != nil {
		t.Errorf("garbage-only input: got %v", got)
	}
	// Unterminated frame (SOR but no EOR) is dropped.
	if got := Split([]byte{0x8E, 0x01, 0x02}); got != nil {
		t.Errorf("unterminated frame: got %v", got)
	}
}

func TestUnescape_NoEscapes_Identity(t *testing.T) {
	in := []byte{0x8E, 0x01, 0x02, 0x03, 0x8F}
	got := Unescape(in)
	if !bytes.Equal(got, in) {
		t.Errorf("identity Unescape failed: got %x want %x", got, in)
	}
}

func TestUnescape_DecodesAllThreeEscapeForms(t *testing.T) {
	// Wire: SOR | 8D AD (= 8D) | 8D AE (= 8E) | 8D AF (= 8F) | EOR
	in := []byte{0x8E, 0x8D, 0xAD, 0x8D, 0xAE, 0x8D, 0xAF, 0x8F}
	want := []byte{0x8E, 0x8D, 0x8E, 0x8F, 0x8F}
	got := Unescape(in)
	if !bytes.Equal(got, want) {
		t.Errorf("Unescape: got %x want %x", got, want)
	}
}

func TestEscape_EncodesAllThreeSpecialBytes(t *testing.T) {
	in := []byte{0x8E, 0x8D, 0x8E, 0x8F, 0x8F}
	want := []byte{0x8E, 0x8D, 0xAD, 0x8D, 0xAE, 0x8D, 0xAF, 0x8F}
	got := Escape(in)
	if !bytes.Equal(got, want) {
		t.Errorf("Escape: got %x want %x", got, want)
	}
}

func TestEscape_DoesNotTouchSORorEOR(t *testing.T) {
	// Even when only SOR/EOR are present, they must not be escaped.
	in := []byte{0x8E, 0x01, 0x02, 0x8F}
	got := Escape(in)
	if got[0] != SOR || got[len(got)-1] != EOR {
		t.Errorf("SOR/EOR mangled: got %x", got)
	}
	if !bytes.Equal(got, in) {
		t.Errorf("benign body altered: got %x want %x", got, in)
	}
}

func TestEscape_Unescape_RoundTrip(t *testing.T) {
	// All non-marker bytes plus the three special bytes should round-trip.
	body := []byte{}
	for b := 0; b < 256; b++ {
		body = append(body, byte(b))
	}
	unesc := append([]byte{SOR}, body...)
	unesc = append(unesc, EOR)

	wire := Escape(unesc)
	back := Unescape(wire)

	if !bytes.Equal(back, unesc) {
		t.Errorf("round-trip mismatch:\n got %s\nwant %s",
			hex.EncodeToString(back), hex.EncodeToString(unesc))
	}
}

func TestParseHeader_Basic(t *testing.T) {
	// Build an unescaped frame: SOR | ver=02 | flen=001b | crc=8e0b | flags=0000 | tor=0001 | body... | EOR
	unesc := []byte{
		SOR,
		0x02,       // version
		0x1B, 0x00, // frame length = 27
		0x0B, 0x8E, // crc
		0x00, 0x00, // flags
		0x01, 0x00, // TOR = PASSING (LE 0x0001)
		0xDE, 0xAD, 0xBE, // dummy body
		EOR,
	}
	h, ok := ParseHeader(unesc)
	if !ok {
		t.Fatal("ParseHeader returned !ok for valid input")
	}
	if h.Version != 0x02 || h.FrameLength != 27 || h.CRC != 0x8E0B ||
		h.Flags != 0 || h.TOR != TORPassing {
		t.Errorf("header parse wrong: %+v", h)
	}
}

func TestParseHeader_TooShort(t *testing.T) {
	if _, ok := ParseHeader([]byte{SOR, 0x01, 0x02}); ok {
		t.Error("expected !ok for short frame")
	}
}

func TestWalkTLV_BasicSequence(t *testing.T) {
	// Header + body: id=01 len=02 val=AABB | id=03 len=04 val=01020304 | EOR
	unesc := []byte{
		SOR,
		0, 0, 0, 0, 0, 0, 0, 0, 0, // 9 header bytes
		0x01, 0x02, 0xAA, 0xBB,
		0x03, 0x04, 0x01, 0x02, 0x03, 0x04,
		EOR,
	}
	type seen struct {
		id, length byte
		val        []byte
	}
	var got []seen
	WalkTLV(unesc, func(id, length byte, val []byte) bool {
		got = append(got, seen{id, length, append([]byte(nil), val...)})
		return true
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 TLVs, got %d", len(got))
	}
	if got[0].id != 0x01 || got[0].length != 2 || !bytes.Equal(got[0].val, []byte{0xAA, 0xBB}) {
		t.Errorf("TLV[0] wrong: %+v", got[0])
	}
	if got[1].id != 0x03 || got[1].length != 4 ||
		!bytes.Equal(got[1].val, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Errorf("TLV[1] wrong: %+v", got[1])
	}
}

func TestWalkTLV_CallbackMutationVisible(t *testing.T) {
	// The contract is that val is a slice into unesc — mutations propagate.
	unesc := []byte{
		SOR,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0x03, 0x04, 0xDE, 0xAD, 0xBE, 0xEF,
		EOR,
	}
	WalkTLV(unesc, func(id, length byte, val []byte) bool {
		if id == 0x03 && length == 4 {
			binary.LittleEndian.PutUint32(val, 0x12345678)
		}
		return true
	})
	want := []byte{
		SOR,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0x03, 0x04, 0x78, 0x56, 0x34, 0x12,
		EOR,
	}
	if !bytes.Equal(unesc, want) {
		t.Errorf("mutation not visible:\n got %x\nwant %x", unesc, want)
	}
}

func TestWalkTLV_StopsOnFalse(t *testing.T) {
	unesc := []byte{
		SOR,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x01, 0x11,
		0x02, 0x01, 0x22,
		0x03, 0x01, 0x33,
		EOR,
	}
	count := 0
	WalkTLV(unesc, func(id, length byte, val []byte) bool {
		count++
		return id != 0x02 // stop after id=0x02
	})
	if count != 2 {
		t.Errorf("expected 2 TLVs walked, got %d", count)
	}
}

func TestWalkTLV_TruncatedTailIgnored(t *testing.T) {
	// Body has a TLV claiming len=10 but only 2 value bytes follow.
	unesc := []byte{
		SOR,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x02, 0xAA, 0xBB, // ok
		0x02, 0x0A, 0xCC, 0xDD, // truncated
		EOR,
	}
	count := 0
	WalkTLV(unesc, func(id, length byte, val []byte) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("expected 1 TLV (truncated tail dropped), got %d", count)
	}
}
