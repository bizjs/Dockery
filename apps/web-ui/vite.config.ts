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
      // dockery-api (Go) runs locally via `make run` on :5001
      '/api': { target: 'http://localhost:5001', changeOrigin: true },
      // docker CLI token realm — same Go process
      '/token': { target: 'http://localhost:5001', changeOrigin: true },
      // Distribution registry runs in docker-compose.dev.yaml on host :5000
      '/v2': { target: 'http://localhost:5000', changeOrigin: true },
    },
  },
});
