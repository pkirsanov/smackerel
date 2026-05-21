import { expect, test } from '@playwright/test';

const SEARCH_HTML = '/pwa/drive-search.html';
const SEARCH_JS = '/pwa/drive-search.js';
const DETAIL_HTML = '/pwa/drive-artifact-detail.html?type=qf&id=qf-test-packet';
const DETAIL_JS = '/pwa/drive-artifact-detail.js';

async function assetText(page, path: string): Promise<string> {
  const response = await page.request.get(path);
  expect(response.ok(), `${path} must be served by the live PWA asset host`).toBeTruthy();
  return response.text();
}

test.describe('QF decision PWA surface', () => {
  test('renders search-card contract for QF generic and trust badge cards', async ({ page }) => {
    await page.goto(SEARCH_HTML);

    await expect(page.locator('#qf-result-template')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-result-title')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-result-summary')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-approval-state')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-packet-id')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-trace-id')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-trust-list')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-open-in-qf')).toHaveCount(1);
    await expect(page.locator('#qf-result-template .qf-open-detail')).toHaveCount(1);
    await expect(page.locator('body')).toContainText('QF Companion');
    await expect(page.locator('body')).toContainText('Read-only');

    const script = await assetText(page, SEARCH_JS);
    expect(script).toContain('result.qf_card');
    expect(script).toContain('card.trust_objects');
    expect(script).toContain('card.deep_link.url');
    expect(script).toContain('qf-open-in-qf');
    expect(script).toContain('type=qf');
    expect(script).not.toContain('card.confidence');
    expect(script).not.toContain('card.score');
    expect(script).not.toContain('card.value');
  });

  test('renders detail-card contract with preserved trust metadata and deep link', async ({ page }) => {
    await page.goto(DETAIL_HTML);

    await expect(page.locator('#qf-packet-panel')).toHaveCount(1);
    await expect(page.locator('#qf-packet-label')).toHaveCount(1);
    await expect(page.locator('#qf-packet-title')).toHaveCount(1);
    await expect(page.locator('#qf-packet-id')).toHaveCount(1);
    await expect(page.locator('#qf-trace-id')).toHaveCount(1);
    await expect(page.locator('#qf-approval-state')).toHaveCount(1);
    await expect(page.locator('#qf-deep-link')).toHaveCount(1);
    await expect(page.locator('#qf-trust-list')).toHaveCount(1);

    const script = await assetText(page, DETAIL_JS);
    expect(script).toContain('detail.qf_card');
    expect(script).toContain('card.trust_objects');
    expect(script).toContain('card.deep_link.url');
    expect(script).toContain('card.deep_link.status');
    expect(script).toContain('qf-packet-panel');
    expect(script).not.toContain('card.confidence');
    expect(script).not.toContain('card.score');
    expect(script).not.toContain('card.value');
  });
});