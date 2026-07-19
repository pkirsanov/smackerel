# Report: BUG-029-008 — Git-derive `SMACKEREL_COMMIT` for local-operator builds

> **Status:** Fixed and certified `done` (2026-07-19). `scripts/commands/config.sh` now derives
> `SMACKEREL_COMMIT` from the git working tree when CI has not exported it, so a locally-built
> (local-operator / `<deploy-host>`) image is self-identifying (`org.opencontainers.image.revision`
> + core `commitHash` ldflag + reported `commit_hash`) instead of the opaque `unknown` the redteam
> observed live. The full bugfix-fastlane specialist pipeline executed this session with fresh
> evidence (provenance unit lane GREEN, regression guard `0 violations`, check/lint clean). The
> `state-transition-guard` certifies the bug to `done`. This is a build-wiring change; a fresh
> rebuild + signed redeploy + `docker inspect` re-check that restamps the RUNNING images is routed to
> `bubbles.devops` as a non-gating operational confirmation. Nothing was built, published, deployed,
> or pushed by this certification packet beyond the scoped local bug-folder commits.

## Scenario-First TDD — RED → GREEN Ordering (Gate G060)

**Claim Source:** executed (prior-session RED capture + current-session GREEN re-run)

Scenario-first evidence for the config-generator provenance regression (`BUG-029-008-SCN-001/002`):

- **RED stage — failing proof first.** With the git-derivation arm stashed out of
  `scripts/commands/config.sh` (reverted to the pre-fix `SMACKEREL_COMMIT="unknown"`), the
  provenance test FAILS because `dev.env` emits the literal `unknown`, which does not match the
  Sub-test 1 regex `^[0-9a-f]{12}(-dirty)?$`. This is the RED capture recorded in the original
  `bug.md` (`./smackerel.sh test unit --go` with the fix stashed). See "Test Evidence → RED" below.
- **GREEN stage — passing proof after the fix.** With the git-derivation arm in place (the shipped
  state at HEAD `1bfd18a0f357`), the provenance test PASSES: Sub-test 1 emits the real HEAD SHA
  `1bfd18a0f357`, Sub-test 2 preserves the exported sentinel `cafef00dba11`, and the `internal/config`
  package is `ok`. See "Current-Session Re-Verification → Fresh Provenance Unit Lane" and "Test
  Evidence → GREEN" below.

### Summary

The running prod core + ml images (redteam observation, live `<deploy-host>`) carried OCI
`org.opencontainers.image.revision = unknown` and core `SMACKEREL_COMMIT = unknown`, so the app's
`commit_hash` reported `unknown` — the running artifact was **not self-identifying**. The Dockerfile
ARG/LABEL/ldflags wiring and the compose `COMMIT_HASH` build-arg were already correct; the defect was
that `scripts/commands/config.sh` had no git-working-tree source for the build SHA. CI images were
fine (CI exports `SMACKEREL_COMMIT`), but the **local-operator / `<deploy-host>` build path builds ON
the host with no CI env**, so `SMACKEREL_COMMIT` fell through to the literal `"unknown"`. Fixed by a
git-derivation arm in `config.sh` that runs ONLY when `SMACKEREL_COMMIT` is unset — deriving
`git rev-parse --short=12 HEAD` (with a `-dirty` suffix on a dirty tree, and an `unknown` fallback
only outside a git checkout), while preserving CI's shell-env precedence.

### Root Cause

See [design.md](design.md) → "Root Cause Analysis" and the live `<deploy-host>` observation in
[bug.md](bug.md). The build-metadata SST resolver had exactly two sources — a CI shell export or the
literal `"unknown"` — and no derivation from the git working tree. Because the `local-operator`
build is the prod build path and runs ON the host with no CI environment, every locally-built image
was stamped `unknown` for `org.opencontainers.image.revision`, the core `commitHash` ldflag, and the
reported `commit_hash`, so it could not be traced back to its source revision.

### Changes

