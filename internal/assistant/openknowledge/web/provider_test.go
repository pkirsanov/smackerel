package web

import (
	"context"
	"errors"
	"testing"
)

func TestWebProvider_InterfaceCompliance(t *testing.T) {
	// Compile-time interface assertions. If any impl drifts out of
	// compliance the test file fails to build.
	var _ WebSearchProvider = (*SearxNG)(nil)
	var _ WebSearchProvider = (*Brave)(nil)
	var _ WebSearchProvider = (*Tavily)(nil)
}

func TestWebProvider_StubsReturnNotConfigured(t *testing.T) {
	cases := []struct {
		name string
		p    WebSearchProvider
	}{
		{"brave", NewBrave()},
		{"tavily", NewTavily()},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Name(); got != tc.name {
				t.Fatalf("Name(): got %q want %q", got, tc.name)
			}
			snips, err := tc.p.Search(context.Background(), "anything", 5)
			if snips != nil {
				t.Fatalf("expected nil snippets, got %d", len(snips))
			}
			if !errors.Is(err, ErrProviderNotConfigured) {
				t.Fatalf("expected ErrProviderNotConfigured, got %v", err)
			}
		})
	}
}

func TestBrave_NeverDialsNetwork(t *testing.T) {
	// Calling with a cancelled context still returns
	// ErrProviderNotConfigured — proves no network round-trip.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewBrave().Search(ctx, "q", 1)
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("want ErrProviderNotConfigured, got %v", err)
	}
}

func TestTavily_NeverDialsNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewTavily().Search(ctx, "q", 1)
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("want ErrProviderNotConfigured, got %v", err)
	}
}

func TestCanonicalContentHash_Deterministic(t *testing.T) {
	h1 := CanonicalContentHash("https://a", "T", "S")
	h2 := CanonicalContentHash("https://a", "T", "S")
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("hash hex length: got %d want 64", len(h1))
	}
	// Adversarial: changing any field changes the hash.
	if CanonicalContentHash("https://b", "T", "S") == h1 {
		t.Fatal("URL change must alter hash")
	}
	if CanonicalContentHash("https://a", "T2", "S") == h1 {
		t.Fatal("Title change must alter hash")
	}
	if CanonicalContentHash("https://a", "T", "S2") == h1 {
		t.Fatal("Snippet change must alter hash")
	}
}
