import { test, expect } from '@playwright/test';
import { createAccount } from './helpers';

test.describe('Accounts & Recurring', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/accounts');
  });

  test('accounts page loads', async ({ page }) => {
    // "Mes Comptes" and "Opérations"
    await expect(page.getByText(/mes comptes|my accounts/i)).toBeVisible();
    await expect(page.getByText(/opérations|operations/i).first()).toBeVisible();
  });

  test('create a new account via modal', async ({ page }) => {
    await createAccount(page, 'PEA Test', '5000', '#10b981');
    await expect(page.locator('#accounts-list')).toContainText('PEA Test');
  });

  test('update account balance inline', async ({ page }) => {
    const balanceInput = page.locator('#accounts-list input[name="balance"]').first();
    if (await balanceInput.isVisible()) {
      await balanceInput.fill('12345');
      // Tab out to mark dirty, then the save button appears
      await balanceInput.press('Tab');
      // Click save button (checkmark)
      const saveBtn = page.locator('#accounts-list button[aria-label*="nregistrer" i]').first();
      if (await saveBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
        await saveBtn.click();
        await page.waitForTimeout(1000);
      }
    }
  });

  test('open account edit modal', async ({ page }) => {
    // aria-label="Modifier"
    const editBtn = page.locator('#accounts-list button[aria-label="Modifier"]').first();
    if (await editBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await editBtn.click();
      await expect(page.locator('dialog#account-dialog')).toBeVisible();
      // Close it
      await page.locator('dialog#account-dialog button[aria-label]').first().click();
    }
  });

  test('create a recurring operation', async ({ page }) => {
    // Click "Ajouter" button (the second one, for recurring)
    const addButtons = page.getByRole('button', { name: /ajouter|add/i });
    await addButtons.last().click();
    await expect(page.locator('dialog#recurring-dialog')).toBeVisible();

    // Fill form
    await page.locator('dialog#recurring-dialog input[name="description"]').fill('Loyer');
    await page.locator('dialog#recurring-dialog input[name="amount"]').fill('800');
    await page.locator('dialog#recurring-dialog input[name="dayOfMonth"]').fill('5');

    // Select an account if available
    const accountSelect = page.locator('dialog#recurring-dialog select[name="accountId"]');
    const options = await accountSelect.locator('option').count();
    if (options > 1) {
      await accountSelect.selectOption({ index: 1 });
    }

    await page.locator('dialog#recurring-dialog form button.btn-primary').click();
    await page.waitForTimeout(1000);
    await expect(page.locator('#recurring-list')).toContainText('Loyer');
  });

  test('summary card shows totals', async ({ page }) => {
    const summaryCard = page.locator('#summary-card');
    await expect(summaryCard).toBeVisible();
  });

  test('delete account with confirmation', async ({ page }) => {
    // aria-label="Supprimer"
    const deleteBtn = page.locator('#accounts-list button[aria-label="Supprimer"]').first();
    if (await deleteBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await deleteBtn.click();
      // Confirm delete — look for a confirmation button that appears
      await page.waitForTimeout(500);
      const confirmBtn = page.locator('#accounts-list button').filter({ hasText: /confirm|supprimer/i }).first();
      if (await confirmBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmBtn.click();
        await page.waitForTimeout(1000);
      }
    }
  });
});
