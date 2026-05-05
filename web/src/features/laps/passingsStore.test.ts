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

function passing(transponder: number, passingNumber: number): ParseResult {
  return {
    kind: 'passing',
    record: {
      passingNumber,
      transponder,
      rtcTimeUs: BigInt(passingNumber * 1_000_000),
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

    expect(store.getSnapshot().passings.map((record) => record.passingNumber)).toEqual([10]);
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

    expect(store.getSnapshot().passings.map((record) => record.passingNumber)).toEqual([4, 3, 2]);
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
});
