/** Configures React/Vite builds, the source alias, and development API proxy. */

import path from 'path';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from 'tailwindcss';
import autoprefixer from 'autoprefixer';
import tailwindConfig from './tailwind.config.js';

export default defineConfig(({ command }) => {
    // Use the active Wails loopback backend when developing the browser UI
    // alongside the desktop app: VIIB_API_PROXY_TARGET=http://127.0.0.1:34115.
    // Keep the target backend-only (not a VITE_* browser-exposed variable).
    const apiProxyTarget = (process.env.VIIB_API_PROXY_TARGET || 'http://127.0.0.1:8080').trim();
    const proxyURL = new URL(apiProxyTarget);
    if (proxyURL.protocol !== 'http:' ||
      !['127.0.0.1', 'localhost', '[::1]'].includes(proxyURL.hostname) ||
      proxyURL.username || proxyURL.password || proxyURL.pathname !== '/' ||
      proxyURL.search || proxyURL.hash) {
      throw new Error('VIIB_API_PROXY_TARGET must be a loopback HTTP origin, e.g. http://127.0.0.1:34115');
    }
    if (command === 'serve') console.info(`[ViiB] API proxy target: ${apiProxyTarget}`);
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
            target: apiProxyTarget,
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
