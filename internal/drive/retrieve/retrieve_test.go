// Spec 038 Scope 7 — SCN-038-019 unit anchor.
//
// TestRetrievePolicyAllowedFileReturnsBytesOrProviderLinkWithCandidates
// proves the contract from scopes.md Test Plan: a non-sensitive
// retrieval returns bytes when within the inline cap, downgrades to a
// provider_link when the file exceeds the cap, surfaces all candidates
// with title/folder/provider/sensitivity labels for disambiguation, and
// refuses with the localized hint when zero results match.
package retrieve

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/drive/policy"
)

// fakeSearcher implements Searcher. The unit tests load it with a
// scripted candidate list and assert exact behavior on the resulting
// RetrieveDelivery.
type fakeSearcher struct {
	candidates []RetrieveCandidate
	err        error
	calls      int
}

func (f *fakeSearcher) SearchDrive(_ context.Context, _ RetrieveRequest) ([]RetrieveCandidate, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.candidates, nil
}

// fakeFetcher implements BytesFetcher. It records calls so tests can
// assert that bytes were (or were NOT) requested.
type fakeFetcher struct {
	bytes []byte
	mime  string
	err   error
	calls int
}

func (f *fakeFetcher) GetArtifactBytes(_ context.Context, _ string) ([]byte, string, error) {
	f.calls++
	if f.err != nil {
		return nil, "", f.err
	}
	return f.bytes, f.mime, nil
}

func newTestService(t *testing.T, searcher Searcher, fetcher BytesFetcher, maxInline int64) *Service {
	t.Helper()
	return NewService(searcher, fetcher, policy.NewEngine(), maxInline, DefaultReasonTable())
}

