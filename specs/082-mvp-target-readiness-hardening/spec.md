# Spec 082 — MVP / <deploy-host> Readiness Hardening

**Status:** Done
**Feature number:** 082
**Release train:** next
**Owner workflow:** full-delivery
**Created:** 2026-06-09

---

## 1. Summary

A holistic MVP → <deploy-host> self-hosted readiness review surfaced nine **in-repo**,
deliverable gaps that each weaken first-deploy reliability, correctness, SST
consistency, supply-chain posture, operability, or documentation. This spec
closes all nine inside the Smackerel product repo. None of the nine require
the physical <deploy-host> host, the knb deploy adapter, or production secrets to be
delivered — they are all changes to committed config, compose contracts, Go
validators, deploy scripts, tests, and docs.

The work is grouped into nine cleanly-separated scopes, each with its own
Gherkin use case(s), implementation surface, required tests (adversarial where
a regression is being prevented), and a strict Definition of Done backed by
real executed evidence (anti-fabrication is NON-NEGOTIABLE).

## 2. Problem / Context

The review confirmed (recon-validated live on 2026-06-09) the following live
defects in the committed tree:

1. **PII / correctness** — `config/smackerel.yaml` hardcodes a real-looking
   personal Telegram `chat_ids: "510638591"` and `user_mapping:
   "510638591:philip"`, so a fresh operator inherits the author's identity as
   the default Telegram recipient/attribution.
2. **Reliability** — the Ollama model-envelope validator
   (`internal/config/config.go::validateModelEnvelopes`) checks each model
   **individually** against the ollama memory envelope but never the
   **concurrent sum** of keep-alive-resident interactive models. On self-hosted,
   `gemma4:26b` (18 432 MiB) + `llama3.1:8b` (6 144 MiB) = 24 576 MiB exceeds
   the 20 G (20 480 MiB) ollama cgroup, so a long-lived `keep_alive` working
   set OOM-kills ollama into a restart crash-loop. The `gemma`-class resident
   figures also disagree between `config/smackerel.yaml` and
   `docs/Operations.md`.
3. **Reliability** — `smackerel-ml` in `deploy/compose.deploy.yml` uses
   read-only root + a RAM `tmpfs` for `HF_HOME` / `SENTENCE_TRANSFORMERS_HOME`,
   so the embedding model re-downloads from HuggingFace on every restart,
   putting an external-network dependency on the reboot-recovery path.
4. **Reliability / safety** — the `nats-data` volume in
   `deploy/compose.deploy.yml` is labelled `com.smackerel.lifecycle:
   ephemeral`, but JetStream holds at-least-once in-flight capture state;
   cleanup tooling must never be able to wipe queued capture events.
5. **SST consistency** — the `searxng` service hardcodes its deploy memory
   limit off-contract (`memory: 256M`, no `cpus` cap) while every other
   service derives `cpus`/`memory` from `deploy_resources.*` via fail-loud
   `${VAR:?...}`.
6. **Supply-chain** — `pgvector/pgvector:pg16`, `nats:2.10-alpine`, and
   `ollama/ollama:rocm` are referenced by mutable tag (outside the
   cosign/SBOM chain) in `deploy/contract.yaml` + `deploy/compose.deploy.yml`.
7. **Operability** — `scripts/deploy/promote.sh` parses the CI build-manifest
   shape (`images:` list, `configBundles:` list) while
   `scripts/commands/build-self-hosted.sh` emits a different shape (`images:`
   map, single `configBundle:` object), so the two manifest paths are
   silently incompatible.
8. **Docs** — there is no single operator-facing go-live readiness checklist
   tying together the five production secrets (spec 051), the L2 knb
   secret-injection dependency (spec 052), local-operator vs CI trust (spec
   017), profile enablement, backup/restore-drill sequencing, and the
   supervised-canary first apply. `specs/_spec-review-report.md` is also
   stale (dated 2026-06-02).
9. **Evaluate-then-act** — `deploy/compose.deploy.yml` pins host-specific
   ROCm render/video GIDs (`44`, `993`) as literals in the **generic**
   compose, which sits awkwardly against the repo's "no env-specific content"
   + deployment-ownership-boundary policy.

## 3. Goals / Non-Goals

### Goals

- Close all nine in-repo gaps above with real code/config/test/doc changes.
- Preserve every existing gate: NO-DEFAULTS / fail-loud SST (Gate G028),
  read-only-root posture (FR-045-003), externalImages lockstep (BUG-049-001),
  the spec-045 resource contract, the spec-042 tailnet-edge bind invariants,
  and "no env-specific content".
- Each scope's required tests actually run and pass with captured evidence;
  regression-preventing tests are adversarial (fail if the protection is
  removed).

### Non-Goals (tracked EXTERNAL dependencies — see §5)

