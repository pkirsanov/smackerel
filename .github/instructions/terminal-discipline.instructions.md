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

## 5. Never Echo Secret Values (ABSOLUTE)

Printing the value of a secret-bearing variable is FORBIDDEN — directly, or
accidentally via a shell-parameter-expansion default. Secrets include
`COSIGN_PASSWORD` (operator cosign signing in `build --target self-hosted`),
`config/smackerel.yaml` secret fields (`runtime.auth_token`, `llm.api_key`,
`telegram.bot_token`, connector `access_token`), any `*_TOKEN` / `*_KEY` /
`*_PASSWORD` / `*_SECRET`, and decrypted env files under `config/generated/`.

```bash
# ❌ FORBIDDEN — prints the VALUE when the var is set
echo "$COSIGN_PASSWORD"
echo "PW=${COSIGN_PASSWORD:-<unset>}"   # :- substitutes the VALUE when set!
echo "PW=${COSIGN_PASSWORD-<unset>}"    # same trap without the colon
set -x; use "$COSIGN_PASSWORD"; set +x  # xtrace prints the value
printenv | grep -i token                # dumps token values
env                                     # dumps every secret value
```

The expansion trap: `${VAR:-X}` and `${VAR-X}` substitute `X` ONLY when `VAR`
is unset/empty. When `VAR` is set, they expand to its value — so a "mask" like
`${SECRET:-<unset>}` prints the real secret whenever it is present. This leaks
into terminal output and, when an agent drives the shell, into the
non-retractable session transcript / context.

REQUIRED — report only presence/absence with a value-safe form:

```bash
[ -n "${COSIGN_PASSWORD:-}" ] && echo "COSIGN_PASSWORD: set" || echo "COSIGN_PASSWORD: unset"
echo "COSIGN_PASSWORD: ${COSIGN_PASSWORD:+set}"   # ":+" emits "set" or empty
echo "len=${#COSIGN_PASSWORD}"                    # length only, never contents
```

Additional rules:
- NEVER ask anyone to paste a secret into chat or a tool — type it into the
  terminal (`read -rs VAR && export VAR`), which the agent never sees.
- `read -rs` is silent BY DESIGN — a blank cursor is not a hung command. Do
  not "confirm" a silent prompt by echoing the variable.
- If a secret value is emitted by mistake, treat it as a security incident:
  `unset` it and rotate the leaked credential. The transcript cannot be
  retracted, so rotation is the only safe remedy.

## Summary Table

| Category | Forbidden | Required |
|----------|-----------|----------|
| File writes | shell redirection, `tee`, heredoc-to-file | IDE file tools |
| Output filtering | `head`, `tail`, filtered command pipes | full unfiltered output |
| Build/test/lint | ad-hoc `go`/`python`/`pytest`/`docker compose` runtime workflow | `./smackerel.sh <command>` |
| Secret values | `echo "$SECRET"`, `${SECRET:-mask}` (expands to value when set), `set -x`/`env` around secrets | `${SECRET:+set}` / `[ -n "$SECRET" ]` presence checks; `read -rs` for input |

Violations of this policy are blocking issues.
