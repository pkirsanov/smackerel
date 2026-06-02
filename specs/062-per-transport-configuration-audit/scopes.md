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

**Status:** not_started
**Owner:** bubbles.implement (after analyst handoff)

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

**DoD:**
- [ ] Registry compiles under `./smackerel.sh build`.
- [ ] Unit tests SCN-062-A01 + SCN-062-A02 PASS.
- [ ] No adapter source file modified (inventory-only).
- [ ] `report.md` Scope 1 evidence block captured.

---

## Scope 2 — Adapter Fail-Loud Wiring

**Status:** not_started
**Owner:** bubbles.implement
**Depends on:** Scope 1

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

**DoD:**
- [ ] All three adapters drive startup fail-loud off the registry.
- [ ] SCN-062-A03 + A04 + A05 PASS (A05 against disposable test stack).
- [ ] Zero new defaults introduced; any retained default has a
      `DefaultedFor` justification reviewed in `report.md`.
- [ ] `report.md` Scope 2 evidence block captured.

---

## Scope 3 — Docs + Test Enforcement

**Status:** not_started
**Owner:** bubbles.implement
**Depends on:** Scope 2

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

**DoD:**
- [ ] `docs/Transport_Configuration.md` renders every registry row.
- [ ] SCN-062-A06 PASS.
- [ ] `./smackerel.sh test unit` exercises the new package by default.
- [ ] `report.md` Scope 3 evidence block captured.
- [ ] User validation captured in `uservalidation.md`.

---

### Definition of Done

- [ ] All 3 scopes reach `done` with evidence captured in `report.md`.
- [ ] All 6 SCN-062-A0x scenarios PASS.
- [ ] `docs/Transport_Configuration.md` is published and cross-linked from `docs/Operations.md`.
- [ ] `./smackerel.sh test unit` includes the new registry tests in its default run.
- [ ] Operator sign-off captured in `uservalidation.md`.
