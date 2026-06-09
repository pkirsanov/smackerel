# Bug Fix Design: [BUG-081-001] CR/LF sanitization of `Smackerel-Last-Error` (Go primary + Python parity mirror)

## Root Cause Analysis

### Investigation Summary
Finding `SEC-081-R1` (orchestrator-verified, OWASP A03) probed the dead-letter
poison path delivered/mirrored by spec 081. Verified evidence (captured in
report.md → Diagnostic Evidence):
- **Python sink:** `ml/app/nats_client.py:683` `last_err = _utf8_truncate(str(exc), 256)`
  then `:685` `headers["Smackerel-Last-Error"] = last_err`. The helper
  `_utf8_truncate` (`:176`) only slices on a UTF-8 byte boundary — it performs NO
  CR/LF or control-character stripping.
- **nats-py encoder (security-agent finding):** nats-py's header encoder applies
  `value.strip()` (leading/trailing whitespace only), so an INTERNAL `\r\n`
  survives and is emitted into the wire framing as an additional header line.
  nats-py 2.x performs no header-value CRLF validation.
- **Go parity (symmetric):** `internal/pipeline/subscriber.go:335`,
  `internal/pipeline/synthesis_subscriber.go:514`, and
  `internal/pipeline/domain_subscriber.go:245` each build `Smackerel-Last-Error`
  via `stringutil.TruncateUTF8(...)` (or raw `errStr`) then `headers.Set(...)` with
  NO CR/LF strip. `nats.Header.Set` does not guard CRLF either.
- **No sanitization anywhere:** a CR/LF-replace/strip grep across all four files
  returns exit 1 (absent).

### Root Cause
Error-string → header value WITHOUT CR/LF (and C0 control) sanitization, on BOTH
runtimes. Spec 081 faithfully MIRRORED the Go reference dead-letter builders,
which themselves never stripped CR/LF; the Python sidecar therefore inherited a
PRE-EXISTING, SHARED Go+Python gap. Spec 081 introduced no new divergence and is
not at fault — it correctly preserved (and thereby propagated) the existing gap.

### Impact Analysis
- Affected components: the dead-letter forensic path on smackerel-core (three Go
  subscribers) and smackerel-ml (`_handle_poison`).
- Affected data: none today. IF a future handler echoed raw `\r\n` artifact content
  into the error string, an injected header line (e.g. a forged `Nats-Msg-Id`
  collapsing distinct poison entries) could cause integrity/observability loss in
  the `deadletter.<subject>` audit trail.
- Affected users: none currently (reachable source unproven).
- Blast radius: one header-value build step per runtime; four files total.

## Fix Design

### Solution Approach
1. **Go primary surface (Scope 1):** Introduce a single shared sanitizer (e.g.
   `stringutil.SanitizeHeaderValue` alongside the existing `TruncateUTF8`) that
   strips/replaces CR (`\r`), LF (`\n`), and other C0 control characters, then apply
   it to the `Smackerel-Last-Error` value in all three subscribers
   (`subscriber.go`, `synthesis_subscriber.go`, `domain_subscriber.go`) — composed
   with the existing 256-byte UTF-8 truncation so both invariants hold.
2. **Python parity mirror (Scope 2):** Apply a byte-for-byte equivalent rule in
   `ml/app/nats_client.py` (e.g. extend or wrap `_utf8_truncate`, or a sibling
   `_sanitize_header_value`) so the Python `Smackerel-Last-Error` value is
   sanitized identically to the Go value for the same input.
3. **Adversarial regression on EACH side:** Feed a `\r\n`-laden error string
   through each runtime's dead-letter build path and assert the message has exactly
   the SIX canonical Smackerel-* header lines and zero injected line. Add a
   parity assertion that the Go and Python sanitized values for the same input are
   byte-for-byte equal.

### Canonical six dead-letter headers (the assertion target)
`Smackerel-Original-Subject`, `Smackerel-Original-Stream`, `Smackerel-Failed-At`,
`Smackerel-Delivery-Count`, `Smackerel-Last-Error`, `Smackerel-Original-Consumer`.
A correct fix yields these six (subject to the existing conditional-presence rules
for last-error and consumer) and NEVER a seventh, injected line.

### Why both runtimes MUST change together (cross-cutting note)
This is primarily a **Go-core hardening item**; the Python side is the **parity
mirror**. Spec 081's entire purpose is byte-for-byte Go↔Python parity of this
dead-letter envelope. A Python-only fix would make the Python output diverge from
the unsanitized Go output and BREAK that parity invariant. Therefore the
sanitization rule must land on both surfaces in the same delivery pass, with an
explicit byte-for-byte parity assertion. This is exactly why the change is NOT a
tiny inline fix and warrants a tracked packet + deliberate delivery pass.

### Alternative Approaches Considered
1. **Python-only sanitization** — REJECTED: breaks spec 081's byte-for-byte Go↔Python
   parity (the Go output would remain unsanitized), trading one defect for another.
2. **Reject/Nak messages whose error string contains CR/LF** — REJECTED: drops
   legitimate dead-letter forensics for a benign error-text quirk; sanitization
   preserves the audit entry while neutralizing the injection.
3. **Rely on nats-py / NATS server to validate header values** — REJECTED: nats-py
   2.x only `strip()`s leading/trailing whitespace and performs no CRLF validation;
   the Go path is equally unguarded. Defense must live in our builders.
4. **URL/percent-encode the whole value** — REJECTED: changes the human-readable
   forensic header content and would still need to be mirrored byte-for-byte; a
   minimal CR/LF/C0 strip is the least-surprising parity-preserving rule.

## Resolved Questions (answered in the 2026-06-08 delivery pass)
- **Q1 — strip vs replace → RESOLVED (replace with a single space):** Every C0
  control and DEL byte is replaced with one ASCII space (`0x20`), keeping the
  forensic text legible and length-preserving. Applied identically on both runtimes
  (`stringutil.SanitizeHeaderValue` / `_sanitize_header_value`).
- **Q2 — control-char scope → RESOLVED (all C0 + DEL):** The rule covers every byte
  `< 0x20` (incl. CR `0x0D`, LF `0x0A`, TAB `0x09`) and DEL `0x7F` — the broader
  defense-in-depth scope — applied identically on both sides.

## Dependency Note
Prod pins `nats-py==2.9.0` (`ml/requirements.txt:8`); the dev environment resolved
`nats-py==2.15.0`. The header-encoder behavior (`value.strip()`, no CRLF
validation) is identical across both versions, so the fix is version-independent
and requires no dependency change.

## Honest Caveat (defense-in-depth, unproven source)
The reachable SOURCE is NOT demonstrated: current ML poison handlers stringify
exceptions via `repr`/fixed strings that escape CR/LF, so no forged header can be
produced today. This packet is a **hardening item**, not an active-exploit
emergency. Severity is LOW. The value is closing a latent, symmetric gap before a
future handler can reach it.

## Testing Strategy
- unit (Go): table test feeding a `\r\n`-laden lastError through each subscriber's
  dead-letter build; assert exactly six header lines, zero injected; RED without
  the sanitizer, GREEN with it.
- unit (Python): pytest feeding a `\r\n`-laden `str(exc)` through `_handle_poison`'s
  header build; assert exactly six header lines, zero injected; RED→GREEN.
- parity (adversarial): assert the Go and Python sanitized values for the same
  CRLF/C0 input are byte-for-byte equal — fails if either side is missing the rule.
- regression: the adversarial cases ARE the regression — removing the sanitizer on
  either runtime reproduces the injection and fails the test RED.
