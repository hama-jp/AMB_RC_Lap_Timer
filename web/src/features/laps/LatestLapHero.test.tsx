import '@testing-library/jest-dom/vitest';
import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';

import { buildPassingFrame } from '../../../tests/protocol/synthetic';
import type { WsClient, WsMessageHandler, WsStateHandler } from '../../transport/wsClient';
import { SETTINGS_STORAGE_KEYS } from '../settings/settingsStore';
import { LatestLapHero } from './LatestLapHero';
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

describe('LatestLapHero', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('renders nothing when transponder is unset', () => {
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });

    const { container } = render(<LatestLapHero store={store} />);

    expect(container).toBeEmptyDOMElement();
  });

  it('shows the placeholder when transponder is set but no laps yet', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });

    render(<LatestLapHero store={store} />);

    expect(screen.getByText('PASSING を待機中')).toBeInTheDocument();
    expect(screen.getByText('— — —')).toBeInTheDocument();
  });

  it('shows latest lap time and Lap # after the first lap completes', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });
    store.start();
    render(<LatestLapHero store={store} />);

    act(() => {
      wsClient.emitPassing(10_000_000n, 1);
      wsClient.emitPassing(31_789_000n, 2);
    });

    expect(screen.getByText('21.789')).toBeInTheDocument();
    expect(screen.getByText('Lap #2')).toBeInTheDocument();
    // First completed lap is also the best lap so far.
    expect(screen.getByText('Best!')).toBeInTheDocument();
  });

  it('shows positive delta when the latest lap is slower than the best', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });
    store.start();
    render(<LatestLapHero store={store} />);

    act(() => {
      wsClient.emitPassing(0n, 1);
      wsClient.emitPassing(21_000_000n, 2); // 21.000 — best
      wsClient.emitPassing(43_500_000n, 3); // 22.500 → +1.500
    });

    expect(screen.getByText('22.500')).toBeInTheDocument();
    expect(screen.getByText('+1.500')).toBeInTheDocument();
  });

  it('shows negative delta when the latest lap beats the previous best', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    const wsClient = new FakeWsClient();
    const store = createPassingsStore({ wsClient });
    store.start();
    render(<LatestLapHero store={store} />);

    act(() => {
      wsClient.emitPassing(0n, 1);
      wsClient.emitPassing(22_000_000n, 2); // 22.000 — first best
      wsClient.emitPassing(43_500_000n, 3); // 21.500 — new best, but delta to NEW best is 0
    });

    expect(screen.getByText('21.500')).toBeInTheDocument();
    expect(screen.getByText('Best!')).toBeInTheDocument();
  });
});

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}
