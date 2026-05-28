# Bug Fix Design: [BUG-020-007] Spec 020 supersession pointer to spec 042

## Root Cause Analysis

### Investigation Summary
Spec-review pass surfaced P0-2: spec 020 prescribes literal `127.0.0.1:HOST_PORT` Docker host binds and a "100% of services bind to 127.0.0.1" acceptance criterion. Spec 042 (`tailnet-edge-bind-pattern`) was filed and shipped later, explicitly reversing that prescription (host binds for `smackerel-core` and `smackerel-ml` now use the fail-loud `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` form per `.github/instructions/smackerel-no-defaults.instructions.md` and gate G028; `postgres` and `nats` have no host `ports:` block at all and are reached via `docker exec` over Tailscale SSH). The deployed code at `deploy/compose.deploy.yml` and the contract test at `internal/deploy/compose_contract_test.go` already match spec 042.

What never happened: the supersession was not back-propagated into spec 020's artifacts. `specs/020-security-hardening/state.json` has no `supersededBy` pointer to `042-tailnet-edge-bind-pattern`. `specs/020-security-hardening/spec.md` and `specs/020-security-hardening/design.md` carry no inline supersession note. A reader picking up spec 020 today is misled into believing literal `127.0.0.1` binding is the current target pattern.

### Root Cause
Process gap, not a code defect: spec 042 did not file a back-pointer edit against spec 020 when it shipped. The drift is artifact-only.

### Impact Analysis
- **Affected components:** `specs/020-security-hardening/spec.md`, `specs/020-security-hardening/design.md`, `specs/020-security-hardening/state.json` — documentation only.
- **Affected data:** none.
- **Affected users:** any human or agent reading spec 020 to model new deploy/compose work. Risk: they reintroduce the literal `127.0.0.1:HOST_PORT` Docker mapping form, which gate G028 would correctly reject, but the spec contradiction creates wasted review cycles.
- **Affected runtime:** none. `deploy/compose.deploy.yml` and `internal/deploy/compose_contract_test.go` already enforce the spec-042 invariant.

## Fix Design

### Solution Approach
Artifact-only, three-file edit inside `specs/020-security-hardening/`:

1. **`state.json` — supersession pointer.** Add a schema-valid `supersededBy` (or equivalent supersession provenance) entry naming `042-tailnet-edge-bind-pattern` and scoping the supersession to the host-bind / loopback-binding prescription (so the rest of spec 020 — auth, crypto hygiene, etc. — is not implied to be superseded). The implementing agent MUST pick the correct field by reading the state schema before editing; the v3 control-plane state model already carries `policySnapshot`, `transitionRequests`, `reworkQueue`, etc., and supersession provenance is one of those structured fields.
2. **`spec.md` — inline supersession note.** Add an inline note at the top of the "Attack Surface: Network Exposure" section (L48–52) and update the L269 success-metric row. The note MUST name spec 042 explicitly and MUST mention the fail-loud `${HOST_BIND_ADDRESS:?...}` form for `smackerel-core` / `smackerel-ml` and the "no host `ports:` block" invariant for `postgres` / `nats`. A document-top banner is acceptable if it explicitly enumerates the affected sections.
3. **`design.md` — inline supersession note.** Mirror the spec.md note at L5–10 of `design.md` (or at the top of the document) pointing to spec 042.

### Affected Files
| File | Change |
|------|--------|
| `specs/020-security-hardening/state.json` | Add `supersededBy` (schema-valid form) → `042-tailnet-edge-bind-pattern`, scoped to host-bind prescription |
| `specs/020-security-hardening/spec.md` | Inline supersession note at the "Attack Surface: Network Exposure" section (L48–52); update or annotate L269 success-metric row |
| `specs/020-security-hardening/design.md` | Inline supersession note at L5–10 |

### Alternative Approaches Considered
1. **Rewrite spec 020 in place to remove the literal `127.0.0.1` prescription entirely.** Rejected — destroys historical reading; spec-042's supersession provenance becomes harder to audit. Inline supersession notes preserve history while pointing the reader at the current pattern.
2. **File a new spec that "owns" the bind pattern and retire both 020 and 042.** Rejected — spec 042 is already that spec; the only missing work is the back-pointer.
3. **Touch `deploy/compose.deploy.yml` to "re-confirm" the spec-042 form.** Rejected — code already matches spec 042; touching it would expand the change boundary and trigger unnecessary code-path review.

### Regression Test Design
This is artifact-only drift. The regression test is the artifact-shape check itself:

- **Pre-fix (adversarial) assertions — MUST FAIL on `main` before the fix:**
  1. `grep -q 'supersededBy.*042-tailnet-edge-bind-pattern' specs/020-security-hardening/state.json` → exit 1.
  2. `grep -q '042-tailnet-edge-bind-pattern' specs/020-security-hardening/spec.md` → exit 1.
  3. `grep -q '042-tailnet-edge-bind-pattern' specs/020-security-hardening/design.md` → exit 1.
- **Post-fix assertions — MUST PASS after the fix:** the same three greps return exit 0.
- **Adversarial case:** the L269 success-metric row, after the fix, must NOT match the bare pattern `100% of services bind to 127.0.0.1` *without* an adjacent supersession reference. The regression script MUST grep for the bare phrase, and if found, MUST verify a `042-tailnet-edge-bind-pattern` reference appears within ±5 lines, otherwise fail. This catches the "annotated-then-someone-stripped-the-annotation" reintroduction path.
- **No bailout patterns:** the script MUST assert directly; no `if file_missing: return 0` early exits.

### Change Boundary
- ALLOWED: any file under `specs/020-security-hardening/` (including this bug folder).
- FORBIDDEN: any file outside `specs/020-security-hardening/` (no `deploy/`, no `internal/`, no `.github/instructions/`, no `specs/042-*`).
- Enforcement: implementing agent MUST run `git diff --name-only` before claiming Done and reject the change if any path outside `specs/020-security-hardening/` appears.
