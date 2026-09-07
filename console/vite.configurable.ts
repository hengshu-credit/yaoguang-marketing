import { defineConfig } from 'vite'
import original from './vite.config'

// Same-origin browser verification against the isolated HTTP integration fixture.
export default defineConfig(async (env) => {
  const base = typeof original === 'function' ? await original(env) : await original
  const target = process.env.CONFIGURABLE_API_URL
  if (!target) throw new Error('CONFIGURABLE_API_URL must name an isolated integration fixture')
  return { ...base, server: { ...base.server, host: '0.0.0.0', port: 5173, hmr: { protocol: 'ws', clientPort: 5173 }, proxy: {
    '/config.js': { target, changeOrigin: true },
    '/console/config.js': { target, changeOrigin: true, rewrite: path => path.replace(/^\/console/, '') },
    '/api': { target, changeOrigin: true },
    '/track': { target, changeOrigin: true }
  } } }
})
