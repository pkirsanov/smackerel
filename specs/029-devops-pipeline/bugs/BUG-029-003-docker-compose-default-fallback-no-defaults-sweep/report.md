# Report: BUG-029-003 — Dev `docker-compose.yml` violates Gate G028 NO-DEFAULTS via 14 `${VAR:-default}` substitutions

## Summary

Closes home-lab readiness re-scan finding **HL-RESCAN-012** (P3): the dev compose file `docker-compose.yml` had **14 forbidden `${VAR:-default}` silent-fallback substitutions** in violation of Gate G028 (NO-DEFAULTS / fail-loud SST policy). The pre-fix state masked real misconfiguration in any unsanctioned `docker compose -f docker-compose.yml up` invocation that did not first run `./smackerel.sh config generate` — the fallback values silently substituted `dev / unknown / unknown` for `SMACKEREL_VERSION / SMACKEREL_COMMIT / SMACKEREL_BUILD_TIME`, silently fell back to `config/generated/dev.env` for `SMACKEREL_ENV_FILE`, and silently treated unset `SMACKEREL_CORE_IMAGE / SMACKEREL_ML_IMAGE` as "build from source".

The fix has three coordinated changes that close 10 of 14 violations and explicitly defer the remaining 4:

1. **`scripts/commands/config.sh`** (+35 lines): adds a 5-block resolution section (`if [[ -z "${X+set}" ]]; then X="default"; fi`) for `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`, `SMACKEREL_CORE_IMAGE`, `SMACKEREL_ML_IMAGE` that resolves shell-env overrides and falls back to safe seed values, plus 5 emission lines into the heredoc that writes `config/generated/<env>.env`. The `+set` form is shell substitution-context "X is set" detection (NOT a Gate G028 violation — the policy is about Compose substitution, not about helper-script SST-emission resolution).
2. **`docker-compose.yml`** (+47 / -10 lines): converts 6 build-metadata + 2 env-file path occurrences from `${X:-default}` to `${X:?error message}` (fails on unset OR empty); converts 2 image-ref occurrences from `${X:-}` to `${X?error message}` (fails on unset only — empty is the build-from-source toggle); leaves 4 volume-mount occurrences unchanged with an inline 11-line deferral comment block documenting the load-bearing empty-value contract with connector code.
3. **`internal/deploy/dev_compose_default_fallback_test.go`** (NEW FILE, 270 lines): static-file lint test in `package deploy` that parses the live dev compose file line-by-line, regex-matches `${VAR:-default}` occurrences, filters via per-var allowlist (the 4 volume-mount vars), and asserts zero unauthorized matches. Includes 4 test functions: `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` (live-file scan), `TestDevComposeContract_AdversarialUnauthorizedDefaultFallback` (4 sub-cases: build-metadata regression / env-file regression / image-ref regression / novel non-allowlisted var), `TestDevComposeContract_AdversarialAllowlistRespected` (per-var-not-per-line gating), `TestDevComposeContract_AdversarialCommentLinesIgnored` (comment-line skip).

**Workflow Mode:** test-to-doc — the fix bundles the SST emission, compose-form conversion, and test addition into a single packet. No production runtime Go/Python code changes; no `deploy/compose.deploy.yml` change; no foreign-owned spec content edits.

**Parent re-scan:** home-lab readiness re-scan 2026-05-14, finding HL-RESCAN-012. Sequential per-finding bug-packet workflow under spec 029 (DevOps Pipeline & Image Governance — owns the `./smackerel.sh up` workflow + dev `docker-compose.yml` + `./smackerel.sh config generate` SST chain).

## Completion Statement

