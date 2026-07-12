# BUG-052-001 — Bundle-secret contract test sandbox missing config/searxng symlink

- **Parent spec:** 052-bundle-secret-injection-contract
- **Severity:** High (blocks the entire `internal/deploy` package test suite → blocks every code push)
- **Status:** done
- **Discovered:** 2026-06-06 (stochastic sweep Round 17 surfaced finding F-047-R17-A; routed to spec 052)
- **Resolved:** 2026-06-06

## Summary

`TestBundleSecretContract_NoLiteralSecretsInSelfHosted` and its four adversarial
sub-tests (`AdversarialA1_DriftDetector`, `AdversarialA2_LeakageDetector`,
`AdversarialA3_DeterminismDetector`, `AdversarialA4_OptOutDetector`) all FAIL
with:

```
ERROR: searxng settings file not found: /tmp/TestBundleSecretContract_.../001/config/searxng/settings.yml
```

The whole `internal/deploy` package therefore reports `FAIL`, which blocks any
code push that runs the deploy package tests.

## Root Cause

The bundle-secret sandbox helper in
`internal/deploy/bundle_secret_contract_test.go` assembles a temporary
`REPO_ROOT` under `t.TempDir()` and symlinks the live config inputs the loader
needs: `config/prometheus`, `config/prompt_contracts`, `config/assistant`,
`config/nats_contract.json`, and `deploy/`.

Commit `a7774fff` (`test(searxng): contract test for config/searxng/settings.yml`)
later added a hard requirement to `scripts/commands/config.sh` (the `--bundle`
flow, line ~2329):

```bash
[[ -f "$SEARXNG_SETTINGS_FILE" ]] || { echo "ERROR: searxng settings file not found: $SEARXNG_SETTINGS_FILE" >&2; exit 1; }
```

`config.sh --bundle` now copies `config/searxng/settings.yml` into the bundle
staging directory (spec 064 SCOPE-17 bind-mount requirement). The spec 052
sandbox was never updated to provide that file, so `runConfigGenerate` exits 1
and every assertion that depends on a generated bundle fails.

This is a **test-infrastructure regression**: the production contract is
correct, but the spec 052 sandbox drifted from the loader's input contract
when searxng became a required bundle input.

## Fix

Symlink `config/searxng` into the sandbox `REPO_ROOT` alongside the other
config inputs, so `config.sh --bundle` finds `config/searxng/settings.yml` and
the bundle stage succeeds.

## Reproduction

```bash
# RED (before fix)
go test -count=1 -run 'TestBundleSecretContract' ./internal/deploy/...
# → FAIL: 5 failures, all "searxng settings file not found"

# GREEN (after fix)
go test -count=1 -run 'TestBundleSecretContract' ./internal/deploy/...
# → ok  github.com/smackerel/smackerel/internal/deploy  56.014s
```
