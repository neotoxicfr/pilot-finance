import { test, expect } from '@playwright/test';
import { TEST_USER, login, logout } from './helpers';

test.describe('Authentication', () => {
  test('health endpoint returns OK', async ({ request }) => {
    const resp = await request.get('/api/health');
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.status).toBe('ok');
  });

  test('login page loads correctly', async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/login');
    await expect(page).toHaveTitle(/Pilot Finance/);
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
  });

  test('dashboard is accessible when authenticated', async ({ page }) => {
    // Uses storageState from global-setup
    await page.goto('/');
    await expect(page).toHaveURL('/');
    await expect(page.locator('nav')).toBeVisible();
  });

  test('logout redirects to login', async ({ page }) => {
    await page.goto('/');
    await logout(page);
    await expect(page).toHaveURL('/login');
  });

  test('login with registered account', async ({ page }) => {
    await page.context().clearCookies();
    await login(page, TEST_USER.email, TEST_USER.password);
    await expect(page).toHaveURL('/');
    await expect(page.locator('nav')).toBeVisible();
  });

  test('login with wrong password returns 401', async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/login');
    await page.locator('input[name="email"]').fill(TEST_USER.email);
    await page.locator('input[name="password"]').fill('WrongPassword1!xx');
    // Capture the POST /login response
    const responsePromise = page.waitForResponse(
      resp => resp.url().includes('/login') && resp.request().method() === 'POST'
    );
    await page.getByRole('button', { name: /se connecter|sign in/i }).click();
    const response = await responsePromise;
    expect(response.status()).toBe(401);
    // Page should stay on /login (htmx:beforeSwap 401 handler redirects back)
    await expect(page).toHaveURL(/\/login/);
  });

  test('protected routes redirect to login when not authenticated', async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/');
    await expect(page).toHaveURL(/\/login/);
  });

  test('forgot-password page loads', async ({ page }) => {
    await page.goto('/forgot-password');
    // Page title: "Mot de passe oublié"
    await expect(page.getByText(/mot de passe oublié|forgot password/i)).toBeVisible();
  });
});
