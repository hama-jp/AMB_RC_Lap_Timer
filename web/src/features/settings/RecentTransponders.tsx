import {
  clearRecentTransponders,
  removeRecentTransponder,
  type SettingsStorage,
} from './settingsStore';

export interface RecentTranspondersProps {
  /** Current MRU list (decimal). Empty list hides the entire section. */
  readonly values: readonly number[];
  /** Called with the picked value's decimal string when a chip is tapped. */
  readonly onPick: (decimal: string) => void;
  /** Called after a chip is removed or the list is cleared, with the new list. */
  readonly onChange: (values: readonly number[]) => void;
  /** Optional storage injection (tests). */
  readonly storage?: SettingsStorage;
}

/**
 * "最近使った" trial chips below the transponder input (Issue #110).
 *
 * Tap-to-fill semantics: a chip writes its value into the input only — the
 * caller still has to press 保存 for it to take effect. Per-chip ✕ removes
 * one entry; the trailing "すべて消去" link wipes the whole list (no
 * confirmation: erroneous deletions are recoverable by saving the value
 * again on the next session).
 */
export function RecentTransponders({
  values,
  onPick,
  onChange,
  storage,
}: RecentTranspondersProps): JSX.Element | null {
  if (values.length === 0) {
    return null;
  }

  function handleRemove(value: number): void {
    removeRecentTransponder(value, storage);
    onChange(values.filter((entry) => entry !== value));
  }

  function handleClearAll(): void {
    clearRecentTransponders(storage);
    onChange([]);
  }

  return (
    <section aria-label="最近使ったトランスポンダー" className="flex flex-col gap-2">
      <p className="text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">最近使った</p>
      <div className="flex flex-wrap items-center gap-2">
        {values.map((value) => {
          const decimal = value.toString(10);
          return (
            <span
              key={value}
              className="inline-flex items-center gap-1 rounded-md border border-slate-700 bg-slate-800 pl-3 pr-1 py-1 font-mono text-sm text-slate-100"
            >
              <button
                aria-label={`${decimal} を入力欄に入れる`}
                className="text-slate-100 outline-none focus:ring-2 focus:ring-cyan-300/40"
                onClick={() => onPick(decimal)}
                type="button"
              >
                {decimal}
              </button>
              <button
                aria-label={`${decimal} を履歴から削除`}
                className="ml-1 rounded px-1 text-slate-400 transition hover:bg-slate-700 hover:text-slate-100 focus:outline-none focus:ring-2 focus:ring-cyan-300/40"
                onClick={() => handleRemove(value)}
                type="button"
              >
                ✕
              </button>
            </span>
          );
        })}
        <button
          aria-label="最近使ったトランスポンダーをすべて消去"
          className="ml-1 text-xs text-slate-500 transition hover:text-slate-300 focus:outline-none focus:underline"
          onClick={handleClearAll}
          type="button"
        >
          すべて消去
        </button>
      </div>
    </section>
  );
}
