# Design: BUG-009-002 — NormalizeURL chaos R30

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Root Cause Analysis

`internal/connector/bookmarks/dedup.go::NormalizeURL` relies on `net/url.URL.Parse` for canonicalisation. `url.Parse` is a lexical parser, not a semantic normalizer:

1. It does **not** strip ASCII control characters from any component — it round-trips whatever bytes the caller hands it, on the (correct) assumption that the caller already validated input.
2. It does **not** elide protocol-default ports — RFC 3986 treats `:443` and "no port" as syntactically different, even though they are semantically equivalent in HTTP.
3. It does **not** trim the trailing DNS-root dot — `Host` is preserved verbatim because the trailing dot is a valid DNS fully-qualified-name indicator.

The previous chaos rounds (R16/R24/C17) hardened `NormalizeURL` against userinfo leaks, `www.` variants, tracking params, and trailing slashes, but never probed control-byte input or default-port/trailing-dot host shapes. R30 closes that gap.

## Fix Approach

All three fixes live in `internal/connector/bookmarks/dedup.go::NormalizeURL` and in one tiny helper. No callers change; the function's signature is unchanged. Behaviour for valid, already-normalized URLs is unchanged.

### 1. Strip ASCII control characters before `url.Parse`

A new private helper `stripURLControlChars(s string) string` removes every byte where `c < 0x20 || c == 0x7F`. It uses a "scan first, allocate only if dirty" fast path so the happy-path is allocation-free (verified by `TestChaosR30_StripURLControlCharsFastPath`). `NormalizeURL` calls it as the first thing after the empty-string guard so every downstream step (parse, lowercase, port elision, trailing-dot strip) sees a control-byte-free string.

**Why strip vs. reject:** Production exports from real bookmark tools (Chrome corrupted profiles, partial-write Firefox JSON, third-party export tools) occasionally smuggle `\t`/`\r` into URL fields. Returning the URL as-is would still feed the bad string to `INSERT` and trip the PG insert failure described in F-CHAOS-R30-001. Stripping is the only behaviour that preserves capture coverage AND prevents the DB-rejection / log-injection / dedup-miss tail.

### 2. Elide protocol-default ports

After lowercasing the scheme and stripping userinfo, the code splits `u.Hostname()` from `u.Port()`, consults a small `defaultPorts := map[string]string{"http":"80","https":"443","ftp":"21"}`, and drops the port if it matches. Non-default ports (`:8080`, `:8443`, `:2121`) are preserved. The reassembly step adds IPv6 brackets back when the bare hostname contains a `:` so `[2001:db8::1]:443` collapses to `[2001:db8::1]` without losing the literal syntax.

**Why the explicit map vs. `url.JoinPort`:** Go's stdlib does not expose a "is this the default port for this scheme" predicate; the map is the smallest correct expression of the policy. The map only lists the three schemes the bookmarks connector actually ingests (browser exports = `http`/`https`; legacy bookmark managers occasionally have `ftp://`). Any future scheme falls through unchanged.

### 3. Strip trailing DNS-root dot from hostname

`strings.TrimRight(u.Hostname(), ".")` runs before the `www.` strip and before port reassembly. `TrimRight` collapses multiple trailing dots (`example.com...`) in one pass.

**Ordering:** trailing-dot strip → `www.` strip → port elision → bracket re-add. This order is important because the `www.` matcher needs the trailing-dot already gone (otherwise `www.example.com.` would not match the `strings.HasPrefix(h, "www.")` test if the stripping happened later in some refactor — the explicit ordering makes that contract obvious).

## Non-Goals

- **Unicode normalisation / IDN punycode.** The R30 probe did not produce findings here; existing chaos R24-001 covers IDN host casing through `strings.ToLower`. A future round can revisit if needed.
- **Userinfo re-validation.** Already covered by F-CHAOS-R24-002.
- **Path normalisation (`./` and `../`).** Not in scope — would require RFC 3986 path resolution which is a much bigger surface and not probed by R30.

## Risk Assessment

- **Backwards-compat risk:** LOW. Any pre-existing `source_ref` value already in the database that contains a control char, an explicit default port, or a trailing dot will no longer dedup against the new normalized form. Concretely: a future re-ingest of the same exporter file will produce a new row (one-time re-dedup, not a duplicate-explosion). Mitigation: documented in the BUG closure note; no migration written because there is no evidence existing rows fall into this shape (no production traffic has been ingested yet).
- **Performance risk:** NEGLIGIBLE. `stripURLControlChars` is `O(len(s))` with a single-pass scan and a fast-path early return; default-port elision is a 3-entry map lookup; trailing-dot strip is `O(trailing-dot-count)`. Total added cost per call is sub-microsecond.
- **Security upside:** Closes one log-injection vector and one INSERT-failure-induced silent-capture-loss vector.

## Test Strategy

Pure-unit, in-package, table-driven. No DB, no live stack. Each new test name carries an `F-CHAOS-R30-NNN` ID so the bug → test mapping is mechanical. Adversarial proof: each test asserts both the positive (control byte / default port / trailing dot is removed) AND a belt-and-braces check that no control byte survives in the result. The proof was demonstrated by temporarily reverting the `stripURLControlChars` call and re-running the R30 suite — 3 of the 6 new tests failed loudly.
