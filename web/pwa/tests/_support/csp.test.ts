/**
 * Spec 077 SCOPE-1b / TP-077-01-03 — Node-level unit test driving the
 * fail-loud SST consumer (`requireSmackerelBaseUrl`) and the CSP guard
 * skeleton (`attachCSPGuard`). Invoked by the shell test
 * `tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh` which
 * runs as part of `./smackerel.sh test unit`.
 *
 * Runs under Node v22 with `--experimental-strip-types` so the .ts can
 * be executed directly without a TypeScript build step. No
 * `@playwright/test` dependency is imported here — the playwright config
 * itself is the only file in this scope that imports it, and the shell
 * driver covers the static composition check (config invokes the helper,
 * config carries no `??` / `||` / hardcoded default).
 *
 * Anchors scenario SCN-077-A10.
 */
import { strict as assert } from "node:assert";
import test from "node:test";

import { requireSmackerelBaseUrl } from "./env.ts";
import { attachCSPGuard } from "./csp.ts";

test("requireSmackerelBaseUrl throws fail-loud when env is unset", () => {
  const saved = process.env.SMACKEREL_BASE_URL;
  delete process.env.SMACKEREL_BASE_URL;
  try {
    assert.throws(
      () => requireSmackerelBaseUrl(),
      (err: unknown) => {
        assert.ok(err instanceof Error, "must throw an Error instance");
        assert.match(
          err.message,
          /SMACKEREL_BASE_URL/,
          "error message MUST name SMACKEREL_BASE_URL by hand",
        );
        return true;
      },
      "fail-loud throw is required when SMACKEREL_BASE_URL is unset",
    );
  } finally {
    if (saved !== undefined) process.env.SMACKEREL_BASE_URL = saved;
  }
});

test("requireSmackerelBaseUrl throws fail-loud when env is empty string", () => {
  const saved = process.env.SMACKEREL_BASE_URL;
  process.env.SMACKEREL_BASE_URL = "";
  try {
    assert.throws(
      () => requireSmackerelBaseUrl(),
      /SMACKEREL_BASE_URL/,
      "empty string MUST be rejected (no silent substitution)",
    );
  } finally {
    if (saved !== undefined) process.env.SMACKEREL_BASE_URL = saved;
    else delete process.env.SMACKEREL_BASE_URL;
  }
});

test("requireSmackerelBaseUrl returns the value when set", () => {
  const saved = process.env.SMACKEREL_BASE_URL;
  process.env.SMACKEREL_BASE_URL = "http://127.0.0.1:18080";
  try {
    assert.equal(requireSmackerelBaseUrl(), "http://127.0.0.1:18080");
  } finally {
    if (saved !== undefined) process.env.SMACKEREL_BASE_URL = saved;
    else delete process.env.SMACKEREL_BASE_URL;
  }
});

test("attachCSPGuard skeleton compiles and exposes the SCOPE-3 contract", () => {
  assert.equal(typeof attachCSPGuard, "function");
  assert.equal(
    attachCSPGuard.length,
    1,
    "must accept exactly one parameter (the Playwright Page)",
  );
  // Skeleton is a no-op in SCOPE-1b; real assertions land in SCOPE-3.
  assert.doesNotThrow(() => attachCSPGuard({} as unknown));
});
