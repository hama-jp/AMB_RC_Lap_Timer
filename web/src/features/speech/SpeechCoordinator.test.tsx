import { act, render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PassingEntry, PassingsSnapshot, PassingsStore } from '../laps/passingsStore';
import { SETTINGS_STORAGE_KEYS } from '../settings/settingsStore';
import type { SpeechController } from './speechController';
import { SpeechCoordinator } from './SpeechCoordinator';

class FakePassingsStore implements PassingsStore {
  private readonly subscribers = new Set<(snapshot: PassingsSnapshot) => void>();
  private snapshot: PassingsSnapshot = { targetTransponder: 1, passings: [], bestLapUs: null };

  start(): () => void {
    return () => {};
  }

  getSnapshot(): PassingsSnapshot {
    return this.snapshot;
  }

  reset(): void {
    this.emit({ targetTransponder: 1, passings: [], bestLapUs: null });
  }

  subscribe(handler: (snapshot: PassingsSnapshot) => void): () => void {
    this.subscribers.add(handler);
    return () => {
      this.subscribers.delete(handler);
    };
  }

  emit(snapshot: PassingsSnapshot): void {
    this.snapshot = snapshot;
    for (const subscriber of [...this.subscribers]) {
      subscriber(snapshot);
    }
  }
}

function createController(
  opts: {
    readonly supported?: boolean;
    readonly unlocked?: boolean;
  } = {},
): SpeechController & { readonly speak: ReturnType<typeof vi.fn> } {
  const speak = vi.fn();
  return {
    isSupported: () => opts.supported ?? true,
    speak,
    cancel: vi.fn(),
    unlock: vi.fn(),
    isUnlocked: () => opts.unlocked ?? true,
  };
}

function entry(lapTimeUs: bigint | null): PassingEntry {
  return {
    lapTimeUs,
    record: {
      passingNumber: Number(lapTimeUs ?? 0n),
      transponder: 1,
      rtcTimeUs: 31_789_000n,
      strength: 120,
      hits: 7,
      flags: 0,
    },
  };
}

describe('SpeechCoordinator', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('speaks the latest lap time when a new passing with lap arrives', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');
    const store = new FakePassingsStore();
    const controller = createController();
    render(<SpeechCoordinator controller={controller} store={store} />);

    act(() => {
      store.emit({ targetTransponder: 1, passings: [entry(21_789_000n)], bestLapUs: null });
    });

    expect(controller.speak).toHaveBeenCalledWith('21.789秒');
  });

  it('does not speak the first passing without a lap time', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');
    const store = new FakePassingsStore();
    const controller = createController();
    render(<SpeechCoordinator controller={controller} store={store} />);

    act(() => {
      store.emit({ targetTransponder: 1, passings: [entry(null)], bestLapUs: null });
    });

    expect(controller.speak).not.toHaveBeenCalled();
  });

  it('does not speak when speech is disabled', () => {
    const store = new FakePassingsStore();
    const controller = createController();
    render(<SpeechCoordinator controller={controller} store={store} />);

    act(() => {
      store.emit({ targetTransponder: 1, passings: [entry(21_789_000n)], bestLapUs: null });
    });

    expect(controller.speak).not.toHaveBeenCalled();
  });

  it('does not speak the same top entry twice', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');
    const store = new FakePassingsStore();
    const controller = createController();
    const topEntry = entry(21_789_000n);
    render(<SpeechCoordinator controller={controller} store={store} />);

    act(() => {
      store.emit({ targetTransponder: 1, passings: [topEntry], bestLapUs: null });
      store.emit({ targetTransponder: 1, passings: [topEntry], bestLapUs: null });
    });

    expect(controller.speak).toHaveBeenCalledTimes(1);
  });

  it('does not speak before unlock', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');
    const store = new FakePassingsStore();
    const controller = createController({ unlocked: false });
    render(<SpeechCoordinator controller={controller} store={store} />);

    act(() => {
      store.emit({ targetTransponder: 1, passings: [entry(21_789_000n)], bestLapUs: null });
    });

    expect(controller.speak).not.toHaveBeenCalled();
  });
});
