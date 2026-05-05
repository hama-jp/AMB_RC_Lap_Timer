import '@testing-library/jest-dom/vitest';
import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type {
  WsClient,
  WsMessage,
  WsMessageHandler,
  WsState,
  WsStateHandler,
} from '../../transport/wsClient';
import { StatusBanner } from './StatusBanner';

class FakeWsClient implements WsClient {
  readonly messageHandlers = new Set<WsMessageHandler>();
  readonly stateHandlers = new Set<WsStateHandler>();
  connectCount = 0;
  disconnectCount = 0;

  connect(): void {
    this.connectCount += 1;
  }

  disconnect(): void {
    this.disconnectCount += 1;
  }

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

  emitState(state: WsState): void {
    for (const handler of [...this.stateHandlers]) {
      handler(state);
    }
  }

  emitMessage(message: WsMessage): void {
    for (const handler of [...this.messageHandlers]) {
      handler(message);
    }
  }
}

describe('StatusBanner', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-05-06T00:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('connects automatically and unsubscribes on unmount', () => {
    const client = new FakeWsClient();
    const { unmount } = render(<StatusBanner wsClient={client} />);

    expect(client.connectCount).toBe(1);
    expect(client.stateHandlers.size).toBe(1);
    expect(client.messageHandlers.size).toBe(1);

    unmount();

    expect(client.stateHandlers.size).toBe(0);
    expect(client.messageHandlers.size).toBe(0);
  });

  it('shows disconnected before lower-priority states', () => {
    const client = new FakeWsClient();
    render(<StatusBanner wsClient={client} autoConnect={false} />);

    act(() => {
      client.emitMessage(JSON.stringify({ type: 'upstream', status: 'reconnecting' }));
      client.emitState({ kind: 'disconnected' });
    });

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('接続が切れました');
    expect(banner.className).toContain('red');
  });

  it('shows reconnecting countdown and updates every second', () => {
    const client = new FakeWsClient();
    render(<StatusBanner wsClient={client} autoConnect={false} />);

    act(() => {
      client.emitState({ kind: 'reconnecting', nextAttemptInMs: 2_500, attempt: 1 });
    });

    expect(screen.getByRole('status')).toHaveTextContent('再接続中…(残り 3 秒)');

    act(() => {
      vi.advanceTimersByTime(1_000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('再接続中…(残り 2 秒)');
    expect(screen.getByRole('status').className).toContain('amber');
  });

  it('shows upstream reconnecting while websocket is connected', () => {
    const client = new FakeWsClient();
    render(<StatusBanner wsClient={client} autoConnect={false} />);

    act(() => {
      client.emitState({ kind: 'connected' });
      client.emitMessage(JSON.stringify({ type: 'upstream', status: 'reconnecting' }));
    });

    expect(screen.getByRole('status')).toHaveTextContent('上流(AMB)に再接続中…');
  });

  it('shows replay finished while websocket is connected', () => {
    const client = new FakeWsClient();
    render(<StatusBanner wsClient={client} autoConnect={false} />);

    act(() => {
      client.emitState({ kind: 'connected' });
      client.emitMessage(JSON.stringify({ type: 'upstream', status: 'finished' }));
    });

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('リプレイ終了');
    expect(banner.className).toContain('sky');
  });

  it('hides banner for healthy or unknown upstream states', () => {
    const client = new FakeWsClient();
    render(<StatusBanner wsClient={client} autoConnect={false} />);

    act(() => {
      client.emitState({ kind: 'connected' });
    });
    expect(screen.queryByRole('status')).not.toBeInTheDocument();

    act(() => {
      client.emitMessage('not json');
      client.emitMessage(JSON.stringify({ type: 'upstream', status: 'connected' }));
    });
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('follows disconnected to reconnecting to connected transitions', () => {
    const client = new FakeWsClient();
    render(<StatusBanner wsClient={client} autoConnect={false} />);

    act(() => {
      client.emitState({ kind: 'disconnected' });
    });
    expect(screen.getByRole('status')).toHaveTextContent('接続が切れました');

    act(() => {
      client.emitState({ kind: 'reconnecting', nextAttemptInMs: 1_000, attempt: 0 });
    });
    expect(screen.getByRole('status')).toHaveTextContent('再接続中');

    act(() => {
      client.emitState({ kind: 'connected' });
    });
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
