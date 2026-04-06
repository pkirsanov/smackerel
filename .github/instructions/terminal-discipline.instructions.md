---
applyTo: "**"
---

# Terminal Discipline Policy (NON-NEGOTIABLE)

## 1. No Piping or Redirecting Output Into Files

Use IDE file-editing tools for file writes. Do not use shell redirection or pipe output into files.

## 2. No Truncating Command Output

Do not use `head`, `tail`, or filtered command pipelines that hide output lines. Capture full output.

## 3. Runtime Operations Flow Through The Repo CLI

Smackerel now has a committed repo CLI and config pipeline for runtime build, test, lint, and service lifecycle operations.

### Forbidden

- Bypassing `./smackerel.sh` with ad-hoc `go`, `python`, `pytest`, or `docker compose` commands for normal runtime work.
- Editing generated config under `config/generated/` by hand.

### Required

Use `./smackerel.sh` for runtime operations and keep the committed Bubbles validation commands for framework/spec governance work:

```bash
./smackerel.sh config generate
./smackerel.sh build
./smackerel.sh check
./smackerel.sh lint
./smackerel.sh format
./smackerel.sh test unit
./smackerel.sh test integration
./smackerel.sh test e2e
./smackerel.sh test stress
./smackerel.sh up
./smackerel.sh down
./smackerel.sh status
./smackerel.sh logs
./smackerel.sh clean smart

bash .github/bubbles/scripts/cli.sh doctor
timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate
bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>
timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>
```

## 4. Allowed Read-Only Commands

Read-only inspection commands are allowed:
- `ls`, `cat`, `find`, `grep`, `wc`
- `git log`, `git diff`, `git status`
- `docker ps`, `docker logs`
- `curl --max-time 5` for quick health checks when a runtime actually exists

## Summary Table

| Category | Forbidden | Required |
|----------|-----------|----------|
| File writes | shell redirection, `tee`, heredoc-to-file | IDE file tools |
| Output filtering | `head`, `tail`, filtered command pipes | full unfiltered output |
| Build/test/lint | ad-hoc `go`/`python`/`pytest`/`docker compose` runtime workflow | `./smackerel.sh <command>` |

Violations of this policy are blocking issues.
