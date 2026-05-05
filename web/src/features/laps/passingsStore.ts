import { createDecoder, type Decoder, type PassingRecord } from '../../protocol';
import type { WsClient, WsMessage, WsState } from '../../transport/wsClient';
import { loadSettings } from '../settings/settingsStore';

export const PASSING_BUFFER_LIMIT = 50;

export interface PassingsSnapshot {
  readonly targetTransponder: number | null;
  readonly passings: readonly PassingRecord[];
}

export interface PassingsStore {
  start(): () => void;
  getSnapshot(): PassingsSnapshot;
  reset(): void;
  subscribe(handler: (snapshot: PassingsSnapshot) => void): () => void;
}

export interface PassingsStoreOptions {
  readonly wsClient: WsClient;
  readonly decoder?: Decoder;
  readonly loadTargetTransponder?: () => number | null;
  readonly limit?: number;
}

export function createPassingsStore(opts: PassingsStoreOptions): PassingsStore {
  return new PassingsStoreImpl(
    opts.wsClient,
    opts.decoder ?? createDecoder(),
    opts.loadTargetTransponder ?? (() => loadSettings().transponder),
    opts.limit ?? PASSING_BUFFER_LIMIT,
  );
}

class PassingsStoreImpl implements PassingsStore {
  private readonly subscribers = new Set<(snapshot: PassingsSnapshot) => void>();
  private readonly wsClient: WsClient;
  private readonly decoder: Decoder;
  private readonly loadTargetTransponder: () => number | null;
  private readonly limit: number;
  private targetTransponder: number | null;
  private passings: PassingRecord[] = [];
  private stopWsMessage: (() => void) | null = null;
  private stopWsState: (() => void) | null = null;

  constructor(
    wsClient: WsClient,
    decoder: Decoder,
    loadTargetTransponder: () => number | null,
    limit: number,
  ) {
    this.wsClient = wsClient;
    this.decoder = decoder;
    this.loadTargetTransponder = loadTargetTransponder;
    this.limit = limit;
    this.targetTransponder = loadTargetTransponder();
  }

  start(): () => void {
    if (this.stopWsMessage !== null || this.stopWsState !== null) {
      return () => this.stop();
    }

    this.targetTransponder = this.loadTargetTransponder();
    this.emit();

    this.stopWsMessage = this.wsClient.onMessage((message) => this.handleMessage(message));
    this.stopWsState = this.wsClient.onStateChange((state) => this.handleState(state));

    return () => this.stop();
  }

  getSnapshot(): PassingsSnapshot {
    return {
      targetTransponder: this.targetTransponder,
      passings: this.passings,
    };
  }

  reset(): void {
    this.decoder.reset();
    this.passings = [];
    this.emit();
  }

  subscribe(handler: (snapshot: PassingsSnapshot) => void): () => void {
    this.subscribers.add(handler);
    return () => {
      this.subscribers.delete(handler);
    };
  }

  private stop(): void {
    this.stopWsMessage?.();
    this.stopWsState?.();
    this.stopWsMessage = null;
    this.stopWsState = null;
  }

  private handleMessage(message: WsMessage): void {
    if (!(message instanceof ArrayBuffer)) {
      return;
    }

    this.targetTransponder = this.loadTargetTransponder();
    if (this.targetTransponder === null) {
      return;
    }

    const results = this.decoder.push(new Uint8Array(message));
    let changed = false;
    for (const result of results) {
      if (result.kind !== 'passing' || result.record.transponder !== this.targetTransponder) {
        continue;
      }
      this.passings = [result.record, ...this.passings].slice(0, this.limit);
      changed = true;
    }

    if (changed) {
      this.emit();
    }
  }

  private handleState(state: WsState): void {
    if (state.kind === 'disconnected' || state.kind === 'reconnecting') {
      this.reset();
    }
  }

  private emit(): void {
    const snapshot = this.getSnapshot();
    for (const subscriber of [...this.subscribers]) {
      subscriber(snapshot);
    }
  }
}
