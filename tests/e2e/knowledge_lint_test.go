package e2e

// T5-12 / BS-005: Lint job runs via scheduler → report visible at GET /api/knowledge/lint.
// Lint detects contradictions and orphan concepts on live data.
//
// Requires: live Docker Compose stack with seeded knowledge layer data.
// Run with: ./smackerel.sh test e2e
