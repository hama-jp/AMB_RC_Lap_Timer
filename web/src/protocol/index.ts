/**
 * Public surface of the AMB P3 protocol parser.
 *
 * This is the only module SPA / consumer code should import from. The
 * sub-modules (`frame.ts`, `escape.ts`, `header.ts`, `tlv.ts`, `records.ts`,
 * `decoder.ts`) may change shape; the names re-exported here are stable.
 */
export { createDecoder } from './decoder.js';
export type { Decoder, DecoderOptions } from './decoder.js';
export {
  TOR,
  type ParseResult,
  type PassingRecord,
  type StatusRecord,
  type MalformedReason,
} from './records.js';