| File | Change |
|------|--------|
| `scripts/commands/config.sh` | ADDED — when `SMACKEREL_COMMIT` is unset, derive `git -C "$REPO_ROOT" rev-parse --short=12 HEAD`; `-dirty` suffix when `git status --porcelain` is non-empty; fall back to `unknown` only when `git rev-parse` fails (no git checkout). Gated by `[[ -z "${SMACKEREL_COMMIT+set}" ]]` so a CI export wins. (+21 in `0f2fb517`) |
| `scripts/commands/config_build_commit_provenance_test.sh` | ADDED — config-generator regression: Sub-test 1 (unset ⇒ real 12-hex SHA via `env -u SMACKEREL_COMMIT`), Sub-test 2 (exported sentinel ⇒ preserved). Output isolated via `SMACKEREL_GENERATED_DIR` temp dir. (+118 in `0f2fb517`) |
| `internal/config/sst_loader_build_commit_provenance_test.go` | ADDED — Go driver `TestSSTLoader_BuildCommitProvenance_BUG029008` that runs the shell test under `./smackerel.sh test unit --go` (with `GIT_CONFIG_* safe.directory=*` for the Docker test surface). (+64 in `0f2fb517`) |
| `docker-compose.yml` | UNCHANGED — already passes `COMMIT_HASH: ${SMACKEREL_COMMIT:?…}` fail-loud (lines 83, 180). Never the defect. |
| `Dockerfile` / `ml/Dockerfile` | UNCHANGED — already `ARG COMMIT_HASH` → `LABEL org.opencontainers.image.revision` + `-ldflags "… -X main.commitHash=${COMMIT_HASH}"`. Never the defect. |

## Test Evidence

> Captured from ACTUAL `./smackerel.sh test unit --go` runs (Docker go-tooling container installs the
> real toolchain, then `go test ./...` runs the provenance driver which invokes the config-generator
> shell test). Home paths scrubbed to `<repo-root>` per terminal-discipline / pii-scan.

### Pre-Fix / adversarial (MUST FAIL) — RED

**Claim Source:** executed — prior-session capture (original `bug.md`), with the git-derivation arm
stashed out of `scripts/commands/config.sh` (reverted to `SMACKEREL_COMMIT="unknown"`); the
provenance test fails because `dev.env` emits the literal `unknown`:

```text
    sst_loader_build_commit_provenance_test.go:61: BUG-029-008 build-commit provenance shell test failed: exit status 1
        FAIL: dev.env SMACKEREL_COMMIT is not a real git SHA — actual: 'unknown'
--- FAIL: TestSSTLoader_BuildCommitProvenance_BUG029008 (6.97s)
FAIL    github.com/smackerel/smackerel/internal/config  7.013s
___GO_RED_EXIT=1___
```

### Post-Fix (MUST PASS) — GREEN

**Claim Source:** executed (this session) — git-derivation arm in place at HEAD `1bfd18a0f357`:

```text
=== RUN   TestSSTLoader_BuildCommitProvenance_BUG029008
    sst_loader_build_commit_provenance_test.go:63: BUG-029-008 build-commit provenance shell test output:
        --- Sub-test 1 (core): SMACKEREL_COMMIT unset -> dev.env carries a real git SHA ---
        PASS: dev.env SMACKEREL_COMMIT=1bfd18a0f357 (real source SHA, self-identifying image)
        --- Sub-test 2 (precedence): exported SMACKEREL_COMMIT wins (CI/shell-env override) ---
        PASS: dev.env SMACKEREL_COMMIT=cafef00dba11 (shell-env / CI override preserved)

        All BUG-029-008 build-commit provenance sub-tests passed
--- PASS: TestSSTLoader_BuildCommitProvenance_BUG029008 (11.23s)
PASS
ok      github.com/smackerel/smackerel/internal/config  11.239s
```

The Sub-test 1 emitted SHA `1bfd18a0f357` matches `git rev-parse --short=12 HEAD` exactly — proving
the git-derivation is live and correct. Sub-test 2 preserved the exported sentinel `cafef00dba11`,
proving CI precedence is intact.

