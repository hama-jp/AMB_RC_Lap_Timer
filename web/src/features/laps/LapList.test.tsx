import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { WsClient, WsMessageHandler, WsStateHandler } from '../../transport/wsClient';
import { SETTINGS_STORAGE_KEYS } from '../settings/settingsStore';
import { LapList } from './LapList';

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
}

describe('LapList', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('asks for a transponder when unset', () => {
    render(<LapList wsClient={new FakeWsClient()} />);

    expect(
      screen.getByText('設定画面で対象トランスポンダーを入力してください。'),
    ).toBeInTheDocument();
  });

  it('shows an empty waiting state when transponder is set', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '1');

    render(<LapList wsClient={new FakeWsClient()} />);

    expect(screen.getByText('トランスポンダー 1 の PASSING を待機中です。')).toBeInTheDocument();
  });
});
