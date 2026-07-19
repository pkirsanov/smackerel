# User Validation: BUG-029-008 — Locally-built images stamp `SMACKEREL_COMMIT=unknown`

**Closure status:** Fixed & Verified (source fix committed in `0f2fb517`; reconciled + certified this session)

## User-facing impact

- **Operators / DevOps:** After the next rebuild + redeploy, a locally-built (local-operator /
  `<deploy-host>`) smackerel-core / smackerel-ml image is **self-identifying**: `docker inspect`
  shows `org.opencontainers.image.revision = <source-sha>` (optionally `-dirty`) instead of the
  opaque `unknown` the redteam observed, and the app's `commit_hash` reports the real source
  revision. CI builds are unchanged (their explicit `SMACKEREL_COMMIT` export still wins). Config
  generation for a non-git source tarball still works (falls back to `unknown`, never blocked).
- **Auditors:** The build-provenance chain (redteam F6) is closed at the SST-generator level. A
  running image can be traced back to the exact source revision it was built from. The fix is
  proven by a committed config-generator regression test (no full self-hosted image build required).
- **End users:** Not applicable — this is internal DevOps build provenance with no end-user surface.

## Acceptance

- AC-01..AC-10 from `spec.md` all pass; full evidence captured in `report.md`.
- The certification packet is scoped to the BUG-029-008 bug folder only; the source fix is verified
  read-only and not re-changed.

## Sign-off

Reconcile + certification (bugfix-fastlane, parent-expanded by `bubbles.iterate`) terminates with the
bug `done` on disk, `state-transition-guard` passing at `done`, and the fresh rebuild + signed
redeploy + `docker inspect` re-check routed to `bubbles.devops` as a non-gating operational step. No
further in-repo follow-up work is required for BUG-029-008.

## Checklist

- [x] Unset `SMACKEREL_COMMIT` ⇒ `dev.env` carries a real 12-hex git SHA (Sub-test 1 emitted `1bfd18a0f357` = HEAD).
- [x] Exported `SMACKEREL_COMMIT` sentinel ⇒ preserved verbatim (Sub-test 2 emitted `cafef00dba11`).
- [x] `./smackerel.sh test unit --go --go-run TestSSTLoader_BuildCommitProvenance_BUG029008` GREEN (exit 0).
- [x] `regression-quality-guard.sh --bugfix` reports an adversarial signal, 0 violations (exit 0).
- [x] `./smackerel.sh check` exit 0; `./smackerel.sh lint` exit 0.
- [x] The git-derivation arm is present at HEAD in `scripts/commands/config.sh`; the two regression test files are committed (`0f2fb517`).
- [x] The already-correct downstream wiring (`docker-compose.yml`, `Dockerfile`, `ml/Dockerfile`) is unchanged.
- [x] Zero source files re-changed by this packet; the fix is verified read-only. `format --check` names only the pre-existing unrelated `internal/config/release_trains_contract_test.go` (outside this boundary).
- [x] Bug marked FIXED & VERIFIED in bug.md; `state.json` `status` = `done`, `certification.status` = `done`.
- [x] Running-image restamp (rebuild + signed redeploy + `docker inspect`) routed to `bubbles.devops` as a non-gating operational step (`redeployRequired: true`).
