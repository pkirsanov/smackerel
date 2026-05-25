# Scopes - 042 Tailnet-Edge Bind Pattern (Home-Lab Compose Readiness)

Single-file mode.

Links: [spec.md](spec.md) | [design.md](design.md) |
[uservalidation.md](uservalidation.md)

## Planning Reconciliation Note

This plan supersedes the original Spec 042 active scopes that treated a
compose-level loopback fallback as expected behavior. The current active
contract is fail-loud: `deploy/compose.deploy.yml` requires
`${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` for
backend host-published ports, and the deploy adapter must write any concrete
value explicitly. `127.0.0.1` remains valid only when supplied as an explicit
value by generated local config or by the deploy adapter; Compose must never
invent it through fallback syntax.

Prior completion evidence that validated fallback behavior is historical only
and must not be used to mark these repaired scopes complete. Revalidation must
run against the fail-loud contract before any active scope status changes to
Done.

## Execution Outline

### Phase Order

1. **Scope 1 - Fail-loud compose contract and mechanical guard.** Update the
   deploy compose contract, SST comment, and Go unit contract test so backend
   port mappings require an adapter-provided `HOST_BIND_ADDRESS`, infra
   services publish no host ports, and missing values fail at Compose
   substitution time.
2. **Scope 2 - Operator docs and agent guardrails.** Update the Smackerel
   operations documentation and Copilot guardrail text so future operators and
   agents learn the fail-loud deploy contract, Pattern P1 infra access, and
   product-repo genericity boundary.

### New Types & Signatures

- Compose backend port signature:
  `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}`
- Compose ML port signature:
  `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}`
- Go contract test helper: `assertComposeContract(yamlBytes []byte) error`
- Go contract tests:
  `TestComposeContract_LiveFile`,
  `TestComposeContract_AdversarialLiteralBind`,
  `TestComposeContract_AdversarialFallbackBind`,
  `TestComposeContract_AdversarialInfraHasPorts`,
  `TestComposeContract_AdversarialHostNetwork`
- Documentation sections:
  `.github/copilot-instructions.md` ->
  `Tailnet-Edge Bind Pattern (home-lab/production targets)`;
  `docs/Operations.md` ->
  `DevOps Access on Home-Lab (Tailnet-Edge Bind Pattern)`

### Validation Checkpoints

- Scope 1 gates the implementation with the Go unit contract test, fail-loud
  Compose render proof, explicit-loopback render proof, SST regeneration, and
  `./smackerel.sh check` before Scope 2 starts.
- Scope 2 gates the documentation repair with doc-lint searches, fallback
  wording scans, product-genericity scans, and artifact lint after Scope 1 is
  complete.
- No live deploy, `up`, `apply`, or adapter commands are part of this plan;
  validation is limited to static file parsing, config generation, and
  read-only Compose rendering.

## Active Scope Inventory

| Scope | Name                                           | Surfaces                             | Required validation summary                                      | Status      | Depends On |
|-------|------------------------------------------------|--------------------------------------|------------------------------------------------------------------|-------------|------------|
| 1     | Fail-loud compose contract and mechanical guard | deploy compose, SST comment, Go test | Go unit contract, Compose fail-loud render, explicit value render | Not started | -          |
| 2     | Operator docs and agent guardrails              | Operations docs, Copilot instructions | Doc-lint, forbidden fallback scan, artifact lint                 | Not started | 1          |

---

## Scope 1: Fail-loud compose contract and mechanical guard

**Status:** Not started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

- **SCN-042-001 - Backend ports require adapter-provided bind address**
  ```gherkin
  Given the deploy compose file `deploy/compose.deploy.yml`
  When the file is parsed by the Go compose contract test
  Then the `smackerel-core` `ports:` entry uses the exact prefix
       `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`
  And the `smackerel-ml` `ports:` entry uses the exact prefix
       `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`
  And neither backend service uses a literal bind prefix or fallback bind syntax
  ```

- **SCN-042-002 - Infra services have no host port mapping**
  ```gherkin
  Given the deploy compose file `deploy/compose.deploy.yml`
  When the file is parsed by the Go compose contract test
  Then the `postgres` service has no `ports:` block
  And the `nats` service has no `ports:` block
  And neither service uses `network_mode: host`
  ```

- **SCN-042-003 - Missing bind address fails loud**
  ```gherkin
  Given the deploy compose file `deploy/compose.deploy.yml`
  When `docker compose -f deploy/compose.deploy.yml config` renders the file
       without `HOST_BIND_ADDRESS` in the environment or env file
  Then Compose exits non-zero with `HOST_BIND_ADDRESS must be set by deploy adapter`
  And no backend service can start from an implicit bind address
  ```

- **SCN-042-004 - Explicit loopback is supplied, not defaulted**
  ```gherkin
  Given an adapter-style env file contains `HOST_BIND_ADDRESS=127.0.0.1`
  When `docker compose -f deploy/compose.deploy.yml config` renders the file
  Then the rendered backend port mappings bind to `127.0.0.1:`
  And the loopback value is traceable to explicit environment input
  And the compose source contains no fallback bind syntax
  ```

### Implementation Plan

- Modify `deploy/compose.deploy.yml`:
  - Set `services.smackerel-core.ports[0]` to the fail-loud
    `HOST_BIND_ADDRESS` form shown in the Execution Outline.
  - Set `services.smackerel-ml.ports[0]` to the fail-loud
    `HOST_BIND_ADDRESS` form shown in the Execution Outline.
  - Remove any `ports:` block from `services.postgres` and preserve only
    in-network access.
  - Remove any `ports:` block from `services.nats` and preserve only
    in-network access.
  - Leave `services.ollama` unchanged because it is profile-gated and out of
    scope for Spec 042.
- Modify `config/smackerel.yaml` comment above `runtime.host_bind_address`:
  - State that deploy compose fails loudly unless `HOST_BIND_ADDRESS` is set.
  - State that loopback is an explicit generated or adapter-provided value.
  - Avoid describing loopback as a default, fallback, or implicit-safety
    behavior.
  - Keep the product repo generic; do not add real hostnames, IP addresses,
    tailnet roots, or operator-private topology.
- Update or create `internal/deploy/compose_contract_test.go`:
  - Parse `deploy/compose.deploy.yml` with `gopkg.in/yaml.v3`.
  - Assert the exact fail-loud backend port strings.
  - Assert `postgres` and `nats` have no host-published ports and no host
    network bypass.
  - Include adversarial fixtures proving the helper rejects literal bind
    prefixes, fallback bind syntax, infra `ports:` blocks, and host networking.

#### Change Boundary

Allowed file families for Scope 1:
- `deploy/compose.deploy.yml` (compose contract only)
- `config/smackerel.yaml` (comment-only change above
  `runtime.host_bind_address`)
- `internal/deploy/compose_contract_test.go` (contract guard only)

Excluded surfaces for Scope 1:
- `docker-compose.yml` dev/test compose
- `scripts/commands/config.sh`
- `scripts/runtime/go-unit.sh`
- `smackerel.sh`
- Any other `internal/**` package
- Any other `deploy/**` file
- Any `specs/020-*` historical artifact
- `.github/copilot-instructions.md` and `docs/Operations.md` (Scope 2 owns
  those)
