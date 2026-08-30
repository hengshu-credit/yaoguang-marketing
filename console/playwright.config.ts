import { defineConfig, devices } from '@playwright/test'

const devHTTPS = process.env.VITE_DEV_HTTPS === 'true'
const baseURL = process.env.PLAYWRIGHT_BASE_URL || `${devHTTPS ? 'https' : 'http'}://${devHTTPS ? 'notifusedev.com' : 'localhost'}:5173`
const browserChannel = process.env.PLAYWRIGHT_CHANNEL

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['html'], ['list']],
  timeout: 30000,
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    ignoreHTTPSErrors: true
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], ...(browserChannel ? { channel: browserChannel } : {}) }
    }
  ],
  webServer: {
    command: 'npm run dev',
    url: `${baseURL}/console/`,
    reuseExistingServer: true,
    ignoreHTTPSErrors: true,
    timeout: 120000
  }
})
