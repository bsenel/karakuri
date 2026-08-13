import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'path';

// Unit tests run in jsdom against the same aliases the app builds with, so an
// import that works in a test works in the bundle.
//
// The end-to-end suite is deliberately not here: Playwright drives a real
// browser against a real server and is its own runner, its own CI job, and its
// own kind of slow. Mixing them means every unit run pays for a browser.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    // e2e/ is Playwright's. Without this exclusion vitest picks up its specs
    // and fails on an import it has no runner for.
    exclude: ['node_modules/**', 'e2e/**', 'dist/**'],
  },
});
