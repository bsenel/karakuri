import { defineConfig } from '@playwright/test';

// The end-to-end suite drives a real browser against a real Karakuri server.
//
// It is a separate runner and a separate CI job from the unit tests on purpose:
// it is the only thing here that can catch a route guard that renders but does
// not navigate, and it is far too slow to run on every save.
//
// The server is started by the CI job rather than by webServer, because it needs
// a database, a bootstrap password and a built frontend — three things that are
// clearer as explicit steps than as a shell string in this file.
export default defineConfig({
  testDir: './e2e',
  // One worker: the suite logs in, edits limits and approves requests against a
  // single server, and those are not independent of each other.
  workers: 1,
  fullyParallel: false,
  // A failing e2e run is usually a real failure rather than a flake, and a
  // retry that hides one is worse than a red build.
  retries: 0,
  timeout: 30_000,
  use: {
    baseURL: process.env.KARAKURI_E2E_URL ?? 'http://127.0.0.1:8080',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
});
