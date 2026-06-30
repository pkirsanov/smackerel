# Ops Packet: [OPS-005] evo-x2 Readiness-Sweep Fixes

> **Owner:** `bubbles.devops`
> **Kind:** Ops findings + supply-chain fix (readiness sweep — smackerel side)
> **Target:** repo-wide (supply-chain hardening; no live host change)
> **Status:** `in_progress` — SF-1 + SF-2 + SF-3 FIXED here; SF-4 remains a
> recorded follow-on disposition (doc refresh, owned by `bubbles.releases`).
> **Sibling packets:** [`OPS-004`](../OPS-004-homelab-activation-handoff/) (shape
> reference). No `releaseTrain` field — no smackerel `_ops` packet declares one.

---

## Summary

This packet tracks the smackerel-side findings from the evo-x2 readiness sweep
(devops + code-review + spec-review passes). The headline fix — **digest-pinning
the two profile-gated third-party images (prometheus, searxng)** — is implemented
and locked here (SF-1). Two small hardening/doc follow-ons are now also closed:
**SF-2** (compose app + prometheus image refs converted to the `${VAR:?…}`
fail-loud form) and **SF-3** (operator breadcrumb for the conditional ollama
co-residence OOM guard). **SF-4** (stale spec/review docs) remains a recorded
follow-on doc refresh owned by `bubbles.releases`.

> Out of scope here (already closed elsewhere): **BUG-069-003** (`/api/capture`
> durability) was fixed and committed under its own bug packet.

---

## Findings Ledger

| ID | Severity | Area | Location | Disposition | Owner |
|----|----------|------|----------|-------------|-------|
| **SF-1** | minor | supply-chain | `config/smackerel.yaml::monitoring.prometheus.image`, `::assistant.open_knowledge.searxng.image`; `deploy/contract.yaml::externalImages[prometheus,searxng]`; `deploy/compose.deploy.yml:353,414` | **FIXED** here — digest-pinned + drift-locked | `bubbles.devops` |
| **SF-2** | nit | fail-loud config | `deploy/compose.deploy.yml` smackerel-core / smackerel-ml / prometheus image refs | **FIXED** here — converted to `${VAR:?…}` fail-loud (matches searxng); compose/config-sync/external-images contract tests stay green | `bubbles.devops` |
| **SF-3** | doc-only | ollama envelope guard | `docs/Deployment.md` Go-Live Readiness Checklist §4 (`--profile ollama`) | **FIXED** here — operator breadcrumb: co-residence OOM guard is enforced only while `OLLAMA_KEEP_ALIVE` keeps interactive models resident | `bubbles.devops` (docs) |
| **SF-4** | doc | stale spec/review docs | `specs/_spec-review-report.md` (omits OPS-003/004); `specs/087-*`, `specs/088-*` (stale CI-as-producer narrative); `specs/082-*` (`releaseTrain=next` vs MVP framing) | follow-on — doc refresh | `bubbles.releases` / `bubbles.analyst` |

---

## SF-1 — prometheus + searxng digest pin (FIXED)

**Finding (devops F3 + code-review F-02):** Both profile-gated third-party images
were **tag-pinned, not digest-pinned**, unlike `postgres`/`nats`/`ollama` (which
are `tag@sha256:…`). `deploy/compose.deploy.yml` consumes them via
`${PROMETHEUS_IMAGE}` / `${SEARXNG_IMAGE:?…}` substitution, so the pin lands in
the SST source + `deploy/contract.yaml`.

**Resolved digests** (LIVE registry, index/manifest-list digest via
`docker buildx imagetools inspect <tag>` — the same method that produced the
existing pins; verified by `nats:2.10-alpine` reproducing its committed digest
`b83efabe…` exactly; resolved twice, stable):

| Image | Pin |
|-------|-----|
| prometheus | `prom/prometheus:v2.55.1@sha256:2659f4c2ebb718e7695cb9b25ffa7d6be64db013daba13e05c875451cf51b0d3` |
| searxng | `searxng/searxng:2026.5.30-bd863f16b@sha256:f134249dd0a1c5521d0712df81438ddfb508fe8caa5b8f76a3d413251a62ba82` |

> Anti-fabrication note: `docker manifest inspect … | grep -m1 digest` returns
> the **first platform sub-manifest** digest (prometheus `b1935d18…`), NOT the
> index digest — the wrong value for a multi-platform `tag@sha256` pin. The
> repo's pins are index digests; SF-1 uses index digests.

**Changes (this packet):**

