package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// providerNameSearxNG is the canonical lowercase label stamped into
// WebSnippet.Provider for SearxNG-sourced snippets. The constant
// matches config.OpenKnowledgeProviderSearxng.
const providerNameSearxNG = "searxng"

// SearxNGOption is a functional option for NewSearxNG.
type SearxNGOption func(*SearxNG)

// WithNow overrides the clock used to stamp WebSnippet.FetchedAt.
// Tests pass a deterministic clock; production callers leave the
// default (time.Now).
func WithNow(now func() time.Time) SearxNGOption {
	return func(s *SearxNG) {
		if now != nil {
			s.now = now
		}
	}
}

// WithSuspiciousSnippetRecorder installs an optional metric sink
// invoked by SanitizeSnippet when a suspicious-prompt-injection
// pattern is detected in a returned snippet. Wiring (cmd/core)
// passes the openknowledge/metrics implementation; tests typically
// leave it nil.
func WithSuspiciousSnippetRecorder(r SuspiciousSnippetRecorder) SearxNGOption {
	return func(s *SearxNG) {
		s.recorder = r
	}
}

// SearxNG is the JSON-format SearxNG search adapter. It performs a
// single HTTP GET against {endpoint}/search and maps each result row
// into a WebSnippet. Outbound network policy is owned by the
// http.Client transport supplied by the caller; this type does not
// dial directly and does not consult environment variables.
//
// Egress: SCOPE-15 hardens the http.RoundTripper to enforce the
// operator's egress allowlist. This type intentionally trusts the
// supplied client.
type SearxNG struct {
	endpoint string
	client   *http.Client
	now      func() time.Time
	recorder SuspiciousSnippetRecorder
}

// NewSearxNG constructs a SearxNG adapter. endpoint is the base URL
// (e.g. "https://searxng.example/" or "http://searxng:8080"); the
// "/search" suffix is appended internally. The caller MUST provide a
// non-nil http.Client; pass http.DefaultClient explicitly if no egress
// policy is required.
//
// Returns ErrInvalidConfig when endpoint is empty or unparseable, or
// when client is nil.
func NewSearxNG(endpoint string, client *http.Client, opts ...SearxNGOption) (*SearxNG, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("%w: empty endpoint", ErrInvalidConfig)
	}
	if client == nil {
		return nil, fmt.Errorf("%w: nil http client", ErrInvalidConfig)
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: endpoint scheme must be http or https", ErrInvalidConfig)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: endpoint missing host", ErrInvalidConfig)
	}
	s := &SearxNG{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   client,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Name implements WebSearchProvider.
func (s *SearxNG) Name() string { return providerNameSearxNG }

type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// Search implements WebSearchProvider. See provider.go for the error
// contract. Adversarial: results with empty URL are skipped (the
// cite-back verifier in SCOPE-08 cannot anchor a citation without a
// URL, so propagating an empty-URL snippet would be a fabrication
// vector).
func (s *SearxNG) Search(ctx context.Context, query string, k int) ([]WebSnippet, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("%w: empty query", ErrInvalidQuery)
	}
	if k <= 0 {
		return nil, fmt.Errorf("%w: k must be > 0", ErrInvalidQuery)
	}
	if k > MaxK {
		return nil, fmt.Errorf("%w: k=%d exceeds MaxK=%d", ErrInvalidQuery, k, MaxK)
	}

	target := s.endpoint + "/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	qv := req.URL.Query()
	qv.Set("q", q)
	qv.Set("format", "json")
	qv.Set("count", strconv.Itoa(k))
	req.URL.RawQuery = qv.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnreachable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: http 429", ErrQuotaExceeded)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("%w: http %d", ErrProviderUnreachable, resp.StatusCode)
	}

	var body searxngResponse
	// SearxNG returns many fields beyond `results` (suggestions,
	// infoboxes, etc.); we accept unknown fields silently.
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	out := make([]WebSnippet, 0, len(body.Results))
	fetchedAt := s.now().UTC()
	for _, r := range body.Results {
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		// SCOPE-15 — treat provider-returned snippet as untrusted
		// text. SanitizeSnippet strips control characters, repairs
		// UTF-8, truncates to MaxSnippetRunes, and (when the
		// recorder is non-nil) emits the suspicious-snippet metric
		// on prompt-injection trigger patterns. ContentHash MUST be
		// computed over the sanitised body so the cite-back
		// verifier (SCOPE-08) keys off the same canonical form the
		// LLM will actually see.
		sanitised := SanitizeSnippet(providerNameSearxNG, r.Content, s.recorder)
		out = append(out, WebSnippet{
			URL:         r.URL,
			Title:       r.Title,
			Snippet:     sanitised,
			ContentHash: CanonicalContentHash(r.URL, r.Title, sanitised),
			FetchedAt:   fetchedAt,
			Provider:    providerNameSearxNG,
		})
	}
	return out, nil
}

// CanonicalContentHash returns the deterministic SHA-256 hex digest
// used as WebSnippet.ContentHash. The canonical form is:
//
//	sha256_hex( URL + "\n" + Title + "\n" + Snippet )
//
// This form is the contract the cite-back verifier (SCOPE-08) relies
// on. Do NOT change it without updating SCOPE-08 in lock-step.
func CanonicalContentHash(rawURL, title, snippet string) string {
	h := sha256.New()
	h.Write([]byte(rawURL))
	h.Write([]byte{'\n'})
	h.Write([]byte(title))
	h.Write([]byte{'\n'})
	h.Write([]byte(snippet))
	return hex.EncodeToString(h.Sum(nil))
}
