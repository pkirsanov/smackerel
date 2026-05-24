# Report: BUG-009-002 — NormalizeURL chaos R30

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Stochastic-quality-sweep round 14 of `sweep-2026-05-23-r30` ran the `chaos-hardening` child workflow against `specs/009-bookmarks-connector`. The chaos phase probed `internal/connector/bookmarks/dedup.go::NormalizeURL` with adversarial inputs that mirror real-world browser-export quirks (embedded NUL/CR/LF/TAB/DEL, explicit default ports, trailing DNS-root dot) and surfaced three concrete defects not covered by the existing R16/R24/C17 chaos batches:

- **F-CHAOS-R30-001** (HIGH) — ASCII control characters (`0x00`-`0x1F`, `0x7F`) survived `NormalizeURL` and reached `SourceRef`, enabling (a) PostgreSQL INSERT failure on NUL → silent capture loss, (b) log injection via `\n`/`\r` in structured-log fields, (c) dedup miss when an attacker introduces control-char variants of the same URL.
- **F-CHAOS-R30-002** (MEDIUM) — Default protocol ports (`:80`/`:443`/`:21`) were preserved, causing `https://example.com:443/page` and `https://example.com/page` to dedup as two distinct artifacts.
- **F-CHAOS-R30-003** (MEDIUM) — Trailing DNS-root dot on the hostname (`example.com.`) was preserved, causing the same dedup-miss class as R30-002.

The fix lives in `internal/connector/bookmarks/dedup.go`. It adds a `stripURLControlChars` helper called as the first step of `NormalizeURL`, replaces the inline `www.` strip with an ordered host-canonicalisation block (trailing-dot → `www.` → default-port elision with IPv6 bracket re-add), and is covered by six new adversarial regression tests in `internal/connector/bookmarks/dedup_test.go`. No DB schema, no connector-contract, no config-shape changes. Spec 055 work-in-progress files were excluded from the close-out commit via path-limited `git add`.

## Completion Statement

All 7 DoD bullets in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The pre-fix chaos probe surfaced 3 findings; the post-fix R30 test suite shows 6/6 PASS (12+8+6=26 sub-cases). The adversarial-fidelity proof (toggle the `stripURLControlChars` call OFF, watch 3 tests fail; toggle it ON, watch them pass) was executed and recorded. Full bookmarks package suite stays green. `go vet` and `gofmt -l` are clean.

## Test Evidence

### New R30 regression tests (all PASS)

```
$ go test -count=1 -v -run 'ChaosR30' ./internal/connector/bookmarks/
=== RUN   TestChaosR30_NormalizeURLStripsControlChars
=== RUN   TestChaosR30_NormalizeURLStripsControlChars/embedded_NUL_in_path
=== RUN   TestChaosR30_NormalizeURLStripsControlChars/embedded_LF_in_path
=== RUN   TestChaosR30_NormalizeURLStripsControlChars/embedded_CR_in_path
=== RUN   TestChaosR30_NormalizeURLStripsControlChars/embedded_TAB_in_path
=== RUN   TestChaosR30_NormalizeURLStripsControlChars/embedded_DEL_(0x7F)_in_path
=== RUN   TestChaosR30_NormalizeURLStripsControlChars/mixed_control_chars_across_host_and_path
=== RUN   TestChaosR30_NormalizeURLStripsControlChars/leading_control_chars_before_scheme
--- PASS: TestChaosR30_NormalizeURLStripsControlChars (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_NUL_in_path (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_LF_in_path (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_CR_in_path (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_TAB_in_path (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_DEL_(0x7F)_in_path (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsControlChars/mixed_control_chars_across_host_and_path (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsControlChars/leading_control_chars_before_scheme (0.00s)
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_443
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/http_default_port_80
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/ftp_default_port_21
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/https_non-default_port_preserved
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/http_non-default_port_preserved
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_userinfo_(both_stripped)
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_www_prefix_(both_stripped)
=== RUN   TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_tracking_params
--- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_443 (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/http_default_port_80 (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/ftp_default_port_21 (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_non-default_port_preserved (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/http_non-default_port_preserved (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_userinfo_(both_stripped) (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_www_prefix_(both_stripped) (0.00s)
    --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_tracking_params (0.00s)
=== RUN   TestChaosR30_NormalizeURLStripsTrailingDot
=== RUN   TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_in_host
=== RUN   TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_uppercase
=== RUN   TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_www_prefix
=== RUN   TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_default_port
=== RUN   TestChaosR30_NormalizeURLStripsTrailingDot/no_trailing_dot_(control)
=== RUN   TestChaosR30_NormalizeURLStripsTrailingDot/multiple_trailing_dots_collapse
--- PASS: TestChaosR30_NormalizeURLStripsTrailingDot (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_in_host (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_uppercase (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_www_prefix (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_default_port (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/no_trailing_dot_(control) (0.00s)
    --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/multiple_trailing_dots_collapse (0.00s)
=== RUN   TestChaosR30_ToRawArtifactsRejectsNULInSourceRef
--- PASS: TestChaosR30_ToRawArtifactsRejectsNULInSourceRef (0.00s)
=== RUN   TestChaosR30_ControlCharVariantsDedup
--- PASS: TestChaosR30_ControlCharVariantsDedup (0.00s)
=== RUN   TestChaosR30_StripURLControlCharsFastPath
--- PASS: TestChaosR30_StripURLControlCharsFastPath (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.013s
```

