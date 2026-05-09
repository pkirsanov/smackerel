# User Validation Checklist — 042 Tailnet-Edge Bind Pattern

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Backend services in `deploy/compose.deploy.yml` use the configurable
      bind address `${HOST_BIND_ADDRESS:-127.0.0.1}` (validated by Scope 1
      Go unit test `TestComposeContract_LiveFile`)
- [x] Infrastructure services (`postgres`, `nats`) in
      `deploy/compose.deploy.yml` have no host port mapping (validated by
      Scope 1 Go unit test `TestComposeContract_LiveFile`)
- [x] Compose default substitution falls back to `127.0.0.1` so a fresh
      `docker compose -f deploy/compose.deploy.yml up` (no env setup) is
      safe-by-default (validated by Scope 1 manual rendering proof)
- [x] Adversarial regression: literal `127.0.0.1:` prefix on backend ports
      would cause the contract test to FAIL (validated by Scope 1 Go unit
      test `TestComposeContract_AdversarialLiteralBind`)
- [x] Adversarial regression: a `ports:` block on `postgres` would cause
      the contract test to FAIL (validated by Scope 1 Go unit test
      `TestComposeContract_AdversarialInfraHasPorts`)
- [x] `.github/copilot-instructions.md` documents the tailnet-edge bind
      pattern under Required Runtime Standards and forbids re-introducing
      literal `127.0.0.1:` for backends (validated by Scope 2 doc-lint
      grep)
- [x] `docs/Operations.md` documents how a devops user reaches Postgres
      and NATS via `docker exec` and how HTTP UI access flows through host
      Caddy (validated by Scope 2 doc-lint grep)
- [x] `config/smackerel.yaml` carries an inline comment above
      `runtime.host_bind_address` cross-referencing the SKILL and the
      override path (validated by Scope 1 raw excerpt)
- [x] Scope 2 follows Scope 1 (DAG ordering preserved); no scope skipped

Unchecked items indicate a user-reported regression.
