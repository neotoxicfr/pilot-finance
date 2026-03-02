import { test as setup, expect } from '@playwright/test';
import { TEST_USER, registerUser, login } from './helpers';

setup('register and authenticate', async ({ page }) => {
  // Check if user already exists by trying to go to dashboard
  await page.goto('/');
  const url = page.url();

  if (url.includes('/login')) {
    // Not logged in — try to register first (fresh DB in CI)
    try {
      await registerUser(page, TEST_USER.email, TEST_USER.password);
    } catch {
      // Registration failed (user already exists or disabled) — navigate fresh and login
      await page.goto('/login');
      await login(page, TEST_USER.email, TEST_USER.password);
    }
  }

  // Verify we're on the dashboard
  await expect(page).toHaveURL('/', { timeout: 15000 });
  await expect(page.locator('nav')).toBeVisible();
  // Save storage state (JWT cookie) for all subsequent tests
  await page.context().storageState({ path: 'e2e/.auth/user.json' });
});
