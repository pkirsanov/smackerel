/**
 * graph-activation.spec.ts — BUG-080-001 SCOPE-04.
 *
 * Test rows T080-04-UI, T080-05-UI, T080-06-UI, T080-08-A11Y and
 * T080-REGRESSION, against the disposable `smackerel-test-e2e-ui` stack.
 *
 * These run WITHOUT request interception. `page.route`/`context.route`
 * are deliberately absent: the repo classes an intercepted test as
 * mocked, and a mocked test cannot satisfy a live-stack DoD row. Every
 * state below is induced through a REAL stack condition — a real
 * dev-token session cookie, a real session drop, the stack's real graph
 * configuration, the real absence of ingested data.
 *
 * The session is established by seeding the lane's own `auth_token`
 * cookie rather than by POSTing /v1/web/login. That is not a way around
 * auth: in this lane's dev-token mode the cookie value IS the shared
 * token, so the seeded session is byte-identical to a posted one (see
 * `authenticate` below). It also keeps the suite off the login rate
 * limiter, which is a real security control and must not be spent,
 * raised, or retried against by tests. Nothing here is intercepted or
 * faked — every request still reaches the live stack and is authorised
 * by the real middleware.
 *
 * The load-bearing design choice is that no test asserts a state in the
 * abstract. Each one first reads the truthful published aggregate (the
 * `graph` section of authenticated /api/health) and then asserts the UI
 * AGREES WITH IT. That is stronger than pinning one expected state, and
 * it is the scope's actual core outcome: Knowledge, Graph availability
 * and readiness must consume ONE closed model and must not be able to
 * disagree. It also means these tests are honest on either a graph-
 * enabled or a graph-disabled stack instead of silently passing on one
 * of them — there is no `if (...) return;` bailout anywhere in this file,
 * which the regression-quality guard forbids.
 */
import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

import { requireSmackerelBaseUrl } from "./_support/env";

const AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

function requireAuthToken(): string {
  if (!AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the BUG-080-001 SCOPE-04 graph " +
        "activation tests but is unset. The e2e-ui lane must export it from " +
        "config/generated/test.env.",
    );
  }
  return AUTH_TOKEN;
}

/** The closed aggregate states the server can publish. */
const AGGREGATE_POLICY_DISABLED = "policy_disabled";

/** The closed exclusive UI states web/pwa/wiki_state.js can paint. */
const STATE_READY = "ready";
const STATE_TRUE_EMPTY = "true-empty";
const STATE_DISABLED = "disabled";
const STATE_UNAUTHENTICATED = "unauthenticated";
const STATE_FORBIDDEN = "forbidden";
const STATE_ROUTE_ABSENT = "route-absent";
const STATE_STORE_UNAVAILABLE = "store-unavailable";
const STATE_SCHEMA_INVALID = "schema-invalid";
const STATE_DEGRADED = "degraded";
const STATE_PARTIAL = "partial";

const ALL_UI_STATES = [
  STATE_READY,
  STATE_TRUE_EMPTY,
  STATE_PARTIAL,
  STATE_DEGRADED,
  STATE_DISABLED,
  STATE_UNAUTHENTICATED,
  STATE_FORBIDDEN,
  STATE_ROUTE_ABSENT,
  STATE_STORE_UNAVAILABLE,
  STATE_SCHEMA_INVALID,
];

/**
 * A distinctive fragment of the disabled explanation, which is owned by
 * web/pwa/wiki_state.js as STATE_COPY[STATE_DISABLED].message ("The
 * knowledge graph is turned off for this deployment. This is a deliberate
 * configuration, not a fault, and there is nothing to retry.").
 *
 * A fragment rather than the whole sentence on purpose: ordinary
 * copy-editing of the rest must not fail the lane, but an explanation
 * that is missing, blanked out, or replaced by an unrelated message still
 * must.
 */
const DISABLED_EXPLANATION_FRAGMENT = "turned off for this deployment";

/**
 * The matching fragment for the store-unavailable explanation, owned by
 * web/pwa/wiki_state.js as STATE_COPY[STATE_STORE_UNAVAILABLE].message
 * ("The knowledge graph store is temporarily unreachable. Your data is
 * not lost; this view can be retried.").
 *
 * Same fragment-not-sentence reasoning as above. The half chosen is the
 * half that carries the meaning under test: the store is unreachable
 * TEMPORARILY, which is what makes this state retryable and therefore
 * different from `disabled`.
 */
const STORE_UNAVAILABLE_EXPLANATION_FRAGMENT = "store is temporarily unreachable";

