import { useEffect, useState } from 'react';

import { SettingsPage } from '../features/settings/SettingsPage';

type AppRoute = '/' | '/settings';

function normalizeHash(hash: string): AppRoute {
  const route = hash.startsWith('#') ? hash.slice(1) : hash;
  return route === '/settings' ? '/settings' : '/';
}

function getCurrentRoute(): AppRoute {
  return normalizeHash(window.location.hash);
}

export function AppRouter(): JSX.Element {
  const [route, setRoute] = useState<AppRoute>(() => getCurrentRoute());

  useEffect(() => {
    function handleHashChange(): void {
      setRoute(getCurrentRoute());
    }

    window.addEventListener('hashchange', handleHashChange);
    handleHashChange();
    return () => {
      window.removeEventListener('hashchange', handleHashChange);
    };
  }, []);

  if (route === '/settings') {
    return <SettingsPage />;
  }

  return (
    <section className="rounded-md border border-slate-800 bg-slate-900/60 p-4 text-sm text-slate-300">
      <p>
        このページが表示されているということは、
        <code className="font-mono text-slate-100">scripts/build.ps1</code> →{' '}
        <code className="font-mono text-slate-100">go:embed</code> →{' '}
        <code className="font-mono text-slate-100">gateway.exe</code> の流れが繋がっています。
      </p>
    </section>
  );
}
