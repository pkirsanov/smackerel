# Release Schema Review

**Purpose:** record a completed read-only review of Smackerel's release schema — release trains and release phase packets — and assess how much of it the current plans repair.

| Field | Value |
|---|---|
| Review date | 2026-08-02 |
| Reviewed commit | `81208a33` (working tree clean, 0 dirty files) |
| Review mode | read-only diagnostic |
| Artifacts promoted | none |
| Files modified by this review | none except this document |

## Verdict

**The current plans do not fix the overall release schema.** Of eleven distinct defects found across the two release axes, the standing plan of record — [`Product_Direction_2026-07-31.md`](Product_Direction_2026-07-31.md) — addresses **one**. The single addressed defect (F4) is diagnosed well and its proposed remedy is the correct root-cause shape, but that finding is the only one in the plan marked "do not promote", is absent from the plan's own routing section, and is sequenced behind ten prior steps. Meanwhile the release-train configuration gate **fails today** (F1) and is not covered by any plan at all.

## Scope And Non-Goals

**In scope.** The release schema across two axes:

| Axis | Source of truth | Gate | Owning agent |
|---|---|---|---|
| Release trains | [`config/release-trains.yaml`](../config/release-trains.yaml), `config/feature-flags.*.yaml` | G110 | `bubbles.train` |
| Release phase packets | [`docs/releases/`](releases/) | G101 | `bubbles.releases` |

**Non-goals.** No configuration file, feature-flag bundle, or release packet was modified. No spec, scope, or state artifact was created or mutated. No finding was promoted. Edits to train configuration and to packets were deliberately withheld because those artifacts belong to `bubbles.train` and `bubbles.releases` respectively; this review routes rather than edits.

## Executed Evidence

### A. Release-train guard — `EXIT=1`

```
bash .github/bubbles/scripts/release-train-guard.sh "$(pwd)"
```

Result: **exit 1**, 7 errors, 266 warnings. Error lines follow.

> Transcript sanitized: repository-absolute path prefixes are elided as `...`, and knb-owned
> target-slot values are replaced with `<redacted-*>` placeholders per the deployment-boundary
> policy. All other characters are verbatim.

```
[release-train-guard][ERROR] train 'mvp' has invalid target_slot '<redacted-target-slot>' (expected prod|staging|<redacted-allowed-slot>|none)
[release-train-guard][ERROR] spec .../specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup status=in_progress missing releaseTrain field
[release-train-guard][ERROR] spec .../specs/061-conversational-assistant/bugs/BUG-061-008-execution-errors-masked-as-saved-as-idea status=in_progress missing releaseTrain field
[release-train-guard][ERROR] spec .../specs/061-conversational-assistant/bugs/BUG-061-007-weather-shortcut-masked-as-saved-as-idea status=in_progress missing releaseTrain field
[release-train-guard][ERROR] spec .../specs/061-conversational-assistant/bugs/BUG-061-006-duplicate-contradictory-capture-ack status=in_progress missing releaseTrain field
[release-train-guard][ERROR] spec .../specs/003-phase2-ingestion/bugs/BUG-003-002-topic-momentum-star-count status=in_progress missing releaseTrain field
[release-train-guard][ERROR] release-train-guard FAILED
```

### B. Release-delivery reconciliation, phase `v1` — `EXIT=0`

```
bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh --repo-root "$(pwd)" --phase v1 --require-coverage
```

Result: **exit 0**. `reconciling phase 'v1' (1 feature annotation(s))`, one row — `v1  retrieval-strategy-routing  optional  NOT-ENFORCED` — then `OK (G101: all required features delivered + validate-certified)`.

### C. Release-delivery reconciliation, phase `mvp` — `EXIT=0`

Same command with `--phase mvp`. Result: **exit 0**, 36 feature annotations, 5 `required` rows (`m1a`, `m2a`, `m2b`, `m4`, `m5d`), all `DELIVERED`.

## Findings

| ID | Severity | Owner | Finding |
|---|---|---|---|
| **F1** | Critical | `bubbles.train` | Release-train SST does not validate |
| **F2** | High | `bubbles.releases` | v1's G101 binding is vacuous |
| **F3** | High | `bubbles.releases` | Three vocabularies for one axis |
| **F4** | High | `bubbles.releases` | v1 packet states falsified claims |
| **F5** | High | `bubbles.releases` | MVP and v1 contradict each other |
| **F6** | High | `bubbles.releases` | The packets are not a census |
| **F7** | Medium | `bubbles.releases` | Train `next` has no phase packet |
| **F8** | Medium | `bubbles.releases` | The packets are stale |
| **F9** | Medium | `bubbles.releases` | Structural gaps in the packet set |
| **F10** | — | — | Coverage assessment of the plan of record |

