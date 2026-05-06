import { useEffect, useMemo, useState } from 'react';

import type { WsClient } from '../../transport/wsClient';
import { formatLapTime } from './formatLapTime';
import { createPassingsStore, type PassingsSnapshot } from './passingsStore';

export interface LapListProps {
  readonly wsClient: WsClient;
}

const INITIAL_SNAPSHOT: PassingsSnapshot = {
  targetTransponder: null,
  passings: [],
};

export function LapList({ wsClient }: LapListProps): JSX.Element {
  const store = useMemo(() => createPassingsStore({ wsClient }), [wsClient]);
  const [snapshot, setSnapshot] = useState<PassingsSnapshot>(() => store.getSnapshot());

  useEffect(() => {
    const unsubscribe = store.subscribe(setSnapshot);
    const stop = store.start();
    setSnapshot(store.getSnapshot());
    return () => {
      unsubscribe();
      stop();
    };
  }, [store]);

  if (snapshot.targetTransponder === null) {
    return (
      <section className="rounded-xl border border-dashed border-slate-700 bg-slate-900/50 p-6 text-sm text-slate-300">
        設定画面で対象トランスポンダーを入力してください。
      </section>
    );
  }

  if (snapshot.passings.length === 0) {
    return (
      <section className="rounded-xl border border-slate-800 bg-slate-900/60 p-6 text-sm text-slate-300">
        <p className="font-medium text-slate-100">
          トランスポンダー {snapshot.targetTransponder} の PASSING を待機中です。
        </p>
        <p className="mt-2 text-slate-400">受信すると新しい順に最大 50 件を表示します。</p>
      </section>
    );
  }

  return (
    <section className="overflow-hidden rounded-xl border border-slate-800 bg-slate-900/70 shadow-xl shadow-slate-950/20">
      <div className="border-b border-slate-800 px-4 py-3">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-cyan-300">Passings</p>
        <h2 className="mt-1 text-xl font-semibold text-slate-50">
          Transponder {snapshot.targetTransponder}
        </h2>
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-800 text-left text-sm">
          <thead className="bg-slate-950/60 text-xs uppercase tracking-wide text-slate-400">
            <tr>
              <th className="px-4 py-3">Passing #</th>
              <th className="px-4 py-3">Lap</th>
              <th className="px-4 py-3">RTC Time</th>
              <th className="px-4 py-3">Strength</th>
              <th className="px-4 py-3">Hits</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800 text-slate-200">
            {snapshot.passings.map(({ record, lapTimeUs }) => (
              <tr key={`${record.passingNumber}-${record.rtcTimeUs.toString()}`}>
                <td className="px-4 py-3 font-mono text-slate-50">{record.passingNumber}</td>
                <td className="px-4 py-3 font-mono text-cyan-100">{formatLapTime(lapTimeUs)}</td>
                <td className="px-4 py-3 font-mono">{formatRtcTime(record.rtcTimeUs)}</td>
                <td className="px-4 py-3 font-mono">{record.strength}</td>
                <td className="px-4 py-3 font-mono">{record.hits}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function formatRtcTime(rtcTimeUs: bigint): string {
  const wholeSeconds = rtcTimeUs / 1_000_000n;
  const fractionalUs = rtcTimeUs % 1_000_000n;
  return `${wholeSeconds.toString()}.${fractionalUs.toString().padStart(6, '0')} s`;
}

export function createInitialLapSnapshotForTest(): PassingsSnapshot {
  return INITIAL_SNAPSHOT;
}
