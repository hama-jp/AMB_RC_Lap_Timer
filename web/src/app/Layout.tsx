import type { ReactNode } from 'react';

import { StatusBanner } from '../features/status/StatusBanner';
import type { WsClient } from '../transport/wsClient';
import { Footer } from './Footer';

export interface LayoutProps {
  readonly wsClient: WsClient;
  readonly children: ReactNode;
}

export function Layout({ wsClient, children }: LayoutProps): JSX.Element {
  return (
    <main className="mx-auto flex min-h-screen max-w-5xl flex-col gap-6 px-4 py-6 sm:px-6 sm:py-10">
      <header className="flex flex-col gap-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-5 shadow-xl shadow-slate-950/20 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.22em] text-cyan-300">
            Trackside monitor
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight text-slate-50">
            AMB RC Lap Timer
          </h1>
          <p className="mt-2 max-w-2xl text-sm text-slate-400">
            ゲートウェイから流れる PASSING を対象トランスポンダーで絞り込みます。
          </p>
        </div>
        <nav className="flex gap-3 text-sm">
          <a className="text-cyan-300 underline-offset-4 hover:underline" href="#/">
            Main
          </a>
          <a className="text-cyan-300 underline-offset-4 hover:underline" href="#/settings">
            Settings
          </a>
          <a className="text-cyan-300 underline-offset-4 hover:underline" href="#/admin">
            Admin
          </a>
        </nav>
      </header>
      <StatusBanner wsClient={wsClient} />
      {children}
      <Footer />
    </main>
  );
}