### F1 — Release-train SST does not validate (Critical, `bubbles.train`)

G110 is a **failing gate today**, not a latent risk. Evidence A. Two distinct causes:

1. [`config/release-trains.yaml`](../config/release-trains.yaml) L16 sets an unsupported `target_slot` for train `mvp`. This review omits concrete target values because knb owns their binding. Train `next` uses a G110-accepted slot at L22.
2. Five `in_progress` bug specs carry no `releaseTrain` field: `BUG-069-004`, `BUG-061-008`, `BUG-061-007`, `BUG-061-006`, `BUG-003-002`.

**Impact.** Every guarded train operation refuses while this stands. No plan of record covers this defect.

### F2 — v1's G101 binding is vacuous (High, `bubbles.releases`)

Evidence B against C. The v1 packet plans roughly two dozen capabilities (the V1-A..J, V2-A/B, V3-A/B/C, V4-A, V5-A/B, V6-A, V7-A/B/C series) and carries exactly **one** `bubbles:feature` annotation — [`releases/v1/features.md`](releases/v1/features.md) L78, `delivery=optional`. The guard therefore prints a green "all required features delivered" against a single non-enforced row. The packet admits this in a machine-binding note at L79: "full-packet machine reconciliation is a future `bubbles.releases` backfill."

**Impact.** G101 on phase v1 asserts nothing. A green exit code is being produced by an empty enforcement set.

### F3 — Three vocabularies for one axis (High, `bubbles.releases`)

| Vocabulary | Values | Source |
|---|---|---|
| Trains | `mvp`, `next` | [`config/release-trains.yaml`](../config/release-trains.yaml) |
| Phases | `mvp`, `v1` | [`docs/releases/`](releases/) |
| Deferral token | `release-v1` | [`releases/mvp/features.md`](releases/mvp/features.md) |

`docs/releases/release-v1/` does not exist, so `deferred-to:release-v1` is a **dangling reference**. It appears in 3 machine-binding annotations (L20, L22, L28); 12 lines in that file mention the token in total. No gate catches it: the reconciliation guard wildcards the token at [`release-delivery-reconciliation-guard.sh`](../.github/bubbles/scripts/release-delivery-reconciliation-guard.sh) L259 — `deferred-to:*) : ;;`.

### F4 — v1 packet states falsified claims (High, `bubbles.releases`)

[`releases/v1/features.md`](releases/v1/features.md) L74 states spec 095 is "**PLANNED / specced — NOT delivered**", `planningOnly: true`, at the `specs_hardened` ceiling, and "Zero source delivered — `internal/retrieval/` does not exist yet."

Verified reality:

| Claim | Actual |
|---|---|
| `planningOnly: true`, `specs_hardened` | [`specs/095-.../state.json`](../specs/095-retrieval-strategy-routing/state.json) → `status=done`, `workflowMode=full-delivery`, `releaseTrain=next` |
| "`internal/retrieval/` does not exist yet" | [`internal/retrieval/`](../internal/retrieval/) exists, 24 `.go` files |

**Impact.** The stale premise is encoded in the L78 `delivery=optional` flag, so it governs a gate's enforcement decision, not merely a prose sentence.

### F5 — MVP and v1 contradict each other (High, `bubbles.releases`)

[`releases/mvp/features.md`](releases/mvp/features.md) L20, L22, L28 (restated L184, L186, L192) mark `m1b-calendar-triggered-briefs`, `m1c-promise-engine-full`, and `m5b-chrome-extension-bridge` as `delivery=deferred-to:release-v1` — that is, **not delivered at MVP**. [`releases/v1/features.md`](releases/v1/features.md) L13, L14, L18 list M1b, M1c, and M5a–d as "**carry forward unchanged**", which asserts prior delivery. Both cannot be true.

Related spec state: [`specs/058-chrome-extension-bridge`](../specs/058-chrome-extension-bridge/) is `blocked`; [`specs/025-knowledge-synthesis-layer`](../specs/025-knowledge-synthesis-layer/) is `done`. The MVP packet **does** record 058 as blocked (L93, L153, L157, L192). The v1 packet contains zero occurrences of either `058` or `blocked`, so its "carry forward unchanged" row silently absorbs a blocked capability.

### F6 — The packets are not a census (High, `bubbles.releases`)

| Measure | Value |
|---|---|
| Numbered spec directories under `specs/` | 109 (plus `specs/_ops`) |
| Distinct specs bound by a `bubbles:feature ... spec=` annotation in any packet | 34 |
| Unbound | 75 — of which **67 are `done`** |
| Train `mvp` members | 19; **1** bound in a packet (`078-cross-surface-surfacing-prioritizer`) |
| Train `next` members | 11; **1** bound (`095-retrieval-strategy-routing`) |

