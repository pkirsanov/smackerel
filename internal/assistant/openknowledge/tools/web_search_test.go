package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/web"
)

// fakeWebProvider lets each test inject the exact snippets / error
// the provider returns. It also captures the args Search was called
// with so the tool's k/query plumbing can be asserted.
type fakeWebProvider struct {
	name     string
	snippets []web.WebSnippet
	err      error

	gotQuery string
	gotK     int
}

func (f *fakeWebProvider) Search(_ context.Context, query string, k int) ([]web.WebSnippet, error) {
	f.gotQuery = query
	f.gotK = k
	return f.snippets, f.err
}

func (f *fakeWebProvider) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}

func TestWebSearch_NilProviderPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil provider")
		}
	}()
	_ = NewWebSearch(nil)
}

func TestWebSearch_NameAndSchema(t *testing.T) {
	t.Parallel()
	tool := NewWebSearch(&fakeWebProvider{})
	if tool.Name() != "web_search" {
		t.Fatalf("name: got %q want %q", tool.Name(), "web_search")
	}
	if len(tool.ParamsSchema()) == 0 {
		t.Fatal("empty params schema")
	}
}

func TestWebSearch_HappyPath(t *testing.T) {
	t.Parallel()
	fetchedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	provider := &fakeWebProvider{
		name: "searxng",
		snippets: []web.WebSnippet{
			{URL: "https://example.com/a", Title: "A", Snippet: "alpha", ContentHash: "hash-a", FetchedAt: fetchedAt, Provider: "searxng"},
			{URL: "https://example.com/b", Title: "B", Snippet: "beta", ContentHash: "hash-b", FetchedAt: fetchedAt, Provider: "searxng"},
		},
	}
	tool := NewWebSearch(provider)

	res, err := tool.Execute(context.Background(), []byte(`{"query":"alpha","k":2}`))
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	if res.Error != nil {
		t.Fatalf("tool error: %v", res.Error)
	}
	if provider.gotQuery != "alpha" || provider.gotK != 2 {
		t.Fatalf("provider args: got (%q,%d) want (alpha,2)", provider.gotQuery, provider.gotK)
	}
	if len(res.Snippets) != 2 || len(res.Sources) != 2 {
		t.Fatalf("snippets=%d sources=%d want 2/2", len(res.Snippets), len(res.Sources))
	}
	if res.Snippets[0].ContentHash != "hash-a" {
		t.Fatalf("snippet[0].ContentHash got %q want hash-a", res.Snippets[0].ContentHash)
	}
	if res.Sources[0].Kind != ok.SourceWeb || res.Sources[0].Web == nil {
		t.Fatalf("source[0]: kind=%v web=%v", res.Sources[0].Kind, res.Sources[0].Web)
	}
	if res.Sources[0].Web.URL != "https://example.com/a" {
		t.Fatalf("source[0].Web.URL got %q", res.Sources[0].Web.URL)
	}
	if res.Sources[0].Web.ContentHash != "hash-a" {
		t.Fatalf("source[0].Web.ContentHash got %q (preservation invariant)", res.Sources[0].Web.ContentHash)
	}
}

func TestWebSearch_RejectsMalformed(t *testing.T) {
	t.Parallel()
	tool := NewWebSearch(&fakeWebProvider{})

	cases := map[string]string{
		"not-json":     `not json`,
		"missing-k":    `{"query":"x"}`,
		"missing-q":    `{"k":3}`,
		"extra-field":  `{"query":"x","k":1,"extra":true}`,
		"wrong-q-type": `{"query":5,"k":1}`,
	}
	for name, body := range cases {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res, err := tool.Execute(context.Background(), []byte(body))
			if err != nil {
				t.Fatalf("hard error: %v", err)
			}
			if res.Error == nil || res.Error.Code != ErrWebSearchMalformed.Code {
				t.Fatalf("error: got %v want %s", res.Error, ErrWebSearchMalformed.Code)
			}
		})
	}
}

func TestWebSearch_RejectsEmptyQuery(t *testing.T) {
	t.Parallel()
	tool := NewWebSearch(&fakeWebProvider{})
	res, err := tool.Execute(context.Background(), []byte(`{"query":"   ","k":3}`))
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	if res.Error == nil || res.Error.Code != ErrWebSearchEmptyQuery.Code {
		t.Fatalf("got %v want %s", res.Error, ErrWebSearchEmptyQuery.Code)
	}
}

func TestWebSearch_RejectsOutOfRangeK(t *testing.T) {
	t.Parallel()
	tool := NewWebSearch(&fakeWebProvider{})
	cases := []string{`{"query":"x","k":0}`, `{"query":"x","k":11}`}
	for _, body := range cases {
		body := body
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			res, err := tool.Execute(context.Background(), []byte(body))
			if err != nil {
				t.Fatalf("hard error: %v", err)
			}
			// k>10 may be rejected at JSON-schema time as malformed OR
			// at the explicit range check; both are acceptable so long
			// as no upstream provider call is made. Anything else is a
			// bug.
			if res.Error == nil {
				t.Fatalf("expected error for body %s", body)
			}
		})
	}
}

func TestWebSearch_ProviderErrorClassification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"unreachable", web.ErrProviderUnreachable, ErrWebSearchUnreachable.Code},
		{"quota", web.ErrQuotaExceeded, ErrWebSearchQuota.Code},
		{"not-configured", web.ErrProviderNotConfigured, ErrWebSearchNotConfigured.Code},
		{"malformed-resp", web.ErrMalformedResponse, ErrWebSearchMalformedResp.Code},
		{"invalid-query", web.ErrInvalidQuery, ErrWebSearchEmptyQuery.Code},
		{"opaque", errors.New("boom"), ErrWebSearchBackend.Code},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeWebProvider{err: c.in}
			tool := NewWebSearch(provider)
			res, err := tool.Execute(context.Background(), []byte(`{"query":"x","k":1}`))
			if err != nil {
				t.Fatalf("hard error: %v", err)
			}
			if res.Error == nil || res.Error.Code != c.want {
				t.Fatalf("error: got %v want %s", res.Error, c.want)
			}
		})
	}
}

func TestWebSearch_SkipsEmptyURLOrHash(t *testing.T) {
	t.Parallel()
	provider := &fakeWebProvider{
		snippets: []web.WebSnippet{
			{URL: "", Title: "no url", ContentHash: "h"},
			{URL: "https://example.com/no-hash", Title: "no hash", ContentHash: ""},
			{URL: "https://example.com/ok", Title: "ok", ContentHash: "h-ok", Snippet: "ok", Provider: "fake"},
		},
	}
	tool := NewWebSearch(provider)
	res, err := tool.Execute(context.Background(), []byte(`{"query":"x","k":3}`))
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	if res.Error != nil {
		t.Fatalf("tool error: %v", res.Error)
	}
	if len(res.Snippets) != 1 || len(res.Sources) != 1 {
		t.Fatalf("expected 1/1 after defensive skip, got %d/%d", len(res.Snippets), len(res.Sources))
	}
	if res.Sources[0].Web.URL != "https://example.com/ok" {
		t.Fatalf("kept wrong row: %s", res.Sources[0].Web.URL)
	}
}
