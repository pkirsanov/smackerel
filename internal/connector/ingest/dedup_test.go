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
	a := ComputeDedupKey("owner", "https://example.com/page", "browser_history_visit", "laptop", 100)
	for i := 0; i < 1000; i++ {
		b := ComputeDedupKey("owner", "https://example.com/page", "browser_history_visit", "laptop", 100)
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
	laptop := ComputeDedupKey("owner", "https://example.com/page", "browser_history_visit", "laptop", 100)
	desktop := ComputeDedupKey("owner", "https://example.com/page", "browser_history_visit", "work-desktop", 100)
	if bytes.Equal(laptop, desktop) {
		t.Fatal("dedup key MUST vary by device id; Chrome Sync would collapse otherwise")
	}
}

// TestComputeDedupKey_VariesByOwner is the BUG-058-DEDUP-KEY-OWNER-ISOLATION
// core regression guard: two DIFFERENT authenticated owners that emit an
// identical (url, content_type, source_device_id, bucket) tuple MUST produce
// DIFFERENT dedup keys. Because dedup_key is the raw_ingest_dedup PRIMARY KEY,
// equal keys would collapse the two owners onto one row — the second owner's
// publish is skipped and they receive the FIRST owner's artifact_id
// (cross-tenant suppression + id disclosure). The sibling same-owner assertion
// pins that legitimate same-owner determinism (and thus same-owner dedup) is
// preserved.
func TestComputeDedupKey_VariesByOwner(t *testing.T) {
	alice := ComputeDedupKey("u-alice", "https://example.com/page", "bookmark", "laptop", 0)
	bob := ComputeDedupKey("u-bob", "https://example.com/page", "bookmark", "laptop", 0)
	if bytes.Equal(alice, bob) {
		t.Fatal("cross-tenant collapse: dedup key MUST vary by owner_user_id; two owners sharing (url, content_type, device, bucket) collide otherwise")
	}
	// Same owner + identical inputs MUST stay deterministic so legitimate
	// same-owner dedup still resolves to one row.
	a1 := ComputeDedupKey("u-alice", "https://example.com/page", "bookmark", "laptop", 0)
	a2 := ComputeDedupKey("u-alice", "https://example.com/page", "bookmark", "laptop", 0)
	if !bytes.Equal(a1, a2) {
		t.Fatal("same owner + identical inputs MUST be deterministic")
	}
}

func TestComputeDedupKey_VariesByBucket(t *testing.T) {
	a := ComputeDedupKey("owner", "https://example.com/page", "browser_history_visit", "laptop", 100)
	b := ComputeDedupKey("owner", "https://example.com/page", "browser_history_visit", "laptop", 101)
	if bytes.Equal(a, b) {
		t.Fatal("dedup key MUST vary by bucket")
	}
}

func TestComputeDedupKey_VariesByURL(t *testing.T) {
	a := ComputeDedupKey("owner", "https://example.com/a", "bookmark", "laptop", 0)
	b := ComputeDedupKey("owner", "https://example.com/b", "bookmark", "laptop", 0)
	if bytes.Equal(a, b) {
		t.Fatal("dedup key MUST vary by URL")
	}
}

