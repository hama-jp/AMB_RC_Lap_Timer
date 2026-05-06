import { describe, expect, it, vi } from 'vitest';

import type { Decoder, ParseResult } from '../../protocol';
import type { WsClient, WsMessageHandler, WsStateHandler } from '../../transport/wsClient';
import { createPassingsStore } from './passingsStore';

class FakeWsClient implements WsClient {
  readonly messageHandlers = new Set<WsMessageHandler>();
  readonly stateHandlers = new Set<WsStateHandler>();

  connect(): void {}

  disconnect(): void {}

  onMessage(handler: WsMessageHandler): () => void {
    this.messageHandlers.add(handler);
    return () => {
      this.messageHandlers.delete(handler);
    };
  }

  onStateChange(handler: WsStateHandler): () => void {
    this.stateHandlers.add(handler);
    return () => {
      this.stateHandlers.delete(handler);
    };
  }

  emitBinary(): void {
    for (const handler of [...this.messageHandlers]) {
      handler(new ArrayBuffer(1));
    }
  }

  emitReconnecting(): void {
    for (const handler of [...this.stateHandlers]) {
      handler({ kind: 'reconnecting', nextAttemptInMs: 1_000, attempt: 0 });
    }
  }
}

function createTestDecoder(results: ParseResult[]): {
  readonly decoder: Decoder;
  readonly push: ReturnType<typeof vi.fn>;
  readonly reset: ReturnType<typeof vi.fn>;
} {
  const push = vi.fn(() => results);
  const reset = vi.fn();
  return {
    decoder: { push, reset },
    push,
    reset,
  };
}

function passing(
  transponder: number,
  passingNumber: number,
  rtcTimeUs = BigInt(passingNumber * 1_000_000),
): ParseResult {
  return {
    kind: 'passing',
    record: {
      passingNumber,
      transponder,
      rtcTimeUs,
      strength: 120,
      hits: 7,
      flags: 0,
    },
  };
}

describe('passingsStore', () => {
  it('keeps matching transponder passings and drops others', () => {
    const wsClient = new FakeWsClient();
    const { decoder } = createTestDecoder([passing(1, 10), passing(2, 11)]);
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
    });

    store.start();
    wsClient.emitBinary();

    expect(store.getSnapshot().passings.map((entry) => entry.record.passingNumber)).toEqual([10]);
  });

  it('caps the ring buffer at the configured limit with newest first', () => {
    const wsClient = new FakeWsClient();
    let nextPassing = 0;
    const decoder: Decoder = {
      push: vi.fn(() => [passing(1, nextPassing++)]),
      reset: vi.fn(),
    };
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
      limit: 3,
    });

    store.start();
    for (let i = 0; i < 5; i += 1) {
      wsClient.emitBinary();
    }

    expect(store.getSnapshot().passings.map((entry) => entry.record.passingNumber)).toEqual([
      4, 3, 2,
    ]);
  });

  it('calculates lap time when inserting matching passings', () => {
    const wsClient = new FakeWsClient();
    const { decoder } = createTestDecoder([
      passing(1, 1, 10_000_000n),
      passing(1, 2, 31_789_000n),
      passing(1, 3, 53_000_000n),
    ]);
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
    });

    store.start();
    wsClient.emitBinary();

    expect(store.getSnapshot().passings.map((entry) => entry.lapTimeUs)).toEqual([
      21_211_000n,
      21_789_000n,
      null,
    ]);
  });

  it('does not decode binary frames when transponder is unset', () => {
    const wsClient = new FakeWsClient();
    const { decoder, push } = createTestDecoder([passing(1, 10)]);
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => null,
    });

    store.start();
    wsClient.emitBinary();

    expect(push).not.toHaveBeenCalled();
    expect(store.getSnapshot().passings).toEqual([]);
  });

  it('resets decoder and buffer on websocket reconnect', () => {
    const wsClient = new FakeWsClient();
    const { decoder, reset } = createTestDecoder([passing(1, 10)]);
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
    });

    store.start();
    wsClient.emitBinary();
    wsClient.emitReconnecting();

    expect(reset).toHaveBeenCalledTimes(1);
    expect(store.getSnapshot().passings).toEqual([]);
  });

  it('uses null lap time for the first passing after reset', () => {
    const wsClient = new FakeWsClient();
    const decoder: Decoder = {
      push: vi
        .fn()
        .mockReturnValueOnce([passing(1, 1, 10_000_000n), passing(1, 2, 31_789_000n)])
        .mockReturnValueOnce([passing(1, 3, 53_000_000n)]),
      reset: vi.fn(),
    };
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
    });

    store.start();
    wsClient.emitBinary();
    wsClient.emitReconnecting();
    wsClient.emitBinary();

    expect(store.getSnapshot().passings.map((entry) => entry.lapTimeUs)).toEqual([null]);
  });

  it('tracks the best (smallest) lap time and clears it on reset', () => {
    const wsClient = new FakeWsClient();
    const decoder: Decoder = {
      push: vi
        .fn()
        .mockReturnValueOnce([
          passing(1, 1, 0n),
          passing(1, 2, 22_000_000n),
          passing(1, 3, 43_500_000n),
          passing(1, 4, 65_000_000n),
        ])
        .mockReturnValueOnce([passing(1, 5, 86_500_000n)]),
      reset: vi.fn(),
    };
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
    });

    store.start();
    wsClient.emitBinary();

    // Laps observed (newest first): 21.5, 21.5, 22.0, null. Best = 21_500_000n.
    expect(store.getSnapshot().bestLapUs).toBe(21_500_000n);

    wsClient.emitReconnecting();
    expect(store.getSnapshot().bestLapUs).toBeNull();

    wsClient.emitBinary();
    // Only one lap after reset, with no previous to diff from → bestLapUs stays null.
    expect(store.getSnapshot().bestLapUs).toBeNull();
  });

  it('keeps the first occurrence of a tied best lap', () => {
    const wsClient = new FakeWsClient();
    const decoder: Decoder = {
      push: vi.fn(() => [
        passing(1, 1, 0n),
        passing(1, 2, 21_000_000n), // lap 21.000 — first best
        passing(1, 3, 42_000_000n), // lap 21.000 — tied, but first wins
      ]),
      reset: vi.fn(),
    };
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
    });

    store.start();
    wsClient.emitBinary();

    expect(store.getSnapshot().bestLapUs).toBe(21_000_000n);
    // The list keeps newest-first; passing #2 (the first best) is still in the buffer.
    const passingNumbers = store.getSnapshot().passings.map((entry) => entry.record.passingNumber);
    expect(passingNumbers).toContain(2);
  });

  it('keeps computed lap times stable after ring buffer overflow', () => {
    const wsClient = new FakeWsClient();
    let nextPassing = 0;
    const decoder: Decoder = {
      push: vi.fn(() => [passing(1, nextPassing, BigInt(nextPassing++ * 1_000_000))]),
      reset: vi.fn(),
    };
    const store = createPassingsStore({
      wsClient,
      decoder,
      loadTargetTransponder: () => 1,
      limit: 3,
    });

    store.start();
    for (let i = 0; i < 5; i += 1) {
      wsClient.emitBinary();
    }

    expect(
      store.getSnapshot().passings.map((entry) => ({
        passingNumber: entry.record.passingNumber,
        lapTimeUs: entry.lapTimeUs,
      })),
    ).toEqual([
      { passingNumber: 4, lapTimeUs: 1_000_000n },
      { passingNumber: 3, lapTimeUs: 1_000_000n },
      { passingNumber: 2, lapTimeUs: 1_000_000n },
    ]);
  });
});
