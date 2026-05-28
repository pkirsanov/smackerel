import { describe, expect, it } from "vitest";
import {
  compilePrivacyFilter,
  compilePrivacyFilterSync,
  PrivacyFilterCompileError,
} from "../../src/background/privacy_filter.js";
import { PRIVACY_PATTERN_CAP } from "../../src/common/validation.js";

describe("privacy filter", () => {
  it("SCN-058-010 dropsDeniedURLBeforeEnqueue: deny pattern matches and shouldDrop returns true", async () => {
    const pf = await compilePrivacyFilter([], ["^https://bank\\.example\\.com/"]);
    expect(pf.shouldDrop("https://bank.example.com/account")).toBe(true);
    expect(pf.shouldDrop("https://other.example.com/page")).toBe(false);
  });

  it("empty allow + empty deny passes everything through", async () => {
    const pf = await compilePrivacyFilter([], []);
    expect(pf.shouldDrop("https://anything.example/")).toBe(false);
  });

  it("non-empty allow restricts to matching URLs", async () => {
    const pf = await compilePrivacyFilter(["^https://allowed\\."], []);
    expect(pf.shouldDrop("https://allowed.example/page")).toBe(false);
    expect(pf.shouldDrop("https://blocked.example/page")).toBe(true);
  });

  it("deny precedes allow", async () => {
    const pf = await compilePrivacyFilter(
      ["^https://allowed\\."],
      ["^https://allowed\\.example/secret"],
    );
    expect(pf.shouldDrop("https://allowed.example/secret/page")).toBe(true);
    expect(pf.shouldDrop("https://allowed.example/public")).toBe(false);
  });

  it("SCN-058-010 (cap) rejectsPatternArrayOver64: compile throws on >64 patterns", () => {
    const overCap = Array.from(
      { length: PRIVACY_PATTERN_CAP + 1 },
      (_, i) => `^https://h${i}\\.example/`,
    );
    expect(() =>
      compilePrivacyFilterSync([], overCap, "pf-test"),
    ).toThrow(PrivacyFilterCompileError);
  });

  it("compile rejects invalid regex with field-tagged error", () => {
    expect(() =>
      compilePrivacyFilterSync([], ["[unterminated"], "pf-test"),
    ).toThrow(PrivacyFilterCompileError);
  });

  it("emits a deterministic privacy_filter_version fingerprint", async () => {
    const a = await compilePrivacyFilter(["^https://a"], ["^https://b"]);
    const b = await compilePrivacyFilter(["^https://a"], ["^https://b"]);
    expect(a.version).toBe(b.version);
    const c = await compilePrivacyFilter(["^https://a"], ["^https://c"]);
    expect(a.version).not.toBe(c.version);
  });
});
