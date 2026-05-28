// Package confirm owns the spec 061 SCOPE-08 confirm-card state
// machine per design §5.4. The machine drives the
// "propose → user confirms → execute" lifecycle for side-effecting
// scenarios (today: notification reminders) and writes one
// `assistant_proposal` audit artifact per terminal outcome
// (`confirmed | discarded_user | discarded_timeout`).
//
// Responsibilities, in one place:
//   - Persist the in-flight ConfirmCard payload in the existing
//     `assistant_conversations.pending_confirm` JSONB column (one
//     pending per (user_id, transport); see
//     `internal/assistant/context` SCOPE-04 substrate).
//   - Drive single-flight Confirm / Discard transitions: a confirm
//     callback that races a timeout MUST execute exactly once. This
//     is achieved by clearing the pending row (Store.Persist with
//     PendingConfirm=nil) BEFORE the audit row is written, so a
//     subsequent callback observes "no pending" and short-circuits.
//   - Write the audit row via Writer.WriteProposalArtifact, which
//     inserts one row into the existing `artifacts` table with
//     `artifact_type='assistant_proposal'` and the per-outcome
//     payload in the additive `assistant_proposal_payload` JSONB
//     column (migration 042).
//
// What this package does NOT own:
//   - The `notification.ConfirmStore` interface backing the tool
//     handlers (that is keyed by ref only and lives separately;
//     see `internal/agent/tools/notification/services.go`). The
//     facade layer reads the proposed-action payload out of the
//     tool's first response and persists it via this Machine —
//     the two storage paths are intentional per design §5.4.
//   - Spec 054 scheduler invocation. `notification_execute` calls
//     scheduler.Schedule with the payload it pulled from the
//     ConfirmStore; this package writes only the audit record.
//
// Architecture: this package is allowed by `internal/assistant/contracts/architecture_test.go`
// (it is NOT in the forbidden list — router/registry/executor/tracer/loader/llm/nats).
package confirm
