import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { SETTINGS_STORAGE_KEYS } from '../settings/settingsStore';
import type { SpeechController } from './speechController';
import { SpeechUnlockOverlay } from './SpeechUnlockOverlay';

function createController(
  opts: {
    readonly supported?: boolean;
    readonly unlocked?: boolean;
  } = {},
): SpeechController & { readonly unlock: ReturnType<typeof vi.fn> } {
  let unlocked = opts.unlocked ?? false;
  const unlock = vi.fn(() => {
    unlocked = true;
  });
  return {
    isSupported: () => opts.supported ?? true,
    speak: vi.fn(),
    cancel: vi.fn(),
    unlock,
    isUnlocked: () => unlocked,
  };
}

describe('SpeechUnlockOverlay', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('shows the unlock button when speech is enabled, supported, and locked', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');

    render(<SpeechUnlockOverlay controller={createController()} />);

    expect(screen.getByRole('button', { name: '🔊 読み上げを有効化' })).toBeInTheDocument();
  });

  it('does not show when speech is disabled', () => {
    render(<SpeechUnlockOverlay controller={createController()} />);

    expect(screen.queryByRole('button', { name: '🔊 読み上げを有効化' })).not.toBeInTheDocument();
  });

  it('does not show when speech is unsupported', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');

    render(<SpeechUnlockOverlay controller={createController({ supported: false })} />);

    expect(screen.queryByRole('button', { name: '🔊 読み上げを有効化' })).not.toBeInTheDocument();
  });

  it('does not show when already unlocked', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');

    render(<SpeechUnlockOverlay controller={createController({ unlocked: true })} />);

    expect(screen.queryByRole('button', { name: '🔊 読み上げを有効化' })).not.toBeInTheDocument();
  });

  it('unlocks and hides after button click', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');
    const controller = createController();
    render(<SpeechUnlockOverlay controller={controller} />);

    fireEvent.click(screen.getByRole('button', { name: '🔊 読み上げを有効化' }));

    expect(controller.unlock).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('button', { name: '🔊 読み上げを有効化' })).not.toBeInTheDocument();
  });
});
