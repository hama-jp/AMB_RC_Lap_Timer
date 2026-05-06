/**
 * Thin fetch wrappers for /admin/api/*.
 *
 * The browser does not see the auth cookie (HttpOnly) so the SPA can never
 * tell up front whether it is logged in — we simply call the API and react
 * to a 401 by redirecting to /admin/login. Same pattern as POST handlers
 * for config: 200 succeeds, 400 returns ValidationError[], 401 means the
 * session expired or was never issued.
 */

/** Mirrors gateway/internal/config/config.go Config. */
export interface AdminConfig {
  listen: string;
  upstream: {
    host: string;
    port: number;
    reconnect: {
      initial_ms: number;
      max_ms: number;
      jitter_ratio: number;
    };
  };
  logging: {
    dir: string;
    max_size_mb: number;
    max_backups: number;
  };
  records: {
    dir: string;
  };
  replay: {
    speed: string;
  };
  server: {
    max_clients: number;
    client_buffer_len: number;
  };
}

export interface ValidationError {
  path: string;
  message: string;
}

export interface PostConfigSuccess {
  applied: string[] | null;
  requires_restart: string[] | null;
  config: AdminConfig;
  changed_fields: string[] | null;
}

export class AuthRequiredError extends Error {
  constructor() {
    super('admin auth required');
    this.name = 'AuthRequiredError';
  }
}

export class ValidationErrorList extends Error {
  readonly errors: ValidationError[];
  constructor(errors: ValidationError[]) {
    super(`validation: ${errors.length} error(s)`);
    this.name = 'ValidationErrorList';
    this.errors = errors;
  }
}

export class RateLimitedError extends Error {
  readonly retryAfterMs: number;
  constructor(retryAfterMs: number) {
    super(`rate limited; retry after ${retryAfterMs}ms`);
    this.name = 'RateLimitedError';
    this.retryAfterMs = retryAfterMs;
  }
}

const JSON_HEADERS = { 'Content-Type': 'application/json' };

export async function fetchAdminConfig(): Promise<AdminConfig> {
  const resp = await fetch('/admin/api/config', {
    method: 'GET',
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  });
  if (resp.status === 401) {
    throw new AuthRequiredError();
  }
  if (!resp.ok) {
    throw new Error(`GET /admin/api/config: ${resp.status}`);
  }
  return (await resp.json()) as AdminConfig;
}

export async function postAdminConfig(cfg: AdminConfig): Promise<PostConfigSuccess> {
  const resp = await fetch('/admin/api/config', {
    method: 'POST',
    credentials: 'same-origin',
    headers: JSON_HEADERS,
    body: JSON.stringify(cfg),
  });
  if (resp.status === 401) {
    throw new AuthRequiredError();
  }
  if (resp.status === 400) {
    const body = (await resp.json()) as { errors?: ValidationError[] };
    throw new ValidationErrorList(body.errors ?? []);
  }
  if (!resp.ok) {
    throw new Error(`POST /admin/api/config: ${resp.status}`);
  }
  return (await resp.json()) as PostConfigSuccess;
}

export async function postAdminLogin(passphrase: string): Promise<void> {
  const resp = await fetch('/admin/api/login', {
    method: 'POST',
    credentials: 'same-origin',
    headers: JSON_HEADERS,
    body: JSON.stringify({ passphrase }),
  });
  if (resp.status === 200) {
    return;
  }
  if (resp.status === 401) {
    throw new AuthRequiredError();
  }
  if (resp.status === 429) {
    const body = (await resp.json()) as { retry_after_ms?: number };
    throw new RateLimitedError(body.retry_after_ms ?? 5000);
  }
  throw new Error(`POST /admin/api/login: ${resp.status}`);
}

export async function postAdminLogout(): Promise<void> {
  await fetch('/admin/api/logout', {
    method: 'POST',
    credentials: 'same-origin',
  });
  // Logout is fire-and-forget — even on a 4xx we treat it as "session
  // gone" because the next protected call will surface AuthRequiredError.
}