These are explicitly **out of scope** and are tracked as external blockers,
NOT silently absorbed or faked:

- (a) On-GPU accelerated inference validation on the physical <deploy-host>.
- (b) The knb L2 secret-injection adapter and population of the five
  production secrets.
- (c) knb backup scheduler / Alertmanager / Grafana standup.
- (d) The actual deploy to <deploy-host>.
- (e) Side-service tailnet exposure binding (`HOST_BIND_ADDRESS` /
  Tailscale ACL) — adapter-owned.

## 4. Product Principle Alignment

This spec touches product principles 6, 8, and 10 lightly, and the
engineering NO-DEFAULTS SST track throughout.

- **Principle 6 (Invisible By Default, Felt Not Heard)** — Scope 1 removes a
  hardcoded default Telegram recipient so the system never silently delivers
  to or attributes captures to a stranger's identity; an unconfigured
  Telegram surface stays silent rather than mis-addressing.
- **Principle 8 (Trust Through Transparency)** — Scope 2's concurrent-envelope
  guard and the Scope 8 go-live checklist make the runtime's resource and
  readiness posture legible and fail-loud instead of failing opaquely at first
  inference; Scope 6 keeps the third-party supply chain pinned and auditable.
- **Principle 10 (QF Companion Boundary)** — no financial-action surface is
  introduced; no QF packet metadata is touched. This spec is infrastructure
  hardening only.

No principle is violated. The Telegram identity change (Scope 1) strengthens
Principle 6 rather than deviating from it.

## 5. Dependencies, Risks & Tracked External Blockers

| # | External blocker | Owner | Why out of repo | Tracked as |
|---|------------------|-------|-----------------|-----------|
| a | On-GPU accel inference validation on physical <deploy-host> | operator + hardware | needs the real Strix Halo iGPU | risk R-082-A |
| b | knb L2 secret-injection adapter + 5 prod secrets population | knb deploy adapter | per deployment-ownership boundary | risk R-082-B |
| c | knb backup scheduler / Alertmanager / Grafana standup | knb | knb-side operational standup | risk R-082-C |
| d | Actual deploy to <deploy-host> | operator | adapter-driven apply | risk R-082-D |
| e | Side-service tailnet exposure binding (HOST_BIND_ADDRESS / Tailscale ACL) | knb deploy adapter | adapter-owned per spec 042 | risk R-082-E |

**Risks**

- **R-082-A** — Scope 2 raises the self-hosted ollama envelope to fit the
  interactive concurrent set; the true GPU residency of `gemma4:26b` under
  ROCm is only verifiable on the <deploy-host> (blocker a). The guard uses published
  resident ceilings; live `ollama ps` verification stays an operator-host
  step.
- **R-082-F** — Scope 6 pins third-party images by digest captured on
  2026-06-09; upstream may publish newer patched images. A deliberate digest
  bump is an explicit, reviewed change (documented), not an automatic drift.

## 6. Functional Requirements

- **FR-082-001** (Scope 1) — `config/smackerel.yaml` MUST ship empty Telegram
  `chat_ids` and `user_mapping` defaults; the runtime MUST treat an empty
  Telegram recipient set as "no Telegram recipient configured" without
  breaking startup, and no real chat-id literal may remain in the repo.
- **FR-082-002** (Scope 2) — `validateModelEnvelopes` MUST additionally reject
  any env whose **distinct interactive hot-path** ollama model resident sum
  exceeds `OLLAMA_MEMORY_LIMIT`, fail-loud, when keep-alive is resident; the
  self-hosted envelope MUST be sized to fit its interactive sum; the `gemma`-class
  resident figures MUST agree between `config/smackerel.yaml` and
  `docs/Operations.md`.
- **FR-082-003** (Scope 3) — `smackerel-ml` MUST mount a persistent named
  volume for the embedding-model cache so restart resilience does not depend
  on HuggingFace reachability, while keeping read-only root intact.
- **FR-082-004** (Scope 4) — the `nats-data` volume MUST be labelled to
  reflect its durability, and `./smackerel.sh clean` flows MUST NOT be able to
  remove `nats-data` on a running self-hosted stack.
- **FR-082-005** (Scope 5) — the `searxng` deploy CPU and memory limits MUST
  be SST-sourced from `config/smackerel.yaml` `deploy_resources.searxng.*` via
  fail-loud `${VAR:?...}`, with an explicit `cpus` cap, consistent with the
  spec-045 resource contract.
- **FR-082-006** (Scope 6) — `pgvector/pgvector:pg16`, `nats:2.10-alpine`, and
  `ollama/ollama:rocm` MUST be pinned by `sha256` digest in
  `deploy/contract.yaml` and `deploy/compose.deploy.yml` in lockstep, enforced
  by the externalImages contract test.
