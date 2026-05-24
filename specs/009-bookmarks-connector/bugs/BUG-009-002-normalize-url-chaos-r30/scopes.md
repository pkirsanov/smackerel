# Scopes: BUG-009-002 — NormalizeURL chaos R30

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Harden NormalizeURL against control chars, default ports, trailing-dot hosts

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BK-FIX-002-001 Control characters are stripped before SourceRef is written
  Given a bookmark URL "http://example.com/path\x00more"
  When NormalizeURL is called
  Then the result does not contain any byte in 0x00-0x1F or 0x7F
  And ToRawArtifacts produces a SourceRef that PostgreSQL TEXT can store without insert failure
  And two URLs that differ only by an embedded control char produce the same normalized SourceRef

Scenario: SCN-BK-FIX-002-002 Default protocol ports are elided
  Given a bookmark URL "https://example.com:443/page"
  When NormalizeURL is called
  Then the result is "https://example.com/page" without the ":443" suffix
  And the same elision applies to "http://...:80/..." and "ftp://...:21/..."
  And non-default ports such as "https://example.com:8443/api" are preserved verbatim

Scenario: SCN-BK-FIX-002-003 Trailing DNS-root dot is stripped from the hostname
  Given a bookmark URL "http://example.com./foo"
  When NormalizeURL is called
  Then the result is "http://example.com/foo" without the trailing dot
  And the strip composes correctly with www. stripping and default-port elision
