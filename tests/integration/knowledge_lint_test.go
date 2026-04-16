package integration

// T5-10 / BS-005: Seed orphan concepts → RunLint → verify findings in DB.
// T5-11 / BS-004: Seed contradiction edges → RunLint → verify high-severity finding.
// Lint detects contradictions on live knowledge layer data.
//
// Requires: live Docker Compose stack with PostgreSQL + NATS + knowledge tables.
// Run with: ./smackerel.sh test integration
