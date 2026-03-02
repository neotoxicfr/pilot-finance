import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

// Pages to audit for accessibility
const pages = [
  { name: 'Dashboard', path: '/' },
  { name: 'Accounts', path: '/accounts' },
  { name: 'Settings', path: '/settings' },
  { name: 'Privacy', path: '/privacy' },
  { name: 'Legal', path: '/legal' },
];

test.describe('Accessibility (axe-core)', () => {
  for (const { name, path } of pages) {
    test(`${name} page has no critical violations`, async ({ page }) => {
      await page.goto(path);
      await page.waitForLoadState('networkidle');

      const results = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
        .disableRules(['color-contrast']) // Tailwind dynamic theming makes this unreliable
        .analyze();

      const critical = results.violations.filter(
        v => v.impact === 'critical' || v.impact === 'serious'
      );

      if (critical.length > 0) {
        const summary = critical.map(
          v => `[${v.impact}] ${v.id}: ${v.help} (${v.nodes.length} elements)`
        ).join('\n');
        expect(critical, `Accessibility violations:\n${summary}`).toHaveLength(0);
      }
    });
  }

  test('Login page has no critical violations', async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
      .disableRules(['color-contrast'])
      .analyze();

    const critical = results.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious'
    );

    if (critical.length > 0) {
      const summary = critical.map(
        v => `[${v.impact}] ${v.id}: ${v.help} (${v.nodes.length} elements)`
      ).join('\n');
      expect(critical, `Accessibility violations:\n${summary}`).toHaveLength(0);
    }
  });
});