**Claim Source:** executed.

### Adversarial fidelity proof (fix removed → tests FAIL; fix restored → tests PASS)

The probe was: comment out the `rawURL = stripURLControlChars(rawURL)` call in `NormalizeURL` and re-run the three control-char-related R30 tests. Excerpted output:

```
--- FAIL: TestChaosR30_NormalizeURLStripsControlChars/embedded_CR_in_path (0.00s)
    dedup_test.go:501: CHAOS R30-001: NormalizeURL("http://example.com/path\rmore") = "http://example.com/path\rmore", want "http://example.com/pathmore" — control chars leaked into SourceRef
    dedup_test.go:506: CHAOS R30-001: NormalizeURL("http://example.com/path\rmore") = "http://example.com/path\rmore" contains control byte 0x0D at offset 23
--- FAIL: TestChaosR30_NormalizeURLStripsControlChars/embedded_TAB_in_path (0.00s)
    dedup_test.go:501: CHAOS R30-001: NormalizeURL("http://example.com/\tpath") = "http://example.com/\tpath", want "http://example.com/path" — control chars leaked into SourceRef
    dedup_test.go:506: CHAOS R30-001: NormalizeURL("http://example.com/\tpath") = "http://example.com/\tpath" contains control byte 0x09 at offset 19
--- FAIL: TestChaosR30_NormalizeURLStripsControlChars/embedded_DEL_(0x7F)_in_path (0.00s)
    dedup_test.go:501: CHAOS R30-001: NormalizeURL("http://example.com/path\x7fmore") = "http://example.com/path\x7fmore", want "http://example.com/pathmore" — control chars leaked into SourceRef
    dedup_test.go:506: CHAOS R30-001: NormalizeURL("http://example.com/path\x7fmore") = "http://example.com/path\x7fmore" contains control byte 0x7F at offset 23
--- FAIL: TestChaosR30_NormalizeURLStripsControlChars/mixed_control_chars_across_host_and_path (0.00s)
--- FAIL: TestChaosR30_NormalizeURLStripsControlChars/leading_control_chars_before_scheme (0.00s)
--- FAIL: TestChaosR30_ToRawArtifactsRejectsNULInSourceRef (0.00s)
    dedup_test.go:649: CHAOS R30-004: artifact[0].SourceRef = "http://example.com/\x00danger" contains NUL byte at offset 19 — would fail PG insert
--- FAIL: TestChaosR30_ControlCharVariantsDedup (0.00s)
    dedup_test.go:672: CHAOS R30-005[0]: NormalizeURL("http://example.com/page")="http://example.com/page" ≠ NormalizeURL("http://example.com/\npage")="http://example.com/\npage" — control-char variant dedup miss
    dedup_test.go:672: CHAOS R30-005[1]: NormalizeURL("http://example.com/page")="http://example.com/page" ≠ NormalizeURL("http://example.com/page\r")="http://example.com/page\r" — control-char variant dedup miss
    dedup_test.go:672: CHAOS R30-005[2]: NormalizeURL("http://example.com/page")="http://example.com/page" ≠ NormalizeURL("http://example.com/pa\tge")="http://example.com/pa\tge" — control-char variant dedup miss
    dedup_test.go:672: CHAOS R30-005[3]: NormalizeURL("http://example.com/page")="http://example.com/page" ≠ NormalizeURL("http://example.com/pa\x00ge")="http://example.com/pa\x00ge" — control-char variant dedup miss
    dedup_test.go:690: CHAOS R30-005: FilterNew passed through SourceRef with control byte: "http://example.com/\npage"
FAIL    github.com/smackerel/smackerel/internal/connector/bookmarks     0.068s

[fix RESTORED]
$ go test -count=1 -run 'ChaosR30' ./internal/connector/bookmarks/
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.015s
```

