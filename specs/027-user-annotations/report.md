# Execution Report: 027 — User Annotations & Interaction Tracking

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Spec 027 introduces a universal annotation model for ratings, notes, tags, and interaction tracking on any artifact. All 8 scopes completed.

---

## Scope Evidence

### Scope 1 — DB Migration & Annotation Types
- Migration `016_user_annotations.sql` creates `annotations` table, `telegram_message_artifacts` mapping, and `artifact_annotation_summary` materialized view.

### Scope 2 — Freeform Annotation Parser
- `internal/annotation/parser.go` parses freeform text into structured annotations: star ratings (1-5), hashtag tags, interaction markers (bookmarked/favorited/pinned/archived), and freeform notes.

### Scope 3 — Annotation Store
- `internal/annotation/store.go` provides PostgreSQL CRUD, materialized view refresh, and summary queries.

### Scope 4 — REST API Endpoints
- `POST/GET /api/artifacts/{id}/annotations` and `GET /api/artifacts/{id}/annotations/summary` in `internal/api/annotations.go`.

### Scope 5 — Telegram Message-Artifact Mapping
- Internal API endpoints for recording and resolving Telegram message-to-artifact associations, enabling reply-based annotation.

### Scope 6 — Telegram Reply-Based Annotation
- Telegram bot detects replies to artifact messages and routes annotation text through the freeform parser.

### Scope 7 — NATS Annotation Events
- ANNOTATIONS stream and `annotations.>` subjects added for event notification.

### Scope 8 — Search Integration
- Annotation-filtered search: search results can be filtered by annotation presence, ratings, and tags.
