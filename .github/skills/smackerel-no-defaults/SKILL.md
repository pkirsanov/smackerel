---
name: smackerel-no-defaults
description: Enforce Smackerel's NO-DEFAULTS / fail-loud SST policy. Use when editing config, Docker Compose, deploy adapters, docs, instructions, skills, env handling, host bind addresses, or any runtime value that could be hidden by fallback syntax.
---

# Smackerel NO-DEFAULTS Guard

## Goal

Prevent silent runtime fallback config. Smackerel must fail loudly when required config is missing so agents and operators cannot accidentally ship hidden defaults.

## Use This Skill When

- Editing `config/smackerel.yaml`, `scripts/commands/config.sh`, `docker-compose.yml`, `deploy/compose.deploy.yml`, or deploy adapter logic.
- Changing docs, instructions, or skills that describe config, env vars, ports, host bind addresses, or deployment.
- Reviewing any use of shell/Compose interpolation, `os.getenv`, `process.env`, or config helper defaults.
- Touching `HOST_BIND_ADDRESS`, port mappings, env files, or Build-Once Deploy-Many config bundles.

## Blocking Rules

- No `${VAR:-default}` or `${VAR-default}` for SST-managed runtime values.
- No language-level fallback for required config (`os.getenv("KEY", "default")`, `process.env.KEY || "default"`, `unwrap_or(...)`).
- No docs language that implies an implicit fallback (`safe-by-default`, `loopback default`, `preserves loopback`).
- No generated config edits by hand.
- No deploy adapter may rely on Compose to invent missing values.

## Required Patterns

| Surface | Required Pattern |
|---------|------------------|
| Compose / shell required value | `${VAR:?clear error message}` |
| Go required env | `os.Getenv("KEY")` plus explicit empty-value error |
| Python required env | `os.environ["KEY"]` or explicit empty-value error |
| TypeScript required env | `process.env.KEY` plus explicit missing-value throw |
| Loopback bind | Explicit `HOST_BIND_ADDRESS=127.0.0.1` in generated/apply-time env, never a Compose fallback |

## HOST_BIND_ADDRESS Contract

The live deploy bind-address contract is:

```yaml
ports:
  - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
```

The deploy adapter must write `HOST_BIND_ADDRESS` explicitly into `app.env` before Compose starts. If it cannot determine the bind address, it must fail before `docker compose up`. `127.0.0.1` is valid only as an explicit configured value.

## Review Checklist

```
[ ] Touched files contain no hidden fallback for required runtime config.
[ ] Any mention of `${VAR:-default}` labels it as forbidden, not recommended.
[ ] `HOST_BIND_ADDRESS` docs say adapter-set explicit value + fail-loud Compose.
[ ] `deploy/compose.deploy.yml` and `internal/deploy/compose_contract_test.go` match.
[ ] Validation evidence uses repo-standard commands from `.github/copilot-instructions.md`.
```

## Anti-Patterns

| Anti-Pattern | Why It Blocks | Fix |
|--------------|---------------|-----|
| `${HOST_BIND_ADDRESS:-127.0.0.1}` in live Compose | Missing adapter config silently becomes loopback | `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` |
| "loopback default" in docs | Teaches agents to rely on fallback behavior | "explicit loopback value" |
| `os.getenv("SMACKEREL_AUTH_TOKEN", "")` for required config | Auth misconfig becomes empty string | Required env read plus explicit empty check |
| Adapter omits `HOST_BIND_ADDRESS` because Compose has a fallback | Adapter contract is incomplete | Adapter writes the value or fails before Compose |
