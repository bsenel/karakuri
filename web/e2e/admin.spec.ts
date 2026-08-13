import { expect, test, type Page } from '@playwright/test';

/**
 * The flow Phase 19 exists to make possible, end to end in a real browser:
 * sign in, set up a tenant, raise a limit, ask for more, approve it, and see
 * the result.
 *
 * This is the only test in the repository that exercises the browser at all —
 * the Go integration suite covers the same API from the other side, and what it
 * cannot catch is a route guard that renders but never navigates, a form that
 * posts the wrong shape, or a page that reads a field the server does not send.
 *
 * It runs against a real server with a real database, which is why it is one
 * worker and not parallel: it logs in, edits limits and approves requests
 * against shared state, and those steps are not independent of each other.
 */

const admin = { id: 'admin', password: process.env.KARAKURI_AUTH_BOOTSTRAP_PASSWORD ?? '' };

async function signIn(page: Page, id: string, password: string) {
  await page.goto('/');
  // The login form is the whole page when there is no session, so waiting for
  // the field is the same as waiting for the app to decide we are signed out.
  await page.getByLabel('User').fill(id);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: /sign in/i }).click();
  await expect(page.getByRole('navigation')).toBeVisible();
}

test.describe('admin flows', () => {
  test('signs in and lands somewhere reachable', async ({ page }) => {
    await signIn(page, admin.id, admin.password);
    // An administrator holds everything, so the first navigation entry is where
    // they land — and every entry is offered.
    await expect(page.getByRole('link', { name: 'Cost' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Users' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Quota' })).toBeVisible();
  });

  test('shows the limits in force beside what was configured, and edits one', async ({
    page,
  }) => {
    await signIn(page, admin.id, admin.password);
    await page.getByRole('link', { name: 'Quota' }).click();
    await page.getByRole('link', { name: 'Limits' }).click();

    await expect(page.getByText('LLM tokens')).toBeVisible();

    // Raise it. A reason is required, which the form enforces by disabling the
    // button — the server refuses it too.
    await page.getByRole('button', { name: 'change' }).first().click();
    await page.getByLabel('New cap').fill('2500000');
    await page.getByLabel('Reason').fill('e2e: the team grew');
    await page.getByRole('button', { name: 'Save' }).click();

    // The edit is visible at once rather than a cache TTL later, because the
    // writing process invalidates its own resolver.
    await expect(page.getByText('2,500,000')).toBeVisible();
    await expect(page.getByText(/Set by admin — e2e: the team grew/)).toBeVisible();

    // And the configured value is still shown beside it, which is the property
    // that keeps an operator reading the YAML file from being misled.
    await expect(page.getByText('1,000,000')).toBeVisible();

    // Put it back, so a re-run starts where this one did.
    await page.getByRole('button', { name: 'reset to configured' }).first().click();
    await expect(page.getByText(/Set by admin/)).toBeHidden();
  });

  test('creates an organisation and a team inside it', async ({ page }) => {
    await signIn(page, admin.id, admin.password);
    await page.getByRole('link', { name: 'Organisations' }).click();

    const org = `e2e-org-${Date.now()}`;
    await page.getByLabel('New organisation').fill(org);
    await page.getByRole('button', { name: 'Add' }).first().click();
    await expect(page.getByText(org)).toBeVisible();

    // The team form only exists inside an organisation, which is the tree
    // enforcing itself in the interface as well as in the API. Scoped to this
    // organisation's card, because every org renders a form with the same
    // label and picking one by position would be picking a different org.
    const card = page.locator('.card').filter({ hasText: org });
    const team = `e2e-team-${Date.now()}`;
    await card.getByLabel('New team').fill(team);
    await card.getByRole('button', { name: 'Add' }).click();
    await expect(card.getByText(team)).toBeVisible();
  });

  test('asks for more quota and approves it', async ({ page }) => {
    await signIn(page, admin.id, admin.password);

    // A twin to ask about. Creating one through the UI keeps this independent
    // of whatever else is in the database.
    await page.getByRole('link', { name: 'Twins' }).click();
    const twinName = `e2e-twin-${Date.now()}`;
    await page.getByLabel('Name').fill(twinName);
    await page.getByRole('button', { name: 'Create' }).click();
    await expect(page.getByText(twinName)).toBeVisible();

    await page.getByRole('link', { name: 'Quota' }).click();
    await page.getByRole('link', { name: 'Requests' }).click();
    await page.getByRole('button', { name: 'Ask for more' }).click();
    await page.getByLabel('Twin').selectOption({ label: twinName });
    await page.getByLabel('Cap').fill('3000000');
    await page.getByLabel('Reason').fill('e2e: launch week');
    await page.getByRole('button', { name: 'Submit' }).click();

    await expect(page.getByText('e2e: launch week')).toBeVisible();
    await expect(page.getByText('pending')).toBeVisible();

    await page.getByRole('button', { name: 'Approve' }).first().click();

    // Approved, and — the part that matters — the override it wrote is now in
    // force. A workflow that recorded a decision and changed no limit would
    // pass every other assertion here.
    await expect(page.getByText('approved')).toBeVisible();
    // Twice: once in the request that asked for it, once in the raises table.
    // The second is the override the approval wrote, which is the whole claim.
    await expect(page.getByRole('cell', { name: '3,000,000' })).toHaveCount(2);
  });

  test('reaches the cost dashboard', async ({ page }) => {
    await signIn(page, admin.id, admin.password);
    await page.getByRole('link', { name: 'Cost' }).click();
    await expect(page.getByRole('heading', { name: 'Cost', level: 1 })).toBeVisible();
    // A fresh deployment has spent nothing, and the page says so rather than
    // rendering an empty chart with no explanation.
    await expect(page.getByRole('heading', { name: 'Per day' })).toBeVisible();
  });
});
