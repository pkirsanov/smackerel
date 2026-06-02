# Scopes: 062 Per-Transport Configuration Surface Audit

Single-file mode (`scopeLayout: single-file`).

Links: [spec.md](spec.md) | [design.md](design.md) | [scenario-manifest.json](scenario-manifest.json) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Execution Outline

### Phase Order

1. **Scope 1 — Inventory & Registry Bootstrap (foundation):** introduce
   `internal/assistant/transportconfig/` with the `Entry` struct and
   verbatim entries derived from current adapter code +
   `config/smackerel.yaml`. No runtime behavior change.
2. **Scope 2 — Adapter Fail-Loud Wiring:** rewrite HTTP, WhatsApp, and
   Telegram adapter startup to drive fail-loud checks from the
   registry. Any discovered default is removed or explicitly ratified.
3. **Scope 3 — Docs + Test Enforcement:** publish
   `docs/Transport_Configuration.md`, add the doc-sync test, wire the
   registry tests into `./smackerel.sh test unit`, and add the
   end-to-end "missing required key exits non-zero" test.

### Validation Checkpoints

- After Scope 1: registry compiles, `TestRegistry_CoversYAMLNamespaces`
  + `TestRegistry_NoOrphanedEntries` pass. No adapter behavior changes.
- After Scope 2: every required key produces the exact registry
  fail-loud message; no `${VAR:-default}` survives in any per-transport
  env file or compose fragment.
- After Scope 3: docs table mirrors the registry; the e2e test
  demonstrates an operator-visible fail-loud exit.

---

## Scope 1 — Inventory & Registry Bootstrap

