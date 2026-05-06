import { useEffect, useState } from 'react';

import { formatLapDelta, formatLapTime } from './formatLapTime';
import type { PassingsSnapshot, PassingsStore } from './passingsStore';

export interface LatestLapHeroProps {
  readonly store: PassingsStore;
}

export function LatestLapHero({ store }: LatestLapHeroProps): JSX.Element | null {
  const [snapshot, setSnapshot] = useState<PassingsSnapshot>(() => store.getSnapshot());

  useEffect(() => {
    const unsubscribe = store.subscribe(setSnapshot);
    setSnapshot(store.getSnapshot());
    return () => {
      unsubscribe();
    };
  }, [store]);

  if (snapshot.targetTransponder === null) {
    return null;
  }

  const latest = snapshot.passings[0];
  if (!latest) {
    return (
      <section
        aria-label="latest lap"
        className="rounded-xl border border-slate-800 bg-slate-900/60 px-6 py-8 text-center text-slate-400"
      >
        <p className="text-xs uppercase tracking-[0.3em] text-slate-500">Latest lap</p>
        <p className="mt-3 text-3xl font-semibold text-slate-300">— — —</p>
        <p className="mt-2 text-sm">PASSING を待機中</p>
      </section>
    );
  }

  const isBest =
    latest.lapTimeUs !== null &&
    snapshot.bestLapUs !== null &&
    latest.lapTimeUs === snapshot.bestLapUs;
  const delta = formatLapDelta(latest.lapTimeUs, snapshot.bestLapUs);

  return (
    <section
      aria-label="latest lap"
      className={`rounded-xl border px-6 py-6 text-center shadow-xl shadow-slate-950/30 ${
        isBest ? 'border-emerald-500/60 bg-emerald-900/30' : 'border-slate-800 bg-slate-900/70'
      }`}
    >
      <p className="text-xs uppercase tracking-[0.3em] text-cyan-300">Latest lap</p>
      <p
        className={`mt-2 font-mono text-6xl font-bold tabular-nums leading-none sm:text-7xl ${
          isBest ? 'text-emerald-200' : 'text-slate-50'
        }`}
      >
        {formatLapTime(latest.lapTimeUs)}
      </p>
      <div className="mt-3 flex flex-wrap items-baseline justify-center gap-3 text-sm">
        <span className="text-slate-400">Lap #{latest.record.passingNumber}</span>
        {delta !== '' ? (
          <span
            className={`font-mono text-base font-semibold ${
              isBest
                ? 'text-emerald-300'
                : delta.startsWith('+')
                  ? 'text-amber-300'
                  : 'text-cyan-300'
            }`}
          >
            {delta}
          </span>
        ) : null}
      </div>
    </section>
  );
}
