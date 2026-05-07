import { useEffect, useRef } from 'react';

import type { PassingEntry, PassingsStore } from '../laps/passingsStore';
import { loadSettings, SETTINGS_CHANGED_EVENT } from '../settings/settingsStore';
import { formatLapTimeForSpeech } from './formatLapTimeForSpeech';
import type { SpeechController } from './speechController';

export interface SpeechCoordinatorProps {
  readonly store: PassingsStore;
  readonly controller: SpeechController;
}

export function SpeechCoordinator({ store, controller }: SpeechCoordinatorProps): null {
  const speechEnabledRef = useRef(loadSettings().speechEnabled);
  const lastSpokenEntryRef = useRef<PassingEntry | null>(null);

  useEffect(() => {
    function refreshSettings(): void {
      speechEnabledRef.current = loadSettings().speechEnabled;
    }

    window.addEventListener(SETTINGS_CHANGED_EVENT, refreshSettings);
    window.addEventListener('storage', refreshSettings);
    refreshSettings();
    return () => {
      window.removeEventListener(SETTINGS_CHANGED_EVENT, refreshSettings);
      window.removeEventListener('storage', refreshSettings);
    };
  }, []);

  useEffect(() => {
    return store.subscribe((snapshot) => {
      const topEntry = snapshot.passings[0] ?? null;
      if (topEntry === null || topEntry === lastSpokenEntryRef.current) {
        return;
      }

      lastSpokenEntryRef.current = topEntry;
      if (
        topEntry.lapTimeUs === null ||
        !speechEnabledRef.current ||
        !controller.isSupported() ||
        !controller.isUnlocked()
      ) {
        return;
      }

      const utterance = formatLapTimeForSpeech(topEntry.lapTimeUs);
      if (utterance === '') {
        return;
      }
      controller.speak(utterance);
    });
  }, [controller, store]);

  return null;
}
