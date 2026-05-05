/// <reference types="vitest" />
import { resolve } from 'node:path';
import { defineConfig } from 'vite';

// Library mode build — the SPA wrapper (Issue #4) will import @amb-rc-lap-timer/web
// later. Until then, the build artifact is just the protocol parser bundle so we
// can verify the parser has no Node-only API leaking into production output.
export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, 'src/protocol/index.ts'),
      name: 'AmbP3',
      fileName: (format) => `protocol.${format}.js`,
      formats: ['es'],
    },
    target: 'es2022',
    sourcemap: true,
    emptyOutDir: true,
  },
  test: {
    globals: true,
    environment: 'node',
    include: ['tests/**/*.test.ts'],
  },
});