- **FR-082-007** (Scope 7) — `scripts/deploy/promote.sh` MUST parse BOTH the
  CI build-manifest shape (list) and the local-operator build-manifest shape
  (map/object) so the two manifest paths are not silently incompatible.
- **FR-082-008** (Scope 8) — `docs/` MUST contain a single operator go-live
  readiness checklist tying together the five production secrets, L2 secret
  injection, local-operator vs CI trust, profile enablement, backup/restore
  sequencing, and the supervised canary; the stale spec-review report MUST get
  a dated addendum.
- **FR-082-009** (Scope 9) — host-specific ROCm render/video GIDs MUST be
  assessed against the no-env-specific-content + deployment-ownership policy;
  if they belong in the adapter, the host-specific portion MUST be routed to
  an adapter-supplied fail-loud env (no silent default, Gate G028);
  documented rationale either way.

## 7. Business Scenarios (Gherkin)

```gherkin
Scenario: SCN-082-A01 — Telegram identity is blank by default
  Given a fresh checkout of config/smackerel.yaml
  When an operator reads telegram.chat_ids and telegram.user_mapping
  Then both are empty strings
  And no file in the repo contains the literal chat-id "510638591"
  And the runtime starts with an empty Telegram recipient set treated as
    "no Telegram recipient configured" rather than crashing

Scenario: SCN-082-B01 — Concurrent interactive ollama sum over-subscription fails loud
  Given OLLAMA_MEMORY_LIMIT resolves to 20G
  And keep-alive is resident (-1 or a multi-hour duration)
  And the distinct interactive hot-path models sum to 24576 MiB
  When config.Load()/Validate() runs
  Then validation fails loud naming the resident set, the sum, and
    OLLAMA_MEMORY_LIMIT

Scenario: SCN-082-B02 — Fitting interactive sum is accepted (no false positive)
  Given OLLAMA_MEMORY_LIMIT resolves to 8G
  And the distinct interactive hot-path models sum to 5120 MiB
  When config.Load()/Validate() runs
  Then validation succeeds

Scenario: SCN-082-C01 — Embedding-model cache survives restart without HuggingFace
  Given deploy/compose.deploy.yml is rendered for smackerel-ml
  When the compose filesystem contract is asserted
  Then smackerel-ml mounts a persistent named volume at the model-cache path
  And read-only root is still true with an explicit tmpfs allowlist

Scenario: SCN-082-D01 — clean cannot wipe queued capture events
  Given the nats-data volume carries a durable lifecycle label/protection
  When a clean flow enumerates removable project volumes
  Then nats-data is excluded from removal on a running self-hosted stack

Scenario: SCN-082-E01 — SearxNG limits are SST-sourced and fail loud
  Given deploy/compose.deploy.yml renders the searxng service
  When the resource contract is asserted
  Then searxng cpus and memory use ${SEARXNG_CPU_LIMIT:?...} and
    ${SEARXNG_MEMORY_LIMIT:?...} sourced from deploy_resources.searxng.*

Scenario: SCN-082-F01 — Third-party infra images are digest-pinned in lockstep
  Given deploy/contract.yaml and deploy/compose.deploy.yml
  When the externalImages contract is asserted
  Then postgres, nats, and ollama images contain @sha256: digests
  And the contract.yaml and compose literals match byte-for-byte

Scenario: SCN-082-G01 — promote.sh parses both manifest shapes
  Given a CI list-shape build manifest and a local-operator map/object manifest
  When promote.sh extracts core/ml refs and the bundle ref+sha for an env
  Then both shapes yield the same extracted values

Scenario: SCN-082-H01 — Operator go-live checklist exists and is wired
  Given docs/ after this spec
  When an operator looks for go-live readiness
  Then one checklist enumerates the 5 secrets, L2 injection, local/CI trust,
    profile enablement, backup/restore sequencing, and supervised canary

Scenario: SCN-082-I01 — ROCm host GIDs are policy-correct
  Given deploy/compose.deploy.yml ollama service
  When the host-specific render/video GIDs are evaluated
  Then either they are routed to an adapter-supplied fail-loud env (no silent
    default) or a documented rationale justifies a generic default, with a
    contract test pinning the decision
```

## 8. Acceptance Criteria

- All nine FRs satisfied with real evidence in `report.md`.
- `./smackerel.sh config generate`, `check`, `lint`, `format --check`, and
  `test unit` all green with captured exit codes.
- `artifact-lint` and `traceability-guard` clean for `specs/082-*`.
- The five external blockers in §5 remain documented as tracked dependencies,
  not implemented or faked.

## 9. Release Train

This feature targets the **`next`** train. It introduces **no feature flags**
(`flagsIntroduced: []`); every change is an unconditional hardening of config,
compose contracts, validators, deploy scripts, tests, and docs. On any other
train the behavior is identical (there is no train-gated branch).
