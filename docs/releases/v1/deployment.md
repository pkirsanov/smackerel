# Deployment — Smackerel v1

> Per [`bubbles-deployment-target-adapter`](../../../.github/skills/bubbles-deployment-target-adapter/SKILL.md) + Gate G081, technical correctness of v1 deployment claims (new outbound-action surface, native-mobile artifact pipeline if V3-B/C lands, expanded connector OAuth flows) MUST be **validated by `bubbles.devops` (Tommy Bean)** before any external publication. This packet is the **narrative**.

## Operational plan to ship v1

v1 continues the Build-Once Deploy-Many architecture (Gate G081, [`docs/Deployment.md`](../../Deployment.md)). Same per-target adapter contract, same per-env config bundles, same cosign + Rekor + SBOM + SLSA provenance pipeline.

### What's NEW in v1 deployment

| Change | Where | Why |
|--------|-------|-----|
| Many new OAuth flows (V1-A Gmail SDK, V1-B/D Graph, V1-C Google Calendar, V1-F Reminders families, V1-G Notes families) | per-spec connector code + per-env config bundle | Each new connector ships per-user OAuth tokens; secrets injection via `bundle-secret-injection-contract` (spec 052) |
| **Outbound Action runtime surface (V2-A)** | new component in Go core | First write-side capability; needs audit log persistence (Postgres table), undo-window storage, rate-limit state |
| Conditional native mobile artifact pipeline (if V3-A chooses native) | new CI pipeline + new release channels (App Store, Play Store, sideload) | Brand-new deployment surface; potentially first signed-app pipeline; not covered by current per-server adapter contract |
| Capability map artifact (V4-A) | docs build + managed-docs sync | New auto-doc output |
| SLO alerting rules (V5-A/B) | Prometheus + observability adapter | Wired alerts per [`bubbles-observability-adapter`](../../../.github/skills/bubbles-observability-adapter/SKILL.md) |
| ML sidecar load growth (V1-I voice transcription via Whisper) | spec 050 ML sidecar scaling | Voice transcription is heavier than text embedding; may require sidecar replicas or model selection |

### Infrastructure requirements (delta from MVP)

| Layer | Delta | Source spec |
|-------|-------|-------------|
| Postgres | New tables: `outbound_action_log`, `outbound_undo_window`, `outbound_rate_limit_state`. New per-source tables per V1-A..I (where existing connector schema cannot host) | V2-A; V1-A..I |
| Secrets | Many new OAuth refresh tokens; expanded `bundle-secret-injection-contract` use | spec 052 |
| ML sidecar | Whisper model availability (V1-I); larger memory budget if local; or `LLM_PROVIDER`-routed cloud STT | spec 050; V1-I |
| Native mobile (conditional) | App-signing key custody; per-store release CI; potentially first non-server deployment artifact | V3-B/V3-C (conditional) |
| Monitoring | New alert rules; new dashboards; alert routing decision | V5-A/B; spec 049 |
| Backup | `outbound_action_log` MUST be backed up (audit-log integrity); spec 048 tier model adequate | spec 048; V2-A |

### Release-train alignment

v1 lands on the **`next` train** per [`config/release-trains.yaml`](../../../config/release-trains.yaml). Each new V-item flag:
- Default-OFF in `mvp` bundle
- Default-ON in `next` bundle (after promotion gates)
- Validated by `release-train-guard.sh`
- Flag bundle writes packeted to `bubbles.train` (sole writer)

Promotion from `next` → `mvp` train follows existing G112/G113 backup-freshness + restore-drill currency gates.

### Rollout sequence

1. **Gate-zero: MVP closed.** All RELEASE-MVP M-items terminal. `bubbles.spec-review` clean. Otherwise v1 work is parked.
2. **V2-A foundation lands FIRST.** No per-target outbound action ships before V2-A is `done` and `bubbles.devops` validates audit-log + undo-window + rate-limit + dry-run mechanics.
3. **V1-A..J connectors land in parallel** as operator engineering capacity allows. Each individually flag-gated.
4. **V2-B per-target outbound actions** land on top of V2-A, one target at a time. Each opt-in.
5. **V3-A operator decision lands.** If native, V3-B/V3-C kick off; if PWA-only, V3-A doc closes and no new spec.
6. **V4-A capability map generator lands** once at least one V1 connector is `done` (so the generator has fresh data).
7. **V5-A/B SLO alerting wires up** after MVP M1a metrics are stable and V2-A is shipping data.
8. **V6-A drift sweep** at v1 close.
9. **v1 packet refresh** flips capabilities from `planned` → `delivered` per `idea-to-release-completion` `finalReleasesPhasePosition: -2`. `bubbles.releases` MUST NOT flip to `delivered` for any spec audit did not certify as `done` (`forbidFabricatedDeliveredClaim` constraint).

### Rollback strategy

Per Gate G081, rollback remains pure pointer-swap. New v1-specific rollback concerns:

- **V2-A outbound-action rollback:** Rolling back a release that performed outbound actions does NOT un-perform those actions. The undo-window may still be open; the audit log persists. V2-A spec MUST document this explicitly. No automatic external-system undo on rollback.
- **OAuth token persistence:** Rolling back connector code does NOT invalidate OAuth tokens already granted; operator decides whether to revoke explicitly.

### Health-check + observability requirements

Per spec 049 + V5-A/B:
- V2-A outbound-action metrics: action-attempts counter (per target), success/failure/cancelled/undone counters, dry-run-vs-real ratio gauge, rate-limit-hit counter
- V1-A..I connector metrics: per-connector poll latency, items-ingested, OAuth refresh failures, schema-mismatch errors
- V4-A generator metrics: regeneration count, drift-detected count
- Alert routing: paged alerts for SLO breaches (V5-A/B); non-paged for non-SLO observability

### Test environment isolation

Per `.github/instructions/bubbles-env-pollution-isolation.instructions.md`:
- V2-A test-mode outbound actions MUST target test stacks ONLY — NEVER prod external systems (no test gmail send to a real address, no test calendar event in a real calendar). Test fixtures are required.
- Per-connector integration tests MUST use disposable OAuth credentials in ephemeral test stacks. NEVER bind a test to a real user account.

### NO-DEFAULTS / SST compliance

Per `.github/instructions/smackerel-no-defaults.instructions.md`:
- All new V-item config (OAuth client IDs, rate-limit thresholds, undo-window durations, SLO targets) MUST be sourced from `config/smackerel.yaml`; no fallbacks
- New env vars consumed by V2-A foundation MUST be fail-loud at startup
- `HOST_BIND_ADDRESS` contract unchanged

### Cross-agent handoff (REQUIRED before external v1 claim)

`bubbles.devops` validates:
- V2-A audit log non-deletable + tamper-evident
- V2-A undo-window enforces per-target-defined window
- V2-A rate-limit state survives process restarts
- All new OAuth flows correctly use `bundle-secret-injection-contract` and do NOT leak secrets to logs / metrics / traces
- New Postgres tables have proper backup coverage (spec 048)
- Native mobile pipeline (if V3-B/C) follows the same Build-Once Deploy-Many invariants AS APPLICABLE to mobile (digest-pinning replaced by store-build-id-pinning; signing-cert custody documented)
