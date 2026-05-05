// Command analyze is a one-off helper to inspect a captured AMB P3 .bin file.
// Used to verify what was actually recorded after the 2026-05-05 incident.
//
// Not part of the production gateway; safe to delete after the investigation.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
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

	// Split at 0x8F 0x8E boundaries (record separator).
	// Then verify each piece starts with 0x8E and ends with 0x8F.
	frames := splitFrames(data)
	fmt.Printf("frames found: %d\n", len(frames))

	torCounts := map[uint16]int{}
	totalBody := 0
	var firstFrame, lastFrame int = -1, -1

	for i, f := range frames {
		body := unescape(f)
		if len(body) < 10 {
			fmt.Printf("frame %d: too short (%d bytes)\n", i, len(body))
			continue
		}
		ver := body[1]
		flen := uint16(body[2]) | uint16(body[3])<<8
		crc := uint16(body[4]) | uint16(body[5])<<8
		flags := uint16(body[6]) | uint16(body[7])<<8
		tor := uint16(body[8]) | uint16(body[9])<<8
		torCounts[tor]++
		totalBody += len(body)
		if firstFrame == -1 {
			firstFrame = i
		}
		lastFrame = i
		_ = ver
		_ = flen
		_ = crc
		_ = flags
		if i < 3 || i >= len(frames)-2 {
			fmt.Printf("frame %d: ver=0x%02x flen=%d crc=0x%04x flags=0x%04x TOR=0x%04x bodylen=%d\n",
				i, ver, flen, crc, flags, tor, len(body))
			fmt.Printf("  hex: %s\n", hex.EncodeToString(body))
		}
	}
	_ = firstFrame
	_ = lastFrame

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
		body := unescape(f)
		if len(body) < 10 {
			continue
		}
		tor := uint16(body[8]) | uint16(body[9])<<8
		if tor != 0x0001 {
			continue
		}
		passingIdx++
		fmt.Printf("  PASSING #%d:\n", passingIdx)
		decodeTLV(body[10 : len(body)-1])
	}
}

func decodeTLV(b []byte) {
	for i := 0; i < len(b); {
		if i+2 > len(b) {
			break
		}
		id := b[i]
		flen := int(b[i+1])
		if i+2+flen > len(b) {
			break
		}
		val := b[i+2 : i+2+flen]
		fmt.Printf("    id=0x%02x len=%d val=%s   %s\n",
			id, flen, hex.EncodeToString(val), passingFieldName(id))
		i += 2 + flen
	}
}

func passingFieldName(id byte) string {
	switch id {
	case 0x01:
		return "PASSING_NUMBER"
	case 0x03:
		return "TRANSPONDER"
	case 0x04:
		return "RTC_TIME"
	case 0x05:
		return "STRENGTH"
	case 0x06:
		return "HITS"
	case 0x08:
		return "FLAGS"
	case 0x0A:
		return "TRAN_CODE"
	case 0x10:
		return "UTC_TIME"
	case 0x81:
		return "DECODER_ID (general)"
	}
	return ""
}

func splitFrames(data []byte) [][]byte {
	var frames [][]byte
	start := -1
	for i := 0; i < len(data); i++ {
		b := data[i]
		if start == -1 {
			if b == 0x8E {
				start = i
			}
			continue
		}
		// inside a frame
		if b == 0x8F {
			frames = append(frames, data[start:i+1])
			start = -1
		}
	}
	return frames
}

func unescape(frame []byte) []byte {
	if len(frame) < 2 {
		return frame
	}
	// frame includes SOR at [0] and EOR at [last]. Inner is everything between.
	inner := frame[1 : len(frame)-1]
	out := make([]byte, 0, len(inner)+2)
	out = append(out, 0x8E)
	escape := false
	for _, b := range inner {
		if escape {
			out = append(out, b-0x20)
			escape = false
			continue
		}
		if b == 0x8D {
			escape = true
			continue
		}
		out = append(out, b)
	}
	out = append(out, 0x8F)
	return out
}

func torName(tor uint16) string {
	switch tor {
	case 0x0000:
		return "RESET"
	case 0x0001:
		return "PASSING ★"
	case 0x0002:
		return "STATUS"
	case 0x0003:
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
