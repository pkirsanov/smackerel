# User Validation: [OPS-001] spec.md status-banner sweep

## Checklist

### [Ops Sweep] [OPS-001] Reconcile spec.md status banners with state.json for 54 certified specs
- [x] **What:** Every certified spec's `spec.md` opens with a banner that matches `state.json: status == "done"`.
  - **Steps:**
    1. From repo root, spot-check 5 random specs across the 4 categories (e.g., 003 from Category A, 030 from Category B, 040 from Category C, 056 from Category D, plus one of the 2 already-correct specs).
    2. For each, run `head -5 specs/NNN-*/spec.md` and confirm the `**Status:**` banner matches the canonical form defined in `spec.md` EB-1..EB-4.
    3. Run the enumeration script: `python3 enumerate_banner_drift.py` (or equivalent inline `python3 -c ...` from terminal history).
    4. Confirm "Total drifted: 0".
    5. Run `git diff --name-only` and confirm only `specs/NNN-*/spec.md` (54 paths) and `specs/_ops/OPS-001-spec-banner-sweep/` (8 paths) appear.
  - **Expected:** All 54 affected `spec.md` files carry the canonical Done banner; the 2 already-correct certified specs are untouched; portfolio drift count is 0; change boundary is clean.
  - **Verify:** the enumeration script reports "Total drifted: 0" AND `git diff --name-only` contains zero forbidden paths.
  - **Evidence:** `report.md` → Test Evidence → Post-Fix Regression Test (populated by `bubbles.implement` when the sweep is dispatched).
  - **Notes:** Artifact-only sweep; no code, compose, or config changes. `tdd.exempt` per `policySnapshot.tdd.mode = exempt`. Workflow mode `spec-scope-hardening` terminates at ceiling `specs_hardened` (G093 blocks `done` for metadata-only packets). Packet authored by `bubbles.bug`; sweep execution dispatched separately by the user via `bubbles.implement`.
