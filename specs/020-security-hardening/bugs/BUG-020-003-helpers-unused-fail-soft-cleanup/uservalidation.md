# User Validation — BUG-020-003 cmd/core/helpers.go unused fail-soft helpers cleanup (HL-RESCAN-014 / Gate G028)

> **STATUS — STUB.** This file is a placeholder authored by the `bubbles.bug` specialist to seed the validation phase. The `bubbles.validate` specialist OWNS this artifact and MUST replace each `[ ]` item below with `[x]` and inline evidence references after the fix lands and validation passes.
>
> **⚠️ CHECKED-BY-DEFAULT POLICY.** Per the `bubbles.bug` mode rules, validation entries MUST use `[x]` (checked) once validated — `[x]` = working as expected (default after validation). Only the USER unchecks items if a regression is observed; specialists MUST NOT leave items `[ ]` after a successful validation pass.

## Checklist

- [x] AC-1 — A single-scope bug packet exists at `specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/` with all 7 standard artifacts (`spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, `state.json`, `scenario-manifest.json`); `state.json` declares `parentWorkflow.mode = "home-lab-readiness-rescan-2026-05-14"`, `workflowMode = "bugfix-fastlane"`, and `discoveryRef.findingId = "HL-RESCAN-014"`. → `report.md` Summary + Validation Evidence → Packet completeness.
- [x] AC-2 — `grep_search` of all `*.go` files for the dead-set symbols (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`, `parseJSONObject`, `parseJSONObjectVal`) returns ZERO matches anywhere in the repo after the fix lands. → `report.md` Validation Evidence → Symbol-removal audit.
- [x] AC-3 — `cmd/core/helpers.go` does not contain any `os.Getenv("KEY")` followed by a `return <literal>` / `return nil` silent-fallback path; any env reads remaining follow the Gate G028 canonical Go pattern. → `report.md` Test Evidence §6 + Validation Evidence → Imports verification.
- [x] AC-4 — `cmd/core/main_test.go` does not contain any test function that locks the silent-fallback semantics for the deleted helpers. → `report.md` Test Evidence §1 (8 preserved `TestParseJSONArray_*` PASS, dead-set tests absent) + Code-diff stat in §5.
- [x] AC-5 — Persistent in-tree adversarial regression guard exists and runs on every `./smackerel.sh test unit --go` invocation; RED→GREEN proof captured in `report.md`. → `report.md` Test Evidence §7 (adversarial AST sub-test) + Validation Evidence → RED→GREEN proof.
- [x] AC-6 — All existing Go unit tests pass after the deletion; `./smackerel.sh test unit --go` returns exit 0; live callers of `parseJSONArray` at `cmd/core/connectors.go:76,103` continue to compile and behave identically. → `report.md` Test Evidence §2 (full Go unit lane PASS) + Validation Evidence → Connectors no-change canary.
- [x] AC-7 — Generic-only constraint preserved: zero real hostnames, IPs, tailnet identifiers, owner-username tokens, real geographic locations, real Tailscale identifiers, or real systemd unit names introduced. PII paths in evidence blocks redacted to `~/smackerel`. The tokens `Gate G028` and `HL-RESCAN-014` are policy/finding identifiers and are explicitly allowed. → `report.md` Audit Evidence → Generic-only constraint verification.

## Acceptance Criteria Verification

| AC | Description | Result | Evidence Reference |
|----|-------------|--------|--------------------|
| AC-1 | Bug packet skeleton with all 7 artifacts | PASS | report.md#summary + Validation Evidence → Packet completeness |
| AC-2 | Dead-set symbols removed from repo | PASS | report.md Validation Evidence → Symbol-removal audit |
| AC-3 | No silent-fallback patterns in cmd/core/helpers.go | PASS | report.md Test Evidence §6 + Validation Evidence → Imports verification |
| AC-4 | Dead-helper test cases removed from cmd/core/main_test.go | PASS | report.md Test Evidence §1 + Test Evidence §5 (Code-diff stat) |
| AC-5 | Persistent regression guard locks the contract | PASS | report.md Test Evidence §7 + Validation Evidence → RED→GREEN proof |
| AC-6 | Existing Go unit tests pass; live callers untouched | PASS | report.md Test Evidence §2 + Validation Evidence → Connectors no-change canary |
| AC-7 | Generic-only constraint preserved | PASS | report.md Audit Evidence → Generic-only constraint verification |