### Bailout scan (no silent-pass patterns in the regression tests)

**Claim Source:** executed — the shell test asserts directly on the emitted
`dev.env::SMACKEREL_COMMIT` against `^[0-9a-f]{12}(-dirty)?$` (Sub-test 1) and `== "$SENTINEL"`
(Sub-test 2). The only `return`/`exit` hits are the `cleanup` trap (`exit "$rc"`) and the standalone
`REPO_ROOT` fallback — NOT test-body bailouts. No `if (…login…) return`, no `assert True`, no
conditional early-return short-circuits an assertion.

## Redeploy / Live-Verification Note (anti-fabrication)

This is a **build-wiring change to `scripts/commands/config.sh`**. It affects only **future builds**:
the value is baked into an image at `config generate` + build time. The already-running prod images
keep `revision=unknown` until they are rebuilt (`./smackerel.sh config generate` + image build) and
redeployed. The live "running images become self-identifying" outcome is a downstream operational
confirmation owned by `bubbles.devops` (non-gating): the mechanism itself is already both
live-observed (the committed redteam measurement — live images carried `revision=unknown` pre-fix)
and unit-proven (unset ⇒ real SHA, this session `1bfd18a0f357`). No build, deploy, host mutation, or
push was performed in this repo — scoped local bug-folder commits only.

<!-- bubbles:certifying-window-begin -->

## Current-Session Re-Verification — 2026-07-19

**Claim Source:** executed (this session)

This section re-runs the fast in-repo evidence lanes fresh in the current session to satisfy the
session-bound execution-evidence standard. The prior-session RED capture above is retained unchanged.
HEAD is `1bfd18a0f357`; the effective-fix commit is `0f2fb517` (git-derivation arm + both regression
test files).

### Fresh Provenance Unit Lane

**Executed:** `./smackerel.sh test unit --go --go-run TestSSTLoader_BuildCommitProvenance_BUG029008 --verbose`

```text
=== RUN   TestSSTLoader_BuildCommitProvenance_BUG029008
    sst_loader_build_commit_provenance_test.go:63: BUG-029-008 build-commit provenance shell test output:
        --- Sub-test 1 (core): SMACKEREL_COMMIT unset -> dev.env carries a real git SHA ---
        PASS: dev.env SMACKEREL_COMMIT=1bfd18a0f357 (real source SHA, self-identifying image)
        --- Sub-test 2 (precedence): exported SMACKEREL_COMMIT wins (CI/shell-env override) ---
        PASS: dev.env SMACKEREL_COMMIT=cafef00dba11 (shell-env / CI override preserved)

        All BUG-029-008 build-commit provenance sub-tests passed
--- PASS: TestSSTLoader_BuildCommitProvenance_BUG029008 (11.23s)
PASS
ok      github.com/smackerel/smackerel/internal/config  11.239s
___UNIT_GO_EXIT=0___
```

The `test unit --go` lane compiles the module and runs `go test ./...` filtered to the provenance
driver. The shell test runs the SST generator twice against the live `config/smackerel.yaml`: unset
⇒ a real 12-hex SHA (`1bfd18a0f357` = HEAD), exported sentinel ⇒ preserved (`cafef00dba11`).

### Fresh Adversarial Regression Guard

**Executed:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix --verbose scripts/commands/config_build_commit_provenance_test.sh internal/config/sst_loader_build_commit_provenance_test.go`

```text
============================================================
  BUBBLES REGRESSION QUALITY GUARD
  Repo: <repo-root>
  Timestamp: 2026-07-19T09:18:53Z
  Bugfix mode: true
============================================================

ℹ️  Scanning scripts/commands/config_build_commit_provenance_test.sh
✅ Adversarial signal detected in scripts/commands/config_build_commit_provenance_test.sh
ℹ️  Scanning internal/config/sst_loader_build_commit_provenance_test.go

============================================================
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 2
  Files with adversarial signals: 1
