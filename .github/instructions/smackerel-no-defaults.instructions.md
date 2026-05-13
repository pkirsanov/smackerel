---
applyTo: "**"
---

# Smackerel NO-DEFAULTS SST Enforcement

This repo enforces fail-loud configuration. Missing runtime config must stop startup immediately; it must never be hidden by fallback syntax.

## Non-Negotiable Rule

For SST-managed runtime values, the following are forbidden in source, Compose, deploy specs, scripts, examples, and docs unless the text explicitly labels the form as **FORBIDDEN**:

- `${VAR:-default}`
- `${VAR-default}`
- `os.getenv("KEY", "default")`
- `process.env.KEY || "default"`
- `unwrap_or(...)` / `unwrap_or_default()` for required config
- Any helper that silently supplies a runtime fallback value

Use fail-loud forms instead:

- Compose / shell substitution: `${VAR:?clear error message}`
- Go: `os.Getenv("KEY")` followed by an explicit empty-value error
- Python: `os.environ["KEY"]` or an explicit empty-value error
- TypeScript: `process.env.KEY` followed by an explicit missing-value throw

## HOST_BIND_ADDRESS Contract

`HOST_BIND_ADDRESS` is a required deployment bind-address value. It is allowed to be `127.0.0.1` only when that value is explicitly present in the SST-generated env file or explicitly written by the deploy adapter. It is never allowed as a Compose fallback.

Required live deploy Compose form:

```yaml
ports:
  - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
```

Rules:

- `deploy/compose.deploy.yml` MUST use `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` for host bind-address interpolation.
- The deploy adapter MUST set `HOST_BIND_ADDRESS` explicitly in `app.env` before running Compose.
- Missing or empty `HOST_BIND_ADDRESS` MUST abort Docker Compose at substitution time.
- Do not describe this as "safe-by-default", "loopback default", or "preserves loopback". Say "explicit loopback value" when `127.0.0.1` is intended.

## Review Checklist

Before finishing any config, deploy, docs, instruction, or skill change:

- Search touched files for `${VAR:-`, `${VAR-`, `safe-by-default`, and `loopback default`.
- Confirm any remaining fallback examples are explicitly marked forbidden.
- If `deploy/compose.deploy.yml` changes, keep `internal/deploy/compose_contract_test.go` in lockstep.
- If docs/instructions/skills mention `HOST_BIND_ADDRESS`, they must state the deploy adapter sets it explicitly and Compose fails loud if missing.
