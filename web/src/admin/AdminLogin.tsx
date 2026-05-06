import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';

import { AuthRequiredError, RateLimitedError, postAdminLogin } from './api';

export function AdminLogin(): JSX.Element {
  const navigate = useNavigate();
  const [passphrase, setPassphrase] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (passphrase === '' || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      await postAdminLogin(passphrase);
      // Strip the entered value before navigating away — the input is in
      // memory only, but a back-navigation should not refill it.
      setPassphrase('');
      navigate('/admin', { replace: true });
    } catch (err) {
      if (err instanceof AuthRequiredError) {
        setError('passphrase が違います');
      } else if (err instanceof RateLimitedError) {
        const seconds = Math.ceil(err.retryAfterMs / 1000);
        setError(`連続失敗のためロック中です。約 ${seconds} 秒後に再試行してください。`);
      } else {
        setError(err instanceof Error ? err.message : 'login failed');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="rounded-xl border border-slate-800 bg-slate-900/70 p-5 shadow-xl shadow-slate-950/20">
      <div className="mb-5">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-cyan-300">Admin</p>
        <h2 className="mt-2 text-2xl font-semibold text-slate-50">管理ログイン</h2>
        <p className="mt-2 text-sm text-slate-400">
          ゲートウェイ起動時にコンソールに表示された使い捨て passphrase
          を入力してください。再起動するたび新しい値になります。
        </p>
      </div>

      <form className="flex flex-col gap-4" onSubmit={(e) => void handleSubmit(e)}>
        <label className="flex flex-col gap-2 text-sm font-medium text-slate-200">
          Passphrase
          <input
            aria-label="passphrase"
            autoComplete="off"
            autoFocus
            className="rounded-md border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-base text-slate-50 outline-none transition focus:border-cyan-400 focus:ring-2 focus:ring-cyan-400/30"
            inputMode="text"
            onChange={(event) => setPassphrase(event.target.value)}
            type="password"
            value={passphrase}
          />
        </label>

        {error !== null ? (
          <p
            className="rounded-md border border-rose-700/50 bg-rose-950/40 px-3 py-2 text-sm text-rose-200"
            role="alert"
          >
            {error}
          </p>
        ) : null}

        <div>
          <button
            className="rounded-md bg-cyan-500 px-4 py-2 text-sm font-semibold text-slate-950 shadow transition enabled:hover:bg-cyan-400 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={submitting || passphrase === ''}
            type="submit"
          >
            {submitting ? '送信中…' : 'ログイン'}
          </button>
        </div>
      </form>
    </section>
  );
}
