import { useState } from 'react';

import { createWsClient, type WsClient } from '../transport/wsClient';
import { Layout } from './Layout';
import { AppRouter } from './router';

export interface AppProps {
  readonly wsClient?: WsClient;
}

export function App({ wsClient: providedWsClient }: AppProps): JSX.Element {
  const [defaultWsClient] = useState(() => providedWsClient ?? createWsClient());
  const wsClient = providedWsClient ?? defaultWsClient;

  return (
    <Layout wsClient={wsClient}>
      <AppRouter wsClient={wsClient} />
    </Layout>
  );
}
