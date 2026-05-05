import { useEffect, useState } from 'react';

import { fetchGatewayVersion } from './healthz';

export function Footer(): JSX.Element {
  const [version, setVersion] = useState('unknown');

  useEffect(() => {
    let active = true;
    void fetchGatewayVersion().then((nextVersion) => {
      if (active) {
        setVersion(nextVersion);
      }
    });
    return () => {
      active = false;
    };
  }, []);

  return (
    <footer className="border-t border-slate-800 pt-4 text-xs text-slate-500">
      gateway version: <span className="font-mono text-slate-300">{version}</span>
    </footer>
  );
}