All seven bug-packet artifacts (`spec.md`, `design.md`, `scopes.md`, `scenario-manifest.json`, `report.md`, `uservalidation.md`, `state.json`) authored. Three source files modified (`scripts/commands/config.sh` +35 / -0 lines, `docker-compose.yml` +47 / -10 lines, new `internal/deploy/dev_compose_default_fallback_test.go` +270 lines). Targeted dev-compose contract suite (4 new test functions + sub-cases) GREEN. Targeted prod-compose canary suite (9 pre-existing `TestComposeContract_*` + spec 047 vuln-gate + BUG-047-001 bundle-hash) GREEN unchanged. Cross-package smoke (`./smackerel.sh test unit --go`) GREEN. RED proof captured by temporarily reverting `${SMACKEREL_VERSION:?...}` → `${SMACKEREL_VERSION:-dev}` on smackerel-core: `TestDevComposeContract_NoUnauthorizedDefaultFallbacks` correctly FAILS with the expected `docker-compose.yml violates Gate G028 (NO-DEFAULTS / fail-loud SST policy) — HL-RESCAN-012:` message naming the regressed var. Restoration returns the test to PASS. Compose substitution GREEN against regenerated `config/generated/dev.env` and `config/generated/test.env`; Compose substitution RED against `--env-file /dev/null` correctly aborts with `error while interpolating services.smackerel-core.env_file.[]: required variable SMACKEREL_ENV_FILE is missing a value: must be set — use ./smackerel.sh up or export SMACKEREL_ENV_FILE=config/generated/dev.env` (proves Gate G028 fail-loud is active end-to-end).

**Status: SHIP_IT** — see [uservalidation.md](uservalidation.md).

## Implementation Code Diff

### Code Diff Evidence

**Claim Source:** executed

```text
$ git diff --stat HEAD -- scripts/commands/config.sh docker-compose.yml
 docker-compose.yml         | 47 ++++++++++++++++++++++++++++++++++++----------
 scripts/commands/config.sh | 35 ++++++++++++++++++++++++++++++++++
 2 files changed, 72 insertions(+), 10 deletions(-)

$ git status --porcelain internal/deploy/dev_compose_default_fallback_test.go
?? internal/deploy/dev_compose_default_fallback_test.go

$ wc -l internal/deploy/dev_compose_default_fallback_test.go
270 internal/deploy/dev_compose_default_fallback_test.go
```

**Interpretation:** the change boundary is exactly three source files: two modified (`scripts/commands/config.sh` +35 / -0 lines, `docker-compose.yml` +47 / -10 lines) and one new (`internal/deploy/dev_compose_default_fallback_test.go` 270 lines). The new test file is untracked and will be added explicitly via `git add` (no `git add .` shortcut) to keep the commit boundary tight against parallel-session WIP files in the working tree.

#### SST emission additions (`scripts/commands/config.sh`)

```text
$ grep -n 'SMACKEREL_VERSION\|SMACKEREL_COMMIT\|SMACKEREL_BUILD_TIME\|SMACKEREL_CORE_IMAGE\|SMACKEREL_ML_IMAGE' scripts/commands/config.sh | head -25
940:# HL-RESCAN-012 / Gate G028 — build-metadata SST resolution. The five
941:# vars below feed the fail-loud `${X:?...}` and `${X?...}` substitution
944:# .github/workflows/ci.yml lines 119-120 export SMACKEREL_VERSION /
945:# SMACKEREL_COMMIT before invoking ./smackerel.sh build, so the CI
948:if [[ -z "${SMACKEREL_VERSION+set}" ]]; then SMACKEREL_VERSION="dev"; fi
949:if [[ -z "${SMACKEREL_COMMIT+set}" ]]; then SMACKEREL_COMMIT="unknown"; fi
950:if [[ -z "${SMACKEREL_BUILD_TIME+set}" ]]; then SMACKEREL_BUILD_TIME="unknown"; fi
951:if [[ -z "${SMACKEREL_CORE_IMAGE+set}" ]]; then SMACKEREL_CORE_IMAGE=""; fi
952:if [[ -z "${SMACKEREL_ML_IMAGE+set}" ]]; then SMACKEREL_ML_IMAGE=""; fi
...
965:SMACKEREL_VERSION=${SMACKEREL_VERSION}
966:SMACKEREL_COMMIT=${SMACKEREL_COMMIT}
967:SMACKEREL_BUILD_TIME=${SMACKEREL_BUILD_TIME}
968:SMACKEREL_CORE_IMAGE=${SMACKEREL_CORE_IMAGE}
969:SMACKEREL_ML_IMAGE=${SMACKEREL_ML_IMAGE}
```

**Interpretation:** the 5-block resolution section (lines 948-952) and the 5 heredoc emission lines (lines 965-969) are present. The resolution uses `${X+set}` substitution-context detection (not a Gate G028 violation — the `+set` form yields `set` if X is set OR `set if X is unset`, and `[[ -z ... ]]` is true only when X is unset). The leading comment block (lines 940-947) names HL-RESCAN-012 + Gate G028 + the CI export precedence chain.

