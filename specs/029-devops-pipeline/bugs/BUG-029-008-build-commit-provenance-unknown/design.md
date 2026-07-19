# Design: BUG-029-008 — Locally-built images stamp `SMACKEREL_COMMIT=unknown`

## Current Truth (Phase 0.55 — solution-blind provenance probe)

**HEAD SHA:** `1bfd18a0f357` (current at reconcile time 2026-07-19)
**Fix commit:** `0f2fb517` ("fix(smackerel): redteam F1/F2/F6/F7 health + provenance + LLM-resilience triage")
**Probed surface:** `scripts/commands/config.sh` build-metadata resolution, `docker-compose.yml`
`COMMIT_HASH` build-arg wiring, `Dockerfile` / `ml/Dockerfile` ARG/LABEL/ldflags wiring, and the two
committed regression tests.

### Findings — the defect and its blast radius

- The running prod core + ml images (redteam observation, live `<deploy-host>`) carried OCI
  `org.opencontainers.image.revision = unknown` and core `SMACKEREL_COMMIT = unknown`, so the app's
  `commit_hash` reported `unknown`. The running artifact was **not self-identifying**.
- The Dockerfile wiring was already CORRECT: `ARG COMMIT_HASH=unknown`,
  `LABEL org.opencontainers.image.revision="${COMMIT_HASH}"`, `-ldflags "… -X main.commitHash=${COMMIT_HASH}"`
  (mirrored in `ml/Dockerfile`).
- `docker-compose.yml` was already CORRECT: it passes `COMMIT_HASH: ${SMACKEREL_COMMIT:?…}` (fail-loud)
  from the generated env file (lines 83, 180).
- The defect was in `scripts/commands/config.sh`: build-metadata resolution had exactly two sources —
  (1) shell env (CI exports `SMACKEREL_COMMIT`), (2) the literal `"unknown"`. There was **no
  derivation from the git working tree**. CI images were fine (CI exports the SHA), but the
  **local-operator / `<deploy-host>` build path builds ON the host with no CI env**, so
  `SMACKEREL_COMMIT` fell through to `"unknown"`.

### Findings — the committed fix (present at HEAD)

`scripts/commands/config.sh` (lines ~2202-2224 at HEAD):

```bash
if [[ -z "${SMACKEREL_COMMIT+set}" ]]; then
  if _sm_git_sha="$(git -C "$REPO_ROOT" rev-parse --short=12 HEAD 2>/dev/null)"; then
    if [[ -n "$(git -C "$REPO_ROOT" status --porcelain 2>/dev/null)" ]]; then
      SMACKEREL_COMMIT="${_sm_git_sha}-dirty"
    else
      SMACKEREL_COMMIT="$_sm_git_sha"
    fi
    unset _sm_git_sha
  else
    SMACKEREL_COMMIT="unknown"
  fi
fi
```

The arm derives the source SHA from the working tree when CI has not exported it, marks a dirty tree,
and falls back to `unknown` ONLY outside a git checkout. Shell-env / CI precedence is preserved (the
arm runs only when `SMACKEREL_COMMIT` is unset).

**Conclusion:** the fix is a build-wiring change, already committed in `0f2fb517`, present at HEAD, and
proven by a committed config-generator regression test. The reconcile path is: verify the source fix
read-only, re-run the static regression FRESH this session, author the full bugfix-fastlane packet,
and certify to `done` — with the RUNNING-image restamp routed to `bubbles.devops` as non-gating.

## Root Cause Analysis

The build-metadata SST resolver had no git-working-tree source. Provenance came ONLY from a CI shell
export or the literal `"unknown"`. Because the `local-operator` (`<deploy-host>`) build is the prod
build path and runs ON the host with no CI environment, every locally-built image was stamped
`unknown` for `org.opencontainers.image.revision`, the core `commitHash` ldflag, and the reported
`commit_hash`. The image could not be traced back to its source revision.

## Design Decisions

### DD-1 — Adopt the sibling bugfix-fastlane packet structure

Mirror the 8-artifact bugfix-fastlane layout used by the certified sibling `BUG-026-007` (a real
source-fix reconcile in this same session): `bug.md`, `spec.md`, `design.md`, `scopes.md`,
`scenario-manifest.json`, `report.md`, `state.json`, `uservalidation.md`. Use the `## Scope N: <Name>`
colon-format for the traceability guard's DoD fidelity.

### DD-2 — Fix location: the SST generator, not the Dockerfile

The Dockerfile ARG/LABEL/ldflags wiring and the compose `COMMIT_HASH` build-arg were already correct.
The ONLY missing link was a git-working-tree source in `scripts/commands/config.sh`. The fix adds that
one derivation arm — the smallest change that makes the local-operator path self-identifying while
leaving the (correct) downstream contract untouched.

