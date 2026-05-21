import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// During `npm run dev`, /api/v1/* requests are proxied to the local Karakuri
// server on :8080. The same paths work in production because the Go server
// embeds web/dist/ at / and routes /api/v1/* before the SPA fallback.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
});