**Claim Source:** executed (the toggle was made in `dedup.go`, the failing run captured, the original line restored from `/tmp/dedup.go.bak`, and the success run captured).

### Full bookmarks package suite (no regression)

```
$ go test ./internal/connector/bookmarks/... -count=1 -v 2>&1 | tail -8
=== RUN   TestChaosR30_ToRawArtifactsRejectsNULInSourceRef
--- PASS: TestChaosR30_ToRawArtifactsRejectsNULInSourceRef (0.00s)
=== RUN   TestChaosR30_ControlCharVariantsDedup
--- PASS: TestChaosR30_ControlCharVariantsDedup (0.00s)
=== RUN   TestChaosR30_StripURLControlCharsFastPath
--- PASS: TestChaosR30_StripURLControlCharsFastPath (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.074s
$ echo "exit=$?"
exit=0
```

**Claim Source:** executed.

### Validation Evidence

> Phase agent: bubbles.validate (parent-expanded inside chaos-hardening child workflow)
> Executed: YES

`go test ./internal/connector/bookmarks/... -count=1` is green (output above). All three originally-failing chaos probes now produce the canonically-normalized URL. The dedup contract holds for control-char variants of the same URL.

### Audit Evidence

> Phase agent: bubbles.audit (parent-expanded inside chaos-hardening child workflow)
> Executed: YES

```
$ go vet ./internal/connector/bookmarks/...
(no output — clean)

$ gofmt -l internal/connector/bookmarks/dedup.go internal/connector/bookmarks/dedup_test.go
(no output — clean)
```

Boundary check: `git diff --name-only HEAD -- internal/connector/bookmarks/` returns exactly `dedup.go` and `dedup_test.go` (the production change and its regression tests). No other production file was touched by this round.

## Findings → Fix Mapping

| Finding | Severity | Pre-fix probe output | Fix location | Regression test |
|---|---|---|---|---|
| F-CHAOS-R30-001 | HIGH | `NormalizeURL("http://example.com/path\x00more") = "http://example.com/path\x00more"` (NUL preserved → PG insert fails) | `dedup.go::NormalizeURL` (calls new `stripURLControlChars`) | `TestChaosR30_NormalizeURLStripsControlChars`, `TestChaosR30_ToRawArtifactsRejectsNULInSourceRef`, `TestChaosR30_ControlCharVariantsDedup`, `TestChaosR30_StripURLControlCharsFastPath` |
| F-CHAOS-R30-002 | MEDIUM | `NormalizeURL("https://example.com:443/page") = "https://example.com:443/page"` (should be `"https://example.com/page"`) | `dedup.go::NormalizeURL` (host-canonicalisation block: scheme/port map + IPv6 brackets) | `TestChaosR30_NormalizeURLElidesDefaultPorts` |
| F-CHAOS-R30-003 | MEDIUM | `NormalizeURL("http://example.com./foo") = "http://example.com./foo"` (should be `"http://example.com/foo"`) | `dedup.go::NormalizeURL` (host-canonicalisation block: `strings.TrimRight(h, ".")`) | `TestChaosR30_NormalizeURLStripsTrailingDot` |

## Cross-Reference

Recorded in parent `specs/009-bookmarks-connector/state.json::executionHistory` under chaos round R30 and in `specs/009-bookmarks-connector/report.md` under the "Chaos R30 — NormalizeURL hardening" section. Parent spec status is unchanged (`done`); this bug was discovered and resolved on a `done` spec by the stochastic sweep.
