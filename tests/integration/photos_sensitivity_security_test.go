//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// Spec 040 hardening — adversarial regressions for MIT-040-S-001 (hash
// reveal-token secrets, constant-time compare on consume) and
// MIT-040-S-007 (close ConsumeRevealToken TOCTOU race). Each test is
// designed to FAIL if its respective fix is reverted.

// mintSensitivePhotoForReveal sets up a sensitive photo and returns its
// record. Cleanup of the artifact and any reveal tokens is registered
// automatically. Shared by every reveal-token security test below.
func mintSensitivePhotoForReveal(t *testing.T, ctx context.Context, store *photolib.Store, label string) *photolib.PhotoRecord {
	t.Helper()
	uniq := testID(t) + "-" + label
	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = "web:reveal-sec:" + uniq
	event.ContentHash = "sha256:reveal-sec:" + uniq
	event.Filename = uniq + ".jpg"
	event.SourceChannel = photolib.SourceChannelWeb
	event.SourceRef = "session:" + uniq
	event.Sensitivity = photolib.ProviderSensitivity{
		Level:  photolib.SensitivitySensitive,
		Source: "test",
		Labels: []string{"financial"},
	}
	record, err := store.PublishPhotoEvent(ctx, "test-reveal-sec", "web", event)
	if err != nil {
		t.Fatalf("publish sensitive photo for reveal-token security test: %v", err)
	}
	cleanupPhoto(t, record.ArtifactID)
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pool := testPool(t)
		if _, err := pool.Exec(cctx, `DELETE FROM photo_reveal_tokens WHERE photo_id=$1`, record.ID); err != nil {
			t.Logf("cleanup reveal tokens for photo %s: %v", record.ID, err)
		}
	})
	return record
}

// splitTokenBlob extracts the raw secret half from the wire format
// `<uuid>.<secret>` so security tests can inspect or tamper with it.
func splitTokenBlob(t *testing.T, blob string) (idPart, secret string) {
	t.Helper()
	parts := strings.SplitN(blob, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("malformed reveal-token blob (expected `<uuid>.<secret>`): %q", blob)
	}
	if parts[0] == "" || parts[1] == "" {
		t.Fatalf("reveal-token blob has empty halves: %q", blob)
	}
	return parts[0], parts[1]
}

// TestMintRevealToken_S001_StoresHashNotPlaintext is the regression for
// MIT-040-S-001: MintRevealToken MUST persist sha256(secret) in
// `photo_reveal_tokens.secret_hash` and MUST NOT persist the plaintext
// secret. The test fails if the hash column is missing, if the column
// holds the raw secret bytes, or if the digest algorithm changes.
func TestMintRevealToken_S001_StoresHashNotPlaintext(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	record := mintSensitivePhotoForReveal(t, ctx, store, "s001-stores-hash")

	now := time.Now().UTC()
	token, err := store.MintRevealToken(ctx, photolib.MintRevealTokenInput{
		PhotoID: record.ID,
		ActorID: "alice",
		TTL:     30 * time.Second,
	}, now)
	if err != nil {
		t.Fatalf("mint reveal token: %v", err)
	}

	_, rawSecret := splitTokenBlob(t, token.Plaintext)

	var stored []byte
	if err := pool.QueryRow(ctx, `SELECT secret_hash FROM photo_reveal_tokens WHERE id=$1`, token.ID).Scan(&stored); err != nil {
		t.Fatalf("load secret_hash for token %s: %v", token.ID, err)
	}

	if len(stored) == 0 {
		t.Fatalf("MIT-040-S-001 regression: secret_hash column is empty — MintRevealToken did not persist a digest")
	}
	if bytes.Equal(stored, []byte(rawSecret)) {
		t.Fatalf("MIT-040-S-001 regression: secret_hash equals plaintext secret bytes (no hashing applied)")
	}
	expected := sha256.Sum256([]byte(rawSecret))
	if !bytes.Equal(stored, expected[:]) {
		t.Fatalf("MIT-040-S-001 regression: secret_hash != SHA-256(secret); got %x expected %x", stored, expected[:])
	}
}

