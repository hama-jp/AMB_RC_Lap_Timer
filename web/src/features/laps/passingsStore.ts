import { createDecoder, type Decoder, type PassingRecord } from '../../protocol';
import type { WsClient, WsMessage, WsState } from '../../transport/wsClient';
import { loadSettings } from '../settings/settingsStore';

export const PASSING_BUFFER_LIMIT = 50;

export interface PassingEntry {
  readonly record: PassingRecord;
  /** Difference from the previous matching PASSING RTC_TIME in microseconds. */
  readonly lapTimeUs: bigint | null;
}

export interface PassingsSnapshot {
  readonly targetTransponder: number | null;
  readonly passings: readonly PassingEntry[];
  /**
   * Smallest non-null `lapTimeUs` across `passings`. Cleared on reset / reconnect.
   * `null` when fewer than two matching passings have been observed.
   */
  readonly bestLapUs: bigint | null;
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
  private passings: PassingEntry[] = [];
  private bestLapUs: bigint | null = null;
  private previousPassing: PassingRecord | null = null;
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
      bestLapUs: this.bestLapUs,
    };
  }

  reset(): void {
    this.decoder.reset();
    this.passings = [];
    this.bestLapUs = null;
    this.previousPassing = null;
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
      const entry: PassingEntry = {
        record: result.record,
        lapTimeUs:
          this.previousPassing === null
            ? null
            : result.record.rtcTimeUs - this.previousPassing.rtcTimeUs,
      };
      this.previousPassing = result.record;
      this.passings = [entry, ...this.passings].slice(0, this.limit);
      // Best lap is the smallest non-null lapTimeUs we've seen since reset.
      // Strict "<" so the first occurrence of a tied time keeps the badge.
      if (
        entry.lapTimeUs !== null &&
        (this.bestLapUs === null || entry.lapTimeUs < this.bestLapUs)
      ) {
        this.bestLapUs = entry.lapTimeUs;
      }
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
