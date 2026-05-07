import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createWsClient,
  type WsClientOptions,
  type WsMessage,
  type WsSocket,
  type WsState,
} from './wsClient.js';

class FakeWebSocket implements WsSocket {
  static instances: FakeWebSocket[] = [];

  binaryType: BinaryType = 'blob';
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<unknown>) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  closeCount = 0;

  constructor(readonly url: string) {
    FakeWebSocket.instances.push(this);
  }

  open(): void {
    this.onopen?.({} as Event);
  }

  receive(data: WsMessage): void {
    this.onmessage?.({ data } as MessageEvent<unknown>);
  }

  serverClose(): void {
    this.onclose?.({} as CloseEvent);
  }

  error(): void {
    this.onerror?.({} as Event);
  }

  close(): void {
    this.closeCount += 1;
    this.onclose?.({} as CloseEvent);
  }
}

function createTestClient(options: WsClientOptions = {}) {
  return createWsClient({
    WebSocket: FakeWebSocket,
    random: () => 0.5,
    ...options,
  });
}

function statesOf(client: ReturnType<typeof createTestClient>): WsState[] {
  const states: WsState[] = [];
  client.onStateChange((state) => states.push(state));
  return states;
}

function lastInstance(): FakeWebSocket {
  const socket = FakeWebSocket.instances.at(-1);
  if (socket === undefined) {
    throw new Error('FakeWebSocket instance was not created');
  }
  return socket;
}

