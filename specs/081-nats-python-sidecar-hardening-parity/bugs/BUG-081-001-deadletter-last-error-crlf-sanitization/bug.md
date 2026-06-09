# Bug: [BUG-081-001] Dead-letter `Smackerel-Last-Error` header built without CR/LF sanitization (Go+Python, defense-in-depth)

## Summary
The dead-letter poison handler on BOTH runtimes builds the `Smackerel-Last-Error`
NATS header from a raw exception/error string with NO CR/LF (or C0 control)
sanitization. An internal `\r\n` inside the error string would survive the
header encoder and inject an additional header line into the
`deadletter.<subject>` message — a latent header-injection / forensic-evidence
integrity gap (OWASP A03). Finding `SEC-081-R1`, orchestrator-verified.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [ ] Medium - Feature broken, workaround exists
- [x] Low - Minor issue, defense-in-depth / latent (reachable source unproven)

**Severity rationale (defense-in-depth, NOT an active exploit):** The risk is
LATENT. The reachable SOURCE is NOT demonstrated — current ML poison handlers
stringify exceptions via `repr`/fixed strings that escape CR/LF, so a forged
header line cannot be produced today. The gap becomes exploitable only if a
FUTURE handler's exception text echoes raw artifact content containing literal
`\r\n`. There is no current data loss, no credential exposure, and no privilege
boundary crossed. This is a hardening item, not an emergency.

## Status
- [ ] Reported
- [x] Confirmed (root cause statically verified at file:line on both runtimes)
- [x] In Progress (fix delivered + validated + audited)
- [x] Fixed (audit-certified SHIP_IT 2026-06-08; sanitizer + all 4 sinks + byte-for-byte parity independently re-verified GREEN)
- [x] Verified (bubbles.audit independently re-ran Go `internal/pipeline`+`internal/stringutil` GREEN and Python `496 passed, 2 skipped`)
- [ ] Closed

**Triage state:** Fixed + audit-certified (state.json `status: done`,
`certification.status: done`). The cross-cutting, parity-preserving fix — spanning the Go
primary surface (three subscribers) and the Python parity mirror — was implemented
in ONE delivery pass so the byte-for-byte Go↔Python parity that is spec 081's entire
purpose is preserved (a one-sided patch would have broken it). The post-validate
`bubbles.audit` phase (separation of duties) independently re-ran both runtimes,
spot-checked all four sinks + the parity pin, judged the in-kind E2E disposition
honest (no live e2e fabricated, Gate G021), and certified `SHIP_IT` — advancing
`Fixed`/`Verified` above. `Closed` is left for the orchestrator's final close (after
the parent spec 081 `SEC-081-R1` concern is updated and the change is merged).

**Delivery + validation + audit (2026-06-08):** The fix was delivered (Go primary + Python
parity mirror in one pass), proven RED→GREEN on both runtimes with a byte-for-byte
parity pin (input `boom\r\nNats-Msg-Id: forged` → output `boom  Nats-Msg-Id: forged`),
independently RE-VERIFIED at the unit+parity level by `bubbles.validate`, and then
independently RE-AUDITED + certified terminal by `bubbles.audit` — see report.md →
"DELIVERY PASS", "VALIDATE RE-VERIFICATION", "CLOSE-OUT AUTHORING PASS", and
"FINAL AUDIT (bubbles.audit)". The bug's regression contract is unit + parity (per
`scenario-manifest.json` and design.md §Testing Strategy); the gated live-stack
integration parity test is covered-in-kind by the executed unit parity pin. No
live-E2E pass was fabricated (Gate G021); parent spec 081 stays untouched.

## Reproduction Steps
Static (root-cause) reproduction — what is verified now:
1. Confirm the Python sink builds the header from a raw, unsanitized string:
   `grep -n "Smackerel-Last-Error\|_utf8_truncate\|str(exc)" ml/app/nats_client.py`
   → `683: last_err = _utf8_truncate(str(exc), 256)` then
   `685: headers["Smackerel-Last-Error"] = last_err` (no CR/LF strip).
2. Confirm `_utf8_truncate` (`ml/app/nats_client.py:176`) only byte-truncates
   UTF-8 — it performs NO CR/LF or control-character stripping.
3. Confirm the Go parity is symmetric:
   `grep -n "Smackerel-Last-Error" internal/pipeline/subscriber.go internal/pipeline/synthesis_subscriber.go internal/pipeline/domain_subscriber.go`
   → `subscriber.go:335`, `synthesis_subscriber.go:514`, `domain_subscriber.go:245`,
   each `headers.Set("Smackerel-Last-Error", ...)` after `TruncateUTF8` with no strip.
