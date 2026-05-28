# Ops Packet: [OPS-001] spec.md status-banner drift across 54 certified specs

## Summary
Spec-review pass P1-1 surfaced that **54 of 56 certified specs** carry a `spec.md` "**Status:**" banner that does not match their `state.json` `status: "done"`. The runtime control plane (`state.json` + workflow-mode ceilings) is the source of truth for spec status; the human-facing `spec.md` banner is the secondary surface a reader sees first. The two have drifted because banners were written at spec-authoring time and were never updated when each spec was certified.

This is **artifact-only narrative drift** across a portfolio of specs. No runtime code changes. No `state.json` changes. No `design.md` / `scopes.md` / `report.md` changes. The fix is one canonical banner line per affected `spec.md`, mechanically derived from `state.json`.

Tracked as an ops packet (not a per-spec bug) because:
1. The defect is identical in shape across 54 specs and is best fixed as one atomic sweep, not 54 individual `BUG-NNN` packets.
2. No single feature owns the drift; it is portfolio-wide governance hygiene.
3. The packet lives under `specs/_ops/` to distinguish it from per-feature `specs/NNN/bugs/` packets.

## Severity
- [ ] Critical
- [ ] High
- [x] Medium — governance documents misrepresent their own certification state; misleads any reader (human or agent) auditing portfolio status; no runtime impact
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed (reproduced — enumerated below)
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. From repo root, enumerate every spec with `state.json: status == "done"`.
2. For each such spec, extract the first occurrence of `**Status:**` from the first ~15 lines of `spec.md`.
3. Compare the banner's first word (lowercased) to `done`.
4. Any spec where the comparison fails is in drift.

The enumeration that produced the 54-spec list (run prior to this packet) returned the breakdown in the "Affected Specs" section below. Two certified specs (the two not in the list) already carry a matching `**Status:** Done` banner.

## Affected Specs (54 total)

### Category A — Banner says "Status: Draft" (23 specs)
`specs/001-smackerel-mvp/`, `specs/002-phase1-foundation/`, `specs/003-phase2-ingestion/`, `specs/004-phase3-intelligence/`, `specs/005-phase4-expansion/`, `specs/006-phase5-advanced/`, `specs/007-google-keep-connector/`, `specs/008-telegram-share-capture/`, `specs/009-bookmarks-connector/`, `specs/010-browser-history-connector/`, `specs/011-maps-connector/`, `specs/012-hospitable-connector/`, `specs/013-guesthost-connector/`, `specs/014-discord-connector/`, `specs/015-twitter-connector/`, `specs/016-weather-connector/`, `specs/017-gov-alerts-connector/`, `specs/019-connector-wiring/`, `specs/025-knowledge-synthesis-layer/`, `specs/026-domain-extraction/`, `specs/027-user-annotations/`, `specs/028-actionable-lists/`.

### Category B — No `**Status:**` banner at all (27 specs)
`specs/021-intelligence-delivery/`, `specs/022-operational-resilience/`, `specs/023-engineering-quality/`, `specs/024-design-doc-reconciliation/`, `specs/029-devops-pipeline/`, `specs/030-observability/`, `specs/031-live-stack-testing/`, `specs/032-documentation-freshness/`, `specs/033-mobile-capture/`, `specs/034-*/` … `specs/037-*/`, `specs/039-*/`, `specs/042-tailnet-edge-bind-pattern/`, `specs/043-*/` … `specs/055-*/`.

(Exact enumeration: `021, 022, 023, 024, 029, 030, 031, 032, 033, 034, 035, 036, 037, 039, 042, 043, 044, 045, 046, 047, 048, 049, 050, 051, 052, 053, 054, 055`.)

### Category C — Multi-word stale banner (3 specs)
- `specs/038-*/spec.md` — `**Status:** Draft (analyst-owned requirements sections only)`
- `specs/040-*/spec.md` — `**Status:** Draft (analyst-owned requirements sections)`
- `specs/041-*/spec.md` — `**Status:** Draft (analyst-owned requirements sections)`

(Exact wording per spec captured at fix time; the literal forms above are the templates the implementing agent MUST match before replacement.)

### Category D — Planning-packet self-description that survived certification (1 spec)
- `specs/056-*/spec.md` — `**Status:** Draft (planning packet — \`specs_hardened\` target)`. `state.json` IS `done`. Banner self-describes as planning-only and was never reconciled when the spec was promoted past `specs_hardened`.

## Expected Behavior
After the fix, every certified spec's `spec.md` opens with a canonical first-line banner immediately under the H1 that names `done` and credits `state.json` as the source of truth:

```
# <H1 title>

**Status:** Done (certified per state.json)
```

For spec 056 (Category D), the banner reads:

```
**Status:** Done (was planning packet — promoted on certification)
```

## Actual Behavior
54 of 56 certified specs misrepresent their own status to a reader who opens `spec.md` first. Banner drift falls into the four categories above.

## Environment
- Repo: `smackerel` @ current `main`
- Authoritative status surface: each spec's `state.json` (`status` field)
- Drifted surface: each spec's `spec.md` (`**Status:**` banner line)
- Discovery: spec-review P1-1
- Packet path: `specs/_ops/OPS-001-spec-banner-sweep/`
- Workflow mode: `spec-scope-hardening` (ceiling `specs_hardened`; gate G093 blocks `done` for planning/metadata-only packets — this packet is metadata-only and therefore terminates at the ceiling)

## Root Cause
Banner text was authored at spec-creation time when most specs were genuinely Draft. The `state.json` → `done` certification flow updates the control-plane status but does NOT mechanically rewrite the human-facing `spec.md` banner. Without a sweeping back-fill, every spec certified historically still reads as Draft (or carries no banner at all because later spec templates dropped the banner line).

This is a recurring class of drift, not a one-off. The packet's secondary value is documenting the drift pattern so a future template / lint guard can prevent reintroduction.

## Related
- Discovery surface: spec-review P1-1 finding
- Workflow mode: `spec-scope-hardening` per `.github/bubbles/workflows.yaml` L1337
- Comparable artifact-only precedent: `specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/` (also promoted to `specs_hardened`)
- Gate references: G028 (NO-DEFAULTS — irrelevant here, code untouched), G093 (planning-only ceiling enforcement — IS relevant; this packet terminates at `specs_hardened`)
