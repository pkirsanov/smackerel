# BUG-052-001 Spec — Bundle-secret sandbox searxng symlink

## Bug Statement

The spec 052 bundle-secret contract test (`internal/deploy/bundle_secret_contract_test.go`)
fails because its temporary sandbox `REPO_ROOT` does not provide
`config/searxng/settings.yml`, which `scripts/commands/config.sh --bundle` now
requires.

## Functional Requirements

- **FR-BUG-052-001-1**: The bundle-secret sandbox MUST provide every config
  input that `config.sh --bundle` requires, including `config/searxng/settings.yml`.
- **FR-BUG-052-001-2**: `go test -run TestBundleSecretContract ./internal/deploy/...`
  MUST pass (all five sub-tests green), restoring a green `internal/deploy`
  package so code pushes are not blocked.
- **FR-BUG-052-001-3**: The fix MUST NOT weaken any spec 052 assertion — the
  placeholder/leakage/determinism/opt-out adversarial guarantees stay intact.

## Acceptance Scenarios

```gherkin
Scenario: SCN-BUG-052-001-1 Sandbox bundle generation finds searxng settings
  Given the bundle-secret sandbox assembles a temporary REPO_ROOT
  When config.sh --bundle runs against that sandbox
  Then config/searxng/settings.yml is present via symlink
  And the loader exits 0 instead of "searxng settings file not found"

Scenario: SCN-BUG-052-001-2 Contract suite is green
  Given the searxng symlink is added to the sandbox
  When go test -run TestBundleSecretContract ./internal/deploy/... runs
  Then all five sub-tests pass
  And the internal/deploy package reports ok

Scenario: SCN-BUG-052-001-3 Adversarial guarantees preserved
  Given the searxng symlink is the only sandbox change
  When the A1 drift, A2 leakage, A3 determinism, and A4 opt-out sub-tests run
  Then each adversarial assertion still holds with no weakened expectation
```

## Out of Scope

- The production `config.sh --bundle` searxng requirement is correct and is NOT
  changed.
- Spec 052 status, certification, and the secret-keys manifest are unchanged.
