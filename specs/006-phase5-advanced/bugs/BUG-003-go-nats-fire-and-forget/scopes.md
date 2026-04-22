# Scopes: BUG-003 — Go Runtime Fire-and-Forget NATS Publishes

## Scope 01: Convert Phase 5 NATS to Request/Reply

**Status:** Not Started
**Priority:** P1
**Depends On:** BUG-001 (ML sidecar handlers must exist first)

### Definition of Done
- [ ] `GenerateMonthlyReport` uses request/reply for LLM-generated report text with 30s timeout
- [ ] `GenerateContentFuel` uses request/reply for LLM writing angles with 15s timeout
- [ ] `GetLearningPaths` publishes to `smk.learning.classify` for LLM difficulty with 10s timeout
- [ ] `CreateQuickReference` publishes to `smk.quickref.generate` for LLM compilation with 15s timeout
- [ ] `DetectSeasonalPatterns` publishes to `smk.seasonal.analyze` for LLM commentary with 15s timeout
- [ ] All 5 features gracefully fall back to local generation on NATS timeout/failure
- [ ] Unit tests verify fallback behavior when NATS is nil or unavailable
- [ ] `./smackerel.sh test unit` passes
