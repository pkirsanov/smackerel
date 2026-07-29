# Report: BUG-019-003 — `artifacts.source_ref` never persisted

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

### Summary

This packet documents a data-integrity defect in the shared connector ingestion front door: `internal/pipeline/ingest.go::PublishRawArtifact` omits `source_ref` from its `INSERT INTO artifacts` column list (`:94`) and probes for duplicates against the `source_url` column while binding `RawArtifact.SourceRef` (`:51`). Consequently `artifacts.source_ref` is NULL for all 17 registered connectors, source-ref deduplication is structurally unreachable, the `idx_artifacts_source (source_id, source_ref)` composite index is dead for connector traffic, and the GuestHost booking-context API together with both bookmark dedup paths match zero rows unconditionally.

Five findings are recorded — `F-BUG-019-003-001` (persistence, D1), `F-BUG-019-003-002` (dedup mis-probe, D2), `F-BUG-019-003-003` (GuestHost identity semantics, D3 — **decided as Option 4, owner-ratified 2026-07-29**), `F-BUG-019-003-004` (a certified sibling bug reasoned against a mechanism that does not exist), and `F-BUG-019-003-005` (a scoping correction preventing wasted fix effort on unaffected `location_clusters` and `photos` read sites).

The packet was authored under workflow mode `product-to-planning` (statusCeiling `specs_hardened`) with Gate G073 Source Code Edit Lockout ACTIVE. **Zero source, migration, config, test, or documentation files were modified.** No test was executed.

### Verification Baseline

All defect claims below were confirmed by direct read-only source inspection at the cited coordinates during packet authoring. **Claim Source: `executed`** — read-only inspection of repository files. No runtime execution, no test invocation.

Source paths inspected:

- `internal/pipeline/ingest.go` (lines 47-52 dedup probe, line 94 `INSERT` column list, lines 57 and 107 log fields)
- `internal/pipeline/processor.go` (line 523 second `INSERT`)
- `internal/connector/connector.go` (line 43 `RawArtifact.SourceRef`)
- `internal/connector/supervisor.go` (line 362 shared publish loop)
- `cmd/core/connectors.go` (lines 73-79 the 17-connector registration literal)
- `internal/db/migrations/001_initial_schema.sql` (line 29 column, line 68 index, lines 322-323 the separate `location_clusters` table)
- `internal/api/context.go` (lines 273-289 booking-context query, comment at 276-277)
- `internal/connector/bookmarks/dedup.go` (lines 131-133, 160, 184-189)
- `internal/connector/guesthost/normalizer.go` (lines 27-36) and `internal/connector/guesthost/types.go` (line 10)
- `internal/connector/photos/store.go` (lines 124, 197, 363) and `internal/drive/scan/service.go` (line 316) — the only `artifacts.source_ref` writers
- `internal/knowledge/sensitivity_query.go` (line 116)
- `internal/connector/weather/weather.go` (lines 150, 273-275) and `internal/connector/maps/normalizer.go` (lines 132-140) — non-reconstructable ref derivations
- `internal/connector/maps/connector.go` (line 475), `internal/connector/maps/patterns.go` (line 225), `internal/connector/photos/routing.go` (line 288) — verified unaffected
- `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/bug.md` (lines 42, 78) — false-premise corroboration
- `config/release-trains.yaml`, `docs/smackerel.md` (lines 2490, 2945), `docs/API.md`, `docs/Architecture.md`, `docs/releases/mvp/features.md`, `.github/bubbles/capability-ledger.yaml` — propagation targets

