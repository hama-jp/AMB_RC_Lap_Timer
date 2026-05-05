// Command anonymize rewrites an AMB P3 .bin capture so that every
// PASSING.TRANSPONDER value is replaced by a deterministic synthetic ID
// (0x00000001, 0x00000002, ...) assigned in observation order. All other
// bytes — header fields, other TLVs, frame ordering, RTC_TIME, etc. — are
// preserved verbatim.
//
// The use case is preparing third-party capture data for inclusion under
// gateway/testdata/captured/. AMB transponder numbers are personal
// identifiers (tied to MyLaps accounts), so raw captures must NOT be
// committed; see Issue #45 and docs/test-strategy.md §7.4.1.
//
// Notes:
//
//   - CRC is NOT recalculated. docs/protocol-p3.md §6 keeps CRC validation
//     as an open item and the initial gateway scope skips it; broken CRC
//     is acceptable for fixture data.
//
//   - The header's FrameLength field is left as-is. It may not match the
//     wire-encoded length of the new frame when the original transponder
//     contained 0x8D / 0x8E / 0x8F bytes (escape encoding shortens), but
//     the parser splits at 0x8F 0x8E boundaries and does not rely on
//     FrameLength (docs/protocol-p3.md §11). This is documented in
//     docs/captured-sessions/2026-05-05.md.
//
//   - The original→synthetic mapping table is printed to stdout and is the
//     ONLY artifact that allows linking back to real transponders. By
//     contract it is never written to disk by this tool. Operators who
//     need it for cross-session correlation should keep a local note.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/p3frame"
)

func main() {
	in := flag.String("in", "", "input raw .bin path (required)")
	out := flag.String("out", "", "output anonymized .bin path (required)")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: anonymize -in <raw.bin> -out <anonymized.bin>")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if err := run(*in, *out, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(inPath, outPath string, report io.Writer) error {
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	processed, mapping, stats, err := Anonymize(raw)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outPath, processed, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	fmt.Fprintf(report, "input : %s (%d bytes, %d frames)\n", inPath, len(raw), stats.frames)
	fmt.Fprintf(report, "output: %s (%d bytes)\n", outPath, len(processed))
	fmt.Fprintf(report, "PASSING frames rewritten: %d\n", stats.passingRewritten)
	fmt.Fprintln(report)
	fmt.Fprintln(report, "TRANSPONDER mapping (original LE bytes → synthetic ID):")
	fmt.Fprintln(report, "  (do NOT commit this output; keep it locally if cross-session correlation is needed)")
	for _, m := range mapping {
		fmt.Fprintf(report, "  %08x  →  0x%08x\n", m.original, m.synthetic)
	}
	return nil
}

// MappingEntry records one observed transponder and its synthetic replacement.
// Order is the order of first observation in the input stream.
type MappingEntry struct {
	original  uint32
	synthetic uint32
}

type stats struct {
	frames           int
	passingRewritten int
}

// Anonymize splits the input stream into P3 frames, rewrites every
// PASSING.TRANSPONDER (4-byte LE) to a deterministic synthetic ID, and
// returns the re-encoded byte stream alongside the mapping table and stats.
//
// Bytes outside frame boundaries (i.e., before the first 0x8E or after the
// last 0x8F) are dropped. This is consistent with how the parser will
// consume the stream and avoids round-tripping garbage that does not belong
// to a frame.
func Anonymize(raw []byte) ([]byte, []MappingEntry, stats, error) {
	frames := p3frame.Split(raw)

	// Allocate roughly the input size; the output is usually a few bytes
	// shorter when original transponders contained 0x8D/0x8E/0x8F.
	out := make([]byte, 0, len(raw))

	type seenKey = uint32
	mappingByID := map[seenKey]uint32{}
	var mappingOrdered []MappingEntry

	var st stats
	st.frames = len(frames)

	for _, f := range frames {
		unesc := p3frame.Unescape(f)
		h, ok := p3frame.ParseHeader(unesc)
		if !ok {
			// Truncated frame — pass through verbatim.
			out = append(out, f...)
			continue
		}
		if h.TOR != p3frame.TORPassing {
			// Non-PASSING frames are unchanged; re-emit the original wire bytes.
			out = append(out, f...)
			continue
		}
		mutated := false
		p3frame.WalkTLV(unesc, func(id, length byte, val []byte) bool {
			if id != p3frame.PassingTransponder || length != 4 {
				return true
			}
			orig := binary.LittleEndian.Uint32(val)
			synth, exists := mappingByID[orig]
			if !exists {
				synth = uint32(len(mappingOrdered) + 1)
				mappingByID[orig] = synth
				mappingOrdered = append(mappingOrdered, MappingEntry{original: orig, synthetic: synth})
			}
			binary.LittleEndian.PutUint32(val, synth)
			mutated = true
			return true
		})
		if mutated {
			st.passingRewritten++
			out = append(out, p3frame.Escape(unesc)...)
		} else {
			// PASSING without a TRANSPONDER TLV — leave bytes alone.
			out = append(out, f...)
		}
	}
	return out, mappingOrdered, st, nil
}
