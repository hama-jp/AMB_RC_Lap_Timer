import '@testing-library/jest-dom/vitest';
import { act, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { WsClient, WsMessageHandler, WsStateHandler } from '../../transport/wsClient';
import { buildPassingFrame } from '../../../tests/protocol/synthetic';
import { SETTINGS_STORAGE_KEYS } from '../settings/settingsStore';
import { LapList } from './LapList';
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

  emitPassing(rtcTimeUs: bigint, passingNumber: number): void {
    const frame = buildPassingFrame({
      passingNumber,
      transponder: 1,
      rtcTimeUs,
      strength: 120,
      hits: 7,
      flags: 0,
    });
    for (const handler of [...this.messageHandlers]) {
      handler(toArrayBuffer(frame));
    }
  }
}

describe('LapList', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('asks for a transponder when unset', () => {
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });

    render(<LapList store={store} wsClient={wsClient} />);

    expect(
      screen.getByText('設定画面で対象トランスポンダーを入力してください。'),
    ).toBeInTheDocument();
  });

  it('shows an empty waiting state when transponder is set', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });

    render(<LapList store={store} wsClient={wsClient} />);

    expect(screen.getByText('トランスポンダー 1 の PASSING を待機中です。')).toBeInTheDocument();
  });

  it('shows dash for the first lap and milliseconds for subsequent laps', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });
    store.start();
    render(<LapList store={store} wsClient={wsClient} />);

    act(() => {
      wsClient.emitPassing(10_000_000n, 1);
      wsClient.emitPassing(31_789_000n, 2);
    });

    expect(screen.getByText('Lap')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
    expect(screen.getByText('21.789')).toBeInTheDocument();
  });
});

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}
