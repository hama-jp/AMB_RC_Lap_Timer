import { useEffect, useState } from 'react';

import { createPassingsStore } from '../features/laps/passingsStore';
import { SpeechCoordinator } from '../features/speech/SpeechCoordinator';
import { SpeechUnlockOverlay } from '../features/speech/SpeechUnlockOverlay';
import { createSpeechController, type SpeechController } from '../features/speech/speechController';
import { createWsClient, type WsClient } from '../transport/wsClient';
import { Layout } from './Layout';
import { AppRouter } from './router';

export interface AppProps {
  readonly wsClient?: WsClient;
  readonly speechController?: SpeechController;
}

export function App({
  wsClient: providedWsClient,
  speechController: providedSpeechController,
}: AppProps): JSX.Element {
  const [defaultWsClient] = useState(() => providedWsClient ?? createWsClient());
  const wsClient = providedWsClient ?? defaultWsClient;
  const [store] = useState(() => createPassingsStore({ wsClient }));
  const [defaultSpeechController] = useState(
    () => providedSpeechController ?? createSpeechController(),
  );
  const speechController = providedSpeechController ?? defaultSpeechController;

  useEffect(() => store.start(), [store]);

  return (
    <Layout wsClient={wsClient}>
      <SpeechCoordinator controller={speechController} store={store} />
      <SpeechUnlockOverlay controller={speechController} />
      <AppRouter store={store} wsClient={wsClient} />
    </Layout>
  );
}
