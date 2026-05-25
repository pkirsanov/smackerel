# Spec 042 — Tailnet-Edge Bind Pattern (Home-Lab Compose Readiness)

## Problem Statement

Smackerel is deployed self-hosted on the operator's home-lab machine (`<deploy-host>`,
Tailscale FQDN `<deploy-host-fqdn>`). Today `deploy/compose.deploy.yml`
hard-binds every host-forwarded port to `127.0.0.1` (a decision recorded in
spec 020 design.md §"Port Binding: HOST_BIND_ADDRESS Variable in Compose"). That
locked the bundle into a single network shape: backends on loopback, infra
exposed on loopback for devops, no way for a host-level reverse proxy to front
the same compose without editing the file.

The canonical Bubbles framework now defines a 3-layer **tailnet-edge bind
pattern** (`bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md`) plus a Caddy
template (`bubbles/templates/caddy-tailnet-snippet.caddy.template`) for that
shape. Spec 042 originally used a compose-level loopback fallback, but that
decision has been superseded by the fail-loud SST contract introduced by the
NO-DEFAULTS policy: deploy compose MUST require an adapter-provided
`HOST_BIND_ADDRESS` value and MUST NOT invent one with `${VAR:-default}`.

- **L1** — host Caddy bound to the tailnet IP, terminates TLS via `tailscale cert`.
- **L2** — in-container nginx (when present) on loopback only.
- **L3** — backends bind a required configurable host address supplied by the
  deploy adapter; infra (Postgres, NATS) is reachable only from inside the
  compose network.

The deploy adapter for smackerel (separate repo `knb/smackerel/home-lab/`) will
write the host Caddy snippet from the canonical template and will write the
final `HOST_BIND_ADDRESS` value into `app.env`. This spec only fixes the
**compose-side prerequisites** so the adapter can do its job:

1. Backends (`smackerel-core`, `smackerel-ml`) MUST bind a configurable address
    through the fail-loud deploy compose form
    `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` so
    the adapter can explicitly choose loopback, a tailnet IP, or another
    operator-approved bind address.
2. Infra (`postgres`, `nats`) MUST NOT publish any host port. DevOps reaches
   them via `docker exec` over the tailnet (Pattern P1 from the SKILL).
3. The compose file MUST fail loudly when `HOST_BIND_ADDRESS` is missing or
    empty. `127.0.0.1` is valid only as an explicit value supplied by the
    deploy adapter or generated env contract, never as a compose fallback.

## Outcome Contract

- `deploy/compose.deploy.yml` binds `smackerel-core` and `smackerel-ml`
  host-published ports through the fail-loud form
  `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` rather
  than a literal `127.0.0.1` or a `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback.
- The deploy adapter writes an explicit `HOST_BIND_ADDRESS` into `app.env`
  before Compose starts. If loopback is intended, the adapter writes
  `127.0.0.1`; if tailnet-edge fronting is intended, the adapter writes the
  target bind address. Missing or empty values abort Compose substitution.
- `deploy/compose.deploy.yml` no longer publishes a host port for `postgres`
  or `nats`. Both services keep their container ports for in-network access.
- `ollama` (profile-gated, off by default) is **not** modified by this spec.
- A unit-class test (`internal/deploy/compose_contract_test.go`) parses
  `deploy/compose.deploy.yml` and mechanically asserts the four invariants
  above, runnable through `./smackerel.sh test unit --go`.
- `.github/copilot-instructions.md` documents the tailnet-edge bind pattern
  under Required Runtime Standards so future agents do not re-introduce the
  literal-`127.0.0.1` hardcode.
- `docs/Operations.md` documents how a devops user reaches Postgres and NATS
  on the home-lab without host port mappings (concrete `docker exec` command
  shapes), and how HTTP UI access flows through host Caddy.
- `config/smackerel.yaml` carries an inline comment above
  `runtime.host_bind_address` cross-referencing the SKILL, the fail-loud
  deploy compose contract, and the explicit-value override path for deploy
  adapters.

## Goals

- Make the home-lab compose bundle **adapter-ready** for the tailnet-edge
  pattern without committing the adapter or any host-side Caddy work in this
  repo (Adapter Locality rule —
  `.github/instructions/bubbles-deployment-target.instructions.md`).
- Reverse the spec 020 decision in a focused, documented way.
- Lock the four compose invariants behind a mechanical Go unit test that runs
  in `./smackerel.sh test unit --go` and fails loudly on regression.

## Non-Goals

- Writing the Caddyfile, basic-auth snippet, or any host-side service config
  (owned by the deploy adapter, separate repo).
- Touching `ollama` (profile-gated, off by default; out of scope).
- Touching `docker-compose.yml` (the dev compose). This spec only modifies
  `deploy/compose.deploy.yml`, the bundled deploy compose.
- Renaming the SST variable `HOST_BIND_ADDRESS` to `HOST_BIND_ADDR`. The
  existing SST contract is preserved; see design.md §"Variable Naming
  Decision".
- Modifying spec 020 historical artifacts. The reversal is documented in
  this spec's design.md and is normative going forward.
- Any deploy adapter changes. Pure compose-readiness work in this repo only.
- Any runtime behavior change to the dev or test stacks. `./smackerel.sh up`,
  `./smackerel.sh test e2e`, etc. behave identically.

## Requirements

### Functional

- **REQ-1** Backend services in `deploy/compose.deploy.yml` (`smackerel-core`,
  `smackerel-ml`) MUST publish their host port on
  `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}`.
  Compose MUST fail loudly if `HOST_BIND_ADDRESS` is unset or empty. The
  `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback form is superseded and forbidden
  as active deploy guidance.
