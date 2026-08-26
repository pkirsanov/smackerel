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
    await section.screenshot({
      path: "synthesis-evidence/today-content.png",
    });
  } else {
    // A non-content state must render NO synthesis prose at all.
    await expect(
      section.locator(".synthesis-insight"),
      `state '${state}' must not render synthesis prose`,
    ).toHaveCount(0);
    await section.screenshot({
      path: `synthesis-evidence/today-${state}.png`,
    });
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

test("the synthesis section is reachable and labelled in the accessibility tree", async ({
  page,
}) => {
  await login(page, "/digest");
  await requireAnsweredState(page, "/digest");

  // A screen-reader user must be able to find this section by its heading, not
  // only by its DOM id. Querying by ROLE is what makes that a real assertion:
  // a div that merely looks like a heading does not satisfy it.
  const heading = page.getByRole("heading", { name: "Synthesis" });
  await expect(
    heading,
    "the synthesis section must expose a heading in the accessibility tree",
  ).toBeVisible();

  const section = page.locator("#synthesis-outcome");
  await expect(
    section,
    "the section must be labelled by its heading so assistive technology can announce it",
  ).toHaveAttribute("aria-labelledby", "synthesis-heading");
});

test("auth loss removes synthesis content from the accessibility tree, not just the DOM", async ({
  page,
}) => {
  // No login. Content leaving the DOM is necessary but not sufficient: an
  // element can be visually gone and still be announced. This asserts absence
  // in the tree assistive technology actually reads.
  await page.goto("/digest");

  await expect(
    page.getByRole("heading", { name: "Synthesis" }),
    "an unauthenticated visitor must not find a synthesis heading in the accessibility tree",
  ).toHaveCount(0);
  await expect(
    page.locator("#synthesis-outcome .synthesis-insight"),
    "an unauthenticated visitor must not find synthesis prose",
  ).toHaveCount(0);
  await expect(
    page.locator("[data-citation-count]"),
    "an unauthenticated visitor must not find citation counts, which are an existence hint",
  ).toHaveCount(0);
});

test("the synthesis section reflows at 320px without horizontal scroll", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await login(page, "/digest");
  const state = await requireAnsweredState(page, "/digest");

  // Exclusivity must survive the narrow viewport; a layout that duplicates the
  // section to reflow would break the one-state guarantee.
  await expect(page.locator("#synthesis-outcome")).toHaveCount(1);

  const overflow = await page.evaluate(() => {
    const el = document.documentElement;
    return el.scrollWidth - el.clientWidth;
  });
  expect(
    overflow,
    `state '${state}' at 320px overflows the viewport by ${overflow}px, forcing horizontal scroll`,
  ).toBeLessThanOrEqual(1);
});

test("the synthesis section reflows at 200% zoom without horizontal scroll", async ({
  page,
}) => {
  // 200% zoom halves the CSS viewport. Emulating it by halving the width is
  // what the reflow requirement actually constrains: content must not demand
  // horizontal scrolling when a low-vision reader doubles the text size.
  await page.setViewportSize({ width: 640, height: 512 });
  await login(page, "/digest");
  const state = await requireAnsweredState(page, "/digest");

  await expect(page.locator("#synthesis-outcome")).toHaveCount(1);

  const overflow = await page.evaluate(() => {
    const el = document.documentElement;
    return el.scrollWidth - el.clientWidth;
  });
  expect(
    overflow,
    `state '${state}' at 200% zoom overflows by ${overflow}px, forcing horizontal scroll`,
  ).toBeLessThanOrEqual(1);
});

test("the retry control class meets the minimum target size", async ({ page }) => {
  await login(page, "/digest");
  await requireAnsweredState(page, "/digest");

  // The retry control renders only in a failure state, which a healthy stack
  // does not reach, so there is no live instance on this page to measure. The
  // matrix test asserts the button carries class "action"; this measures what
  // that class is actually worth in the stylesheet the page just shipped.
  const size = await page.evaluate(() => {
    const probe = document.createElement("button");
    probe.className = "action";
    probe.textContent = "Retry synthesis";
    document.body.appendChild(probe);
    const rect = probe.getBoundingClientRect();
    const measured = { width: rect.width, height: rect.height };
    probe.remove();
    return measured;
  });

  expect(
    Math.min(size.width, size.height),
    `a control with class "action" renders ${size.width}x${size.height}, below the 24px minimum target size`,
  ).toBeGreaterThanOrEqual(24);
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
    // A control a keyboard user cannot reach is not offered to them.
    await retryControl.focus();
    await expect(
      retryControl,
      "the retry control must be keyboard focusable",
    ).toBeFocused();
  } else {
    // Offering retry in a healthy state would imply something is wrong when it
    // is not, which is the same dishonesty pointing the other way.
    await expect(
      retryControl,
      `state '${state}' is not a failure yet a retry control is offered, implying a problem that does not exist`,
    ).toHaveCount(0);
  }
});

