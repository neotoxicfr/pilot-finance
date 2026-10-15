import { test, expect } from '@playwright/test';
import { TEST_USER } from './helpers';

// Passkeys de bout en bout, avec l'authentificateur WebAuthn virtuel de
// Playwright (context.credentials, ≥ 1.64). Le serveur doit tourner avec
// HOST=localhost:3000 : sans HOST, les passkeys sont désactivées et la carte
// n'est pas rendue.
//
// Couvre les correctifs de sécurité v2.25.0 : mot de passe exigé pour ajouter
// un Passkey (SEC-03), session WebAuthn signée à usage unique (SEC-02) et
// vérification utilisateur exigée (SEC-09) — la connexion échouerait si le
// flag UV de l'authentificateur n'était pas accepté.

const RP_ID = 'localhost';

test.describe('Passkeys', () => {
  test.beforeEach(async ({ context }) => {
    // Avant toute navigation : sinon la page voit le WebAuthn natif (absent).
    await context.credentials.install();
  });

  test('ajout refusé avec un mauvais mot de passe', async ({ page, context }) => {
    await page.goto('/settings');
    const card = page.locator('[x-data="passkeyData"]');
    await expect(card).toBeVisible();
    const before = await card.locator('#passkeys-list > div').count();

    await card.locator('#passkey-password').fill('pas-le-bon-mot-de-passe');
    await card.getByRole('button', { name: /passkey/i }).click();

    // Aucune cérémonie WebAuthn : le serveur refuse avant d'émettre un challenge.
    await page.waitForTimeout(1000);
    expect(await context.credentials.get({ rpId: RP_ID })).toHaveLength(0);
    await page.reload();
    await expect(page.locator('[x-data="passkeyData"] #passkeys-list > div')).toHaveCount(before);
  });

  test('ajout puis connexion par Passkey', async ({ page, context, browser, baseURL }) => {
    await page.goto('/settings');
    const card = page.locator('[x-data="passkeyData"]');
    await expect(card).toBeVisible();
    const before = await card.locator('#passkeys-list > div').count();

    await card.locator('#passkey-password').fill(TEST_USER.password);
    // Succès → window.location.reload() : attendre la nouvelle page.
    await Promise.all([
      page.waitForEvent('load'),
      card.getByRole('button', { name: /passkey/i }).click(),
    ]);
    await expect(page.locator('[x-data="passkeyData"] #passkeys-list > div')).toHaveCount(before + 1);

    const [credential] = await context.credentials.get({ rpId: RP_ID });
    expect(credential).toBeTruthy();

    // Navigateur vierge (aucun cookie) qui ne détient que cette passkey.
    const fresh = await browser.newContext({ baseURL });
    try {
      await fresh.credentials.install();
      await fresh.credentials.create(RP_ID, {
        id: credential.id,
        userHandle: credential.userHandle,
        privateKey: credential.privateKey,
        publicKey: credential.publicKey,
        signCount: credential.signCount,
      });
      const login = await fresh.newPage();
      await login.goto('/login');
      await login.getByRole('button', { name: /passkey/i }).click();
      await expect(login).toHaveURL('/', { timeout: 15000 });
      await expect(login.locator('nav')).toBeVisible();
    } finally {
      await fresh.close();
    }
  });
});