- **REQ-2** Infrastructure services in `deploy/compose.deploy.yml` (`postgres`,
  `nats`) MUST NOT have a `ports:` block and MUST NOT use `network_mode: host`.
  Container ports remain reachable from peer containers via the compose default
  network.
- **REQ-3** A test runnable via `./smackerel.sh test unit --go` MUST parse
  `deploy/compose.deploy.yml` and assert REQ-1 and REQ-2, including
  adversarial rejection of literal bind prefixes, default-fallback
  substitutions, and host-network bypasses.
- **REQ-4** The SST variable `HOST_BIND_ADDRESS` (already exported by
  `scripts/commands/config.sh` and present in generated env contracts) MUST
  remain the canonical bind-address variable. No new env var is introduced;
  no SST plumbing is duplicated. The final concrete value for a real target is
  owned by the deploy adapter and MUST remain generic in this product repo.
- **REQ-5** `.github/copilot-instructions.md` MUST contain a "Tailnet-Edge
  Bind Pattern" subsection under Required Runtime Standards.
- **REQ-6** `docs/Operations.md` MUST contain a "DevOps Access on Home-Lab
  (Tailnet-Edge Pattern)" section with the canonical `docker exec` command
  shapes for Postgres and NATS plus a note on HTTPS UI access.
- **REQ-7** `config/smackerel.yaml` MUST carry an inline comment above
  `runtime.host_bind_address` pointing the reader to the SKILL and the
  fail-loud explicit-value override path.

### Non-Functional

- **NFR-1** No change to dev/test stack behavior. `./smackerel.sh up`,
  `./smackerel.sh test e2e`, `./smackerel.sh test integration` continue to
  work unchanged because `docker-compose.yml` (dev/test) is not touched.
- **NFR-2** Zero new SST values. Re-use the already-exported
  `HOST_BIND_ADDRESS`.
- **NFR-3** No registry, signing, or build-pipeline changes (Build-Once
  Deploy-Many contract preserved).
- **NFR-4** Adapter Locality preserved: no `deploy/<target>/` files are
  added or modified by this spec.
- **NFR-5** No hidden deploy defaults. Active requirements, docs, tests, and
  compose examples MUST teach fail-loud `HOST_BIND_ADDRESS` substitution and
  explicit adapter-provided values only.
- **NFR-6** Product-repo genericity preserved: this spec MUST NOT include real
  home-lab hostnames, IP addresses, tailnet roots, or operator-private
  topology.

## User Scenarios (Gherkin)

### Scenario: Backend ports use the configurable bind address

```gherkin
Given the deploy compose file `deploy/compose.deploy.yml`
When the file is parsed
Then the `smackerel-core` `ports:` entry uses the prefix `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`
And the `smackerel-ml` `ports:` entry uses the prefix `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`
And neither backend service uses a literal bind prefix or a `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback
```

### Scenario: Infra services have no host port mapping

```gherkin
Given the deploy compose file `deploy/compose.deploy.yml`
When the file is parsed
Then the `postgres` service has no `ports:` block
And the `nats` service has no `ports:` block
```

### Scenario: Missing bind address fails loud

```gherkin
Given the deploy compose file `deploy/compose.deploy.yml`
When `docker compose -f deploy/compose.deploy.yml config` renders the file
   without `HOST_BIND_ADDRESS` set in the environment
