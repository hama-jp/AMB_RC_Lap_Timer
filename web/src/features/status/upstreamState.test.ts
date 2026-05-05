import { describe, expect, it } from 'vitest';

import { parseUpstreamMessage } from './upstreamState';

describe('parseUpstreamMessage', () => {
  it.each([['connected'], ['reconnecting'], ['finished'], ['mock'], ['replay']] as const)(
    'parses upstream %s messages',
    (status) => {
      expect(parseUpstreamMessage(JSON.stringify({ type: 'upstream', status }))).toEqual({
        kind: status,
      });
    },
  );

  it('returns null for non-json text', () => {
    expect(parseUpstreamMessage('not json')).toBeNull();
  });

  it('returns null for unrelated text frames', () => {
    expect(parseUpstreamMessage(JSON.stringify({ type: 'other', status: 'connected' }))).toBeNull();
  });

  it('falls back to unknown for unknown upstream shapes', () => {
    expect(parseUpstreamMessage(JSON.stringify({ type: 'upstream', status: 'paused' }))).toEqual({
      kind: 'unknown',
    });
    expect(parseUpstreamMessage(JSON.stringify({ type: 'upstream' }))).toEqual({
      kind: 'unknown',
    });
  });
});
