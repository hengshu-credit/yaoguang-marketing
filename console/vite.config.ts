/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { lingui } from '@lingui/vite-plugin'
import { loadEnv } from 'vite'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'
import { readFileSync } from 'fs'
import path from 'path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const usePolling = env.VITE_USE_POLLING !== 'false'
  const useHTTPS = env.VITE_DEV_HTTPS === 'true'
  const backendURL = env.VITE_BACKEND_URL || 'https://localapi.notifuse.com:4000'

  return {
    base: '/console/',
    plugins: [
      react({
        babel: {
          plugins: ['@lingui/babel-plugin-lingui-macro'],
        },
      }),
      tailwindcss(),
      lingui(),
    ],
    server: {
      host: env.VITE_DEV_HOST || '0.0.0.0',
      port: Number(env.VITE_DEV_PORT || 5173),
      strictPort: true,
      https: useHTTPS ? {
        key: readFileSync(env.VITE_DEV_KEY || resolve(__dirname, 'certificates/key.pem')),
        cert: readFileSync(env.VITE_DEV_CERT || resolve(__dirname, 'certificates/cert.pem')),
      } : undefined,
      watch: {
        usePolling,
        interval: Number(env.VITE_POLLING_INTERVAL || 500),
      },
      hmr: {
        protocol: useHTTPS ? 'wss' : 'ws',
        clientPort: Number(env.VITE_HMR_CLIENT_PORT || 8081),
      },
      proxy: {
        '/config.js': {
          target: backendURL,
          changeOrigin: true,
          secure: false,
          rewrite: (path) => path.replace(/^\/console/, ''),
        },
        '/console/config.js': {
          target: backendURL,
          changeOrigin: true,
          secure: false,
          rewrite: (path) => path.replace(/^\/console/, ''),
        },
      },
    },
    test: {
      globals: true,
      environment: 'jsdom',
      setupFiles: ['./src/__tests__/setup.tsx'],
      include: ['**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
      coverage: {
        reporter: ['text', 'json', 'html'],
        exclude: ['node_modules/', 'src/__tests__/setup.tsx'],
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
      extensions: ['.js', '.jsx', '.ts', '.tsx', '.json'],
    },
    optimizeDeps: {
      include: ['@fortawesome/react-fontawesome', '@fortawesome/fontawesome-svg-core'],
    },
  }
})
