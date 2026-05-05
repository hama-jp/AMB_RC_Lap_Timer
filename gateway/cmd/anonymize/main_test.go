package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/p3frame"
)

// buildPassing constructs an unescaped PASSING frame with the given TRANSPONDER
// value and returns the wire-encoded (escaped) bytes.
func buildPassing(t *testing.T, transponder uint32) []byte {
	t.Helper()
	body := []byte{
		// header (after SOR)
		0x02,       // Version
		0x00, 0x00, // FrameLength (left as 0; not validated)
		0x00, 0x00, // CRC
		0x00, 0x00, // Flags
		0x01, 0x00, // TOR = PASSING
		// body TLVs
		// PASSING_NUMBER (id=0x01, len=4)
		0x01, 0x04, 0xAA, 0xBB, 0xCC, 0xDD,
		// TRANSPONDER (id=0x03, len=4) — placeholder; filled in below
		0x03, 0x04, 0, 0, 0, 0,
		// HITS (id=0x06, len=2) — kept to verify other TLVs are untouched
		0x06, 0x02, 0x42, 0x00,
	}
	// Patch in the transponder bytes (LE).
	tIdx := len(body) - 8 // position of the 4 placeholder bytes
	binary.LittleEndian.PutUint32(body[tIdx:tIdx+4], transponder)

	unesc := append([]byte{p3frame.SOR}, body...)
	unesc = append(unesc, p3frame.EOR)
	return p3frame.Escape(unesc)
}

// buildStatus constructs a STATUS frame with arbitrary body bytes; included
// to verify Anonymize leaves non-PASSING frames unchanged.
func buildStatus(t *testing.T) []byte {
	t.Helper()
	unesc := []byte{
		p3frame.SOR,
		0x02, 0x00, 0x00, 0xA0, 0x58, 0x00, 0x00, 0x02, 0x00,
		0x06, 0x01, 0x77,
		p3frame.EOR,
	}
	return p3frame.Escape(unesc)
}

func TestAnonymize_RemapsTransponders_DeterministicByObservationOrder(t *testing.T) {
	stream := bytes.Join([][]byte{
		buildPassing(t, 0x0052998D), // first → synthetic 1 (note: original contains 0x8D)
		buildPassing(t, 0x004AE65E), // second new → synthetic 2
		buildPassing(t, 0x0052998D), // repeat of first → synthetic 1
		buildPassing(t, 0x004AE65E), // repeat of second → synthetic 2
	}, nil)

	out, mapping, st, err := Anonymize(stream)
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if st.frames != 4 || st.passingRewritten != 4 {
		t.Errorf("stats: %+v want frames=4 rewritten=4", st)
	}
	if len(mapping) != 2 {
		t.Fatalf("expected 2 unique transponders, got %d", len(mapping))
	}
	if mapping[0].synthetic != 1 || mapping[1].synthetic != 2 {
		t.Errorf("synthetic IDs not 1,2 in order: %+v", mapping)
	}

	// Decode the output and confirm every TRANSPONDER value is now the
	// synthetic ID matching the order of the source frames.
	wantSeq := []uint32{1, 2, 1, 2}
	frames := p3frame.Split(out)
	if len(frames) != 4 {
		t.Fatalf("output frame count: got %d want 4", len(frames))
	}
	for i, f := range frames {
		body := p3frame.Unescape(f)
		var got uint32
		p3frame.WalkTLV(body, func(id, length byte, val []byte) bool {
			if id == p3frame.PassingTransponder && length == 4 {
				got = binary.LittleEndian.Uint32(val)
				return false
			}
			return true
		})
		if got != wantSeq[i] {
			t.Errorf("frame %d transponder: got 0x%08x want 0x%08x", i, got, wantSeq[i])
		}
	}
}

func TestAnonymize_LeavesOtherTLVsUnchanged(t *testing.T) {
	stream := buildPassing(t, 0x0052998D)
	out, _, _, err := Anonymize(stream)
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	frames := p3frame.Split(out)
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	body := p3frame.Unescape(frames[0])

	type seen struct {
		id  byte
		val []byte
	}
	var got []seen
	p3frame.WalkTLV(body, func(id, length byte, val []byte) bool {
		got = append(got, seen{id, append([]byte(nil), val...)})
		return true
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 TLVs, got %d", len(got))
	}
	// PASSING_NUMBER must be untouched.
	if got[0].id != 0x01 || !bytes.Equal(got[0].val, []byte{0xAA, 0xBB, 0xCC, 0xDD}) {
		t.Errorf("PASSING_NUMBER altered: %+v", got[0])
	}
	// TRANSPONDER must be the synthetic ID 1 (LE).
	if got[1].id != 0x03 || !bytes.Equal(got[1].val, []byte{0x01, 0x00, 0x00, 0x00}) {
		t.Errorf("TRANSPONDER wrong: %+v", got[1])
	}
	// HITS must be untouched.
	if got[2].id != 0x06 || !bytes.Equal(got[2].val, []byte{0x42, 0x00}) {
		t.Errorf("HITS altered: %+v", got[2])
	}
}

func TestAnonymize_NonPassingFrames_PassThroughVerbatim(t *testing.T) {
	statusFrame := buildStatus(t)
	stream := append([]byte(nil), statusFrame...)
	stream = append(stream, buildPassing(t, 0x12345678)...)

	out, mapping, st, err := Anonymize(stream)
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if st.passingRewritten != 1 {
		t.Errorf("passingRewritten: got %d want 1", st.passingRewritten)
	}
	if len(mapping) != 1 {
		t.Errorf("mapping count: got %d want 1", len(mapping))
	}

	frames := p3frame.Split(out)
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	// First frame (STATUS) must be byte-identical to the input.
	if !bytes.Equal(frames[0], statusFrame) {
		t.Errorf("STATUS frame altered:\n got %x\nwant %x", frames[0], statusFrame)
	}
}

func TestAnonymize_EmptyInput(t *testing.T) {
	out, mapping, st, err := Anonymize(nil)
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if len(out) != 0 || len(mapping) != 0 || st.frames != 0 || st.passingRewritten != 0 {
		t.Errorf("expected zero-output/zero-mapping; got out=%d map=%d stats=%+v",
			len(out), len(mapping), st)
	}
}
