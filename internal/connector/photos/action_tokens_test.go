package photos

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestPhotoActionTokenRejectsScopeDriftAndExpiry covers SCN-040-009: the
// confirm-time guard MUST reject any drift in the action scope, expired
// tokens, double-spend, missing text confirmation for delete, and actor
// mismatch. Adversarial cases prove the validator does not happily
// rubber-stamp tokens just because the request body decodes.
func TestPhotoActionTokenRejectsScopeDriftAndExpiry(t *testing.T) {
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	scope := ActionScope{
		PhotoIDs: []string{"photo-a", "photo-b"},
	}
	mintInput := MintPhotoActionTokenInput{
		ActorID:       "actor-1",
		Action:        ActionArchive,
		Scope:         scope,
		BytesEstimate: 5_000_000,
		TTL:           5 * time.Minute,
	}
	token, err := MintActionTokenInMemory(mintInput, base)
	if err != nil {
		t.Fatalf("mint baseline token: %v", err)
	}
	if token.ScopeHash == "" || token.PhotoCount != 2 {
		t.Fatalf("mint did not normalize scope: %+v", token)
	}
	if token.ExpiresAt.IsZero() || !token.ExpiresAt.After(token.CreatedAt) {
		t.Fatalf("expires_at not set after created_at: %+v", token)
	}

	t.Run("happy_path_matching_scope_succeeds", func(t *testing.T) {
		if err := ConfirmActionTokenInMemory(*token, ConfirmPhotoActionTokenInput{
			TokenID: token.ID,
			ActorID: "actor-1",
			Scope:   scope,
		}, base.Add(time.Minute)); err != nil {
			t.Fatalf("confirm should succeed for matching scope, got %v", err)
		}
	})

	t.Run("scope_drift_extra_photo_rejected", func(t *testing.T) {
		drifted := ActionScope{PhotoIDs: []string{"photo-a", "photo-b", "photo-extra"}}
		err := ConfirmActionTokenInMemory(*token, ConfirmPhotoActionTokenInput{
			TokenID: token.ID,
			ActorID: "actor-1",
			Scope:   drifted,
		}, base.Add(time.Minute))
		if !errors.Is(err, ErrActionTokenScopeDrift) {
			t.Fatalf("expected ErrActionTokenScopeDrift, got %v", err)
		}
	})

	t.Run("scope_drift_swap_photo_rejected", func(t *testing.T) {
		swapped := ActionScope{PhotoIDs: []string{"photo-a", "photo-c"}}
		err := ConfirmActionTokenInMemory(*token, ConfirmPhotoActionTokenInput{
			TokenID: token.ID,
			ActorID: "actor-1",
			Scope:   swapped,
		}, base.Add(time.Minute))
		if !errors.Is(err, ErrActionTokenScopeDrift) {
			t.Fatalf("expected ErrActionTokenScopeDrift on swap, got %v", err)
		}
	})

	t.Run("scope_normalisation_does_not_falsely_match", func(t *testing.T) {
		// Same set of IDs, just reordered + duplicates: this should be
		// accepted because the canonical hash is order-independent.
		reordered := ActionScope{PhotoIDs: []string{"photo-b", "photo-a", "photo-a"}}
		if err := ConfirmActionTokenInMemory(*token, ConfirmPhotoActionTokenInput{
			TokenID: token.ID,
			ActorID: "actor-1",
			Scope:   reordered,
		}, base.Add(time.Minute)); err != nil {
			t.Fatalf("reordered identical scope should match, got %v", err)
		}
	})

	t.Run("expired_token_rejected", func(t *testing.T) {
		err := ConfirmActionTokenInMemory(*token, ConfirmPhotoActionTokenInput{
			TokenID: token.ID,
			ActorID: "actor-1",
			Scope:   scope,
		}, token.ExpiresAt.Add(time.Second))
		if !errors.Is(err, ErrActionTokenExpired) {
			t.Fatalf("expected ErrActionTokenExpired, got %v", err)
		}
	})

	t.Run("already_consumed_rejected", func(t *testing.T) {
		consumed := *token
		consumedAt := base.Add(2 * time.Minute)
		consumed.ConsumedAt = &consumedAt
		err := ConfirmActionTokenInMemory(consumed, ConfirmPhotoActionTokenInput{
			TokenID: token.ID,
			ActorID: "actor-1",
			Scope:   scope,
		}, base.Add(3*time.Minute))
		if !errors.Is(err, ErrActionTokenAlreadyConsumed) {
			t.Fatalf("expected ErrActionTokenAlreadyConsumed, got %v", err)
		}
	})

	t.Run("actor_mismatch_rejected", func(t *testing.T) {
		err := ConfirmActionTokenInMemory(*token, ConfirmPhotoActionTokenInput{
			TokenID: token.ID,
			ActorID: "intruder",
			Scope:   scope,
		}, base.Add(time.Minute))
		if !errors.Is(err, ErrActionTokenActorMismatch) {
			t.Fatalf("expected ErrActionTokenActorMismatch, got %v", err)
		}
	})

	t.Run("delete_requires_text_confirmation", func(t *testing.T) {
		deleteInput := MintPhotoActionTokenInput{
			ActorID: "actor-1",
			Action:  ActionDelete,
			Scope:   scope,
			TTL:     2 * time.Minute,
		}
		deleteToken, err := MintActionTokenInMemory(deleteInput, base)
		if err != nil {
			t.Fatalf("mint delete token: %v", err)
		}
		if !deleteToken.RequiresText {
			t.Fatal("delete tokens must mark requires_text=true")
		}
		// Adversarial: empty text confirmation is rejected even though
		// scope/actor/expiry are otherwise valid — proves the gate is
		// evaluated rather than silently skipped.
		if err := ConfirmActionTokenInMemory(*deleteToken, ConfirmPhotoActionTokenInput{
			TokenID: deleteToken.ID,
			ActorID: "actor-1",
			Scope:   scope,
		}, base.Add(time.Minute)); !errors.Is(err, ErrActionTokenTextMissing) {
			t.Fatalf("expected ErrActionTokenTextMissing, got %v", err)
		}
		if err := ConfirmActionTokenInMemory(*deleteToken, ConfirmPhotoActionTokenInput{
			TokenID:          deleteToken.ID,
			ActorID:          "actor-1",
			Scope:            scope,
			TextConfirmation: strings.ToUpper(string(ActionDelete)),
		}, base.Add(time.Minute)); err != nil {
			t.Fatalf("delete with correct text confirmation should succeed, got %v", err)
		}
	})

	t.Run("invalid_action_rejected_at_mint", func(t *testing.T) {
		_, err := MintActionTokenInMemory(MintPhotoActionTokenInput{
			ActorID: "actor-1",
			Action:  ActionKind("nuke_provider"),
			Scope:   scope,
			TTL:     time.Minute,
		}, base)
		if !errors.Is(err, ErrActionTokenInvalidAction) {
			t.Fatalf("expected ErrActionTokenInvalidAction, got %v", err)
		}
	})

	t.Run("empty_scope_rejected_at_mint", func(t *testing.T) {
		_, err := MintActionTokenInMemory(MintPhotoActionTokenInput{
			ActorID: "actor-1",
			Action:  ActionArchive,
			Scope:   ActionScope{},
			TTL:     time.Minute,
		}, base)
		if !errors.Is(err, ErrActionTokenScopeEmpty) {
			t.Fatalf("expected ErrActionTokenScopeEmpty, got %v", err)
		}
	})
}
