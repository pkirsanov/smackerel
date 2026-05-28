# User Validation: [BUG-015-003] Spec 015 forward pointer to spec 056

## Checklist

### [Bug Fix] [BUG-015-003] Record spec-056 extension pointer in spec 015
- [x] **What:** Spec 015 now records that spec 056 implements the `SyncModeAPI` + `SyncModeHybrid` path, both in `state.json` (additive forward-pointer entry) and inline in `spec.md` (top-of-document banner + API Access Strategy section note) and `design.md` (top-of-document banner).
  - **Steps:**
    1. Open `specs/015-twitter-connector/state.json` and confirm an additive forward-pointer entry (e.g., `extensions[]` with `extendedBy: "056-twitter-api-connector"`, or `supersessions[]` fallback) scoped to `SyncModeAPI` + `SyncModeHybrid`.
    2. Open `specs/015-twitter-connector/spec.md` and confirm a top-of-document banner naming spec 056 and clarifying that spec 015 ships the archive path only.
    3. Confirm a second inline note appears above (or immediately under) the "API Access Strategy — Critical Design Decision" section heading pointing at spec 056 as the implementation home.
    4. Open `specs/015-twitter-connector/design.md` and confirm the same forward-pointer banner appears at the top of the document.
  - **Expected:** All three artifacts point a reader at spec 056 for the API/hybrid implementation; no reader can conclude that spec 015's deliverables include API/hybrid code.
  - **Verify:** `grep -n '056-twitter-api-connector' specs/015-twitter-connector/{state.json,spec.md,design.md}` returns at least one match per file (spec.md ≥ 2).
  - **Evidence:** report.md → Post-Fix Regression Test section
  - **Notes:** Artifact-only fix. No code, compose, or test files are touched. Recipe mirrors `specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/`.
