export interface SpeechController {
  /** Returns whether this browser exposes the Web Speech synthesis API. */
  isSupported(): boolean;
  /** Cancels the current utterance before speaking the next one. */
  speak(text: string, opts?: SpeechOptions): void;
  /** Cancels all queued and active utterances. */
  cancel(): void;
  /** Unlocks iOS Safari speech by speaking one empty utterance after a user gesture. */
  unlock(): void;
  /** Returns whether the explicit unlock gesture has already completed. */
  isUnlocked(): boolean;
}

export interface SpeechOptions {
  readonly lang?: string;
  readonly rate?: number;
  readonly volume?: number;
}

export interface SpeechControllerOptions {
  readonly synthesis?: SpeechSynthesis | undefined;
  readonly UtteranceCtor?: typeof SpeechSynthesisUtterance | undefined;
  readonly initiallyUnlocked?: boolean;
}

const DEFAULT_LANG = 'ja-JP';
const DEFAULT_RATE = 1.0;
const DEFAULT_VOLUME = 1.0;

export function createSpeechController(opts: SpeechControllerOptions = {}): SpeechController {
  const synthesis = Object.hasOwn(opts, 'synthesis') ? opts.synthesis : getDefaultSynthesis();
  const UtteranceCtor = Object.hasOwn(opts, 'UtteranceCtor')
    ? opts.UtteranceCtor
    : getDefaultUtteranceCtor();
  let unlocked = opts.initiallyUnlocked ?? false;
  let warnedUnsupported = false;

  function isSupported(): boolean {
    return synthesis !== undefined && UtteranceCtor !== undefined;
  }

  function warnUnsupportedOnce(): void {
    if (warnedUnsupported) {
      return;
    }
    warnedUnsupported = true;
    console.warn('Speech synthesis is not supported in this browser.');
  }

  function createUtterance(text: string, options: SpeechOptions = {}): SpeechSynthesisUtterance {
    const utterance = new UtteranceCtor!(text);
    utterance.lang = options.lang ?? DEFAULT_LANG;
    utterance.rate = options.rate ?? DEFAULT_RATE;
    utterance.volume = options.volume ?? DEFAULT_VOLUME;
    return utterance;
  }

  return {
    isSupported,
    speak(text, options) {
      if (synthesis === undefined || UtteranceCtor === undefined) {
        warnUnsupportedOnce();
        return;
      }

      synthesis.cancel();
      synthesis.speak(createUtterance(text, options));
    },
    cancel() {
      if (synthesis === undefined || UtteranceCtor === undefined) {
        warnUnsupportedOnce();
        return;
      }

      synthesis.cancel();
    },
    unlock() {
      if (unlocked) {
        return;
      }

      if (synthesis === undefined || UtteranceCtor === undefined) {
        warnUnsupportedOnce();
        return;
      }

      synthesis.speak(createUtterance(''));
      unlocked = true;
    },
    isUnlocked() {
      return unlocked;
    },
  };
}

function getDefaultSynthesis(): SpeechSynthesis | undefined {
  return typeof window === 'undefined' ? undefined : window.speechSynthesis;
}

function getDefaultUtteranceCtor(): typeof SpeechSynthesisUtterance | undefined {
  return typeof window === 'undefined' ? undefined : window.SpeechSynthesisUtterance;
}
