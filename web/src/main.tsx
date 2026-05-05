/**
 * Entry point for the AMB RC Lap Timer SPA.
 *
 * Issue #4 (parent #54) splits the SPA into 5 waves:
 *   #4-A (#55) — this file: SPA scaffolding only, render an empty App
 *   #4-B (#56) — WebSocket client wiring
 *   #4-C (#57) — settings UI + localStorage
 *   #4-D (#58) — connection-state banner
 *   #4-E (#59) — PASSING list and overall layout
 *
 * The `web/src/protocol/` parser (PR #51) is intentionally untouched here.
 */
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from './app/App';
import './index.css';

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('#root element not found in index.html');
}

createRoot(rootEl).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
