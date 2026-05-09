# Design 042 — Tailnet-Edge Bind Pattern (Home-Lab Compose Readiness)

Links: [spec.md](spec.md) | [scopes.md](scopes.md) |
[uservalidation.md](uservalidation.md)

## Overview

This design locks `deploy/compose.deploy.yml` into the L3 invariants of the
canonical tailnet-edge bind pattern. The bundled deploy compose file becomes
**adapter-ready**: a host-level Caddy on the tailnet IP can front the backends
without any further edits to the bundle, and infra services (Postgres, NATS)
are unreachable from outside the compose network. DevOps access flows through
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
| L3    | `smackerel-core` and `smackerel-ml` host-published ports | `${HOST_BIND_ADDRESS:-127.0.0.1}` | Receive traffic only from the host loopback (or whichever NIC the operator chooses) |

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
| `deploy/compose.deploy.yml`     | (1) Change `smackerel-core` port mapping prefix from `127.0.0.1:` to `${HOST_BIND_ADDRESS:-127.0.0.1}:`. (2) Same for `smackerel-ml`. (3) Delete the `ports:` block from `postgres` (replace with explanatory comment). (4) Delete the `ports:` block from `nats` (replace with explanatory comment). |
| `config/smackerel.yaml`         | Add a multi-line comment above `runtime.host_bind_address` cross-referencing the SKILL and explaining how a deploy adapter overrides the value. No value or schema change. |
| `internal/deploy/compose_contract_test.go` | New Go unit test, package `deploy`, parses `deploy/compose.deploy.yml` with `gopkg.in/yaml.v3` and asserts the four invariants. |
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

## Spec 020 Decision Reversal

Spec 020 design.md §"Port Binding: HOST_BIND_ADDRESS Variable in Compose"
recorded:

> **Decision:** Use literal `127.0.0.1` in `docker-compose.yml` port entries.
> The `HOST_BIND_ADDRESS` env var serves as SST audit trail and is available
> for future Compose templating if needed.

The rationale at the time was "Docker Compose environment variable
substitution in port strings has inconsistent behavior across Compose
versions, and a literal `127.0.0.1` is simpler, clearer, and guaranteed
correct."

This spec **reverses that decision for `deploy/compose.deploy.yml`** (the
home-lab/production deploy compose). Reasons:

1. **Compose v2 substitution in port strings is reliable.** Docker Compose
   v2 (the only version this project targets — see `smackerel.sh` runtime
   contract) supports `${VAR:-default}` substitution inside `ports:` entries
   with consistent behavior. The 2025 Compose-version-fragility concern no
   longer applies.
2. **The pattern is canonical.** The Bubbles framework now ships a
   tailnet-edge bind pattern SKILL and a Caddy template that assume the
   compose backend ports use a substitutable address. Hardcoding `127.0.0.1`
   prevents the pattern from being used.
3. **The default is preserved.** `${HOST_BIND_ADDRESS:-127.0.0.1}` falls
   back to `127.0.0.1` if the env var is unset, so any local
   `docker compose -f deploy/compose.deploy.yml up` (without env setup) is
   identical in behavior to today.
4. **Scope is bounded.** `docker-compose.yml` (dev/test) is not changed by
   this reversal. Only `deploy/compose.deploy.yml` (the deploy bundle) is
   affected. Spec 020's broader `127.0.0.1`-only stance for dev/test
   compose remains intact.

## Variable Naming Decision

