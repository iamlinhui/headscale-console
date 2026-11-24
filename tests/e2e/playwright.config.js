import { defineConfig } from '@playwright/test'
export default defineConfig({
  testDir: '.', outputDir: '../../build/test-results', timeout: 30000, workers: 1,
  use: { baseURL: 'http://127.0.0.1:3081', browserName: 'chromium', channel: 'chrome', viewport: { width: 1440, height: 1000 } },
  webServer: [
    { cwd: process.cwd(), command: 'go run ./tests/fixtures/demo', url: 'http://127.0.0.1:3081/api/health', reuseExistingServer: false, timeout: 60000 },
    { cwd: process.cwd(), command: 'go run ./tests/fixtures/headscale', url: 'http://127.0.0.1:3082/api/health', reuseExistingServer: false, timeout: 60000 }
  ],
  reporter: 'list'
})