describe('createWsClient', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('connects to /ws by default and configures binary arraybuffer delivery', () => {
    const client = createTestClient();
    const states = statesOf(client);

    client.connect();
    const socket = lastInstance();

    expect(socket.url).toBe('/ws');
    expect(socket.binaryType).toBe('arraybuffer');
    expect(states).toEqual([{ kind: 'connecting' }]);

    socket.open();

    expect(states).toEqual([{ kind: 'connecting' }, { kind: 'connected' }]);
  });

  it('resolves relative URLs to websocket URLs in browser environments', () => {
    vi.stubGlobal('location', new URL('https://timer.local:8080/race'));

    const client = createTestClient({ url: '/ws' });

    client.connect();

    expect(lastInstance().url).toBe('wss://timer.local:8080/ws');
  });

  it('notifies text and binary messages without JSON parsing', () => {
    const client = createTestClient();
    const messages: WsMessage[] = [];
    client.onMessage((message) => messages.push(message));
    client.connect();
    const socket = lastInstance();
    const binary = new ArrayBuffer(3);

    socket.receive('{"kind":"status"}');
    socket.receive(binary);

    expect(messages).toEqual(['{"kind":"status"}', binary]);
  });

  it('removes message and state subscriptions with unsubscribe', () => {
    const client = createTestClient();
    const states: WsState[] = [];
    const messages: WsMessage[] = [];
    const unsubscribeState = client.onStateChange((state) => states.push(state));
    const unsubscribeMessage = client.onMessage((message) => messages.push(message));

    client.connect();
    const socket = lastInstance();
    socket.receive('first');
    unsubscribeState();
    unsubscribeMessage();
    socket.open();
    socket.receive('second');

    expect(states).toEqual([{ kind: 'connecting' }]);
    expect(messages).toEqual(['first']);
  });

  it('reconnects with exponential backoff and resets attempts after a successful open', () => {
    const client = createTestClient({
      initialBackoffMs: 1_000,
      maxBackoffMs: 30_000,
      jitterRatio: 0,
    });
    const states = statesOf(client);

    client.connect();
    lastInstance().serverClose();

    expect(states).toEqual([
      { kind: 'connecting' },
      { kind: 'reconnecting', nextAttemptInMs: 1_000, attempt: 0 },
    ]);
    expect(FakeWebSocket.instances).toHaveLength(1);

    vi.advanceTimersByTime(999);
    expect(FakeWebSocket.instances).toHaveLength(1);

    vi.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(2);
    lastInstance().serverClose();

    expect(states.at(-1)).toEqual({
      kind: 'reconnecting',
      nextAttemptInMs: 2_000,
      attempt: 1,
    });

    vi.advanceTimersByTime(2_000);
    const successfulSocket = lastInstance();
    successfulSocket.open();
    successfulSocket.serverClose();

    expect(states.at(-1)).toEqual({
      kind: 'reconnecting',
      nextAttemptInMs: 1_000,
      attempt: 0,
    });
  });

  it('does not reconnect after manual disconnect during a pending retry', () => {
    const client = createTestClient({
      initialBackoffMs: 1_000,
      jitterRatio: 0,
    });
    const states = statesOf(client);

    client.connect();
    lastInstance().serverClose();
    client.disconnect();
    vi.advanceTimersByTime(1_000);

    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(states.at(-1)).toEqual({ kind: 'disconnected' });

    client.connect();
    lastInstance().serverClose();

    expect(FakeWebSocket.instances).toHaveLength(2);
    expect(states.at(-1)).toEqual({
      kind: 'reconnecting',
      nextAttemptInMs: 1_000,
      attempt: 0,
    });
  });

  it('closes the active socket on manual disconnect without scheduling reconnect', () => {
    const client = createTestClient({
      initialBackoffMs: 1_000,
      jitterRatio: 0,
    });
    const states = statesOf(client);

    client.connect();
    const socket = lastInstance();
    socket.open();
    client.disconnect();
    vi.advanceTimersByTime(1_000);

    expect(socket.closeCount).toBe(1);
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(states.at(-1)).toEqual({ kind: 'disconnected' });
  });

  it('saturates exponential backoff at max before applying jitter', () => {
    const client = createTestClient({
      initialBackoffMs: 1_000,
      maxBackoffMs: 3_000,
      jitterRatio: 0,
    });
    const states = statesOf(client);
    const expectedDelays = [1_000, 2_000, 3_000, 3_000];

    client.connect();
    for (let i = 0; i < expectedDelays.length; i += 1) {
      lastInstance().serverClose();
      const state = states.at(-1);

      expect(state?.kind).toBe('reconnecting');
      if (state?.kind !== 'reconnecting') {
        throw new Error('expected reconnecting state');
      }
      expect(state.nextAttemptInMs).toBe(expectedDelays[i]);
      expect(state.attempt).toBe(i);

      vi.advanceTimersByTime(state.nextAttemptInMs);
    }
  });

  it('keeps jittered delay inside the configured lower and upper bounds', () => {
    const lowClient = createTestClient({
      initialBackoffMs: 30_000,
      maxBackoffMs: 30_000,
      jitterRatio: 0.2,
      random: () => 0,
    });
    const lowStates = statesOf(lowClient);
    lowClient.connect();
    lastInstance().serverClose();

    FakeWebSocket.instances = [];
    vi.clearAllTimers();

    const highClient = createTestClient({
      initialBackoffMs: 30_000,
      maxBackoffMs: 30_000,
      jitterRatio: 0.2,
      random: () => 1,
    });
    const highStates = statesOf(highClient);
    highClient.connect();
    lastInstance().serverClose();

    expect(lowStates.at(-1)).toEqual({
      kind: 'reconnecting',
      nextAttemptInMs: 24_000,
      attempt: 0,
    });
    expect(highStates.at(-1)).toEqual({
      kind: 'reconnecting',
      nextAttemptInMs: 36_000,
      attempt: 0,
    });
  });

  it('closes and reconnects after websocket error events', () => {
    const client = createTestClient({ jitterRatio: 0 });
    const states = statesOf(client);

    client.connect();
    const socket = lastInstance();
    socket.error();

    expect(socket.closeCount).toBe(1);
    expect(states.at(-1)).toEqual({
      kind: 'reconnecting',
      nextAttemptInMs: 1_000,
      attempt: 0,
    });
  });

  // Issue #100 — iPhone Safari can silently kill the socket while the page
  // is hidden (locked). On return-to-visible the WS handle "looks open" but
  // is actually dead and onclose never fires, so the user is stuck on the
  // empty "PASSING を待機中" screen until they re-trigger anything that
  // pokes the WS. We force-reconnect on visible-after-long-hidden.
  describe('visibility reconnect (#100)', () => {
    class FakeVisibility {
      private hiddenFlag = false;
      private nowMs = 0;
      private readonly handlers = new Set<() => void>();

      isHidden(): boolean {
        return this.hiddenFlag;
      }
      subscribe(handler: () => void): () => void {
        this.handlers.add(handler);
        return () => {
          this.handlers.delete(handler);
        };
      }
      now(): number {
        return this.nowMs;
      }
      // Test hooks:
      hide(): void {
        this.hiddenFlag = true;
        this.fire();
      }
      show(): void {
        this.hiddenFlag = false;
        this.fire();
      }
      advance(ms: number): void {
        this.nowMs += ms;
      }
      get subscriberCount(): number {
        return this.handlers.size;
      }
      private fire(): void {
        for (const h of [...this.handlers]) {
          h();
        }
      }
    }

    it('force-reconnects when visible after a long hidden window', () => {
      const visibility = new FakeVisibility();
      const client = createTestClient({
        visibility,
        visibilityReconnectThresholdMs: 5_000,
        jitterRatio: 0,
      });
      client.connect();
      const before = lastInstance();
      before.open();
      const states: WsState[] = [];
      client.onStateChange((s) => states.push(s));

      // 30 sec hidden — well past the threshold.
      visibility.hide();
      visibility.advance(30_000);
      visibility.show();

      // The previous socket was force-closed and a fresh one opened.
      expect(before.closeCount).toBe(1);
      expect(FakeWebSocket.instances).toHaveLength(2);
      // First state we observe after subscribe is the new connecting.
      expect(states[0]).toEqual({ kind: 'connecting' });
    });

    it('does NOT reconnect on a quick tab flip below the threshold', () => {
      const visibility = new FakeVisibility();
      const client = createTestClient({
        visibility,
        visibilityReconnectThresholdMs: 5_000,
        jitterRatio: 0,
      });
      client.connect();
      const before = lastInstance();
      before.open();

      visibility.hide();
      visibility.advance(1_000); // only 1s hidden
      visibility.show();

      expect(before.closeCount).toBe(0);
      expect(FakeWebSocket.instances).toHaveLength(1);
    });

    it('ignores visibility events that arrive before connect()', () => {
      const visibility = new FakeVisibility();
      createTestClient({
        visibility,
        visibilityReconnectThresholdMs: 5_000,
      });
      // Subscribe happens on connect(); since we never connected, no
      // socket should be created by visibility traffic.
      visibility.hide();
      visibility.advance(60_000);
      visibility.show();

      expect(FakeWebSocket.instances).toHaveLength(0);
    });

    it('unsubscribes from visibility on disconnect()', () => {
      const visibility = new FakeVisibility();
      const client = createTestClient({
        visibility,
        visibilityReconnectThresholdMs: 5_000,
      });
      client.connect();
      expect(visibility.subscriberCount).toBe(1);

      client.disconnect();
      expect(visibility.subscriberCount).toBe(0);

      // Late visibility events after disconnect must not spawn sockets.
      visibility.hide();
      visibility.advance(30_000);
      visibility.show();
      expect(FakeWebSocket.instances).toHaveLength(1); // only the original
    });

    it('does not reconnect when no hidden→visible transition happened', () => {
      const visibility = new FakeVisibility();
      const client = createTestClient({
        visibility,
        visibilityReconnectThresholdMs: 5_000,
      });
      client.connect();
      const before = lastInstance();
      before.open();

      // Fire a visibilitychange while still visible — no-op.
      visibility.show();

      expect(before.closeCount).toBe(0);
      expect(FakeWebSocket.instances).toHaveLength(1);
    });

    it('handles multiple hide/show cycles independently', () => {
      const visibility = new FakeVisibility();
      const client = createTestClient({
        visibility,
        visibilityReconnectThresholdMs: 5_000,
      });
      client.connect();
      const first = lastInstance();
      first.open();

      // Cycle 1: long hidden → reconnect
      visibility.hide();
      visibility.advance(10_000);
      visibility.show();
      expect(FakeWebSocket.instances).toHaveLength(2);
      lastInstance().open();

      // Cycle 2: short hidden → no reconnect
      visibility.hide();
      visibility.advance(2_000);
      visibility.show();
      expect(FakeWebSocket.instances).toHaveLength(2);

      // Cycle 3: long again → reconnect
      visibility.hide();
      visibility.advance(60_000);
      visibility.show();
      expect(FakeWebSocket.instances).toHaveLength(3);
    });

    it('opt-out: passing visibility:null disables the hook', () => {
      const client = createTestClient({ visibility: null });
      client.connect();
      // No throw, no extra sockets, no listeners. Smoke check.
      expect(FakeWebSocket.instances).toHaveLength(1);
    });
  });
});