```

### Implementation Plan

1. Add `stripURLControlChars(s string) string` to `internal/connector/bookmarks/dedup.go` with a "scan first, allocate only if dirty" fast path that drops every byte where `c < 0x20 || c == 0x7F`. (F-CHAOS-R30-001)
2. Call `rawURL = stripURLControlChars(rawURL)` at the top of `NormalizeURL`, before `url.Parse`, after the empty-string guard. (F-CHAOS-R30-001)
3. After the existing scheme/host lowercasing and userinfo strip, replace the inline `www.` strip with a host-canonicalisation block that:
   - `strings.TrimRight(u.Hostname(), ".")` to drop the DNS-root dot (F-CHAOS-R30-003)
   - strips the `www.` prefix if still present (IMP-009-R-002 preserved)
   - reads `u.Port()` and consults `defaultPorts := map[string]string{"http":"80","https":"443","ftp":"21"}` to clear the port when it matches the scheme default (F-CHAOS-R30-002)
   - re-adds IPv6 brackets when the bare hostname contains `:` so `[2001:db8::1]:443` collapses to `[2001:db8::1]`
   - reassembles `u.Host` as `hostname` or `hostname + ":" + port` depending on whether the port was elided
4. Add adversarial regression tests in `internal/connector/bookmarks/dedup_test.go`:
   - `TestChaosR30_NormalizeURLStripsControlChars` — 7 sub-cases covering NUL, LF, CR, TAB, DEL, mixed host+path, leading bytes (SCN-BK-FIX-002-001)
   - `TestChaosR30_NormalizeURLElidesDefaultPorts` — 8 sub-cases covering https/http/ftp default + non-default + interaction with userinfo / www. / tracking params (SCN-BK-FIX-002-002)
   - `TestChaosR30_NormalizeURLStripsTrailingDot` — 6 sub-cases covering plain trailing dot, uppercase, www., default port, no-dot control, multi-dot collapse (SCN-BK-FIX-002-003)
   - `TestChaosR30_ToRawArtifactsRejectsNULInSourceRef` — end-to-end via `ToRawArtifacts` that no SourceRef contains 0x00 (SCN-BK-FIX-002-001)
   - `TestChaosR30_ControlCharVariantsDedup` — proves two URLs that differ only by embedded control bytes normalize to the same string, and the nil-pool FilterNew path stays correct (SCN-BK-FIX-002-001)
   - `TestChaosR30_StripURLControlCharsFastPath` — fast-path unit (allocation-free on clean input, empty-input passthrough) (SCN-BK-FIX-002-001)
5. Prove adversarial fidelity: temporarily comment out the `stripURLControlChars` call in `NormalizeURL`; re-run the 3 control-char-related tests; confirm they FAIL; restore the call; confirm they PASS again. (Acceptance criterion #4 in spec.md)
6. Run `go test ./internal/connector/bookmarks/... -count=1` and confirm the full bookmarks package suite is green.
7. Run `go vet ./internal/connector/bookmarks/...` and `gofmt -l internal/connector/bookmarks/dedup.go internal/connector/bookmarks/dedup_test.go` — both clean.
8. Append a "Chaos R30 — NormalizeURL hardening" section to `specs/009-bookmarks-connector/report.md` and one R30 entry to `specs/009-bookmarks-connector/state.json::executionHistory`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BK-FIX-002-01 | `TestChaosR30_NormalizeURLStripsControlChars` | unit | `internal/connector/bookmarks/dedup_test.go` | 7/7 sub-cases PASS; result contains no byte `< 0x20` or `== 0x7F` | SCN-BK-FIX-002-001 |
| T-BK-FIX-002-02 | `TestChaosR30_ToRawArtifactsRejectsNULInSourceRef` | unit | `internal/connector/bookmarks/dedup_test.go` | `ToRawArtifacts` output has no NUL byte in any `SourceRef` | SCN-BK-FIX-002-001 |
| T-BK-FIX-002-03 | `TestChaosR30_ControlCharVariantsDedup` | unit | `internal/connector/bookmarks/dedup_test.go` | 4 control-char variant pairs normalize to identical strings; `FilterNew` nil-pool path correct | SCN-BK-FIX-002-001 |
| T-BK-FIX-002-04 | `TestChaosR30_StripURLControlCharsFastPath` | unit | `internal/connector/bookmarks/dedup_test.go` | clean input returned unchanged; empty-string passthrough | SCN-BK-FIX-002-001 |
| T-BK-FIX-002-05 | `TestChaosR30_NormalizeURLElidesDefaultPorts` | unit | `internal/connector/bookmarks/dedup_test.go` | 8/8 sub-cases PASS — defaults elided, non-defaults preserved, composition with userinfo / www. / tracking params correct | SCN-BK-FIX-002-002 |
| T-BK-FIX-002-06 | `TestChaosR30_NormalizeURLStripsTrailingDot` | unit | `internal/connector/bookmarks/dedup_test.go` | 6/6 sub-cases PASS — plain, uppercase, with www., with default port, no-dot control, multi-dot collapse | SCN-BK-FIX-002-003 |
| T-BK-FIX-002-07 | Full bookmarks package suite | unit | `internal/connector/bookmarks/...` | `go test ./internal/connector/bookmarks/... -count=1` exit 0; no regression in any existing test | All three |
| T-BK-FIX-002-08 | Adversarial proof | manual | (procedure documented in report.md) | Toggling the `stripURLControlChars` call OFF causes 3 R30 tests to FAIL; toggling it back ON returns all R30 tests to PASS | SCN-BK-FIX-002-001 |

### Definition of Done

- [x] `Scenario SCN-BK-FIX-002-001 Control characters are stripped before SourceRef is written` — `NormalizeURL` strips every byte where `c < 0x20 || c == 0x7F` before `url.Parse` and the result is verified to contain no control byte. **Phase:** implement
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestChaosR30_NormalizeURLStripsControlChars$|TestChaosR30_ToRawArtifactsRejectsNULInSourceRef$|TestChaosR30_ControlCharVariantsDedup$|TestChaosR30_StripURLControlCharsFastPath$' ./internal/connector/bookmarks/
  > === RUN   TestChaosR30_NormalizeURLStripsControlChars
  > --- PASS: TestChaosR30_NormalizeURLStripsControlChars (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_NUL_in_path (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_LF_in_path (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_CR_in_path (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_TAB_in_path (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsControlChars/embedded_DEL_(0x7F)_in_path (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsControlChars/mixed_control_chars_across_host_and_path (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsControlChars/leading_control_chars_before_scheme (0.00s)
  > === RUN   TestChaosR30_ToRawArtifactsRejectsNULInSourceRef
  > --- PASS: TestChaosR30_ToRawArtifactsRejectsNULInSourceRef (0.00s)
  > === RUN   TestChaosR30_ControlCharVariantsDedup
  > --- PASS: TestChaosR30_ControlCharVariantsDedup (0.00s)
  > === RUN   TestChaosR30_StripURLControlCharsFastPath
  > --- PASS: TestChaosR30_StripURLControlCharsFastPath (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.024s
  > ```
