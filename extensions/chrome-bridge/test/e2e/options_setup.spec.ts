// Spec 058 BUG-058-002 (BLOCKER-1) — options-page setup e2e.
//
// Covers the operator options-page setup flow: filling the form, saving,
// persistence across reloads, and the bearer-token reveal/mask toggle — all in
// a real headless Chromium against the real built extension UI.

import { test, expect } from "./fixtures";

test("operator can configure the extension through the options page and the values persist", async ({
  ext,
}) => {
  const page = await ext.openOptions();
  await expect(page).toHaveTitle(/Smackerel Chrome Bridge/);

  await page.fill("#base_url", "https://smk.example.org");
  await page.fill("#bearer_token", "operator-paseto-token");
  await page.fill("#source_device_id", "laptop-1");
  await page.fill("#dedup_window_seconds", "1800");
  await page.fill("#dwell_threshold_seconds", "120");
  await page.click("#save");

  // A successful save reports OK and writes chrome.storage.local.
  await expect(page.locator("#ok")).not.toHaveText("");
  await expect(page.locator("#errors")).toHaveText("");

  const stored = await page.evaluate(
    () =>
      new Promise<Record<string, unknown>>((resolve) => {
        chrome.storage.local.get(
          ["base_url", "bearer_token", "source_device_id", "dedup_window_seconds"],
          (items) => resolve(items),
        );
      }),
  );
  expect(stored.base_url).toBe("https://smk.example.org");
  expect(stored.bearer_token).toBe("operator-paseto-token");
  expect(stored.source_device_id).toBe("laptop-1");

  // Reopening the options page repopulates the saved values.
  const page2 = await ext.openOptions();
  await expect(page2.locator("#base_url")).toHaveValue("https://smk.example.org");
  await expect(page2.locator("#source_device_id")).toHaveValue("laptop-1");
});

test("the bearer-token field is masked by default and the Reveal button toggles visibility", async ({
  ext,
}) => {
  const page = await ext.openOptions();
  await page.fill("#bearer_token", "secret-token-value");

  // Masked by default (password input).
  await expect(page.locator("#bearer_token")).toHaveAttribute("type", "password");

  await page.click("#reveal_token");
  await expect(page.locator("#bearer_token")).toHaveAttribute("type", "text");

  // Toggling again re-masks.
  await page.click("#reveal_token");
  await expect(page.locator("#bearer_token")).toHaveAttribute("type", "password");
});

test("invalid configuration is rejected with a visible error and is not persisted", async ({
  ext,
}) => {
  const page = await ext.openOptions();
  await page.fill("#base_url", "not-a-url");
  await page.fill("#bearer_token", "t");
  await page.fill("#source_device_id", "BAD DEVICE ID!"); // violates [a-z0-9-]
  await page.fill("#dedup_window_seconds", "1800");
  await page.fill("#dwell_threshold_seconds", "120");
  await page.click("#save");

  await expect(page.locator("#errors")).not.toHaveText("");

  const stored = await page.evaluate(
    () =>
      new Promise<Record<string, unknown>>((resolve) => {
        chrome.storage.local.get(["source_device_id"], (items) => resolve(items));
      }),
  );
  expect(stored.source_device_id).toBeUndefined();
});
