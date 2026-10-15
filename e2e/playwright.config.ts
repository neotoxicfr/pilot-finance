import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // Sequential — shared DB state
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? 'github' : 'html',
  timeout: 30_000,

  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [
    // — Auth setup (runs once, shared by all browsers) —
    {
      name: 'setup',
      testMatch: /global-setup\.ts/,
    },

    // — Desktop browsers —
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
    },
    {
      name: 'firefox',
      use: {
        ...devices['Desktop Firefox'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
    },
    // WebKit is opt-in (`default: false`, Playwright ≥ 1.64) — flaky
    // cookie/storageState handling on Linux, so neither CI nor a plain
    // `npx playwright test` runs it. input.css carries an iOS-Safari-only fix
    // (@supports -webkit-touch-callout), so run
    // `npx playwright test --project=webkit` locally before a release.
    {
      name: 'webkit',
      default: false,
      use: {
        ...devices['Desktop Safari'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
    },

    // — Mobile viewport —
    {
      name: 'mobile-chrome',
      use: {
        ...devices['Pixel 7'],
        storageState: 'e2e/.auth/user.json',
      },
      dependencies: ['setup'],
    },
  ],
});