============================================================
___RQG_EXIT=0___
```

The shell regression carries the adversarial signal (its own documented adversarial proof: reverting
the git-derivation arm makes Sub-test 1 fail). 0 violations, no silent-pass bailout.

### Fresh Check

**Executed:** `./smackerel.sh check`

```text
$ ./smackerel.sh check
config-validate: <repo-root>/config/generated/dev.env.tmp.3960441 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
$ echo "Exit Code: $?"
Exit Code: 0
___CHECK_EXIT=0___
```

### Fresh Lint

**Executed:** `./smackerel.sh lint`

```text
$ ./smackerel.sh lint
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: web/extension/manifest.json
  OK: web/extension/manifest.firefox.json

=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/extension/background.js

=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
___LINT_EXIT=0___
```

The `lint` lane runs shellcheck over the shell surface (including the changed
`scripts/commands/config.sh`); the git-derivation arm carries no shellcheck finding (exit 0).

### Fresh Format Check

**Executed:** `./smackerel.sh format --check`

```text
$ ./smackerel.sh format --check
internal/config/release_trains_contract_test.go
$ echo "FORMAT_EXIT=$?"
___FORMAT_EXIT=1___
```

`format --check` names ONLY the pre-existing, unrelated `internal/config/release_trains_contract_test.go`
— a Go file last touched by the deploy-boundary commit `386a4e06` ("refactor(deploy): enforce generic
self-hosted boundary"), NOT by the BUG-029-008 fix commit `0f2fb517` (verified: `git show --stat
0f2fb517 | grep -c release_trains_contract_test.go` = 0), and OUTSIDE this bug's change boundary. The
three files this bug's fix touches (`scripts/commands/config.sh`,
`scripts/commands/config_build_commit_provenance_test.sh`,
`internal/config/sst_loader_build_commit_provenance_test.go`) are absent from the flagged set — the
provenance shell/Go files carry no formatter delta, and `config.sh` is a shell file (not gofmt's
surface). `RC=1` is caused solely by the repo-baseline Go file; the finding is routed, not fixed (see
"Discovered Issues"). This exactly mirrors the disposition of the certified sibling `BUG-026-007`.

### Code Diff Evidence

**Claim Source:** executed (this session, git-backed verification)

The delivery delta is the git-working-tree derivation of `SMACKEREL_COMMIT`, shipped in `0f2fb517`
("fix(smackerel): redteam F1/F2/F6/F7 health + provenance + LLM-resilience triage"). The delivery
files are `scripts/commands/config.sh`, `scripts/commands/config_build_commit_provenance_test.sh`, and
`internal/config/sst_loader_build_commit_provenance_test.go` — all non-spec source files (this
satisfies the G093 delivery-implementation-delta via G053-compatible Code Diff Evidence).

```text
$ git rev-parse --short=12 HEAD
1bfd18a0f357

$ git show --stat --format='commit %h  %s' 0f2fb517 -- scripts/commands/config.sh scripts/commands/config_build_commit_provenance_test.sh internal/config/sst_loader_build_commit_provenance_test.go
commit 0f2fb517  fix(smackerel): redteam F1/F2/F6/F7 health + provenance + LLM-resilience triage
 .../sst_loader_build_commit_provenance_test.go     |  64 +++++++++++
 scripts/commands/config.sh                         |  21 +++-
 .../config_build_commit_provenance_test.sh         | 118 +++++++++++++++++++++
 3 files changed, 202 insertions(+), 1 deletion(-)
```

The git-derivation arm is present at HEAD:

```text
$ grep -n '_sm_git_sha\|SMACKEREL_COMMIT="unknown"\|rev-parse --short=12' scripts/commands/config.sh
2213:  if _sm_git_sha="$(git -C "$REPO_ROOT" rev-parse --short=12 HEAD 2>/dev/null)"; then
2215:      SMACKEREL_COMMIT="${_sm_git_sha}-dirty"
2217:      SMACKEREL_COMMIT="$_sm_git_sha"
2219:    unset _sm_git_sha
2221:    SMACKEREL_COMMIT="unknown"
```

The already-correct downstream wiring is intact at HEAD:

```text
$ grep -n 'COMMIT_HASH' docker-compose.yml
83:        COMMIT_HASH: ${SMACKEREL_COMMIT:?must be set in env file (run ./smackerel.sh config generate)}
180:        COMMIT_HASH: ${SMACKEREL_COMMIT:?must be set in env file (run ./smackerel.sh config generate)}

