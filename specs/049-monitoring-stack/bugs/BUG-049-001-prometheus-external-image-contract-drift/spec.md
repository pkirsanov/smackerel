# BUG-049-001 — Prometheus external image missing from `deploy/contract.yaml::externalImages`

| Field | Value |
|-------|-------|
| Parent spec | `specs/049-monitoring-stack/` |
| Discovered by | Sweep `sweep-2026-05-23-r30` round 7 (`devops-to-doc` mapped from `devops` trigger) |
| Discovered at HEAD | `700171b2b637057ec41f88330bc38c070fd9c14b` |
| Severity | medium |
| Class | devops · Build-Once Deploy-Many · pinned-external-image contract drift |
| Status | resolved |

## Problem Statement

Spec 049 added the `prometheus` service to `deploy/compose.deploy.yml` with a
profile-gated, SST-pinned image (`config/smackerel.yaml::monitoring.prometheus.image:
"prom/prometheus:v2.55.1"`). The contract document that enumerates third-party,
pinned-for-reproducibility images, `deploy/contract.yaml::externalImages`, lists
only `postgres`, `nats`, and `ollama`. The `prom/prometheus` pin is missing.

```yaml
# deploy/contract.yaml (current — line 22-30)
externalImages:
- name: postgres
  image: pgvector/pgvector:pg16
- name: nats
  image: nats:2.10-alpine
- name: ollama
  image: ollama/ollama:0.23.2
```

```yaml
# deploy/compose.deploy.yml (line 289)
prometheus:
  image: ${PROMETHEUS_IMAGE}      # resolves to prom/prometheus:v2.55.1
  profiles: [monitoring]
```

## Why It Matters

`deploy/contract.yaml` is the operator-facing contract that deploy-adapter
overlays consume to know which images they must pull (and, for project-built
images, cosign-verify) at apply time. The skeleton at
`deploy/_example/target-skeleton/apply.sh` documents this explicitly:

> `--image-<service>=sha256:<digest>` (one per service in `deploy/contract.yaml`)

Adapter overlays that implement an offline-capable apply flow (pre-pull cache,
air-gapped mirror, signature verification audit-trail) will silently miss the
`prom/prometheus` image when an operator first enables `--profile monitoring`,
because the contract doc does not advertise it. This is real artifact-vs-contract
drift introduced by spec 049 and surfaced by the round 7 devops probe.

## Scenarios (Gherkin)

### SCN-049-B001 — externalImages enumerates every non-built service image

```gherkin
Given the deploy contract file at deploy/contract.yaml
And  the deploy compose file at deploy/compose.deploy.yml
When  the contract drift checker enumerates every service image referenced by compose.deploy.yml
Then  every image that is NOT built by this project (i.e. not smackerel-core or smackerel-ml)
And   that is referenced via a SST-rendered ${IMAGE}-style substitution
And   that has a fixed digest/tag pin in config/smackerel.yaml
Must  be named in deploy/contract.yaml::externalImages with the matching image string.
```

### SCN-049-B002 — Adversarial drift detection

```gherkin
Given the contract test from SCN-049-B001
When  a developer adds a fourth third-party service to compose.deploy.yml
But   forgets to update deploy/contract.yaml::externalImages
Then  the contract test must fail with a clear, actionable error
And   the failure must name the missing image and the SST key that pins it.
```

## Out Of Scope

- Trivy vulnerability scanning of external images. Spec 047's CI vulnerability
  gate intentionally scopes only project-built images (`smackerel-core`,
  `smackerel-ml`); none of the four third-party externals are scanned. That is a
  deliberate, documented boundary and not in scope for this bug.
- Cosign signature verification for external images. This project does not sign
  third-party images. Adapter overlays may verify upstream-publisher signatures
  out-of-band, but that is adapter-private and not in scope here.
- The Compose `profile: monitoring` gating. Already correctly implemented via
  spec 049 and verified by `internal/deploy/monitoring_bind_contract_test.go`.

## Acceptance Criteria

1. `deploy/contract.yaml::externalImages` lists the `prom/prometheus:v2.55.1`
   pin alongside the other three external images, with a comment noting that it
   is only required when the operator enables `--profile monitoring`.
2. A new Go contract test in `internal/deploy/` parses both
   `deploy/contract.yaml` and `deploy/compose.deploy.yml`, and asserts that
   every non-built service image referenced by compose is enumerated in
   `externalImages`. The test must include an adversarial sub-test that
   demonstrates it would fail if `prom/prometheus` were removed from the
   contract.
3. `./smackerel.sh test unit --go` is green (existing 5 monitoring contracts +
   the new drift contract).
4. `docs/Deployment.md` references `deploy/contract.yaml::externalImages` as
   the canonical pin list (one-line cross-reference in the Monitoring Profile
   section).

## Product Principle Alignment

Spec 049 surfaced this drift via routine devops probing — Principle 1 ("Observe
First, Ask Second") at the agent-orchestration layer. The fix preserves
Principle 9 ("Design For Restart, Not Perfection") for adapter operators: they
should be able to enable `--profile monitoring` without having to discover the
required external image by failure.
