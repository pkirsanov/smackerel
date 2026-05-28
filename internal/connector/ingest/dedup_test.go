// Spec 058 Scope 2 — unit tests for the dedup keyer and the no-op
// store. Postgres-backed behavior is covered by the integration tests
// alongside the rest of the ingest live-stack suite.
package ingest

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestComputeDedupKey_Deterministic(t *testing.T) {
	a := ComputeDedupKey("https://example.com/page", "browser_history_visit", "laptop", 100)
	for i := 0; i < 1000; i++ {
		b := ComputeDedupKey("https://example.com/page", "browser_history_visit", "laptop", 100)
		if !bytes.Equal(a, b) {
			t.Fatalf("non-deterministic at iter %d", i)
		}
	}
	if len(a) != 32 {
		t.Fatalf("expected SHA-256 (32 bytes), got %d", len(a))
	}
}

// TestComputeDedupKey_VariesByDevice is the spec 058 SCN-058-008
// canary: two devices observing the same URL in the same time bucket
// MUST produce distinct dedup keys (Chrome Sync case).
func TestComputeDedupKey_VariesByDevice(t *testing.T) {
	laptop := ComputeDedupKey("https://example.com/page", "browser_history_visit", "laptop", 100)
	desktop := ComputeDedupKey("https://example.com/page", "browser_history_visit", "work-desktop", 100)
	if bytes.Equal(laptop, desktop) {
		t.Fatal("dedup key MUST vary by device id; Chrome Sync would collapse otherwise")
	}
}

func TestComputeDedupKey_VariesByBucket(t *testing.T) {
	a := ComputeDedupKey("https://example.com/page", "browser_history_visit", "laptop", 100)
	b := ComputeDedupKey("https://example.com/page", "browser_history_visit", "laptop", 101)
	if bytes.Equal(a, b) {
		t.Fatal("dedup key MUST vary by bucket")
	}
}

func TestComputeDedupKey_VariesByURL(t *testing.T) {
	a := ComputeDedupKey("https://example.com/a", "bookmark", "laptop", 0)
	b := ComputeDedupKey("https://example.com/b", "bookmark", "laptop", 0)
	if bytes.Equal(a, b) {
		t.Fatal("dedup key MUST vary by URL")
	}
}

func TestComputeDedupKey_VariesByContentType(t *testing.T) {
	a := ComputeDedupKey("https://example.com/page", "bookmark", "laptop", 0)
	b := ComputeDedupKey("https://example.com/page", "browser_history_visit", "laptop", 0)
	if bytes.Equal(a, b) {
		t.Fatal("dedup key MUST vary by content type")
	}
}

// TestComputeDedupKey_BoundaryCollisionResistance pins the spec 058
// scopes.md adversarial regression: inputs designed to collide if the
// separator were absent ("a"+"bc" vs "ab"+"c") MUST produce different
// keys.
func TestComputeDedupKey_BoundaryCollisionResistance(t *testing.T) {
	// Pair 1: url splits.
	a := ComputeDedupKey("a", "bc", "device", 0)
	b := ComputeDedupKey("ab", "c", "device", 0)
	if bytes.Equal(a, b) {
		t.Fatal("boundary collision: separator hygiene missing between url and content_type")
	}
	// Pair 2: content_type / device splits.
	a = ComputeDedupKey("url", "type", "dev", 0)
	b = ComputeDedupKey("url", "typedev", "", 0)
	if bytes.Equal(a, b) {
		t.Fatal("boundary collision: separator hygiene missing between content_type and device")
	}
}

func TestPassthroughDedupStore_AlwaysPublishes(t *testing.T) {
	store := PassthroughDedupStore{}
	row := DedupRow{Key: []byte("k")}
	calls := 0
	got, deduped, err := store.ResolveOrPublish(context.Background(), row, func(ctx context.Context) (string, error) {
		calls++
		return "art-123", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected publish called once, got %d", calls)
	}
	if got != "art-123" || deduped {
		t.Fatalf("expected (art-123, false), got (%q, %v)", got, deduped)
	}
}

func TestPassthroughDedupStore_PropagatesPublishError(t *testing.T) {
	store := PassthroughDedupStore{}
	sentinel := errors.New("publish failed")
	_, _, err := store.ResolveOrPublish(context.Background(), DedupRow{Key: []byte("k")}, func(ctx context.Context) (string, error) {
		return "", sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestPassthroughDedupStore_RejectsNilPublish(t *testing.T) {
	_, _, err := PassthroughDedupStore{}.ResolveOrPublish(context.Background(), DedupRow{Key: []byte("k")}, nil)
	if err == nil {
		t.Fatal("expected error when publish callback is nil")
	}
}