- [x] `Scenario SCN-BK-FIX-002-002 Default protocol ports are elided` — http/https/ftp default ports are dropped; non-default ports are preserved. **Phase:** implement
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestChaosR30_NormalizeURLElidesDefaultPorts' ./internal/connector/bookmarks/
  > === RUN   TestChaosR30_NormalizeURLElidesDefaultPorts
  > --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_443 (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/http_default_port_80 (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/ftp_default_port_21 (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_non-default_port_preserved (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/http_non-default_port_preserved (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_userinfo_(both_stripped) (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_www_prefix_(both_stripped) (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLElidesDefaultPorts/https_default_port_with_tracking_params (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.012s
  > ```
- [x] `Scenario SCN-BK-FIX-002-003 Trailing DNS-root dot is stripped from the hostname` — `example.com.` collapses to `example.com`; multi-dot trailing collapses too. **Phase:** implement
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestChaosR30_NormalizeURLStripsTrailingDot' ./internal/connector/bookmarks/
  > === RUN   TestChaosR30_NormalizeURLStripsTrailingDot
  > --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_in_host (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_uppercase (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_www_prefix (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/trailing_dot_with_default_port (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/no_trailing_dot_(control) (0.00s)
  >     --- PASS: TestChaosR30_NormalizeURLStripsTrailingDot/multiple_trailing_dots_collapse (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.010s
  > ```
- [x] Adversarial proof — reverting the `stripURLControlChars` call causes 3 R30 tests to FAIL with concrete log lines naming the leaked control byte; restoring the call returns the suite to green. **Phase:** test
  > Evidence (excerpt — full transcript in report.md):
  > ```
  > [fix REMOVED]
  > --- FAIL: TestChaosR30_NormalizeURLStripsControlChars
  >     dedup_test.go:501: CHAOS R30-001: NormalizeURL("http://example.com/path\rmore") = "http://example.com/path\rmore" — control chars leaked into SourceRef
  >     dedup_test.go:506: CHAOS R30-001: NormalizeURL("http://example.com/path\rmore") contains control byte 0x0D at offset 23
  > --- FAIL: TestChaosR30_ToRawArtifactsRejectsNULInSourceRef
  >     dedup_test.go:649: CHAOS R30-004: artifact[0].SourceRef contains NUL byte at offset 19 — would fail PG insert
  > --- FAIL: TestChaosR30_ControlCharVariantsDedup
  >     dedup_test.go:672: CHAOS R30-005[0]: NormalizeURL(clean)≠NormalizeURL(dirty) — control-char variant dedup miss
  > FAIL    github.com/smackerel/smackerel/internal/connector/bookmarks     0.068s
  >
  > [fix RESTORED]
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.015s
  > ```
- [x] Full bookmarks package suite stays green. **Phase:** test
  > Evidence (full -v transcript tail; -count=1 forces no cache):
  > ```
  > $ go test ./internal/connector/bookmarks/... -count=1 -v 2>&1 | tail -8
  > === RUN   TestChaosR30_ToRawArtifactsRejectsNULInSourceRef
  > --- PASS: TestChaosR30_ToRawArtifactsRejectsNULInSourceRef (0.00s)
  > === RUN   TestChaosR30_ControlCharVariantsDedup
  > --- PASS: TestChaosR30_ControlCharVariantsDedup (0.00s)
  > === RUN   TestChaosR30_StripURLControlCharsFastPath
  > --- PASS: TestChaosR30_StripURLControlCharsFastPath (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.074s
  > $ echo "exit=$?"
  > exit=0
  > ```
- [x] `go vet` and `gofmt -l` are clean on the modified files. **Phase:** audit
  > Evidence (separate commands so exit codes are individually visible):
  > ```
  > $ go vet ./internal/connector/bookmarks/...
  > $ echo "vet exit=$?"
  > vet exit=0
  > $ gofmt -l internal/connector/bookmarks/dedup.go internal/connector/bookmarks/dedup_test.go
  > $ echo "fmt exit=$?"
  > fmt exit=0
  > # both commands emit no output and exit 0 — vet clean, fmt clean
  > ```
- [x] Parent `specs/009-bookmarks-connector/state.json` has a new R30 execution-history entry and `report.md` has a Chaos R30 section that names this bug and lists the three F-CHAOS-R30-NNN findings with status `Fixed`. **Phase:** docs
  > Evidence (grep returns matches in BOTH parent files):
  > ```
  > $ grep -n 'BUG-009-002-normalize-url-chaos-r30' \
  >     specs/009-bookmarks-connector/report.md \
  >     specs/009-bookmarks-connector/state.json
  > specs/009-bookmarks-connector/report.md:681:**Bug:** [BUG-009-002-normalize-url-chaos-r30](bugs/BUG-009-002-normalize-url-chaos-r30/spec.md) — status `done`.
  > specs/009-bookmarks-connector/state.json:300:      "summary": "Stochastic-quality-sweep sweep-2026-05-23-r30 round 14, parent-expanded child workflow mode chaos-hardening. ... Spawned specs/009-bookmarks-connector/bugs/BUG-009-002-normalize-url-chaos-r30/ with full 6-artifact set ..."
  > ```