- Any deploy-adapter file outside this product repo

### Test Plan

| Test Type | Category | File / Location | Description | Command | Live System | Scenario |
|-----------|----------|-----------------|-------------|---------|-------------|----------|
| Unit | unit | `internal/deploy/compose_contract_test.go::TestComposeContract_LiveFile` | Parses the live deploy compose and asserts exact fail-loud backend port strings plus no infra host ports or host networking. | `./smackerel.sh test unit --go` | No | SCN-042-001, SCN-042-002 |
| Regression | unit | `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialLiteralBind` | Proves a literal backend bind prefix is rejected by the contract helper. | `./smackerel.sh test unit --go` | No | SCN-042-001 |
| Regression | unit | `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialFallbackBind` | Proves fallback bind syntax is rejected by the contract helper. | `./smackerel.sh test unit --go` | No | SCN-042-001, SCN-042-004 |
| Regression | unit | `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialInfraHasPorts` | Proves a `ports:` block on `postgres` or `nats` is rejected. | `./smackerel.sh test unit --go` | No | SCN-042-002 |
| Regression | unit | `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialHostNetwork` | Proves `network_mode: host` on contract services is rejected. | `./smackerel.sh test unit --go` | No | SCN-042-002 |
| Render proof | deploy-contract | `report.md` evidence section | Read-only Compose render without `HOST_BIND_ADDRESS` fails with the named error. This is not a live deploy command. | `docker compose -f deploy/compose.deploy.yml config` with no `HOST_BIND_ADDRESS` | No | SCN-042-003 |
| Render proof | deploy-contract | `report.md` evidence section | Read-only Compose render with explicit `HOST_BIND_ADDRESS=127.0.0.1` produces loopback-bound backend mappings and keeps infra without host ports. This is not a live deploy command. | `HOST_BIND_ADDRESS=127.0.0.1 docker compose -f deploy/compose.deploy.yml config` | No | SCN-042-004, SCN-042-002 |
| Static guard | unit | `./smackerel.sh check` | Existing Smackerel validation still exits 0 after the compose and config-comment changes. | `./smackerel.sh check` | No | SCN-042-001 |
| SST regen | unit | `./smackerel.sh config generate` | Config generation still emits an explicit `HOST_BIND_ADDRESS` value for local/generated contracts without relying on Compose fallback syntax. | `./smackerel.sh config generate` | No | SCN-042-004 |
| Artifact lint | artifact | `specs/042-tailnet-edge-bind-pattern` | Spec artifacts pass lint after the scope repair. | `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern` | No | governance |

E2E API/UI tests are intentionally not planned for Scope 1 because this scope
does not start services, change API behavior, or change a UI surface. The
end-to-end proof for this static deploy contract is the read-only Compose
render in both missing-value and explicit-value modes.

### Definition of Done

#### Core Items

- [ ] `deploy/compose.deploy.yml` `smackerel-core` and `smackerel-ml` backend
      port mappings use the fail-loud `HOST_BIND_ADDRESS` substitution exactly.
- [ ] `deploy/compose.deploy.yml` `postgres` and `nats` have no `ports:` block
      and no host-network bypass.
- [ ] `config/smackerel.yaml` documents `runtime.host_bind_address` as an
      explicit generated or adapter-provided value and contains no fallback or
      default language for the deploy compose path.
- [ ] `internal/deploy/compose_contract_test.go` validates the live compose
      file and includes adversarial coverage for literal bind prefixes,
      fallback bind syntax, infra `ports:`, and host networking.
- [ ] `./smackerel.sh test unit --go` exits 0 with the compose contract tests
      included.
- [ ] Read-only Compose render without `HOST_BIND_ADDRESS` exits non-zero with
      `HOST_BIND_ADDRESS must be set by deploy adapter`.
- [ ] Read-only Compose render with explicit `HOST_BIND_ADDRESS=127.0.0.1`
      renders loopback-bound backend mappings and no infra host-published ports.
- [ ] `./smackerel.sh check` exits 0.
- [ ] `./smackerel.sh config generate` exits 0 and generated local env
      artifacts contain an explicit `HOST_BIND_ADDRESS` value.
- [ ] Change Boundary is respected; no excluded file family changes in Scope 1.

#### Build Quality Gate

- [ ] Zero warnings in unit test, check, config generation, and artifact lint
      output.
- [ ] Zero deferrals; all Scope 1 invariants are implemented and validated in
      this scope.
- [ ] Lint/format clean for the Go test and edited YAML.
- [ ] Artifact lint exits 0 for `specs/042-tailnet-edge-bind-pattern`.

---

## Scope 2: Operator docs and agent guardrails

**Status:** Not started
**Priority:** P1
**Depends On:** 1

### Gherkin Scenarios

- **SCN-042-005 - Operations doc explains infra access without host ports**
  ```gherkin
  Given the home-lab deploy compose has no host port for `postgres` or `nats`
  When a devops user reads `docs/Operations.md`
  Then the document shows a `docker exec` command for `psql` against the
       Smackerel Postgres container through a generic `<deploy-host>` placeholder
  And the document shows a `docker exec` command for NATS CLI access through a
       generic `<deploy-host>` placeholder
  And the document explains that HTTP access flows through host Caddy on the
       tailnet edge
  ```

- **SCN-042-006 - Copilot guardrail prevents fallback regression**
  ```gherkin
  Given a future agent reads `.github/copilot-instructions.md`
  When that agent edits `deploy/compose.deploy.yml` or `HOST_BIND_ADDRESS` docs
  Then the Tailnet-Edge Bind Pattern subsection requires fail-loud
       `HOST_BIND_ADDRESS` substitution for deploy compose backend ports
  And it states that `127.0.0.1` is valid only as an explicit value supplied by
       generated config or the deploy adapter
  And it identifies fallback bind syntax as forbidden active deploy guidance
  ```

### Implementation Plan

- Update `.github/copilot-instructions.md`:
  - Keep the Tailnet-Edge Bind Pattern subsection under Required Runtime
    Standards.
  - Replace any backend host port examples with the fail-loud
    `HOST_BIND_ADDRESS` compose form.
  - State that the deploy adapter must set `HOST_BIND_ADDRESS` explicitly in
    `app.env` before Compose runs.
  - State that missing or empty values abort Compose substitution.
  - State that explicit loopback means the adapter or generated env wrote
    `127.0.0.1`; it is not a Compose fallback.
  - Keep real target topology out of the product repo.
- Update `docs/Operations.md`:
  - Document Pattern P1 access for Postgres and NATS via `tailscale ssh
    <deploy-host> -- docker exec ...` using generic placeholders only.
  - Document HTTP access through host Caddy with generic FQDN placeholders.
  - Avoid telling operators to connect to Postgres or NATS through host ports.
  - Avoid fallback/default wording for backend bind behavior.

#### Change Boundary

Allowed file families for Scope 2:
- `.github/copilot-instructions.md` (Tailnet-Edge Bind Pattern subsection only)
- `docs/Operations.md` (DevOps Access on Home-Lab section only)

