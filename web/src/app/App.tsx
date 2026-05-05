/**
 * Root SPA component.
 *
 * #4-A scope: render the title with enough Tailwind to confirm the build
 * pipeline and asset embedding work. Real layout (#4-E / #59) and feature
 * panels (#4-B / #4-C / #4-D) arrive in subsequent waves.
 */
export function App(): JSX.Element {
  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col gap-6 px-6 py-10">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight text-slate-50">AMB RC Lap Timer</h1>
        <p className="mt-2 text-sm text-slate-400">
          SPA 骨格(#4-A)。WebSocket・設定 UI・接続バナー・ラップ表示は順次追加されます。
        </p>
      </header>
      <section className="rounded-md border border-slate-800 bg-slate-900/60 p-4 text-sm text-slate-300">
        <p>
          このページが表示されているということは、
          <code className="font-mono text-slate-100">scripts/build.ps1</code> →{' '}
          <code className="font-mono text-slate-100">go:embed</code> →{' '}
          <code className="font-mono text-slate-100">gateway.exe</code> の流れが繋がっています。
        </p>
      </section>
    </main>
  );
}