$ grep -nE 'ARG COMMIT_HASH|org.opencontainers.image.revision|commitHash' Dockerfile
16:ARG COMMIT_HASH=unknown
20:     … go build … -ldflags="… -X main.commitHash=${COMMIT_HASH} …" -o /bin/smackerel-core ./cmd/core; \
22:     … go build … -ldflags="… -X main.commitHash=${COMMIT_HASH} …" -o /bin/smackerel-core ./cmd/core; \
41:ARG COMMIT_HASH=unknown
44:LABEL org.opencontainers.image.revision="${COMMIT_HASH}"
```

The fixed mechanism is therefore proven present at HEAD: the SST generator derives the source SHA
(`config.sh:2213-2221`), compose passes it fail-loud as the `COMMIT_HASH` build-arg (lines 83/180),
and the Dockerfile bakes it into the OCI revision label + the core `commitHash` ldflag (lines
16/20/22/41/44).

## Discovered Issues

| Date | Issue | Disposition | Reference |
|------|-------|-------------|-----------|
| 2026-07-19 | `./smackerel.sh format --check` names a pre-existing gofmt finding in `internal/config/release_trains_contract_test.go`, a Go file outside this bug's change boundary. | Repo-baseline gofmt finding not introduced by BUG-029-008 (last touched by the deploy-boundary commit `386a4e06`; `git show --stat 0f2fb517 \| grep -c` = 0). The three files this bug's fix touches are formatter-clean and absent from the flagged set. The Go file is left untouched (outside the change boundary). | report.md § Fresh Format Check |
| 2026-07-08 | The wiring change affects only future builds; the already-running prod images keep `revision=unknown` until rebuilt + redeployed. | Non-gating operational step routed to `bubbles.devops` (`redeployRequired: true`): a fresh `./smackerel.sh config generate` + image build + signed redeploy + `docker inspect` re-check. The mechanism is source-committed and unit-proven; the redeploy is an operational apply, not a code change. | report.md § Redeploy / Live-Verification Note |

## Parent-Expanded Specialist Phase Evidence

**Claim Source:** executed (this session, 2026-07-19)

Executed in-session by the bugfix-fastlane runner. This runtime lacks `runSubagent`, so each phase
owner was parent-expanded directly (`expandedBy: bubbles.iterate`) per the documented smackerel
precedent (BUG-047-004 / BUG-047-005 / BUG-026-007). Each phase below was genuinely executed; raw
output is captured inline or in the sections above.

### Phase: implement

The delivery delta (the git-working-tree derivation of `SMACKEREL_COMMIT`) is committed in `0f2fb517`
and confirmed present at HEAD `1bfd18a0f357` (see § Code Diff Evidence — the git-derivation arm at
`config.sh:2213-2221` and the two regression test files). Fresh compile/config integrity via
`./smackerel.sh check` returns clean (`CHECK_EXIT=0`, § Fresh Check). No source file was re-changed by
this reconcile packet.

### Phase: test

**Executed:** `./smackerel.sh test unit --go --go-run TestSSTLoader_BuildCommitProvenance_BUG029008 --verbose` (§ Fresh Provenance Unit Lane)

`--- PASS: TestSSTLoader_BuildCommitProvenance_BUG029008 (11.23s)`, `ok github.com/smackerel/smackerel/internal/config`,
`UNIT_GO_EXIT=0`. Sub-test 1 proves unset ⇒ the real HEAD SHA `1bfd18a0f357`; Sub-test 2 proves an
exported sentinel `cafef00dba11` is preserved.

### Phase: regression

**Executed:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix … config_build_commit_provenance_test.sh …` (§ Fresh Adversarial Regression Guard)

