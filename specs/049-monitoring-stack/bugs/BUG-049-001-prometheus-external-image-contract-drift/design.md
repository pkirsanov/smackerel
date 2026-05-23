# Design — BUG-049-001 — Prometheus external image contract drift

## Current Truth

```text
$ grep -nA1 "externalImages:" deploy/contract.yaml
22:externalImages:
23-- name: postgres
24-  image: pgvector/pgvector:pg16
25-- name: nats
26-  image: nats:2.10-alpine
27-- name: ollama
28-  image: ollama/ollama:0.23.2

$ grep -n "prom/prometheus" config/smackerel.yaml deploy/compose.deploy.yml
config/smackerel.yaml:883:    image: "prom/prometheus:v2.55.1"
(no match in deploy/compose.deploy.yml — image flows via ${PROMETHEUS_IMAGE} from SST)

$ grep -n "PROMETHEUS_IMAGE" deploy/compose.deploy.yml
289:    image: ${PROMETHEUS_IMAGE}
```

The pin lives in SST (`config/smackerel.yaml::monitoring.prometheus.image`) and
flows through `scripts/commands/config.sh` (lines 452-468 extract, line ~1530
write to env file) into `${PROMETHEUS_IMAGE}` for compose substitution. The
`deploy/contract.yaml::externalImages` list is the operator-facing summary that
documents which third-party images the deploy adapter must pull. The pin source
exists; the contract summary is missing one entry.

## Design

### Fix 1 — Contract Update

Append the `prom/prometheus` pin to `deploy/contract.yaml::externalImages` with
a comment noting the `monitoring` profile gating and the SST key that owns the
pin.

```yaml
externalImages:
- name: postgres
  image: pgvector/pgvector:pg16
- name: nats
  image: nats:2.10-alpine
- name: ollama
  image: ollama/ollama:0.23.2
# Spec 049 — only required when operator enables `--profile monitoring`.
# SST pin: config/smackerel.yaml::monitoring.prometheus.image
- name: prometheus
  image: prom/prometheus:v2.55.1
  profile: monitoring
```

The new optional `profile:` field is descriptive metadata for adapter overlays;
the existing entries omit it (they are in the default profile set). Adding a
field is backward-compatible: no existing reader of `externalImages` parses
fields other than `name` + `image`.

### Fix 2 — Regression Contract Test

Add `internal/deploy/external_images_contract_test.go` that:

1. Parses `deploy/contract.yaml` and extracts `externalImages[*].image`.
2. Parses `deploy/compose.deploy.yml` and enumerates `services.*.image` entries.
3. For each service image, classifies it:
   - **Project-built**: image refers to `smackerel-core` or `smackerel-ml`
     (these come from `images:` in the contract, not `externalImages`).
   - **SST-substituted external**: image is `${VAR}`-style and the resolved
     value (looked up in `config/smackerel.yaml`) is pinned to a third-party
     registry.
4. Asserts every SST-substituted external image is enumerated in
   `externalImages` with the matching image string.
5. Includes an adversarial sub-test that builds an in-memory contract YAML with
   `prom/prometheus` removed from `externalImages` and asserts the checker
   fails with a clear error naming the missing image and SST key.

Test categorization: `unit` (no docker, no compose runtime, pure file parsing).

### Fix 3 — Doc Cross-Reference

Append a one-line note to `docs/Deployment.md` in the "Monitoring Profile (Spec
049 — Optional)" section directing adapter operators to
`deploy/contract.yaml::externalImages` for the canonical external-image pin
list.

## Adversarial Considerations

- **Drift recurrence**: a future contributor could add a fifth third-party
  service (e.g., Grafana for spec 049 follow-on) and again forget the contract
  update. The new contract test forces the update at lint time.
- **Comment-only enforcement**: relying on the `# Spec 049 — ...` comment in
  the YAML is not enough — humans miss it during PR review. The Go contract
  test is the durable mechanism.
- **Schema brittleness**: adapters that already parse `externalImages` may
  not tolerate the new `profile:` field. Verified by reading
  `deploy/_example/target-skeleton/apply.sh` — it only mentions `name` +
  `image` semantics; unknown fields are ignored by every YAML parser by
  default. Backward-compatible.
- **False positive risk**: the contract test must NOT flag `${REGISTRY}` or
  service-template substitutions where the resolved value is a project-built
  image. The classifier in step 3 above explicitly excludes those.

## Out Of Scope

Already enumerated in spec.md. No changes to spec 047 scope, no changes to
cosign verification, no changes to the monitoring profile mechanics.

## Risk

- Low: pure documentation + a new pure-parsing contract test. No runtime path,
  no behaviour change for any operator who has already implemented an adapter
  (the new `profile:` field is additive and ignored by older parsers).
