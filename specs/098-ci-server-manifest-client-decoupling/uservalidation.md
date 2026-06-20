# User Validation — Spec 098 CI Server-Manifest / Client-Build Decoupling

**Status:** in_progress · **Workflow mode:** full-delivery

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
- [ ] **Operator confirms the real CI run** — on the next non-release push the
  CI `build` workflow publishes `build-manifest-<sha>.yaml` while `build-clients`
  is skipped. (Requires a push, which this in-repo validation withholds; this is
  the only open item and the reason the spec is held at `in_progress`.)

## Sign-off

Engineering complete and proven in-repo. The single open checklist item is the
live CI confirmation, which requires a push that is intentionally withheld
(pushing would trigger the very client build this spec gates). Operator sign-off
on the live run promotes the spec to `done` together with the structured spec
commit (state-transition-guard Check 17).
