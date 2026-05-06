/**
 * Per-field "when does this take effect" labels, matching the buckets in
 * docs/architecture.md §3.5.5 and the Go classifier in
 * gateway/internal/httpsrv/handlers_admin.go::classifyChanged.
 */
export type ApplyTiming = 'immediate' | 'next-reconnect' | 'next-start' | 'restart';

export const APPLY_TIMING_LABEL: Record<ApplyTiming, string> = {
  immediate: '即時',
  'next-reconnect': '次回再接続から',
  'next-start': '次回起動から',
  restart: '再起動必要',
};

const TIMINGS: Record<string, ApplyTiming> = {
  listen: 'restart',
  'upstream.host': 'immediate',
  'upstream.port': 'immediate',
  'upstream.reconnect.initial_ms': 'next-reconnect',
  'upstream.reconnect.max_ms': 'next-reconnect',
  'upstream.reconnect.jitter_ratio': 'next-reconnect',
  'logging.dir': 'restart',
  'logging.max_size_mb': 'restart',
  'logging.max_backups': 'restart',
  'records.dir': 'next-start',
  'replay.speed': 'next-start',
  'server.max_clients': 'immediate',
  'server.client_buffer_len': 'immediate',
};

export function applyTimingFor(path: string): ApplyTiming {
  return TIMINGS[path] ?? 'next-start';
}
