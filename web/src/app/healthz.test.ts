import { describe, expect, it, vi } from 'vitest';

import { fetchGatewayVersion } from './healthz';

describe('fetchGatewayVersion', () => {
  it('returns the version from /healthz', async () => {
    const fetchImpl = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ version: 'v1.2.3' }),
      }),
    ) as unknown as typeof fetch;

    await expect(fetchGatewayVersion(fetchImpl)).resolves.toBe('v1.2.3');
    expect(fetchImpl).toHaveBeenCalledWith('/healthz', {
      headers: { accept: 'application/json' },
    });
  });

  it('returns unknown on fetch failure or malformed payload', async () => {
    const failingFetch = vi.fn(() =>
      Promise.reject(new Error('network')),
    ) as unknown as typeof fetch;
    const malformedFetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ version: 123 }),
      }),
    ) as unknown as typeof fetch;

    await expect(fetchGatewayVersion(failingFetch)).resolves.toBe('unknown');
    await expect(fetchGatewayVersion(malformedFetch)).resolves.toBe('unknown');
  });
});
