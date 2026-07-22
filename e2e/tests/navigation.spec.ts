import { test, expect } from '@playwright/test';

test.describe('Navigation & Layout', () => {
  test('nav bar shows all links', async ({ page }) => {
    await page.goto('/');
    const nav = page.locator('nav');
    await expect(nav).toBeVisible();
    // Logo link + dashboard nav link both have href="/", use .first()
    await expect(nav.locator('a[href="/"]').first()).toBeVisible();
    await expect(nav.locator('a[href="/accounts"]')).toBeVisible();
    await expect(nav.locator('a[href="/settings"]')).toBeVisible();
    // Logout is now behind a confirmation dialog — assert the opener button, not the (hidden) form submit.
    await expect(nav.locator('[x-data="logoutData"] > button')).toBeVisible();
  });

  test('active nav item has aria-current', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('nav a[href="/"][aria-current="page"]')).toBeVisible();
    await page.goto('/accounts');
    await expect(page.locator('nav a[href="/accounts"][aria-current="page"]')).toBeVisible();
  });

  test('navigate between pages via nav', async ({ page }) => {
    await page.goto('/');
    await page.locator('nav a[href="/accounts"]').click();
    await expect(page).toHaveURL('/accounts');
    // Use the nav link with aria-current (not the logo)
    await page.locator('nav a[href="/"]').first().click();
    await expect(page).toHaveURL('/');
  });

  test('404 page for unknown routes', async ({ page }) => {
    await page.goto('/this-does-not-exist-at-all');
    await expect(page.getByText('404')).toBeVisible();
  });

  test('privacy page loads', async ({ page }) => {
    await page.goto('/privacy');
    await expect(page.getByRole('heading', { name: /confidentialité|privacy/i })).toBeVisible();
  });

  test('legal page loads', async ({ page }) => {
    await page.goto('/legal');
    await expect(page.getByText(/mentions légales|legal/i)).toBeVisible();
  });

  test('CSP headers are present', async ({ page }) => {
    const response = await page.goto('/');
    const csp = response?.headers()['content-security-policy'];
    expect(csp).toBeTruthy();
    expect(csp).toContain("default-src 'none'");
    expect(csp).toContain('nonce-');
  });

  test('HSTS header is present', async ({ page }) => {
    const response = await page.goto('/');
    const hsts = response?.headers()['strict-transport-security'];
    expect(hsts).toBeTruthy();
  });
});
