/**
 * Spec 083 Scope 11 — shared helpers for the card-rewards Dashboard /
 * Recommendations / Rotating-Verify / Admin e2e-ui specs.
 *
 * Lives under _support/ so playwright.config's testIgnore keeps it out of test
 * discovery. NO request interception anywhere: every helper drives the REAL
 * /v1/web/login + /api/* surfaces against the disposable `smackerel-test-e2e-ui`
 * stack, exactly like the Scope 10 specs.
 */
import { expect, type Page } from "@playwright/test";

const AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

export function requireAuthToken(): string {
  if (!AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the spec 083 Scope 11 card-rewards " +
        "e2e-ui tests but is unset. The e2e-ui lane must export it from " +
        "config/generated/test.env.",
    );
  }
  return AUTH_TOKEN;
}

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

// login exchanges the shared dev token for an auth_token cookie on the shared
// browser context (page.request shares the cookie jar with page.goto).
export async function login(page: Page, next = "/cards"): Promise<void> {
  const resp = await page.request.post("/v1/web/login", {
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Accept: "text/html",
    },
    data: new URLSearchParams({ token: requireAuthToken(), next }).toString(),
    maxRedirects: 0,
  });
  expect(
    [200, 302, 303],
    `/v1/web/login must accept the dev token; got ${resp.status()}`,
  ).toContain(resp.status());
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
