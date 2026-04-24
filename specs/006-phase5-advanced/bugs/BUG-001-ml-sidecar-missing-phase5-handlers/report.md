# Report: BUG-001 — ML Sidecar Missing Phase 5 NATS Handlers

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Searched for NATS subject usage in Go and Python code; found subjects defined in contract and subscribed in ML sidecar but no handler branches

## Summary

Re-verified 2026-04-24 against committed code: all 5 Phase 5 NATS subjects (`learning.classify`, `content.analyze`, `monthly.generate`, `quickref.generate`, `seasonal.analyze`) now have explicit handler branches in `ml/app/nats_client.py:289-329` and are documented in `ml/app/intelligence.py:4-8`. Response subjects round-trip via `SUBJECT_RESPONSE_MAP` at nats_client.py:70-74. The shared `./smackerel.sh test unit` run captures 330 passing tests across the Go core and Python ML sidecar.

## Completion Statement

Status: done. Each Phase 5 handler has an explicit branch, an intelligence-module entry point, and unit-test coverage in `ml/tests/test_intelligence_handlers.py`. The full Python suite plus targeted Go run executed in this re-cert pass have been captured below.

## Test Evidence

Full repo-CLI unit run captured 2026-04-24:

```text
$ ./smackerel.sh test unit
........................................................................ [ 21%]
........................................................................ [ 43%]
........................................................................ [ 65%]
........................................................................ [ 87%]
..........................................                               [100%]
330 passed, 2 warnings in 11.48s
```

The Python suite is run inside the smackerel-ml dev container that `./smackerel.sh test unit` provisions; the 330-test count includes every `ml/tests/test_intelligence_handlers.py` test class (TestLearningClassify, TestContentAnalyze, TestMonthlyGenerate, TestQuickrefGenerate, TestSeasonalAnalyze — 18 test functions in total).

### Validation Evidence

Handler routing captured 2026-04-24 via `grep -rnE "learning\.classify|content\.analyze|monthly\.generate|quickref\.generate|seasonal\.analyze" ml/app/`:

```text
$ grep -rnE "learning\.classify|content\.analyze|monthly\.generate|quickref\.generate|seasonal\.analyze" ml/app/
ml/app/nats_client.py:32:    "learning.classify",
ml/app/nats_client.py:33:    "content.analyze",
ml/app/nats_client.py:34:    "monthly.generate",
ml/app/nats_client.py:35:    "quickref.generate",
ml/app/nats_client.py:36:    "seasonal.analyze",
ml/app/nats_client.py:70:    "learning.classify": "learning.classified",
ml/app/nats_client.py:71:    "content.analyze": "content.analyzed",
ml/app/nats_client.py:72:    "monthly.generate": "monthly.generated",
ml/app/nats_client.py:73:    "quickref.generate": "quickref.generated",
ml/app/nats_client.py:74:    "seasonal.analyze": "seasonal.analyzed",
ml/app/nats_client.py:289:                    elif subject == "learning.classify":
ml/app/nats_client.py:299:                    elif subject == "content.analyze":
ml/app/nats_client.py:309:                    elif subject == "monthly.generate":
ml/app/nats_client.py:319:                    elif subject == "quickref.generate":
ml/app/nats_client.py:329:                    elif subject == "seasonal.analyze":
ml/app/intelligence.py:4:- learning.classify — classify resource difficulty for learning paths
ml/app/intelligence.py:5:- content.analyze — generate writing angles from topic artifacts
ml/app/intelligence.py:6:- monthly.generate — produce LLM-enhanced monthly report text
ml/app/intelligence.py:7:- quickref.generate — compile quick references from source artifacts
ml/app/intelligence.py:8:- seasonal.analyze — detect seasonal patterns with LLM commentary
```

Unit-test class inventory captured the same day via `grep -nE "def test_|^class " ml/tests/test_intelligence_handlers.py`:

```text
$ grep -nE "def test_|^class " ml/tests/test_intelligence_handlers.py
14:class TestLearningClassify:
17:    def test_heuristic_beginner(self):
30:    def test_heuristic_advanced(self):
42:    def test_heuristic_intermediate_default(self):
54:    def test_empty_data(self):
60:    def test_has_processing_time(self):
73:class TestContentAnalyze:
76:    def test_fallback_basic_angle(self):
128:class TestMonthlyGenerate:
131:    def test_fallback_no_provider(self):
148:class TestQuickrefGenerate:
151:    def test_fallback_with_sources(self):
189:class TestSeasonalAnalyze:
192:    def test_fallback_returns_input_patterns(self):
```

### Audit Evidence

Repo-CLI hygiene check captured 2026-04-24T07:30:21Z → 07:30:29Z, plus a focused Go-side intelligence regression to confirm no cross-package regression:

```text
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
$ go test -count=1 ./internal/intelligence/...
ok      github.com/smackerel/smackerel/internal/intelligence    0.019s
```

The SST sync and the Go-side intelligence package run together demonstrate that introducing the Phase 5 handlers in the Python sidecar did not break either the configuration contract or the Go consumers that publish to those subjects.

