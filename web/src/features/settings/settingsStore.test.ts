import {
  SETTINGS_STORAGE_KEYS,
  clearSettings,
  loadSettings,
  parseTransponderInput,
  saveSettings,
  type AppSettings,
} from './settingsStore';

class MemoryStorage implements Pick<Storage, 'getItem' | 'removeItem' | 'setItem'> {
  readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

describe('settingsStore', () => {
  it('round-trips settings through localStorage', () => {
    const storage = new MemoryStorage();
    const settings: AppSettings = { transponder: 123456, speechEnabled: true };

    saveSettings(settings, storage);

    expect(loadSettings(storage)).toEqual(settings);
  });

  it('uses the required storage keys only', () => {
    expect(Object.values(SETTINGS_STORAGE_KEYS)).toEqual([
      'amb-rc:setting:transponder',
      'amb-rc:setting:speech.enabled',
    ]);
    for (const key of Object.values(SETTINGS_STORAGE_KEYS)) {
      expect(key.startsWith('amb-rc:setting:')).toBe(true);
    }
  });

  it('normalizes hex transponder input to a decimal number', () => {
    expect(parseTransponderInput('0x000000ff')).toEqual({
      ok: true,
      value: 255,
      normalized: '255',
    });
  });

  it('accepts an empty transponder as unset', () => {
    expect(parseTransponderInput('')).toEqual({ ok: true, value: null, normalized: '' });
  });

  it('rejects invalid transponder input', () => {
    expect(parseTransponderInput('abc').ok).toBe(false);
    expect(parseTransponderInput('-1').ok).toBe(false);
    expect(parseTransponderInput('0x100000000').ok).toBe(false);
  });

  it('stores an unset transponder as an empty string', () => {
    const storage = new MemoryStorage();

    saveSettings({ transponder: null, speechEnabled: false }, storage);

    expect(storage.getItem(SETTINGS_STORAGE_KEYS.transponder)).toBe('');
    expect(storage.getItem(SETTINGS_STORAGE_KEYS.speechEnabled)).toBe('false');
  });

  it('clears only settings keys', () => {
    const storage = new MemoryStorage();
    storage.setItem(SETTINGS_STORAGE_KEYS.transponder, '123');
    storage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');
    storage.setItem('amb-rc:state:other', 'keep');

    clearSettings(storage);

    expect(storage.getItem(SETTINGS_STORAGE_KEYS.transponder)).toBeNull();
    expect(storage.getItem(SETTINGS_STORAGE_KEYS.speechEnabled)).toBeNull();
    expect(storage.getItem('amb-rc:state:other')).toBe('keep');
  });
});
