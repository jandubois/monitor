import { test, expect } from '@playwright/test';

// These tests require a running server at BASE_URL (default: http://localhost:8080)
// and the AUTH_TOKEN environment variable to be set

const AUTH_TOKEN = process.env.AUTH_TOKEN || 'changeme';

test.describe('Authentication', () => {
  test('redirects to login when not authenticated', async ({ page }) => {
    // Navigate first, then clear localStorage
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();

    // Should show login form
    await expect(page.getByPlaceholder('Enter your token')).toBeVisible();
  });

  test('can login with valid token', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();

    // Enter token and submit
    await page.getByPlaceholder('Enter your token').fill(AUTH_TOKEN);
    await page.getByRole('button', { name: /sign in/i }).click();

    // Should redirect to dashboard and show the title
    await expect(page.getByRole('heading', { name: /monitor dashboard/i })).toBeVisible({
      timeout: 10000,
    });
  });

  test('shows error for invalid token', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();

    // Enter invalid token
    await page.getByPlaceholder('Enter your token').fill('invalid-token');
    await page.getByRole('button', { name: /sign in/i }).click();

    // Should show error message
    await expect(page.getByText('Invalid token')).toBeVisible({
      timeout: 5000,
    });
  });
});
