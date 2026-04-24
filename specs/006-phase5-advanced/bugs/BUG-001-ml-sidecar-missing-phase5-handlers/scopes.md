# Scopes: BUG-001 — ML Sidecar Missing Phase 5 NATS Handlers

## Scope 01: Implement Phase 5 NATS Handlers

**Status:** Done
**Priority:** P1

### Definition of Done
- [x] `learning.classify` handler processes messages and returns difficulty classification
  **Evidence:** `ml/app/nats_client.py:289` routes `learning.classify` to the handler in `ml/app/intelligence.py`; module docstring at `intelligence.py:4` documents the contract.
- [x] `content.analyze` handler processes messages and returns writing angles
  **Evidence:** `ml/app/nats_client.py:299` routes `content.analyze`; intelligence.py:5 documents “generate writing angles from topic artifacts”.
- [x] `monthly.generate` handler processes messages and returns report text
  **Evidence:** `ml/app/nats_client.py:309` routes `monthly.generate`; intelligence.py:6 documents “produce LLM-enhanced monthly report text”.
- [x] `quickref.generate` handler processes messages and returns compiled reference
  **Evidence:** `ml/app/nats_client.py:319` routes `quickref.generate`; intelligence.py:7 documents “compile quick references from source artifacts”.
- [x] `seasonal.analyze` handler processes messages and returns seasonal insights
  **Evidence:** `ml/app/nats_client.py:329` routes `seasonal.analyze`; intelligence.py:8 documents “detect seasonal patterns with LLM commentary”. Response mapping at nats_client.py:70-74 closes the loop with `learning.classified`, `content.analyzed`, `monthly.generated`, `quickref.generated`, `seasonal.analyzed`.
- [x] Each handler has LLM fallback for when provider is unavailable
  **Evidence:** `ml/tests/test_intelligence_handlers.py` covers fallback paths: `TestContentAnalyze.test_fallback_basic_angle` (line 76), `TestMonthlyGenerate.test_fallback_no_provider` (line 131), `TestQuickrefGenerate.test_fallback_with_sources` (line 151), `TestSeasonalAnalyze.test_fallback_returns_input_patterns` (line 192). `TestLearningClassify` covers heuristic classification used when LLM is unavailable.
- [x] Unit tests cover all 5 handlers (happy path + LLM failure fallback)
  **Evidence:** `ml/tests/test_intelligence_handlers.py` has 5 test classes (TestLearningClassify, TestContentAnalyze, TestMonthlyGenerate, TestQuickrefGenerate, TestSeasonalAnalyze) with 18 test functions covering happy path, fallback, and edge cases. See report.md Test Evidence for the full `./smackerel.sh test unit` run.
- [x] ML sidecar logs no longer show "Unknown subject" for Phase 5 subjects
  **Evidence:** `ml/app/nats_client.py:289-329` contains explicit `elif subject == "learning.classify":` … `elif subject == "seasonal.analyze":` branches before the catch-all “Unknown subject” branch — verified by `grep -nE "learning.classify|content.analyze|monthly.generate|quickref.generate|seasonal.analyze" ml/app/nats_client.py`.
- [x] `./smackerel.sh test unit` passes
  **Evidence:** Captured 2026-04-24 via repo CLI — `330 passed, 2 warnings in 11.48s`. See report.md Test Evidence.