Of the 18 unbound `mvp`-train specs, the MVP packet records 2 (`097`, `099`) as lineage prose, leaving **16 wholly unaccounted** — including [`specs/100-unified-journey-ui-transformation`](../specs/100-unified-journey-ui-transformation/), which is `done` and user-facing. All 10 unbound `next`-train specs are unaccounted. [`specs/089-runtime-model-hotswap-persistent-selection`](../specs/089-runtime-model-hotswap-persistent-selection/) is `done` with **no train at all**.

The MVP packet self-disclaims exhaustiveness at L176: "This record is not asserted to be an exhaustive census of post-freeze mvp-train specs."

### F7 — Train `next` has no phase packet (Medium, `bubbles.releases`)

[`docs/releases/`](releases/) contains only `mvp/` and `v1/`. Train `next` — 11 specs — has no phase packet, so no G101 surface exists for it.

### F8 — The packets are stale (Medium, `bubbles.releases`)

| Measure | Value |
|---|---|
| Last commit touching `docs/releases/` | `5869ee3c`, 2026-07-12 |
| Newest date appearing inside any packet | 2026-06-23 |
| Commits touching `specs/` since `5869ee3c` | **183** |
| Review date | 2026-08-02 |

### F9 — Structural gaps (Medium, `bubbles.releases`)

- No `docs/releases/README.md` phase index exists.
- [`releases/v1/features.md`](releases/v1/features.md) L129 self-flags that its proposed slots `specs/077`–`specs/090` collide with real unrelated specs and calls this "non-blocking". It is not: it invalidates every row of v1's Plan-to-Release Traceability table.
- [`releases/mvp/actions.md`](releases/mvp/actions.md) L76-77 marks itself "HISTORICAL SNAPSHOT ... superseded ... not the live dispatch list", so the MVP phase has **no live dispatch list**.
- v1 actions carry no owners, dates, or exit criteria.

### F10 — Coverage assessment of the plan of record

[`Product_Direction_2026-07-31.md`](Product_Direction_2026-07-31.md) lists "release truth" in its reviewed target (L13) and contains exactly one release finding: **A4-LEDGER** (L208).

**Credit where due.** A4-LEDGER independently identifies the same spec-095/V7 drift as F4, and diagnoses it more sharply than staleness:

> "G101 therefore declines to enforce delivery for a feature that is already delivered, so a stale premise now owns a release gate's enforcement decision, not merely a status sentence."

Its remedy — generate release claims **and their machine-binding delivery annotations** from a runtime capability ledger, failing on projection drift (invariant **A4**, L422) — is the correct root-cause shape.

**Why coverage is nonetheless 1 of 11.**

| Observation | Location |
|---|---|
| A4-LEDGER is the only finding in the document marked `Promote to spec: **no**` | L208 |
| It is absent from §6 Spec Promotion Candidates (12 routing rows, none owns it) | L437-457 |
| §7 records "Findings promoted: no" and "Specs/design/scopes/reports/state updated or created: no" | L458-465 |
| Its mechanical defence is bound to Step 11, the terminal node of the Sequencing DAG, gated behind Steps 1-10 | §5 |
| "no product stack, test suite, exploit, browser journey, database query, JetStream workload, or provider delivery was run" | L17 |
| "Gate G101 was not executed" | L481 |

Because no gate was executed, the plan reviewed the packets (the G101 axis) and never examined train configuration (the G110 axis) — which is precisely where the schema actually fails today.

## Coverage Matrix

| # | Defect | Axis | Addressed by `Product_Direction_2026-07-31.md`? |
|---:|---|---|---|
| 1 | F1a — G110-rejected `target_slot` on train `mvp` | G110 | no |
| 2 | F1b — 5 `in_progress` bug specs missing `releaseTrain` | G110 | no |
| 3 | F2 — v1 G101 binding is vacuous (1 optional annotation) | G101 | no |
| 4 | F3 — three vocabularies; dangling `deferred-to:release-v1` | G101 | no |
| 5 | F4 — v1 packet's falsified spec-095 claims | G101 | **yes** (A4-LEDGER) |
| 6 | F5 — MVP/v1 mutual contradiction | G101 | no |
| 7 | F6 — packets are not a census (75 unbound, 67 `done`) | G101 | no |
| 8 | F7 — train `next` has no phase packet | G101 | no |
| 9 | F8 — packets stale by 183 spec-touching commits | G101 | no |
| 10 | F9a — no phase index; v1 slot collisions invalidate traceability | G101 | no |
| 11 | F9b — MVP has no live dispatch list; v1 actions lack owners | G101 | no |

**Coverage: 1 of 11.**

## Assessment Of A4-LEDGER's Remedy

Credited above. Three weaknesses limit it:

