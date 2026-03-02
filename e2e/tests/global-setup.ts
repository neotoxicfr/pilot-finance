import { test as setup, expect } from '@playwright/test';
import { TEST_USER, registerUser, login } from './helpers';

setup('register and authenticate', async ({ page }) => {
  // Try to register — if user already exists, fall back to login
  try {
    await registerUser(page, TEST_USER.email, TEST_USER.password);
  } catch {
    // Registration failed (email already used) — login instead
    await login(page, TEST_USER.email, TEST_USER.password);
  }
  // Verify we're on the dashboard
  await expect(page).toHaveURL('/');
  await expect(page.locator('nav')).toBeVisible();
  // Save storage state (JWT cookie) for all subsequent tests
  await page.context().storageState({ path: 'e2e/.auth/user.json' });
});
