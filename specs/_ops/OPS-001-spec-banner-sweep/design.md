# Ops Packet Design: [OPS-001] spec.md status-banner sweep

## Root Cause Analysis

### Investigation Summary
Each spec carries two status surfaces: (1) the authoritative runtime control plane in `state.json` (`status` field, governed by workflow-mode ceilings and `state-transition-guard.sh`), and (2) the human-facing `**Status:**` banner near the top of `spec.md`. The control-plane → banner sync was never automated. When a spec was certified (status promoted in `state.json`), the banner was not rewritten. Specs authored under later templates omitted the banner line entirely. The result is portfolio-wide narrative drift: 54 of 56 certified specs misrepresent their own status to a reader who opens `spec.md` first.

### Root Cause
**Process / template gap, not a code defect.** There is no mechanism — neither a lint guard nor a certification-time hook — that rewrites the spec.md banner when `state.json: status` reaches a terminal value. Spec authoring templates have also evolved: earlier templates required the banner, later templates dropped it, hence Categories A vs B.

### Impact Analysis
- **Affected components:** 54 `spec.md` files under `specs/NNN-*/` (one per affected spec) plus the 8 artifacts of this ops packet.
- **Affected data:** none.
- **Affected users:** any human or agent auditing portfolio status by reading `spec.md` first. Risk: false "still Draft" conclusion, redundant certification cycles, misallocated work.
- **Affected runtime:** none. `state.json` is unchanged and remains authoritative.

## Fix Design

### Solution Approach
A single-line edit per affected `spec.md`, mechanically derived from the per-spec category. The implementing agent processes the 4 categories with 4 distinct edit shapes:

| Category | Count | Edit shape |
|---|---|---|
| A — banner says "Draft" | 23 | Replace the single line `**Status:** Draft` with `**Status:** Done (certified per state.json)`. |
| B — no banner at all | 27 | Insert `**Status:** Done (certified per state.json)` as a new line immediately after the H1, with one blank line on each side (do not duplicate an existing blank line). |
| C — multi-word stale banner | 3 | Read the exact existing `**Status:**` line; replace it wholesale with `**Status:** Done (certified per state.json)`. |
| D — spec 056 special case | 1 | Replace `**Status:** Draft (planning packet — \`specs_hardened\` target)` with `**Status:** Done (was planning packet — promoted on certification)`. |

The implementing agent MUST use IDE file-editing tools (`replace_string_in_file` / `multi_replace_string_in_file`), NEVER shell redirection or `python … > file`, per `agent-common.md` Terminal Discipline.

### Affected Files
| Category | File(s) | Lines touched per file |
|---|---|---|
| A | 23 × `specs/NNN-*/spec.md` | 1 line replaced |
| B | 27 × `specs/NNN-*/spec.md` | 1 line inserted (+ at most 1 blank line normalized) |
| C | 3 × `specs/{038,040,041}-*/spec.md` | 1 line replaced |
| D | 1 × `specs/056-*/spec.md` | 1 line replaced |
| Packet | 8 × `specs/_ops/OPS-001-spec-banner-sweep/*` | full artifact set |

### Alternative Approaches Considered
1. **Per-spec `BUG-NNN` packet for each of 54 specs.** Rejected — process-heavy, generates 54 × 8 = 432 artifact files for a one-line-per-spec fix. Ops-packet scope is exactly the right size.
2. **Auto-rewrite the banner inside `state-transition-guard.sh` on every certification.** Rejected for this packet — that is the correct long-term fix and should be filed as a follow-on, but doing it here expands change boundary outside `specs/`.
3. **Delete the banner entirely from every spec and rely on `state.json` only.** Rejected — the banner is a useful UX surface for a reader who opens `spec.md` first; deleting it makes auditing worse.
4. **Use a single banner line that says "see state.json" without naming status.** Rejected — defeats the point of the banner.

### Regression Test Design
This is artifact-only documentation drift. Regression coverage is artifact-shape:

- **Pre-fix (adversarial) — MUST FAIL on `main` before the sweep:**
  - The enumeration script described in `bug.md` Reproduction Steps reports "Total drifted: 54".
- **Post-fix — MUST PASS after the sweep:**
  - Same enumeration script reports "Total drifted: 0".
  - For each Category A/C/D file, `grep -E '^\*\*Status:\*\*' spec.md` returns the exact canonical line.
  - For each Category B file, the same grep returns the canonical line AND the line sits on the second non-blank logical line of the file (right after H1).
- **Idempotence — MUST PASS after re-running the sweep:**
  - Re-applying the same edits produces zero `git diff`.
- **Change-boundary adversarial guard — MUST PASS:**
  - `git diff --name-only` returns only `specs/NNN-*/spec.md` paths for the 54 specs plus `specs/_ops/OPS-001-spec-banner-sweep/` paths. No `state.json`, no `design.md`, no `scopes.md`, no code, no compose.
- **No-overreach adversarial guard — MUST PASS:**
  - The 2 already-correct certified specs do NOT appear in `git diff --name-only`.
- **No bailout patterns:** every assertion grep MUST be direct (`grep -q ... ; echo $?`) — no `if file_missing: return 0` early exits.

### Change Boundary
- ALLOWED: `specs/NNN-*/spec.md` for the 54 enumerated specs only; all artifacts under `specs/_ops/OPS-001-spec-banner-sweep/`.
- FORBIDDEN: any file outside the above — including `state.json` of the 54 specs, `design.md`/`scopes.md`/`report.md`/`uservalidation.md`/`scenario-manifest.json` of the 54 specs, any code (`internal/`, `cmd/`, `ml/`, `web/`), any compose (`docker-compose*.yml`, `deploy/`), any policy (`.github/instructions/`, `.specify/memory/`), any `spec.md` outside the 54-spec list.
- Enforcement: implementing agent MUST run `git diff --name-only` before claiming Done and reject the change if any forbidden path appears.

## Workflow Mode & Ceiling
- Mode: `spec-scope-hardening` (per `.github/bubbles/workflows.yaml` L1337).
- `statusCeiling: specs_hardened` — gate G093 blocks promotion to `done` for planning / metadata-only packets. This packet IS metadata-only, so it terminates at `specs_hardened`.
- `tdd.exempt` applies: zero runtime code changed; regression coverage is artifact-shape by construction.
- Required gates (per workflow mode): G001, G002, G006, G007, G008, G010, G011, G012, G014, G015, G016, G073.