// TestConsumeRevealToken_S001_RejectsWrongSecret covers the consume-side
// half of MIT-040-S-001: presenting a token blob whose UUID half is
// valid but whose secret half is tampered MUST fail with the generic
// not-found error. A "wrong secret" leak (distinct error code) regresses
// the fix because it tells an attacker that the token id exists.
func TestConsumeRevealToken_S001_RejectsWrongSecret(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	record := mintSensitivePhotoForReveal(t, ctx, store, "s001-rejects-wrong")

	now := time.Now().UTC()
	token, err := store.MintRevealToken(ctx, photolib.MintRevealTokenInput{
		PhotoID: record.ID,
		ActorID: "alice",
		TTL:     30 * time.Second,
	}, now)
	if err != nil {
		t.Fatalf("mint reveal token: %v", err)
	}

	idPart, rawSecret := splitTokenBlob(t, token.Plaintext)
	// Flip the last character so the secret stays the same length but
	// no longer hashes to the stored digest. Cycle through hex chars to
	// guarantee a real change regardless of the original last byte.
	tampered := flipLastHexChar(rawSecret)
	if tampered == rawSecret {
		t.Fatalf("test bug: tampered secret equals original (%q)", rawSecret)
	}
	tamperedBlob := idPart + "." + tampered

	_, err = store.ConsumeRevealToken(ctx, record.ID, "alice", tamperedBlob, time.Now().UTC())
	if err == nil {
		t.Fatalf("MIT-040-S-001 regression: ConsumeRevealToken accepted a tampered secret")
	}
	if !errors.Is(err, photolib.ErrRevealTokenNotFound) {
		t.Fatalf("MIT-040-S-001 regression: tampered-secret error must collapse to ErrRevealTokenNotFound (no leak that the secret was wrong); got: %v", err)
	}

	// Belt-and-suspenders: the original token MUST still be consumable
	// after the failed tampered attempt (the failed compare MUST NOT
	// burn the row).
	consumed, err := store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, time.Now().UTC())
	if err != nil {
		t.Fatalf("MIT-040-S-001 regression: original token unconsumable after failed tampered attempt: %v", err)
	}
	if consumed.ConsumedAt == nil {
		t.Fatalf("MIT-040-S-001 regression: consume succeeded but consumed_at not set")
	}
}

// TestConsumeRevealToken_S001_AcceptsCorrectSecret is the positive
// control for MIT-040-S-001: presenting the exact returned blob MUST
// succeed. Without this, a regression that always rejects (hash compare
// inverted, wrong digest algorithm, etc.) would slip past the
// rejects-wrong-secret test.
func TestConsumeRevealToken_S001_AcceptsCorrectSecret(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	record := mintSensitivePhotoForReveal(t, ctx, store, "s001-accepts-right")

	now := time.Now().UTC()
	token, err := store.MintRevealToken(ctx, photolib.MintRevealTokenInput{
		PhotoID: record.ID,
		ActorID: "alice",
		TTL:     30 * time.Second,
	}, now)
	if err != nil {
		t.Fatalf("mint reveal token: %v", err)
	}

	consumed, err := store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, time.Now().UTC())
	if err != nil {
		t.Fatalf("MIT-040-S-001 regression: ConsumeRevealToken rejected the exact returned blob: %v", err)
	}
	if consumed.ConsumedAt == nil {
		t.Fatalf("MIT-040-S-001 regression: consume succeeded but consumed_at not set")
	}
	if consumed.ID != token.ID {
		t.Fatalf("MIT-040-S-001 regression: consume returned id %s, expected %s", consumed.ID, token.ID)
	}
}

