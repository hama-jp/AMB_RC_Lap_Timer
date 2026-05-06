import { useEffect, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';

import {
  AuthRequiredError,
  ValidationErrorList,
  fetchAdminConfig,
  postAdminConfig,
  postAdminLogout,
  type AdminConfig,
  type ValidationError,
} from './api';
import { APPLY_TIMING_LABEL, applyTimingFor } from './applyTimings';
import { ADMIN_CONFIG_DEFAULTS } from './defaults';

type ErrorMap = Record<string, string>;

interface SaveOutcome {
  applied: string[];
  requiresRestart: string[];
  changedCount: number;
}

export function AdminPage(): JSX.Element {
  const navigate = useNavigate();
  const [current, setCurrent] = useState<AdminConfig | null>(null);
  const [draft, setDraft] = useState<AdminConfig | null>(null);
  const [errors, setErrors] = useState<ErrorMap>({});
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const cfg = await fetchAdminConfig();
        if (!cancelled) {
          setCurrent(cfg);
          setDraft(cfg);
        }
      } catch (err) {
        if (cancelled) return;
        if (err instanceof AuthRequiredError) {
          navigate('/admin/login', { replace: true });
          return;
        }
        setLoadError(err instanceof Error ? err.message : 'failed to load config');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [navigate]);

  function setField<K extends string>(path: K, value: unknown): void {
    if (!draft) return;
    setOutcome(null);
    setDraft(setPath(draft, path, value));
    if (errors[path] !== undefined) {
      // Clear stale error on the field the user is editing.
      const next = { ...errors };
      delete next[path];
      setErrors(next);
    }
  }

  async function handleSave(): Promise<void> {
    if (!draft || saving) return;
    setSaving(true);
    setErrors({});
    setOutcome(null);
    try {
      const result = await postAdminConfig(draft);
      setCurrent(result.config);
      setDraft(result.config);
      setOutcome({
        applied: result.applied ?? [],
        requiresRestart: result.requires_restart ?? [],
        changedCount: (result.changed_fields ?? []).length,
      });
    } catch (err) {
      if (err instanceof AuthRequiredError) {
        navigate('/admin/login', { replace: true });
        return;
      }
      if (err instanceof ValidationErrorList) {
        setErrors(toErrorMap(err.errors));
        return;
      }
      setLoadError(err instanceof Error ? err.message : 'save failed');
    } finally {
      setSaving(false);
    }
  }

  function handleResetToDefaults(): void {
    setOutcome(null);
    setErrors({});
    setDraft(ADMIN_CONFIG_DEFAULTS);
  }

  async function handleLogout(): Promise<void> {
    await postAdminLogout();
    navigate('/admin/login', { replace: true });
  }

  if (loadError !== null) {
    return (
      <section className="rounded-xl border border-rose-700/50 bg-rose-950/40 p-5 text-sm text-rose-100">
        <p className="font-semibold">設定の読み込みに失敗しました</p>
        <p className="mt-2 font-mono text-xs">{loadError}</p>
      </section>
    );
  }
  if (current === null || draft === null) {
    return (
      <section className="rounded-xl border border-slate-800 bg-slate-900/70 p-5 text-sm text-slate-400">
        設定を読み込んでいます…
      </section>
    );
  }

  return (
    <section className="flex flex-col gap-5">
      <header className="rounded-xl border border-slate-800 bg-slate-900/70 p-5 shadow-xl shadow-slate-950/20">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-cyan-300">Admin</p>
            <h2 className="mt-2 text-2xl font-semibold text-slate-50">サーバ設定</h2>
            <p className="mt-2 text-sm text-slate-400">
              ゲートウェイの <code className="font-mono">config.json</code> を編集します。
              <span className="ml-1 text-slate-500">
                走行者個人の設定(対象トランスポンダー / 音声)は{' '}
                <a className="text-cyan-300 underline-offset-4 hover:underline" href="#/settings">
                  /settings
                </a>{' '}
                を使ってください。
              </span>
            </p>
          </div>
          <button
            className="rounded-md border border-slate-700 px-3 py-1.5 text-xs text-slate-200 hover:bg-slate-800"
            onClick={() => void handleLogout()}
            type="button"
          >
            ログアウト
          </button>
        </div>
      </header>

      {outcome !== null ? (
        <div
          className="rounded-md border border-emerald-700/50 bg-emerald-950/40 px-4 py-3 text-sm text-emerald-100"
          role="status"
        >
          <p className="font-semibold">保存しました({outcome.changedCount} 項目変更)</p>
          {outcome.requiresRestart.length > 0 ? (
            <p className="mt-1 text-emerald-200">
              次の項目はゲートウェイ再起動後に有効になります:{' '}
              <span className="font-mono">{outcome.requiresRestart.join(', ')}</span>
            </p>
          ) : null}
        </div>
      ) : null}

      <Section title="Upstream">
        <Field
          current={current.upstream.host}
          error={errors['upstream.host']}
          label="Host"
          onChange={(v) => setField('upstream.host', v)}
          path="upstream.host"
          value={draft.upstream.host}
        />
        <NumberField
          current={current.upstream.port}
          error={errors['upstream.port']}
          label="Port"
          onChange={(v) => setField('upstream.port', v)}
          path="upstream.port"
          value={draft.upstream.port}
        />
        <NumberField
          current={current.upstream.reconnect.initial_ms}
          error={errors['upstream.reconnect.initial_ms']}
          label="Reconnect initial (ms)"
          onChange={(v) => setField('upstream.reconnect.initial_ms', v)}
          path="upstream.reconnect.initial_ms"
          value={draft.upstream.reconnect.initial_ms}
        />
        <NumberField
          current={current.upstream.reconnect.max_ms}
          error={errors['upstream.reconnect.max_ms']}
          label="Reconnect max (ms)"
          onChange={(v) => setField('upstream.reconnect.max_ms', v)}
          path="upstream.reconnect.max_ms"
          value={draft.upstream.reconnect.max_ms}
        />
        <NumberField
          current={current.upstream.reconnect.jitter_ratio}
          error={errors['upstream.reconnect.jitter_ratio']}
          label="Reconnect jitter (0–1)"
          onChange={(v) => setField('upstream.reconnect.jitter_ratio', v)}
          path="upstream.reconnect.jitter_ratio"
          step="0.05"
          value={draft.upstream.reconnect.jitter_ratio}
        />
      </Section>

      <Section title="Server">
        <Field
          current={current.listen}
          error={errors['listen']}
          label="Listen"
          onChange={(v) => setField('listen', v)}
          path="listen"
          value={draft.listen}
        />
        <NumberField
          current={current.server.max_clients}
          error={errors['server.max_clients']}
          label="Max clients"
          onChange={(v) => setField('server.max_clients', v)}
          path="server.max_clients"
          value={draft.server.max_clients}
        />
        <NumberField
          current={current.server.client_buffer_len}
          error={errors['server.client_buffer_len']}
          label="Client buffer length"
          onChange={(v) => setField('server.client_buffer_len', v)}
          path="server.client_buffer_len"
          value={draft.server.client_buffer_len}
        />
      </Section>

      <Section title="Logging">
        <Field
          current={current.logging.dir}
          error={errors['logging.dir']}
          label="Log directory"
          onChange={(v) => setField('logging.dir', v)}
          path="logging.dir"
          value={draft.logging.dir}
        />
        <NumberField
          current={current.logging.max_size_mb}
          error={errors['logging.max_size_mb']}
          label="Max size (MB)"
          onChange={(v) => setField('logging.max_size_mb', v)}
          path="logging.max_size_mb"
          value={draft.logging.max_size_mb}
        />
        <NumberField
          current={current.logging.max_backups}
          error={errors['logging.max_backups']}
          label="Max backups"
          onChange={(v) => setField('logging.max_backups', v)}
          path="logging.max_backups"
          value={draft.logging.max_backups}
        />
      </Section>

      <Section title="Recording">
        <Field
          current={current.records.dir}
          error={errors['records.dir']}
          label="Records directory"
          onChange={(v) => setField('records.dir', v)}
          path="records.dir"
          value={draft.records.dir}
        />
      </Section>

      <Section title="Replay">
        <SelectField
          current={current.replay.speed}
          error={errors['replay.speed']}
          label="Speed"
          onChange={(v) => setField('replay.speed', v)}
          options={['realtime', 'fast', 'instant']}
          path="replay.speed"
          value={draft.replay.speed}
        />
      </Section>

      <div className="sticky bottom-0 -mx-4 flex items-center justify-end gap-3 border-t border-slate-800 bg-slate-950/95 px-4 py-3 backdrop-blur sm:-mx-6 sm:px-6">
        <button
          className="rounded-md border border-slate-700 px-4 py-2 text-sm text-slate-200 hover:bg-slate-800"
          onClick={handleResetToDefaults}
          type="button"
        >
          初期値に戻す
        </button>
        <button
          className="rounded-md bg-cyan-500 px-4 py-2 text-sm font-semibold text-slate-950 shadow transition enabled:hover:bg-cyan-400 disabled:cursor-not-allowed disabled:opacity-60"
          disabled={saving}
          onClick={() => void handleSave()}
          type="button"
        >
          {saving ? '保存中…' : '保存'}
        </button>
      </div>
    </section>
  );
}

