export const SETTINGS_STORAGE_KEYS = {
  transponder: 'amb-rc:setting:transponder',
  speechEnabled: 'amb-rc:setting:speech.enabled',
  recentTransponders: 'amb-rc:setting:transponder.recents',
} as const;

export const SETTINGS_CHANGED_EVENT = 'amb-rc:settings-changed';

/** Maximum recent transponder history kept in localStorage (Issue #110). */
export const RECENT_TRANSPONDERS_LIMIT = 5;

export type AppSettings = {
  readonly transponder: number | null;
  readonly speechEnabled: boolean;
};

export type SettingsStorage = Pick<Storage, 'getItem' | 'removeItem' | 'setItem'>;

type TransponderParseResult =
  | {
      readonly ok: true;
      readonly value: number | null;
      readonly normalized: string;
    }
  | {
      readonly ok: false;
      readonly message: string;
    };

const UINT32_MAX = 0xffffffffn;
const TRANSPONDER_DECIMAL_PATTERN = /^\d+$/u;
const TRANSPONDER_HEX_PATTERN = /^0x[0-9a-f]+$/iu;

export function parseTransponderInput(input: string): TransponderParseResult {
  const trimmed = input.trim();

  if (trimmed === '') {
    return { ok: true, value: null, normalized: '' };
  }

  if (!TRANSPONDER_DECIMAL_PATTERN.test(trimmed) && !TRANSPONDER_HEX_PATTERN.test(trimmed)) {
    return {
      ok: false,
      message: 'トランスポンダーIDは10進数、または0x付き16進数で入力してください。',
    };
  }

  const value = BigInt(trimmed.toLowerCase());
  if (value > UINT32_MAX) {
    return {
      ok: false,
      message: 'トランスポンダーIDは0から0xFFFFFFFFまでの範囲で入力してください。',
    };
  }

  const normalized = value.toString(10);
  return { ok: true, value: Number(value), normalized };
}

export function loadSettings(storage: SettingsStorage = window.localStorage): AppSettings {
  const transponderRaw = storage.getItem(SETTINGS_STORAGE_KEYS.transponder) ?? '';
  const parsedTransponder = parseTransponderInput(transponderRaw);

  return {
    transponder: parsedTransponder.ok ? parsedTransponder.value : null,
    speechEnabled: storage.getItem(SETTINGS_STORAGE_KEYS.speechEnabled) === 'true',
  };
}

export function saveSettings(
  settings: AppSettings,
  storage: SettingsStorage = window.localStorage,
): void {
  storage.setItem(
    SETTINGS_STORAGE_KEYS.transponder,
    settings.transponder === null ? '' : settings.transponder.toString(10),
  );
  storage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, String(settings.speechEnabled));

  if (typeof window !== 'undefined') {
    window.dispatchEvent(new Event(SETTINGS_CHANGED_EVENT));
  }
}

export function clearSettings(storage: SettingsStorage = window.localStorage): void {
  storage.removeItem(SETTINGS_STORAGE_KEYS.transponder);
  storage.removeItem(SETTINGS_STORAGE_KEYS.speechEnabled);
}

/**
 * Load the recent-transponder MRU list (Issue #110). Returns up to
 * RECENT_TRANSPONDERS_LIMIT entries, newest first. Malformed JSON, non-array
 * payloads, and non-uint32 values are silently dropped so a polluted key
 * cannot break the settings page.
 */
export function loadRecentTransponders(
  storage: SettingsStorage = window.localStorage,
): readonly number[] {
  const raw = storage.getItem(SETTINGS_STORAGE_KEYS.recentTransponders);
  if (raw === null || raw === '') {
    return [];
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) {
    return [];
  }
  const out: number[] = [];
  for (const entry of parsed) {
    if (typeof entry !== 'number' || !Number.isInteger(entry) || entry < 0 || entry > 0xffffffff) {
      continue;
    }
    if (out.includes(entry)) {
      continue; // dedupe a malformed-but-valid duplicate-bearing payload
    }
    out.push(entry);
    if (out.length >= RECENT_TRANSPONDERS_LIMIT) {
      break;
    }
  }
  return out;
}

function writeRecentTransponders(values: readonly number[], storage: SettingsStorage): void {
  if (values.length === 0) {
    storage.removeItem(SETTINGS_STORAGE_KEYS.recentTransponders);
  } else {
    storage.setItem(SETTINGS_STORAGE_KEYS.recentTransponders, JSON.stringify(values));
  }
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new Event(SETTINGS_CHANGED_EVENT));
  }
}

/**
 * Add `value` to the front of the recent-transponder list, removing any
 * existing copy and capping at RECENT_TRANSPONDERS_LIMIT. Out-of-range or
 * non-integer values are ignored so bad calls don't pollute storage.
 */
export function addRecentTransponder(
  value: number,
  storage: SettingsStorage = window.localStorage,
): void {
  if (!Number.isInteger(value) || value < 0 || value > 0xffffffff) {
    return;
  }
  const current = loadRecentTransponders(storage);
  const filtered = current.filter((entry) => entry !== value);
  const next = [value, ...filtered].slice(0, RECENT_TRANSPONDERS_LIMIT);
  writeRecentTransponders(next, storage);
}

/** Remove a single entry from the recent list. No-op if not present. */
export function removeRecentTransponder(
  value: number,
  storage: SettingsStorage = window.localStorage,
): void {
  const current = loadRecentTransponders(storage);
  const next = current.filter((entry) => entry !== value);
  if (next.length === current.length) {
    return;
  }
  writeRecentTransponders(next, storage);
}

/** Wipe the entire recent-transponder list. */
export function clearRecentTransponders(storage: SettingsStorage = window.localStorage): void {
  if (storage.getItem(SETTINGS_STORAGE_KEYS.recentTransponders) === null) {
    return;
  }
  writeRecentTransponders([], storage);
}
