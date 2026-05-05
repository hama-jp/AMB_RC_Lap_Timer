import { useEffect, useState } from 'react';

import type { WsClient, WsState } from '../../transport/wsClient';
import { parseUpstreamMessage, type UpstreamState } from './upstreamState';

export interface StatusBannerProps {
  readonly wsClient: WsClient;
  readonly autoConnect?: boolean;
}

const INITIAL_WS_STATE: WsState = { kind: 'connecting' };
const INITIAL_UPSTREAM_STATE: UpstreamState = { kind: 'unknown' };

export function StatusBanner({
  wsClient,
  autoConnect = true,
}: StatusBannerProps): JSX.Element | null {
  const [wsState, setWsState] = useState<WsState>(INITIAL_WS_STATE);
  const [upstreamState, setUpstreamState] = useState<UpstreamState>(INITIAL_UPSTREAM_STATE);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [reconnectDeadlineMs, setReconnectDeadlineMs] = useState<number | null>(null);

  useEffect(() => {
    const unsubscribeState = wsClient.onStateChange((state) => {
      setWsState(state);
      if (state.kind === 'reconnecting') {
        setReconnectDeadlineMs(Date.now() + state.nextAttemptInMs);
      } else {
        setReconnectDeadlineMs(null);
      }
    });
    const unsubscribeMessage = wsClient.onMessage((message) => {
      if (typeof message !== 'string') {
        return;
      }
      const parsed = parseUpstreamMessage(message);
      if (parsed !== null) {
        setUpstreamState(parsed);
      }
    });

    if (autoConnect) {
      wsClient.connect();
    }

    return () => {
      unsubscribeMessage();
      unsubscribeState();
    };
  }, [autoConnect, wsClient]);

  useEffect(() => {
    if (wsState.kind !== 'reconnecting') {
      return undefined;
    }

    setNowMs(Date.now());
    const interval = window.setInterval(() => {
      setNowMs(Date.now());
    }, 1_000);

    return () => {
      window.clearInterval(interval);
    };
  }, [wsState.kind, reconnectDeadlineMs]);

  const banner = getBanner(wsState, upstreamState, reconnectDeadlineMs, nowMs);
  if (banner === null) {
    return null;
  }

  return (
    <div
      className={`rounded-md border px-4 py-3 text-sm font-medium shadow-lg ${banner.className}`}
      role="status"
    >
      {banner.text}
    </div>
  );
}

interface BannerView {
  readonly text: string;
  readonly className: string;
}

function getBanner(
  wsState: WsState,
  upstreamState: UpstreamState,
  reconnectDeadlineMs: number | null,
  nowMs: number,
): BannerView | null {
  if (wsState.kind === 'disconnected') {
    return {
      text: '接続が切れました',
      className: 'border-red-500/50 bg-red-500/15 text-red-100',
    };
  }

  if (wsState.kind === 'reconnecting') {
    const remainingMs =
      reconnectDeadlineMs === null ? wsState.nextAttemptInMs : reconnectDeadlineMs - nowMs;
    const remainingSeconds = Math.max(0, Math.ceil(remainingMs / 1_000));
    return {
      text: `再接続中…(残り ${remainingSeconds} 秒)`,
      className: 'border-amber-400/50 bg-amber-400/15 text-amber-100',
    };
  }

  if (wsState.kind !== 'connected') {
    return null;
  }

  if (upstreamState.kind === 'reconnecting') {
    return {
      text: '上流(AMB)に再接続中…',
      className: 'border-amber-400/50 bg-amber-400/15 text-amber-100',
    };
  }

  if (upstreamState.kind === 'finished') {
    return {
      text: 'リプレイ終了',
      className: 'border-sky-400/50 bg-sky-400/15 text-sky-100',
    };
  }

  return null;
}
