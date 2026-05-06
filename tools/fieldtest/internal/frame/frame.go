// Package frame builds wire-encoded P3 PASSING / STATUS frames.
//
// It intentionally duplicates a tiny slice of gateway/internal/p3frame so the
// fieldtest tools can stay outside the gateway module without exposing
// internal/. See docs/protocol-p3.md §2-§7 for the format being emitted.
package frame

import "encoding/binary"

const (
	SOR = 0x8E
	EOR = 0x8F
	ESC = 0x8D

	headerSize = 10

	torPassing = 0x0001
	torStatus  = 0x0002

	idPassingNumber = 0x01
	idTransponder   = 0x03
	idRTCTime       = 0x04
	idStrength      = 0x05
	idHits          = 0x06
	idFlags         = 0x08

	idStatusNoise        = 0x01
	idStatusGPS          = 0x06
	idStatusTemperature  = 0x07
	idStatusInputVoltage = 0x0C

	idDecoderID = 0x81
)

// PassingArgs collects the fields required to emit a single PASSING record.
type PassingArgs struct {
	PassingNumber uint32
	Transponder   uint32
	RTCTimeUs     uint64
	Strength      uint16
	Hits          uint16
	Flags         uint16
	DecoderID     uint32
}

// BuildPassing returns an escape-encoded PASSING frame ready for the wire.
func BuildPassing(a PassingArgs) []byte {
	body := make([]byte, 0, 40)
	body = appendTLV32(body, idPassingNumber, a.PassingNumber)
	body = appendTLV32(body, idTransponder, a.Transponder)
	body = appendTLV64(body, idRTCTime, a.RTCTimeUs)
	body = appendTLV16(body, idStrength, a.Strength)
	body = appendTLV16(body, idHits, a.Hits)
	body = appendTLV16(body, idFlags, a.Flags)
	body = appendTLV32(body, idDecoderID, a.DecoderID)
	return finalize(torPassing, body)
}

// StatusArgs collects the fields required to emit a single STATUS record.
type StatusArgs struct {
	Noise        uint16
	Temperature  uint16
	InputVoltage uint8
	GPS          uint8
	DecoderID    uint32
}

// BuildStatus returns an escape-encoded STATUS frame ready for the wire.
func BuildStatus(a StatusArgs) []byte {
	body := make([]byte, 0, 21)
	body = appendTLV16(body, idStatusNoise, a.Noise)
	body = appendTLV16(body, idStatusTemperature, a.Temperature)
	body = append(body, idStatusInputVoltage, 1, a.InputVoltage)
	body = append(body, idStatusGPS, 1, a.GPS)
	body = appendTLV32(body, idDecoderID, a.DecoderID)
	return finalize(torStatus, body)
}

func finalize(tor uint16, body []byte) []byte {
	totalLen := headerSize + len(body) + 1 // +1 for trailing EOR
	unesc := make([]byte, totalLen)
	unesc[0] = SOR
	unesc[1] = 0x02 // Version (matches captured fixtures)
	binary.LittleEndian.PutUint16(unesc[2:4], uint16(totalLen))
	// CRC, Flags left zero — gateway scope skips CRC verification (docs §6).
	binary.LittleEndian.PutUint16(unesc[8:10], tor)
	copy(unesc[headerSize:], body)
	unesc[len(unesc)-1] = EOR
	return escape(unesc)
}

func escape(unesc []byte) []byte {
	if len(unesc) < 2 {
		out := make([]byte, len(unesc))
		copy(out, unesc)
		return out
	}
	out := make([]byte, 0, len(unesc)+8)
	out = append(out, unesc[0])
	for _, b := range unesc[1 : len(unesc)-1] {
		switch b {
		case ESC, SOR, EOR:
			out = append(out, ESC, b+0x20)
		default:
			out = append(out, b)
		}
	}
	out = append(out, unesc[len(unesc)-1])
	return out
}

func appendTLV16(body []byte, id byte, v uint16) []byte {
	body = append(body, id, 2)
	return binary.LittleEndian.AppendUint16(body, v)
}

func appendTLV32(body []byte, id byte, v uint32) []byte {
	body = append(body, id, 4)
	return binary.LittleEndian.AppendUint32(body, v)
}

func appendTLV64(body []byte, id byte, v uint64) []byte {
	body = append(body, id, 8)
	return binary.LittleEndian.AppendUint64(body, v)
}
