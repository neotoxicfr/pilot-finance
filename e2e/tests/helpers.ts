import { Page, expect } from '@playwright/test';

export const TEST_USER = {
  email: 'e2e-playwright@test.local',
  password: 'E2eTestPass1!secure',
};

/** Register a new user via the login page register toggle */
export async function registerUser(page: Page, email: string, password: string) {
  await page.goto('/login');
  // Toggle to register mode — button text: "Pas encore de compte ? S'inscrire"
  await page.getByRole('button', { name: /s'inscrire|sign up/i }).click();
  // Fill form fields
  await page.locator('input[name="email"]').fill(email);
  await page.locator('input[name="password"]').fill(password);
  await page.locator('input[name="confirmPassword"]').fill(password);
  // Check consent checkbox (required for registration)
  await page.locator('input[name="consent"]').check();
  // Wait for strength validation to enable the submit button
  await expect(page.getByRole('button', { name: /créer un compte|create account/i })).toBeEnabled({ timeout: 5000 });
  await page.getByRole('button', { name: /créer un compte|create account/i }).click();
  // HTMX sends HX-Redirect → full page navigation to /
  // Use Promise.race: either URL changes to / or nav becomes visible
  await Promise.race([
    page.waitForURL('/', { timeout: 15000 }),
    expect(page.locator('nav')).toBeVisible({ timeout: 15000 }),
  ]);
}

/** Login with existing credentials */
export async function login(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.locator('input[name="email"]').fill(email);
  await page.locator('input[name="password"]').fill(password);
  await page.getByRole('button', { name: /se connecter|sign in/i }).click();
  // HTMX sends HX-Redirect → full page navigation to /
  await Promise.race([
    page.waitForURL('/', { timeout: 15000 }),
    expect(page.locator('nav')).toBeVisible({ timeout: 15000 }),
  ]);
}

/** Logout via the confirmation dialog in nav */
export async function logout(page: Page) {
  // Logout is gated behind a confirmation dialog (<dialog id="logout-dialog">).
  // Open it via the nav opener button, then submit the POST form inside.
  await page.locator('nav [x-data="logoutData"] > button').click();
  await expect(page.locator('dialog#logout-dialog')).toBeVisible({ timeout: 3000 });
  await page.locator('dialog#logout-dialog form[action="/logout"] button[type="submit"]').click();
  await page.waitForURL('/login', { timeout: 5000 });
}

/** Create an account via the accounts page modal */
export async function createAccount(page: Page, name: string, balance: string, color = '#3b82f6') {
  await page.goto('/accounts');
  // Click "Ajouter" button
  await page.getByRole('button', { name: /ajouter|add/i }).first().click();
  // Wait for dialog to open
  await expect(page.locator('dialog#account-dialog')).toBeVisible({ timeout: 3000 });
  // Fill form
  await page.locator('dialog#account-dialog input[name="name"]').fill(name);
  await page.locator('dialog#account-dialog input[name="balance"]').fill(balance);
  await page.locator('dialog#account-dialog input[name="color"]').fill(color, { force: true });
  // Submit — button text: "Valider"
  await page.locator('dialog#account-dialog form button.btn-primary').click();
  // Wait for HTMX swap of #accounts-list
  await expect(page.locator('#accounts-list')).toContainText(name, { timeout: 5000 });
}
