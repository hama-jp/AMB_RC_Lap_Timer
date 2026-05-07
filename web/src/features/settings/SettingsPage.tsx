import { useState, type FormEvent } from 'react';

import { RecentTransponders } from './RecentTransponders';
import {
  addRecentTransponder,
  loadRecentTransponders,
  loadSettings,
  parseTransponderInput,
  saveSettings,
} from './settingsStore';

export function SettingsPage(): JSX.Element {
  const [initialSettings] = useState(() => loadSettings());
  const [transponderInput, setTransponderInput] = useState(() =>
    initialSettings.transponder === null ? '' : initialSettings.transponder.toString(10),
  );
  const [speechEnabled, setSpeechEnabled] = useState(initialSettings.speechEnabled);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [recents, setRecents] = useState<readonly number[]>(() => loadRecentTransponders());

  function handleSubmit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();

    const parsedTransponder = parseTransponderInput(transponderInput);
    if (!parsedTransponder.ok) {
      setSaved(false);
      setError(parsedTransponder.message);
      return;
    }

    saveSettings({
      transponder: parsedTransponder.value,
      speechEnabled,
    });
    if (parsedTransponder.value !== null) {
      addRecentTransponder(parsedTransponder.value);
      setRecents(loadRecentTransponders());
    }
    setTransponderInput(parsedTransponder.normalized);
    setError(null);
    setSaved(true);
  }

  return (
    <section className="rounded-xl border border-slate-800 bg-slate-900/70 p-5 shadow-xl shadow-slate-950/20">
      <div className="mb-5">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-cyan-300">Settings</p>
        <h2 className="mt-2 text-2xl font-semibold text-slate-50">設定</h2>
        <p className="mt-2 text-sm text-slate-400">
          トランスポンダーIDと音声読み上げの設定をこのブラウザーに保存します。
        </p>
      </div>

      <form className="flex flex-col gap-5" onSubmit={handleSubmit}>
        <label className="flex flex-col gap-2 text-sm font-medium text-slate-200">
          トランスポンダーID
          <input
            aria-label="トランスポンダーID"
            className="rounded-md border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-base text-slate-50 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/30"
            inputMode="text"
            onChange={(event) => {
              setTransponderInput(event.currentTarget.value);
              setSaved(false);
              setError(null);
            }}
            placeholder="例: 123456 または 0x1E240"
            type="text"
            value={transponderInput}
          />
          <span className="text-xs font-normal text-slate-500">
            空欄の場合は未設定です。0〜0xFFFFFFFF の10進数または16進数を指定できます。
          </span>
        </label>

        <RecentTransponders
          onChange={setRecents}
          onPick={(decimal) => {
            setTransponderInput(decimal);
            setSaved(false);
            setError(null);
          }}
          values={recents}
        />

        <label className="flex items-center gap-3 rounded-lg border border-slate-800 bg-slate-950/70 px-4 py-3 text-sm font-medium text-slate-200">
          <input
            aria-label="音声読み上げを有効にする"
            checked={speechEnabled}
            className="h-4 w-4 accent-cyan-400"
            onChange={(event) => {
              setSpeechEnabled(event.currentTarget.checked);
              setSaved(false);
            }}
            type="checkbox"
          />
          音声読み上げを有効にする
        </label>

        {error ? (
          <p
            className="rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-200"
            role="alert"
          >
            {error}
          </p>
        ) : null}

        {saved ? (
          <p
            className="rounded-md border border-emerald-500/40 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-200"
            role="status"
          >
            保存しました
          </p>
        ) : null}

        <button
          className="w-fit rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-cyan-200 focus:outline-none focus:ring-2 focus:ring-cyan-200 focus:ring-offset-2 focus:ring-offset-slate-950"
          type="submit"
        >
          保存
        </button>
      </form>
    </section>
  );
}