func TestComputeDedupKey_VariesByContentType(t *testing.T) {
	a := ComputeDedupKey("owner", "https://example.com/page", "bookmark", "laptop", 0)
	b := ComputeDedupKey("owner", "https://example.com/page", "browser_history_visit", "laptop", 0)
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
	a := ComputeDedupKey("owner", "a", "bc", "device", 0)
	b := ComputeDedupKey("owner", "ab", "c", "device", 0)
	if bytes.Equal(a, b) {
		t.Fatal("boundary collision: separator hygiene missing between url and content_type")
	}
	// Pair 2: content_type / device splits.
	a = ComputeDedupKey("owner", "url", "type", "dev", 0)
	b = ComputeDedupKey("owner", "url", "typedev", "", 0)
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

// TestComputeDedupKey_SeparatorInjectionResistance is the Chaos Sweep
// Round 18 (2026-06-06) adversarial extension of the boundary-collision
// test. It attempts to FORGE a key collision by smuggling the \x00
// separator byte across field boundaries — the classic injection
// attack on delimiter-joined keys. With an explicit per-field separator
// these MUST NOT collide, even when the attacker controls url + device
// (content_type is allowlist-pinned by the ingest handler, but the
// keyer itself must stay injection-resistant regardless).
func TestComputeDedupKey_SeparatorInjectionResistance(t *testing.T) {
	// Move the separator across the url/device boundary under a fixed
	// content_type + bucket: ("a\x00", dev="b") vs ("a", dev="\x00b").
	k1 := ComputeDedupKey("owner", "a\x00", "bookmark", "b", 0)
	k2 := ComputeDedupKey("owner", "a", "bookmark", "\x00b", 0)
	if bytes.Equal(k1, k2) {
		t.Fatal("separator injection across the url/device boundary forged a collision")
	}
	// Attempt to absorb the content_type anchor into the url with an
	// injected separator: (url="x", ct="bookmark") vs
	// (url="x\x00bookmark", ct=""). Not reachable through the ingest
	// allowlist, but the keyer MUST still distinguish them.
	k3 := ComputeDedupKey("owner", "x", "bookmark", "dev", 0)
	k4 := ComputeDedupKey("owner", "x\x00bookmark", "", "dev", 0)
	if bytes.Equal(k3, k4) {
		t.Fatal("separator injection absorbing the content_type anchor forged a collision")
	}
}

// memDedupStore is a minimal in-memory DedupStore for unit-level
// cross-tenant isolation tests. It keys SOLELY on the (owner-inclusive)
// row.Key bytes — exactly as the production PostgresDedupStore keys on
// the dedup_key PRIMARY KEY — so it faithfully exercises the
// owner-namespacing guarantee without a live Postgres.
type memDedupStore struct {
	byKey map[string]string // string(Key) -> artifact_id
}

func newMemDedupStore() *memDedupStore {
	return &memDedupStore{byKey: make(map[string]string)}
}

func (m *memDedupStore) ResolveOrPublish(ctx context.Context, row DedupRow, publish PublishFunc) (string, bool, error) {
	if publish == nil {
		return "", false, errors.New("ingest: publish callback required")
	}
	if len(row.Key) == 0 {
		return "", false, errors.New("ingest: dedup row missing Key")
	}
	if id, ok := m.byKey[string(row.Key)]; ok {
		return id, true, nil
	}
	id, err := publish(ctx)
	if err != nil {
		return "", false, err
	}
	m.byKey[string(row.Key)] = id
	return id, false, nil
}

// publishConst returns a PublishFunc that yields a fixed artifact id.
func publishConst(id string) PublishFunc {
	return func(context.Context) (string, error) { return id, nil }
}

// TestDedupStore_CrossOwnerIsolation is the BUG-058-DEDUP-KEY-OWNER-ISOLATION
// store-level adversarial proof. Two authenticated owners emit an IDENTICAL
// (url, content_type, source_device_id, bucket) tuple; the store MUST give
// each its OWN artifact (no collapse), while a same-owner repeat MUST still
// dedup. If the owner_user_id is ever dropped from the key preimage, owner B
// collides onto owner A's row, receives owner A's artifact_id, and this test
// fails at the cross-tenant-collapse assertion.
func TestDedupStore_CrossOwnerIsolation(t *testing.T) {
	const (
		url    = "https://github.com"
		ct     = "bookmark"
		device = "laptop" // a natural operator-set device id both owners pick
		bucket = int64(0)
	)
	store := newMemDedupStore()
	ctx := context.Background()

	// Owner A: first write — fresh publish, gets art-A.
	keyA := ComputeDedupKey("u-alice", url, ct, device, bucket)
	gotA, dupA, err := store.ResolveOrPublish(ctx, DedupRow{Key: keyA, OwnerUserID: "u-alice"}, publishConst("art-A"))
	if err != nil {
		t.Fatalf("owner A publish: unexpected error: %v", err)
	}
	if dupA {
		t.Fatal("owner A first write must NOT be a dedup hit")
	}
	if gotA != "art-A" {
		t.Fatalf("owner A: got artifact %q, want art-A", gotA)
	}

	// Owner B: SAME (url, content_type, device, bucket) but a DIFFERENT owner.
	// MUST be a fresh publish that yields owner B's OWN artifact id.
	keyB := ComputeDedupKey("u-bob", url, ct, device, bucket)
	if bytes.Equal(keyA, keyB) {
		t.Fatal("cross-tenant collapse: owner A and owner B produced the SAME dedup key")
	}
	gotB, dupB, err := store.ResolveOrPublish(ctx, DedupRow{Key: keyB, OwnerUserID: "u-bob"}, publishConst("art-B"))
	if err != nil {
		t.Fatalf("owner B publish: unexpected error: %v", err)
	}
	if dupB {
		t.Fatal("cross-tenant collapse: owner B was deduped onto owner A's row (publish skipped)")
	}
	if gotB != "art-B" {
		t.Fatalf("owner B must receive its OWN artifact id; got %q (art-A would be a cross-tenant id leak)", gotB)
	}

	// Owner A again, identical tuple — legitimate same-owner dedup hit that
	// MUST resolve back to the original art-A (NOT a second publish).
	gotA2, dupA2, err := store.ResolveOrPublish(ctx, DedupRow{Key: ComputeDedupKey("u-alice", url, ct, device, bucket), OwnerUserID: "u-alice"}, publishConst("art-A-SECOND"))
	if err != nil {
		t.Fatalf("owner A repeat: unexpected error: %v", err)
	}
	if !dupA2 {
		t.Fatal("same owner + identical tuple MUST be a dedup hit")
	}
	if gotA2 != "art-A" {
		t.Fatalf("same-owner dedup must resolve to the original art-A; got %q", gotA2)
	}
}
