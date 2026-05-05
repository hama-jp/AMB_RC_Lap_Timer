import { AppRouter } from './router';

export function App(): JSX.Element {
  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col gap-6 px-6 py-10">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight text-slate-50">AMB RC Lap Timer</h1>
        <p className="mt-2 text-sm text-slate-400">
          SPA 骨格(#4)。WebSocket・設定 UI から順に、接続バナーとラップ表示を追加します。
        </p>
        <nav className="mt-4 flex gap-3 text-sm">
          <a className="text-cyan-300 underline-offset-4 hover:underline" href="#/">
            Main
          </a>
          <a className="text-cyan-300 underline-offset-4 hover:underline" href="#/settings">
            Settings
          </a>
        </nav>
      </header>
      <AppRouter />
    </main>
  );
}