func TestRetrievePolicyAllowedFileReturnsBytesOrProviderLinkWithCandidates(t *testing.T) {
	ctx := context.Background()

	t.Run("non_sensitive_within_inline_cap_returns_bytes_with_candidate", func(t *testing.T) {
		searcher := &fakeSearcher{candidates: []RetrieveCandidate{{
			ArtifactID:  "drive:google:conn:boarding-pass",
			Title:       "Lisbon boarding pass.pdf",
			Folder:      "Travel/Portugal",
			Sensitivity: "none",
			SizeBytes:   12_000,
			Provider:    "google",
			ProviderURL: "https://drive.example/lisbon",
		}}}
		fetcher := &fakeFetcher{bytes: []byte("PDF-PAYLOAD"), mime: "application/pdf"}
		svc := newTestService(t, searcher, fetcher, 5_000_000)

		delivery, err := svc.Retrieve(ctx, RetrieveRequest{Channel: ChannelTelegram, Query: "lisbon boarding pass"})
		if err != nil {
			t.Fatalf("Retrieve: %v", err)
		}
		if delivery.Mode != ModeBytes {
			t.Fatalf("delivery.Mode = %q, want %q", delivery.Mode, ModeBytes)
		}
		if string(delivery.Bytes) != "PDF-PAYLOAD" {
			t.Fatalf("delivery.Bytes = %q, want PDF-PAYLOAD", string(delivery.Bytes))
		}
		if delivery.MimeType != "application/pdf" {
			t.Fatalf("delivery.MimeType = %q, want application/pdf", delivery.MimeType)
		}
		if delivery.PolicyReason != "allowed" {
			t.Fatalf("delivery.PolicyReason = %q, want allowed", delivery.PolicyReason)
		}
		if len(delivery.Candidates) != 1 {
			t.Fatalf("delivery.Candidates len = %d, want 1", len(delivery.Candidates))
		}
		got := delivery.Candidates[0]
		if got.Title != "Lisbon boarding pass.pdf" || got.Folder != "Travel/Portugal" ||
			got.Provider != "google" || got.Sensitivity != "none" {
			t.Fatalf("candidate metadata wrong: %+v", got)
		}
		if fetcher.calls != 1 {
			t.Fatalf("fetcher.calls = %d, want 1 (bytes path must call provider)", fetcher.calls)
		}
	})

	t.Run("non_sensitive_oversized_downgrades_to_provider_link_no_bytes_fetch", func(t *testing.T) {
		searcher := &fakeSearcher{candidates: []RetrieveCandidate{{
			ArtifactID:  "drive:google:conn:large-pdf",
			Title:       "Large guidebook.pdf",
			Folder:      "Travel",
			Sensitivity: "none",
			SizeBytes:   200_000_000, // 200 MB, far above the 5 MB cap
			Provider:    "google",
			ProviderURL: "https://drive.example/large",
		}}}
		fetcher := &fakeFetcher{} // must not be called
		svc := newTestService(t, searcher, fetcher, 5_000_000)

		delivery, err := svc.Retrieve(ctx, RetrieveRequest{Channel: ChannelTelegram, Query: "guidebook"})
		if err != nil {
			t.Fatalf("Retrieve: %v", err)
		}
		if delivery.Mode != ModeProviderLink {
			t.Fatalf("delivery.Mode = %q, want %q (oversize must downgrade)", delivery.Mode, ModeProviderLink)
		}
		if delivery.URL != "https://drive.example/large" {
			t.Fatalf("delivery.URL = %q, want provider URL", delivery.URL)
		}
		if delivery.PolicyReason != "size_exceeds_inline_limit" {
			t.Fatalf("delivery.PolicyReason = %q, want size_exceeds_inline_limit", delivery.PolicyReason)
		}
		if len(delivery.Bytes) != 0 {
			t.Fatalf("delivery.Bytes len = %d, want 0 (provider_link must NOT include bytes)", len(delivery.Bytes))
		}
		if !strings.Contains(delivery.Hint, "too large") {
			t.Fatalf("delivery.Hint = %q, want oversize hint", delivery.Hint)
		}
		if fetcher.calls != 0 {
			t.Fatalf("fetcher.calls = %d, want 0 (oversize must not fetch bytes)", fetcher.calls)
		}
	})

	t.Run("multiple_candidates_returns_disambiguation_with_full_labels", func(t *testing.T) {
		searcher := &fakeSearcher{candidates: []RetrieveCandidate{
			{
				ArtifactID:  "drive:google:conn:boarding-pass-may",
				Title:       "Lisbon boarding pass May.pdf",
				Folder:      "Travel/Portugal",
				Sensitivity: "none",
				SizeBytes:   8_000,
				Provider:    "google",
				ProviderURL: "https://drive.example/may",
			},
			{
				ArtifactID:  "drive:google:conn:boarding-pass-jun",
				Title:       "Lisbon boarding pass June.pdf",
				Folder:      "Travel/Portugal",
				Sensitivity: "none",
				SizeBytes:   8_000,
				Provider:    "google",
				ProviderURL: "https://drive.example/june",
			},
		}}
		fetcher := &fakeFetcher{} // must not be called for disambiguation
		svc := newTestService(t, searcher, fetcher, 5_000_000)

		delivery, err := svc.Retrieve(ctx, RetrieveRequest{Channel: ChannelTelegram, Query: "lisbon boarding pass"})
		if err != nil {
			t.Fatalf("Retrieve: %v", err)
		}
		if delivery.Mode != ModeDisambiguate {
			t.Fatalf("delivery.Mode = %q, want %q", delivery.Mode, ModeDisambiguate)
		}
		if len(delivery.Candidates) != 2 {
			t.Fatalf("delivery.Candidates len = %d, want 2", len(delivery.Candidates))
		}
		if !strings.Contains(delivery.Hint, "Multiple drive files matched") {
			t.Fatalf("delivery.Hint = %q, want disambiguation hint", delivery.Hint)
		}
		for _, c := range delivery.Candidates {
			if c.Title == "" || c.Folder == "" || c.Provider == "" || c.Sensitivity == "" {
				t.Fatalf("candidate missing label: %+v", c)
			}
		}
		if fetcher.calls != 0 {
			t.Fatalf("fetcher.calls = %d, want 0 (disambiguation must not fetch bytes)", fetcher.calls)
		}
	})

	t.Run("zero_candidates_refuses_with_localized_hint", func(t *testing.T) {
		searcher := &fakeSearcher{}
		fetcher := &fakeFetcher{}
		svc := newTestService(t, searcher, fetcher, 5_000_000)

		delivery, err := svc.Retrieve(ctx, RetrieveRequest{Channel: ChannelTelegram, Query: "nonexistent"})
		if err != nil {
			t.Fatalf("Retrieve: %v", err)
		}
		if delivery.Mode != ModeRefused {
			t.Fatalf("delivery.Mode = %q, want %q", delivery.Mode, ModeRefused)
		}
		if delivery.PolicyReason != "no_match" {
			t.Fatalf("delivery.PolicyReason = %q, want no_match", delivery.PolicyReason)
		}
		if !strings.Contains(delivery.Hint, "No drive files matched") {
			t.Fatalf("delivery.Hint = %q, want no-match hint", delivery.Hint)
		}
		if fetcher.calls != 0 {
			t.Fatalf("fetcher.calls = %d, want 0", fetcher.calls)
		}
	})

	t.Run("disambiguation_pick_routes_through_policy_again", func(t *testing.T) {
		// Adversarial: a malicious channel adapter could try to bypass
		// policy by sending SelectedArtifactID directly. The service MUST
		// re-apply the policy check against the picked candidate.
		searcher := &fakeSearcher{candidates: []RetrieveCandidate{{
			ArtifactID:  "drive:google:conn:passport-scan",
			Title:       "Passport scan.pdf",
			Folder:      "Identity",
			Sensitivity: "identity",
			SizeBytes:   90_000,
			Provider:    "google",
			ProviderURL: "https://drive.example/passport",
		}}}
		fetcher := &fakeFetcher{bytes: []byte("DO NOT LEAK")}
		svc := newTestService(t, searcher, fetcher, 5_000_000)

		delivery, err := svc.Retrieve(ctx, RetrieveRequest{
			Channel:            ChannelTelegram,
			Query:              "passport",
			SelectedArtifactID: "drive:google:conn:passport-scan",
		})
		if err != nil {
			t.Fatalf("Retrieve: %v", err)
		}
		if delivery.Mode != ModeSecureLink {
			t.Fatalf("delivery.Mode = %q, want %q (sensitive picks must downgrade)", delivery.Mode, ModeSecureLink)
		}
		if fetcher.calls != 0 {
			t.Fatalf("fetcher.calls = %d, want 0 (sensitive picks must NOT fetch bytes)", fetcher.calls)
		}
	})

	t.Run("search_error_propagates", func(t *testing.T) {
		searcher := &fakeSearcher{err: errors.New("db down")}
		fetcher := &fakeFetcher{}
		svc := newTestService(t, searcher, fetcher, 5_000_000)
		if _, err := svc.Retrieve(ctx, RetrieveRequest{Channel: ChannelTelegram, Query: "anything"}); err == nil {
			t.Fatalf("Retrieve: want error when search fails")
		}
	})

	t.Run("unknown_channel_rejected", func(t *testing.T) {
		searcher := &fakeSearcher{}
		fetcher := &fakeFetcher{}
		svc := newTestService(t, searcher, fetcher, 5_000_000)
		if _, err := svc.Retrieve(ctx, RetrieveRequest{Channel: Channel("web"), Query: "anything"}); err == nil {
			t.Fatalf("Retrieve: want error for unknown channel")
		}
	})

	t.Run("empty_query_rejected", func(t *testing.T) {
		searcher := &fakeSearcher{}
		fetcher := &fakeFetcher{}
		svc := newTestService(t, searcher, fetcher, 5_000_000)
		if _, err := svc.Retrieve(ctx, RetrieveRequest{Channel: ChannelTelegram}); err == nil {
			t.Fatalf("Retrieve: want error for empty query")
		}
	})
}
