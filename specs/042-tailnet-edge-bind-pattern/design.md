# Design 042 — Tailnet-Edge Bind Pattern (Home-Lab Compose Readiness)

Links: [spec.md](spec.md) | [scopes.md](scopes.md) |
[uservalidation.md](uservalidation.md)

## Design Brief

### Current State

The analyst-owned spec now defines the deploy compose truth as fail-loud:
`deploy/compose.deploy.yml` must require
`${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` for
`smackerel-core` and `smackerel-ml`. Older Spec 042 design text still treated
the colon-dash loopback fallback form as the active deploy pattern, which
conflicts with the Smackerel NO-DEFAULTS / fail-loud SST policy.

### Target State

Spec 042 design uses one active contract: the deploy adapter writes an explicit
`HOST_BIND_ADDRESS` value into `app.env`, and Docker Compose aborts when that
value is absent or empty. `127.0.0.1` remains valid only when it is the explicit
adapter-provided value, not because Compose invented it through fallback syntax.

### Patterns to Follow

- `deploy/compose.deploy.yml` backend ports use fail-loud Compose substitution.
- `internal/deploy/compose_contract_test.go` is the mechanical contract guard
  for backend port syntax and no host-published infra ports.
- `.github/instructions/smackerel-no-defaults.instructions.md` is the binding
  policy for `HOST_BIND_ADDRESS` wording and fallback rejection.
- `docs/Operations.md` keeps product-repo examples generic with placeholders
  instead of real hostnames, IPs, tailnet roots, or operator-private topology.

### Patterns to Avoid

- Do not use the superseded colon-dash `HOST_BIND_ADDRESS` fallback form in
  active deploy design, Compose examples, or validation expectations.
- Do not describe the bind behavior as defaulting, implicitly loopback-bound,
  or fallback-preserved behavior.
- Do not hardcode real home-lab hostnames, IP addresses, tailnet identifiers,
  or deploy-adapter repository paths in this product repo.

### Resolved Decisions

- Keep the existing variable name `HOST_BIND_ADDRESS`.
- The deploy adapter owns the final concrete value for real targets.
- Missing or empty `HOST_BIND_ADDRESS` is a configuration error at Compose
  substitution time.
- Postgres and NATS remain in-network only with no host-published ports.

### Open Questions

- None for the design repair.

## Overview

This design locks `deploy/compose.deploy.yml` into the L3 invariants of the
canonical tailnet-edge bind pattern. The bundled deploy compose file becomes
**adapter-ready**: the deploy adapter supplies the exact host bind address for
backend ports before Compose starts, and infra services (Postgres, NATS) are
unreachable from outside the compose network. DevOps access flows through
`docker exec` over Tailscale SSH (Pattern P1).

This is a small, focused change. There is no new SST value, no new image, no
runtime behavior change for `dev` or `test`, and no adapter code in this
repo.

## Architecture

### The 3-Layer Tailnet-Edge Pattern (reference)

| Layer | Component                                | Bound To                  | Purpose |
|-------|------------------------------------------|---------------------------|---------|
| L1    | Host Caddy on `<deploy-host>`                   | Tailscale IP (<host-tailnet-ip>) | TLS termination via `tailscale cert`, optional basic-auth, reverse-proxy to L3 |
| L2    | (None for smackerel — backends are not behind in-container nginx) | n/a | n/a |
| L3    | `smackerel-core` and `smackerel-ml` host-published ports | `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` | Bind only to the explicit address supplied by the deploy adapter; missing or empty values abort Compose |

Infra services (`postgres`, `nats`) sit **inside** the compose network with
NO host port mapping. DevOps reaches them via:

- `tailscale ssh <deploy-host>` (or local SSH) → `docker exec -it <container> psql ...`
- `tailscale ssh <deploy-host>` → `docker exec -it <container> nats sub ...`

This is **Pattern P1** from
`bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md`. It is the recommended
default for CLI access because it requires zero host port exposure and reuses
the Tailscale identity (`tailscale ssh`) for authentication.

### File-by-File Edits

