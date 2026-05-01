package confirm
// Spec 038 Scope 6 — SCN-038-016 unit anchor.
//
// TestLowConfidenceRoutingRequiresUserConfirmationBeforeProviderWrite asserts
// the contract enforced by the confirmations workflow:
//
//  1. When classifier confidence is below the configured confirm threshold
//     (drive.classification.confirm_threshold), the Save Service MUST
//     persist a pending drive_confirmation row and refuse to commit.
//  2. The pending row is exactly-once resolvable. A second concurrent
//     resolve call observes ErrAlreadyResolved instead of triggering a
//     duplicate provider write.
//  3. Resolving with Outcome=commit moves the row to Status=committed,
//     records the channel ('web' or 'telegram'), stamps decided_at, and
//     unblocks downstream commit.
//  4. Resolving with Outcome=no_save records Status=no_save so the
//     downstream commit path skips the provider write entirely.
//  5. Adversarial: an expired row MUST refuse resolution (Status=expired,
//     ErrExpired returned) so a stale Telegram reply cannot reanimate a
//     long-abandoned save.
//  6. Adversarial: an unknown outcome MUST surface ErrInvalidChoice so
//     callers cannot accidentally write 'pending' choices that look like
//     they committed.
package confirm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLowConfidenceRoutingRequiresUserConfirmationBeforeProviderWrite(t *testing.T) {
	clock := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	store := NewMemoryStore(2 * time.Hour)
	store.SetClock(func() time.Time { return clock })

	t.Run("creates pending row that pauses save (happy path)", func(t *testing.T) {
		c, err := store.Create(context.Background(), CreateInput{
			Kind:             KindSave,
			SourceArtifactID: "artifact-low-conf-1",
			SaveRequestID:    "save-req-1",
			RuleID:           "rule-receipts",
			Payload: Payload{
				Classification: "receipt",
				Sensitivity:    "financial",
				Confidence:     0.42,
				RenderedPath:   "Receipts/2026",
				Title:          "receipt.png",
				ProviderID:     "google",
			},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if c.Status != StatusPending {
			t.Fatalf("Status = %q, want pending — confirmation MUST not auto-commit", c.Status)
		}
		if c.Payload.Confidence != 0.42 {
			t.Fatalf("Payload.Confidence = %v, want 0.42 (caller-supplied)", c.Payload.Confidence)
		}
		got, err := store.Get(context.Background(), c.ID)
		if err != nil {
			t.Fatalf("Get pending row: %v", err)
		}
		if got.Status != StatusPending {
			t.Fatalf("after re-read Status = %q, want pending", got.Status)
		}
	})

	t.Run("commit outcome moves row to committed and records channel", func(t *testing.T) {
		c, err := store.Create(context.Background(), CreateInput{
			Kind:             KindSave,
			SourceArtifactID: "artifact-commit",
			Payload:          Payload{Confidence: 0.55, RenderedPath: "Receipts/2026"},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		resolved, err := store.Resolve(context.Background(), c.ID, ChannelWeb, Choice{
			Outcome: OutcomeCommit,
		})
		if err != nil {
			t.Fatalf("Resolve commit: %v", err)
		}
		if resolved.Status != StatusCommitted {
			t.Fatalf("Status = %q, want committed", resolved.Status)
		}
		if resolved.Channel != ChannelWeb {
			t.Fatalf("Channel = %q, want web", resolved.Channel)
		}
		if resolved.DecidedAt.IsZero() {
			t.Fatalf("DecidedAt is zero — resolve MUST stamp the wall clock")
		}
	})

	t.Run("no_save outcome records refusal so save service skips provider write", func(t *testing.T) {
		c, err := store.Create(context.Background(), CreateInput{
			Kind:             KindSave,
			SourceArtifactID: "artifact-nosave",
			Payload:          Payload{Confidence: 0.40, RenderedPath: "Receipts/2026"},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		resolved, err := store.Resolve(context.Background(), c.ID, ChannelTelegram, Choice{
			Outcome:      OutcomeNoSave,
			NoSaveReason: "user_rejected_in_telegram",
		})
		if err != nil {
			t.Fatalf("Resolve no_save: %v", err)
		}
		if resolved.Status != StatusNoSave {
			t.Fatalf("Status = %q, want no_save", resolved.Status)
		}
		if resolved.Choice.NoSaveReason != "user_rejected_in_telegram" {
			t.Fatalf("Choice.NoSaveReason = %q, want user_rejected_in_telegram", resolved.Choice.NoSaveReason)
		}
	})

	t.Run("reroute outcome captures the user's new classification", func(t *testing.T) {
		c, err := store.Create(context.Background(), CreateInput{
			Kind:             KindClassification,
			SourceArtifactID: "artifact-reroute",
			Payload:          Payload{Classification: "receipt", Confidence: 0.30},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		resolved, err := store.Resolve(context.Background(), c.ID, ChannelWeb, Choice{
			Outcome:           OutcomeReroute,
			NewClassification: "manual",
		})
		if err != nil {
			t.Fatalf("Resolve reroute: %v", err)
		}
		if resolved.Status != StatusRerouted {
			t.Fatalf("Status = %q, want rerouted", resolved.Status)
		}
		if resolved.Choice.NewClassification != "manual" {
			t.Fatalf("Choice.NewClassification = %q, want manual", resolved.Choice.NewClassification)
		}
	})

	// Adversarial: a second concurrent resolve MUST observe ErrAlreadyResolved
	// and MUST NOT mutate the row. This is the exactly-once contract that
	// blocks duplicate provider writes when both web and Telegram race to
	// answer the same prompt.
	t.Run("concurrent resolves collapse onto a single committed outcome (adversarial)", func(t *testing.T) {
		c, err := store.Create(context.Background(), CreateInput{
			Kind:             KindSave,
			SourceArtifactID: "artifact-race",
			Payload:          Payload{Confidence: 0.49, RenderedPath: "Receipts/2026"},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		var (
			wg          sync.WaitGroup
			outcomes    = make([]Status, 8)
			errResolved = make([]error, 8)
		)
		wg.Add(8)
		for i := 0; i < 8; i++ {
			idx := i
			go func() {
				defer wg.Done()
				res, err := store.Resolve(context.Background(), c.ID, ChannelWeb, Choice{Outcome: OutcomeCommit})
				outcomes[idx] = res.Status
				errResolved[idx] = err
			}()
		}
		wg.Wait()
		committedCount := 0
		alreadyResolvedCount := 0
		for i := 0; i < 8; i++ {
			switch {
			case errResolved[i] == nil && outcomes[i] == StatusCommitted:
				committedCount++
			case errors.Is(errResolved[i], ErrAlreadyResolved):
				alreadyResolvedCount++
			default:
				t.Errorf("call %d: status=%q err=%v — want exactly-one committed plus already_resolved", i, outcomes[i], errResolved[i])
			}
		}
		if committedCount != 1 {
			t.Fatalf("committedCount = %d, want exactly 1 (exactly-once resolve)", committedCount)
		}
		if alreadyResolvedCount != 7 {
			t.Fatalf("alreadyResolvedCount = %d, want 7", alreadyResolvedCount)
		}
	})

	// Adversarial: a stale confirmation past expires_at MUST refuse to
	// commit. This blocks an old Telegram reply from re-triggering a save
	// the user already abandoned.
	t.Run("expired confirmation refuses resolve (adversarial)", func(t *testing.T) {
		expClock := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
		expStore := NewMemoryStore(time.Hour)
		expStore.SetClock(func() time.Time { return expClock })
		c, err := expStore.Create(context.Background(), CreateInput{
			Kind:             KindSave,
			SourceArtifactID: "artifact-expired",
			Payload:          Payload{Confidence: 0.30},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		expClock = expClock.Add(2 * time.Hour) // step past expiry
		expStore.SetClock(func() time.Time { return expClock })
		resolved, err := expStore.Resolve(context.Background(), c.ID, ChannelWeb, Choice{Outcome: OutcomeCommit})
		if !errors.Is(err, ErrExpired) {
			t.Fatalf("Resolve(expired) error = %v, want ErrExpired", err)
		}
		if resolved.Status != StatusExpired {
			t.Fatalf("Resolve(expired) Status = %q, want expired", resolved.Status)
		}
	})

	// Adversarial: an unknown outcome MUST surface ErrInvalidChoice so a
	// future regression that introduces a new outcome cannot silently
	// commit instead of failing loud.
	t.Run("unknown outcome rejected (adversarial)", func(t *testing.T) {
		c, err := store.Create(context.Background(), CreateInput{
			Kind:             KindSave,
			SourceArtifactID: "artifact-bad-choice",
			Payload:          Payload{Confidence: 0.49},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		_, err = store.Resolve(context.Background(), c.ID, ChannelWeb, Choice{Outcome: "approve"})
		if !errors.Is(err, ErrInvalidChoice) {
			t.Fatalf("Resolve(bad outcome) err = %v, want ErrInvalidChoice", err)
		}
		got, _ := store.Get(context.Background(), c.ID)
		if got.Status != StatusPending {
			t.Fatalf("after invalid choice Status = %q, want pending (no commit)", got.Status)
		}
	})
}
