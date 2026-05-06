import { useEffect, useState } from 'react';

import { LapList } from '../features/laps/LapList';
import type { PassingsStore } from '../features/laps/passingsStore';
import { SettingsPage } from '../features/settings/SettingsPage';
import type { WsClient } from '../transport/wsClient';

type AppRoute = '/' | '/settings';

function normalizeHash(hash: string): AppRoute {
  const route = hash.startsWith('#') ? hash.slice(1) : hash;
  return route === '/settings' ? '/settings' : '/';
}

function getCurrentRoute(): AppRoute {
  return normalizeHash(window.location.hash);
}

export interface AppRouterProps {
  readonly wsClient: WsClient;
  readonly store: PassingsStore;
}

export function AppRouter({ store, wsClient }: AppRouterProps): JSX.Element {
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

  return <LapList store={store} wsClient={wsClient} />;
}
