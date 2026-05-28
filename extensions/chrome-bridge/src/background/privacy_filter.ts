// Compiled allow/deny URL matcher per design §4.2 step 2 and §4.4. The deny
// list is enforced BEFORE enqueue so denied URLs never leave the browser
// (SCN-058-010, design §5.3 privacy guarantee). The allow list, when non-empty,
// further restricts to URLs that match at least one allow pattern; an empty
// allow list means "no allow-list restriction".
//
// The pattern cap (64 each, enforced by validation.ts at options-save time)
// is also re-enforced here as a defense in depth in case a stale chrome.storage
// row pre-dates the cap.

import { PRIVACY_PATTERN_CAP } from "../common/validation.js";

export interface CompiledPrivacyFilter {
  readonly version: string;
  shouldDrop(url: string): boolean;
}

class PrivacyFilter implements CompiledPrivacyFilter {
  readonly version: string;
  private readonly allow: RegExp[];
  private readonly deny: RegExp[];
  constructor(allow: RegExp[], deny: RegExp[], version: string) {
    this.allow = allow;
    this.deny = deny;
    this.version = version;
  }
  shouldDrop(url: string): boolean {
    for (const r of this.deny) {
      if (r.test(url)) return true;
    }
    if (this.allow.length === 0) return false;
    for (const r of this.allow) {
      if (r.test(url)) return false;
    }
    return true;
  }
}

export class PrivacyFilterCompileError extends Error {}

function compileList(patterns: string[], label: string): RegExp[] {
  if (patterns.length > PRIVACY_PATTERN_CAP) {
    throw new PrivacyFilterCompileError(
      `${label} exceeds cap of ${PRIVACY_PATTERN_CAP} patterns (got ${patterns.length})`,
    );
  }
  const out: RegExp[] = [];
  for (let i = 0; i < patterns.length; i++) {
    try {
      out.push(new RegExp(patterns[i]));
    } catch (err) {
      throw new PrivacyFilterCompileError(
        `${label}[${i}] invalid regex: ${(err as Error).message}`,
      );
    }
  }
  return out;
}

// Deterministic fingerprint of the operator's pattern set; stamped into every
// outgoing artifact's metadata.privacy_filter_version (design §2.2).
async function fingerprint(allow: string[], deny: string[]): Promise<string> {
  const enc = new TextEncoder();
  const payload = enc.encode(JSON.stringify({ allow, deny }));
  const digest = await crypto.subtle.digest("SHA-256", payload);
  const hex = Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  return `pf-${hex.slice(0, 16)}`;
}

export async function compilePrivacyFilter(
  allowPatterns: string[],
  denyPatterns: string[],
): Promise<CompiledPrivacyFilter> {
  const allow = compileList(allowPatterns, "privacy_allow_patterns");
  const deny = compileList(denyPatterns, "privacy_deny_patterns");
  const version = await fingerprint(allowPatterns, denyPatterns);
  return new PrivacyFilter(allow, deny, version);
}

// Synchronous variant used by hot listener paths. Callers must pre-compute the
// version via fingerprint() or compilePrivacyFilter().
export function compilePrivacyFilterSync(
  allowPatterns: string[],
  denyPatterns: string[],
  version: string,
): CompiledPrivacyFilter {
  const allow = compileList(allowPatterns, "privacy_allow_patterns");
  const deny = compileList(denyPatterns, "privacy_deny_patterns");
  return new PrivacyFilter(allow, deny, version);
}
