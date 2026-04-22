# Design: BUG-001 — ML Sidecar Missing Phase 5 NATS Handlers

## Fix Design

Add 5 new handler branches to `ml/app/nats_client.py::_consume_loop` and corresponding handler implementations:

### 1. `learning.classify` Handler
- **Input:** `{ artifact_id, title, summary, content_snippet }`
- **Output:** `{ artifact_id, difficulty: "beginner"|"intermediate"|"advanced", key_takeaway: string }`
- **LLM Prompt:** Classify educational resource difficulty and extract key takeaway
- **Fallback:** Return heuristic classification if LLM unavailable

### 2. `content.analyze` Handler
- **Input:** `{ topic_id, topic_name, capture_count, source_diversity, supporting_ids }`
- **Output:** `{ topic_id, angles: [{ title, uniqueness_rationale, format_suggestion }] }`
- **LLM Prompt:** Generate writing angles from topic depth and diversity
- **Fallback:** Return empty angles if LLM unavailable

### 3. `monthly.generate` Handler
- **Input:** Full `MonthlyReport` JSON with all assembled data sections
- **Output:** `{ report_text: string, word_count: int }`
- **LLM Prompt:** Synthesize monthly self-knowledge report from structured data
- **Fallback:** Return empty (Go falls back to `assembleMonthlyReportText`)

### 4. `quickref.generate` Handler
- **Input:** `{ concept, source_artifacts: [{ id, title, summary }] }`
- **Output:** `{ concept, content: string, source_artifact_ids: [] }`
- **LLM Prompt:** Compile concise quick reference from source material
- **Fallback:** Return concatenated summaries

### 5. `seasonal.analyze` Handler
- **Input:** `{ month, patterns: [{ month, volume_this_year, volume_last_year, top_topics }] }`
- **Output:** `{ patterns: [{ pattern, month, observation, actionable }] }`
- **LLM Prompt:** Generate human-readable seasonal insights
- **Fallback:** Return structured observations without LLM enhancement

### Implementation Approach

Create a new module `ml/app/intelligence.py` with the 5 handler functions, following the same pattern as `synthesis.py::handle_extract`. Each handler:
1. Receives data dict + LLM provider/model/key params
2. Builds a prompt from the structured input
3. Calls the LLM via the existing provider abstraction
4. Parses the response into structured output
5. Returns result dict with `success: true/false`

Wire each handler into `_consume_loop` with new `elif subject ==` branches.

### Files Changed
- `ml/app/nats_client.py` — add 5 elif branches
- `ml/app/intelligence.py` — new module with 5 handler functions
- `ml/tests/test_intelligence_handlers.py` — unit tests for each handler
