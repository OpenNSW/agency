import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { resolve } from 'node:path'

// Only resolves the dev-server-only /config.js proxy target below (see
// server.proxy) — never affects `vite build` or vitest. Hardcoded to
// 'development' since that's the mode `pnpm dev` always resolves to. Merges
// frontend/.env with process.env (process.env wins), matching start-dev.sh's
// own precedence when it runs several agencies' dev servers side by side, each
// with its own VITE_API_BASE_URL.
const devEnv = loadEnv('development', process.cwd(), '')

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': resolve(import.meta.dirname, 'src'),
    },
  },
  server: {
    port: process.env.VITE_PORT ? parseInt(process.env.VITE_PORT, 10) : 5174,
    proxy: {
      // The backend is the single source of runtime config in dev and prod
      // alike (see backend/internal/web + src/runtimeConfig.ts): in prod one
      // server serves both the SPA and /config.js from one origin, and in dev
      // this proxies to that agency's own backend instance instead of
      // reimplementing config assembly here. Each agency's dev server proxies
      // to its own backend, since VITE_API_BASE_URL is already agency-specific
      // (see start-dev.sh), so running several agencies side by side still
      // gets each the right config.
      '/config.js': {
        target: devEnv.VITE_API_BASE_URL || 'http://localhost:8081',
        changeOrigin: true,
      },
    },
  },
})
