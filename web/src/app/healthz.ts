export interface HealthzPayload {
  readonly version?: unknown;
}

export async function fetchGatewayVersion(
  fetchImpl: typeof fetch | undefined = globalThis.fetch,
): Promise<string> {
  if (fetchImpl === undefined) {
    return 'unknown';
  }

  try {
    const response = await fetchImpl('/healthz', {
      headers: { accept: 'application/json' },
    });
    if (!response.ok) {
      return 'unknown';
    }

    const payload = (await response.json()) as HealthzPayload;
    return typeof payload.version === 'string' && payload.version.length > 0
      ? payload.version
      : 'unknown';
  } catch {
    return 'unknown';
  }
}