| File                            | Edit |
|---------------------------------|------|
| `deploy/compose.deploy.yml`     | (1) Change `smackerel-core` port mapping prefix from `127.0.0.1:` to `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`. (2) Same for `smackerel-ml`. (3) Delete the `ports:` block from `postgres` (replace with explanatory comment). (4) Delete the `ports:` block from `nats` (replace with explanatory comment). |
| `config/smackerel.yaml`         | Add a multi-line comment above `runtime.host_bind_address` cross-referencing the SKILL, the fail-loud deploy compose contract, and the adapter-owned explicit-value path. No value or schema change. |
| `internal/deploy/compose_contract_test.go` | Go unit test, package `deploy`, parses `deploy/compose.deploy.yml` with `gopkg.in/yaml.v3` and asserts the four invariants, including rejection of fallback syntax. |
| `.github/copilot-instructions.md` | Add a "Tailnet-Edge Bind Pattern (home-lab/production targets)" subsection inside the "Required Runtime Standards" section. |
| `docs/Operations.md`            | Add a "DevOps Access on Home-Lab (Tailnet-Edge Pattern)" section with concrete `docker exec` shapes for Postgres and NATS, plus the HTTPS Caddy URL flow. |

### What Is NOT Edited

- `docker-compose.yml` (dev/test compose): unchanged. Local dev still binds
  loopback ports for postgres/nats so devs can `psql` directly. The deploy
  compose has different needs.
- `scripts/commands/config.sh`: unchanged. `HOST_BIND_ADDRESS` is already
  exported at line 398 / 805 from `runtime.host_bind_address` in
  `config/smackerel.yaml`, and is already present in
  `config/generated/{dev,test,home-lab}.env`. Re-using the existing SST
  value is the explicit non-goal.
- `deploy/contract.yaml`: unchanged. Image list, signing requirements, and
  config-bundle environments are stable.
- `deploy/_example/target-skeleton/`: unchanged. The skeleton is a generic
  template; its compose-side guidance is now correctly satisfied by this
  spec.
- `specs/020-security-hardening/`: NOT edited. Spec 020 is a historical
  artifact. This spec's design.md §"Spec 020 Decision Reversal" makes the
  reversal normative going forward.
- `ollama` service in `deploy/compose.deploy.yml`: NOT edited.
  `ollama` is profile-gated (`profiles: [ollama]`) and is not started in
  the default home-lab deployment. If a future spec enables ollama on
  home-lab, that spec is responsible for migrating ollama to the same
  pattern.

## Spec 020 Decision Reversal And Spec 042 Fallback Supersession

Spec 020 design.md §"Port Binding: HOST_BIND_ADDRESS Variable in Compose"
recorded:

> **Decision:** Use literal `127.0.0.1` in `docker-compose.yml` port entries.
> The `HOST_BIND_ADDRESS` env var serves as SST audit trail and is available
> for future Compose templating if needed.

The rationale at the time was "Docker Compose environment variable
substitution in port strings has inconsistent behavior across Compose
versions, and a literal `127.0.0.1` is simpler, clearer, and guaranteed
correct."

This spec reverses that decision for `deploy/compose.deploy.yml` and also
supersedes Spec 042's original compose-level fallback design. Current active
deploy truth is:

1. **Deploy compose is fail-loud.** Backend port mappings use
   `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` so
   Compose refuses to render when the deploy adapter has not supplied a value.
2. **The adapter owns explicit values.** If loopback is intended, the adapter
   writes `HOST_BIND_ADDRESS=127.0.0.1` into `app.env`. If tailnet-edge
   fronting requires a different bind address, the adapter writes that value.
3. **The product repo remains generic.** Smackerel documents placeholders and
   contracts only; real hostnames, IPs, tailnet roots, and Caddy fragments are
   owned by the deploy adapter.
4. **Scope is bounded.** `docker-compose.yml` (dev/test) is not changed by
   this reversal. Only `deploy/compose.deploy.yml` (the deploy bundle) is
   affected. Spec 020's broader `127.0.0.1`-only stance for dev/test compose
   remains intact.

## Variable Naming Decision

The canonical pattern may use the conceptual name `HOST_BIND_ADDR`. The
smackerel SST already exports
the value under the longer name `HOST_BIND_ADDRESS` (config.sh line 398,
yaml `runtime.host_bind_address`, present in every generated env file).

