# Scopes: BUG-001 — ML Sidecar Missing Phase 5 NATS Handlers

## Scope 01: Implement Phase 5 NATS Handlers

**Status:** Not Started
**Priority:** P1

### Definition of Done
- [ ] `learning.classify` handler processes messages and returns difficulty classification
- [ ] `content.analyze` handler processes messages and returns writing angles
- [ ] `monthly.generate` handler processes messages and returns report text
- [ ] `quickref.generate` handler processes messages and returns compiled reference
- [ ] `seasonal.analyze` handler processes messages and returns seasonal insights
- [ ] Each handler has LLM fallback for when provider is unavailable
- [ ] Unit tests cover all 5 handlers (happy path + LLM failure fallback)
- [ ] ML sidecar logs no longer show "Unknown subject" for Phase 5 subjects
- [ ] `./smackerel.sh test unit` passes

DoD items un-checked because the fix has not been verified in this artifact pass (status: in_progress).