## Bounded-Scope Validation

Confirmed: this packet did NOT touch:

- Foreign-owned spec 020 content (`spec.md`, `design.md`, `scopes.md`, `state.json`, `report.md`, `uservalidation.md` at the parent feature level — read-only references only).
- Other parent specs' bug packets under the `home-lab-readiness-rescan-2026-05-14` discovery mode.
- Parallel-session WIP under `specs/041-qf-companion-connector/` and `specs/052-bundle-secret-injection-contract/`.
- Live production callers of `parseJSONArray` at `cmd/core/connectors.go:76,103`.
- The ML sidecar (`ml/`) — Python equivalent finding HL-RESCAN-013 closed separately as BUG-020-002.

Evidence: see `report.md` Validation Evidence → Connectors no-change canary + Test Evidence §2 (full Go unit lane PASS shows parallel-WIP packages green) + `git status` clean for foreign-owned paths.

## Cross-Reference Verification

Confirmed: cross-references in [`spec.md`](spec.md) Sister Packets + Cross-References Already Filed in Sister Packets sections still resolve correctly after this packet lands. The docs phase updates the sister packets that previously deferred work to HL-RESCAN-014 (BUG-020-002 uservalidation.md line 107, BUG-044-001 spec.md line 62 + design.md line 18 + scopes.md line 209, BUG-029-003 report.md line 347) so they reference this closed packet by path.

## Sequel Surfaces

Documented per [`design.md`](design.md) DD-6. These are surfaces that BUG-020-003 explicitly does NOT close (outside this packet's change boundary per [`spec.md`](spec.md) Bounded Surface) but which the operator may choose to file as separate sequel bug packets.

### Candidate sequel packet — BUG-020-004 (proposed)

**Title:** `cmd/core/connectors.go` `parseJSONArray` live callers silently coerce parse errors to empty exclusion lists; convert to fail-loud `(value, error)` reads or refuse connector construction

**Surface:** [`cmd/core/connectors.go`](../../../../cmd/core/connectors.go) lines 76 and 103.

**Live caller evidence (verified by [`spec.md`](spec.md) Detection > Verified Call-Site Inventory):**

- Line 76: `parseJSONArray(cfg.BookmarksExcludeDomains)` — used to construct the bookmarks-connector exclusion-domain list.
- Line 103: `parseJSONArray(cfg.BrowserHistoryCustomSkipDomains)` — used to construct the browser-history-connector skip-domain list.

**Why deferred from BUG-020-003:**

- BUG-020-003 is bounded to the dead-set helpers in `cmd/core/helpers.go` per [`spec.md`](spec.md) Bounded Surface section.
- Converting the live `parseJSONArray` callers to fail-loud `(value, error)` reads (or refusing connector construction on parse error) requires a separate spec-level decision about acceptable connector-construction-time failure semantics — the bookmarks and browser-history connectors currently construct successfully even when their exclusion lists fail to parse (silently treating the result as "no exclusions"). Changing that behaviour is a deliberate operator-facing trade-off, not a Gate G028 mechanical cleanup.
- The 8 `TestParseJSONArray_*` test cases preserved by BUG-020-003 will need to be revisited if `parseJSONArray`'s signature changes from `[]interface{}` to `(value []interface{}, err error)`.

**Recommended sequel resolution path (if operator chooses to take it on):**

- **Option A (preferred):** Convert `parseJSONArray` to `parseJSONArray(s string) ([]interface{}, error)` returning a typed error on parse failure; update both `cmd/core/connectors.go` callers to either propagate the error to refuse connector construction, or explicitly degrade to empty-list with a `slog.Error` on the connector's structured logger (NOT a silent fallback — the logger call MUST name the connector + the parse-error context).
- **Option B (defer-with-policy):** Leave `parseJSONArray` as-is BUT document the silent-fallback semantics in `cmd/core/helpers.go` doc comment AND in `docs/Connector_Development.md` so future connector authors are aware of the trade-off.

**Owner:** Operator chooses (no automatic dispatch from BUG-020-003).

**File path (when filed):** `specs/020-security-hardening/bugs/BUG-020-004-connectors-parsejsonarray-live-callers-fail-loud/`

- [ ] Operator has reviewed this sequel-surface entry and decided whether to file BUG-020-004.