function Section({
  title,
  children,
}: {
  readonly title: string;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <details className="rounded-xl border border-slate-800 bg-slate-900/70 p-5" open>
      <summary className="cursor-pointer text-sm font-semibold uppercase tracking-[0.2em] text-cyan-300">
        {title}
      </summary>
      <div className="mt-4 flex flex-col gap-4">{children}</div>
    </details>
  );
}

interface FieldProps {
  readonly path: string;
  readonly label: string;
  readonly value: string;
  readonly current: string;
  readonly error: string | undefined;
  readonly onChange: (next: string) => void;
}

function Field({ path, label, value, current, error, onChange }: FieldProps): JSX.Element {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_minmax(0,1.4fr)_auto] sm:items-start">
      <div className="flex flex-col gap-1">
        <span className="text-sm font-medium text-slate-200">{label}</span>
        <span className="font-mono text-[11px] text-slate-500">{path}</span>
        <span className="text-[11px] text-slate-500">
          現在値:{' '}
          <span className="font-mono text-slate-300">{current === '' ? '(empty)' : current}</span>
        </span>
      </div>
      <div className="flex flex-col gap-1">
        <input
          aria-label={path}
          className={`rounded-md border bg-slate-950 px-3 py-2 font-mono text-sm text-slate-50 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/30 ${
            error !== undefined ? 'border-rose-600' : 'border-slate-700'
          }`}
          onChange={(e) => onChange(e.target.value)}
          type="text"
          value={value}
        />
        {error !== undefined ? (
          <p className="text-xs text-rose-300" role="alert">
            {error}
          </p>
        ) : null}
      </div>
      <ApplyTimingBadge path={path} />
    </div>
  );
}