4. Confirm neither runtime sanitizes CR/LF in the dead-letter header build path
   (a sanitization grep returns exit 1 — absent on all four files).

Dynamic (latent-exploit) reproduction — exercised at the unit seam in the delivery pass:
5. Construct an error string containing `"...\r\nNats-Msg-Id: forged"` and route it
   through each runtime's dead-letter header builder (`_handle_poison` /
   `publishToDeadLetter`) at the unit seam.
6. Assert the resulting `deadletter.<subject>` headers contain EXACTLY the six
   canonical Smackerel-* lines and zero injected line. Proven RED before the fix and
   GREEN after on both runtimes (report.md → DELIVERY PASS); the byte-for-byte
   parity pin makes the Go and Python sanitized values identical.

## Expected Behavior
The `Smackerel-Last-Error` header value MUST be neutralized before it is set:
CR (`\r`), LF (`\n`), and ideally other C0 control characters MUST be stripped or
replaced so the dead-letter message always contains EXACTLY the six canonical
Smackerel-* header lines and ZERO injected line — identically on Go and Python.

## Actual Behavior
On both runtimes the raw (only UTF-8-truncated) error string is set directly as
the header value. nats-py's header encoder applies `value.strip()` (leading/
trailing whitespace only, per the security-agent finding), so an INTERNAL `\r\n`
survives and the NATS wire framing interprets it as a new header line. nats-py
2.x performs no header-value CRLF validation; the Go `nats.Header.Set` path is
equally unguarded.

## Environment
- Services: smackerel-core (Go) and smackerel-ml (Python sidecar)
- Version: HEAD of `main` (spec 081 certified `done` 2026-06-06)
- Sink files: `ml/app/nats_client.py:683,685`; `internal/pipeline/subscriber.go:335`;
  `internal/pipeline/synthesis_subscriber.go:514`; `internal/pipeline/domain_subscriber.go:245`
- Dependency: prod pins `nats-py==2.9.0` (`ml/requirements.txt:8`); dev resolved
  `2.15.0` — header-encoder (`value.strip()`) behavior identical across both.

## Error Output
```
# Latent — no forged header is produced today. Illustrative shape of the
# injection IF a future handler echoed raw "\r\nNats-Msg-Id: forged" into exc:
Smackerel-Original-Subject: <subject>
Smackerel-Last-Error: <leading text>
Nats-Msg-Id: forged          <-- INJECTED 7th line (collapses distinct poison entries)
```
(Illustrative only. The reachable source is unproven; this packet records a
defense-in-depth hardening item, NOT an executed exploit — see report.md.)

## Root Cause
Error-string → header value WITHOUT CR/LF (and C0 control) sanitization, on BOTH
runtimes. The original spec 081 parity work faithfully MIRRORED the Go reference
(`publishToDeadLetter` / `publishSynthesisToDeadLetter` / domain subscriber),
which itself never stripped CR/LF. So the Python sidecar inherited a PRE-EXISTING,
SHARED Go+Python gap; spec 081 introduced NO new divergence and is not at fault.

## Related
- Parent feature: `specs/081-nats-python-sidecar-hardening-parity/` (status `done`, UNCHANGED by this bug)
- Finding: `SEC-081-R1` (OWASP A03 — Injection), orchestrator-verified, LOW / defense-in-depth
- Go primary surfaces: `internal/pipeline/subscriber.go:335`, `internal/pipeline/synthesis_subscriber.go:514`, `internal/pipeline/domain_subscriber.go:245`
- Python parity mirror: `ml/app/nats_client.py:683,685` (helper `_utf8_truncate` at `:176`)
- Cross-cutting delivery precedent: `BUG-056-002` (PKCE user-context auth), `BUG-059-001` (gkeepapi sidecar pin)

## Cross-Cutting Delivery Rationale
This is a LOW-severity, defense-in-depth hardening item whose reachable source is
unproven. The proportionate fix is cross-cutting: CR/LF sanitization had to land on
the Go primary surface (three subscribers) AND the Python parity mirror in the
SAME pass, because byte-for-byte Go↔Python parity is spec 081's core invariant —
a one-sided patch would itself break that invariant. Both runtimes were therefore
updated together in one deliberate pass with adversarial regressions on each side
(RED→GREEN) and a byte-for-byte parity pin. Priority: low; it was non-blocking for
current operation, and the fix is now delivered + validated.
