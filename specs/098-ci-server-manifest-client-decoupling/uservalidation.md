# User Validation — Spec 098 CI Server-Manifest / Client-Build Decoupling

**Status:** done · **Workflow mode:** full-delivery

## Context

This validates that the CI server deploy manifest is decoupled from the mobile
client build, so a missing Android signing secret can no longer block a home-lab
SERVER deploy — while preserving Build-Once-Deploy-Many client integrity on
actual releases.

## Checklist

- [x] **Problem is real and verified** — `publish-build-manifest` hard-depended
  on `build-clients`, which fail-fasts on a missing `ANDROID_KEYSTORE_BASE64`;
  specs 087/088/089 `state.json` confirm "build-clients ✗ → publish-build-manifest
  SKIPPED → build-manifest NOT published". (baseline acceptance)
- [x] **Non-release server deploy is unblocked** — on a non-release push with no
  Android secret, `build-clients` is skipped and `publish-build-manifest` still
  publishes a server-only manifest (core + ml + bundles + chrome-bridge).
- [x] **Release client integrity preserved** — a tag push (or explicit
  `build_clients: true` dispatch) still builds, signs, pushes-by-digest, and pins
  the AAB + APK; a client failure on a release still blocks the manifest.
- [x] **Server-only manifest is promotable** — matches the existing
  `local-build-manifest` clients-absent shape that `promote.sh` already consumes
  (core + ml + per-env bundle ref + sha256); no parser change.
- [x] **knb-side contract documented, not changed** — a server-only manifest does
  not contract the android platform, so `E025-CLIENT-MANIFEST-NO-DIGEST` has
  nothing to fail-close on; the operator-private knb adapter is out of repo scope.
- [x] **Drift-protected** — the lockstep contract test asserts the release-gate,
  skip-tolerance, and success-gate, with adversarial proof that a non-release
  manifest is accepted without android digests AND a release requires them.
- [x] **Gates green** — contract tests 12/12 (`SCOPED_UNIT_EXIT=0`),
  `CHECK_EXIT=0`, `LINT_EXIT=0`. (see report.md)
- [ ] **Operator confirms the real CI run (operator-observable confirmation, NOT a
  098 blocker)** — on a *green* non-release push the CI `build` workflow should
  publish `build-manifest-<sha>.yaml` while `build-clients` is skipped. This is
  currently un-confirmable because the `main` build is RED at a **foreign,
  pre-existing** gate (`build-images` → "Trivy vulnerability scan — smackerel-ml",
  run 27865311625), upstream of and unrelated to spec 098. Spec 098 is certified
  on the in-repo drift-detector contract proof (established pattern for CI-config
  specs); this live confirmation follows once the foreign Trivy-ml gate is
  resolved separately (see report.md → Discovered Issues).

## Sign-off

Engineering complete, committed (spec(098) `f7148da2`), pushed, and certified
`done` on the in-repo drift-detector proof (12/12 `internal/deploy` contract
tests incl. 3 adversarial workflow-shape probes). The `build.yml` conditional
gating (release-gate + skip-tolerance + success-gate) is asserted against the
live workflow file. The one residual item — operator confirmation of a green
live CI run publishing the server-only manifest — is an operator-observable
confirmation currently blocked by a foreign, pre-existing `build-images` Trivy-ml
failure (NOT caused by spec 098); it is dispositioned as a separate ops/security
concern in report.md → Discovered Issues and does not gate 098's certification.