| # | Weakness | Consequence |
|---|---|---|
| 1 | **Scoped to projection drift only** | A generated ledger fixes stale claims but cannot fix an invalid enum in the train SST (F1), a missing `releaseTrain` (F1), a phase that does not exist (F3/F7), or a dangling `deferred-to:` (F3). Those are topology defects upstream of projection. |
| 2 | **Owner bundling** | Packet projection is `bubbles.releases`; train configuration is `bubbles.train`. One finding cannot route to two owners — a plausible reason it routed to neither. |
| 3 | **Circular regression fixture** | A4 names "the spec 095 V7 mismatch" as its regression fixture, while tactical move T5 assigns correcting that same row to a documentation edit. If T5 lands first, the fixture can no longer fail. |

## Recommended Improvements

Ranked. **Not executed by this review.**

| # | Improvement | Owner |
|---:|---|---|
| 1 | Split the release axis into two findings: packet projection and failing train schema | `bubbles.releases` / `bubbles.train` |
| 2 | Promote A4-LEDGER and add it to §6 of the product-direction plan — currently that document's sole orphan finding | plan owner |
| 3 | Un-gate release repair from Step 11; G110 fails today and depends on none of Steps 1-10 | plan owner |
| 4 | Execute the release gates during review and record `G101`/`G110` command + exit code in the evidence appendix; the "nothing was run" boundary is what hid F1 | reviewing agent |
| 5 | Widen A4's invariant from *projection drift* to *release topology completeness*: every train has a phase packet; every terminal spec on a train appears in exactly one packet; `deferred-to:<phase>` resolves to a real phase directory | `bubbles.releases` |
| 6 | Add a census invariant so packets must be exhaustive per train | `bubbles.releases` |
| 7 | Give A4 a pre-state regression fixture that fails before the T5 edit, or sequence T5 after the generator lands | plan owner |

## Evidence Boundary

**Executed for this review:**

- The two release guards, with exit codes recorded verbatim above (`release-train-guard.sh` → exit 1; `release-delivery-reconciliation-guard.sh` for phases `v1` and `mvp` → exit 0).
- `jq` reads of `state.json` files under `specs/`.
- `git log` and `git rev-list --count` for packet recency and spec churn.
- Directory and file listings under `docs/releases/`, `specs/`, and `internal/retrieval/`.
- `grep`/`sed` reads over the packets, the train configuration, the reconciliation guard, and the product-direction plan.

**Not executed:**

- No product stack was started. No test suite was run.
- No deployment and no train operation (cut, promote, rollback, retire) was performed.
- No packet, configuration file, or feature-flag bundle was modified.
- No finding was promoted to a spec, scope, or state artifact.

**Boundary on F10.** The assessment of [`Product_Direction_2026-07-31.md`](Product_Direction_2026-07-31.md) is a **static read of that document** — its findings table, routing section, artifact-outputs section, sequencing, and appendix. It is not a re-execution or re-validation of that plan's own findings. Where this document says the plan "does not address" a defect, that means the defect is absent from the plan's text, not that the plan was tested against it.

**Divergences from the review intake packet.** Re-verification at `81208a33` produced four corrections, recorded here rather than silently substituted:

| Item | Intake packet | Verified at `81208a33` |
|---|---|---|
| F3 `release-v1` occurrences | 3 | 3 machine-binding annotations (L20, L22, L28); 12 lines mention the token |
| F4 claim location | `v1/features.md` L73 | **L74** (L73 is blank); the `delivery=optional` annotation is L78 |
| F5 "058 recorded as blocked in neither packet" | asserted | **False for MVP** — recorded blocked at L93, L153, L157, L192. True for v1, which mentions neither `058` nor `blocked` |
| F6 census | 110 spec dirs, 35 bound, 76 unbound (67 `done`) | **109** numbered dirs (+`specs/_ops`), **34** bound, **75** unbound, **67** `done` (unchanged) |

The two guard invocations were **not** re-run for this document; their exit codes and output are reproduced as executed earlier in the same session at the same commit.

## Routing And Next Actions

| Finding | Route to | Action |
|---|---|---|
| F1 | `bubbles.train` | Correct train `mvp` release metadata so G110 accepts its abstract slot. Keep the concrete deployment binding in knb. Add `releaseTrain` to the 5 `in_progress` bug specs. Restores G110. |
| F2-F9 | `bubbles.releases` | Packet reconciliation, census completeness, phase index, `next` packet, vocabulary unification, contradiction resolution, live dispatch lists. |
| F10 improvements | plan owner / owning agents per the table above | Promote and re-sequence A4-LEDGER; widen its invariant. |

This review made **no edits to train configuration and no edits to any release packet**, by design: those artifacts are owned by `bubbles.train` and `bubbles.releases`. Correcting them here would have crossed an artifact-ownership boundary and produced changes their owners did not author.
