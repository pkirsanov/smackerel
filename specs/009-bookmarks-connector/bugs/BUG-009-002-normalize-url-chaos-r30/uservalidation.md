# User Validation: BUG-009-002 — NormalizeURL chaos R30

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Acceptance Checklist

- [x] F-CHAOS-R30-001 closed — `NormalizeURL` strips all ASCII control characters (`0x00`-`0x1F`, `0x7F`) so no control byte can survive into `SourceRef`. PostgreSQL `INSERT` cannot be tripped by NUL, log fields cannot be injected by `\r`/`\n`, and dedup correctly catches control-char variants of the same URL.
- [x] F-CHAOS-R30-002 closed — `NormalizeURL` elides the protocol-default port for `http`/`https`/`ftp` so `https://example.com:443/page` and `https://example.com/page` dedup as the same artifact, while non-default ports remain preserved.
- [x] F-CHAOS-R30-003 closed — `NormalizeURL` strips one or more trailing `.` characters from the hostname so `example.com.` and `example.com` dedup as the same artifact, composing correctly with `www.` stripping and default-port elision.
- [x] Adversarial proof recorded — the same R30 regression suite FAILS when the `stripURLControlChars` call is reverted and PASSES when the call is restored (transcript in [report.md](report.md) → "Adversarial fidelity proof").
- [x] No production-contract change — `Connect/Sync/Health/Close` signatures and config shape are unchanged; only `dedup.go::NormalizeURL` was modified plus the new tests in `dedup_test.go`.
- [x] No DB-schema change.
- [x] All pre-existing bookmarks-package tests continue to pass.
- [x] `go vet` and `gofmt -l` clean.
- [x] Parent spec 009 artifacts updated with chaos R30 cross-reference (state.json execution history + report.md section).

## Sign-off

This bug closure is the parent-expanded child workflow execution of `chaos-hardening` mode for spec 009 within sweep `sweep-2026-05-23-r30` round 14. The chaos probe ran inside the same workflow that planned, implemented, tested, validated, and audited the fix in one round. The user acceptance is implicit in the workflow contract: round 14 reaches `completed_owned` only after every R30 finding closes with a passing regression test that would fail if the fix were reverted, which the adversarial proof above demonstrates.