interface NumberFieldProps {
  readonly path: string;
  readonly label: string;
  readonly value: number;
  readonly current: number;
  readonly error: string | undefined;
  readonly onChange: (next: number) => void;
  readonly step?: string;
}

function NumberField({
  path,
  label,
  value,
  current,
  error,
  onChange,
  step,
}: NumberFieldProps): JSX.Element {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_minmax(0,1.4fr)_auto] sm:items-start">
      <div className="flex flex-col gap-1">
        <span className="text-sm font-medium text-slate-200">{label}</span>
        <span className="font-mono text-[11px] text-slate-500">{path}</span>
        <span className="text-[11px] text-slate-500">
          現在値: <span className="font-mono text-slate-300">{current}</span>
        </span>
      </div>
      <div className="flex flex-col gap-1">
        <input
          aria-label={path}
          className={`rounded-md border bg-slate-950 px-3 py-2 font-mono text-sm text-slate-50 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/30 ${
            error !== undefined ? 'border-rose-600' : 'border-slate-700'
          }`}
          onChange={(e) => {
            // Empty input → 0; the validator will reject it later if the field
            // requires >= 1, surfacing a precise per-field message.
            const n = e.target.value === '' ? 0 : Number(e.target.value);
            onChange(Number.isFinite(n) ? n : 0);
          }}
          step={step}
          type="number"
          value={value}
        />
        {error !== undefined ? (
          <p className="text-xs text-rose-300" role="alert">
            {error}
          </p>
        ) : null}
      </div>
      <ApplyTimingBadge path={path} />
    </div>
  );
}