The canonical SKILL.md uses the conceptual name `HOST_BIND_ADDR` ("backends
bind `${HOST_BIND_ADDR:-127.0.0.1}`"). The smackerel SST already exports
the value under the longer name `HOST_BIND_ADDRESS` (config.sh line 398,
yaml `runtime.host_bind_address`, present in every generated env file).

**Decision: keep `HOST_BIND_ADDRESS`.** Reasons:

- Existing SST contract is already plumbed end-to-end. Renaming would touch
  config.sh, smackerel.yaml, all three generated env files, plus stale
  references in spec 020 design.md and bug.md history.
- The SKILL.md variable name `HOST_BIND_ADDR` is a pattern label, not a
  binding contract. Per-project naming is allowed and expected.
- deploy adapter adapter authors will see in `docs/Operations.md` and in
  `.github/copilot-instructions.md` that the smackerel-specific name is
  `HOST_BIND_ADDRESS`. The adapter sets that name when it wants to override
  the loopback default (uncommon — most home-lab deployments leave it at
  loopback and let the host Caddy do the tailnet-IP binding).

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
| `smackerel-core` host port             | Loopback (literal 127.0.0.1)        | Loopback by default; configurable to a NIC by the deploy adapter |
| `smackerel-ml` host port               | Loopback (literal 127.0.0.1)        | Loopback by default; configurable to a NIC by the deploy adapter |
| `postgres` host port                   | Loopback `${POSTGRES_HOST_PORT}`    | **None** (in-network only) |
| `nats` client + monitor host ports     | Loopback `${NATS_*_HOST_PORT}`      | **None** (in-network only) |
| `ollama` host port (profile-gated)     | Loopback (literal 127.0.0.1)        | Unchanged (out of scope) |

Removing infra host ports also closes a small operational footgun: a future
agent or operator cannot accidentally `psql -h 127.0.0.1 -p <pg_host_port>`
into the deploy compose's database from another tool that happens to be
reading the same `.env` file. All Postgres/NATS access on home-lab now
flows through `docker exec`, which is the auditable path.

The `${HOST_BIND_ADDRESS:-127.0.0.1}` default ensures the deploy compose
remains safe-by-default for any local `docker compose -f deploy/compose.deploy.yml up`
run a developer might do for testing.

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
   `^\$\{HOST_BIND_ADDRESS:-127\.0\.0\.1\}:\$\{CORE_HOST_PORT\}:\$\{CORE_CONTAINER_PORT\}$`
2. `services.smackerel-ml.ports[0]` matches the regex
   `^\$\{HOST_BIND_ADDRESS:-127\.0\.0\.1\}:\$\{ML_HOST_PORT\}:\$\{ML_CONTAINER_PORT\}$`
3. `services.postgres` has no `ports` field (or the field is the empty list).
4. `services.nats` has no `ports` field (or the field is the empty list).

The test runs as part of `./smackerel.sh test unit --go` (which executes
`go test ./...` per `scripts/runtime/go-unit.sh`). No new test
infrastructure is required.

### Local rendering check (manual proof — captured in scope DoD evidence)

`docker compose -f deploy/compose.deploy.yml --env-file
config/generated/home-lab.env config` is run as a one-shot read-only
inspection to confirm Compose-level substitution renders the expected
backend port mappings (`127.0.0.1:41001:8080` for core,
`127.0.0.1:41002:8081` for ml) and that no `ports:` block appears under
`postgres` or `nats`. This is captured as evidence in `report.md`.

### Static guard — `./smackerel.sh check`

`./smackerel.sh check` is run to confirm the existing config-validation
path still exits 0 after the compose edits. No new check rules are added
in this spec.

### Adversarial regression (in the unit test)

The test includes an explicit adversarial assertion: a fixture-style
sub-test that loads a copy of the compose file with literal `127.0.0.1:`
in the `smackerel-core` port entry MUST cause the contract assertion to
FAIL. This proves the test would catch a regression to the spec 020
hardcoded form. This satisfies the bug-fix-style adversarial requirement
even though this is technically a forward-looking design change rather
than a bug fix.

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

### Risk: An operator runs `docker compose -f deploy/compose.deploy.yml up` locally with `HOST_BIND_ADDRESS` accidentally set to a public IP

**Likelihood:** Very low. The env var is project-specific and only the
deploy adapter adapter sets it. A developer running the deploy compose locally
without the home-lab env file gets the `:-127.0.0.1` default.
**Mitigation:** The default substitution makes the safe value the
zero-config behavior. `.github/copilot-instructions.md` documents the
intent so future agents do not change the default to anything else.

### Open Question: When (if ever) does smackerel use Pattern P5 (host Caddy + basic_auth) instead of Pattern P1 (docker exec) for Postgres/NATS access?

Today: never. The two infra services have no plain-HTTP UI and no
business case for being fronted by Caddy. Pattern P1 is canonical for
both. If a future spec adds a Postgres web UI (e.g., pgweb) or a NATS
dashboard, that spec would consume the canonical Caddy template and
add the appropriate Caddy block. Out of scope for this spec.

## Reconciliation Notes

- Spec 020 §"Port Binding: HOST_BIND_ADDRESS Variable in Compose" is
  reversed for `deploy/compose.deploy.yml` only. Spec 020's reasoning
  for using literal `127.0.0.1` in `docker-compose.yml` (dev/test) is
  unaffected.
- The Bubbles SKILL `bubbles-tailnet-edge-pattern` (in the framework
  repo) is the canonical pattern source. This spec implements the L3
  contract for smackerel.
- The deploy adapter adapter spec (separate repo, separate spec) will consume
  `bubbles/templates/caddy-tailnet-snippet.caddy.template` and write the
  L1 Caddy snippet. Adapter Locality
  (`.github/instructions/bubbles-deployment-target.instructions.md`)
  forbids that work from happening in this repo.
