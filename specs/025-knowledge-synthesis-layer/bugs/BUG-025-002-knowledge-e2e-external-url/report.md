# Execution Report: BUG-025-002 Knowledge synthesis E2E external URL extraction failure

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Make knowledge synthesis E2E deterministic - 2026-04-27

### Summary
- Bug packet created by `bubbles.bug` during 039 e2e blocker packetization.
- No production code, test code, parent 025 artifacts, or 039 certification fields were modified by this packetization pass.
- This packet is separate from empty-store stats because the external URL fixture dependency requires a distinct regression plan.

### Evidence Provenance
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** The workflow supplied the failing e2e signature. Source inspection through IDE tools confirmed that `tests/e2e/knowledge_synthesis_test.go` includes a non-owned external URL in the required capture body. Runtime reproduction and red-stage output are assigned to the fix/test owner.

### Bug Reproduction - Before Fix
**Phase:** bug
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted
**Interpretation:** No terminal command was executed in this packetization pass. The owner must capture the current red output from a targeted knowledge synthesis E2E run before changing source or test code.

```text
Observed from workflow context:
Knowledge synthesis e2e fails on external URL extraction.

Source inspection notes:
- tests/e2e/knowledge_synthesis_test.go captures JSON containing url "https://example.com/synthesis-e2e-test".
- The same request includes text content, but the required E2E path can still fail if URL extraction treats the non-owned URL as mandatory.
- Required E2E gates should use deterministic stack-owned data sources.
```

### Test Evidence
No tests were run by `bubbles.bug` for this packet. Required red-stage and green-stage evidence belongs to the implementation and test phases recorded in [scopes.md](scopes.md).

### Change Boundary
Allowed implementation surfaces:
- `tests/e2e/knowledge_synthesis_test.go`
- A stack-owned local fixture path if URL behavior remains part of the scenario
- Capture/extraction production code only if targeted evidence proves product behavior is wrong for text-plus-url capture

Protected surfaces for this bug:
- Empty-store stats query, tracked separately in `BUG-025-001-knowledge-stats-empty-store`
- Recommendation engine feature 039 artifacts and certification fields

## Scope 1 Implementation Evidence - 2026-04-28

### Root Cause
**Phase:** implement
**Command:** `./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
**Exit Code:** 1
**Claim Source:** executed

The required knowledge synthesis E2E fixture sent both `url` and `text`. The capture processor chooses URL extraction first when `url` is present, so the test required successful remote extraction from `https://example.com/synthesis-e2e-test` before deterministic text could be used. Pre-fix output showed the failure at the live capture path:

```text
=== RUN   TestKnowledgeSynthesis_PipelineRoundTrip
	knowledge_synthesis_test.go:38: capture returned 422: {"error":{"code":"EXTRACTION_FAILED","message":"content extraction failed: HTTP 404 fetching https://example.com/synthesis-e2e-test"}}
--- FAIL: TestKnowledgeSynthesis_PipelineRoundTrip (0.30s)
```

### Implementation Change
**Phase:** implement
**Command:** source edit via `apply_patch`
**Exit Code:** 0
**Claim Source:** executed

Changed `tests/e2e/knowledge_synthesis_test.go` only. The required fixture now captures deterministic text-only content with the same context marker, preserves real `/api/capture`, real artifact processing polling, and real `/api/knowledge/stats` assertions, and adds a regression guard that fails if the required fixture reintroduces `url`, `http://`, `https://`, or `example.com/synthesis-e2e-test`.

### Focused Green Evidence
**Phase:** implement
**Command:** `timeout 900 ./smackerel.sh test e2e --go-run TestKnowledgeSynthesis_PipelineRoundTrip`
**Exit Code:** 0
**Claim Source:** executed

```text
go-e2e: applying -run selector: TestKnowledgeSynthesis_PipelineRoundTrip
=== RUN   TestKnowledgeSynthesis_PipelineRoundTrip
	knowledge_synthesis_test.go:115: capture response: 200 {"artifact_id":"01KQ9HAN9VPKVW9HJN2WE9MNPY","title":"Synthesis E2E deterministic article about knowledge management systems, organizational learning, con","artifact_type":"generic","summary":"","conne
	knowledge_synthesis_test.go:171: synthesis stats: completed=0 pending=1 failed=0 total=1
--- PASS: TestKnowledgeSynthesis_PipelineRoundTrip (34.24s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        34.249s
```

### Repository Validation Evidence
**Phase:** implement
**Command:** `timeout 120 ./smackerel.sh check`
**Exit Code:** 0
**Claim Source:** executed

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 0, rejected: 0
scenario-lint: OK
```

**Phase:** implement
**Command:** `timeout 600 ./smackerel.sh format --check`
**Exit Code:** 0
**Claim Source:** executed

```text
42 files already formatted
```

### Broad E2E Evidence
**Phase:** implement
**Command:** `./smackerel.sh test e2e`
**Exit Code:** 124
**Claim Source:** executed

The broad E2E run timed out before a full-suite pass could be proven. Visible shell scenarios through IMAP sync passed, including capture error responses, voice capture, knowledge graph, graph entities, search, Telegram flows, digest flows, web UI/detail/settings pages, connector framework, and IMAP sync. No captured output showed a recurrence of the knowledge synthesis external URL extraction failure, but this remains unresolved because the command exit status was `124`.

## Completion Statement
**Phase:** implement
**Command:** none
**Exit Code:** not-run
**Claim Source:** interpreted

Implementation ownership resolved the knowledge synthesis E2E external URL root cause in the focused regression path. The bug packet is not ready for final validation completion because the broader E2E command exited `124` and the validation-owned fixed marker remains unchecked.
