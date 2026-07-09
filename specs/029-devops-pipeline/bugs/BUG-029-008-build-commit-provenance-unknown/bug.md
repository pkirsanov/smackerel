# BUG-029-008 — Locally-built images stamp `SMACKEREL_COMMIT=unknown` (no source-SHA provenance)

- **Severity:** MEDIUM (redteam **F6**)
- **Owning spec:** `029-devops-pipeline` (owns the SMACKEREL_COMMIT / build-metadata SST contract; cf. BUG-029-003)
- **Source:** redteam adversarial interrogation of the LIVE smackerel prod deployment on evo-x2
- **Status:** FIXED IN-REPO (build wiring; verified statically) — requires rebuild+redeploy — not pushed

## Summary

The running prod core + ml images carry OCI `org.opencontainers.image.revision = unknown` and core
`SMACKEREL_COMMIT = unknown`, so the app's `commit_hash` reports `unknown` — the running artifact is
**not self-identifying**. The Dockerfile ARG/LABEL/ldflags wiring was already correct; the defect was
that the **local-operator (evo-x2) build path never supplied the SHA**, so it fell through to the
literal `"unknown"`.

## Reproduction

**Redteam (live prod):** `docker inspect` core+ml → `org.opencontainers.image.revision=unknown`;
core `SMACKEREL_COMMIT=unknown`; app `commit_hash` would report `unknown`.

**In-repo static confirmation:**

- [Dockerfile](../../../../Dockerfile) is correctly wired: `ARG COMMIT_HASH=unknown`,
  `LABEL org.opencontainers.image.revision="${COMMIT_HASH}"`, and
  `-ldflags "… -X main.commitHash=${COMMIT_HASH}"`. (ml/Dockerfile mirrors this.)
- [docker-compose.yml](../../../../docker-compose.yml) passes `COMMIT_HASH: ${SMACKEREL_COMMIT:?…}`
  from the generated env file.
- [scripts/commands/config.sh](../../../../scripts/commands/config.sh) generated that env value with:

  ```bash
  if [[ -z "${SMACKEREL_COMMIT+set}" ]]; then SMACKEREL_COMMIT="unknown"; fi
  ```

  CI exports `SMACKEREL_COMMIT`, so CI images were fine — but the **local-operator / evo-x2 build
  builds ON the host with no CI env**, so it fell through to `"unknown"`.

## Root cause

The build-metadata resolution had exactly two sources: (1) shell env (CI), (2) the literal
`"unknown"`. There was **no derivation from the git working tree**, so any non-CI (local-operator)
build — which is the evo-x2 prod build path — produced `unknown` provenance.

## Fix (in-repo — build wiring)

[scripts/commands/config.sh](../../../../scripts/commands/config.sh): when `SMACKEREL_COMMIT` is
**unset** (no CI export), derive the source SHA from the git working tree
(`git rev-parse --short=12 HEAD`, with a `-dirty` suffix when the tree is dirty so a locally-modified
build never claims a clean SHA). Falls back to `"unknown"` **only** outside a git checkout (e.g. a
source tarball). **CI / shell-env precedence is preserved** — the derivation arm runs only when
`SMACKEREL_COMMIT` is unset, so a CI `SMACKEREL_COMMIT=<sha>` export still wins.

Verified **statically** via a config-generator test (per the finding: *do not run a full home-lab
image build*):

- [scripts/commands/config_build_commit_provenance_test.sh](../../../../scripts/commands/config_build_commit_provenance_test.sh) — Sub-test 1: unset ⇒ real 12-hex SHA; Sub-test 2: exported sentinel ⇒ preserved.
- [internal/config/sst_loader_build_commit_provenance_test.go](../../../../internal/config/sst_loader_build_commit_provenance_test.go) — Go driver (runs under `test unit --go`; sets git `safe.directory=*` for the container test surface).

## Test evidence

**RED (pre-fix source, new test) — `./smackerel.sh test unit --go` with fix stashed:**

```
    sst_loader_build_commit_provenance_test.go:61: BUG-029-008 build-commit provenance shell test failed: exit status 1
        FAIL: dev.env SMACKEREL_COMMIT is not a real git SHA — actual: 'unknown'
--- FAIL: TestSSTLoader_BuildCommitProvenance_BUG029008 (6.97s)
FAIL    github.com/smackerel/smackerel/internal/config  7.013s
___GO_RED_EXIT=1___
```

**GREEN (fix in place) — `./smackerel.sh test unit`:**

```
[go-unit] go test ./... finished OK
ok      github.com/smackerel/smackerel/internal/config  7.572s
___FULL_UNIT_EXIT=0___
```

## Redeploy note

The wiring change only affects **future builds**. The already-running prod images keep
`revision=unknown` until they are **rebuilt** (`./smackerel.sh config generate` + image build) **and
redeployed** by the operator. No push / rebuild / redeploy performed here.