The per-claim confirmation table is recorded in [bug.md → Verification Evidence](bug.md#verification-evidence-static-planning-mode).

### Test Evidence

**No tests were executed for this packet.** Gate G073 (Source Code Edit Lockout) is active under workflow mode `product-to-planning`, whose declared constraint is `focus: planning_only` with `allowImplementationForFindings: false`. Executing the fix or its tests is outside this mode's authority.

Fabricating test output here would violate the anti-fabrication policy, so this section records the absence honestly rather than presenting a placeholder result.

The complete runtime reproduction sequence a fixer must execute — with expected pre-fix outcomes stated so a genuine reproduction can be distinguished from an environment fault — is specified in [bug.md → Reproduction Plan](bug.md#reproduction-plan-not-executed-in-planning-mode). The full test matrix (unit, integration against a real ephemeral test database, and e2e), including the adversarial regression fixture requirement, is specified across the six scopes in [scopes.md](scopes.md).

The adversarial regression contract is the load-bearing test requirement of this packet: the fixture must use two artifacts sharing the same `source_id` and the same non-empty `source_ref` but carrying **different** `content_hash` values, so that the pre-existing `ON CONFLICT (content_hash)` clause cannot mask a missing `source_ref` path. A fixture in which every row already satisfies the broken code path would be tautological and is explicitly disallowed by [scopes.md → Scope 1 Adversarial Regression Requirement](scopes.md#adversarial-regression-requirement-blocking).

### Scenario Coverage

Eight scenarios with stable identifiers are declared in [spec.md](spec.md) and registered in [scenario-manifest.json](scenario-manifest.json): `SCN-BUG-019-003-001` through `SCN-BUG-019-003-008`. Each maps to at least one acceptance criterion and at least one Test Plan row in [scopes.md](scopes.md). `SCN-BUG-019-003-005` is no longer decision-blocked following the D3 Option 4 ratification of 2026-07-29; like all eight scenarios it remains planned but unexecuted until a `bugfix-fastlane` delivery run implements it.

### Unresolved Items

| Item | Finding | Owner | Why it is not resolved here |
|---|---|---|---|
| `internal/pipeline/processor.go:523` disposition | `F-BUG-019-003-001` | Fixer, recorded during Scope 3 | Requires determining at runtime whether that path creates or only updates rows. Cannot be settled by inspection alone. |
| Backfill reconstructability for the conditional connector class | `F-BUG-019-003-001` | Fixer, recorded during Scope 5 | Depends on whether each connector independently wrote its identifier into `metadata` JSONB, which requires a survey against real stored rows. |

### Resolved Decisions

| Item | Finding | Resolution | Ratifier / decider |
|---|---|---|---|
| GuestHost booking identity semantics — should `source_ref` carry `event.ID`, `EntityID`, a composite, or dual persistence? | `F-BUG-019-003-003` | **Option 4** — `source_ref = event.EntityID` for booking event types, `event.ID` retained in `metadata`, coupled to Re-ingestion **Option B**. The consumer contract at `internal/api/context.go:284` is expressed in business-entity terms; event IDs are transport artifacts and entity IDs are domain identity, and only an entity-keyed column lets `idx_artifacts_source` serve the many-to-one lookup. Options table: [design.md](design.md#d3-identity-semantics-decision-decided--option-4). | Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by `bubbles.bug` on behalf of the operator. Tracked as `TR-BUG-019-003-001` (`resolved`). |
| Re-ingestion matched-row semantics — skip, update-in-place, or new revision? | `F-BUG-019-003-002` | **Option B** (update-in-place, raw-capture columns only, re-published to NATS). Decision of record in [design.md](design.md#re-ingestion-semantics-and-the-over-suppression-risk); it is a hard prerequisite of D3 Option 4, since the three booking lifecycle events collide on one ref by design and skip-on-match would freeze bookings in their created state. | Packet decision of record (`bubbles.bug`), locked in by the D3 Option 4 ratification above. The fixer must still **observe and assert** the behaviour in Scope 2 — that is execution, not decision. |

### Routing Raised

`F-BUG-019-003-004` records that `specs/016-weather-connector/bugs/BUG-016-W3-source-ref-collision/` (status `done`) was analysed and certified on the premise that pipeline deduplication uses `SourceRef` as part of artifact identity (`bug.md:78`) — a premise that D1 and D2 falsify. That packet is a **foreign artifact** and was not reopened, modified, or re-certified by this work. Its own fix remains independently valid. The routing item was **accepted as advisory** on 2026-07-29 (`TR-BUG-019-003-002`, `OR-02`): a premise-correction annotation is to be appended to that packet **after** BUG-019-003 is fixed, with **no re-certification** implied or authorised. Nothing under `specs/016-weather-connector/` was modified in the ratification run either.

### Discovered Issues

| Date | Issue | Disposition |
|---|---|---|
| 2026-07-28 | The maps `source_ref` read/write sites cited during triage (`internal/connector/maps/patterns.go:225`, `internal/connector/maps/connector.go:475`) operate on the `location_clusters` table, not `artifacts`. `location_clusters.source_ref` is `TEXT NOT NULL` (`001_initial_schema.sql:323`) and is correctly written. Likewise `internal/connector/photos/routing.go:288` reads the `photos` table. | Recorded as `F-BUG-019-003-005` scoping correction in [bug.md](bug.md#f-bug-019-003-005--scoping-correction-recorded-to-prevent-wasted-fix-effort) and as a Non-Goal in [spec.md](spec.md#non-goals) so the fixer does not spend effort on unaffected code. |
| 2026-07-28 | `internal/knowledge/sensitivity_query.go:116` uses `COALESCE(source_url, COALESCE(source_ref, ''))` and therefore degrades safely rather than failing. | Recorded as affected-but-non-failing. Not a blocker; a post-fix precedence review is a Scope 3 DoD item. |
| 2026-07-28 | `.github/bubbles/capability-ledger.yaml` is the Bubbles framework ledger and carries no product-level provenance capability entry. | The corresponding Scope 6 DoD item is written as a re-verification item rather than an edit item, so the fixer does not invent a capability entry that does not exist. |

### Completion Statement

This bug packet is **complete for the `product-to-planning` workflow mode** and has reached that mode's status ceiling of `specs_hardened`. Eight artifacts were created under `specs/019-connector-wiring/bugs/BUG-019-003-source-ref-never-persisted/`; zero source, migration, config, test, or documentation files were modified; zero tests were executed.

The fix itself is **not started** and is deliberately out of this mode's authority. Every scope in [scopes.md](scopes.md) is `Not Started` and every Definition-of-Done item is unchecked, which is the correct and intended state for a planning-mode packet rather than an omission. The D3 identity-semantics decision was ruled on 2026-07-29 (Option 4, owner-ratified — see [uservalidation.md](uservalidation.md) `OR-01`) and delivery via `bugfix-fastlane` was approved (`OR-03`), but **neither promotes this packet**: promotion beyond `specs_hardened` still requires a delivery-capable workflow mode to actually run and produce the full evidence chain specified in the six scopes. Approval schedules that run; it does not substitute for it.
