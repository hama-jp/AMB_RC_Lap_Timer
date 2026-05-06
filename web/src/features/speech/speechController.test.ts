import { describe, expect, it, vi } from 'vitest';

import { createSpeechController } from './speechController';

class FakeUtterance {
  lang = '';
  rate = 0;
  volume = 0;

  constructor(readonly text: string) {}
}

function createFakeSynthesis(): {
  readonly synthesis: SpeechSynthesis;
  readonly speak: ReturnType<typeof vi.fn>;
  readonly cancel: ReturnType<typeof vi.fn>;
  readonly calls: string[];
} {
  const calls: string[] = [];
  const speak = vi.fn(() => calls.push('speak'));
  const cancel = vi.fn(() => calls.push('cancel'));
  return {
    synthesis: { speak, cancel } as unknown as SpeechSynthesis,
    speak,
    cancel,
    calls,
  };
}

const UtteranceCtor = FakeUtterance as unknown as typeof SpeechSynthesisUtterance;

describe('speechController', () => {
  it('is unsupported without SpeechSynthesis and no-ops with one warning', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const controller = createSpeechController({
      synthesis: undefined,
      UtteranceCtor,
    });

    expect(controller.isSupported()).toBe(false);
    controller.speak('test');
    controller.unlock();
    controller.cancel();

    expect(controller.isUnlocked()).toBe(false);
    expect(warn).toHaveBeenCalledTimes(1);
    warn.mockRestore();
  });

  it('cancels before speaking a new utterance', () => {
    const { synthesis, speak, cancel, calls } = createFakeSynthesis();
    const controller = createSpeechController({ synthesis, UtteranceCtor });

    controller.speak('test');

    expect(calls).toEqual(['cancel', 'speak']);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(speak).toHaveBeenCalledTimes(1);
  });

  it('sets default utterance options on speak', () => {
    const { synthesis, speak } = createFakeSynthesis();
    const controller = createSpeechController({ synthesis, UtteranceCtor });

    controller.speak('test');

    const utterance = speak.mock.calls[0]?.[0] as SpeechSynthesisUtterance;
    expect(utterance.lang).toBe('ja-JP');
    expect(utterance.rate).toBe(1.0);
    expect(utterance.volume).toBe(1.0);
  });

  it('allows overriding utterance options', () => {
    const { synthesis, speak } = createFakeSynthesis();
    const controller = createSpeechController({ synthesis, UtteranceCtor });

    controller.speak('test', { lang: 'en-US', rate: 1.2, volume: 0.5 });

    const utterance = speak.mock.calls[0]?.[0] as SpeechSynthesisUtterance;
    expect(utterance.lang).toBe('en-US');
    expect(utterance.rate).toBe(1.2);
    expect(utterance.volume).toBe(0.5);
  });

  it('unlocks by speaking one empty utterance', () => {
    const { synthesis, speak } = createFakeSynthesis();
    const controller = createSpeechController({ synthesis, UtteranceCtor });

    controller.unlock();

    const utterance = speak.mock.calls[0]?.[0] as FakeUtterance;
    expect(utterance.text).toBe('');
    expect(controller.isUnlocked()).toBe(true);
  });

  it('keeps unlock idempotent', () => {
    const { synthesis, speak } = createFakeSynthesis();
    const controller = createSpeechController({ synthesis, UtteranceCtor });

    controller.unlock();
    controller.unlock();

    expect(speak).toHaveBeenCalledTimes(1);
  });

  it('does not unlock from speak alone', () => {
    const { synthesis } = createFakeSynthesis();
    const controller = createSpeechController({ synthesis, UtteranceCtor });

    controller.speak('test');

    expect(controller.isUnlocked()).toBe(false);
  });
});
