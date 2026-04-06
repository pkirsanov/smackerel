---
applyTo: "**"
---

# Terminal Discipline Policy (NON-NEGOTIABLE)

## 1. No Piping or Redirecting Output Into Files

Use IDE file-editing tools for file writes. Do not use shell redirection or pipe output into files.

## 2. No Truncating Command Output

Do not use `head`, `tail`, or filtered command pipelines that hide output lines. Capture full output.

## 3. Current Repo State: No Runtime CLI Yet

Smackerel does not currently have a committed runtime CLI, build pipeline, or service stack in the repository.

### Forbidden

- Inventing `./smackerel.sh` commands.
- Running ad-hoc build, lint, test, deploy, or service-management commands against source trees that do not exist.
- Claiming runtime validation for Go, Python, Docker Compose, PostgreSQL, NATS, or Ollama assets that are not committed yet.

### Required

Use the committed Bubbles validation commands for current repo work:

```bash
bash .github/bubbles/scripts/cli.sh doctor
timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate
bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>
timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>
```

Once a real repo CLI is committed, update this instruction file, `.specify/memory/agents.md`, and `.github/copilot-instructions.md` together.

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
| Build/test/lint | invented `./smackerel.sh` or ad-hoc runtime commands | committed Bubbles validation commands only, until runtime tooling exists |

Violations of this policy are blocking issues.
