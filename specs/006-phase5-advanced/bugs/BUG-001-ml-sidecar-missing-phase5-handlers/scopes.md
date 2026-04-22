# Scopes: BUG-001 — ML Sidecar Missing Phase 5 NATS Handlers

## Scope 01: Implement Phase 5 NATS Handlers

**Status:** Not Started
**Priority:** P1

### Definition of Done
- [x] `learning.classify` handler processes messages and returns difficulty classification
- [x] `content.analyze` handler processes messages and returns writing angles
- [x] `monthly.generate` handler processes messages and returns report text
- [x] `quickref.generate` handler processes messages and returns compiled reference
- [x] `seasonal.analyze` handler processes messages and returns seasonal insights
- [x] Each handler has LLM fallback for when provider is unavailable
- [x] Unit tests cover all 5 handlers (happy path + LLM failure fallback)
- [x] ML sidecar logs no longer show "Unknown subject" for Phase 5 subjects
- [x] `./smackerel.sh test unit` passes