#### Fail-loud forms in `docker-compose.yml` (smackerel-core + smackerel-ml)

```text
$ grep -nE 'SMACKEREL_(VERSION|COMMIT|BUILD_TIME|ENV_FILE|CORE_IMAGE|ML_IMAGE)' docker-compose.yml | head -20
59:    image: ${SMACKEREL_CORE_IMAGE?must be set in env file (run ./smackerel.sh config generate); empty value is allowed for build-from-source dev}
75:        VERSION: ${SMACKEREL_VERSION:?must be set in env file (run ./smackerel.sh config generate)}
76:        COMMIT_HASH: ${SMACKEREL_COMMIT:?must be set in env file (run ./smackerel.sh config generate)}
77:        BUILD_TIME: ${SMACKEREL_BUILD_TIME:?must be set in env file (run ./smackerel.sh config generate)}
108:    env_file: ${SMACKEREL_ENV_FILE:?must be set — use ./smackerel.sh up or export SMACKEREL_ENV_FILE=config/generated/dev.env}
146:    image: ${SMACKEREL_ML_IMAGE?must be set in env file (run ./smackerel.sh config generate); empty value is allowed for build-from-source dev}
162:        VERSION: ${SMACKEREL_VERSION:?must be set in env file (run ./smackerel.sh config generate)}
163:        COMMIT_HASH: ${SMACKEREL_COMMIT:?must be set in env file (run ./smackerel.sh config generate)}
164:        BUILD_TIME: ${SMACKEREL_BUILD_TIME:?must be set in env file (run ./smackerel.sh config generate)}
194:    env_file: ${SMACKEREL_ENV_FILE:?must be set — use ./smackerel.sh up or export SMACKEREL_ENV_FILE=config/generated/dev.env}
```

**Interpretation:** all 10 converted occurrences are visible: 6 build-metadata `:?` forms (lines 75-77 + 162-164), 2 env-file `:?` forms (lines 108 + 194), 2 image-ref `?` (no-colon) forms (lines 59 + 146). Each error message names the regression target (Gate G028 / fix path) inline.

#### Volume-mount deferral (4 explicit allowlist entries with inline comment)

```text
$ grep -nE '\$\{[A-Z_]+:-' docker-compose.yml
120:      # `${X:-./data/...}` form because the env file emits them empty by
130:      - ${BOOKMARKS_IMPORT_DIR:-./data/bookmarks-import}:/data/bookmarks-import:ro
131:      - ${MAPS_IMPORT_DIR:-./data/maps-import}:/data/maps-import:ro
132:      - ${BROWSER_HISTORY_PATH:-./data/browser-history/History}:/data/browser-history/History:ro
133:      - ${TWITTER_ARCHIVE_DIR:-./data/twitter-archive}:/data/twitter-archive:ro
```

**Interpretation:** exactly 4 `${VAR:-default}` occurrences remain (lines 130-133 — the 4 volume-mount vars). Line 120 is part of the allowlist-justification comment block documenting the load-bearing empty-value contract with connector code. The full 11-line comment block at lines 115-125 documents (a) why these are not converted, (b) the connector `${X:+/data/...}` env-override pattern that consumes the empty signal, (c) the subsequent connector-refactor packet.

#### New test file structure

