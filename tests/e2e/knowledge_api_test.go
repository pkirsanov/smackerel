package e2e

// T3-11..T3-14: Knowledge API endpoint E2E tests.
// Knowledge query returns pre-synthesized answer via GET /api/knowledge/concepts.
// Knowledge API lists concept pages with sort, filter, pagination.
//
// Requires: live Docker Compose stack with seeded knowledge layer data.
// Run with: ./smackerel.sh test e2e
