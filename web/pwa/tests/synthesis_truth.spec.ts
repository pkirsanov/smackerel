// BUG-004-004 SCOPE-05 — Today and Status synthesis truth in a real browser.
//
// These run against the live stack with no request interception. That matters
// more here than usual: the whole point of this bug is that a page must report
// what is actually in the database, so a test that fed the page its own answer
// would be testing nothing at all.
import { test, expect, type Page } from "@playwright/test";
import { login } from "./_support/cardrewards";

// The closed set from internal/web/synthesis_projection.go. Asserting
// membership catches a template that invents a state name, and asserting there
// is exactly ONE such element catches a template that renders two states at
// once — the failure mode the projection exists to make impossible.
const SYNTHESIS_STATES = [
  "never_run",
  "current",
  "quiet",
  "stale",
  "partial",
  "failed_without_output",
  "failed_with_prior_output",
  "unavailable",
  "auth_required",
];

const CONTENT_STATES = ["current", "stale", "partial"];

// States that mean "this reader was not shown the durable answer". For a LOGGED
// IN reader every one of these is a wiring failure, not an outcome. They are
// called out by name because both of them silently satisfy every other
// assertion in this file: auth_required and unavailable are members of the
// closed set, are not never_run, are not failure states, carry no prose and
// offer no retry. A suite that did not reject them here would pass green
// against a page that never reads the database at all.
const NON_ANSWER_STATES = ["auth_required", "unavailable"];

async function requireAnsweredState(page: Page, path: string): Promise<string> {
  const state = await synthesisStateOn(page, path);
  expect(
    NON_ANSWER_STATES,
    `${path} reported '${state}' for an authenticated reader; the page is not reading durable state, so every other assertion here would be vacuous`,
  ).not.toContain(state);
  return state;
}

async function synthesisStateOn(page: Page, path: string): Promise<string> {
  await page.goto(path);
  const sections = page.locator("#synthesis-outcome");
  await expect(
    sections,
    `${path} must render exactly one synthesis section; two would mean two states are showing at once`,
  ).toHaveCount(1);
  const state = await sections.getAttribute("data-synthesis-state");
  expect(
    SYNTHESIS_STATES,
    `${path} reported synthesis state '${state}', which is not in the closed set the projection defines`,
  ).toContain(state);
  return state ?? "";
}

test("Today renders exactly one synthesis state drawn from the closed set", async ({
  page,
}) => {
  await login(page, "/digest");
  await requireAnsweredState(page, "/digest");
});

test("a real committed synthesis is rendered from storage, never as empty or broken", async ({
  page,
}) => {
  await login(page, "/digest");

  // Drive a REAL synthesis through the live API so the page has something
  // durable to report. Without this the check could pass against a never-run
  // system and prove nothing about rendering.
  const retry = await page.request.post("/api/synthesis/retry", {
    data: { cadence: "daily" },
  });
  expect(
    [200, 409],
    `synthesis retry must be accepted or report an existing run; got ${retry.status()}`,
  ).toContain(retry.status());
  const body = await retry.json();
  const outputId = body?.output?.outputId ?? "";
  expect(
    outputId,
    "a completed synthesis trigger must name its output; without one there is no persisted run to render",
  ).not.toBe("");

  const state = await requireAnsweredState(page, "/digest");

  // A committed output must never read back as though nothing ever ran, and a
  // quiet window must never read back as a failure.
  expect(
    state,
    `output ${outputId} is committed yet Today reports never_run; emptiness is being shown as absence`,
  ).not.toBe("never_run");
  expect(
    state,
    `output ${outputId} is committed yet Today reports failed_without_output; a completed run is not a failure`,
  ).not.toBe("failed_without_output");

  const section = page.locator("#synthesis-outcome");
  if (CONTENT_STATES.includes(state)) {
    // A content state must actually carry content, and every rendered insight
    // must disclose that it is sourced.
    const insights = section.locator(".synthesis-insight");
    await expect(
      insights,
      `state '${state}' is a content state but rendered no insight; a headline with nothing under it is not an answer`,
    ).not.toHaveCount(0);
    const citationBadges = section.locator("[data-citation-count]");
    await expect(
      citationBadges.first(),
      "a rendered insight must disclose its citation count",
    ).toBeVisible();
  } else {
    // A non-content state must render NO synthesis prose at all.
    await expect(
      section.locator(".synthesis-insight"),
      `state '${state}' must not render synthesis prose`,
    ).toHaveCount(0);
  }
});

test("Today and Status report the same durable synthesis state", async ({
  page,
}) => {
  await login(page, "/digest");
  const todayState = await requireAnsweredState(page, "/digest");
  const statusState = await requireAnsweredState(page, "/status");

  // Both pages read one model through one reader. If they can disagree, one of
  // them is answering from something other than the durable state.
  expect(
    statusState,
    `Today reports '${todayState}' and Status reports '${statusState}' for the same durable state; the two surfaces disagree`,
  ).toBe(todayState);
});

test("an unauthenticated visitor is never shown synthesis content", async ({
  page,
}) => {
  // No login on this context. A protected page must turn the visitor away
  // rather than render another reader's synthesis.
  for (const path of ["/digest", "/status"]) {
    const response = await page.goto(path);
    expect(response, `GET ${path} returned no response`).not.toBeNull();
    const status = response!.status();
    const turnedAway = status === 401 || status === 403 || page.url().includes("/login");
    expect(
      turnedAway,
      `${path} unauthenticated returned ${status} at ${page.url()}; expected an honest auth outcome, never a rendered page`,
    ).toBe(true);

    // Structural: whatever was served carries no synthesis prose.
    await expect(
      page.locator("#synthesis-outcome .synthesis-insight"),
      `${path} unauthenticated rendered synthesis prose; private content survived an auth failure`,
    ).toHaveCount(0);
  }
});

test("the retry control is offered only when synthesis has actually failed", async ({
  page,
}) => {
  await login(page, "/digest");
  const state = await requireAnsweredState(page, "/digest");
  const retryControl = page.locator("[data-synthesis-retry]");

  if (state === "failed_without_output" || state === "failed_with_prior_output") {
    await expect(
      retryControl,
      `state '${state}' is a failure and must offer a retry`,
    ).toBeVisible();
  } else {
    // Offering retry in a healthy state would imply something is wrong when it
    // is not, which is the same dishonesty pointing the other way.
    await expect(
      retryControl,
      `state '${state}' is not a failure yet a retry control is offered, implying a problem that does not exist`,
    ).toHaveCount(0);
  }
});
