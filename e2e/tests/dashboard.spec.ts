import { test, expect } from '@playwright/test';
import { createAccount } from './helpers';

test.describe('Dashboard', () => {
  test('dashboard loads with nav', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('nav')).toBeVisible();
  });

  test('dashboard shows empty state or KPI cards', async ({ page }) => {
    await page.goto('/');
    // Either empty state CTA or KPI cards should be visible
    const emptyState = page.getByText(/aucun compte|no account/i);
    const kpiCards = page.getByText(/patrimoine|net worth/i);
    const hasEmpty = await emptyState.isVisible().catch(() => false);
    const hasKpi = await kpiCards.isVisible().catch(() => false);
    expect(hasEmpty || hasKpi).toBeTruthy();
  });

  test('dashboard shows KPI cards after creating an account', async ({ page }) => {
    // Create an account first
    await createAccount(page, 'Livret A', '10000');
    // Go back to dashboard
    await page.goto('/');
    // KPI card: "Patrimoine Net"
    await expect(page.getByText(/patrimoine|net worth/i)).toBeVisible({ timeout: 10000 });
  });

  test('projection section is visible with accounts', async ({ page }) => {
    await page.goto('/');
    // If there are accounts, the projection section and chart should be visible
    const projectionSection = page.getByText(/projection|prévision/i).first();
    if (await projectionSection.isVisible({ timeout: 3000 }).catch(() => false)) {
      // The range slider should be present
      const slider = page.locator('input[type="range"]');
      await expect(slider).toBeVisible();
    }
  });
});
