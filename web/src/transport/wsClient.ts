export interface WsClient {
  connect(): void;
  disconnect(): void;
  onMessage(handler: WsMessageHandler): () => void;
  onStateChange(handler: WsStateHandler): () => void;
}

export type WsMessage = ArrayBuffer | string;
export type WsMessageHandler = (data: WsMessage) => void;
export type WsStateHandler = (state: WsState) => void;

export type WsState =
  | { kind: 'connecting' }
  | { kind: 'connected' }
  | { kind: 'reconnecting'; nextAttemptInMs: number; attempt: number }
  | { kind: 'disconnected' };

export interface WsSocket {
  binaryType: BinaryType;
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent<unknown>) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  close(): void;
}

export interface WsSocketConstructor {
  new (url: string): WsSocket;
}

export type WsTimeoutHandle = unknown;
export type WsSetTimeout = (handler: () => void, delayMs: number) => WsTimeoutHandle;
export type WsClearTimeout = (handle: WsTimeoutHandle) => void;

export interface WsClientOptions {
  readonly url?: string;
  readonly initialBackoffMs?: number;
  readonly maxBackoffMs?: number;
  readonly jitterRatio?: number;
  readonly random?: () => number;
  readonly setTimeout?: WsSetTimeout;
  readonly clearTimeout?: WsClearTimeout;
  readonly WebSocket?: WsSocketConstructor;
}

const DEFAULT_URL = '/ws';
const DEFAULT_INITIAL_BACKOFF_MS = 1_000;
const DEFAULT_MAX_BACKOFF_MS = 30_000;
const DEFAULT_JITTER_RATIO = 0.2;

export function createWsClient(opts: WsClientOptions = {}): WsClient {
  const url = resolveWsUrl(opts.url ?? DEFAULT_URL);
  const initialBackoffMs = nonNegative(opts.initialBackoffMs ?? DEFAULT_INITIAL_BACKOFF_MS);
  const maxBackoffMs = nonNegative(opts.maxBackoffMs ?? DEFAULT_MAX_BACKOFF_MS);
  const jitterRatio = nonNegative(opts.jitterRatio ?? DEFAULT_JITTER_RATIO);
  const random = opts.random ?? Math.random;
  const setTimer = opts.setTimeout ?? defaultSetTimeout;
  const clearTimer = opts.clearTimeout ?? defaultClearTimeout;
  const WebSocketCtor = resolveWebSocketCtor(opts.WebSocket);

  const messageHandlers = new Set<WsMessageHandler>();
  const stateHandlers = new Set<WsStateHandler>();

  let socket: WsSocket | null = null;
  let reconnectTimer: { readonly handle: WsTimeoutHandle } | null = null;
  let reconnectAttempt = 0;
  let generation = 0;
  let manualDisconnect = false;

  function emitState(state: WsState): void {
    for (const handler of [...stateHandlers]) {
      handler(state);
    }
  }

  function emitMessage(data: WsMessage): void {
    for (const handler of [...messageHandlers]) {
      handler(data);
    }
  }

  function clearReconnectTimer(): void {
    if (reconnectTimer === null) {
      return;
    }
    clearTimer(reconnectTimer.handle);
    reconnectTimer = null;
  }

  function openSocket(): void {
    const socketGeneration = ++generation;
    emitState({ kind: 'connecting' });

    const nextSocket = new WebSocketCtor(url);
    nextSocket.binaryType = 'arraybuffer';
    socket = nextSocket;

    nextSocket.onopen = () => {
      if (socketGeneration !== generation || socket !== nextSocket) {
        return;
      }
      reconnectAttempt = 0;
      emitState({ kind: 'connected' });
    };

    nextSocket.onmessage = (event) => {
      if (socketGeneration !== generation || socket !== nextSocket) {
        return;
      }
      if (typeof event.data === 'string' || event.data instanceof ArrayBuffer) {
        emitMessage(event.data);
      }
    };

    nextSocket.onclose = () => {
      if (socketGeneration !== generation || socket !== nextSocket) {
        return;
      }
      socket = null;
      if (manualDisconnect) {
        emitState({ kind: 'disconnected' });
        return;
      }
      scheduleReconnect();
    };

    nextSocket.onerror = () => {
      if (socketGeneration !== generation || socket !== nextSocket) {
        return;
      }
      nextSocket.close();
    };
  }

  function scheduleReconnect(): void {
    const attempt = reconnectAttempt;
    const nextAttemptInMs = nextBackoffMs(attempt, {
      initialBackoffMs,
      maxBackoffMs,
      jitterRatio,
      random,
    });
    reconnectAttempt += 1;

    emitState({ kind: 'reconnecting', nextAttemptInMs, attempt });
    reconnectTimer = {
      handle: setTimer(() => {
        reconnectTimer = null;
        if (manualDisconnect) {
          return;
        }
        openSocket();
      }, nextAttemptInMs),
    };
  }

  return {
    connect(): void {
      manualDisconnect = false;
      clearReconnectTimer();
      if (socket !== null) {
        return;
      }
      reconnectAttempt = 0;
      openSocket();
    },

    disconnect(): void {
      manualDisconnect = true;
      clearReconnectTimer();
      reconnectAttempt = 0;

      const closingSocket = socket;
      socket = null;
      generation += 1;

      if (closingSocket !== null) {
        closingSocket.close();
      }
      emitState({ kind: 'disconnected' });
    },

    onMessage(handler: WsMessageHandler): () => void {
      messageHandlers.add(handler);
      return () => {
        messageHandlers.delete(handler);
      };
    },

    onStateChange(handler: WsStateHandler): () => void {
      stateHandlers.add(handler);
      return () => {
        stateHandlers.delete(handler);
      };
    },
  };
}