/**
 * Establish a real browser session against the running stack by seeding
 * the lane's dev-token session cookie.
 *
 * This is equivalent to a login rather than a substitute for one. The
 * disposable e2e-ui stack runs in dev-token mode (AuthConfig.Enabled =
 * false), where /v1/web/login compares the posted token against the
 * shared SMACKEREL_AUTH_TOKEN and then writes THAT SAME token as the
 * cookie value (`d.authCookie(token, false)` in internal/api/web_login.go),
 * which the middleware later reads straight back out and uses as the
 * bearer token (`extractBearerTokenWithSource` in internal/api/router.go).
 * The cookie produced here is therefore byte-identical to the one a POST
 * would mint, so no assertion downstream is weakened by seeding it.
 *
 * Posting would be actively wrong for this file. /v1/web/login is rate
 * limited by `httprate.LimitByIP(20, 1*time.Minute)` as a deliberate
 * credential-stuffing defence (spec 070) and that limiter is itself under
 * test, so it must stay exactly as-is. Five specs each spending limiter
 * budget on a redundant login buys nothing, and retrying past a
 * protective control would be worse than the waste. The real login flow
 * keeps its own dedicated coverage in auth_login.spec.ts, which is where
 * it belongs.
 *
 * The origin comes from requireSmackerelBaseUrl() — the same fail-loud SST
 * consumer playwright.config.ts uses for `use.baseURL` — so the cookie is
 * scoped to exactly the origin these tests navigate to by construction,
 * not by a duplicated literal. Attributes mirror the server's own cookie:
 * HttpOnly, SameSite=Lax, host-only, and Path=/ derived from the origin
 * URL. `Secure` is deliberately absent because the handler drops it
 * outside production and this stack is plain HTTP.
 *
 * Safe to call once per test: each test gets a fresh browser context, and
 * addCookies replaces any same-name/domain/path entry, so it is idempotent.
 */
async function authenticate(page: Page): Promise<void> {
  const token = requireAuthToken();
  await page.context().addCookies([
    {
      name: "auth_token",
      value: token,
      url: new URL(requireSmackerelBaseUrl()).origin,
      httpOnly: true,
      sameSite: "Lax",
    },
  ]);
}

interface GraphAggregate {
  ready: boolean;
  activation: string;
  state: string;
  code: string;
}

/**
 * Read the ONE published aggregate. Returns null when the section is
 * absent, which is itself meaningful: the section lives inside the
 * authenticated branch of /api/health, so absence means the caller has
 * no session rather than that the graph is healthy.
 */
async function readPublishedAggregate(request: APIRequestContext): Promise<GraphAggregate | null> {
  const resp = await request.get("/api/health", { headers: { Accept: "application/json" } });
  expect(resp.status(), "/api/health is the liveness probe and must answer 200").toBe(200);
  const body = await resp.json();
  return body && body.graph ? (body.graph as GraphAggregate) : null;
}

/** The single state the page painted, read from the status region. */
async function paintedState(page: Page, statusSelector: string): Promise<string> {
  const node = page.locator(statusSelector);
  await expect(node, `${statusSelector} must exist so a state is observable`).toHaveCount(1);
  const state = await node.getAttribute("data-graph-state");
  expect(state, `${statusSelector} must declare exactly one closed state`).not.toBeNull();
  expect(
    ALL_UI_STATES,
    `painted state ${state} must belong to the closed vocabulary`,
  ).toContain(state as string);
  return state as string;
}