**Decision: keep `HOST_BIND_ADDRESS`.** Reasons:

- Existing SST contract is already plumbed end-to-end. Renaming would touch
  config.sh, smackerel.yaml, all three generated env files, plus stale
  references in spec 020 design.md and bug.md history.
- The SKILL.md variable name `HOST_BIND_ADDR` is a pattern label, not a
  binding contract. Per-project naming is allowed and expected.
- deploy adapter authors will see in `docs/Operations.md` and in
  `.github/copilot-instructions.md` that the smackerel-specific name is
  `HOST_BIND_ADDRESS`. The adapter sets that name on every deploy; when
  loopback is intended, the adapter writes `127.0.0.1` explicitly and lets the
  host Caddy do the tailnet-IP binding.

If the project later decides to align with the SKILL's `HOST_BIND_ADDR`
short name, that is a separate, mechanical rename spec.

## Data Model

No data model changes.

## API/Contracts

No API or wire-protocol contract changes. The compose contract changes
described above are validated by the new unit test.

## UI/UX

Not applicable — no UI surface in this spec.

## Security/Compliance

This spec **improves** the network exposure posture of the home-lab deploy
bundle:

| Change                                | Pre-spec exposure                  | Post-spec exposure                  |
|---------------------------------------|-------------------------------------|--------------------------------------|
| `smackerel-core` host port             | Loopback (literal 127.0.0.1)        | Explicit adapter-provided bind address; Compose fails if missing |
| `smackerel-ml` host port               | Loopback (literal 127.0.0.1)        | Explicit adapter-provided bind address; Compose fails if missing |
| `postgres` host port                   | Loopback `${POSTGRES_HOST_PORT}`    | **None** (in-network only) |
| `nats` client + monitor host ports     | Loopback `${NATS_*_HOST_PORT}`      | **None** (in-network only) |
| `ollama` host port (profile-gated)     | Loopback (literal 127.0.0.1)        | Unchanged (out of scope) |

Removing infra host ports also closes a small operational footgun: a future
agent or operator cannot accidentally `psql -h 127.0.0.1 -p <pg_host_port>`
into the deploy compose's database from another tool that happens to be
reading the same `.env` file. All Postgres/NATS access on home-lab now
flows through `docker exec`, which is the auditable path.

The fail-loud substitution ensures deploy compose cannot start from an implicit
bind address. Local inspection must supply an explicit generated or
adapter-style `HOST_BIND_ADDRESS` value; missing values fail with the named
Compose error.

## Observability

No metrics or logs are added or removed. The new unit test produces a
single `PASS` line in `go test ./...` output when the compose contract
holds.

## Testing Strategy

### Unit (Go) — primary coverage

`internal/deploy/compose_contract_test.go` (new file, package
`deploy`) parses `deploy/compose.deploy.yml` using
`gopkg.in/yaml.v3` (already in `go.sum` line 194) and asserts:

1. `services.smackerel-core.ports[0]` matches the regex
   `^\$\{HOST_BIND_ADDRESS:\?HOST_BIND_ADDRESS must be set by deploy adapter\}:\$\{CORE_HOST_PORT\}:\$\{CORE_CONTAINER_PORT\}$`
2. `services.smackerel-ml.ports[0]` matches the regex
   `^\$\{HOST_BIND_ADDRESS:\?HOST_BIND_ADDRESS must be set by deploy adapter\}:\$\{ML_HOST_PORT\}:\$\{ML_CONTAINER_PORT\}$`
3. `services.postgres` has no `ports` field (or the field is the empty list).
4. `services.nats` has no `ports` field (or the field is the empty list).

The test runs as part of `./smackerel.sh test unit --go` (which executes
`go test ./...` per `scripts/runtime/go-unit.sh`). No new test
infrastructure is required.

### Local rendering check (manual proof — captured in scope DoD evidence)

`docker compose -f deploy/compose.deploy.yml config` is run as a one-shot
read-only inspection without `HOST_BIND_ADDRESS` to confirm Compose exits with
`HOST_BIND_ADDRESS must be set by deploy adapter`. A second render with an
explicit adapter-style value such as `HOST_BIND_ADDRESS=127.0.0.1` confirms the
backend port mappings render to loopback and that no `ports:` block appears
under `postgres` or `nats`. This is captured as evidence in `report.md`.