### DD-3 — Precedence: shell-env / CI wins (fail-safe, not fail-loud)

Provenance derivation is **best-effort**, not fail-loud: it fills a gap that would otherwise be
`unknown`. It runs only when `SMACKEREL_COMMIT` is unset, so a CI export always wins, and it falls
back to `unknown` (never a fabricated SHA) outside a git checkout so config generation is never
blocked on a missing `.git`. This is distinct from the runtime NO-DEFAULTS policy (which governs
required runtime secrets/config), because a source SHA is provenance metadata, not runtime behavior —
`unknown` is a truthful "provenance unavailable" marker, not a silent functional default.

### DD-4 — Static verification, no full self-hosted image build

Per the finding ("do not run a full self-hosted image build"), the fix is proven by a
config-generator regression that runs the generator twice against the live `config/smackerel.yaml`
and asserts the emitted `SMACKEREL_COMMIT`:

- `scripts/commands/config_build_commit_provenance_test.sh` — Sub-test 1 (unset ⇒ real 12-hex SHA,
  regex `^[0-9a-f]{12}(-dirty)?$`); Sub-test 2 (exported sentinel ⇒ preserved).
- `internal/config/sst_loader_build_commit_provenance_test.go` — Go driver
  `TestSSTLoader_BuildCommitProvenance_BUG029008` that runs the shell test under
  `./smackerel.sh test unit --go`, with `GIT_CONFIG_* safe.directory=*` so `git rev-parse` works
  under the Docker test surface.

The dirty-suffix (FR-03) and non-git fallback (FR-04) arms are covered by Sub-test 1's
`(-dirty)?` regex tolerance and the `else` fallback branch respectively; they are documented code
behaviors rather than separately-asserted scenarios, keeping every declared scenario mapped to a
concrete asserting sub-test.

### DD-5 — Adversarial proof (the test detects a regression)

The regression is adversarial: reverting the git-derivation arm in `scripts/commands/config.sh` back
to the pre-fix `SMACKEREL_COMMIT="unknown"` makes Sub-test 1 FAIL (dev.env reverts to
`SMACKEREL_COMMIT=unknown`, which does not match the `^[0-9a-f]{12}(-dirty)?$` regex). This is the
RED capture recorded in the original bug.md; the current-session GREEN run (Sub-test 1 emitted the
real HEAD SHA `1bfd18a0f357`) is the paired GREEN.

### DD-6 — Certification boundary: source verified read-only, packet is spec-only

The effective source fix is already committed (`0f2fb517`). This reconcile packet verifies the fix
read-only and does NOT re-change any source file. All packet mutations land under the BUG-029-008 bug
folder. The G093 delivery-implementation-delta requirement is satisfied by the G053-compatible Code
Diff Evidence in `report.md`, which cites the real non-spec delivery files
(`scripts/commands/config.sh`, `scripts/commands/config_build_commit_provenance_test.sh`,
`internal/config/sst_loader_build_commit_provenance_test.go`) shipped in `0f2fb517`.

### DD-7 — Running-image restamp is a NON-GATING devops step

The wiring change affects only **future builds**. The already-running prod images keep
`revision=unknown` until they are rebuilt (`./smackerel.sh config generate` + image build) and
redeployed. That rebuild + signed redeploy + live `docker inspect` re-check is routed to
`bubbles.devops` as a non-gating operational confirmation (`redeployRequired: true`). It does NOT
block certification: the fix is source-committed and unit-proven, and the redeploy is an operational
apply, not a code change.

## Affected Files

**Delivery (already committed in `0f2fb517`, verified read-only — NOT re-changed here):**

- `scripts/commands/config.sh` — the git-derivation arm (+21 lines in `0f2fb517`).
- `scripts/commands/config_build_commit_provenance_test.sh` — the config-generator regression (+118).
- `internal/config/sst_loader_build_commit_provenance_test.go` — the Go driver (+64).

**Already-correct downstream contract (unchanged — never the defect):**

- `docker-compose.yml` — `COMMIT_HASH: ${SMACKEREL_COMMIT:?…}` fail-loud build-arg (lines 83, 180).
- `Dockerfile` / `ml/Dockerfile` — `ARG COMMIT_HASH` → `LABEL org.opencontainers.image.revision` +
  `-ldflags "… -X main.commitHash=${COMMIT_HASH}"`.

**Certification packet (this reconcile):**

- `specs/029-devops-pipeline/bugs/BUG-029-008-build-commit-provenance-unknown/` — all 8 artifacts.

## Rollback

Pure git revert of the certification commits restores the prior `fixed_in_repo` stuck status; zero
runtime impact (the source fix in `0f2fb517` is independent of this packet and is not reverted).
