# Cross-Spec Packet — spec 054 (Notification Intelligence Handler)

**Source:** spec 061 (Conversational Assistant), SCOPE-08 — Notifications skill (v1 #3)
**Target owner:** spec 054 (Notification Intelligence Handler) owner
**Routed by:** bubbles.implement during spec 061 SCOPE-08 substrate landing
**Date routed:** 2026-05-28
**Change shape:** Additive, backward compatible. Zero-valued new fields preserve current behavior exactly.

---

## 1. Why this packet exists

spec 061 SCOPE-08 (Notifications skill) reuses spec 054's scheduler instead of building a parallel one. Design.md §5.3 + scopes.md SCOPE-08 Implementation Plan step 5 explicitly require:

> spec 054 scheduler `Job.Source` + `Job.Originator` additive fields | SCOPE-08 (`notification_execute` tool calls `scheduler.Schedule(Job{Source, Originator})`) | Additive-field packet (zero-valued backward compatible) + spec 054 test updates

Without these two fields the spec 054 scheduler cannot attribute a scheduled job back to (a) the capability that requested it ("assistant.skill.notifications" vs other producers) and (b) the originating user/transport for audit + revocation purposes. Both attributions are required for design.md §8.2 structured-log fields and §5.3 confirm-card audit (`assistant_proposal` artifact lineage).

## 2. Requested change

Add two additive fields to the spec 054 scheduler `Job` type (whichever struct is the public scheduler request payload — likely under `internal/scheduler/`):

```go
type Job struct {
    // ... existing fields unchanged ...

    // Source identifies the capability that registered this job.
    // Zero value ("") preserves the current behavior — jobs registered by
    // legacy producers continue to work without changes.
    //
    // Convention: dot-namespaced capability identifier, e.g.
    //   "assistant.skill.notifications"
    //   "scheduler.delivery"
    // The scheduler MUST NOT interpret Source semantically; it is opaque
    // metadata recorded alongside the job for downstream lineage.
    Source string

    // Originator identifies the upstream actor responsible for the job.
    // Zero value (Originator{}) preserves current behavior.
    //
    // For assistant-originated jobs the convention is:
    //   Originator{Transport: "telegram", ConfirmRef: "<ULID>"}
    // where ConfirmRef is the assistant_proposal confirm reference so
    // every scheduled reminder is joinable back to its audit row.
    Originator Originator
}

type Originator struct {
    // Transport is the canonical assistant transport identifier
    // ("telegram", "web", "cli", …). Empty for non-assistant jobs.
    Transport string

    // ConfirmRef is the opaque ULID issued by notification_propose
    // (spec 061 SCOPE-08) and recorded in the
    // `assistant_confirm_pending` + `assistant_proposal_payload`
    // rows. Empty for non-assistant jobs.
    ConfirmRef string
}
```

## 3. Persistence contract

The spec 054 scheduler's job-storage rows MUST add two nullable columns to record these fields, e.g.

```sql
ALTER TABLE scheduler_jobs ADD COLUMN source     TEXT;
ALTER TABLE scheduler_jobs ADD COLUMN originator JSONB;
```

Both NULL for legacy rows. Index neither column unless lineage queries justify it; the assistant uses `Originator.ConfirmRef` for join queries but stores its own copy in `assistant_proposal_payload` so a missing scheduler index is acceptable.

## 4. Backward-compat guarantees the spec 061 implementer relies on

- Zero-valued `Source=""` and `Originator=Originator{}` MUST be accepted and persisted as NULL.
- Existing spec 054 callers MUST continue to compile unchanged (the new fields default to their zero values).
- The scheduler's job-dispatch loop MUST NOT change behavior based on these fields. They are metadata-only.
- The scheduler's existing observability (metrics, traces) MAY emit the new fields as labels/attributes when present.

## 5. Tests the spec 054 owner is asked to add

- Unit test: `Job{Source: "assistant.skill.notifications", Originator: Originator{Transport: "telegram", ConfirmRef: "01H…"}}` round-trips through `Schedule` → storage → dispatch with fields preserved.
- Unit test: legacy `Job{}` (zero `Source`/`Originator`) round-trips and persists NULL.
- Integration test: SELECT after `Schedule` proves the `source`/`originator` columns reflect the call values.

## 6. Acceptance criteria for this packet (closed when ALL met)

- [ ] spec 054 owner reviews and approves the field additions.
- [ ] spec 054 ships the type changes + migration + tests.
- [ ] spec 061 SCOPE-08's `notification_execute` tool can replace its temporary `notificationSchedulerStub` (in `cmd/core/wiring_assistant_skills.go`) with the real spec 054 scheduler binding.
- [ ] spec 061 SCOPE-08 BS-004 e2e (full reminder dispatch) becomes runnable.

## 7. Current state of the spec 061 side (so spec 054 owner knows what is waiting)

- `internal/agent/tools/notification/services.go` already declares the `Scheduler` interface the bound implementation must satisfy:
  ```go
  type Scheduler interface {
      Schedule(ctx context.Context, when time.Time, payload string, source string, originator string) (jobID string, err error)
  }
  ```
  `source` is the proposed `Job.Source`; `originator` is the proposed `Job.Originator.ConfirmRef` (and the binding shim adds the `Transport` from the originating message).
- `cmd/core/wiring_assistant_skills.go` wires a `notificationSchedulerStub` that returns `errNotificationSchedulerUnbound` so any production attempt to schedule a reminder fails loud and the trace surfaces this exact packet.
- `internal/agent/tools/notification/pg_confirm_store.go` + migration `043_assistant_confirm_pending.sql` are already shipped (verified by `TestMachinePg*` integration tests against real PG — see report.md §SCOPE-08 evidence).
- `internal/assistant/confirm/` package (Machine + PgWriter) ships the `assistant_proposal` audit row regardless of outcome (verified by 9 unit tests + 2 PG integration tests).

## 8. What spec 061 will NOT change while this packet is open

- The temporary `notificationSchedulerStub` stays in place. Notifications cannot be activated end-to-end (BS-004 e2e remains `[ ]`) until this packet lands.
- The assistant skill manifest (config/prompt_contracts/notification-schedule-v1.yaml) continues to advertise the notification scenario; runtime behavior is gated by `assistant.skills.notifications.enabled` (default false in SST until the binding lands).

## 9. Open questions for the spec 054 owner

1. Are the field names `Source` and `Originator` acceptable, or does spec 054 prefer different names (e.g., `Producer`, `Caller`)? The spec 061 side will follow whatever spec 054 ratifies.
2. Should `Originator` be a struct or a JSON-encoded string? The struct form is preferred for type safety; the string form is acceptable if spec 054's existing API prefers it. Either is backward compatible because the field is new.
3. Does spec 054 want a parallel `assistant.skill.notifications`-shaped metric label, or is the `source` column sufficient for downstream observability?

---

**Routing status:** packet authored 2026-05-28 by `bubbles.implement` during spec 061 SCOPE-08 substrate landing. spec 054 owner ownership transfer pending. No spec 054 artifacts modified by this packet — it is a routed request, not an applied change.
