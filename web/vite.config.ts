/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// SPA mode (Issue #55, parent #4): Vite serves index.html as the entry,
// builds a hashed asset bundle, and outputs to web/dist/. scripts/build.ps1
// then copies that into gateway/internal/webassets/dist/ for go:embed.
//
// Tests run with the protocol/ unit suites (Node env) plus the React app
// component tests (jsdom env). Vitest picks the env per-file via a docblock
// pragma; the workspace default is jsdom for *.test.tsx and node for the
// existing *.test.ts under tests/protocol/.
export default defineConfig({
  appType: 'spa',
  plugins: [react()],
  build: {
    target: 'es2022',
    sourcemap: true,
    emptyOutDir: true,
  },
  test: {
    globals: true,
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx', 'tests/**/*.test.ts', 'tests/**/*.test.tsx'],
    environmentMatchGlobs: [['{src,tests}/**/*.test.tsx', 'jsdom']],
  },
});
