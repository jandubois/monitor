import { test, expect } from '@playwright/test';

const AUTH_TOKEN = process.env.AUTH_TOKEN || 'changeme';

// Helper to login before each test
test.beforeEach(async ({ page }) => {
  await page.goto('/');

  // Check if already logged in by looking for the login form
  const loginForm = page.getByPlaceholder('Enter your token');
  if (await loginForm.isVisible({ timeout: 2000 }).catch(() => false)) {
    await loginForm.fill(AUTH_TOKEN);
    await page.getByRole('button', { name: /sign in/i }).click();
    await expect(page.getByRole('heading', { name: /monitor dashboard/i })).toBeVisible({
      timeout: 10000,
    });
  }
});

test.describe('Dashboard', () => {
  test('displays dashboard header', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /monitor dashboard/i })).toBeVisible();
  });

  test('shows status cards', async ({ page }) => {
    // Wait for dashboard to load - use exact text to avoid matching multiple elements
    await expect(page.getByText('Watchers', { exact: true })).toBeVisible();
    await expect(page.getByText('Active Probes')).toBeVisible();
    await expect(page.getByText('Status Summary')).toBeVisible();
  });

  test('has configure button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /configure/i })).toBeVisible();
  });

  test('can navigate to configuration page', async ({ page }) => {
    await page.getByRole('button', { name: /configure/i }).click();

    // Should show configuration tabs or content
    await expect(page.getByText(/watchers|probes|notification/i)).toBeVisible({
      timeout: 5000,
    });
  });

  test('has keyword filter input', async ({ page }) => {
    await expect(page.getByPlaceholder('Filter by keyword...')).toBeVisible();
  });

  test('can filter probes by keyword', async ({ page }) => {
    const filterInput = page.getByPlaceholder('Filter by keyword...');
    await filterInput.fill('nonexistent-keyword-xyz');

    // Should show "no probes match" or similar message
    await expect(page.getByText(/no probes match/i)).toBeVisible({
      timeout: 5000,
    });
  });
});