Then Compose fails with the message `HOST_BIND_ADDRESS must be set by deploy adapter`
And no backend service can start from an implicit bind address
```

### Scenario: Explicit loopback remains adapter-provided

```gherkin
Given the deploy adapter intends loopback-only exposure behind host Caddy
When it writes `HOST_BIND_ADDRESS=127.0.0.1` into `app.env`
Then the rendered backend port mappings bind to `127.0.0.1:`
And the loopback value is traceable to adapter-provided configuration, not a compose fallback
```

### Scenario: Operations doc tells devops how to reach infra

```gherkin
Given the home-lab compose has no host port for postgres or nats
When a devops user reads `docs/Operations.md`
Then the document shows a `docker exec` command for `psql` against the
   `smackerel-home-lab-postgres` container (Pattern P1)
And the document shows a `docker exec` command for `nats` CLI access
And the document shows that core API and ML sidecar HTTPS access goes
   through host Caddy on `<deploy-host-fqdn>`
```

### Scenario: Copilot guardrail prevents regression to literal 127.0.0.1

```gherkin
Given a future agent reads `.github/copilot-instructions.md`
When that agent considers re-introducing literal `127.0.0.1:` prefixes in
   `deploy/compose.deploy.yml` for backend services
Then the "Tailnet-Edge Bind Pattern" subsection under Required Runtime
   Standards explicitly forbids it and points to this spec
```

## Acceptance Criteria

- AC-1 Running `./smackerel.sh test unit --go` passes including the new
  `internal/deploy/compose_contract_test.go` file.
- AC-2 Running `grep -nE '^\s+-\s+"127\.0\.0\.1:' deploy/compose.deploy.yml`
  returns ZERO matches for the `smackerel-core` and `smackerel-ml` blocks.
- AC-3 Running `grep -nF '${HOST_BIND_ADDRESS:-127.0.0.1}' deploy/compose.deploy.yml`
  returns ZERO matches. The fallback form may appear only in docs or tests that
  explicitly label it forbidden or superseded.
- AC-4 Running `grep -nE '^\s+ports:' deploy/compose.deploy.yml` shows no
  `ports:` line for `postgres` or `nats`. Any host-published service governed
  by the deploy contract uses the fail-loud `HOST_BIND_ADDRESS` substitution.
- AC-5 `internal/deploy/compose_contract_test.go` rejects all of these
  adversarial forms: literal `127.0.0.1:` backend prefixes,
  `${HOST_BIND_ADDRESS:-127.0.0.1}` backend prefixes, non-empty infra `ports:`
  blocks, and `network_mode: host` on contract services.
- AC-6 `./smackerel.sh check` exits 0.
- AC-7 `./smackerel.sh config generate` exits 0 and generated env artifacts
  contain an explicit `HOST_BIND_ADDRESS` value for generic/local contracts.
  Real target values remain adapter-owned.
- AC-8 `docker compose -f deploy/compose.deploy.yml config` without any
  `HOST_BIND_ADDRESS` value exits non-zero with the named fail-loud error; the
  same render with an explicit `HOST_BIND_ADDRESS=127.0.0.1` value produces
  loopback-bound backend mappings.
- AC-9 `bash .github/bubbles/scripts/artifact-lint.sh
  specs/042-tailnet-edge-bind-pattern` exits 0.

## Out-of-Repo Coordination

The deploy adapter at `knb/smackerel/home-lab/` will, in a separate change set:

- Render `bubbles/templates/caddy-tailnet-snippet.caddy.template` into a
  Caddyfile snippet on `<deploy-host>`.
- Wire that snippet into the host Caddy configuration on the tailnet IP.
- Add a basic-auth credential file behind the Caddy `_devops-auth.snippet`.

This spec produces a compose bundle that is **ready** for that adapter work
to consume but does not require the adapter to exist for the spec to be
correct.

Links: [design.md](design.md) | [scopes.md](scopes.md) |
[uservalidation.md](uservalidation.md) |
[../../bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md](../../bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md) (canonical pattern, in the bubbles framework repo)

## Superseded Historical Requirements

The original Spec 042 plan used `${HOST_BIND_ADDRESS:-127.0.0.1}` to preserve a
compose-level loopback default. That requirement is superseded by BUG-029-003
and the Smackerel NO-DEFAULTS policy. The fallback form is retained here only as
historical context and as a forbidden regression marker; it is not active
deploy guidance. Current deploy truth is fail-loud substitution plus an
adapter-provided explicit value.
