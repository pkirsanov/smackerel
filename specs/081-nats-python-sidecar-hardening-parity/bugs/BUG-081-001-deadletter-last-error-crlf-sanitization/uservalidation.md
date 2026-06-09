# User Validation Checklist: [BUG-081-001]

## Checklist

- [x] Bug BUG-081-001 documented and triaged with verified diagnostic evidence (finding SEC-081-R1)
- [ ] Go core CR/LF-sanitizes `Smackerel-Last-Error` across all three subscribers (delivered + pipeline-verified 2026-06-08; awaiting your hands-on confirmation)
- [ ] Python sidecar CR/LF-sanitizes `Smackerel-Last-Error` with the same rule (delivered + pipeline-verified 2026-06-08; awaiting your hands-on confirmation)
- [ ] Adversarial regression on EACH runtime: a `\r\n`-laden error yields exactly six canonical headers, zero injected (delivered + pipeline-verified 2026-06-08; awaiting your hands-on confirmation)
- [ ] Byte-for-byte Go↔Python parity of the sanitized value preserved (delivered + pipeline-verified 2026-06-08; awaiting your hands-on confirmation)

The fix is delivered, independently re-verified GREEN on both runtimes
(Go `internal/pipeline` + `internal/stringutil`; Python `496 passed, 2 skipped`),
and audit-certified terminal (state.json `status: done`). Items 2–5 are left
unchecked by design: they record YOUR hands-on user acceptance, which the audit
agent does not self-tick on your behalf — tick each one once you have confirmed the
behavior. Severity is LOW / defense-in-depth and the reachable source is unproven.
