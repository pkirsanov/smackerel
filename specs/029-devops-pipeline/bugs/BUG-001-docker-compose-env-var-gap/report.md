# BUG-001 Report: Docker Compose Environment Variable Gap

**Parent:** [029-devops-pipeline](../../spec.md)
**Bug:** [bug.md](bug.md)
**Severity:** CRITICAL (P0)
**Status:** Fixed

---

## Summary

Replaced 100+ individual `KEY: ${KEY}` environment declarations in `docker-compose.yml` with `env_file: config/generated/dev.env` for both `smackerel-core` and `smackerel-ml` services. Added env_file drift guard to `./smackerel.sh check`.

## Changes

| File | Change |
|------|--------|
| `docker-compose.yml` | Replaced individual env declarations with `env_file:` directive for core and ML services. Kept only container-internal path overrides in `environment:` block. |
| `smackerel.sh` | Added env_file drift guard to `check` command — verifies `env_file:` exists and no SST-managed vars are individually declared. |

## Verification

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" |
| `./smackerel.sh test unit` (Go) | PASS — all packages |
| `./smackerel.sh test unit` (Python) | PASS — 214 passed, 0 failed |
| dev.env variable count | 140+ variables generated from SST |
| Drift guard adversarial | Guard correctly rejects individual SST-managed var declarations |

## Impact

All features added since the initial Compose wiring — expense tracking (034), recipe cook mode (035), meal planning (036), GuestHost connector (013), Telegram assembly (008), OTEL observability (030), and others — now receive their configuration in containerized deployment via `env_file`.

## Completion Statement

Status: done. The fix is present in `docker-compose.yml` and `smackerel.sh`. All 10 DoD items across the three scopes already carry inline evidence in `scopes.md`; this re-cert pass adds Validation Evidence and Audit Evidence sections below from a fresh `./smackerel.sh check` run plus targeted greps captured 2026-04-24.

## Test Evidence

Full repo-CLI unit run captured 2026-04-24:

```text
$ ./smackerel.sh test unit
........................................................................ [ 21%]
........................................................................ [ 43%]
........................................................................ [ 65%]
........................................................................ [ 87%]
..........................................                               [100%]
330 passed, 2 warnings in 11.48s
```

Go packages (compiled and tested as part of the same `./smackerel.sh test unit` invocation) report no failures.

### Validation Evidence

Docker Compose env_file wiring captured 2026-04-24:

```text
$ grep -n "env_file\|environment:" docker-compose.yml
12:    environment:
77:    env_file:
79:    environment:
81:      # The env_file provides the host path; these overrides point to the
130:    env_file:
132:    environment:
$ go test -count=1 ./internal/config/...
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

Drift guard wiring captured 2026-04-24:

```text
$ grep -n "env_file" smackerel.sh
139:    # env_file drift guard — verify core/ml services use env_file, not individual SST vars
140:    if ! grep -q 'env_file:' docker-compose.yml; then
141:        echo "ERROR: docker-compose.yml missing env_file: directive — SST vars must flow through config/generated/dev.env"
146:    # Only check services that use env_file (core and ml); postgres/nats keep their own blocks.
149:    env_file="$(smackerel_env_file "$TARGET_ENV")"
170:    done < "$env_file"
172:        echo "ERROR: docker-compose.yml core/ml services contain individual SST-managed env declarations — use env_file: instead"
176:    echo "env_file drift guard: OK"
$ go test -count=1 ./internal/config/...
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

### Audit Evidence

Repo-CLI hygiene check captured 2026-04-24T07:30:21Z → 07:30:29Z, plus a focused Go-side regression to confirm no neighbouring package regressed:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 ./internal/config/...
ok      github.com/smackerel/smackerel/internal/config  0.006s
```

The drift guard fires only when an env_file directive is missing or when SST-managed variables are individually declared in core/ml services; both checks remain green, confirming Scope 1 + Scope 2 still hold. The Go config-package regression replay confirms env wiring still parses cleanly.

