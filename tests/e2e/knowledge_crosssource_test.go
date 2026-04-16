package e2e

// T4-07 / BS-003: Multi-source ingest → cross-source connection visible in entity profile API.
//
// This E2E regression test verifies:
// 1. Capture two artifacts from different source types referencing the same concept
// 2. Synthesis pipeline processes both → concept page has 2+ source types
// 3. Cross-source connection detected and assessed
// 4. CROSS_SOURCE_CONNECTION edge visible via GET /api/knowledge/concepts/{id}
//
// Requires full live stack.
// Run with: ./smackerel.sh test e2e