```text
$ grep -nE '^func |^var devComposeDefaultFallbackAllowlist|^var devComposeDefaultFallbackRegex' internal/deploy/dev_compose_default_fallback_test.go
36:var devComposeDefaultFallbackAllowlist = map[string]string{
72:var devComposeDefaultFallbackRegex = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*):-[^}]*\}`)
77:func findDevComposeUnauthorizedDefaultFallbacks(yamlBytes []byte, allowlist map[string]string) []string {
108:func TestDevComposeContract_NoUnauthorizedDefaultFallbacks(t *testing.T) {
136:func TestDevComposeContract_AdversarialUnauthorizedDefaultFallback(t *testing.T) {
196:func TestDevComposeContract_AdversarialAllowlistRespected(t *testing.T) {
242:func TestDevComposeContract_AdversarialCommentLinesIgnored(t *testing.T) {
```

```text
$ grep -n 'HL-RESCAN-012' internal/deploy/dev_compose_default_fallback_test.go | head -5
2:// HL-RESCAN-012 / Gate G028 — dev compose file `docker-compose.yml` static-file lint
17:// HL-RESCAN-012 / Gate G028 attribution: package docstring (this file
130:                "docker-compose.yml violates Gate G028 (NO-DEFAULTS / fail-loud SST policy) — HL-RESCAN-012:\n  - %s\n(use ${VAR:?error} fail-loud form, or add VAR to devComposeDefaultFallbackAllowlist with justification)",
181:                                "(HL-RESCAN-012 regression: %s — fixture %q produced empty unauthorized list; the lint helper is over-permissive)",
```

**Interpretation:** the new test file declares one allowlist (`devComposeDefaultFallbackAllowlist`), one regex (`devComposeDefaultFallbackRegex`), one helper (`findDevComposeUnauthorizedDefaultFallbacks`), and four `Test*` functions. HL-RESCAN-012 attribution appears in the package docstring (line 2), in the docstring rationale (line 17), in the live-file lint failure message (line 130), and in the adversarial sub-case failure message (line 181). The breadcrumb is uniformly visible in every failure path.

**Excluded surfaces (negative evidence):** zero lines changed in `internal/deploy/compose_contract_test.go` (the prod-compose contract test is in a separate file targeting `deploy/compose.deploy.yml`, bit-identical to HEAD). Zero lines changed in `deploy/compose.deploy.yml`. Zero lines changed in `config/smackerel.yaml`. Zero lines changed in `scripts/lib/runtime.sh`. Zero lines changed in `.github/workflows/*` (HL-RESCAN-011 is a separate sprint item). Zero lines changed in production runtime Go code under `internal/auth/`, `internal/config/`, `internal/api/`, `cmd/`. Zero lines changed in any other `specs/**` directory.

## Test Evidence

### Validation Evidence

**Claim Source:** executed

#### Targeted dev-compose contract suite (full new test set)

```text
$ go test -count=1 -v -run '^TestDevComposeContract' ./internal/deploy/...
=== RUN   TestDevComposeContract_NoUnauthorizedDefaultFallbacks
    dev_compose_default_fallback_test.go:132: contract OK: docker-compose.yml has zero unauthorized ${VAR:-default} forms (allowlist size = 4, all allowlist entries justified inline in the YAML and in the test source)
--- PASS: TestDevComposeContract_NoUnauthorizedDefaultFallbacks (0.00s)
=== RUN   TestDevComposeContract_AdversarialUnauthorizedDefaultFallback
--- PASS: TestDevComposeContract_AdversarialUnauthorizedDefaultFallback (0.00s)
=== RUN   TestDevComposeContract_AdversarialAllowlistRespected
    dev_compose_default_fallback_test.go:238: adversarial OK: allowlisted ${BOOKMARKS_IMPORT_DIR:-...} accepted, rogue ${ROGUE_VAR:-rogue-default} rejected; full unauthorized list:
        8:${ROGUE_VAR:-rogue-default}
--- PASS: TestDevComposeContract_AdversarialAllowlistRespected (0.00s)
=== RUN   TestDevComposeContract_AdversarialCommentLinesIgnored
    dev_compose_default_fallback_test.go:269: adversarial OK: comment-line documentation reference ignored, active forbidden form rejected; full unauthorized list:
        6:${ROGUE_VAR:-rogue-default}
--- PASS: TestDevComposeContract_AdversarialCommentLinesIgnored (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.007s
```

**Interpretation:** all 4 new test functions PASS. The live-file scan confirms zero unauthorized `${X:-default}` forms in the post-fix `docker-compose.yml`. The adversarial allowlist-respected test correctly accepts an allowlisted `${BOOKMARKS_IMPORT_DIR:-...}` while rejecting a rogue `${ROGUE_VAR:-rogue-default}` on the same fixture. The adversarial comment-line test correctly skips a comment line containing `${VAR:-default}` documentation reference. Wall clock: 0.007s. Exit status: 0 / `PASS`.

#### Targeted prod-compose canary suite (full pre-existing contracts)

```text
$ go test -count=1 -v -run '^TestComposeContract|^TestVulnGateContract|^TestBundleHashContract' ./internal/deploy/... | tail -25
    --- PASS: TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028) (0.00s)
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind
--- PASS: TestComposeContract_AdversarialDefaultFallbackBind (0.00s)
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms
--- PASS: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.021s
```

**Interpretation:** all 9 pre-existing `TestComposeContract_*` top-level tests + sub-tests + 11 `TestVulnGateContract_*` + 5 `TestBundleHashContract_*` PASS unchanged. The new dev-compose test is purely additive against a different file (`docker-compose.yml`) and does not over-reach into the prod-compose contract surface (`deploy/compose.deploy.yml`).

#### Cross-package smoke

```text
$ ./smackerel.sh test unit --go
... (full go test ./... output across cmd/* and internal/* packages — final marker shown) ...
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

**Interpretation:** `[go-unit] go test ./... finished OK` is the Smackerel runtime test runner's success marker for the full Go unit suite. No regression in any other internal/* package; the new test function is purely additive and confined to `internal/deploy/`.

#### SST emission (Compose-substitution-context plumbing)

```text
$ bash scripts/commands/config.sh --env dev 2>&1 | tail -3
Generated ~/smackerel/config/generated/dev.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml

$ grep -nE '^SMACKEREL_(VERSION|COMMIT|BUILD_TIME|CORE_IMAGE|ML_IMAGE|ENV_FILE)=' config/generated/dev.env
5:SMACKEREL_ENV_FILE=config/generated/dev.env
10:SMACKEREL_VERSION=dev
11:SMACKEREL_COMMIT=unknown
12:SMACKEREL_BUILD_TIME=unknown
13:SMACKEREL_CORE_IMAGE=
14:SMACKEREL_ML_IMAGE=

$ bash scripts/commands/config.sh --env test 2>&1 | tail -3
Generated ~/smackerel/config/generated/test.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml

$ grep -nE '^SMACKEREL_(VERSION|COMMIT|BUILD_TIME|CORE_IMAGE|ML_IMAGE|ENV_FILE)=' config/generated/test.env
5:SMACKEREL_ENV_FILE=config/generated/test.env
10:SMACKEREL_VERSION=dev
11:SMACKEREL_COMMIT=unknown
12:SMACKEREL_BUILD_TIME=unknown
13:SMACKEREL_CORE_IMAGE=
14:SMACKEREL_ML_IMAGE=
```

**Interpretation:** both regenerated env files carry the 5 new SST-emitted vars + the pre-existing `SMACKEREL_ENV_FILE` line (6 total). The values are safe seed values (`dev` / `unknown` / `unknown` / empty / empty) when no shell-env override is present — exactly the SCN-029-003-A contract.

#### Compose substitution GREEN

```text
$ docker compose --env-file config/generated/dev.env -f ./docker-compose.yml config -q
$ echo "Exit: $?"
Exit: 0

$ docker compose --env-file config/generated/test.env -f ./docker-compose.yml config -q
$ echo "Exit: $?"
Exit: 0
```

**Interpretation:** both regenerated env files satisfy the new fail-loud `${X:?...}` and `${X?...}` substitution forms. Compose `config -q` validates the YAML and exits 0, proving the SST emission successfully feeds the Compose substitution context for the sanctioned developer workflow. SCN-029-003-B contract met.

#### Compose substitution RED proof of fail-loud

```text
$ docker compose --env-file /dev/null -f docker-compose.yml config -q 2>&1 | grep -i 'error\|fail\|missing\|smackerel' | head -3
error while interpolating services.smackerel-core.env_file.[]: required variable SMACKEREL_ENV_FILE is missing a value: must be set — use ./smackerel.sh up or export SMACKEREL_ENV_FILE=config/generated/dev.env
$ echo "Exit: $?"
Exit: 1
```

**Interpretation:** Compose aborts with the exact named-var error message authored in `docker-compose.yml`. The error names `SMACKEREL_ENV_FILE` (one of the new fail-loud-substituted vars) and embeds the fix path (`./smackerel.sh up or export SMACKEREL_ENV_FILE=config/generated/dev.env`). Gate G028 fail-loud is verified active end-to-end. SCN-029-003-C contract met.

#### Static checks

```text
$ go vet ./internal/deploy/...
$ echo "exit=$?"
exit=0

$ gofmt -l internal/deploy/
$ echo "gofmt-exit=$?"
gofmt-exit=0
```

**Interpretation:** zero `go vet` warnings, zero `gofmt -l` output (empty list ⇒ no formatting drift). Static surface is clean.

### Red→Green proof (scenario-first TDD)

**Claim Source:** executed

#### RED phase — temporarily revert one converted form

Reverted `docker-compose.yml` line 75 from `VERSION: ${SMACKEREL_VERSION:?must be set in env file (run ./smackerel.sh config generate)}` to `VERSION: ${SMACKEREL_VERSION:-dev}` via `replace_string_in_file` (single-form revert; smackerel-core service only; smackerel-ml line 162 left intact to keep the fixture minimal).

```text
$ go test -count=1 -v -run '^TestDevComposeContract_NoUnauthorizedDefaultFallbacks$' ./internal/deploy/...
=== RUN   TestDevComposeContract_NoUnauthorizedDefaultFallbacks
    dev_compose_default_fallback_test.go:130: docker-compose.yml violates Gate G028 (NO-DEFAULTS / fail-loud SST policy) — HL-RESCAN-012:
          - 75:${SMACKEREL_VERSION:-dev} (use ${VAR:?error} fail-loud form, or add VAR to devComposeDefaultFallbackAllowlist with justification)
        
--- FAIL: TestDevComposeContract_NoUnauthorizedDefaultFallbacks (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.007s
FAIL
```

**Interpretation (RED):** the lint test correctly catches the regression. Failure message names the line number (75), the violating form (`${SMACKEREL_VERSION:-dev}`), the violating var (`SMACKEREL_VERSION`), the fix path (use `${VAR:?error}` fail-loud form, or add to allowlist with justification), and the breadcrumb (`HL-RESCAN-012`). Future maintainer hitting this FAIL has every signpost needed to navigate to this bug packet.

#### GREEN phase — restore strict `${SMACKEREL_VERSION:?...}`

Restored `docker-compose.yml` line 75 back to `VERSION: ${SMACKEREL_VERSION:?must be set in env file (run ./smackerel.sh config generate)}` via `replace_string_in_file`.

```text
$ go test -count=1 -v -run '^TestDevComposeContract_NoUnauthorizedDefaultFallbacks$' ./internal/deploy/...
=== RUN   TestDevComposeContract_NoUnauthorizedDefaultFallbacks
    dev_compose_default_fallback_test.go:132: contract OK: docker-compose.yml has zero unauthorized ${VAR:-default} forms (allowlist size = 4, all allowlist entries justified inline in the YAML and in the test source)
--- PASS: TestDevComposeContract_NoUnauthorizedDefaultFallbacks (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.008s
```

**Interpretation (GREEN):** restoration returns the test to PASS. RED→GREEN proof complete: the new lint test would catch a real-world regression (any one of the 10 converted vars accidentally reverted to `:-` form) and pinpoints the violating line, var, and fix path.

## Audit Evidence

### Audit Evidence

<!-- audit-marker: blocking-gate-reviewed -->

#### Cross-package smoke

**Claim Source:** executed

```text
$ ./smackerel.sh test unit --go
... (full go test ./... output across cmd/* and internal/* packages — final marker shown) ...
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

**Interpretation:** the full Go unit suite (every `internal/*` and `cmd/*` package, plus `tests/e2e/agent`, `tests/integration`, `tests/stress/readiness`) runs clean. No test in any other package regresses as a side effect of the BUG-029-003 changes. The new `TestDevComposeContract_*` tests are bounded to `internal/deploy/dev_compose_default_fallback_test.go` and target a different live file (`docker-compose.yml`) than the pre-existing `TestComposeContract_*` tests in `internal/deploy/compose_contract_test.go` (which target `deploy/compose.deploy.yml`). Cross-package blast radius is zero by construction.

#### Canary suite

**Claim Source:** executed

The targeted `^TestComposeContract|^TestVulnGateContract|^TestBundleHashContract` regex run (Validation Evidence > Targeted prod-compose canary suite) shows:

- `TestComposeContract_LiveFile` — PASS (positive canary; `deploy/compose.deploy.yml` complies with spec 042 / spec 049 contracts).
- `TestComposeContract_AdversarialLiteralBind` — PASS (BUG-042 spec 042 canary).
- `TestComposeContract_AdversarialInfraHasPorts` — PASS (BUG-042 spec 042 canary).
- `TestComposeContract_AdversarialMultiPortsBypass` — PASS (BUG-042-001 canary).
- `TestComposeContract_AdversarialMLMultiPortsBypass` — PASS (BUG-042-001 canary).
- `TestComposeContract_AdversarialNetworkModeHostBypass` (5 sub-cases) — all PASS (BUG-042-002 canary).
- `TestComposeContract_AdversarialOllamaLiteralBind` (2 sub-cases) — both PASS (BUG-042-003 canary).
- `TestComposeContract_AdversarialDefaultFallbackBind` (3 sub-cases) — all PASS (BUG-042-004 canary).
- `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` (2 sub-cases) — both PASS (BUG-042-005 canary).
- `TestVulnGateContract_LiveFile` + 10 adversarial sub-tests — all PASS (spec 047 vuln-gate contract canary).
- `TestBundleHashContract_LiveFile` + 4 adversarial sub-tests — all PASS (BUG-047-001 bundle-hash contract canary).

**Interpretation:** zero canary regression. The new dev-compose test is purely additive against a different file (`docker-compose.yml`) than any pre-existing contract test, so the canaries cannot regress as a side effect.

#### Regression Evidence

**Claim Source:** executed

The new `TestDevComposeContract_*` (4 functions + sub-cases) tests are **persistent in-tree adversarial Go unit tests** that run automatically on:

- every developer `./smackerel.sh test unit --go` invocation (local pre-push)
- every CI `unit-tests` job (the `unit-tests` matrix invokes `go test ./...` against the full module)
- every `go test ./internal/deploy/...` invocation

A future regression that reverts any one of the 10 converted vars in `docker-compose.yml` to `${X:-default}` form will FAIL RED at the `unit-tests` gate before merge with the named-var unauthorized-list message + HL-RESCAN-012 breadcrumb. This is verified by the RED→GREEN proof under Test Evidence.

The dev-compose contract surface is a **static-file invariant**: there is no daemon, no container, no network, no concurrency, no cross-process state involved in the lint. The Go test suite IS the contract enforcement layer. Compose substitution evidence is supplementary execution-time proof that the SST plumbing is correct end-to-end.

#### Constraint Adherence

**Claim Source:** executed

- **Change boundary:** confirmed by `git diff --stat HEAD -- scripts/commands/config.sh docker-compose.yml` (2 files modified, 72 insertions, 10 deletions) plus `git status --porcelain internal/deploy/dev_compose_default_fallback_test.go` (1 new file, untracked, 270 lines). Plus the 7 BUG-029-003 packet artifacts under `specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/`. No production runtime Go/Python code modified, no `internal/deploy/compose_contract_test.go` edited, no `deploy/compose.deploy.yml` edited, no `config/smackerel.yaml` edited, no `scripts/lib/runtime.sh` edited, no `.github/workflows/*` edited (HL-RESCAN-011 is a separate sprint item), no foreign-owned `specs/**` directory edited.
- **No defaults regression:** Gate G028 NO-DEFAULTS / fail-loud SST policy is strengthened by this fix — the test now mechanically locks compliance against future drift on the dev compose file. 10 of 14 violations converted to fail-loud forms; the remaining 4 are explicitly allowlisted with inline justification + per-var allowlist gate in the lint test. The dev-compose Gate G028 surface now matches the prod-compose Gate G028 surface (both are locked by static-file lint tests).
- **PII scan:** no real hostnames, no real IPs, no real Tailscale identifiers, no real Linux usernames in any of the seven packet artifacts (the evidence captures use `~/` for home paths per the repo policy). The only address strings are loopback / generic SST substitution forms.
- **Bubbles.devops mode discipline:** all bug-local artifacts (this packet) authored; foreign-owned parent-spec files (`specs/029-devops-pipeline/spec.md`, `design.md`, `scopes.md`, `state.json`, `uservalidation.md`, `report.md`) NOT edited.
- **Single-bug-scope discipline:** the change addresses HL-RESCAN-012 (P3) only. The remaining home-lab readiness re-scan findings (HL-RESCAN-011 — CI workflow; HL-RESCAN-013 — ml/app/auth.py module-import-time fail-loud; HL-RESCAN-014 — cmd/core/helpers.go dead helper functions) are tracked under their own per-finding bug-packets in the parent home-lab-readiness-rescan-2026-05-14 sweep.

## Verdict (DevOps)

**Status: SHIP_IT** — see [uservalidation.md](uservalidation.md).
