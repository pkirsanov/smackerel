import { describe, expect, it } from "vitest";
import {
  validateBaseURL,
  validateBearerToken,
  validateSourceDeviceID,
  validateDedupWindowSeconds,
  validateDwellThresholdSeconds,
  validatePatternList,
  validateOptions,
  ValidationError,
  PRIVACY_PATTERN_CAP,
} from "../../src/common/validation.js";

describe("validation", () => {
  it("base_url accepts https and rejects non-loopback http", () => {
    expect(validateBaseURL("https://h.example/")).toBe("https://h.example");
    expect(validateBaseURL("http://localhost:8080")).toBe("http://localhost:8080");
    expect(() => validateBaseURL("http://h.example/")).toThrow(ValidationError);
    expect(() => validateBaseURL("")).toThrow(ValidationError);
    expect(() => validateBaseURL("ftp://h/")).toThrow(ValidationError);
  });

  it("bearer_token required and trimmed", () => {
    expect(validateBearerToken("  tok ")).toBe("tok");
    expect(() => validateBearerToken("")).toThrow(ValidationError);
  });

  it("source_device_id enforces [a-z0-9-]{1,32}", () => {
    expect(validateSourceDeviceID("laptop-1")).toBe("laptop-1");
    expect(() => validateSourceDeviceID("Laptop")).toThrow(ValidationError);
    expect(() => validateSourceDeviceID("a".repeat(33))).toThrow(ValidationError);
    expect(() => validateSourceDeviceID("")).toThrow(ValidationError);
  });

  it("dedup window clamped to [60, 86400]", () => {
    expect(validateDedupWindowSeconds(1800)).toBe(1800);
    expect(validateDedupWindowSeconds("60")).toBe(60);
    expect(validateDedupWindowSeconds(86400)).toBe(86400);
    expect(() => validateDedupWindowSeconds(59)).toThrow(ValidationError);
    expect(() => validateDedupWindowSeconds(86401)).toThrow(ValidationError);
    expect(() => validateDedupWindowSeconds(1.5)).toThrow(ValidationError);
  });

  it("dwell threshold clamped to [0, 3600]", () => {
    expect(validateDwellThresholdSeconds(0)).toBe(0);
    expect(validateDwellThresholdSeconds(3600)).toBe(3600);
    expect(() => validateDwellThresholdSeconds(-1)).toThrow(ValidationError);
    expect(() => validateDwellThresholdSeconds(3601)).toThrow(ValidationError);
  });

  it("pattern list caps at 64 and rejects invalid regex", () => {
    const ok = Array.from({ length: PRIVACY_PATTERN_CAP }, (_, i) => `^h${i}$`);
    expect(validatePatternList("privacy_deny_patterns", ok)).toHaveLength(64);
    const over = ok.concat(["^extra$"]);
    expect(() =>
      validatePatternList("privacy_deny_patterns", over),
    ).toThrow(ValidationError);
    expect(() =>
      validatePatternList("privacy_deny_patterns", ["[unterminated"]),
    ).toThrow(ValidationError);
  });

  it("validateOptions composes all fields and trims the URL", () => {
    const got = validateOptions({
      base_url: "https://h.example/",
      bearer_token: "tok",
      source_device_id: "laptop",
      dedup_window_seconds: 1800,
      dwell_threshold_seconds: 120,
      privacy_allow_patterns: [],
      privacy_deny_patterns: ["^https://bank\\."],
    });
    expect(got.base_url).toBe("https://h.example");
    expect(got.privacy_deny_patterns).toEqual(["^https://bank\\."]);
  });
});
