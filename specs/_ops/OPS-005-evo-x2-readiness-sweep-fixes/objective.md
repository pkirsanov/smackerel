# Ops Packet: [OPS-005] evo-x2 Readiness-Sweep Fixes

> **Owner:** `bubbles.devops`
> **Kind:** Ops findings + supply-chain fix (readiness sweep — smackerel side)
> **Target:** repo-wide (supply-chain hardening; no live host change)
> **Status:** `in_progress` — SF-1 FIXED here; SF-2..SF-4 are recorded
> follow-on dispositions (no code change in this packet beyond SF-1).
> **Sibling packets:** [`OPS-004`](../OPS-004-homelab-activation-handoff/) (shape
> reference). No `releaseTrain` field — no smackerel `_ops` packet declares one.

---

## Summary

This packet tracks the smackerel-side findings from the evo-x2 readiness sweep
(devops + code-review + spec-review passes). The headline fix — **digest-pinning
the two profile-gated third-party images (prometheus, searxng)** — is implemented
and locked here (SF-1). The remaining findings (SF-2..SF-4) are recorded with
file:line, severity, disposition, and owner for follow-on work; they are NOT
fixed in this packet.

> Out of scope here (already closed elsewhere): **BUG-069-003** (`/api/capture`
> durability) was fixed and committed under its own bug packet.

---

## Findings Ledger

| ID | Severity | Area | Location | Disposition | Owner |
|----|----------|------|----------|-------------|-------|
| **SF-1** | minor | supply-chain | `config/smackerel.yaml::monitoring.prometheus.image`, `::assistant.open_knowledge.searxng.image`; `deploy/contract.yaml::externalImages[prometheus,searxng]`; `deploy/compose.deploy.yml:353,414` | **FIXED** here — digest-pinned + drift-locked | `bubbles.devops` |
| **SF-2** | nit | fail-loud config | `deploy/compose.deploy.yml:124` (`${SMACKEREL_CORE_IMAGE}`), `:184` (`${SMACKEREL_ML_IMAGE}`), `:353` (`${PROMETHEUS_IMAGE}`) | follow-on — convert to `${VAR:?…}` fail-loud (searxng `:414` already does) | `bubbles.devops` |
| **SF-3** | doc-only | ollama envelope guard | `internal/config/config.go:389`; `internal/assistant/openknowledge/modelswitch/allowlist.go:144,264` | follow-on — operator-guidance breadcrumb only (guard scope is conditional by design) | `bubbles.implement` / docs |
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

## SF-2 — compose `${VAR}` not fail-loud `${VAR:?}` (code F-04)

`deploy/compose.deploy.yml` app + prometheus image refs use plain `${VAR}`
substitution; `searxng:414` already uses the `${VAR:?…}` fail-loud form. Under
`smackerel-no-defaults`, an unset/empty image var should abort substitution
loudly rather than resolve to an empty image. **Disposition:** follow-on; convert
`:124`, `:184`, `:353` to `${VAR:?…}` and keep `internal/deploy/compose_contract_test.go`
in lockstep.

## SF-3 — concurrent ollama envelope SUM guard scope (code F-03)

`validateModelEnvelopes` enforces the co-resident VRAM sum only conditionally
(`envelopeMiB != 0` and the switchable/gather sets are populated) —
`internal/config/config.go:389` documents this; `allowlist.go:144,264` implement
it. This is **by design**, not a bug. **Disposition:** doc-only — add an
operator-guidance breadcrumb (when the envelope is unset, co-residence is the
operator's responsibility) so the conditional scope is discoverable.

## SF-4 — stale spec/review docs (spec-review F3/F4/F5)

`specs/_spec-review-report.md` omits OPS-003/004; specs 087/088 carry a stale
"CI-as-producer" narrative (the live producer is the local-operator build path,
CI is `disabled_manually`); spec 082 frames `releaseTrain=next` vs MVP.
**Disposition:** follow-on doc refresh; no runtime impact.

---

## Out of Scope (recorded for completeness)

- **BUG-069-003** (`/api/capture` durability) — already FIXED + committed under
  its own bug packet; not re-addressed here.