interface BackoffOptions {
  readonly initialBackoffMs: number;
  readonly maxBackoffMs: number;
  readonly jitterRatio: number;
  readonly random: () => number;
}

function nextBackoffMs(attempt: number, opts: BackoffOptions): number {
  const normalizedAttempt = Math.max(0, Math.trunc(attempt));
  let base = opts.initialBackoffMs;

  for (let i = 0; i < normalizedAttempt; i += 1) {
    const next = base * 2;
    if (!Number.isFinite(next) || next <= 0 || next > opts.maxBackoffMs) {
      base = opts.maxBackoffMs;
      break;
    }
    base = next;
  }

  if (base > opts.maxBackoffMs) {
    base = opts.maxBackoffMs;
  }

  if (opts.jitterRatio > 0) {
    const delta = Math.trunc((opts.random() * 2 - 1) * opts.jitterRatio * base);
    base += delta;
  }

  return Math.max(0, base);
}

function nonNegative(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.max(0, value);
}

function resolveWsUrl(url: string): string {
  if (url.startsWith('ws://') || url.startsWith('wss://')) {
    return url;
  }

  const location = globalThis.location;
  if (location === undefined) {
    return url;
  }

  const resolved = new URL(url, location.href);
  if (resolved.protocol === 'https:') {
    resolved.protocol = 'wss:';
  } else if (resolved.protocol === 'http:') {
    resolved.protocol = 'ws:';
  }
  return resolved.toString();
}

function resolveWebSocketCtor(provided: WsSocketConstructor | undefined): WsSocketConstructor {
  if (provided !== undefined) {
    return provided;
  }
  if (typeof globalThis.WebSocket === 'undefined') {
    throw new Error('WebSocket is not available in this environment');
  }
  return globalThis.WebSocket as unknown as WsSocketConstructor;
}

const defaultSetTimeout: WsSetTimeout = (handler, delayMs) =>
  globalThis.setTimeout(handler, delayMs);

const defaultClearTimeout: WsClearTimeout = (handle) => {
  globalThis.clearTimeout(handle as ReturnType<typeof globalThis.setTimeout>);
};