### Static guard — `./smackerel.sh check`

`./smackerel.sh check` is run to confirm the existing config-validation
path still exits 0 after the compose edits. No new check rules are added
in this spec.

### Adversarial regression (in the unit test)

The test includes an explicit adversarial assertion: a fixture-style
sub-test that loads a copy of the compose file with literal `127.0.0.1:`
in the `smackerel-core` port entry MUST cause the contract assertion to
FAIL. This proves the test would catch a regression to the spec 020
hardcoded form. A second adversarial assertion rejects the superseded
default-fallback form so the test would also catch a regression to the original
Spec 042 fallback design. This satisfies the bug-fix-style adversarial
requirement even though this is technically a forward-looking design change
rather than a bug fix.

### Out-of-scope test surfaces

- No `e2e-api` test — the spec does not change runtime API behavior.
- No `e2e-ui` test — there is no UI surface.
- No `integration` test — no service interaction surface change.
- No `stress` / `load` test — no latency or throughput SLA is defined.

The compose contract is a static-file invariant, so a unit-class lint
test is the correct and sufficient level. Adding higher-level tests
would not improve confidence and would add infrastructure dependencies
the spec does not need.

## Risks & Open Questions

### Risk: Compose v2 might re-introduce port substitution flakiness

**Likelihood:** Very low. Compose v2.x has shipped reliable port-string
substitution since 2.6 (2022). The repo runtime CLI pins Compose v2.
**Mitigation:** The unit test asserts the literal compose-source string
shape, not the rendered output. The manual `docker compose ... config`
evidence captures the rendered shape for record. If a Compose version
ever changes the substitution semantics, the unit test still proves the
source contract; a separate spec would address the rendering shift.

### Risk: An operator supplies an unintended public bind address

**Likelihood:** Very low. The value is project-specific and owned by the
deploy adapter, not by ad-hoc product-repo commands.
**Mitigation:** Missing values fail loudly, explicit loopback must be written
as `127.0.0.1`, and product-repo docs stay generic.
`.github/copilot-instructions.md` documents the contract so later agents do not
reintroduce fallback syntax or real target topology.

### Open Question: When (if ever) does smackerel use Pattern P5 (host Caddy + basic_auth) instead of Pattern P1 (docker exec) for Postgres/NATS access?

Today: never. The two infra services have no plain-HTTP UI and no
business case for being fronted by Caddy. Pattern P1 is canonical for
both. If a future spec adds a Postgres web UI (e.g., pgweb) or a NATS
dashboard, that spec would consume the canonical Caddy template and
add the appropriate Caddy block. Out of scope for this spec.

## Reconciliation Notes

- Spec 042's original compose-level loopback fallback design is superseded.
  Active deploy design now requires fail-loud `HOST_BIND_ADDRESS` substitution
  plus an adapter-provided explicit value.
- Spec 020 §"Port Binding: HOST_BIND_ADDRESS Variable in Compose" is
  reversed for `deploy/compose.deploy.yml` only. Spec 020's reasoning
  for using literal `127.0.0.1` in `docker-compose.yml` (dev/test) is
  unaffected.
- The Bubbles SKILL `bubbles-tailnet-edge-pattern` (in the framework
  repo) is the canonical pattern source. This spec implements the L3
  contract for smackerel.
- The deploy-adapter spec (separate repo, separate spec) will consume
  `bubbles/templates/caddy-tailnet-snippet.caddy.template` and write the
  L1 Caddy snippet. Adapter Locality
  (`.github/instructions/bubbles-deployment-target.instructions.md`)
  forbids that work from happening in this repo.

## Superseded Design Decisions

The original Spec 042 design used `${HOST_BIND_ADDRESS:-127.0.0.1}` to keep a
compose-level loopback fallback when no bind address was provided. That design
is superseded by the Smackerel NO-DEFAULTS policy and the current analyst-owned
spec. The fallback form is retained here only as historical context and as a
forbidden regression marker; it is not active deploy guidance.
