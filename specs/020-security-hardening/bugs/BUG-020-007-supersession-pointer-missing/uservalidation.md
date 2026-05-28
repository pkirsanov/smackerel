# User Validation: [BUG-020-007] Spec 020 supersession pointer to spec 042

## Checklist

### [Bug Fix] [BUG-020-007] Record spec-042 supersession in spec 020
- [x] **What:** Spec 020 now records that spec 042 supersedes its host-bind prescription, both in `state.json` and inline in `spec.md` / `design.md`.
  - **Steps:**
    1. Open `specs/020-security-hardening/state.json` and confirm a `supersededBy` (or equivalent schema-valid supersession provenance) entry naming `042-tailnet-edge-bind-pattern`.
    2. Open `specs/020-security-hardening/spec.md` and confirm an inline supersession note above or inside the "Attack Surface: Network Exposure" section (L48–52) naming spec 042 and the fail-loud `${HOST_BIND_ADDRESS:?...}` form.
    3. Confirm the L269 success-metric row "Host port binding" either references spec 042 or is rewritten to match the spec-042 invariant (no host `ports:` block for `postgres` / `nats`).
    4. Open `specs/020-security-hardening/design.md` and confirm the same supersession note appears at L5–10 (or top of the document).
  - **Expected:** All three artifacts point a reader at spec 042 for the canonical bind-address pattern; no reader can conclude that literal `127.0.0.1:HOST_PORT` is still the current target.
  - **Verify:** `grep -n '042-tailnet-edge-bind-pattern' specs/020-security-hardening/{state.json,spec.md,design.md}` returns at least one match per file.
  - **Evidence:** report.md → Post-Fix Regression Test section
  - **Notes:** Artifact-only fix. No code, compose, or test files are touched.
