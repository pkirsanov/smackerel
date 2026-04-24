# Execution Report — BUG-002: CI Docker Build on PR Events

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

| Field | Value |
|-------|-------|
| Bug ID | BUG-002 |
| Finding ID | DO-002 |
| Parent Spec | 029-devops-pipeline |
| Severity | HIGH — RESOLVED (verified not reproducible) |
| Resolution | No code change required — already addressed |
| Workflow Mode | bugfix-fastlane |

## Execution Evidence

### Verification Steps Performed

1. **Reviewed `.github/workflows/ci.yml` trigger block** — Confirmed `pull_request: branches: [main]` is present (line 10).
2. **Reviewed `build` job configuration** — Confirmed no `if:` condition limits execution to main-only. The job runs on all workflow trigger events.
3. **Reviewed `build` job steps** — Confirmed `./smackerel.sh build` is executed, which compiles both Docker images.
4. **Cross-referenced parent spec scope** — Scope 1 of spec 029-devops-pipeline created this CI workflow with intentional PR coverage.

### CI Workflow Structure (Verified)

| Job | Trigger Events | Has `if:` Guard | Runs on PRs |
|-----|---------------|-----------------|-------------|
| `lint-and-test` | push + pull_request | No | Yes |
| `build` | push + pull_request | No | Yes |
| `integration` | push + pull_request | Yes (`refs/heads/main`) | No (correct) |

### Scope Completion

| Scope | Status | Evidence |
|-------|--------|----------|
| 1: Verify CI Build Runs on PRs | Done | All 3 DoD items verified against committed `ci.yml` |

## Conclusion

Bug BUG-002 (finding DO-002) is formally closed. The reported issue was already addressed by Scope 1 of spec 029-devops-pipeline. No code changes were required for this bug closure.

## Completion Statement

Status: done. All 3 DoD items verified 2026-04-24 against the committed `.github/workflows/ci.yml`. The `pull_request` trigger is present, the `build` job carries no `if:` guard, and `./smackerel.sh build` is executed. Captured grep evidence below.

## Test Evidence

This bug required no code change — the CI workflow itself is exercised on every push and pull_request to `main`. Re-cert pass executed the repo-CLI sanity check plus a Go-side regression to confirm no breakage:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 ./internal/config/...
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

### Validation Evidence

CI workflow trigger + build job structure captured 2026-04-24:

```text
$ sed -n '1,12p' .github/workflows/ci.yml
# Smackerel CI — lint, unit test, build on every push/PR
# Runs ./smackerel.sh commands per project CLI contract
name: CI

on:
  push:
    branches: [ main ]
    tags: [ 'v*' ]
  pull_request:
    branches: [ main ]
$ awk '/^[[:space:]]*build:/,/^[a-z]/' .github/workflows/ci.yml | head -16
  build:
    needs: lint-and-test
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
      packages: write
    steps:
    - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4.3.1
    - name: Build Docker images
      run: |
        export SMACKEREL_VERSION="${GITHUB_REF_NAME}"
        export SMACKEREL_COMMIT="${GITHUB_SHA:0:12}"
        export SMACKEREL_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        ./smackerel.sh build
```

DoD mapping:
- DoD-1 (`pull_request` trigger) → line 9-10 of the first capture.
- DoD-2 (no `if:` guard on `build`) → second capture goes from `build:` directly to `needs:` and `runs-on:` without an `if:` line.
- DoD-3 (`./smackerel.sh build` runs) → last line of the second capture.

### Audit Evidence

Repo-CLI hygiene check captured 2026-04-24T07:30:21Z → 07:30:29Z, plus an integration-job cross-check to confirm only the gated `integration` job (not `build`) is restricted to main:

```text
$ grep -nE "^[[:space:]]+if:" .github/workflows/ci.yml
92:    if: github.ref == 'refs/heads/main'
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
```

The single `if:` clause in the workflow is on the `integration` job at line 92 (immediately after `integration:` declaration at line 91), confirming `build` job has no main-only restriction and PR runs are honoured.

