import { useEffect, useState } from 'react';

import { loadSettings, SETTINGS_CHANGED_EVENT } from '../settings/settingsStore';
import type { SpeechController } from './speechController';

export interface SpeechUnlockOverlayProps {
  readonly controller: SpeechController;
}

export function SpeechUnlockOverlay({ controller }: SpeechUnlockOverlayProps): JSX.Element | null {
  const [speechEnabled, setSpeechEnabled] = useState(() => loadSettings().speechEnabled);
  const [unlocked, setUnlocked] = useState(() => controller.isUnlocked());

  useEffect(() => {
    function refreshSettings(): void {
      setSpeechEnabled(loadSettings().speechEnabled);
      setUnlocked(controller.isUnlocked());
    }

    window.addEventListener(SETTINGS_CHANGED_EVENT, refreshSettings);
    window.addEventListener('storage', refreshSettings);
    refreshSettings();
    return () => {
      window.removeEventListener(SETTINGS_CHANGED_EVENT, refreshSettings);
      window.removeEventListener('storage', refreshSettings);
    };
  }, [controller]);

  if (!speechEnabled || !controller.isSupported() || unlocked) {
    return null;
  }

  function handleUnlock(): void {
    controller.unlock();
    setUnlocked(controller.isUnlocked());
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 px-4 backdrop-blur-sm">
      <div className="max-w-sm rounded-2xl border border-cyan-300/40 bg-slate-900 p-5 text-center shadow-2xl shadow-cyan-950/40">
        <p className="text-sm font-medium text-slate-300">
          iPhone / ブラウザーの音声制限を解除します。
        </p>
        <button
          className="mt-4 rounded-full bg-cyan-300 px-6 py-3 text-base font-bold text-slate-950 transition hover:bg-cyan-200 focus:outline-none focus:ring-2 focus:ring-cyan-100 focus:ring-offset-2 focus:ring-offset-slate-950"
          onClick={handleUnlock}
          type="button"
        >
          🔊 読み上げを有効化
        </button>
      </div>
    </div>
  );
}