// TestConsumeRevealToken_S007_PreventsConcurrentDoubleConsume is the
// critical adversarial regression for MIT-040-S-007. N=10 goroutines
// race to consume the same token after a single shared start signal.
// The single-use guarantee documented in sensitivity.go REQUIRES that
// exactly one goroutine wins; the other N-1 MUST fail with the generic
// not-found / consumed error. The test FAILS if both safeguards
// (FOR UPDATE on the SELECT plus `WHERE consumed_at IS NULL` on the
// UPDATE) are removed — the original buggy state — because multiple
// goroutines pass the consumed_at != nil Go check and multiple UPDATEs
// land. The `start` channel barrier is deterministic (no timing
// assumptions) so the test is not flaky.
func TestConsumeRevealToken_S007_PreventsConcurrentDoubleConsume(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	record := mintSensitivePhotoForReveal(t, ctx, store, "s007-race")

	now := time.Now().UTC()
	token, err := store.MintRevealToken(ctx, photolib.MintRevealTokenInput{
		PhotoID: record.ID,
		ActorID: "alice",
		TTL:     5 * time.Minute, // generous TTL so the race itself is the only failure mode
	}, now)
	if err != nil {
		t.Fatalf("mint reveal token: %v", err)
	}

	const n = 10
	start := make(chan struct{})
	results := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // deterministic launch barrier
			_, err := store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, time.Now().UTC())
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	failureSamples := make([]string, 0, n)
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		failures++
		// Acceptable failure modes: ErrRevealTokenConsumed (FOR UPDATE
		// serialized us behind the winner) or ErrRevealTokenNotFound
		// (we won FOR UPDATE but lost the conditional UPDATE — the
		// belt-and-suspenders predicate). Anything else is wrong.
		if !errors.Is(err, photolib.ErrRevealTokenConsumed) && !errors.Is(err, photolib.ErrRevealTokenNotFound) {
			t.Errorf("MIT-040-S-007: unexpected race-loss error: %v", err)
		}
		failureSamples = append(failureSamples, err.Error())
	}

	if successes != 1 {
		t.Fatalf("MIT-040-S-007 race regression: expected exactly 1 successful consume out of %d concurrent attempts, got %d successes / %d failures (samples: %v)", n, successes, failures, failureSamples)
	}
	if failures != n-1 {
		t.Fatalf("MIT-040-S-007 race regression: expected %d failed consumes, got %d", n-1, failures)
	}

	// Belt-and-suspenders: a follow-up consume attempt MUST also fail.
	if _, err := store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, time.Now().UTC()); err == nil {
		t.Fatalf("MIT-040-S-007 regression: post-race consume on already-consumed token unexpectedly succeeded")
	}
}

// TestConsumeRevealToken_S007_RejectsAlreadyConsumed covers the
// sequential single-use guarantee for MIT-040-S-007: a second consume
// of an already-consumed token MUST fail. This is the non-concurrent
// floor that anchors the concurrency test above.
func TestConsumeRevealToken_S007_RejectsAlreadyConsumed(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	record := mintSensitivePhotoForReveal(t, ctx, store, "s007-already-consumed")

	now := time.Now().UTC()
	token, err := store.MintRevealToken(ctx, photolib.MintRevealTokenInput{
		PhotoID: record.ID,
		ActorID: "alice",
		TTL:     30 * time.Second,
	}, now)
	if err != nil {
		t.Fatalf("mint reveal token: %v", err)
	}

	if _, err := store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, time.Now().UTC()); err != nil {
		t.Fatalf("first consume failed: %v", err)
	}

	_, err = store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, time.Now().UTC())
	if err == nil {
		t.Fatalf("MIT-040-S-007 regression: second consume on already-consumed token unexpectedly succeeded")
	}
	// Either generic not-found or explicit consumed is acceptable; both
	// preserve the single-use invariant. Anything else regresses the
	// fix (e.g., a leaked ErrRevealTokenInvalidPayload would imply the
	// row was deleted unexpectedly).
	if !errors.Is(err, photolib.ErrRevealTokenConsumed) && !errors.Is(err, photolib.ErrRevealTokenNotFound) {
		t.Fatalf("MIT-040-S-007 regression: second-consume error must be Consumed or NotFound, got: %v", err)
	}
}

// flipLastHexChar swaps the final hex character of a hex-encoded string
// for a guaranteed-different one. Used by S-001 tampering test.
func flipLastHexChar(s string) string {
	if s == "" {
		return s
	}
	last := s[len(s)-1]
	var swap byte
	switch {
	case last >= '0' && last <= '9':
		// rotate digits forward by one (9 wraps to 0)
		swap = (last-'0'+1)%10 + '0'
	case last >= 'a' && last <= 'f':
		// rotate hex letters forward (f wraps to a)
		swap = (last-'a'+1)%6 + 'a'
	case last >= 'A' && last <= 'F':
		swap = (last-'A'+1)%6 + 'A'
	default:
		// non-hex tail; mutate to '0' which is guaranteed different
		swap = '0'
		if last == '0' {
			swap = '1'
		}
	}
	return s[:len(s)-1] + string(swap)
}