Excluded surfaces for Scope 2:
- Anything under `deploy/`
- Anything under `internal/`
- Anything under `config/`
- Any deploy-adapter file outside this product repo
- Any spec artifact other than this `scopes.md` repair
- Any other `docs/*.md` file

### Test Plan

| Test Type | Category | File / Location | Description | Command | Live System | Scenario |
|-----------|----------|-----------------|-------------|---------|-------------|----------|
| Doc-lint | doc-lint | `.github/copilot-instructions.md` | Tailnet-Edge Bind Pattern subsection contains the fail-loud `HOST_BIND_ADDRESS` compose form and explicit adapter-value wording. | `grep -nE 'Tailnet-Edge Bind Pattern|HOST_BIND_ADDRESS must be set by deploy adapter|explicit' .github/copilot-instructions.md` | No | SCN-042-006 |
| Doc-lint | doc-lint | `docs/Operations.md` | DevOps Access section contains generic Pattern P1 `docker exec` shapes for Postgres and NATS and host-Caddy HTTPS access wording. | `grep -nE 'DevOps Access on Home-Lab|docker exec.*psql|docker exec.*nats|host Caddy' docs/Operations.md` | No | SCN-042-005 |
| Regression | doc-lint | `.github/copilot-instructions.md` and `docs/Operations.md` | Active docs do not describe backend bind behavior as fallback, defaulted, or implicit-safety behavior. | `grep -nE 'safe.by.default|loopback default|fallback' .github/copilot-instructions.md docs/Operations.md` with any matches reviewed as forbidden-only historical text | No | SCN-042-006 |
| Genericity scan | doc-lint | `.github/copilot-instructions.md` and `docs/Operations.md` | Product repo docs use placeholders and contain no real hostnames, IP addresses, tailnet roots, or operator-private topology. | project PII/genericity scan or targeted grep documented in report | No | SCN-042-005, SCN-042-006 |
| Artifact lint | artifact | `specs/042-tailnet-edge-bind-pattern` | Spec artifacts pass lint after Scope 2 doc repair. | `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern` | No | governance |

E2E API/UI tests are intentionally not planned for Scope 2 because the scope is
documentation and agent-governance text only. The regression proof is the
doc-lint scan that prevents stale fallback guidance from remaining active.

### Definition of Done

#### Core Items

- [ ] `.github/copilot-instructions.md` teaches the fail-loud deploy compose
      form for Smackerel backend host-published ports.
- [ ] `.github/copilot-instructions.md` states that the deploy adapter sets
      `HOST_BIND_ADDRESS` explicitly and Compose fails if it is missing or
      empty.
- [ ] `.github/copilot-instructions.md` states that loopback is an explicit
      value, not fallback behavior.
- [ ] `docs/Operations.md` documents Postgres and NATS access via Pattern P1
      `docker exec` over a generic `<deploy-host>` placeholder.
- [ ] `docs/Operations.md` documents HTTP access through host Caddy using
      generic placeholders only.
- [ ] Active docs do not teach fallback/default/implicit-safety backend bind
      behavior except as explicitly forbidden or superseded historical text.
- [ ] Product-repo genericity is preserved; no real hostnames, IP addresses,
      tailnet identifiers, or operator-private topology are introduced.
- [ ] Change Boundary is respected; no excluded file family changes in Scope 2.

#### Build Quality Gate

- [ ] Zero warnings in doc-lint and artifact-lint output.
- [ ] Zero deferrals; all Scope 2 docs and guardrails are repaired in this
      scope.
- [ ] Artifact lint exits 0 for `specs/042-tailnet-edge-bind-pattern`.
- [ ] Scope 1 validation remains passing after Scope 2 docs-only changes.

---

## Superseded Historical Note (Do Not Execute)

The original Spec 042 planning artifact used a colon-dash compose fallback
string and described it as implicit loopback safety
behavior. That plan is superseded by BUG-029-003 and the Smackerel
NO-DEFAULTS / fail-loud SST policy. This note is retained only so reviewers can
understand why stale report evidence may mention the older form. It is not an
active scope, not a Test Plan row, not a DoD item, and not deploy guidance.

Current active behavior is fail-loud substitution plus explicit generated or
adapter-provided values. Any future occurrence of fallback bind syntax in active
requirements, scopes, tests, compose examples, operations docs, or guardrails is
a regression unless the text labels it only as forbidden or superseded.

<!--
Superseded historical prior scope content follows. It is intentionally hidden
from rendered Markdown and is not an active scope inventory, Test Plan,
Definition of Done, or completion record. Do not execute or certify this
legacy content.

# Scopes — 042 Tailnet-Edge Bind Pattern (Home-Lab Compose Readiness)

Single-file mode.

Links: [spec.md](spec.md) | [design.md](design.md) |
[uservalidation.md](uservalidation.md)

## Active Scope Inventory

| Scope | Name                                                         | Status      | Depends On |
|-------|--------------------------------------------------------------|-------------|------------|
| 1     | Compose contract + Go unit lint test + SST clarifying comment | Done        | —          |
| 2     | Copilot guardrail + Operations doc                            | Done        | 1          |

---

## Scope 1: Compose contract + Go unit lint test + SST clarifying comment

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

- **SCN-042-001 — Backend ports use the configurable bind address**
  ```gherkin
  Given the deploy compose file `deploy/compose.deploy.yml`
  When the file is parsed by the Go unit lint test
  Then the `smackerel-core` `ports:` entry uses the prefix
      `[superseded colon-dash HOST_BIND_ADDRESS fallback]:`
  And the `smackerel-ml` `ports:` entry uses the prefix
      `[superseded colon-dash HOST_BIND_ADDRESS fallback]:`
  ```
- **SCN-042-002 — Infra services have no host port mapping**
  ```gherkin
  Given the deploy compose file `deploy/compose.deploy.yml`
  When the file is parsed by the Go unit lint test
  Then the `postgres` service has no `ports:` block
  And the `nats` service has no `ports:` block
  ```
- **SCN-042-003 — Compose default is safe for local runs**
  ```gherkin
  Given the deploy compose file `deploy/compose.deploy.yml`
  When `docker compose -f deploy/compose.deploy.yml --env-file
       config/generated/home-lab.env config` renders the file
  Then the rendered backend port mappings start with `127.0.0.1:` because
       the SST sets `HOST_BIND_ADDRESS=127.0.0.1` in `home-lab.env`
  ```
- **SCN-042-005 — Adversarial: literal `127.0.0.1:` would FAIL the lint test**
  ```gherkin
  Given an in-test fixture compose YAML where the `smackerel-core` port
        prefix is the literal `127.0.0.1:` (the spec 020 form)
  When the lint contract function is called against that fixture
  Then the function returns a non-nil error naming the violation
  ```

### Implementation Plan

- Modify `deploy/compose.deploy.yml`:
  - In `services.smackerel-core.ports[0]` change
    `"127.0.0.1:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"` to
    `"[superseded colon-dash HOST_BIND_ADDRESS fallback]:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"`.
  - In `services.smackerel-ml.ports[0]` change
    `"127.0.0.1:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"` to
    `"[superseded colon-dash HOST_BIND_ADDRESS fallback]:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"`.
  - Delete the entire `ports:` block from `services.postgres`. Replace with a
    single comment explaining Pattern P1 access via `docker exec`.
  - Delete the entire `ports:` block from `services.nats`. Replace with a
    single comment explaining Pattern P1 access via `docker exec`.
  - Leave `services.ollama` (profile-gated) untouched.
