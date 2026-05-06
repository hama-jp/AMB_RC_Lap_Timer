import '@testing-library/jest-dom/vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { SETTINGS_STORAGE_KEYS } from '../features/settings/settingsStore';
import type { SpeechController } from '../features/speech/speechController';
import type { WsClient, WsMessage, WsMessageHandler, WsStateHandler } from '../transport/wsClient';
import { buildPassingFrame } from '../../tests/protocol/synthetic';
import { App } from './App';

class FakeWsClient implements WsClient {
  readonly messageHandlers = new Set<WsMessageHandler>();
  readonly stateHandlers = new Set<WsStateHandler>();
  connectCount = 0;

  connect(): void {
    this.connectCount += 1;
  }

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

  emitMessage(message: WsMessage): void {
    for (const handler of [...this.messageHandlers]) {
      handler(message);
    }
  }
}

function createSpeechController(): SpeechController & { readonly speak: ReturnType<typeof vi.fn> } {
  let unlocked = false;
  const speak = vi.fn();
  return {
    isSupported: () => true,
    speak,
    cancel: vi.fn(),
    unlock: vi.fn(() => {
      unlocked = true;
    }),
    isUnlocked: () => unlocked,
  };
}

describe('App integration', () => {
  beforeEach(() => {
    window.location.hash = '#/';
    window.localStorage.clear();
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ version: 'test-version' }),
        }),
      ),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders PASSING frames from the shared websocket client', async () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    const wsClient = new FakeWsClient();
    render(<App wsClient={wsClient} />);
    const passingFrame = buildPassingFrame({
      passingNumber: 42,
      transponder: 1,
      rtcTimeUs: 1_234_567n,
      strength: 88,
      hits: 9,
      flags: 0,
    });

    await screen.findByText('gateway version:');
    act(() => {
      wsClient.emitMessage(toArrayBuffer(passingFrame));
    });

    expect(wsClient.connectCount).toBe(1);
    expect(screen.getByText('42')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
    expect(screen.getByText('1.234567 s')).toBeInTheDocument();
    expect(screen.getByText('88')).toBeInTheDocument();
    expect(screen.getByText('9')).toBeInTheDocument();
    expect(screen.getByText('test-version')).toBeInTheDocument();
  });

  it('speaks lap time after speech unlock', async () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');
    const wsClient = new FakeWsClient();
    const speechController = createSpeechController();
    render(<App speechController={speechController} wsClient={wsClient} />);

    await screen.findByText('gateway version:');
    fireEvent.click(screen.getByRole('button', { name: '🔊 読み上げを有効化' }));

    act(() => {
      wsClient.emitMessage(
        toArrayBuffer(
          buildPassingFrame({
            passingNumber: 1,
            transponder: 1,
            rtcTimeUs: 10_000_000n,
            strength: 88,
            hits: 9,
            flags: 0,
          }),
        ),
      );
      wsClient.emitMessage(
        toArrayBuffer(
          buildPassingFrame({
            passingNumber: 2,
            transponder: 1,
            rtcTimeUs: 31_789_000n,
            strength: 88,
            hits: 9,
            flags: 0,
          }),
        ),
      );
    });

    expect(speechController.speak).toHaveBeenCalledWith('21.789秒');
  });
});

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}
