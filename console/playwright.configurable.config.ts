import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e/features', testMatch: 'configurable-marketing.spec.ts',
  workers: 1, retries: 0, timeout: 90000, reporter: 'list',
  use: {
    ...devices['Desktop Chrome'], baseURL: 'http://127.0.0.1:5173',
    viewport: { width: 1600, height: 1000 }, screenshot: 'only-on-failure', trace: 'retain-on-failure',
    launchOptions: process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH, args: ['--no-sandbox'] } : {}
  }
})
