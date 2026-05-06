import type { AdminConfig } from './api';

/**
 * Frontend-bundled "factory defaults" used by the "初期値に戻す" button.
 * Mirrors gateway/internal/config/config.go's Defaults() so the user can
 * revert without a server round-trip. Drift between this file and Defaults()
 * is caught by handlers_admin_test.go validating the round-tripped config,
 * but the canonical source is still the Go side.
 */
export const ADMIN_CONFIG_DEFAULTS: AdminConfig = {
  listen: ':8080',
  upstream: {
    host: '192.168.1.21',
    port: 5403,
    reconnect: {
      initial_ms: 1000,
      max_ms: 30000,
      jitter_ratio: 0.2,
    },
  },
  logging: {
    dir: './logs',
    max_size_mb: 5,
    max_backups: 5,
  },
  records: {
    dir: './records',
  },
  replay: {
    speed: 'realtime',
  },
  server: {
    max_clients: 100,
    client_buffer_len: 64,
  },
};
