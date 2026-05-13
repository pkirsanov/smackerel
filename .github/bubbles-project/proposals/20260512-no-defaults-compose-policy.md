# Bubbles Framework Change Proposal

- Title: Make Compose no-defaults / fail-loud SST policy explicit in portable config and deployment guidance
- Slug: no-defaults-compose-policy
- Created: 2026-05-12
- Created From: smackerel
- Requested Upstream Repo: bubbles

## Summary

Clarify upstream Bubbles config-SST and deployment-target guidance so agents never treat `${VAR:-default}` as acceptable for runtime configuration. The portable instructions already say fail-loud config is required, but they should explicitly call out Docker Compose interpolation, adapter-supplied env files, and bind-address variables.

## Why This Must Be Upstream

Downstream repos must not directly edit framework-managed instruction or skill files. Smackerel now carries a project-owned instruction and skill for this rule, but the general Bubbles framework should teach the same pattern everywhere:

- Defaults belong in SST or generated env files, not Compose interpolation.
- Required Compose values should use `${VAR:?clear error}`.
- Deploy adapters must write required env values explicitly before runtime startup.
- Missing required config must fail at substitution/startup time.

## Proposed Bubbles Change

Update the upstream equivalents of:

- `instructions/bubbles-config-sst.instructions.md`
- `instructions/bubbles-deployment-target.instructions.md`
- `skills/bubbles-config-sst/SKILL.md`
- `skills/bubbles-deployment-target-adapter/SKILL.md`

with a portable, project-agnostic rule:

```yaml
ports:
  - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${HOST_PORT}:${CONTAINER_PORT}"
```

The example may use a generic bind-address variable, but the rule should apply to all SST-managed required runtime values: ports, bind addresses, endpoints, env-file paths, image refs, credentials, and service URLs.

## Acceptance Criteria

- [ ] Upstream config-SST instruction has a dedicated Compose interpolation section: no `${VAR:-default}` for SST-managed runtime values.
- [ ] Upstream deployment-target instruction states adapters must materialize required env values before runtime startup.
- [ ] Upstream config-SST skill checklist includes Compose/deploy fail-loud substitution.
- [ ] Upstream deployment-target skill anti-patterns include `${VAR:-default}` for adapter-owned runtime config.
- [ ] Checksums regenerated in upstream Bubbles and downstream refresh no longer treats these policy additions as local drift.

## Downstream Bridge In Smackerel

Until the upstream change lands, Smackerel enforces the rule through project-owned surfaces:

- `.github/instructions/smackerel-no-defaults.instructions.md`
- `.github/skills/smackerel-no-defaults/SKILL.md`
- `.github/copilot-instructions.md`
- `docs/Development.md`, `docs/Operations.md`, `deploy/README.md`
