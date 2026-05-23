# Scopes — BUG-049-001 — Prometheus external image contract drift

## Scope BUG-049-001-S1 — Contract update + regression test + doc cross-reference

**Status:** Done

### Work Items

1. Append `prom/prometheus:v2.55.1` entry (with `profile: monitoring` metadata
   and comments naming the SST key) to `deploy/contract.yaml::externalImages`.
2. Add `internal/deploy/external_images_contract_test.go` that parses both
   `deploy/contract.yaml` and `deploy/compose.deploy.yml`, classifies each
   compose service image, and asserts every SST-substituted external image is
   enumerated in `externalImages`. Include adversarial sub-tests:
   - Omit `prom/prometheus` from in-memory contract → expect failure naming
     the missing image and the SST key.
   - Omit `nats` from in-memory contract → expect failure naming `nats`.
3. Append a one-line cross-reference to `docs/Deployment.md` in the Monitoring
   Profile section pointing at `deploy/contract.yaml::externalImages`.
4. Run `./smackerel.sh test unit --go` and capture green output as evidence in
   `report.md`.

### Definition of Done

- [x] `deploy/contract.yaml::externalImages` lists `prom/prometheus:v2.55.1`
      with `profile: monitoring` metadata and a comment naming the SST key.
- [x] `internal/deploy/external_images_contract_test.go` exists and is green.
- [x] The new test contains at least one adversarial sub-test that demonstrates
      it would fail if `prom/prometheus` were omitted from the contract
      (`TestExternalImagesContract_AdversarialMissingPrometheus`).
- [x] The new test contains at least one adversarial sub-test that demonstrates
      it would fail if any other external image (e.g., `nats`) were omitted
      (`TestExternalImagesContract_AdversarialMissingNats`).
- [x] `docs/Deployment.md` Monitoring Profile section cross-references
      `deploy/contract.yaml::externalImages` as the canonical pin list.
- [x] `./smackerel.sh test unit --go` proxy (`go test ./internal/deploy/`) green;
      evidence captured in `report.md` (paths already relative).
- [x] `state-transition-guard.sh` PASS captured in `report.md` at commit time.

  Evidence: see `report.md` Step 5 (state-transition-guard PASS output captured at commit time after artifact lint and check-22 evidence accounting are green).

- [x] No changes touch unrelated WIP (spec 055 notification source / ntfy
      adapter). Path-limited `git add` only — staging captured in `report.md`.

  Evidence: `git diff --cached --name-status` filtered to BUG-049-001 packet plus `deploy/contract.yaml`, `internal/deploy/external_images_contract_test.go`, and `docs/Deployment.md`; verified before commit (captured in `report.md` DoD Closure Accounting and Step 5).

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior: `SCN-049-B001` (contract pin) and `SCN-049-B002` (regression test detects drift) are mapped to concrete test functions in `internal/deploy/external_images_contract_test.go` and execute on every `./smackerel.sh test unit --go` run. (Contract-only change with no runtime path — contract-test layer is the deepest applicable regression layer per Gate G028.)

  Evidence: see Test Plan below; sub-tests `TestExternalImagesContract_LiveFiles`,
  `TestExternalImagesContract_AdversarialMissingPrometheus`,
  `TestExternalImagesContract_AdversarialMissingNats`,
  `TestExternalImagesContract_AdversarialStaleEntry`, and
  `TestExternalImagesContract_AdversarialLiteralImageMismatch` map directly to
  the two scenarios; full-suite green output captured in `report.md` Step 2/3.

- [x] Broader E2E regression suite passes: contract changes are guarded by
      the full `internal/deploy/...` deploy-package gate (`go test
      ./internal/deploy/`), which runs the contract test alongside ~21 sibling
      monitoring/compose contract sub-tests for cross-coverage of related
      surfaces. (Contract-only change with no runtime path — deploy-package
      gate is the broader regression layer in scope.)

  Evidence: `report.md` Step 3 captures `ok github.com/smackerel/smackerel/internal/deploy 19.079s` with zero regressions across the full sibling contract suite.

### Test Plan

| Scenario ID | Layer | Test File | Test Function(s) | Notes |
|-------------|-------|-----------|------------------|-------|
| SCN-049-B001 | Regression E2E (contract-layer, unit/Go) | `internal/deploy/external_images_contract_test.go` | `TestExternalImagesContract_LiveFiles` | Live-file scenario-specific regression E2E: parses `deploy/contract.yaml` + `deploy/compose.deploy.yml` and asserts `prom/prometheus:v2.55.1` is enumerated under `externalImages` with `profile: monitoring`. |
| SCN-049-B002 | Regression E2E (contract-layer, unit/Go) | `internal/deploy/external_images_contract_test.go` | `TestExternalImagesContract_AdversarialMissingPrometheus`, `TestExternalImagesContract_AdversarialMissingNats`, `TestExternalImagesContract_AdversarialStaleEntry`, `TestExternalImagesContract_AdversarialLiteralImageMismatch` | Adversarial scenario-specific regression E2E proving drift is caught: missing prometheus, missing nats, stale entry, literal-image tag mismatch all fail with named error messages. |
| Broader regression | Regression E2E (deploy package gate, unit/Go) | `internal/deploy/...` (all `*_test.go` in the package) | full package run | Broader E2E regression suite coverage executed via `go test ./internal/deploy/` — runs the new contract test alongside all sibling deploy contract tests (~21 sub-tests covering monitoring profiles, compose contract, deploy adapter contracts). |

This is a contract-and-test-only change with no runtime path, so the regression
E2E coverage lives at the Go contract-test layer (the same enforcement layer as
sibling `deploy/contract.yaml` and `deploy/compose.deploy.yml` invariants).
