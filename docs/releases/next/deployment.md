# Deployment — Smackerel `next`

> **Technical-accuracy routing.** This document describes promotion shape, not a
> concrete host. `bubbles.devops` owns validation of every technical claim below
> before it is relied on externally (OPS-N2 in [`actions.md`](actions.md)). No
> hostname, IP address, tailnet identifier, operator account, or per-target manifest
> appears here or anywhere else in this repository — those live exclusively in the
> knb deploy-adapter overlay.

## What phase `next` deploys to

Release train `next` targets slot **`staging`** (`config/release-trains.yaml`, owned
by `bubbles.train`). `staging` is the promotion-candidate slot: a capability set
proves itself there before it is promoted onto the `mvp` train's `home-lab` slot.

The train-to-phase binding is one-to-one and is registered in
[`../README.md`](../README.md): train `next` ⇄ phase `next` ⇄ this packet. That
binding is what gives the train a Gate G101 surface. Before this packet existed the
train had eleven member specs and no delivery enforcement at all.

## Artifact contract

`next` introduces no new artifact classes. It rides the existing repository
deployment contract unchanged:

| Artifact | Identity | Mutable? |
|---|---|---|
| `smackerel-core` image | content-addressed digest | no |
| `smackerel-ml` image | content-addressed digest | no |
| per-environment config bundle | deterministic, generated from the single-source config for a given source SHA | no |
| build manifest | CI artifact keyed by source SHA | no |
| deployment manifest pointer | operator-controlled, lives in the deploy-adapter overlay | yes |

Invariants that this phase must not weaken, restated so a reader of this packet does
not have to reconstruct them:

- **Digests only.** A mutable tag in a deployment manifest is a deploy-time defect, not a style preference.
- **CI stops at registry push.** The build pipeline never applies, never connects to a target. Promotion is an operator action against the adapter.
- **Verify before start.** Signature and bundle-hash verification happen before any container starts. There is no bypass flag.
- **Rollback is a pointer swap.** Rollback never rebuilds and never pulls source.
- **The adapter consumes, never builds.** No compiler, bundler, or image build runs inside an adapter script.

## Promotion sequence for this phase

1. A source SHA is built by CI and published with its manifest.
2. Gate G101 for phase `next` is green — every `required` binding in [`features.md`](features.md) resolves to a terminal, non-blocked, validate-certified spec.
3. The operator promotes the manifest onto the `staging` slot through the adapter, by digest.
4. Post-apply verification confirms the running digests match the deployment manifest. Drift is a blocking error, not a warning.
5. Only after `staging` holds does a promotion onto the `home-lab` slot become a candidate — and that is a `bubbles.train` decision, not a `bubbles.releases` one.

Step 2 is the part this packet adds. It did not previously exist for this train.

## Rollout characteristics specific to `next`

| Capability | Rollout consideration |
|---|---|
| `095` retrieval-strategy routing | Behaviour is governed by fail-loud configuration under the single-source config file. A missing key aborts startup rather than silently defaulting — that is intended, and an operator seeing the fail-loud error is the system working. |
| `088` runtime-switchable models | Model selection changes at runtime without a redeploy. That means a deploy does **not** pin model behaviour; a promotion validating "the answer quality" is validating the model that happened to be selected. Record the selected model alongside any quality claim. |
| `080` knowledge-graph public API | A new externally-reachable read surface. Its exposure is an edge-layer decision made in the deploy adapter, not here. Default posture: not published beyond the trusted network boundary. |
| `081` NATS sidecar hardening parity | Touches the bus between the Go core and the Python ML sidecar. The sidecar remains compute-only: it holds no datastore credentials and reaches data only through the owning service tier over the typed contract wire. A deploy that hands the sidecar a database or bus URL violates that boundary. |
| `082` target readiness hardening | Its checks are preconditions on the target, and they run before apply. Failing them is the intended stop. |

## Health checks and observability

Every service in this phase exposes health and metrics endpoints on the existing
contract. Telemetry emitted during any test category carries an `env=test*` label and
targets the ephemeral test stack's own monitoring. The production/operate plane is
read-only from a feature scope. Nothing in this phase writes to prod monitoring, prod
backup paths, or a deployment manifest from a test.

Alerting for this phase is the existing rule set. Note honestly that no
retrieval-quality or synthesis-quality SLO alert exists yet — the reserved
`next-retrieval-quality-pipeline` slot carries the evaluation gate that would make
such an alert meaningful. Claiming quality monitoring before that lands would be a
fabricated capability.

## Rollback strategy

Pointer swap only. The previous deployment manifest is re-applied from version
history and the prior digests are re-pinned. No rebuild, no source pull, no partial
state.

Phase-specific note: because `088` makes model selection a runtime property, a
rollback restores the **code** but not necessarily the **model selection** that was
active before. If a rollback is motivated by answer quality, the operator must also
confirm which model is selected after the swap.

## What this phase does NOT change

- No new host singleton is touched. Reverse proxy, container runtime, host firewall, init system, and mesh-VPN identity are all unchanged by this phase and are adapter-owned regardless.
- No new persistent store. Every capability here operates over the one existing store (Product Principle 5).
- No new secret class. Multi-provider model credentials belong to `096`, which is blocked and therefore not part of this phase's deployable set.