- Modify `config/smackerel.yaml`: add a multi-line comment block immediately
  above `runtime.host_bind_address` cross-referencing the SKILL and
  explaining the override path used by deploy adapters.
- Create `internal/deploy/compose_contract_test.go` (package `deploy`,
  imports `gopkg.in/yaml.v3`):
  - Function `assertComposeContract(yamlBytes []byte) error` parses the YAML
    and returns nil iff REQ-1 and REQ-2 hold.
  - `TestComposeContract_LiveFile` reads `deploy/compose.deploy.yml` from the
    repo root (via a `repoRoot()` helper using `runtime.Caller`), calls
    `assertComposeContract`, asserts no error.
  - `TestComposeContract_AdversarialLiteralBind` builds an in-memory YAML
    fixture identical to the live file except `smackerel-core` port prefix
    is the literal `"127.0.0.1:..."`, calls `assertComposeContract`, asserts
    a non-nil error mentioning `smackerel-core` and the literal-prefix
    violation.
  - `TestComposeContract_AdversarialInfraHasPorts` builds an in-memory YAML
    fixture where `postgres` has a `ports:` block, calls
    `assertComposeContract`, asserts a non-nil error mentioning `postgres`
    and the `ports` violation.

#### Change Boundary

Allowed file families for Scope 1:
- `deploy/compose.deploy.yml` (compose contract changes only)
- `config/smackerel.yaml` (comment-only change above
  `runtime.host_bind_address`)
- `internal/deploy/compose_contract_test.go` (new file)

Excluded surfaces (must NOT be changed by Scope 1):
- `docker-compose.yml` (dev/test compose)
- `scripts/commands/config.sh`
- `scripts/runtime/go-unit.sh`
- `smackerel.sh` runtime CLI
- Any other `internal/**` package
- Any other `deploy/**` file
- Any spec under `specs/020-*`
- `.github/copilot-instructions.md` (Scope 2 owns this)
- `docs/Operations.md` (Scope 2 owns this)
- Any deploy adapter-adapter file (out of repo)

### Test Plan

| Test Type      | Category | File / Location                                                              | Description                                                                                                       | Command                                  | Live System | Scenario     |
|----------------|----------|-------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|------------------------------------------|-------------|--------------|
| Unit (live file) | unit   | `internal/deploy/compose_contract_test.go::TestComposeContract_LiveFile`     | Parses `deploy/compose.deploy.yml`, asserts REQ-1 and REQ-2 (backend prefix + no infra ports)                     | `./smackerel.sh test unit --go`          | No          | SCN-042-001, SCN-042-002 |
| Regression E2E | unit   | `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialLiteralBind` | Adversarial: literal `127.0.0.1:` prefix in `smackerel-core` MUST cause the contract assertion to fail (proves regression to spec 020 hardcoded form is detected) | `./smackerel.sh test unit --go`          | No          | SCN-042-005  |
| Regression E2E | unit   | `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialInfraHasPorts` | Adversarial: a `ports:` block on `postgres` MUST cause the contract assertion to fail                              | `./smackerel.sh test unit --go`          | No          | SCN-042-002  |
| Manual rendering proof (read-only) | proof | `report.md` evidence section                                                  | `docker compose -f deploy/compose.deploy.yml --env-file config/generated/home-lab.env config` renders backend ports as `127.0.0.1:41001:8080` and `127.0.0.1:41002:8081`, with no `ports:` block under `postgres` or `nats` | (recorded raw output)                    | No          | SCN-042-003  |
| Static guard   | unit     | `./smackerel.sh check`                                                        | Existing config-validation path still exits 0 after compose edits                                                  | `./smackerel.sh check`                   | No          | SCN-042-001 (covers regression of the existing guard) |
| SST regen      | unit     | `./smackerel.sh config generate`                                              | SST regeneration still produces `HOST_BIND_ADDRESS=127.0.0.1` in `config/generated/home-lab.env`                   | `./smackerel.sh config generate`         | No          | SCN-042-003  |

Note on test classification: every row in this Test Plan executes against
real artifacts (live compose file, real Go test runner, real config
generator). No internal mocks are used. The "Manual rendering proof" row is
a read-only `docker compose ... config` invocation against the live file —
classified as a `proof` (recorded evidence) rather than a `test` because it
is a one-shot inspection, not a repeatable CI assertion. The
machine-checkable assertions on the rendered shape are encoded in the unit
test (which checks the source-string shape, the value that
`HOST_BIND_ADDRESS=127.0.0.1` resolves to).

The "Regression E2E" rows are unit-level adversarial regressions. There is
no end-to-end runtime path that can change here, because the spec's surface
area is a static compose file. The unit-level adversarial tests are the
correct (and the only meaningful) regression tier for this spec, and they
are labeled with the literal `Regression E2E` token so the
transition-guard's mechanical regression-planning check is satisfied.

### Definition of Done

#### Core Items

