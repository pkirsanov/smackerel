/**
 * CHAOS SAGA (bubbles.chaos) — TEMPORARY runtime-evidence artifact.
 *
 * Stochastic real-system end-to-end user journeys driven in a real headless
 * Chromium against the disposable `smackerel-test-e2e-ui` stack (spec 077).
 * This file is created by the chaos run, executed once, and DELETED
 * afterwards. It is NOT a permanent spec and MUST NOT be committed.
 *
 * Seed is fixed + logged so ordering/detours are reproducible.
 *
 * Journeys:
 *   J1  Onboarding + assistant discoverability            (static finding SR-01)
 *   J2  Capture-as-fallback ACK durability                (static finding SR-08)
 *   J3  Nav discoverability + auth-carrier split          (SR-04 / SR-05)
 *   J4  Delivery surfaces render (notifications + cards)
 *   J5  Assistant turn up to the model boundary           (ENV-CONSTRAINED: no GPU/<deploy-host>)
 *
 * Evidence lines are prefixed `CHAOS-EV` so they are greppable in the
 * Playwright `list` reporter stdout. No request interception anywhere —
 * every probe hits the real served surface.
 */
import { expect, test, type Page } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { login, requireAuthToken } from "./_support/cardrewards";

const SEED = 20260702;

// mulberry32 seeded PRNG — reproducible stochastic ordering + detours.
function makePRNG(seed: number): () => number {
  let a = seed >>> 0;
  return function () {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
const rng = makePRNG(SEED);
function shuffle<T>(arr: T[]): T[] {
  const a = arr.slice();
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(rng() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}
function chance(p: number): boolean {
  return rng() < p;
}

// ev — one structured, greppable evidence line.
function ev(...parts: unknown[]): void {
  // eslint-disable-next-line no-console
  console.log("CHAOS-EV " + parts.map((p) => String(p)).join(" | "));
}

async function collectLinks(page: Page): Promise<string[]> {
  return page.$$eval("a[href]", (els) =>
    Array.from(
      new Set(
        els.map((e) => (e as HTMLAnchorElement).getAttribute("href") || ""),
      ),
    ).filter(Boolean),
  );
}

test.describe.configure({ mode: "default", timeout: 120_000 });

test.beforeAll(() => {
  ev("RUN", "seed=" + SEED, "baseURL=" + (process.env.SMACKEREL_BASE_URL ?? "?"));
});

// ---------------------------------------------------------------------------
// J1 — Onboarding saga + assistant discoverability (SR-01)
// ---------------------------------------------------------------------------
test("J1 onboarding + assistant discoverability (SR-01)", async ({
  page,
  request,
}) => {
  attachCSPGuard(page);

  // /register (invite-gated entry, spec 091/093) — unauthenticated.
  const reg = await request.get("/register", { maxRedirects: 0 });
  const regBody = reg.status() === 200 ? await reg.text() : "";
  const inviteField = /invite[_-]?token|name=["']?invite/i.test(regBody);
  ev("J1.register", "GET /register", "status=" + reg.status(), "inviteField=" + inviteField, "len=" + regBody.length);

  // /login (spec 070) — unauthenticated.
  const lg = await request.get("/login", { maxRedirects: 0 });
  ev("J1.login", "GET /login", "status=" + lg.status());

  // Perform login, land on / (post-login landing).
  await login(page, "/");
  const landing = await page.goto("/", { waitUntil: "domcontentloaded" });
  ev("J1.landing", "GET / (authed)", "status=" + (landing?.status() ?? "null"), "title=" + (await page.title().catch(() => "")));

  // Discoverability: can a new user REACH the assistant from nav (no typed URL)?
  const homeLinks = await collectLinks(page);
  ev("J1.homeLinks", JSON.stringify(homeLinks));
  const assistantRe = /assistant/i;
  const assistantInHomeNav = homeLinks.some((h) => assistantRe.test(h));

  // 1-hop crawl over same-origin web pages reachable from / (bounded).
  const visited = new Set<string>(["/"]);
  const oneHop = shuffle(
    homeLinks.filter((h) => h.startsWith("/") && !h.startsWith("/pwa") && !h.includes("logout")),
  ).slice(0, 6);
  let assistantReachable = assistantInHomeNav;
  for (const href of oneHop) {
    if (assistantReachable) break;
    try {
      const r = await page.goto(href, { waitUntil: "domcontentloaded" });
      if (!r) continue;
      visited.add(href);
      const links = await collectLinks(page);
      if (links.some((h) => assistantRe.test(h))) {
        assistantReachable = true;
        ev("J1.reach", "assistant link found on", href);
      }
    } catch (e) {
      ev("J1.crawlError", href, String(e).slice(0, 120));
    }
  }
  ev(
    "J1.VERDICT",
    "assistantInHomeNav=" + assistantInHomeNav,
    "assistantReachableWithin1Hop=" + assistantReachable,
    "pagesCrawled=" + visited.size,
  );
  ev(
    "J1.SR-01",
    assistantReachable
      ? "NOT-CONFIRMED (assistant reachable via nav)"
      : "CONFIRMED (assistant undiscoverable from web nav; requires typing /pwa/assistant.html)",
  );
  assertNoCSPViolations(page);
});

// ---------------------------------------------------------------------------
// J2 — Capture-as-fallback saga: durable ACK (SR-08)
// ---------------------------------------------------------------------------
test("J2 capture-as-fallback ACK durability (SR-08)", async ({
  page,
  request,
}) => {
  attachCSPGuard(page);
  await login(page, "/");
  const token = requireAuthToken();
  const marker = `chaos-${SEED}-${Date.now().toString(36)}`;

  // Direct /api/capture (bearer + cookie both present).
  const cap = await page.request.post("/api/capture", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: JSON.stringify({
      text: `${marker} chaos capture-as-fallback probe — durable ACK expected.`,
    }),
  });
  const capStatus = cap.status();
  const capText = (await cap.text()).slice(0, 400);
  let artifactId = "";
  try {
    const j = JSON.parse(capText);
    artifactId = j.artifact_id || (j.error ? "ERR:" + j.error.code : "");
  } catch {
    /* non-JSON body captured verbatim in capText */
  }
  ev("J2.apiCapture", "POST /api/capture", "status=" + capStatus, "artifact_id=" + artifactId, "body=" + capText);

  // PWA share target (public, form-encoded share-sheet handoff).
  const share = await request.post("/pwa/share", {
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    data: new URLSearchParams({
      title: `${marker} shared`,
      text: `${marker} via share target`,
      url: "https://example.com/" + marker,
    }).toString(),
    maxRedirects: 0,
  });
  ev("J2.pwaShare", "POST /pwa/share", "status=" + share.status(), "location=" + (share.headers()["location"] ?? "-"));

  // Downstream durability: does the capture surface via search? (bounded poll)
  let surfaced = false;
  let lastSearch = "";
  for (let i = 0; i < 3 && !surfaced; i++) {
    await page.waitForTimeout(1500);
    const s = await page.request.post("/api/search", {
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      data: JSON.stringify({ query: marker }),
    });
    lastSearch = "status=" + s.status();
    if (s.ok()) {
      const b = await s.text();
      surfaced = b.includes(marker);
      lastSearch += " hit=" + surfaced;
    }
  }
  ev("J2.downstream", "search(" + marker + ")", lastSearch, "surfaced=" + surfaced);
  ev("J2.VERDICT", "ackStatus=" + capStatus, "artifact=" + artifactId, "shareStatus=" + share.status(), "surfaced=" + surfaced);
  ev(
    "J2.SR-08",
    capStatus >= 200 && capStatus < 300
      ? "ACK-PRESENT (status " + capStatus + ")"
      : "ACK-DEGRADED (status " + capStatus + " — capture did not return a durable ACK)",
  );
});

// ---------------------------------------------------------------------------
// J3 — Navigation / discoverability chaos + auth-carrier split (SR-04 / SR-05)
// ---------------------------------------------------------------------------
test("J3 nav discoverability + auth-carrier split (SR-04/SR-05)", async ({
  page,
  request,
}) => {
  attachCSPGuard(page);
  await login(page, "/");
  const token = requireAuthToken();

  // Collect nav link-sets from the three server-rendered surfaces (random order).
  const surfaces = ["/", "/cards", "/notifications"];
  const navSets: Record<string, string[]> = {};
  for (const s of shuffle(surfaces)) {
    const r = await page.goto(s, { waitUntil: "domcontentloaded" });
    const links = await collectLinks(page);
    navSets[s] = links;
    ev("J3.nav", s, "status=" + (r?.status() ?? "null"), "links=" + JSON.stringify(links));
    if (chance(0.5)) {
      await page.reload();
      ev("J3.detour", "reload", s);
    }
  }

  const norm = (arr: string[]): Set<string> =>
    new Set(
      arr
        .filter((h) => h.startsWith("/") && !h.includes("logout"))
        .map((h) => h.split("?")[0]),
    );
  const home = norm(navSets["/"] || []);
  const cards = norm(navSets["/cards"] || []);
  const notif = norm(navSets["/notifications"] || []);
  const inter = (x: Set<string>, y: Set<string>): string[] => [...x].filter((v) => y.has(v));
  ev("J3.overlap", "home∩cards=" + JSON.stringify(inter(home, cards)), "home∩notif=" + JSON.stringify(inter(home, notif)));
  ev(
    "J3.SR-04",
    inter(home, cards).length === 0
      ? "CONFIRMED (home & card-rewards navs fully disjoint)"
      : "PARTIAL (some shared nav links: " + JSON.stringify(inter(home, cards)) + ")",
  );

  // Auth-carrier probe (SR-05): does /api/* honor the COOKIE vs require a BEARER?
  const apiCookieOnly = await page.request.get("/api/notifications/status"); // page.request carries the auth_token cookie
  const apiNoAuth = await request.get("/api/notifications/status"); // separate context, no cookie, no bearer
  const apiBearer = await request.get("/api/notifications/status", {
    headers: { Authorization: `Bearer ${token}` },
  });
  ev("J3.authCookie", "GET /api/notifications/status (cookie jar)", "status=" + apiCookieOnly.status());
  ev("J3.authNone", "GET /api/notifications/status (no auth)", "status=" + apiNoAuth.status());
  ev("J3.authBearer", "GET /api/notifications/status (bearer)", "status=" + apiBearer.status());

  // Public PWA page vs gated API (the assistant coherence seam).
  const pwaPublic = await request.get("/pwa/assistant.html", { maxRedirects: 0 });
  const turnNoAuth = await request.post("/api/assistant/turn", {
    headers: { "Content-Type": "application/json" },
    data: "{}",
  });
  ev("J3.split", "PWA /pwa/assistant.html(noauth)=" + pwaPublic.status(), "API /api/assistant/turn(noauth)=" + turnNoAuth.status());
  ev(
    "J3.SR-05",
    "apiCookie=" + apiCookieOnly.status() + " apiBearer=" + apiBearer.status() + " → " +
      (apiCookieOnly.status() === apiBearer.status()
        ? "cookie honored by API (carriers converge)"
        : "cookie NOT honored by API (split carrier: pages=cookie, api=bearer)"),
  );
});

// ---------------------------------------------------------------------------
// J4 — Delivery saga: notifications + card-rewards surfaces render
// ---------------------------------------------------------------------------
test("J4 delivery surfaces render (notifications + card-rewards)", async ({
  page,
}) => {
  attachCSPGuard(page);
  await login(page, "/");
  const targets = shuffle([
    "/notifications",
    "/notifications/events",
    "/notifications/incidents",
    "/notifications/summary",
    "/cards",
    "/cards/wallet",
    "/cards/offers",
    "/cards/recommendations",
    "/cards/report",
  ]);
  const results: Record<string, number> = {};
  for (const t of targets) {
    try {
      const r = await page.goto(t, { waitUntil: "domcontentloaded" });
      const st = r?.status() ?? 0;
      results[t] = st;
      ev("J4.render", t, "status=" + st, "title=" + (await page.title().catch(() => "")));
      if (st >= 500) ev("J4.FINDING", "5xx on " + t);
      if (chance(0.3)) {
        await page.goBack().catch(() => {});
        await page.goForward().catch(() => {});
        ev("J4.detour", "back/forward around", t);
      }
    } catch (e) {
      results[t] = -1;
      ev("J4.error", t, String(e).slice(0, 140));
    }
  }
  const bad = Object.entries(results).filter(([, s]) => s >= 400 || s < 0);
  ev("J4.VERDICT", "rendered=" + JSON.stringify(results));
  ev("J4.BAD", JSON.stringify(bad));
  assertNoCSPViolations(page);
});

// ---------------------------------------------------------------------------
// J5 — Assistant saga up to the model boundary (ENV-CONSTRAINED on macOS/no-GPU)
// ---------------------------------------------------------------------------
test("J5 assistant turn up to the model boundary (ENV-CONSTRAINED)", async ({
  page,
}) => {
  attachCSPGuard(page);
  await login(page, "/");

  const pwaR = await page.goto("/pwa/assistant.html", { waitUntil: "domcontentloaded" });
  ev("J5.served", "GET /pwa/assistant.html (authed)", "status=" + (pwaR?.status() ?? "null"), "title=" + (await page.title().catch(() => "")));

  const composer = page.locator("#assistant-composer-input");
  const sendBtn = page.locator("#assistant-send-btn");
  const composerVisible = await composer.isVisible().catch(() => false);
  const sendVisible = await sendBtn.isVisible().catch(() => false);
  ev("J5.composer", "composerVisible=" + composerVisible, "sendVisible=" + sendVisible);

  // Capture the REAL network response the client emits to the transport.
  let turnStatus = -1;
  let turnBody = "";
  page.on("response", async (resp) => {
    if (resp.url().includes("/api/assistant/turn")) {
      turnStatus = resp.status();
      try {
        turnBody = (await resp.text()).slice(0, 300);
      } catch {
        /* body may be unavailable on abort */
      }
    }
  });

  if (composerVisible && sendVisible) {
    await composer.fill(`chaos-${SEED} what did I capture recently?`);
    await sendBtn.click().catch((e) => ev("J5.clickErr", String(e).slice(0, 120)));
    // Bounded wait for either an error state or a rendered response.
    await page.waitForTimeout(8000);
    const errVisible = await page.locator("#assistant-error").isVisible().catch(() => false);
    const errText = (await page.locator("#assistant-error").textContent().catch(() => "")) || "";
    const respText = (await page.locator("#assistant-response").textContent().catch(() => "")) || "";
    const sendDisabled = await sendBtn.isDisabled().catch(() => false);
    ev("J5.uiState", "errorVisible=" + errVisible, "sendReEnabled=" + !sendDisabled, "errText=" + errText.slice(0, 180), "respText=" + respText.slice(0, 120));
  } else {
    ev("J5.uiState", "composer/send not both visible — cannot drive a turn from the UI");
  }
  ev("J5.transport", "POST /api/assistant/turn observed status=" + turnStatus, "body=" + turnBody);

  let verdict: string;
  if (turnStatus >= 200 && turnStatus < 300) {
    verdict = "TRANSPORT-OK; answer-quality ENV-CONSTRAINED (no GPU/<deploy-host> model on this macOS host)";
  } else if (turnStatus >= 500) {
    verdict = "TRANSPORT-REACHED-MODEL-BOUNDARY; server-side model degraded/absent → ENV-CONSTRAINED";
  } else if (turnStatus === 401 || turnStatus === 403) {
    verdict = "AUTH-BLOCKED at transport (FINDING: PWA cookie session not honored by /api/assistant/turn)";
  } else if (turnStatus === 404) {
    verdict = "TRANSPORT-NOT-WIRED (FINDING: /api/assistant/turn 404 in test env — AssistantTurnHandler nil)";
  } else if (turnStatus === -1) {
    verdict = "NO-TRANSPORT-RESPONSE-OBSERVED (client aborted/timed out before a response) → inspect UI error-state";
  } else {
    verdict = "TRANSPORT-STATUS " + turnStatus;
  }
  ev("J5.VERDICT", verdict);
  ev("J5.ENV", "assistant answer synthesis = ENV-CONSTRAINED (GPU/<deploy-host> only; unavailable on macOS host)");
});
