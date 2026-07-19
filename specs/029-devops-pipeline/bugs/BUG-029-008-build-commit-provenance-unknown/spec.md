# Spec: BUG-029-008 — Locally-built images stamp `SMACKEREL_COMMIT=unknown` (no source-SHA provenance)

**Parent Spec:** 029-devops-pipeline
**Discovered:** 2026-07-08 (redteam adversarial interrogation of the LIVE smackerel prod deployment on `<deploy-host>`, finding F6)
**Mode:** bugfix-fastlane (real source fix already committed; reconcile + certify)
**Release Train:** mvp

## Use Cases

- **UC-01 — A locally-built (local-operator) image is self-identifying.** When the repo owner builds
  smackerel-core / smackerel-ml ON the deploy host (the `local-operator` path, no CI environment),
  `./smackerel.sh config generate` MUST stamp the built image with the real source SHA of the git
  working tree — via OCI `org.opencontainers.image.revision`, the core `commitHash` ldflag, and the
  app's reported `commit_hash` — so an operator inspecting a running container can trace it back to
  the exact source revision, instead of the opaque `unknown` the redteam observed live.

- **UC-02 — CI builds keep their explicit source SHA.** When `.github/workflows/ci.yml` (or any
  future release pipeline) exports `SMACKEREL_COMMIT=<sha>` before `config generate`, that explicit
  export MUST win unchanged — the git-derivation must NEVER override a value the build environment
  already set.

- **UC-03 — A locally-modified build never claims a clean SHA.** When the git working tree is dirty
  at `config generate` time, the derived provenance MUST carry a `-dirty` suffix so a modified build
  is not misrepresented as a pristine tagged revision.

- **UC-04 — A non-git source (tarball) still generates.** When `config generate` runs outside a git
  checkout (e.g. an unpacked source tarball with no `.git`), the resolver MUST fall back to the
  literal `unknown` rather than fail — provenance is best-effort, config generation is not blocked.

## Functional Requirements

- **FR-01 — Git-derive `SMACKEREL_COMMIT` when unset.** `scripts/commands/config.sh` MUST, when
  `SMACKEREL_COMMIT` is unset in the environment, derive the value from the git working tree via
  `git -C "$REPO_ROOT" rev-parse --short=12 HEAD` and emit it into `config/generated/<env>.env`.

- **FR-02 — Preserve shell-env / CI precedence.** The git-derivation arm MUST run ONLY when
  `SMACKEREL_COMMIT` is unset (`[[ -z "${SMACKEREL_COMMIT+set}" ]]`), so an exported
  `SMACKEREL_COMMIT=<sha>` from CI is passed through verbatim.

- **FR-03 — Mark a dirty tree.** When `git status --porcelain` reports a dirty working tree, the
  derived value MUST carry a `-dirty` suffix (`<sha>-dirty`).

- **FR-04 — Fall back to `unknown` only outside a git checkout.** If `git rev-parse` fails (no git
  repo), the value MUST fall back to the literal `unknown` — never a fabricated SHA.

- **FR-05 — Preserve the downstream wiring.** The already-correct downstream contract MUST remain
  intact: `docker-compose.yml` passes `COMMIT_HASH: ${SMACKEREL_COMMIT:?…}` (fail-loud), and the
  `Dockerfile` / `ml/Dockerfile` `ARG COMMIT_HASH` → `LABEL org.opencontainers.image.revision` +
  `-ldflags "… -X main.commitHash=${COMMIT_HASH}"` wiring is unchanged (it was never the defect).

- **FR-06 — Automated static regression.** A config-generator regression test MUST prove the
  behavior without a full self-hosted image build: (a) unset ⇒ a real 12-hex SHA (optionally
  `-dirty`), never `unknown`; (b) an exported sentinel ⇒ preserved verbatim.

## Acceptance Criteria

- **AC-01** — With `SMACKEREL_COMMIT` UNSET, `./smackerel.sh config generate` emits a real 12-hex
  git SHA (matching `git rev-parse --short=12 HEAD`, optionally `-dirty`) into
  `config/generated/dev.env::SMACKEREL_COMMIT` — never the literal `unknown`.
- **AC-02** — With `SMACKEREL_COMMIT=<sentinel>` exported, `config generate` emits that sentinel
  verbatim into `dev.env` (CI / shell-env override preserved).
- **AC-03** — `./smackerel.sh test unit --go --go-run TestSSTLoader_BuildCommitProvenance_BUG029008`
  passes GREEN (both sub-tests) with exit 0.
- **AC-04** — `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix
  scripts/commands/config_build_commit_provenance_test.sh` reports an adversarial signal with 0
  violations (exit 0).
- **AC-05** — `./smackerel.sh check` exits 0 (config in sync with SST, env-file drift OK).
- **AC-06** — `./smackerel.sh lint` exits 0 (no shellcheck finding on the changed `config.sh`).
- **AC-07** — The git-derivation arm is present at HEAD in `scripts/commands/config.sh` and the two
  regression test files are committed (Code Diff Evidence cites fix commit `0f2fb517`).
- **AC-08** — `bash .github/bubbles/scripts/state-transition-guard.sh <bug-dir>` returns exit 0 at
  `done`; `bash .github/bubbles/scripts/artifact-lint.sh <bug-dir>` returns PASSED.
- **AC-09** — The certification packet touches ONLY paths under the BUG-029-008 bug folder; the
  source fix is verified read-only and NOT re-changed.
- **AC-10** — The live "running images become self-identifying" outcome is certified on the source
  fix + the static regression test; the fresh rebuild + signed redeploy that stamps the RUNNING
  images is routed to `bubbles.devops` as a NON-GATING operational step (redeploy-required).