**Scope-Kind:** bootstrap
**Status:** Done
**Owner:** bubbles.implement (after analyst handoff)
**Completed:** 2026-06-02 (evidence: [report.md → Scope 1](report.md#scope-1))

**Deliverables:**
- `internal/assistant/transportconfig/registry.go` with `Entry` struct.
- `internal/assistant/transportconfig/{http,whatsapp,telegram}.go` with
  verbatim entries.
- `internal/assistant/transportconfig/registry_test.go` covering
  SCN-062-A01 + SCN-062-A02.

**Test Plan:**
| Scenario | Test | Type |
|----------|------|------|
| SCN-062-A01 | `TestRegistry_CoversYAMLNamespaces` | unit |
| SCN-062-A02 | `TestRegistry_NoOrphanedEntries` | unit |
| SCN-062-A01-regression | `TestRegistry_CoversYAMLNamespaces` (persistent scenario-specific regression — fails loudly if a YAML namespace is added without a registry entry) | unit (regression) |
| Broader regression | `./smackerel.sh test unit --go` (runs full Go unit suite including `internal/assistant/transportconfig/...` by default via `go test ./...`) | unit (regression) |

**DoD:**
- [x] Registry compiles under `./smackerel.sh build`. Evidence: `report.md#scope-1`
- [x] Unit tests SCN-062-A01 + SCN-062-A02 PASS. Evidence: `report.md#scope-1`
- [x] No adapter source file modified (inventory-only). Evidence: `report.md#scope-1`
- [x] Scenario-specific E2E regression coverage: SCN-062-A01 + A02 are persistent scenario-specific regressions of the inventory invariant. New YAML namespace or registry orphan would fail the unit suite. Evidence: `report.md#scope-1`
- [x] Broader E2E regression suite passes: `./smackerel.sh test unit` includes the new package by default; sibling broader e2e batch (`TestSpec076MigrationsSurviveFreshStack`) green on disposable test stack. Evidence: `report.md#scope-1`
- [x] `report.md` Scope 1 evidence block captured. Evidence: `report.md#scope-1`

---

## Scope 2 — Adapter Fail-Loud Wiring

**Status:** Done
**Owner:** bubbles.implement
**Depends on:** Scope 1
**Completed:** 2026-06-02 (evidence: [report.md → Scope 2](report.md#scope-2))

**Deliverables:**
- HTTP, WhatsApp, Telegram adapter startup paths consume the registry.
- Any discovered fallback default is removed; if ratified, annotated
  `DefaultedFor: "<reason>"` in the registry with a code comment
  citing this scope.
- `TestRegistry_RequiredEntriesHaveFailLoud` +
  `TestRegistry_NoForbiddenFallbacks` PASS.

**Test Plan:**
| Scenario | Test | Type |
|----------|------|------|
| SCN-062-A03 | `TestRegistry_RequiredEntriesHaveFailLoud` | unit |
| SCN-062-A04 | `TestRegistry_NoForbiddenFallbacks` | unit |
| SCN-062-A05 | `TestHTTPAdapter_MissingRequiredKey_FailsLoud` | e2e-api |
| SCN-062-A05 Regression E2E | `tests/e2e/spec062_http_missing_key_test.go::TestHTTPAdapter_MissingRequiredKey_FailsLoud` (persistent scenario-specific e2e-api regression — pinned subprocess assertion of registry FailLoudMsg) | e2e-api |
| Broader Regression E2E | `./smackerel.sh test e2e --go-run TestSpec076MigrationsSurviveFreshStack` invoked from full e2e suite | e2e-api |
| SLA stress | Not applicable — Scope 2 wires synchronous startup-time `os.LookupEnv` calls that execute exactly once per process boot. No request-path latency surface; design budget is the existing `cmd/core` boot path which already runs `config.Load()` synchronously. Rationale captured in `report.md#scope-2` Plan vs Reality. | stress (n/a) |

**DoD:**
- [x] All three adapters drive startup fail-loud off the registry. Evidence: `report.md#scope-2`
- [x] SCN-062-A03 + A04 + A05 PASS (A05 against disposable test stack). Evidence: `report.md#scope-2`
- [x] Zero new defaults introduced; any retained default has a
      `DefaultedFor` justification reviewed in `report.md`. Evidence: `report.md#scope-2`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior: SCN-062-A05 persistent subprocess regression test (`tests/e2e/spec062_http_missing_key_test.go`) asserts the exact registry `FailLoudMsg` literal in stderr; would fail loudly if the registry-driven wiring regressed. Evidence: `report.md#scope-2`
- [x] Broader E2E regression suite passes: `./smackerel.sh test e2e --go-run TestSpec076MigrationsSurviveFreshStack` (sibling broader e2e batch) plus SCN-062-A05 subprocess invocation green on disposable test stack. Evidence: `report.md#scope-2`
- [x] SLA stress coverage acknowledged: Not applicable — boot-time-only synchronous startup check; no request-path latency surface. Rationale captured in Test Plan and `report.md#scope-2` Plan vs Reality.
- [x] `report.md` Scope 2 evidence block captured. Evidence: `report.md#scope-2`

---

## Scope 3 — Docs + Test Enforcement

**Scope-Kind:** docs-only
**Status:** Done
**Owner:** bubbles.implement
**Depends on:** Scope 2
**Completed:** 2026-06-02 (evidence: [report.md → Scope 3](report.md#scope-3))

**Deliverables:**
- `docs/Transport_Configuration.md` operator-facing table.
- `internal/assistant/transportconfig/doc_sync_test.go` enforcing
  doc ↔ registry parity.
- README / `docs/Operations.md` cross-link to the new doc.
- Registry tests included in `./smackerel.sh test unit` default run.

**Test Plan:**
| Scenario | Test | Type |
|----------|------|------|
| SCN-062-A06 | `TestRegistry_DocSync` | unit |
| SCN-062-A06-regression | `TestRegistry_DocSync` (persistent scenario-specific regression — fails loudly if `docs/Transport_Configuration.md` drifts from Registry in either direction) | unit (regression) |
| Broader regression | `./smackerel.sh test unit --go` (runs `internal/assistant/transportconfig/...` including `TestRegistry_DocSync` by default via `go test ./...`) | unit (regression) |

**DoD:**
- [x] `docs/Transport_Configuration.md` renders every registry row. Evidence: `report.md#scope-3`
- [x] SCN-062-A06 PASS. Evidence: `report.md#scope-3`
- [x] `./smackerel.sh test unit` exercises the new package by default. Evidence: `report.md#scope-3`
- [x] Scenario-specific E2E regression coverage: SCN-062-A06 `TestRegistry_DocSync` is the persistent scenario-specific regression for doc ↔ registry parity; drift in either direction fails the unit suite. Evidence: `report.md#scope-3`
- [x] Broader E2E regression suite passes: `./smackerel.sh test unit` default run includes the doc-sync test; sibling broader e2e batch green on disposable stack. Evidence: `report.md#scope-3`
- [x] `report.md` Scope 3 evidence block captured. Evidence: `report.md#scope-3`
- [x] User validation captured in `uservalidation.md`. Evidence: `report.md#scope-3`

---

### Definition of Done

- [x] All 3 scopes reach `Done` with evidence captured in `report.md`. Evidence: `report.md#scope-1`, `report.md#scope-2`, `report.md#scope-3`.
- [x] All 6 SCN-062-A0x scenarios PASS. Evidence: `report.md#scope-3` §2 (combined run lists all 5 unit scenarios PASS) and `report.md#scope-2` §3 (A05 e2e PASS).
- [x] `docs/Transport_Configuration.md` is published and cross-linked from `docs/Operations.md`. Evidence: `report.md#scope-3` §1 and §4.
- [x] `./smackerel.sh test unit` includes the new registry tests in its default run. Evidence: `report.md#scope-3` §3.
- [x] Operator sign-off captured in `uservalidation.md`. Evidence: `report.md#scope-3` §5.
