import { HashRouter, Navigate, Route, Routes } from 'react-router-dom';

import { AdminLogin } from '../admin/AdminLogin';
import { AdminPage } from '../admin/AdminPage';
import { LapList } from '../features/laps/LapList';
import { LatestLapHero } from '../features/laps/LatestLapHero';
import type { PassingsStore } from '../features/laps/passingsStore';
import { SettingsPage } from '../features/settings/SettingsPage';
import type { WsClient } from '../transport/wsClient';

export interface AppRouterProps {
  readonly wsClient: WsClient;
  readonly store: PassingsStore;
}

function LapView({ store, wsClient }: AppRouterProps): JSX.Element {
  return (
    <div className="flex flex-col gap-4">
      <LatestLapHero store={store} />
      <LapList store={store} wsClient={wsClient} />
    </div>
  );
}

/**
 * The SPA mounts under HashRouter so the gateway can keep serving index.html
 * for `/` only — every UI route, including `/admin/*`, lives behind the `#`.
 * Switching to BrowserRouter would force the gateway's static handler to
 * fall back to index.html for unknown paths, which collides with the
 * existing `/admin` and `/admin/api/*` Go routes (PR #88 / #92).
 */
export function AppRouter(props: AppRouterProps): JSX.Element {
  return (
    <HashRouter>
      <Routes>
        <Route path="/" element={<LapView {...props} />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/admin/login" element={<AdminLogin />} />
        <Route path="/admin" element={<AdminPage />} />
        <Route path="*" element={<Navigate replace to="/" />} />
      </Routes>
    </HashRouter>
  );
}
