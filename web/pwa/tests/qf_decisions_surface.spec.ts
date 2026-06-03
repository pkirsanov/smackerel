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
    // Spec 077 SCOPE-3 / F-077-3-001 / TP-077-03-08: every descendant
    // selector below targets markup that lives ONLY inside the inert
    // <template id="qf-result-template"> document fragment. Playwright
    // `locator()` does NOT descend into <template>.content, so each of
    // these assertions previously timed out at 0 elements (or, in the
    // `toContainText('QF Companion')` case, was structurally
    // unsatisfiable). Read the template content directly via
    // page.evaluate so the contract is asserted without instantiating
    // the template through the search-results JS.
    const tmplSummary = await page.locator('#qf-result-template').evaluate(
      (el: Element) => {
        const tmpl = el as HTMLTemplateElement;
        const root = tmpl.content;
        const has = (sel: string) => root.querySelectorAll(sel).length;
        return {
          text: root.textContent ?? '',
          counts: {
            title: has('.qf-result-title'),
            summary: has('.qf-result-summary'),
            approval: has('.qf-approval-state'),
            packetId: has('.qf-packet-id'),
            traceId: has('.qf-trace-id'),
            trustList: has('.qf-trust-list'),
            openInQF: has('.qf-open-in-qf'),
            openDetail: has('.qf-open-detail'),
          },
        };
      },
    );
    expect(tmplSummary.counts).toEqual({
      title: 1,
      summary: 1,
      approval: 1,
      packetId: 1,
      traceId: 1,
      trustList: 1,
      openInQF: 1,
      openDetail: 1,
    });
    expect(tmplSummary.text).toContain('QF Companion');
    expect(tmplSummary.text).toContain('Read-only');

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