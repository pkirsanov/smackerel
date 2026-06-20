// Spec 058 — MV3 manifest contract (unit tier).
//
// Why this file exists: the chrome-bridge `manifest.json` security contract —
// MV3 version, the minimum-permissions set (design §4), the Round-R14
// least-privilege `host_permissions` hardening (spec 058 Finding B), and the
// restrictive CSP — was, before this file, asserted ONLY inside the Playwright
// e2e spec `test/e2e/sideload_smoke.spec.ts`. That lane needs a real built
// extension + real headless Chromium (`./smackerel.sh test e2e-ext`) and does
// NOT run in the fast `vitest` unit feedback loop. So a regression that
// silently re-widened `host_permissions` back to `<all_urls>` (or to cleartext
// `http://*/*`), dropped a capture permission, or relaxed the CSP would slip
// past the unit suite and only fail behind the heavy browser gate.
//
// `manifest.json` is a static JSON file, so the contract is unit-checkable with
// a pure read+assert — the appropriate tier for it (mirroring the repo's
// established static-manifest contract tests, e.g. the web/extension parity
// contract in `internal/web/extension_parity_contract_test.go`). This file
// lifts the static-JSON-checkable subset of the e2e manifest assertions into
// the unit tier for fast-feedback regression protection, with adversarial twins
// that prove the guard is non-tautological.

import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const MANIFEST_PATH = resolve(here, "../../manifest.json");

// MV3 minimum-permissions set (design §4): the bookmarks + history capture
// APIs plus storage (IndexedDB WAL / options) and alarms (SW wake). Exact set —
// no broader grant (e.g. tabs, cookies, <all_urls>) is permitted.
const EXPECTED_PERMISSIONS = ["alarms", "bookmarks", "history", "storage"];

// Least-privilege host access established in Round R14 (spec 058 Finding B).
// `host_permissions` MUST be exactly the operator-base-URL set the CSP
// `connect-src` governs (the single ingest POST) — never the broad
// `<all_urls>` grant and never the cleartext `http://*/*` wildcard.
const EXPECTED_HOST_PERMISSIONS = ["https://*/*", "http://localhost/*", "http://127.0.0.1/*"];
const FORBIDDEN_HOST_PERMISSIONS = ["<all_urls>", "http://*/*"];

interface ChromeManifest {
  manifest_version?: number;
  permissions?: string[];
  host_permissions?: string[];
  content_security_policy?: { extension_pages?: string };
}

function sortedEqual(a: string[], b: string[]): boolean {
  return JSON.stringify([...a].sort()) === JSON.stringify([...b].sort());
}

// validateManifestContract returns the list of contract violations for a
// manifest object (empty array = compliant). It is a pure function so the
// live-file test and the adversarial twins below share exactly one validator —
// the same property the e2e sideload smoke asserts against a loaded extension.
export function validateManifestContract(m: ChromeManifest): string[] {
  const violations: string[] = [];

  if (m.manifest_version !== 3) {
    violations.push(`manifest_version must be 3 (MV3), got ${String(m.manifest_version)}`);
  }

  const perms = m.permissions ?? [];
  if (!sortedEqual(perms, EXPECTED_PERMISSIONS)) {
    violations.push(
      `permissions must be exactly the MV3 minimum set ${JSON.stringify(EXPECTED_PERMISSIONS)}, got ${JSON.stringify(perms)}`,
    );
  }

  const hostPerms = m.host_permissions ?? [];
  for (const forbidden of FORBIDDEN_HOST_PERMISSIONS) {
    if (hostPerms.includes(forbidden)) {
      violations.push(
        `host_permissions MUST NOT include the over-broad grant ${JSON.stringify(forbidden)} (R14 least-privilege)`,
      );
    }
  }
  if (!sortedEqual(hostPerms, EXPECTED_HOST_PERMISSIONS)) {
    violations.push(
      `host_permissions must be exactly the least-privilege set ${JSON.stringify(EXPECTED_HOST_PERMISSIONS)}, got ${JSON.stringify(hostPerms)}`,
    );
  }

  const csp = m.content_security_policy?.extension_pages ?? "";
  if (!csp.includes("script-src 'self'")) {
    violations.push("CSP extension_pages must pin script-src 'self'");
  }
  if (!csp.includes("object-src 'self'")) {
    violations.push("CSP extension_pages must pin object-src 'self'");
  }
  if (csp.includes("'unsafe-eval'")) {
    violations.push("CSP extension_pages must NOT allow 'unsafe-eval'");
  }

  return violations;
}

describe("manifest contract (spec 058 — MV3 minimum permissions + R14 least-privilege host access)", () => {
  const manifest = JSON.parse(readFileSync(MANIFEST_PATH, "utf8")) as ChromeManifest;

  it("the committed manifest.json satisfies the MV3 + least-privilege contract", () => {
    expect(validateManifestContract(manifest)).toEqual([]);
  });

  it("pins the exact R14 least-privilege host_permissions set on the committed manifest", () => {
    // Direct assertion on the real file (not only via the validator) so the
    // intended grant is documented and locked at the unit tier.
    expect((manifest.host_permissions ?? []).slice().sort()).toEqual(
      [...EXPECTED_HOST_PERMISSIONS].sort(),
    );
    expect(manifest.host_permissions).not.toContain("<all_urls>");
    expect(manifest.host_permissions).not.toContain("http://*/*");
  });

  it("adversarial: forbids re-widening host_permissions back to <all_urls>", () => {
    const regressed: ChromeManifest = { ...manifest, host_permissions: ["<all_urls>"] };
    const v = validateManifestContract(regressed);
    expect(v.length).toBeGreaterThan(0);
    expect(v.join("\n")).toContain("<all_urls>");
  });

  it("adversarial: forbids re-widening host_permissions to cleartext http://*/*", () => {
    const regressed: ChromeManifest = {
      ...manifest,
      host_permissions: ["https://*/*", "http://*/*"],
    };
    expect(validateManifestContract(regressed).join("\n")).toContain("http://*/*");
  });

  it("adversarial: rejects an over-broad permission beyond the MV3 minimum set (e.g. tabs)", () => {
    const regressed: ChromeManifest = {
      ...manifest,
      permissions: [...EXPECTED_PERMISSIONS, "tabs"],
    };
    expect(validateManifestContract(regressed).join("\n")).toContain("minimum set");
  });

  it("adversarial: rejects a dropped capture permission (e.g. history removed)", () => {
    const regressed: ChromeManifest = {
      ...manifest,
      permissions: ["alarms", "bookmarks", "storage"],
    };
    expect(validateManifestContract(regressed).join("\n")).toContain("minimum set");
  });

  it("adversarial: rejects a CSP that allows 'unsafe-eval'", () => {
    const regressed: ChromeManifest = {
      ...manifest,
      content_security_policy: {
        extension_pages: "script-src 'self' 'unsafe-eval'; object-src 'self'",
      },
    };
    expect(validateManifestContract(regressed).join("\n")).toContain("unsafe-eval");
  });
});
