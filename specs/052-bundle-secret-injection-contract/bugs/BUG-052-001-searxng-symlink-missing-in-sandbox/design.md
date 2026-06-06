# BUG-052-001 Design — Bundle-secret sandbox searxng symlink

## Current Truth

`internal/deploy/bundle_secret_contract_test.go` builds an isolated sandbox in
`t.TempDir()` so the adversarial sub-tests can tamper with byte copies of
`scripts/commands/config.sh` and `config/smackerel.yaml` without mutating the
live repo. All other loader inputs are symlinked to the live repo:

```go
symlink(repoRoot/config/prometheus,      tmpRoot/config/prometheus)
symlink(repoRoot/config/prompt_contracts, tmpRoot/config/prompt_contracts)
symlink(repoRoot/config/assistant,        tmpRoot/config/assistant)
symlink(repoRoot/config/nats_contract.json, tmpRoot/config/nats_contract.json)
symlink(repoRoot/deploy,                  tmpRoot/deploy)
```

`config.sh --bundle` (line ~2320) defines and hard-requires:

```bash
SEARXNG_SETTINGS_FILE="$REPO_ROOT/config/searxng/settings.yml"
[[ -f "$SEARXNG_SETTINGS_FILE" ]] || { echo "ERROR: searxng settings file not found: $SEARXNG_SETTINGS_FILE" >&2; exit 1; }
...
mkdir -p "$STAGE_DIR/config/searxng"
cp "$SEARXNG_SETTINGS_FILE" "$STAGE_DIR/config/searxng/settings.yml"
```

Because the sandbox never provided `config/searxng`, the loader exits 1 before
emitting a bundle and every assertion fails.

## Fix

Add one symlink in the sandbox helper, grouped with the other config inputs:

```go
symlink(filepath.Join(repoRoot, "config", "searxng"),
    filepath.Join(tmpRoot, "config", "searxng"))
```

`config/searxng/settings.yml` is static (not tampered by any adversarial
sub-test), so a symlink to the live file is correct and keeps the sandbox
deterministic. The file-header doc comment enumerating the symlinked config
inputs is updated to include `config/searxng/`.

## Why a symlink (not a copy)

The searxng settings are never mutated by any sub-test (only `config.sh` and
`config/smackerel.yaml` are tampered, and those are real-file copies). A symlink
matches the treatment of `prometheus`, `prompt_contracts`, and `assistant` and
keeps the sandbox minimal.

## Change Boundary

- **Allowed:** `internal/deploy/bundle_secret_contract_test.go` (sandbox helper +
  doc comment).
- **Excluded:** `scripts/commands/config.sh`, `config/searxng/settings.yml`,
  spec 052 `spec.md`/`design.md`/`scopes.md`/`state.json` certification fields,
  all runtime/source code.

## Test Strategy

| Test | Type | Asserts |
|------|------|---------|
| T-BUG-052-001-1 | regression (red→green) | `go test -run TestBundleSecretContract ./internal/deploy/...` passes |
| T-BUG-052-001-2 | adversarial-preservation | A1/A2/A3/A4 sub-tests still enforce their original expectations |
