# User Validation Checklist — Spec 080 Knowledge Graph Public API

## Checklist

- [x] Baseline checklist initialized for spec 080 — Knowledge Graph Public API
- [x] spec.md covers all 8 endpoints (topics list+detail, people list+detail, places list+detail, time, graph edges)
- [x] Outcome Contract present with Intent / Success Signal / Hard Constraints / Failure Condition
- [x] Product Principle Alignment present (P2, P5, P8)
- [x] Cross-link contract `{targetKind, targetId, targetLabel, reason}` declared as the single shape
- [x] 15 Gherkin scenarios authored (SCN-080-01..15) including adversarial cases (auth, scope, cursor, window, kind, limit)
- [x] Release Train declared (`next`) with default-off-on-mvp policy
- [x] design.md drafted with package layout, reason taxonomy, cursor design, SST config block
- [ ] bubbles.plan has decomposed into ~3–4 scopes (scopes.md)
- [ ] bubbles.implement has shipped handlers and registered route + scope
- [ ] live-stack integration tests cover every SCN-080-* scenario
- [ ] spec 073 Scope 5 PWA verified against the live endpoints
- [ ] BUG-073-UPSTREAM-API-GAP closed
