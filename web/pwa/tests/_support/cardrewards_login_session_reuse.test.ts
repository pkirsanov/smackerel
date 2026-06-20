/**
 * Spec 077 e2e-ui lane — BUG-002 regression: card-rewards login session reuse.
 *
 * Node-level unit test (node:test runner, --experimental-strip-types) that
 * locks in the BUG-002 fix: the shared `login()` helper in
 * `web/pwa/tests/_support/cardrewards.ts` performs the real /v1/web/login POST
 * at most ONCE per worker and replays the cached auth_token cookie on every
 * subsequent call. The bug was a per-test login that, at ~40 card-rewards tests
 * sharing one CI runner IP, blew past the production web-login rate limit
 * `httprate.LimitByIP(20, 1*time.Minute)` (internal/api/router.go, spec 070) so
 * the later card-rewards e2e-ui tests got HTTP 429. That limiter is a real
 * credential-stuffing defense (itself under test in
 * internal/api/web_login_ratelimit_test.go) and MUST stay as-is — the fix is
 * test-side session reuse only.
 *
 * Driver: tests/unit/web/bug_077_002_login_session_reuse_test.sh (auto-discovered
 * by `./smackerel.sh test unit` via the spec 077 SCOPE-2 tests/unit/web/*.sh
 * convention). No `@playwright/test` test-runner context is required — the
 * helper's runtime `expect` import loads standalone under Node 22 and the Page
 * is a hand-rolled stub (mirrors csp.test.ts). The driver exports
 * SMACKEREL_AUTH_TOKEN so the helper's module-load `requireAuthToken()` resolves.
 *
 * Anchors SCN-077-BUG-002-01 (worker session reuse) and SCN-077-BUG-002-02
 * (no per-test login POST reintroduced in any cardrewards_*.spec.ts).
 */
import { strict as assert } from "node:assert";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";

import { login } from "./cardrewards.ts";

// A representative auth_token cookie shaped exactly as BrowserContext.cookies()
// returns it (the shape the fix captures and later replays via addCookies).
const AUTH_COOKIE = {
  name: "auth_token",
  value: "stub-session-token",
  domain: "127.0.0.1",
  path: "/",
  expires: -1,
  httpOnly: true,
  secure: false,
  sameSite: "Lax" as const,
};

// makeStubPage builds a minimal Playwright-Page-shaped stub. `onPost` lets each
// test decide what the /v1/web/login POST does: count it, or throw to prove it
// was never called on the cache-hit path.
function makeStubPage(onPost: (url: string) => { status: () => number }) {
  const added: unknown[][] = [];
  const page = {
    request: {
      post(url: string) {
        return Promise.resolve(onPost(url));
      },
    },
    context() {
      return {
        cookies() {
          return Promise.resolve([AUTH_COOKIE]);
        },
        addCookies(cookies: unknown[]) {
          added.push(cookies);
          return Promise.resolve();
        },
      };
    },
  };
  return { page, added };
}

// SCN-077-BUG-002-01 — the first login POSTs once and caches; the second login
// (a DIFFERENT context, as Playwright hands each test a fresh context) reuses
// the cached cookie via addCookies and MUST NOT POST again.
//
// Intrinsically adversarial: the second stub's POST THROWS, so if the
// worker-scoped cache were removed (the bug reintroduced), login() would POST on
// the second call and this test would fail with the tripwire error.
test("SCN-077-BUG-002-01 — login POSTs once per worker, then reuses the cached session", async () => {
  let postCount = 0;
  const first = makeStubPage(() => {
    postCount += 1;
    return { status: () => 303 };
  });
  await login(first.page as never);
  assert.equal(
    postCount,
    1,
    "first login MUST perform exactly one real /v1/web/login POST",
  );
  assert.equal(
    first.added.length,
    0,
    "first login captures the live Set-Cookie; it does not replay cookies",
  );

  const second = makeStubPage(() => {
    throw new Error(
      "REGRESSION: login() POSTed /v1/web/login a second time instead of " +
        "reusing the cached session — this is the BUG-002 rate-limit defect",
    );
  });
  await login(second.page as never);
  assert.equal(
    postCount,
    1,
    "second login MUST NOT POST again (cache hit) — still exactly one POST total",
  );
  assert.equal(
    second.added.length,
    1,
    "second login MUST replay the cached cookie via addCookies",
  );
  const replayed = second.added[0] as Array<{ name: string }>;
  assert.ok(
    Array.isArray(replayed) && replayed.some((c) => c.name === "auth_token"),
    "replayed cookie set MUST contain the auth_token cookie",
  );
});

// SCN-077-BUG-002-02 — structural guard: no cardrewards_*.spec.ts may carry its
// OWN /v1/web/login POST. Every card-rewards spec must route login through the
// shared, worker-cached helper. Reintroducing a per-test login POST is exactly
// the bug and would re-cross the rate limit at scale.
test("SCN-077-BUG-002-02 — no cardrewards spec reintroduces a per-test /v1/web/login POST", () => {
  const testsDir = join(import.meta.dirname, "..");
  const forbidden = /request\.post\(\s*["']\/v1\/web\/login["']/;

  // Adversarial self-check: the detector MUST match a known-bad line, so a green
  // result can never be a tautology against a broken regex.
  assert.ok(
    forbidden.test('await page.request.post("/v1/web/login", {'),
    "forbidden-POST detector regex must match a real per-test login POST",
  );

  const cardSpecs = readdirSync(testsDir).filter((f) =>
    /^cardrewards_.*\.spec\.ts$/.test(f),
  );
  assert.ok(
    cardSpecs.length >= 5,
    `expected the cardrewards spec corpus to be present; found ${cardSpecs.length}`,
  );

  const offenders: string[] = [];
  for (const f of cardSpecs) {
    const body = readFileSync(join(testsDir, f), "utf8");
    if (forbidden.test(body)) offenders.push(f);
  }
  assert.deepEqual(
    offenders,
    [],
    "these cardrewards specs reintroduced a per-test /v1/web/login POST instead " +
      `of using the shared cached login(): ${offenders.join(", ")}`,
  );
});
