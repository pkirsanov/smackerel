# Spec: [BUG-081-001] CR/LF-sanitized `Smackerel-Last-Error` dead-letter header (Go+Python parity)

## Problem Statement
Spec 081 established byte-for-byte Go↔Python parity for the NATS JetStream
dead-letter path. Both runtimes build the `Smackerel-Last-Error` audit header from
a raw error string that is only UTF-8-truncated — never CR/LF-sanitized. A future
error string containing internal `\r\n` could inject an extra header line into the
`deadletter.<subject>` forensic message (OWASP A03). The gap is latent
(defense-in-depth) but symmetric across both runtimes, so it must be closed on
both at once to preserve spec 081's parity invariant.

## Outcome Contract
**Intent:** The `Smackerel-Last-Error` header value MUST be neutralized (CR, LF,
and C0 control characters stripped or replaced) before it is set, IDENTICALLY on
the Go core (three subscribers) and the Python sidecar, so a dead-letter message
always carries exactly the six canonical Smackerel-* header lines and zero
injected line.
**Success Signal:** A CRLF-laden error string fed through the dead-letter build
path on EACH runtime yields exactly six canonical header lines and no injected
header; the Go and Python outputs remain byte-for-byte identical.
**Hard Constraints:** Byte-for-byte Go↔Python parity (spec 081's core invariant)
MUST be preserved — the sanitization rule MUST be the same on both sides; the
existing 256-byte UTF-8 truncation invariant MUST be kept; no new dependency; no
change to the six canonical header names or their conditional-presence rules.
**Failure Condition:** Any dead-letter message where an internal `\r\n` in the
error string produces a 7th header line, OR a fix applied to only one runtime
(which itself breaks parity).

## Goals
- Strip/replace CR+LF (and C0 control chars) from `Smackerel-Last-Error` on the Go
  primary surface AND the Python parity mirror, with an identical rule.
- Add an adversarial regression on EACH runtime proving a CRLF-laden error string
  yields exactly the six canonical header lines and zero injected line.
- Preserve the byte-for-byte Go↔Python parity that spec 081 exists to guarantee.

## Non-Goals
- Changing the dead-letter subject, stream routing, or the six canonical header
  names / conditional-presence rules.
- Altering the 256-byte UTF-8 truncation contract.
- Adding or bumping any dependency (the nats-py pin is unchanged).
- Proving an active end-to-end exploit (the reachable source is unproven; this is
  a defense-in-depth hardening item).

## Requirements
- R1: `Smackerel-Last-Error` is CR/LF-and-C0-sanitized before `headers.Set(...)` in
  `internal/pipeline/subscriber.go`, `internal/pipeline/synthesis_subscriber.go`,
  and `internal/pipeline/domain_subscriber.go`.
- R2: `Smackerel-Last-Error` is CR/LF-and-C0-sanitized before assignment in
  `ml/app/nats_client.py` using a rule byte-for-byte equivalent to R1.
- R3: The sanitization runs AFTER (or composed with) the existing 256-byte UTF-8
  truncation so both invariants hold together.
- R4: Adversarial regression on each runtime: a `\r\n`-laden error string yields
  exactly the six canonical Smackerel-* header lines and zero injected line.
- R5: Go and Python sanitized outputs for the same input remain byte-for-byte equal.

## User Scenarios (Gherkin)
```gherkin
Scenario: CRLF-laden error string injects no extra header (Go core)
  Given a Go subscriber exhausts a message with lastError "boom\r\nNats-Msg-Id: forged"
  When publishToDeadLetter builds the dead-letter headers
  Then the message has exactly the six canonical Smackerel-* header lines
  And no Nats-Msg-Id or other injected header line is present

Scenario: CRLF-laden error string injects no extra header (Python sidecar)
  Given the ML poison handler exhausts a message with str(exc) "boom\r\nNats-Msg-Id: forged"
  When _handle_poison builds the dead-letter headers
  Then the message has exactly the six canonical Smackerel-* header lines
  And no Nats-Msg-Id or other injected header line is present

Scenario: Go and Python sanitization stay byte-for-byte identical
  Given the same CRLF-and-control-laden error string on both runtimes
  When each builds its Smackerel-Last-Error header value
  Then the two sanitized values are byte-for-byte equal (parity preserved)
```

## Acceptance Criteria
- AC-1 (R1): the three Go subscribers sanitize CR/LF/C0 before `headers.Set("Smackerel-Last-Error", ...)`.
- AC-2 (R2): `ml/app/nats_client.py` sanitizes CR/LF/C0 before setting the header, with the same rule.
- AC-3 (R4, Go): a Go regression test with a `\r\n`-laden lastError asserts exactly six header lines, zero injected.
- AC-4 (R4, Python): a Python regression test with a `\r\n`-laden `str(exc)` asserts exactly six header lines, zero injected.
- AC-5 (R5/adversarial): the Go and Python sanitized values for the same input are byte-for-byte equal; removing the sanitization on either side makes its regression fail RED.
