import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from 'path';

// https://vite.dev/config/
export default defineConfig({
  plugins: [react({}), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      // dockery-api (Go) runs locally via `make run` on :5001
      '/api': { target: 'http://localhost:5001', changeOrigin: true },
      // docker CLI token realm — same Go process
      '/token': { target: 'http://localhost:5001', changeOrigin: true },
      // Distribution registry runs in docker-compose.dev.yaml on host :4999
      '/v2': { target: 'http://localhost:5000', changeOrigin: true },
    },
  },
});
