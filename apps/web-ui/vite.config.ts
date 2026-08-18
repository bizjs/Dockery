import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

// APP_VERSION is baked into the bundle at build time and rendered in
// the footer. The Dockerfile requires it as a build-arg; dev builds
// outside Docker (pnpm dev / pnpm build) default to "dev".
const APP_VERSION = process.env.APP_VERSION || 'dev';

// https://vite.dev/config/
export default defineConfig({
  plugins: [react({}), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  define: {
    __APP_VERSION__: JSON.stringify(APP_VERSION),
  },
  server: {
    proxy: {
      // :5001 is the `make dev` all-in-one container (nginx fronting
      // dockery-api + registry), or a bare-metal `make run` api — both
      // answer /api and /token on the same port.
      '/api': { target: 'http://localhost:5001', changeOrigin: true },
      // docker CLI token realm — same upstream
      '/token': { target: 'http://localhost:5001', changeOrigin: true },
      // Raw registry API. The UI itself never calls /v2 (it goes through
      // /api/registry/*); kept for poking the registry from the browser.
      // With `make dev`, nginx exposes /v2 on :5001 as well.
      '/v2': { target: 'http://localhost:5001', changeOrigin: true },
    },
  },
});
