/** Configures React/Vite builds, the source alias, and development API proxy. */

import path from 'path';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from 'tailwindcss';
import autoprefixer from 'autoprefixer';
import tailwindConfig from './tailwind.config.js';

export default defineConfig(() => {
    return {
      css: {
        postcss: {
          plugins: [
            tailwindcss(tailwindConfig),
            autoprefixer(),
          ],
        },
      },
      server: {
        port: 3000,
        host: '0.0.0.0',
        // Proxy API requests to Go backend during development
        proxy: {
          '/api': {
            target: 'http://127.0.0.1:8080',
            changeOrigin: true,
          },
        },
      },
      plugins: [react()],
      worker: { format: 'es' as const },
      // API keys are stored securely in the backend (encrypted in SQLite).
      // Do NOT inject API keys into the frontend bundle via define.
      resolve: {
        alias: {
          '@': path.resolve(__dirname, '.'),
        }
      },
      build: {
        outDir: 'dist',
        sourcemap: false,
        minify: 'esbuild' as const,
      }
    };
});
