/**
 * Spec 083 Scope 11 — shared helpers for the card-rewards Dashboard /
 * Recommendations / Rotating-Verify / Admin e2e-ui specs.
 *
 * Lives under _support/ so playwright.config's testIgnore keeps it out of test
 * discovery. NO request interception anywhere: every helper drives the REAL
 * /v1/web/login + /api/* surfaces against the disposable `smackerel-test-e2e-ui`
 * stack, exactly like the Scope 10 specs.
 *
 * The worker-scoped login session-reuse core lives in the Playwright-free
 * `cardrewards_session.ts` so the BUG-002 regression can unit-test it without
 * @playwright/test on the unit import graph (see that file). `login()` below is
 * the e2e-ui entry point: it injects Playwright's `expect` status assertion so
 * e2e-ui assertion behavior is unchanged, then delegates to the shared core.
 */
import { expect, type Page } from "@playwright/test";

import {
  login as loginWithSession,
  requireAuthToken,
} from "./cardrewards_session.ts";

// Re-export requireAuthToken so ./cardrewards keeps offering it (unchanged
// public surface); it now lives in the Playwright-free session core.
export { requireAuthToken };

export function uniqueSuffix(): string {
  return Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8);
}

// isoDate returns a YYYY-MM-DD date offsetDays from today (UTC).
export function isoDate(offsetDays: number): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() + offsetDays);
  return d.toISOString().slice(0, 10);
}

function authHeaders(): Record<string, string> {
  return {
    Authorization: `Bearer ${requireAuthToken()}`,
    "Content-Type": "application/json",
  };
}

// login establishes an authenticated session on `page`'s browser context via
// the shared worker-scoped session-reuse core (cardrewards_session.ts). The
// first call per worker performs the real /v1/web/login POST; subsequent calls
// reuse the cached auth_token cookie. The `expect(...).toContain(...)` status
// assertion is injected here — byte-identical to the original inline form — so
// the core stays Playwright-free for the unit lane while e2e-ui assertion
// behavior is unchanged.
export async function login(page: Page, next = "/cards"): Promise<void> {
  await loginWithSession(page, next, (status) =>
    expect(
      [200, 302, 303],
      `/v1/web/login must accept the dev token; got ${status}`,
    ).toContain(status),
  );
}

export interface SeededCard {
  id: string;
  card_catalog_id: string;
}

export async function createCustomCardAPI(
  page: Page,
  custom: {
    name: string;
    issuer: string;
    card_type: string;
    annual_fee_cents: number;
    nickname?: string;
    note?: string;
  },
): Promise<SeededCard> {
  const resp = await page.request.post("/api/cards", {
    headers: authHeaders(),
    data: JSON.stringify({ custom }),
  });
  expect(resp.status(), `seed POST /api/cards: ${await resp.text()}`).toBe(201);
  return resp.json();
}

export async function createOfferAPI(
  page: Page,
  userCardID: string,
  offer: { title: string; category: string; rate: number; rate_type: string },
): Promise<void> {
  const resp = await page.request.post(`/api/cards/${userCardID}/offers`, {
    headers: authHeaders(),
    data: JSON.stringify(offer),
  });
  expect(
    [200, 201],
    `seed POST /api/cards/${userCardID}/offers: ${await resp.text()}`,
  ).toContain(resp.status());
}

export async function createSelectionAPI(
  page: Page,
  userCardID: string,
  sel: {
    category: string;
    period_label: string;
    enrolled: boolean;
    effective_start: string;
    effective_end: string;
  },
): Promise<void> {
  const resp = await page.request.post(`/api/cards/${userCardID}/selections`, {
    headers: authHeaders(),
    data: JSON.stringify(sel),
  });
  expect(
    [200, 201],
    `seed POST /api/cards/${userCardID}/selections: ${await resp.text()}`,
  ).toContain(resp.status());
}

export async function createCategoryAliasAPI(
  page: Page,
  alias: {
    canonical_category: string;
    equivalents?: string[];
    starred?: boolean;
    priority?: number | null;
  },
): Promise<void> {
  const resp = await page.request.post("/api/card-category-aliases", {
    headers: authHeaders(),
    data: JSON.stringify(alias),
  });
  expect(
    [200, 201],
    `seed POST /api/card-category-aliases: ${await resp.text()}`,
  ).toContain(resp.status());
}

// createObservationAPI seeds one per-source rotating-category observation. An
// undated observation reconciles to the active lifecycle (deriveLifecycle
// default), which is what the dashboard "active rotating" panel reads.
export async function createObservationAPI(
  page: Page,
  obs: {
    card_catalog_id: string;
    period_label: string;
    categories: string[];
    confidence: number;
    source_name: string;
    source_url: string;
    period_start?: string;
    period_end?: string;
    limit_cents?: number | null;
  },
): Promise<void> {
  const resp = await page.request.post("/api/card-rotating/observations", {
    headers: authHeaders(),
    data: JSON.stringify(obs),
  });
  expect(
    resp.status(),
    `seed POST /api/card-rotating/observations: ${await resp.text()}`,
  ).toBe(201);
}

// reconcileAPI runs the real Scope 06 reconciler over every stored observation.
// threshold is required server-side (no hidden default); the test supplies it.
export async function reconcileAPI(
  page: Page,
  threshold = 0.7,
  trigger = "manual",
): Promise<void> {
  const resp = await page.request.post("/api/card-rotating/reconcile", {
    headers: authHeaders(),
    data: JSON.stringify({ threshold, trigger }),
  });
  expect(
    resp.status(),
    `POST /api/card-rotating/reconcile: ${await resp.text()}`,
  ).toBe(200);
}

export async function generateRecommendationsAPI(
  page: Page,
  period?: string,
): Promise<void> {
  const resp = await page.request.post("/api/card-recommendations/generate", {
    headers: authHeaders(),
    data: JSON.stringify(period ? { period } : {}),
  });
  expect(
    resp.status(),
    `POST /api/card-recommendations/generate: ${await resp.text()}`,
  ).toBe(200);
}
