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

test("attachCSPGuard exposes the SCOPE-3 contract and wires page listeners", () => {
  assert.equal(typeof attachCSPGuard, "function");
  assert.equal(
    attachCSPGuard.length,
    1,
    "must accept exactly one parameter (the Playwright Page)",
  );
  // SCOPE-3 wires real listeners. Drive a stub page that records the
  // `on(...)` / `exposeBinding(...)` / `addInitScript(...)` calls so
  // this Node-level test can keep running without `@playwright/test`
  // installed.
  const calls: { kind: string; arg: unknown }[] = [];
  const stub = {
    on(event: string, _fn: unknown) {
      calls.push({ kind: `on:${event}`, arg: null });
    },
    exposeBinding(name: string, _fn: unknown) {
      calls.push({ kind: `exposeBinding:${name}`, arg: null });
      return Promise.resolve();
    },
    addInitScript(_fn: unknown) {
      calls.push({ kind: "addInitScript", arg: null });
      return Promise.resolve();
    },
  };
  assert.doesNotThrow(() => attachCSPGuard(stub as never));
  const kinds = calls.map((c) => c.kind).sort();
  assert.deepEqual(
    kinds,
    [
      "addInitScript",
      "exposeBinding:__spec077ReportCSPViolation",
      "on:console",
      "on:pageerror",
    ],
    "SCOPE-3 guard must wire console + pageerror listeners and expose the CSP-violation binding + init script",
  );
});

// ============================================================================
// BUG-001 Chaos Regression Tests
// ============================================================================

test("BUG-001: attachCSPGuard warns on unexpected exposeBinding error (chaos regression)", async () => {
  // TP-077-BUG-001-01 / SCN-077-BUG-001-01
  // Stub that rejects with an unexpected error (NOT "already exposed")
  const warnCalls: string[] = [];
  const originalWarn = console.warn;
  console.warn = (...args: unknown[]) =>
    warnCalls.push(args.map(String).join(" "));

  const failingPage = {
    on() {},
    exposeBinding() {
      return Promise.reject(new Error("Target page closed"));
    },
    addInitScript() {
      return Promise.reject(new Error("Execution context was destroyed"));
    },
  };

  attachCSPGuard(failingPage as never);

  // Must wait for promises to settle
  await new Promise((r) => setTimeout(r, 50));

  console.warn = originalWarn;

  assert.ok(
    warnCalls.some((w) => w.includes("[csp.ts] exposeBinding failed")),
    `Expected warning about exposeBinding failure; got: ${JSON.stringify(warnCalls)}`,
  );
  assert.ok(
    warnCalls.some((w) => w.includes("[csp.ts] addInitScript failed")),
    `Expected warning about addInitScript failure; got: ${JSON.stringify(warnCalls)}`,
  );
});

test("BUG-001: attachCSPGuard silently handles already-bound error (expected case)", async () => {
  // TP-077-BUG-001-02 / SCN-077-BUG-001-02
  const warnCalls: string[] = [];
  const originalWarn = console.warn;
  console.warn = (...args: unknown[]) =>
    warnCalls.push(args.map(String).join(" "));

  const alreadyBoundPage = {
    on() {},
    exposeBinding() {
      // Playwright's actual error message for duplicate binding
      return Promise.reject(new Error("Function already exposed"));
    },
    addInitScript() {
      return Promise.resolve();
    },
  };

  attachCSPGuard(alreadyBoundPage as never);
  await new Promise((r) => setTimeout(r, 50));

  console.warn = originalWarn;

  const cspWarnings = warnCalls.filter((w) => w.includes("[csp.ts]"));
  assert.equal(
    cspWarnings.length,
    0,
    `Should NOT warn for expected already-bound error; got: ${JSON.stringify(cspWarnings)}`,
  );
});
