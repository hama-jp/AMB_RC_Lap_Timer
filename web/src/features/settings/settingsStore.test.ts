import {
  RECENT_TRANSPONDERS_LIMIT,
  SETTINGS_STORAGE_KEYS,
  addRecentTransponder,
  clearRecentTransponders,
  clearSettings,
  loadRecentTransponders,
  loadSettings,
  parseTransponderInput,
  removeRecentTransponder,
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
      'amb-rc:setting:transponder.recents',
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

  describe('recent transponders (#110)', () => {
    it('returns an empty list when nothing is stored yet', () => {
      const storage = new MemoryStorage();
      expect(loadRecentTransponders(storage)).toEqual([]);
    });

    it('adds new entries to the front and dedupes', () => {
      const storage = new MemoryStorage();
      addRecentTransponder(111, storage);
      addRecentTransponder(222, storage);
      addRecentTransponder(333, storage);
      // Re-saving 222 must move it to the front, not duplicate.
      addRecentTransponder(222, storage);
      expect(loadRecentTransponders(storage)).toEqual([222, 333, 111]);
    });

    it(`caps the list at ${RECENT_TRANSPONDERS_LIMIT} entries (LRU)`, () => {
      const storage = new MemoryStorage();
      for (let i = 1; i <= RECENT_TRANSPONDERS_LIMIT + 2; i += 1) {
        addRecentTransponder(i * 100, storage);
      }
      const list = loadRecentTransponders(storage);
      expect(list).toHaveLength(RECENT_TRANSPONDERS_LIMIT);
      // Newest first; the two oldest (100, 200) were dropped.
      expect(list[0]).toBe((RECENT_TRANSPONDERS_LIMIT + 2) * 100);
      expect(list).not.toContain(100);
      expect(list).not.toContain(200);
    });

    it('rejects out-of-range and non-integer values silently', () => {
      const storage = new MemoryStorage();
      addRecentTransponder(-1, storage);
      addRecentTransponder(0xffffffff + 1, storage);
      addRecentTransponder(1.5, storage);
      addRecentTransponder(Number.NaN, storage);
      expect(loadRecentTransponders(storage)).toEqual([]);
    });

    it('removes a single entry without touching the rest', () => {
      const storage = new MemoryStorage();
      addRecentTransponder(111, storage);
      addRecentTransponder(222, storage);
      addRecentTransponder(333, storage);
      removeRecentTransponder(222, storage);
      expect(loadRecentTransponders(storage)).toEqual([333, 111]);
      // No-op when the value is absent.
      removeRecentTransponder(999, storage);
      expect(loadRecentTransponders(storage)).toEqual([333, 111]);
    });

    it('clears the entire list', () => {
      const storage = new MemoryStorage();
      addRecentTransponder(111, storage);
      addRecentTransponder(222, storage);
      clearRecentTransponders(storage);
      expect(loadRecentTransponders(storage)).toEqual([]);
      expect(storage.getItem(SETTINGS_STORAGE_KEYS.recentTransponders)).toBeNull();
    });

    it('drops malformed JSON and recovers', () => {
      const storage = new MemoryStorage();
      storage.setItem(SETTINGS_STORAGE_KEYS.recentTransponders, '{not json');
      expect(loadRecentTransponders(storage)).toEqual([]);
      // Subsequent add still works (recovery).
      addRecentTransponder(42, storage);
      expect(loadRecentTransponders(storage)).toEqual([42]);
    });

    it('drops non-array payloads', () => {
      const storage = new MemoryStorage();
      storage.setItem(SETTINGS_STORAGE_KEYS.recentTransponders, '"hello"');
      expect(loadRecentTransponders(storage)).toEqual([]);
    });

    it('filters out invalid entries inside an array payload', () => {
      const storage = new MemoryStorage();
      storage.setItem(
        SETTINGS_STORAGE_KEYS.recentTransponders,
        JSON.stringify([111, -5, 0xffffffff + 1, 222, 'bad', 333]),
      );
      expect(loadRecentTransponders(storage)).toEqual([111, 222, 333]);
    });
  });
});