interface SelectFieldProps {
  readonly path: string;
  readonly label: string;
  readonly value: string;
  readonly current: string;
  readonly options: readonly string[];
  readonly error: string | undefined;
  readonly onChange: (next: string) => void;
}

function SelectField({
  path,
  label,
  value,
  current,
  options,
  error,
  onChange,
}: SelectFieldProps): JSX.Element {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_minmax(0,1.4fr)_auto] sm:items-start">
      <div className="flex flex-col gap-1">
        <span className="text-sm font-medium text-slate-200">{label}</span>
        <span className="font-mono text-[11px] text-slate-500">{path}</span>
        <span className="text-[11px] text-slate-500">
          現在値: <span className="font-mono text-slate-300">{current}</span>
        </span>
      </div>
      <div className="flex flex-col gap-1">
        <select
          aria-label={path}
          className={`rounded-md border bg-slate-950 px-3 py-2 font-mono text-sm text-slate-50 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/30 ${
            error !== undefined ? 'border-rose-600' : 'border-slate-700'
          }`}
          onChange={(e) => onChange(e.target.value)}
          value={value}
        >
          {options.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </select>
        {error !== undefined ? (
          <p className="text-xs text-rose-300" role="alert">
            {error}
          </p>
        ) : null}
      </div>
      <ApplyTimingBadge path={path} />
    </div>
  );
}

function ApplyTimingBadge({ path }: { readonly path: string }): JSX.Element {
  const timing = applyTimingFor(path);
  const colour =
    timing === 'restart'
      ? 'border-amber-700 bg-amber-950/60 text-amber-200'
      : timing === 'immediate'
        ? 'border-emerald-700 bg-emerald-950/60 text-emerald-200'
        : 'border-slate-700 bg-slate-900 text-slate-300';
  return (
    <span
      className={`self-start rounded-md border px-2 py-1 text-[10px] font-semibold uppercase tracking-wider ${colour}`}
    >
      {APPLY_TIMING_LABEL[timing]}
    </span>
  );
}

// setPath returns a deep copy of cfg with the dotted path set to value. The
// AdminConfig shape is small and known so a hand-rolled switch is clearer
// than a generic deep-set utility (and avoids a lodash dependency).
function setPath<K extends string>(cfg: AdminConfig, path: K, value: unknown): AdminConfig {
  const next: AdminConfig = JSON.parse(JSON.stringify(cfg)) as AdminConfig;
  switch (path) {
    case 'listen':
      next.listen = value as string;
      break;
    case 'upstream.host':
      next.upstream.host = value as string;
      break;
    case 'upstream.port':
      next.upstream.port = value as number;
      break;
    case 'upstream.reconnect.initial_ms':
      next.upstream.reconnect.initial_ms = value as number;
      break;
    case 'upstream.reconnect.max_ms':
      next.upstream.reconnect.max_ms = value as number;
      break;
    case 'upstream.reconnect.jitter_ratio':
      next.upstream.reconnect.jitter_ratio = value as number;
      break;
    case 'logging.dir':
      next.logging.dir = value as string;
      break;
    case 'logging.max_size_mb':
      next.logging.max_size_mb = value as number;
      break;
    case 'logging.max_backups':
      next.logging.max_backups = value as number;
      break;
    case 'records.dir':
      next.records.dir = value as string;
      break;
    case 'replay.speed':
      next.replay.speed = value as string;
      break;
    case 'server.max_clients':
      next.server.max_clients = value as number;
      break;
    case 'server.client_buffer_len':
      next.server.client_buffer_len = value as number;
      break;
  }
  return next;
}

function toErrorMap(errors: ValidationError[]): ErrorMap {
  const out: ErrorMap = {};
  for (const e of errors) {
    out[e.path] = e.message;
  }
  return out;
}
