// Command analyze is a one-off helper to inspect a captured AMB P3 .bin file.
// Used to verify what was actually recorded after the 2026-05-05 incident
// and as a sanity check after running cmd/anonymize.
//
// Not part of the production gateway; safe to delete after the investigation.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/p3frame"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: analyze <bin-path>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("file size: %d bytes\n\n", len(data))

	frames := p3frame.Split(data)
	fmt.Printf("frames found: %d\n", len(frames))

	torCounts := map[uint16]int{}

	for i, f := range frames {
		body := p3frame.Unescape(f)
		h, ok := p3frame.ParseHeader(body)
		if !ok {
			fmt.Printf("frame %d: too short (%d bytes)\n", i, len(body))
			continue
		}
		torCounts[h.TOR]++
		if i < 3 || i >= len(frames)-2 {
			fmt.Printf("frame %d: ver=0x%02x flen=%d crc=0x%04x flags=0x%04x TOR=0x%04x bodylen=%d\n",
				i, h.Version, h.FrameLength, h.CRC, h.Flags, h.TOR, len(body))
			fmt.Printf("  hex: %s\n", hex.EncodeToString(body))
		}
	}

	fmt.Println()
	fmt.Println("TOR distribution:")
	for tor, n := range torCounts {
		fmt.Printf("  TOR 0x%04x : %d frames    %s\n", tor, n, torName(tor))
	}

	// Decode every PASSING (TOR=0x0001) so we can confirm distinct
	// transponders and lap timestamps actually got captured.
	fmt.Println()
	fmt.Println("PASSING records:")
	passingIdx := 0
	for _, f := range frames {
		body := p3frame.Unescape(f)
		h, ok := p3frame.ParseHeader(body)
		if !ok || h.TOR != p3frame.TORPassing {
			continue
		}
		passingIdx++
		fmt.Printf("  PASSING #%d:\n", passingIdx)
		p3frame.WalkTLV(body, func(id, flen byte, val []byte) bool {
			fmt.Printf("    id=0x%02x len=%d val=%s   %s\n",
				id, flen, hex.EncodeToString(val), passingFieldName(id))
			return true
		})
	}
}

func torName(tor uint16) string {
	switch tor {
	case 0x0000:
		return "RESET"
	case p3frame.TORPassing:
		return "PASSING ★"
	case p3frame.TORStatus:
		return "STATUS"
	case p3frame.TORVersion:
		return "VERSION"
	case 0x0004:
		return "RESEND"
	case 0x0005:
		return "CLEAR_PASSING"
	case 0x0013:
		return "SERVER_SETTINGS"
	case 0x0015:
		return "SESSION"
	case 0x0016:
		return "NETWORK_SETTINGS"
	case 0x0018:
		return "WATCHDOG"
	case 0x0020:
		return "PING"
	case 0x0024:
		return "GET_TIME"
	case 0x0028:
		return "GENERAL_SETTINGS"
	case 0x002D:
		return "SIGNALS"
	case 0x002F:
		return "LOOP_TRIGGER"
	case 0x0030:
		return "GPS_INFO"
	case 0x0045:
		return "FIRST_CONTACT"
	case 0x004A:
		return "TIMELINE"
	case 0xFFFF:
		return "ERROR"
	}
	return "(undocumented)"
}

func passingFieldName(id byte) string {
	switch id {
	case p3frame.PassingPassingNumber:
		return "PASSING_NUMBER"
	case p3frame.PassingTransponder:
		return "TRANSPONDER"
	case p3frame.PassingRTCTime:
		return "RTC_TIME"
	case p3frame.PassingStrength:
		return "STRENGTH"
	case p3frame.PassingHits:
		return "HITS"
	case p3frame.PassingFlags:
		return "FLAGS"
	case 0x0A:
		return "TRAN_CODE"
	case 0x10:
		return "UTC_TIME"
	case p3frame.GeneralDecoderID:
		return "DECODER_ID (general)"
	}
	return ""
}