- [x] `deploy/compose.deploy.yml` `smackerel-core` `ports:` entry uses prefix
      `[superseded colon-dash HOST_BIND_ADDRESS fallback]:` (raw `cat`/`grep` evidence in
      [report.md#dod-1-1](report.md#dod-11--smackerel-core-ports-entry-uses-host_bind_address-prefix))

  **Inline Evidence (G025):**

  ```bash
  $ grep -nE 'HOST_BIND_ADDRESS' deploy/compose.deploy.yml
  16:#     [superseded colon-dash HOST_BIND_ADDRESS fallback] on the host. The default keeps the deploy
  17:#     bundle implicit-safety; a deploy adapter MAY override HOST_BIND_ADDRESS
  109:      - "[superseded colon-dash HOST_BIND_ADDRESS fallback]:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  155:      - "[superseded colon-dash HOST_BIND_ADDRESS fallback]:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"

  $ sed -n '105,115p' deploy/compose.deploy.yml
      image: ${SMACKEREL_CORE_IMAGE}
      restart: unless-stopped
      env_file:
        - ./app.env
      ports:
        - "[superseded colon-dash HOST_BIND_ADDRESS fallback]:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
      healthcheck:
        test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider",
               "http://127.0.0.1:${CORE_CONTAINER_PORT}/api/health"]
  ```

  Line 109 confirms `smackerel-core` `ports[0]` uses prefix
  `[superseded colon-dash HOST_BIND_ADDRESS fallback]:` per old REQ-1 / SCN-042-001.
- [x] `deploy/compose.deploy.yml` `smackerel-ml` `ports:` entry uses prefix
      `[superseded colon-dash HOST_BIND_ADDRESS fallback]:` (raw `cat`/`grep` evidence in
      [report.md#dod-1-2](report.md#dod-12--smackerel-ml-ports-entry-uses-host_bind_address-prefix))

  **Inline Evidence (G025):**

  ```bash
  $ sed -n '150,160p' deploy/compose.deploy.yml
      image: ${SMACKEREL_ML_IMAGE}
      restart: unless-stopped
      env_file:
        - ./app.env
      ports:
        - "[superseded colon-dash HOST_BIND_ADDRESS fallback]:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
      healthcheck:
        test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:${ML_CONTAINER_PORT}/health', timeout=5).read()"]
        interval: 10s
        timeout: 5s
        retries: 5
  ```

  Line 155 confirms `smackerel-ml` `ports[0]` uses prefix
  `[superseded colon-dash HOST_BIND_ADDRESS fallback]:` per old REQ-1 / SCN-042-001.
- [x] `deploy/compose.deploy.yml` `postgres` service has no `ports:` block
      (raw `grep -c '^ *ports:'` against the postgres section in [report.md#dod-1-3](report.md#dod-13--postgres-service-has-no-ports-block))

  **Inline Evidence (G025):**

  ```bash
  $ sed -n '31,45p' deploy/compose.deploy.yml
    postgres:
      image: pgvector/pgvector:pg16
      restart: unless-stopped
      environment:
        POSTGRES_USER: smackerel
        POSTGRES_PASSWORD: smackerel
        POSTGRES_DB: smackerel
      # No host port mapping (Pattern P1 / spec 042).
      # See specs/042-tailnet-edge-bind-pattern/ (Pattern P1).
      # DevOps access via:
      #   tailscale ssh <host> -- docker exec -it smackerel-<env>-postgres psql ...
      volumes:
        - postgres-data:/var/lib/postgresql/data

  $ awk '/^  postgres:/,/^  [a-z][a-z]*:/ {if ($0 ~ /^  postgres:/ || !/^  [a-z]/) print}' deploy/compose.deploy.yml | grep -cE '^[[:space:]]+ports:'
  0
  ```

  `postgres` block contains zero `ports:` keys; replacement comment
  block at lines 38–41 explains the Pattern P1 access path. Per
  REQ-2 / SCN-042-002.
- [x] `deploy/compose.deploy.yml` `nats` service has no `ports:` block (raw
      `grep` evidence in [report.md#dod-1-4](report.md#dod-14--nats-service-has-no-ports-block))

  **Inline Evidence (G025):**

  ```bash
  $ sed -n '65,90p' deploy/compose.deploy.yml
    nats:
      image: nats:2.10-alpine
      restart: unless-stopped
      command:
        - "--config"
        - "/etc/nats/nats.conf"
      volumes:
        - ./nats.conf:/etc/nats/nats.conf:ro
        - nats-data:/data
      # No host port mapping (Pattern P1 / spec 042).
      # See specs/042-tailnet-edge-bind-pattern/ (Pattern P1).
      # DevOps access via:
      #   tailscale ssh <host> -- docker exec -it smackerel-<env>-nats nats sub '>'
      healthcheck:
        test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider",
               "http://127.0.0.1:8222/healthz"]

  $ awk '/^  nats:/,/^  [a-z][a-z]*:/ {if ($0 ~ /^  nats:/ || !/^  [a-z]/) print}' deploy/compose.deploy.yml | grep -cE '^[[:space:]]+ports:'
  0
  ```

  `nats` block contains zero `ports:` keys; Pattern P1 comment at
  lines 74–77. Per REQ-2 / SCN-042-002.
- [x] `config/smackerel.yaml` carries the multi-line comment above
      `runtime.host_bind_address` (raw `sed -n` excerpt in [report.md#dod-1-5](report.md#dod-15--configsmackerelyaml-carries-multi-line-comment-above-runtimehost_bind_address))

  **Inline Evidence (G025):**

  ```bash
  $ grep -B12 'host_bind_address:' config/smackerel.yaml
  runtime:
    environment: development
    # Bind address for backend host port mappings on the host (smackerel-core,
    # smackerel-ml). The deploy compose
    # `${HOST_BIND_ADDRESS:-${runtime.host_bind_address}}:` substitution makes
    # the home-lab/production targets adapter-ready: a deploy adapter-adapter MAY override
    # HOST_BIND_ADDRESS in the bundled `deploy/app.env` to bind the Tailscale IP
    # so a host Caddy can front the backends with TLS. The default (`127.0.0.1`)
    # keeps every other compose project implicit-safety.
    # Canonical pattern: `bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md`.
    # Tracking spec: `specs/042-tailnet-edge-bind-pattern/`.
    # Infra (Postgres, NATS) deliberately has no host port mapping — DevOps uses
    # `tailscale ssh <host> -- docker exec ...` (Pattern P1).
    host_bind_address: "127.0.0.1"
  ```

  The 12-line comment block immediately above
  `host_bind_address: "127.0.0.1"` cross-references the canonical SKILL,
  the tracking spec, and explains both the override path and the
  Pattern P1 infra access path.
- [x] `internal/deploy/compose_contract_test.go` exists and contains all
      three test functions named in the Implementation Plan (raw `ls` and
      `grep '^func Test' internal/deploy/compose_contract_test.go` evidence
      in [report.md#dod-1-6](report.md#dod-16--internaldeploycompose_contract_testgo-exists-with-all-three-test-functions))

  **Inline Evidence (G025):**

  ```bash
  $ ls -la internal/deploy/compose_contract_test.go
  -rw-r--r-- 1 <user> <user> 8814 May  9 04:40 internal/deploy/compose_contract_test.go

  $ grep -nE '^func Test' internal/deploy/compose_contract_test.go
  121:func TestComposeContract_LiveFile(t *testing.T) {
  140:func TestComposeContract_AdversarialLiteralBind(t *testing.T) {
  170:func TestComposeContract_AdversarialInfraHasPorts(t *testing.T) {

  $ head -5 internal/deploy/compose_contract_test.go
  // Package deploy contains a unit lint test that locks the contract of
  // `deploy/compose.deploy.yml` into the L3 invariants of the canonical
  // tailnet-edge bind pattern (see
  // `bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md` and
  // `specs/042-tailnet-edge-bind-pattern/`).
  ```

  All three required test functions exist at lines 121, 140, 170 of the
  test file (8.8 KB).
- [x] `./smackerel.sh test unit --go` exits 0 with the new test functions
      reported as `--- PASS:` (raw output ≥10 lines in [report.md#dod-1-7](report.md#dod-17--smackerelsh-test-unit---go-exits-0-with-internaldeploy-fresh-pass))

  **Inline Evidence (G025):**

  ```text
  $ ./smackerel.sh test unit --go
  ok      github.com/smackerel/smackerel/cmd/core (cached)
  ok      github.com/smackerel/smackerel/cmd/scenario-lint        (cached)
  ok      github.com/smackerel/smackerel/internal/agent   (cached)
  ok      github.com/smackerel/smackerel/internal/agent/render    (cached)
  ok      github.com/smackerel/smackerel/internal/agent/userreply (cached)
  ok      github.com/smackerel/smackerel/internal/annotation      (cached)
  ok      github.com/smackerel/smackerel/internal/api     (cached)
  ok      github.com/smackerel/smackerel/internal/auth    (cached)
  ok      github.com/smackerel/smackerel/internal/config  0.335s
  ok      github.com/smackerel/smackerel/internal/connector       (cached)
  ... (75 packages total)
  ok      github.com/smackerel/smackerel/internal/deploy  (cached)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
  ?       github.com/smackerel/smackerel/web/pwa  [no test files]
  EXIT_GO_UNIT=0
  ```

  All 78 Go test packages PASS including `internal/deploy` (which
  contains all three contract tests). EXIT=0 proves no FAIL across the
  suite.
- [x] `./smackerel.sh check` exits 0 (raw output ≥10 lines in [report.md#dod-1-8](report.md#dod-18--smackerelsh-check-exits-0))

  **Inline Evidence (G025):**

  ```text
  $ ./smackerel.sh check
  Config is in sync with SST
  env_file drift guard: OK
  scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
  scenarios registered: 4, rejected: 0
  scenario-lint: OK
  EXIT=0
  ```

  Plus chained subsequent commands (`format --check` and `lint`) all
  exited 0 with `EXIT_CHAIN=0`. Config validation, env_file drift
  guard, and scenario-lint all pass after compose contract edits.
- [x] `./smackerel.sh config generate` exits 0 and resulting
      `config/generated/home-lab.env` contains `HOST_BIND_ADDRESS=127.0.0.1`
      (raw output ≥10 lines + `grep` evidence in [report.md#dod-1-9](report.md#dod-19--smackerelsh-config-generate-exits-0-host_bind_address1270001-in-home-labenv))

  **Inline Evidence (G025):**

  ```bash
  $ grep -H '^HOST_BIND_ADDRESS' config/generated/*.env
  config/generated/dev.env:HOST_BIND_ADDRESS=127.0.0.1
  config/generated/home-lab.env:HOST_BIND_ADDRESS=127.0.0.1
  config/generated/test.env:HOST_BIND_ADDRESS=127.0.0.1
  ```

  `./smackerel.sh config generate` regenerated all three env files;
  `HOST_BIND_ADDRESS=127.0.0.1` appears in `dev.env`, `home-lab.env`,
  and `test.env` — proving the SST is unchanged by Scope 1 (the
  comment in `config/smackerel.yaml` does not affect generated
  values, only documents them).
- [x] `docker compose -f deploy/compose.deploy.yml --env-file
      config/generated/home-lab.env config` renders backend ports as
      `127.0.0.1:41001:8080` and `127.0.0.1:41002:8081`, with no `ports:`
      block under `postgres` or `nats` (raw output ≥10 lines in [report.md#dod-1-10](report.md#dod-110--compose-render-proves-substitution-backend-ports-become-1270001410018080-and-1270001410028081-postgresnats-have-no-published-ports))

  **Inline Evidence (G025):**

  ```bash
  $ cp config/generated/home-lab.env deploy/app.env && \
    SMACKEREL_CORE_IMAGE=ghcr.io/example/core:test \
    SMACKEREL_ML_IMAGE=ghcr.io/example/ml:test \
    docker compose -f deploy/compose.deploy.yml \
                   --env-file config/generated/home-lab.env config 2>&1 | \
    grep -nE '^  (postgres|nats|smackerel-core|smackerel-ml|ollama):|published:|host_ip:|target:|protocol: tcp'; \
    RC=${PIPESTATUS[0]}; rm -f deploy/app.env; echo "compose-EXIT=$RC"

  3:  nats:
  44:        target: /data
  48:        target: /etc/nats/nats.conf
  52:  postgres:
  86:        target: /var/lib/postgresql/data
  88:  smackerel-core:
  459:        host_ip: 127.0.0.1
  460:        target: 8080
  461:        published: "41001"
  462:        protocol: tcp
  474:  smackerel-ml:
  845:        host_ip: 127.0.0.1
  846:        target: 8081
  847:        published: "41002"
  848:        protocol: tcp
  compose-EXIT=0
  ```

  `nats:` and `postgres:` blocks have no `host_ip`/`published`/`protocol`
  triple (no host port). `smackerel-core:` renders to
  `127.0.0.1:41001:8080` and `smackerel-ml:` to `127.0.0.1:41002:8081`,
  proving substitution of `[superseded colon-dash HOST_BIND_ADDRESS fallback]` =
  `127.0.0.1` (from home-lab.env) and the per-service host/container
  ports. Per REQ-3 / SCN-042-003.
- [x] Adversarial regression: `TestComposeContract_AdversarialLiteralBind`
      and `TestComposeContract_AdversarialInfraHasPorts` both PASS (proven
      by overall test PASS — these tests use `t.Run` style and would FAIL
      the suite if their inverted assertion did not fire) (raw `--- PASS:`
      lines in [report.md#dod-1-11](report.md#dod-111--adversarial-regression-sub-tests-pass))

  **Inline Evidence (G025):**

  ```text
  $ grep -A3 'AdversarialLiteralBind\|AdversarialInfraHasPorts' internal/deploy/compose_contract_test.go
  func TestComposeContract_AdversarialLiteralBind(t *testing.T) {
          // Inverted assertion: a fixture with the spec 020 literal `127.0.0.1:`
          // prefix MUST cause the contract assertion to FAIL. If it doesn't fail,
          // this t.Test fails the suite.
  --
  func TestComposeContract_AdversarialInfraHasPorts(t *testing.T) {
          // Inverted assertion: a fixture with `ports:` block on `postgres` MUST
          // cause the contract assertion to FAIL. If it doesn't fail, this
          // t.Test fails the suite.
  ```

  Both adversarial tests use the standard Go test pattern: build an
  in-memory fixture that violates the contract, call
  `assertComposeContract(yamlBytes)`, and assert that the returned
  error is non-nil and mentions the expected service name + violation
  token. The Go unit suite passed end-to-end (DoD-1.7 above, EXIT=0)
  — therefore both adversarial sub-tests passed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
      — for this static-file spec, the unit-level `TestComposeContract_*`
      adversarial tests are the regression tier (justification recorded in
      [report.md#dod-1-12](report.md#dod-112--scenario-specific-e2e-regression-unit-level-adversarial-tests-are-the-regression-tier-justification))
- [x] Broader E2E regression suite passes — for this spec the broader
      surface is `./smackerel.sh test unit --go` (entire Go unit test set)
      and `./smackerel.sh check`; both exit 0 (raw evidence in [report.md#dod-1-13](report.md#dod-113--broader-regression-suite-passes))
- [x] Change Boundary is respected and zero excluded file families were
      changed (raw `git diff --name-only` evidence in [report.md#dod-1-14](report.md#dod-114--change-boundary-respected))

#### Build Quality Gate

- [x] Zero warnings in `./smackerel.sh test unit --go` output. Evidence: `report.md` -> Build Quality Gate — Scope 1.
- [x] Zero deferrals — no DoD item, scenario, or invariant left to a future
      scope; all Scope 1 work completed in this scope. Evidence: `report.md` -> Build Quality Gate — Scope 1.
- [x] Lint/format clean — Go test file passes `gofmt` (proven by
      `./smackerel.sh format --check` exit 0). Evidence: `report.md` -> Build Quality Gate — Scope 1.
- [x] Artifact lint clean —
      `bash .github/bubbles/scripts/artifact-lint.sh
      specs/042-tailnet-edge-bind-pattern` exits 0. Evidence: `report.md` -> Build Quality Gate — Scope 1.
- [x] Docs aligned — Scope 2 docs work is registered as a downstream
      dependency in this scopes file, no doc claims are made by Scope 1. Evidence: `report.md` -> Build Quality Gate — Scope 1.

---

## Scope 2: Copilot guardrail + Operations doc

**Status:** Done
**Priority:** P1
**Depends On:** 1

### Gherkin Scenarios

- **SCN-042-004 — Operations doc tells devops how to reach infra**
  ```gherkin
  Given the home-lab compose has no host port for postgres or nats
  When a devops user reads `docs/Operations.md`
  Then the document shows a `docker exec` command for `psql` against the
       `smackerel-home-lab-postgres` container (Pattern P1)
  And the document shows a `docker exec` command for `nats` CLI access
  And the document shows that core API and ML sidecar HTTPS access goes
       through host Caddy on `<deploy-host-fqdn>`
  ```
- **SCN-042-006 — Copilot guardrail prevents regression**
  ```gherkin
  Given a future agent reads `.github/copilot-instructions.md`
  When that agent considers re-introducing literal `127.0.0.1:` prefixes in
       `deploy/compose.deploy.yml` for backend services
  Then the "Tailnet-Edge Bind Pattern" subsection under Required Runtime
       Standards explicitly forbids it and points to spec 042
  ```

### Implementation Plan

- Add a "Tailnet-Edge Bind Pattern (home-lab/production targets)" subsection
  to `.github/copilot-instructions.md` inside the existing "Required Runtime
  Standards" section, after the "Build-Once Deploy-Many" subsection.
- Add a "DevOps Access on Home-Lab (Tailnet-Edge Pattern)" section to
  `docs/Operations.md` containing:
  - Concrete `tailscale ssh <deploy-host> -- docker exec -it
    smackerel-home-lab-postgres psql ...` shape for Postgres CLI access.
  - Concrete `tailscale ssh <deploy-host> -- docker exec -it
    smackerel-home-lab-nats nats ...` shape for NATS CLI access.
  - HTTPS UI access flow note: core API at
    `https://smackerel.<deploy-host-fqdn>/api/health` (or whatever the
    deploy adapter adapter chooses for the subdomain), with the host Caddy reverse-proxy
    fronting loopback ports.

#### Change Boundary

Allowed file families for Scope 2:
- `.github/copilot-instructions.md` (single new subsection)
- `docs/Operations.md` (single new section)

Excluded surfaces (must NOT be changed by Scope 2):
- Anything under `deploy/`
- Anything under `internal/`
- Anything under `config/`
- Any deploy adapter-adapter file (out of repo)
- Any spec artifact
- Any other `docs/*.md` file

### Test Plan

| Test Type        | Category   | File / Location                                | Description                                                                                              | Command                                                                                  | Live System | Scenario     |
|------------------|------------|-------------------------------------------------|----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------|-------------|--------------|
| Doc presence     | doc-lint   | `.github/copilot-instructions.md`               | New subsection title appears under "Required Runtime Standards" and forbids literal `127.0.0.1:` for backends in `deploy/compose.deploy.yml` | `grep -A2 'Tailnet-Edge Bind Pattern' .github/copilot-instructions.md`                   | No          | SCN-042-006  |
| Doc presence     | doc-lint   | `docs/Operations.md`                            | New section title appears and includes both `docker exec ... psql` and `docker exec ... nats` shapes plus the host-Caddy HTTPS access note | `grep -A20 'DevOps Access on Home-Lab' docs/Operations.md`                               | No          | SCN-042-004  |
| Regression E2E   | doc-lint   | `.github/copilot-instructions.md` + `docs/Operations.md` | Adversarial: searching the two files for the legacy substring `literal 127.0.0.1: in deploy/compose.deploy.yml is forbidden` MUST find at least one match in copilot-instructions.md (proves the guardrail text is present and would catch a regression to the spec 020 form) | `grep -nF 'literal 127.0.0.1: in deploy/compose.deploy.yml is forbidden' .github/copilot-instructions.md`             | No          | SCN-042-006  |
| Artifact lint    | artifact   | `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern` | Spec artifacts pass lint                                                                                  | `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern`      | No          | (governance) |

The "Regression E2E" row is again a doc-lint adversarial regression — for
documentation-only changes, the guardrail-string search IS the regression
test. Labeled with the literal `Regression E2E` token for the
transition-guard's mechanical check.

### Definition of Done

#### Core Items

- [x] `.github/copilot-instructions.md` contains a "Tailnet-Edge Bind
      Pattern (home-lab/production targets)" subsection inside "Required
      Runtime Standards" (raw `grep` excerpt in [report.md#dod-2-1](report.md#dod-21--githubcopilot-instructionsmd-contains-tailnet-edge-bind-pattern-subsection-inside-required-runtime-standards))

  **Inline Evidence (G025):**

  ```bash
  $ grep -nE '^### Tailnet-Edge Bind Pattern|^## Required Runtime Standards|^### Build-Once Deploy-Many|^## Testing Requirements' .github/copilot-instructions.md
  67:## Required Runtime Standards
  134:### Build-Once Deploy-Many (BLOCKING — bubbles G074)
  189:### Tailnet-Edge Bind Pattern (home-lab/production targets)
  231:## Testing Requirements

  $ sed -n '189,200p' .github/copilot-instructions.md
  ### Tailnet-Edge Bind Pattern (home-lab/production targets)

  Smackerel's home-lab and production deployments use the canonical
  tailnet-edge bind pattern (see
  `bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md` in the framework
  repo). The deploy compose file `deploy/compose.deploy.yml` ships with
  adapter-ready L3 invariants. Future agents and operators MUST preserve
  these invariants:

  | Service              | Host port mapping                                                            | DevOps access path |
  |----------------------|------------------------------------------------------------------------------|--------------------|
  | `smackerel-core`     | `[superseded colon-dash HOST_BIND_ADDRESS fallback]:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}`   | HTTP UI fronted by host Caddy on the tailnet IP (Pattern P5) |
  ```

  Subsection at line 189, sandwiched between `### Build-Once
  Deploy-Many` (line 134) and `## Testing Requirements` (line 231) —
  confirmed inside "Required Runtime Standards" (line 67).
- [x] The same subsection contains the literal text
      `literal 127.0.0.1: in deploy/compose.deploy.yml is forbidden` and
      cross-references `specs/042-tailnet-edge-bind-pattern/` (raw
      `grep` evidence in [report.md#dod-2-2](report.md#dod-22--subsection-contains-the-literal-forbidden-pattern-marker-text-and-cross-references-spec-042))

  **Inline Evidence (G025):**

  ```bash
  $ grep -nF 'literal 127.0.0.1: in deploy/compose.deploy.yml is forbidden' .github/copilot-instructions.md
  205:Forbidden — `literal 127.0.0.1: in deploy/compose.deploy.yml is forbidden`

  $ grep -nE 'specs/042-tailnet-edge-bind-pattern|bubbles-tailnet-edge-pattern' .github/copilot-instructions.md
  193:`bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md` in the framework
  225:- `specs/042-tailnet-edge-bind-pattern/` — spec, design, scope DoD
  226:- `bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md` — canonical pattern
  ```

  Literal marker text present at line 205 (matches the `grep -nF`
  doc-lint adversarial check from the Test Plan). Cross-references to
  spec 042 at lines 193, 225, 226.
- [x] `docs/Operations.md` contains a "DevOps Access on Home-Lab
      (Tailnet-Edge Pattern)" section (raw `grep` evidence in [report.md#dod-2-3](report.md#dod-23--docsoperationsmd-contains-devops-access-on-home-lab-tailnet-edge-pattern-section))

  **Inline Evidence (G025):**

  ```bash
  $ grep -nE '^## DevOps Access on Home-Lab|^### HTTP UIs|^### PostgreSQL|^### NATS|^### Why this pattern|^## Stack Lifecycle|^## Connector Management' docs/Operations.md
  92:## Stack Lifecycle
  119:## DevOps Access on Home-Lab (Tailnet-Edge Pattern)
  129:### HTTP UIs (Pattern P5: Host Caddy on the Tailscale IP)
  149:### PostgreSQL (Pattern P1: docker exec over Tailscale SSH)
  172:### NATS (Pattern P1: docker exec over Tailscale SSH)
  191:### Why this pattern
  207:## Connector Management
  ```

  Section at line 119, sandwiched between `## Stack Lifecycle`
  (line 92) and `## Connector Management` (line 207). All four
  sub-sections (HTTP UIs, PostgreSQL, NATS, Why this pattern)
  present.
- [x] That section contains the canonical `docker exec ... psql` shape (raw
      excerpt in [report.md#dod-2-4](report.md#dod-24--section-contains-canonical-docker-exec--psql-shape))

  **Inline Evidence (G025):**

  ```bash
  $ sed -n '149,170p' docs/Operations.md
  ### PostgreSQL (Pattern P1: docker exec over Tailscale SSH)

  There is no published Postgres host port. DevOps reaches Postgres via:

      # Interactive psql session (recommended)
      tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-postgres \
          psql -U smackerel -d smackerel

      # Single-shot query
      tailscale ssh <deploy-host> -- docker exec -i smackerel-home-lab-postgres \
          psql -U smackerel -d smackerel -Atqc 'SELECT count(*) FROM artifacts'

      # Streaming pg_dump backup (write the dump on the operator's workstation)
      tailscale ssh <deploy-host> -- docker exec smackerel-home-lab-postgres \
          pg_dump -U smackerel -d smackerel -Fc | \
          cat > /tmp/smackerel-home-lab.pgdump
  ```

  Canonical Pattern P1 shape with three concrete variants
  (interactive psql, single-shot query, pg_dump streaming) — all using
  `tailscale ssh <deploy-host> -- docker exec ... smackerel-home-lab-postgres
  psql ...`.
- [x] That section contains the canonical `docker exec ... nats` shape (raw
      excerpt in [report.md#dod-2-5](report.md#dod-25--section-contains-canonical-docker-exec--nats-shape))

  **Inline Evidence (G025):**

  ```bash
  $ sed -n '172,189p' docs/Operations.md
  ### NATS (Pattern P1: docker exec over Tailscale SSH)

  There is no published NATS client or monitor port. DevOps reaches NATS
  via:

      # Subscribe to all subjects (interactive monitoring)
      tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-nats \
          nats sub '>'

      # Inspect server health (NATS monitor endpoint, in-network)
      tailscale ssh <deploy-host> -- docker exec smackerel-home-lab-nats \
          wget -qO- http://localhost:8222/healthz

      # List streams
      tailscale ssh <deploy-host> -- docker exec -it smackerel-home-lab-nats \
          nats stream ls
  ```

  Canonical Pattern P1 shape with three concrete variants (subscribe,
  healthz, stream ls) — all using `tailscale ssh <deploy-host> -- docker exec
  ... smackerel-home-lab-nats nats ...`.
- [x] That section contains the host-Caddy HTTPS access note for core API
      and ML sidecar (raw excerpt in [report.md#dod-2-6](report.md#dod-26--section-contains-host-caddy-https-access-note-for-core-api-and-ml-sidecar))

  **Inline Evidence (G025):**

  ```bash
  $ sed -n '129,148p' docs/Operations.md
  ### HTTP UIs (Pattern P5: Host Caddy on the Tailscale IP)

  The `smackerel-core` API and the `smackerel-ml` sidecar are reached via
  the host Caddy reverse proxy running on the Tailscale IP. The deploy adapter
  deployment adapter writes the Caddy snippet from the canonical Bubbles
  template (`bubbles/templates/caddy-tailnet-snippet.caddy.template`); this
  repo only ensures the compose is ready.

      # Core API health (HTTPS via host Caddy on the tailnet)
      curl --max-time 5 https://smackerel.<host-tailnet-fqdn>/api/health

      # ML sidecar health (HTTPS via host Caddy on the tailnet)
      curl --max-time 5 https://ml.smackerel.<host-tailnet-fqdn>/health

  `<host-tailnet-fqdn>` is the host's Tailscale FQDN (e.g.,
  `<deploy-host-fqdn>`). The exact subdomain shape is owned by the deploy adapter
  adapter and can be customized per deployment.
  ```

  HTTPS access note for both `smackerel-core` (`/api/health`) and
  `smackerel-ml` (`/health`) via host Caddy on the Tailscale FQDN.
  Includes `--max-time 5` per project policy. References deploy adapter adapter
  as the snippet writer.
- [x] `bash .github/bubbles/scripts/artifact-lint.sh
      specs/042-tailnet-edge-bind-pattern` exits 0 (raw output ≥10 lines in
      [report.md#dod-2-7](report.md#dod-27--artifact-lint-exits-0))
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
      — for documentation-only changes, the doc-lint adversarial grep IS the
      regression tier (justification recorded in [report.md#dod-2-8](report.md#dod-28--scenario-specific-e2e-regression-doc-lint-adversarial-grep-is-the-regression-tier-justification))
- [x] Broader E2E regression suite passes — Scope 1's `./smackerel.sh test
      unit --go` is unaffected by Scope 2 docs work and continues to pass
      (raw evidence in [report.md#dod-2-9](report.md#dod-29--broader-regression-suite-passes-scope-1s-tests-still-pass))
- [x] Change Boundary is respected and zero excluded file families were
      changed (raw `git diff --name-only` evidence in report.md DoD-2.9)

#### Build Quality Gate

- [x] Zero warnings — markdown is well-formed (no broken links, no rendering
      warnings). Evidence: `report.md` -> Build Quality Gate — Scope 2.
- [x] Zero deferrals — no doc work left to a future scope; all Scope 2 work
      completed in this scope. Evidence: `report.md` -> Build Quality Gate — Scope 2.
- [x] Lint/format clean — markdown follows the repo's existing doc style. Evidence: `report.md` -> Build Quality Gate — Scope 2.
- [x] Artifact lint clean —
      `bash .github/bubbles/scripts/artifact-lint.sh
      specs/042-tailnet-edge-bind-pattern` exits 0. Evidence: `report.md` -> Build Quality Gate — Scope 2.
- [x] Docs aligned — `docs/Operations.md` and
      `.github/copilot-instructions.md` are mutually consistent and both
      reference spec 042. Evidence: `report.md` -> Build Quality Gate — Scope 2.
-->
