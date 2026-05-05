/**
 * Fixed 10-byte header parsing (docs/protocol-p3.md §3).
 *
 * Layout (from the start of the unescaped frame):
 *   off 0: SOR (0x8E)
 *   off 1: Version (uint8)
 *   off 2: Frame Length (uint16 LE) — unescaped total length, SOR/EOR included
 *   off 4: CRC (uint16 LE)         — not validated; spec §6 / §9 #2 open
 *   off 6: Flags (uint16 LE)
 *   off 8: TOR (uint16 LE)         — Type Of Record; see ./records
 */

export const HEADER_SIZE = 10;

export interface FrameHeader {
  /** Protocol version byte (always 0x02 in observed captures). */
  readonly version: number;
  /**
   * Total unescaped frame length in bytes, including SOR and EOR.
   * Decoders should check this against the unescaped buffer length.
   * (docs/protocol-p3.md §9 #1)
   */
  readonly frameLength: number;
  /** CRC field. Not validated by this library (spec §6 / §9 #2). */
  readonly crc: number;
  /** Flags (uint16 LE). All-zero in 2026-05-05 capture. */
  readonly flags: number;
  /** Type Of Record. Use to dispatch to record-type-specific parsing. */
  readonly tor: number;
}

/**
 * Parse the fixed header from an unescaped frame.
 *
 * @returns null if the frame is shorter than HEADER_SIZE.
 */
export function parseHeader(unesc: Uint8Array): FrameHeader | null {
  if (unesc.length < HEADER_SIZE) {
    return null;
  }
  const view = new DataView(unesc.buffer, unesc.byteOffset, unesc.byteLength);
  return {
    version: view.getUint8(1),
    frameLength: view.getUint16(2, true),
    crc: view.getUint16(4, true),
    flags: view.getUint16(6, true),
    tor: view.getUint16(8, true),
  };
}