`RQG_EXIT=0`; adversarial signal detected, 0 violations / 0 warnings (2026-07-19T09:18:53Z). The
config-generator regression re-blocks a revert of the git-derivation arm: with the arm stashed,
Sub-test 1 goes RED (`SMACKEREL_COMMIT is not a real git SHA — actual: 'unknown'`).

### Phase: simplify

**Executed:** `./smackerel.sh check` (§ Fresh Check)

`CHECK_EXIT=0` (config in sync with SST, env-file drift OK, scenario-lint OK). The mechanism is a
single conditional arm inside the existing build-metadata resolution block — no new module, dead
branch, or duplication; it reuses the existing `$REPO_ROOT` and `SMACKEREL_GENERATED_DIR` seams.

### Phase: stabilize

The change is fail-safe (best-effort provenance), gated by `[[ -z "${SMACKEREL_COMMIT+set}" ]]` so a
CI export always wins, and falls back to `unknown` outside a git checkout so config generation is
never blocked. `./smackerel.sh check` confirms config is in sync with SST, so runtime stability is
unchanged at HEAD.

### Phase: security

The fix touches only the SST config-generator's build-metadata resolution and its regression test. It
adds no skip/force/insecure path, changes no secret or credential material, and introduces no new
network egress. `git rev-parse --short=12 HEAD` is a read-only local git query. Emitting the real
source SHA into an image label is provenance transparency (it improves auditability), not a secret
leak.

### Validation Evidence

**Executed:** `bash .github/bubbles/scripts/artifact-lint.sh <bug-dir>` + independent re-verification

The provenance unit lane is GREEN this session (`--- PASS`, `ok internal/config`, `UNIT_GO_EXIT=0`),
the adversarial regression guard passes (`0 violations`), `check` and `lint` are clean, and
`format --check` names only the pre-existing unrelated Go file. The git-derivation arm and the
downstream compose/Dockerfile wiring are git-verified present at HEAD (§ Code Diff Evidence). Artifact
lint passes and the `state-transition-guard` sweep returns a passing verdict at `done`. The live
"running images self-identify" outcome is certified on the source fix + the unit-proven mechanism,
with the fresh rebuild + `docker inspect` re-check routed to `bubbles.devops` as non-gating.

### Audit Evidence

**Executed:** delivery-delta + change-boundary audit (this session)

Independent audit (a separate authority from validate) confirms the runtime delivery delta is
confined to `scripts/commands/config.sh` + the two regression test files, all shipped in `0f2fb517`
and verified read-only here (NOT re-changed). The certification packet's own mutations are confined to
the BUG-029-008 bug folder — `git status --short` is clean before the packet commits and lists only
bug-folder paths in the staged diff. The already-correct `docker-compose.yml` / `Dockerfile` /
`ml/Dockerfile` wiring is untouched. The pre-existing `internal/config/release_trains_contract_test.go`
gofmt finding is outside the boundary and left alone. The change boundary declared in `scopes.md` and
`design.md` is respected. Audit verdict: pass.

### Completion Statement

The bug is reproduced (live `<deploy-host>` redteam observation: running images carried
`revision=unknown`), the git-working-tree derivation of `SMACKEREL_COMMIT` plus the config-generator
regression are implemented and committed (`0f2fb517`, present at HEAD `1bfd18a0f357`), and the full
bugfix-fastlane specialist pipeline (implement, test, regression, simplify, stabilize, security,
validate, audit) executed this session with fresh evidence (provenance unit lane GREEN with the real
HEAD SHA, regression guard `0 violations`, check/lint clean). The `state-transition-guard` certifies
the bug to `done`. The live "running images become self-identifying" confirmation on the rebuilt image
is owned by `bubbles.devops` as a non-gating operational step; the mechanism is already both
live-observed and unit-proven. Nothing was built, published, deployed, or pushed by this certification
packet beyond the scoped local bug-folder commits.
