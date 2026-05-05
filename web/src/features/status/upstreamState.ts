export type UpstreamState =
  | { kind: 'unknown' }
  | { kind: 'connected' }
  | { kind: 'reconnecting' }
  | { kind: 'finished' }
  | { kind: 'mock' }
  | { kind: 'replay' };

const UPSTREAM_KINDS = new Set<UpstreamState['kind']>([
  'connected',
  'reconnecting',
  'finished',
  'mock',
  'replay',
]);

export function parseUpstreamMessage(text: string): UpstreamState | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return null;
  }

  if (!isRecord(parsed) || parsed.type !== 'upstream') {
    return null;
  }

  if (typeof parsed.status !== 'string') {
    return { kind: 'unknown' };
  }

  if (UPSTREAM_KINDS.has(parsed.status as UpstreamState['kind'])) {
    return { kind: parsed.status as UpstreamState['kind'] };
  }

  return { kind: 'unknown' };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
