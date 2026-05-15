# User Validation: BUG-020-004 — ML NATS client `SMACKEREL_AUTH_TOKEN` fail-loud read at connect time

## Checklist

- [x] `ml/app/nats_client.py` no longer re-reads `SMACKEREL_AUTH_TOKEN` from `os.environ` at connect time.
  Evidence: `report.md#validation-evidence`.

- [x] `ml/app/nats_client.py` consumes the canonical fail-loud-read constant `_AUTH_TOKEN` from `app.auth`.
  Evidence: `report.md#implementation-evidence`.

- [x] ML unit-test suite exits successfully with the `_AUTH_TOKEN`-patched behavioural tests and source-contract audit.
  Evidence: `report.md#test-evidence`.

- [x] Adversarial regression quality guard detects a bugfix signal in `ml/tests/test_nats_client.py` with zero guard violations.
  Evidence: `report.md#regression-quality-evidence`.

- [x] Gate G028 NO-DEFAULTS / fail-loud SST audit grep is clean for the `SMACKEREL_AUTH_TOKEN` surface in `ml/app/nats_client.py`.
  Evidence: `report.md#validation-evidence`.

## Cross-References

- Bug packet root: `specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/`
- Bug specification: `spec.md`
- Root cause and fix design: `design.md`
- Fix scope structure: `scopes.md`
- Execution evidence: `report.md`
- Scenario contract registry: `scenario-manifest.json`
- Control-plane state: `state.json`
