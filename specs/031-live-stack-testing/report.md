# Execution Report: 031 — Live Stack Testing

Links: [spec.md](spec.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Spec 031 establishes live-stack integration and E2E testing infrastructure: Docker test stack setup, database migration tests, NATS stream tests, artifact CRUD + vector search tests, and full capture-to-search E2E tests. All 6 scopes completed.

---

## Scope Evidence

### Scope 1 — Test Stack Infrastructure
- Test environment uses separate Compose project with isolated volumes, ports in the 45000-47999 range.

### Scope 2 — Database Migration Tests
- `tests/integration/db_migration_test.go` validates all 17 migrations run cleanly against a fresh PostgreSQL instance.

### Scope 3 — NATS Stream Tests
- `tests/integration/nats_stream_test.go` validates all 11 NATS streams are created with correct subjects and retention.

### Scope 4 — Artifact CRUD & Vector Search
- `tests/integration/artifact_crud_test.go` tests create, read, update, and vector similarity search against the live stack.

### Scope 5 — Capture-to-Search E2E
- `tests/e2e/capture_process_search_test.go` validates the full pipeline: capture → process → embed → search with live services.

### Scope 6 — Knowledge Layer E2E
- `tests/e2e/knowledge_*_test.go` suite (8 files) validates concept store, entity profiles, cross-source connections, lint auditing, and synthesis against the live stack.
