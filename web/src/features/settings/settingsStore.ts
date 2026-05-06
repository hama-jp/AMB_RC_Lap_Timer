export const SETTINGS_STORAGE_KEYS = {
  transponder: 'amb-rc:setting:transponder',
  speechEnabled: 'amb-rc:setting:speech.enabled',
} as const;

export const SETTINGS_CHANGED_EVENT = 'amb-rc:settings-changed';

export type AppSettings = {
  readonly transponder: number | null;
  readonly speechEnabled: boolean;
};

type SettingsStorage = Pick<Storage, 'getItem' | 'removeItem' | 'setItem'>;

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