test.describe("BUG-080-001 SCOPE-04 — Knowledge Graph activation truth", () => {
  test("Regression: explicit disabled Graph stays in shell and never reports Available", async ({ page }, testInfo) => {
    await authenticate(page);
    const aggregate = await readPublishedAggregate(page.request);
    expect(aggregate, "an authenticated caller must receive the graph aggregate").not.toBeNull();
    const disabled = aggregate!.state === AGGREGATE_POLICY_DISABLED;

    await page.goto("/pwa/wiki.html");
    // Web-first assertion, not page.waitForFunction: the page ships a
    // strict `script-src 'self'` CSP with no 'unsafe-eval', and
    // waitForFunction's in-page polling loop needs eval, so it raises
    // EvalError against this (correct) policy. This form polls out of
    // page and needs no eval. The value is asserted concretely because
    // web/pwa/wiki.js sets the literal "true".
    await expect(page.locator("html")).toHaveAttribute("data-wiki-ready", "true");

    if (disabled) {
      // SCN-080-001-04: no local navigation, status, or static page may
      // claim a working Graph journey.
      await expect(
        page.locator("#wiki-landing-nav"),
        "a disabled deployment must not offer section navigation at all — an " +
          "aria-disabled link is still keyboard-reachable and still advertises " +
          "a section that cannot load",
      ).toHaveCount(0);
      await expect(page.locator("#wiki-landing-status")).toHaveAttribute(
        "data-graph-state",
        STATE_DISABLED,
      );
      await expect(page.locator("#wiki-landing-subtitle")).not.toContainText("Browse your knowledge graph");
      // The word "Available" must never appear as a claim while disabled.
      await expect(page.locator("body")).not.toContainText(/\bAvailable\b/);

      // Everything above is a NEGATION, and a page that rendered a blank
      // panel would satisfy every one of them. SCN-080-001-04 also
      // requires the shell to SHOW the unavailable explanation, so assert
      // it is actually on screen. Scoped to the status element's own
      // message node, not the body: #wiki-landing-subtitle carries
      // similar wording, and matching that instead would prove nothing
      // about the status region.
      const disabledStatus = page.locator("#wiki-landing-status");
      await expect(
        disabledStatus,
        "the disabled status region must be perceivable, not merely present in the DOM",
      ).toBeVisible();
      const disabledExplanation = disabledStatus.locator(".graph-state-message");
      await expect(
        disabledExplanation,
        "the disabled explanation must be rendered and visible, not an empty panel",
      ).toBeVisible();
      await expect(
        disabledExplanation,
        "the disabled state must say WHY the graph is unavailable",
      ).toContainText(DISABLED_EXPLANATION_FRAGMENT);

      // That copy ends "there is nothing to retry", and web/pwa/wiki_state.js
      // backs the promise structurally: the disabled entry carries no
      // `action`, unlike store-unavailable and degraded which both define
      // "Try this view again". Assert the promise holds. A deliberately
      // disabled capability must offer no recovery affordance, because
      // retrying can never help — an offered retry would be a real
      // contradiction between the copy and the behaviour.
      await expect(
        disabledStatus.locator(".graph-state-action"),
        "a deliberately disabled capability must not offer a retry action",
      ).toHaveCount(0);

      // T080-04-UI requires durable visual evidence. playwright.config.ts
      // keeps `screenshot: "only-on-failure"` globally — correct for the
      // suite, but it means a PASSING disabled run leaves no image to
      // cite. Capture one explicitly, in this branch only, and attach it
      // so the artifact is associated with this test in the report
      // instead of being a loose file nothing points at.
      const disabledShot = testInfo.outputPath("graph-disabled-wiki-landing.png");
      await page.screenshot({ path: disabledShot, fullPage: true });
      await testInfo.attach("graph-disabled-wiki-landing.png", {
        path: disabledShot,
        contentType: "image/png",
      });
    } else {
      // The aggregate says the capability is NOT deliberately off, so the
      // UI must not claim it is. This branch asserts just as hard as the
      // other one — it is the agreement invariant, not a skip.
      await expect(
        page.locator("#wiki-landing-status"),
        "an enabled deployment must not paint the disabled state",
      ).not.toHaveAttribute("data-graph-state", STATE_DISABLED);
      await expect(page.locator("#wiki-landing-nav")).toHaveCount(1);
    }

    // In BOTH cases readiness must never be advertised as ready unless the
    // published aggregate says so. This is the claim the bug was filed for.
    if (!aggregate!.ready) {
      await expect(
        page.locator("body"),
        "the shell must not claim a ready Graph journey while the published " +
          "aggregate reports not-ready",
      ).not.toContainText(/Graph is ready|Graph: ready/i);
    }
  });

  test("Regression: all-family true empty is actionable and contains no sample topology", async ({ page }) => {
    await authenticate(page);
    const aggregate = await readPublishedAggregate(page.request);
    expect(aggregate).not.toBeNull();

    await page.goto("/pwa/wiki_topics.html");
    await expect(page.locator("#wiki-topics-index")).toHaveAttribute("aria-busy", "false");
    const state = await paintedState(page, "#wiki-topics-status");

    // Whatever the stack's data situation is, the painted state must be
    // consistent with it: a graph with no rows is true-empty, never a
    // silently blank list, and never an error.
    const rows = await page.locator("#wiki-topics-list .wiki-list-item").count();
    if (state === STATE_READY) {
      expect(rows, "the ready state must be backed by at least one real row").toBeGreaterThan(0);
    } else if (state === STATE_TRUE_EMPTY) {
      expect(rows, "true-empty must render no fabricated rows").toBe(0);
      // SCN-080-001-05: actionable guidance, and NOTHING that reads as a
      // retry, a route error, or an unavailable claim.
      await expect(page.locator("#wiki-topics-status")).toContainText(
        "Nothing has been synthesized into your knowledge graph yet",
      );
      await expect(
        page.locator("#wiki-topics-status .graph-state-action"),
        "true-empty offers a capture/source next step, not a retry",
      ).toHaveAttribute("href", "/pwa/connectors.html");
      await expect(page.locator("#wiki-topics-status")).not.toContainText(/unavailable|error|failed|retry/i);
      // A polite status, never an alert: an empty graph is not a fault.
      await expect(page.locator("#wiki-topics-status")).toHaveAttribute("role", "status");
    } else {
      // Any other state must be a genuine fault state, not a disguised
      // empty. Asserting membership keeps this branch from being a pass-all.
      expect(
        [STATE_DISABLED, STATE_UNAUTHENTICATED, STATE_FORBIDDEN, STATE_ROUTE_ABSENT,
          STATE_STORE_UNAVAILABLE, STATE_SCHEMA_INVALID, STATE_DEGRADED, STATE_PARTIAL],
        `unexpected state ${state} on the topics index`,
      ).toContain(state);
      expect(rows, "a fault state must not leave stale rows on screen").toBe(0);
    }

    // No sample/demo topology may ever be shipped as if it were the
    // user's own graph.
    await expect(page.locator("body")).not.toContainText(/sample|demo|example graph|placeholder/i);
  });

  test("Regression: auth route store and schema failures are exclusive and private", async ({ page, context }, testInfo) => {
    await authenticate(page);

    // Paint a real authenticated view first so there IS prior private
    // content to leak. Without this the privacy assertion below would be
    // vacuous — it would pass against a page that never had content.
    await page.goto("/pwa/wiki_topics.html");
    await expect(page.locator("#wiki-topics-index")).toHaveAttribute("aria-busy", "false");
    const priorState = await paintedState(page, "#wiki-topics-status");
    expect(ALL_UI_STATES, "the authenticated paint must resolve a real state").toContain(priorState);

    // The authenticated paint above is the only moment in this file where a
    // REAL store outage is observable, so assert that state's full contract
    // here before the session drop below overwrites it. The lane induces the
    // outage by stopping the graph store container on a running stack
    // (scripts/runtime/web-e2e-ui.sh store-unavailable phase); on a healthy
    // stack the else arm runs instead. BOTH arms assert — the else arm proves
    // no OTHER state may wear the store fault's copy or its recovery
    // affordance — so this is an exclusivity check, not a bailout.
    const priorStatus = page.locator("#wiki-topics-status");
    if (priorState === STATE_STORE_UNAVAILABLE) {
      // An unreachable store must yield nothing rather than something
      // plausible. This is the whole promise of the fail-soft contract.
      await expect(
        page.locator("#wiki-topics-list .wiki-list-item"),
        "an unreachable store must not leave or invent rows on screen",
      ).toHaveCount(0);

      // Perceivable, and it explains itself — a blank panel would satisfy
      // every negation but tell the user nothing.
      await expect(
        priorStatus,
        "the store fault must be perceivable, not merely present in the DOM",
      ).toBeVisible();
      await expect(
        priorStatus.locator(".graph-state-message"),
        "the store fault must say the store is unreachable and the data is not lost",
      ).toContainText(STORE_UNAVAILABLE_EXPLANATION_FRAGMENT);

      // The deliberate contrast with `disabled`. web/pwa/wiki_state.js gives
      // this state an action ("Try this view again") and gives disabled NONE,
      // because a transient outage CAN be recovered from and a deliberate
      // configuration cannot. Assert the affordance is present AND natively
      // operable: a pointer-only <div> would be a real defect, and an
      // unfocusable one would strand a keyboard user at the outage.
      const storeAction = priorStatus.locator(".graph-state-action");
      await expect(
        storeAction,
        "a retryable outage must offer exactly one recovery action, unlike disabled",
      ).toHaveCount(1);
      const storeActionTag = await storeAction.evaluate((n) => n.tagName.toLowerCase());
      expect(
        ["a", "button"],
        `store recovery action must be natively focusable, got <${storeActionTag}>`,
      ).toContain(storeActionTag);
      await storeAction.focus();
      await expect(
        storeAction,
        "the store recovery action must be reachable by keyboard",
      ).toBeFocused();

      // A fault announces assertively. true-empty is a polite `status`
      // because an empty graph is not a fault; this IS one, and announcing
      // it politely would let the outage pass unnoticed.
      await expect(priorStatus).toHaveAttribute("role", "alert");
      await expect(priorStatus).toHaveAttribute("aria-live", "assertive");
      expect(
        await page.locator("#wiki-topics-index [role=alert], #wiki-topics-index [role=status]").count(),
        "exactly one live region may be present while the store is unreachable",
      ).toBe(1);

      // Evidence line, in the same console idiom chaos_saga_20260702.spec.ts
      // uses (the reporter prints test stdout). Because this file is
      // state-adaptive, a PASS alone cannot tell a reader WHICH arm ran, and
      // a phase that silently reverted to a healthy stack would look
      // identical. Printing the observed arm makes that distinguishable from
      // the lane output instead of inferable. Emitted AFTER the assertions,
      // so it can only appear when they held.
      console.log(
        `GRAPH-EV store-unavailable | painted=${priorState} | rows=0 | ` +
          `action=<${storeActionTag}> "${(await storeAction.textContent())?.trim()}" | ` +
          "role=alert aria-live=assertive",
      );
    } else {
      // Exclusivity in the other direction: only the store fault may wear
      // the store fault's copy and its retry affordance. A state model that
      // leaked either into a neighbouring state fails here. Note degraded
      // also labels its button "Try this view again", so the marker — not
      // the label — is what distinguishes them.
      await expect(
        priorStatus.locator('[data-graph-retry="store-unavailable"]'),
        `${priorState} must not offer the store fault's retry affordance`,
      ).toHaveCount(0);
      await expect(
        priorStatus,
        `${priorState} must not borrow the store fault's explanation`,
      ).not.toContainText(STORE_UNAVAILABLE_EXPLANATION_FRAGMENT);

      // Same purpose as the line above: name the arm that ran, so a store
      // phase that failed to induce the outage is visible in the output
      // rather than passing as an unremarkable green.
      console.log(`GRAPH-EV store-exclusivity | painted=${priorState} | storeRetryAffordance=0 | storeCopy=absent`);
    }

    // Value-safe in EVERY state, the store fault above included: the status
    // may name a closed condition but never the request path or the status
    // line that produced it.
    await expect(priorStatus).not.toContainText("/api/");
    await expect(priorStatus).not.toContainText(/HTTP \d\d\d/);

    // Induce a REAL session rejection: drop the session cookie. The next
    // read gets a genuine 401 from the server — no interception.
    await context.clearCookies();
    await page.goto("/pwa/wiki_topics.html");
    await expect(page.locator("#wiki-topics-index")).toHaveAttribute("aria-busy", "false");

    const afterState = await paintedState(page, "#wiki-topics-status");
    expect(
      [STATE_UNAUTHENTICATED, STATE_FORBIDDEN],
      `dropping the session must produce an identity state, got ${afterState}`,
    ).toContain(afterState);

    // Exclusivity: the identity state must NOT be dressed as any other.
    await expect(page.locator("#wiki-topics-status")).not.toHaveAttribute("data-graph-state", STATE_TRUE_EMPTY);
    await expect(page.locator("#wiki-topics-status")).not.toHaveAttribute("data-graph-state", STATE_ROUTE_ABSENT);
    await expect(page.locator("#wiki-topics-status")).not.toHaveAttribute("data-graph-state", STATE_STORE_UNAVAILABLE);

    // Privacy: prior rows, labels and counts must be GONE, not merely hidden
    // behind the recovery UI.
    await expect(
      page.locator("#wiki-topics-list .wiki-list-item"),
      "prior private rows must be removed before the recovery state paints",
    ).toHaveCount(0);
    await expect(page.locator("#wiki-topics-list")).not.toHaveAttribute("data-people-count", /.*/);

    // Recovery must be a real, reachable action.
    await expect(page.locator("#wiki-topics-status .graph-state-action")).toHaveAttribute(
      "href",
      "/pwa/index.html",
    );
    // A fault announces once, assertively, and never leaks the request path.
    await expect(page.locator("#wiki-topics-status")).toHaveAttribute("role", "alert");
    await expect(page.locator("#wiki-topics-status")).not.toContainText("/api/");
    await expect(page.locator("#wiki-topics-status")).not.toContainText(/HTTP \d\d\d/);
    // Exactly one live region, so a screen reader is not told twice.
    expect(
      await page.locator("#wiki-topics-index [role=alert], #wiki-topics-index [role=status]").count(),
      "exactly one live region may be present per paint",
    ).toBe(1);

    // The pixel half of the privacy claim. Everything above proves the DOM
    // and accessibility halves — prior rows removed, no surviving count
    // attribute, one assertive live region, no path or status-code leak —
    // but a row can be absent from the DOM and still be on screen (a stale
    // paint, a detached overlay, a cached canvas). This artifact is the
    // durable visual evidence that NO prior private graph content remains
    // visible after session loss. Captured explicitly because
    // playwright.config.ts keeps `screenshot: "only-on-failure"`, so a
    // PASSING run would otherwise leave no image to cite. Same
    // outputPath/attach pattern as the disabled capture above, so the
    // artifact is associated with this test rather than a loose file.
    const authLossShot = testInfo.outputPath("auth-loss-privacy.png");
    await page.screenshot({ path: authLossShot, fullPage: true });
    await testInfo.attach("auth-loss-privacy.png", {
      path: authLossShot,
      contentType: "image/png",
    });
  });

  test("Knowledge Graph activation states remain keyboard and screen-reader operable at desktop and 320px 200 percent zoom", async ({ page }) => {
    await authenticate(page);

    for (const viewport of [
      { width: 1280, height: 800, label: "desktop" },
      // 320 CSS px at 200% zoom is the spec's narrow target: emulate by
      // halving the layout viewport at double scale.
      { width: 320, height: 640, label: "320px/200%" },
    ]) {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await page.goto("/pwa/wiki_topics.html");
      await expect(page.locator("#wiki-topics-index")).toHaveAttribute("aria-busy", "false");

      const state = await paintedState(page, "#wiki-topics-status");
      expect(ALL_UI_STATES).toContain(state);

      // No horizontal page scroll at any state or width.
      const overflows = await page.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
      );
      expect(overflows, `horizontal page scroll at ${viewport.label}`).toBe(false);

      // The status contract is state-dependent, and BOTH arms assert —
      // this is not a bailout. web/pwa/wiki_state.js renderReadState
      // deliberately gives STATE_READY no message at all: it sets
      // statusNode.hidden and strips role and aria-live, because a
      // working view has nothing to announce. Asserting visibility
      // unconditionally would demand the opposite of that design, and
      // would only ever have passed because an author `display` rule
      // was overriding the `hidden` attribute.
      if (state === STATE_READY) {
        await expect(
          page.locator("#wiki-topics-status"),
          `a ready view carries no status message, so nothing may be painted at ${viewport.label}`,
        ).not.toBeVisible();
        expect(
          await page.locator("#wiki-topics-index [role=alert], #wiki-topics-index [role=status]").count(),
          `a ready view must announce nothing at ${viewport.label}`,
        ).toBe(0);
      } else {
        // The status must be perceivable, not merely present.
        await expect(
          page.locator("#wiki-topics-status"),
          `the ${state} state must be perceivable at ${viewport.label}`,
        ).toBeVisible();
        // Exactly one live region per paint, at every width.
        expect(
          await page.locator("#wiki-topics-index [role=alert], #wiki-topics-index [role=status]").count(),
          `exactly one live region at ${viewport.label}`,
        ).toBe(1);
      }

      // No overlap between the status region and the family rows. This is
      // a separate failure mode from horizontal scroll: a status block
      // can sit ON TOP of content at a narrow width while the page still
      // fits horizontally, which silently obscures verified rows. Boxes
      // are null when a node is not rendered (the ready state hides the
      // status), and a non-rendered node cannot overlap anything.
      const statusBox = await page.locator("#wiki-topics-status").boundingBox();
      const listBox = await page.locator("#wiki-topics-list").boundingBox();
      if (statusBox && listBox) {
        const intersects =
          statusBox.x < listBox.x + listBox.width &&
          listBox.x < statusBox.x + statusBox.width &&
          statusBox.y < listBox.y + listBox.height &&
          listBox.y < statusBox.y + statusBox.height;
        expect(
          intersects,
          `status region overlaps the family rows at ${viewport.label}`,
        ).toBe(false);
      }

      // Any offered recovery action must be keyboard-operable, not
      // pointer-only. Buttons and links both qualify; a div would not.
      const action = page.locator("#wiki-topics-status .graph-state-action");
      if ((await action.count()) > 0) {
        const tag = await action.first().evaluate((n) => n.tagName.toLowerCase());
        expect(["a", "button"], `recovery action must be natively focusable, got <${tag}>`).toContain(tag);
        await action.first().focus();
        await expect(action.first()).toBeFocused();
      }
    }
  });

  test("Knowledge and Wiki journeys remain coherent after Graph activation repair", async ({ page }) => {
    await authenticate(page);
    const aggregate = await readPublishedAggregate(page.request);
    expect(aggregate).not.toBeNull();
    const disabled = aggregate!.state === AGGREGATE_POLICY_DISABLED;

    // T080-REGRESSION: every graph surface must resolve through the ONE
    // model and agree with the published aggregate. Before this scope the
    // pages each derived their own state, so they could disagree.
    const surfaces: Array<{ path: string; section: string; status: string }> = [
      { path: "/pwa/wiki_topics.html", section: "#wiki-topics-index", status: "#wiki-topics-status" },
      { path: "/pwa/wiki_people.html", section: "#wiki-people-index", status: "#wiki-people-status" },
      { path: "/pwa/wiki_places.html", section: "#wiki-places-index", status: "#wiki-places-status" },
      { path: "/pwa/wiki_time.html", section: "#wiki-time-section", status: "#wiki-time-status" },
    ];

    const observed: string[] = [];
    for (const surface of surfaces) {
      await page.goto(surface.path);
      await expect(page.locator(surface.section)).toHaveAttribute("aria-busy", "false");
      const state = await paintedState(page, surface.status);
      observed.push(state);

      if (disabled) {
        expect(state, `${surface.path} must report disabled when the aggregate does`).toBe(STATE_DISABLED);
      } else {
        expect(
          state,
          `${surface.path} must not report disabled while the aggregate reports ${aggregate!.state}`,
        ).not.toBe(STATE_DISABLED);
      }
    }

    // Coherence: the surfaces share one backend condition, so they must
    // not disagree about whether the capability is on.
    const disabledCount = observed.filter((s) => s === STATE_DISABLED).length;
    expect(
      disabledCount === 0 || disabledCount === observed.length,
      `graph surfaces disagree about activation: ${JSON.stringify(observed)}`,
    ).toBeTruthy();

    // The pre-existing Wiki landing journey still works.
    await page.goto("/pwa/wiki.html");
    // Same CSP-safe readiness wait as above — no in-page eval.
    await expect(page.locator("html")).toHaveAttribute("data-wiki-ready", "true");
    await expect(page.locator("h1")).toHaveText("Wiki");
  });

  /**
   * F-080-06-ROWMISS regression.
   *
   * A DETAIL read whose row is gone is NOT an absent route. The server
   * already says so precisely: internal/api/graphapi/topics.go answers
   * `404` with the TYPED envelope code `not_found`, whereas a genuinely
   * unmounted route yields a bare Chi `404` with no envelope at all. The
   * Go model encodes exactly this distinction and corrects for it in
   * internal/graphsynthetic/synthetic.go ("A populated list whose own
   * first row 404s is a missing row, not an absent route.").
   *
   * The UI model had drifted from it: web/pwa/wiki_state.js classified
   * EVERY 404 as CODE_ROUTE_ABSENT regardless of the typed code, so a
   * stale or deleted topic link told the user "This knowledge graph view
   * is not available in the running build. It is not empty — it is not
   * deployed." That is false — the view IS deployed — and route-absent
   * carries NO recovery action, so the user was dead-ended by a wrong
   * explanation. It also left CODE_ROW_MISSING unreachable, i.e. dead.
   *
   * This test induces the condition with a REAL server 404: a well-formed
   * but non-existent id on the live stack. No interception, no fixture.
   * It is state-adaptive without a bailout — it first asks the API what
   * actually happened for that id, then requires the UI to agree with
   * THAT. On a disabled or store-down stack the API answers 503 and the
   * matching arm asserts; only when the API genuinely answered 404 does
   * the row-missing arm apply. Every arm asserts.
   */
  test("Regression: a missing graph row is not reported as an absent route", async ({
    page,
  }) => {
    await authenticate(page);

    // Well-formed ULID shape, guaranteed absent: the boot seed uses real
    // generated ULIDs, and this all-zero id can never collide with one.
    const absentId = "00000000000000000000000000";

    // Ask the live server what this id actually yields, so the UI
    // assertion below is anchored to the real backend condition.
    const probe = await page.request.get(`/api/topics/${absentId}`, {
      headers: { Accept: "application/json" },
    });
    const probeStatus = probe.status();
    let probeCode = "";
    try {
      const body = await probe.json();
      if (body && body.error && typeof body.error.code === "string") {
        probeCode = body.error.code;
      }
    } catch {
      // A bare 404 from an unmounted route has no parseable envelope.
      // Leaving probeCode empty is the meaningful outcome, not an error.
    }

    await page.goto(`/pwa/wiki_topics.html?id=${absentId}`);
    // The DETAIL view signals readiness on its own section via
    // markReady() in web/pwa/wiki_lib.js. `data-wiki-ready` belongs to the
    // landing page (wiki.js) only and is never set here, so waiting on it
    // would time out and report a harness fault as a product failure.
    await expect(page.locator("#wiki-topic-detail")).toHaveAttribute("aria-busy", "false");

    const painted = await paintedState(page, "#wiki-topic-status");
    const statusText = ((await page.locator("#wiki-topic-status").textContent()) ?? "").trim();

    // eslint-disable-next-line no-console
    console.log(
      `GRAPH-EV row-missing | probeStatus=${probeStatus} | probeCode=${probeCode || "<none>"} | painted=${painted}`,
    );

    if (probeStatus === 404 && probeCode === "not_found") {
      // The route ANSWERED. It is present. Claiming otherwise is a lie.
      expect(
        painted,
        "a typed 404 not_found means the row is gone, not that the route is absent",
      ).not.toBe(STATE_ROUTE_ABSENT);
      expect(
        statusText,
        "the user must not be told a deployed view is 'not deployed'",
      ).not.toMatch(/not deployed|not available in the running build/i);
      // Personal content must not survive a failed detail read.
      await expect(page.locator("#wiki-topic-detail-heading")).toHaveText("");
    } else if (probeStatus === 503) {
      // Disabled or store-down stack: the UI must agree with THAT.
      expect(
        [STATE_DISABLED, STATE_STORE_UNAVAILABLE],
        `a 503 (${probeCode || "<none>"}) must paint disabled or store-unavailable, not ${painted}`,
      ).toContain(painted);
    } else {
      // Any other backend answer still must not be presented as a
      // working graph built from a row that does not exist.
      expect(
        painted,
        `an absent row must never paint ready (probe ${probeStatus})`,
      ).not.toBe(STATE_READY);
    }
  });

  /**
   * T080-06-RENDER — ui-unit, NOT e2e-ui. Classified honestly.
   *
   * `route-absent` and `schema-invalid` are UNREACHABLE against a
   * current, correctly-functioning core, and that is this bug's own
   * doing rather than a gap:
   *
   *   * route-absent needs a BARE 404. internal/api/router.go registers
   *     the graph manifest atomically behind one always-true
   *     GraphCapability guard, so every known graph path is mounted in
   *     BOTH the enabled and disabled states and a disabled deployment
   *     answers a typed 503 instead. A silent Chi 404 cannot happen.
   *   * schema-invalid needs a 400 or a typed 5xx. The PWA builds every
   *     graph request from fixed internal defaults -- it never sends a
   *     user-controlled cursor, window or kind -- so it cannot elicit
   *     one from a healthy server.
   *
   * They remain correct DEFENSIVE states (version skew during a rolling
   * deploy, a misrouting proxy, a future paginating UI). Their RENDER
   * contract is therefore verified here by executing the REAL module in
   * a REAL browser against a REAL DOM node. Nothing is mocked: these are
   * the shipped functions called with real inputs. What is not claimed
   * is live inducement -- hence ui-unit, and hence this test does not
   * satisfy any live-stack DoD row.
   *
   * It is deliberately a WHOLE-VOCABULARY sweep rather than two cases.
   * F-080-06-ROWMISS showed a source-text containment test can pass
   * while a real branch is wrong, so this asserts behaviour: that all
   * ten states render DISTINCTLY and cannot collapse into one another.
   */
  test("Unit: every closed graph state renders a distinct exclusive contract", async ({ page }) => {
    await authenticate(page);
    await page.goto("/pwa/wiki_topics.html");
    await expect(page.locator("#wiki-topics-index")).toHaveAttribute("aria-busy", "false");

    const result = await page.evaluate(async () => {
      const m = await import("/pwa/wiki_state.js");

      // 1. The REAL classifier across the status/envelope matrix.
      const classified = {
        bare404: m.classifyStatus(404, ""),
        typed404NotFound: m.classifyStatus(404, "not_found"),
        plain400: m.classifyStatus(400, ""),
        cursor400: m.classifyStatus(400, "invalid_cursor"),
        schema500: m.classifyStatus(500, "schema_error"),
        plain500: m.classifyStatus(500, ""),
        disabled503: m.classifyStatus(503, "capability_disabled"),
        store503: m.classifyStatus(503, "store_unavailable"),
        bare503: m.classifyStatus(503, ""),
        unauth401: m.classifyStatus(401, ""),
        forbidden403: m.classifyStatus(403, ""),
      };

      // 2. The REAL renderer, for every closed state, into a real node.
      const states = [
        m.STATE_LOADING, m.STATE_READY, m.STATE_TRUE_EMPTY, m.STATE_PARTIAL,
        m.STATE_DEGRADED, m.STATE_DISABLED, m.STATE_UNAUTHENTICATED,
        m.STATE_FORBIDDEN, m.STATE_ROUTE_ABSENT, m.STATE_STORE_UNAVAILABLE,
        m.STATE_SCHEMA_INVALID,
      ];
      // Each state is rendered TWICE: once with no retry handler, and
      // once with a real one. The renderer only paints a retry button
      // when the CALLER can actually retry (copy.action plus an onRetry
      // function), which is why a single-shot render would understate
      // the contract. The interesting question is not "is a button
      // present" but "does this state offer retry EVEN WHEN retry is
      // available" -- that is what separates a transient fault from a
      // permanent one.
      const render = (st, opts) => {
        const node = document.createElement("div");
        document.body.appendChild(node);
        m.renderReadState(node, st, opts);
        const action = node.querySelector(".graph-state-action");
        const out = {
          declared: node.getAttribute("data-graph-state"),
          message: (node.querySelector(".graph-state-message")?.textContent ?? "").trim(),
          actionCount: node.querySelectorAll(".graph-state-action").length,
          actionTag: action ? action.tagName.toLowerCase() : "",
          actionHref: action ? (action.getAttribute("href") ?? "") : "",
          role: node.getAttribute("role") ?? "",
        };
        node.remove();
        return out;
      };

      const rendered = [];
      for (const st of states) {
        rendered.push({
          state: st,
          bare: render(st, {}),
          withRetry: render(st, { onRetry: () => {} }),
        });
      }
      return { classified, rendered };
    });

    // eslint-disable-next-line no-console
    console.log(`GRAPH-EV render-matrix | ${JSON.stringify(result.classified)}`);

    // --- Classifier: the distinctions that carry meaning ---
    expect(result.classified.bare404, "a bare 404 is a genuinely absent route").toBe(
      "F080-SYNTH-ROUTE-ABSENT",
    );
    expect(
      result.classified.typed404NotFound,
      "a typed 404 not_found is a missing ROW, not an absent route",
    ).toBe("F080-SYNTH-ROW-MISSING");
    expect(
      result.classified.bare404 === result.classified.typed404NotFound,
      "the two 404 meanings must not collapse into one code",
    ).toBe(false);
    expect(result.classified.plain400).toBe("F080-SYNTH-SCHEMA-INVALID");
    expect(result.classified.cursor400).toBe("F080-SYNTH-CURSOR-INVALID");
    expect(result.classified.schema500).toBe("F080-SYNTH-SCHEMA-INVALID");
    expect(result.classified.plain500).toBe("F080-SYNTH-SERVER-ERROR");
    expect(
      result.classified.disabled503 === result.classified.store503,
      "deliberately-off must not collapse into store-is-down",
    ).toBe(false);
    expect(result.classified.unauth401).toBe("F080-SYNTH-UNAUTHENTICATED");
    expect(
      result.classified.unauth401 === result.classified.forbidden403,
      "a rejected session must not collapse into a denied scope",
    ).toBe(false);

    // --- Renderer: every state declares itself, and none collapse ---
    const byState = new Map(result.rendered.map((r) => [r.state, r]));
    const declared = result.rendered.map((r) => r.bare.declared);
    expect(
      new Set(declared).size,
      `all ${declared.length} states must declare distinct values, got ${JSON.stringify(declared)}`,
    ).toBe(declared.length);
    for (const r of result.rendered) {
      expect(r.bare.declared, `${r.state} must declare its own state`).toBe(r.state);
    }

    // --- The recovery-affordance contract ---
    //
    // This is the user-visible difference between "retrying may help"
    // and "retrying cannot help". Asserted against the render that WAS
    // given a working retry handler, so a state that withholds retry is
    // proven to withhold it deliberately rather than for want of a
    // callback.
    for (const st of ["route-absent", "schema-invalid", "disabled"]) {
      const r = byState.get(st)!;
      expect(r.bare.message.length, `${st} must explain itself`).toBeGreaterThan(0);
      expect(
        r.withRetry.actionCount,
        `${st} must offer NO retry even when a retry handler is available: retrying cannot resolve it`,
      ).toBe(0);
    }

    // The deliberate contrast: a transient store fault IS retryable, but
    // only when the caller can actually perform the retry. Painting a
    // dead retry button would be worse than painting none.
    const storeDown = byState.get("store-unavailable")!;
    expect(
      storeDown.bare.actionCount,
      "no retry button may be painted when the caller supplied no way to retry",
    ).toBe(0);
    expect(
      storeDown.withRetry.actionCount,
      "store-unavailable must offer a retry when one is possible",
    ).toBe(1);
    expect(["a", "button"]).toContain(storeDown.withRetry.actionTag);

    // true-empty routes to capture rather than retry: it is a LINK, and
    // it is offered unconditionally because it needs no retry handler.
    const trueEmpty = byState.get("true-empty")!;
    expect(trueEmpty.bare.actionCount, "true-empty guidance needs no retry handler").toBe(1);
    expect(trueEmpty.bare.actionTag).toBe("a");
    expect(trueEmpty.bare.actionHref, "true-empty must point at capture, not retry").toBe(
      "/pwa/connectors.html",
    );

    expect(byState.get("ready")!.bare.message, "ready has nothing to say").toBe("");
  });
});