- `config/smackerel.yaml` — `monitoring.prometheus.image` and
  `assistant.open_knowledge.searxng.image` now carry `@sha256:…`.
- `deploy/contract.yaml` — `externalImages[prometheus]` and `[searxng]` now carry
  `@sha256:…`, matching the `postgres`/`nats`/`ollama` format.
- `internal/deploy/external_images_contract_test.go` — the prometheus smoke check
  was upgraded to a **digest-pin lock** and a matching **searxng** lock was added,
  so both `${VAR}`-substituted entries are now byte-matched against their
  digest-pinned value (Check 3 byte-match intentionally skips `${…}` images;
  this lock closes that gap = TASK 2). Bumping either image now requires
  resolving the new live index digest and updating SST + contract in lockstep.

**Validation:** see `state.json::execution.validation` (config generate +
focused `ExternalImages` Go test).

---

## SF-2 — compose `${VAR}` → `${VAR:?…}` fail-loud (code F-04) — FIXED

`deploy/compose.deploy.yml` app + prometheus image refs used plain `${VAR}`
substitution; `searxng` already used the `${VAR:?…}` fail-loud form. Under
`smackerel-no-defaults`, an unset/empty image var must abort substitution loudly
with a named message rather than resolve to an empty/invalid image reference.
**Resolution (this packet):** converted the three image refs to the fail-loud
form, matching searxng (only the guard changed; the resolved value is unchanged):

- `smackerel-core` → `${SMACKEREL_CORE_IMAGE:?SMACKEREL_CORE_IMAGE must be set by the deploy adapter at apply}`
- `smackerel-ml` → `${SMACKEREL_ML_IMAGE:?SMACKEREL_ML_IMAGE must be set by the deploy adapter at apply}`
- `prometheus` → `${PROMETHEUS_IMAGE:?PROMETHEUS_IMAGE must be set when the monitoring profile is enabled}`

**Lockstep check (the guardrail):** no contract test byte-matches these compose
image strings. `internal/deploy/external_images_contract_test.go` Check 3
intentionally SKIPS `${…}`-substituted images, and its digest-pin lock reads
`deploy/contract.yaml` (not the compose `${VAR}` form); the `${VAR:?…}` form keeps
the `${` prefix so both stay green. `compose_contract_test.go` only inspects
`ports:`/`network_mode:`; `dev_compose_default_fallback_test.go` scans
`docker-compose.yml` (not the deploy compose) and only matches the forbidden `:-`
form. **No revert needed — SF-2 kept.**

**Validation:** config generate exit 0; focused
`Compose|ConfigSync|ExternalImages` Go suite green (`internal/deploy ok`,
`internal/config ok`; `go test ./... finished OK`).

## SF-3 — concurrent ollama envelope SUM guard scope (code F-03) — FIXED

`validateModelEnvelopes` enforces the co-resident VRAM sum only conditionally —
it is active only while `OLLAMA_KEEP_ALIVE` keeps the interactive hot-path models
resident (`-1` or a duration ≥ 10m); with a short/zero keep-alive the sum guard
relaxes by design (`internal/config/config.go`;
`internal/assistant/openknowledge/modelswitch/allowlist.go`). This is **by
design**, not a bug, but the conditional scope was not discoverable to operators.
**Resolution (this packet):** added a short operator breadcrumb to the
`docs/Deployment.md` **Go-Live Readiness Checklist** under §4 "Compose profile
enablement" (the `--profile ollama` item, where operators provision ollama):

> Keep `OLLAMA_KEEP_ALIVE` resident on home-lab (`-1` or a duration ≥ 10m): the
> `validateModelEnvelopes` co-residence OOM guard is enforced only while
> keep-alive keeps the interactive hot-path models resident. With a short/zero
> keep-alive the sum guard relaxes and avoiding co-residence OOM becomes the
> operator's responsibility.

Minimal, accurate, no duplication (sits next to the existing
`validateModelEnvelopes` + `ollama ps`/OOM checklist items).

## SF-4 — stale spec/review docs (spec-review F3/F4/F5)

`specs/_spec-review-report.md` omits OPS-003/004; specs 087/088 carry a stale
"CI-as-producer" narrative (the live producer is the local-operator build path,
CI is `disabled_manually`); spec 082 frames `releaseTrain=next` vs MVP.
**Disposition:** follow-on doc refresh; no runtime impact.

---

## Out of Scope (recorded for completeness)

- **BUG-069-003** (`/api/capture` durability) — already FIXED + committed under
  its own bug packet; not re-addressed here.
