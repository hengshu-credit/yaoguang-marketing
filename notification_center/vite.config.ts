import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { readFileSync } from 'fs'
import path from 'path'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const usePolling = env.VITE_USE_POLLING !== 'false'
  const useHTTPS = env.VITE_DEV_HTTPS === 'true'

  return {
    base: '/notification-center/',
    plugins: [tailwindcss(), react()],
    server: {
      host: env.VITE_DEV_HOST || '0.0.0.0',
      port: Number(env.VITE_DEV_PORT || 5174),
      strictPort: true,
      https: useHTTPS ? {
        key: readFileSync(env.VITE_DEV_KEY || path.resolve(__dirname, 'certificates/key.pem')),
        cert: readFileSync(env.VITE_DEV_CERT || path.resolve(__dirname, 'certificates/cert.pem')),
      } : undefined,
      watch: {
        usePolling,
        interval: Number(env.VITE_POLLING_INTERVAL || 500),
      },
      hmr: {
        protocol: useHTTPS ? 'wss' : 'ws',
        clientPort: Number(env.VITE_HMR_CLIENT_PORT || 8081),
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
  }
})
