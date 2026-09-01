import { test, expect, Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * axe-core audit (audit finding S-16).
 *
 * Three blind spots were closed here:
 *  1. `color-contrast` was disabled on every page — it is now enabled, and each
 *     page is audited in BOTH themes (the app picks its theme from
 *     `prefers-color-scheme` unless localStorage overrides it, so a single run
 *     only ever exercised one palette).
 *  2. Only `critical` / `serious` violations failed the build — every violation
 *     of the selected WCAG tag set now fails, minus an explicit, documented
 *     exception list.
 *  3. Only six at-rest pages were covered — the public auth pages and one
 *     "open state" (modal displayed) are now included.
 */

const WCAG_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

/**
 * Rules knowingly not enforced yet. Keep this list short, and keep a reason on
 * every entry — an empty exception list is the goal.
 *
 * - target-size: WCAG 2.2 AA (2.5.8). Tracked separately (audit S-17/S-18);
 *   enabling it today would make the suite permanently red on the colour
 *   swatches and the inline balance controls.
 */
const KNOWN_GAPS = ['target-size'];

const THEMES = ['light', 'dark'] as const;

/** Pages reachable with the authenticated storage state. */
const AUTHED_PAGES = [
  { name: 'Dashboard', path: '/' },
  { name: 'Accounts', path: '/accounts' },
  { name: 'Settings', path: '/settings' },
  { name: 'Audit log', path: '/admin/audit' },
  { name: 'Privacy', path: '/privacy' },
  { name: 'Legal', path: '/legal' },
];

/** Public pages — audited signed out. */
const PUBLIC_PAGES = [
  { name: 'Login', path: '/login' },
  { name: 'Register', path: '/register' },
  { name: 'Forgot password', path: '/forgot-password' },
  { name: 'Reset password (invalid token)', path: '/reset-password?token=a11y-audit-invalid' },
];

/**
 * The theme is chosen by an inline <head> script: localStorage 'theme' wins,
 * otherwise prefers-color-scheme. Drop the stored value so emulateMedia is
 * authoritative, then assert the resulting `.dark` state so a silent failure to
 * switch themes cannot make the contrast audit vacuous.
 */
async function gotoWithTheme(page: Page, path: string, theme: 'light' | 'dark') {
  await page.addInitScript(() => {
    try {
      window.localStorage.removeItem('theme');
    } catch {
      /* storage unavailable — the app falls back to prefers-color-scheme */
    }
  });
  await page.emulateMedia({ colorScheme: theme });
  await page.goto(path);
  await page.waitForLoadState('networkidle');

  const state = await page.evaluate(() => ({
    // Only the app layout renders <html lang="…">; a bare http.Error response
    // is wrapped by the browser in a synthetic document with no lang.
    isAppPage: document.documentElement.hasAttribute('lang'),
    isDark: document.documentElement.classList.contains('dark'),
  }));

  // Guard against a vacuous contrast audit: if the theme did not actually
  // switch, every page would be audited twice in the same palette.
  if (state.isAppPage) {
    expect(state.isDark, `theme emulation failed: expected ${theme} on ${path}`).toBe(theme === 'dark');
  }
}

async function expectNoViolations(page: Page, label: string) {
  const results = await new AxeBuilder({ page })
    .withTags(WCAG_TAGS)
    .disableRules(KNOWN_GAPS)
    .analyze();

  const summary = results.violations
    .map(v => `[${v.impact}] ${v.id}: ${v.help} (${v.nodes.length} elements)\n    ${v.nodes.map(n => n.target.join(' ')).join('\n    ')}`)
    .join('\n');

  expect(results.violations, `Accessibility violations on ${label}:\n${summary}`).toHaveLength(0);
}

test.describe('Accessibility (axe-core)', () => {
  for (const { name, path } of AUTHED_PAGES) {
    test(`${name} page passes axe in light and dark themes`, async ({ page }) => {
      for (const theme of THEMES) {
        await gotoWithTheme(page, path, theme);
        await expectNoViolations(page, `${name} (${theme})`);
      }
    });
  }

  for (const { name, path } of PUBLIC_PAGES) {
    test(`${name} page passes axe in light and dark themes`, async ({ page }) => {
      await page.context().clearCookies();
      for (const theme of THEMES) {
        await gotoWithTheme(page, path, theme);
        await expectNoViolations(page, `${name} (${theme})`);
      }
    });
  }

  // "Open state": the at-rest audits never saw a modal, which is where focus
  // management and contrast regressions actually live.
  test('Account modal (open state) passes axe in light and dark themes', async ({ page }) => {
    for (const theme of THEMES) {
      await gotoWithTheme(page, '/accounts', theme);
      await page.getByRole('button', { name: /ajouter|add/i }).first().click();
      await expect(page.locator('dialog#account-dialog')).toBeVisible({ timeout: 5000 });
      // `toBeVisible` resolves as soon as the dialog is painted, which is while
      // its open transition is still running. axe would then sample colours
      // blended against the backdrop and report contrast failures that do not
      // exist at rest. Wait for the panel to reach full opacity first.
      await expect
        .poll(async () => page.locator('#account-dialog .modal-panel')
          .evaluate(el => getComputedStyle(el).opacity), { timeout: 5000 })
        .toBe('1');
      await expectNoViolations(page, `Account modal (${theme})`);
    }
  });
});
