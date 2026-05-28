// Options-page validators per design §4.4. Exported as pure functions so the
// vitest suite can exercise every branch independently of the DOM.

export class ValidationError extends Error {
  readonly field: string;
  constructor(field: string, message: string) {
    super(`${field}: ${message}`);
    this.field = field;
    this.name = "ValidationError";
  }
}

export const PRIVACY_PATTERN_CAP = 64;
export const SOURCE_DEVICE_ID_PATTERN = /^[a-z0-9-]{1,32}$/;

export function validateBaseURL(input: unknown): string {
  if (typeof input !== "string" || input.length === 0) {
    throw new ValidationError("base_url", "required");
  }
  const trimmed = input.trim().replace(/\/+$/, "");
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    throw new ValidationError("base_url", "must be a valid URL");
  }
  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
    throw new ValidationError("base_url", "must be http(s)");
  }
  // http is permitted only for loopback (operator-local dev). Production
  // sideload flows MUST use https per design §5.3.
  if (parsed.protocol === "http:") {
    const host = parsed.hostname;
    const isLoopback =
      host === "localhost" || host === "127.0.0.1" || host === "::1";
    if (!isLoopback) {
      throw new ValidationError(
        "base_url",
        "http is permitted only for loopback hosts",
      );
    }
  }
  return trimmed;
}

export function validateBearerToken(input: unknown): string {
  if (typeof input !== "string" || input.length === 0) {
    throw new ValidationError("bearer_token", "required");
  }
  return input.trim();
}

export function validateSourceDeviceID(input: unknown): string {
  if (typeof input !== "string" || input.length === 0) {
    throw new ValidationError("source_device_id", "required");
  }
  const v = input.trim();
  if (!SOURCE_DEVICE_ID_PATTERN.test(v)) {
    throw new ValidationError(
      "source_device_id",
      "must be 1-32 chars from [a-z0-9-]",
    );
  }
  return v;
}

function validateInt(
  field: string,
  input: unknown,
  min: number,
  max: number,
): number {
  let n: number;
  if (typeof input === "number") {
    n = input;
  } else if (typeof input === "string" && input.trim() !== "") {
    n = Number(input);
  } else {
    throw new ValidationError(field, "required integer");
  }
  if (!Number.isInteger(n)) {
    throw new ValidationError(field, "must be an integer");
  }
  if (n < min || n > max) {
    throw new ValidationError(field, `must be in [${min}, ${max}]`);
  }
  return n;
}

export function validateDedupWindowSeconds(input: unknown): number {
  return validateInt("dedup_window_seconds", input, 60, 86400);
}

export function validateDwellThresholdSeconds(input: unknown): number {
  return validateInt("dwell_threshold_seconds", input, 0, 3600);
}

export function validatePatternList(field: string, input: unknown): string[] {
  if (!Array.isArray(input)) {
    throw new ValidationError(field, "must be an array of regex strings");
  }
  if (input.length > PRIVACY_PATTERN_CAP) {
    throw new ValidationError(
      field,
      `must contain at most ${PRIVACY_PATTERN_CAP} patterns`,
    );
  }
  const out: string[] = [];
  for (let i = 0; i < input.length; i++) {
    const p = input[i];
    if (typeof p !== "string" || p.length === 0) {
      throw new ValidationError(field, `entry ${i} must be a non-empty string`);
    }
    // Compile-test the regex so the operator gets immediate feedback rather
    // than a silent runtime failure inside the privacy filter.
    try {
      new RegExp(p);
    } catch (err) {
      throw new ValidationError(
        field,
        `entry ${i} is not a valid regex: ${(err as Error).message}`,
      );
    }
    out.push(p);
  }
  return out;
}

export interface OptionsState {
  base_url: string;
  bearer_token: string;
  source_device_id: string;
  dedup_window_seconds: number;
  dwell_threshold_seconds: number;
  privacy_allow_patterns: string[];
  privacy_deny_patterns: string[];
}

export function validateOptions(input: Record<string, unknown>): OptionsState {
  return {
    base_url: validateBaseURL(input.base_url),
    bearer_token: validateBearerToken(input.bearer_token),
    source_device_id: validateSourceDeviceID(input.source_device_id),
    dedup_window_seconds: validateDedupWindowSeconds(input.dedup_window_seconds),
    dwell_threshold_seconds: validateDwellThresholdSeconds(
      input.dwell_threshold_seconds,
    ),
    privacy_allow_patterns: validatePatternList(
      "privacy_allow_patterns",
      input.privacy_allow_patterns ?? [],
    ),
    privacy_deny_patterns: validatePatternList(
      "privacy_deny_patterns",
      input.privacy_deny_patterns ?? [],
    ),
  };
}
