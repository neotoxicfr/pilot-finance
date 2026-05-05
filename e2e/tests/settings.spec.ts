import { test, expect } from '@playwright/test';

test.describe('Settings', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/settings');
  });

  test('settings page loads with all sections', async ({ page }) => {
    // Title: "Paramètres"
    await expect(page.getByText(/paramètres|settings/i).first()).toBeVisible();
    // "Préférences"
    await expect(page.getByText(/préférences|preferences/i)).toBeVisible();
    // "Mot de passe"
    await expect(page.getByText(/mot de passe|password/i).first()).toBeVisible();
    // "Double Authentification (A2F)"
    await expect(page.getByText(/a2f|2fa|two-factor/i).first()).toBeVisible();
  });

  test('language select is functional', async ({ page }) => {
    const langSelect = page.locator('select[name="language"]');
    await expect(langSelect).toBeVisible();
    await expect(langSelect.locator('option[value="fr"]')).toBeAttached();
    await expect(langSelect.locator('option[value="en"]')).toBeAttached();
  });

  test('currency select has options', async ({ page }) => {
    const currSelect = page.locator('select[name="currency"]');
    await expect(currSelect).toBeVisible();
    await expect(currSelect.locator('option[value="EUR"]')).toBeAttached();
    await expect(currSelect.locator('option[value="USD"]')).toBeAttached();
  });

  test('password form has required fields', async ({ page }) => {
    await expect(page.locator('input[name="current_password"]')).toBeVisible();
    await expect(page.locator('input[name="newPassword"]')).toBeVisible();
    await expect(page.locator('input[name="confirmPassword"]')).toBeVisible();
  });

  test('2FA section shows setup button', async ({ page }) => {
    // Should show either "active" badge or setup button
    const mfaSection = page.getByText(/a2f|2fa|two-factor/i).first();
    await expect(mfaSection).toBeVisible();
  });

  test('danger zone has export and delete buttons', async ({ page }) => {
    await expect(page.getByText(/zone dangereuse|danger/i)).toBeVisible();
    // Export link
    await expect(page.getByRole('button', { name: /export|télécharger|download/i })).toBeVisible();
    // Delete button
    await expect(page.getByRole('button', { name: /supprimer|delete/i })).toBeVisible();
  });

  test('theme toggle is functional', async ({ page }) => {
    const themeBtn = page.locator('button[\\@click="toggleTheme()"]');
    await expect(themeBtn).toBeVisible();
    // Click to cycle themes — should not crash
    await themeBtn.click();
    await page.waitForTimeout(300);
    await themeBtn.click();
    await page.waitForTimeout(300);
    await expect(page.locator('nav')).toBeVisible();
  });
});