test("a retry reports a persisted outcome and the page agrees with it", async ({
  page,
}) => {
  await login(page, "/digest");

  const retry = await page.request.post("/api/synthesis/retry", {
    data: { cadence: "daily" },
  });
  expect(
    [200, 409],
    `retry must be accepted or report an existing run; got ${retry.status()}`,
  ).toContain(retry.status());
  const body = await retry.json();
  const outputId = body?.output?.outputId ?? "";
  expect(
    outputId,
    "a retry that reports success must name the output it persisted; confirmation without an identity is not truthful feedback",
  ).not.toBe("");

  // The page must agree with what the mutation claimed. Reporting success and
  // then rendering a state that denies it is the dishonesty this bug is about.
  const state = await requireAnsweredState(page, "/digest");
  expect(
    state,
    `retry reported persisting ${outputId} yet the page reports '${state}'`,
  ).not.toBe("never_run");
  expect(
    state,
    `retry reported persisting ${outputId} yet the page reports '${state}'`,
  ).not.toBe("failed_without_output");
});

test("reading run history never triggers a run and never masquerades as no history", async ({
  page,
}) => {
  await login(page, "/digest");

  // Make sure there is at least one run to read, so an empty history below
  // would be a real defect rather than a legitimately empty system.
  await page.request.post("/api/synthesis/retry", { data: { cadence: "daily" } });

  const readRuns = async (query: string) => {
    const res = await page.request.get(`/api/synthesis/runs?${query}`);
    expect(
      res.status(),
      `GET runs?${query} must succeed; got ${res.status()}`,
    ).toBe(200);
    const body = await res.json();
    expect(
      Array.isArray(body?.runs),
      `GET runs?${query} must return a runs array, never null`,
    ).toBe(true);
    return body.runs.length as number;
  };

  const before = await readRuns("limit=25");
  expect(
    before,
    "history reported zero runs immediately after a trigger; an empty list here would be reporting no history where history exists",
  ).toBeGreaterThan(0);

  // Reading is not writing. Repeated reads, including a narrowed one, must not
  // create runs -- a history view that quietly triggers work would inflate the
  // very record it claims to be reporting.
  await readRuns("limit=5");
  await readRuns("limit=25");
  const after = await readRuns("limit=25");

  expect(
    after,
    `reading history changed the run count from ${before} to ${after}; a read path is creating runs`,
  ).toBe(before);
});

test("a rerun of the same window adds no duplicate to Today or run history", async ({
  page,
}) => {
  await login(page, "/digest");

  const trigger = async () => {
    const res = await page.request.post("/api/synthesis/retry", {
      data: { cadence: "daily" },
    });
    expect(
      [200, 409],
      `retry must be accepted or report an existing run; got ${res.status()}`,
    ).toContain(res.status());
    const body = await res.json();
    return (body?.output?.outputId ?? "") as string;
  };

  const first = await trigger();
  expect(first, "the first trigger must name an output").not.toBe("");
  const second = await trigger();

  // Same window, same identity. A second row here would mean the idempotency
  // anchor is not holding and history would double-count real work.
  expect(
    second,
    `the same window produced ${first} then ${second}; a rerun minted a second identity`,
  ).toBe(first);

  const res = await page.request.get("/api/synthesis/runs?limit=50");
  expect(res.status()).toBe(200);
  const body = await res.json();
  const ids = (body.runs as Array<{ outputId: string }>).map((r) => r.outputId);
  const occurrences = ids.filter((id) => id === first).length;
  expect(
    occurrences,
    `output ${first} appears ${occurrences} times in run history; a rerun duplicated the record`,
  ).toBe(1);

  // Today must agree: one section, one state, no second rendering of the run.
  await requireAnsweredState(page, "/digest");
  await expect(page.locator("#synthesis-outcome")).toHaveCount(1);
});
