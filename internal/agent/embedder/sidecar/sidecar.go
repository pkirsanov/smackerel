// Package sidecar implements agent.Embedder by calling the ML
// sidecar's POST /embed endpoint over HTTP with Bearer auth.
//
// BUG-061-004 — replaces agent.NoopEmbedder in production. The
// NoopEmbedder returns a fixed unit vector for every input, which
// collapses the router's cosine-similarity scoring to a tie at 1.0
// for every scenario and forces an alphabetical tie-break. With a
// real sentence-transformer embedding, distinct inputs produce
// distinct vectors and the router can actually select the closest
// scenario.
package sidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Embedder is an agent.Embedder that delegates to the ML sidecar's
// POST /embed endpoint. Construct via New; the zero value is not
// usable.
type Embedder struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// New constructs a sidecar Embedder. baseURL is the ML sidecar root
// (e.g., "http://smackerel-ml:8081") — no trailing slash required.
// authToken is the Bearer token expected by the sidecar's
// verify_auth dependency. timeout is the per-call HTTP timeout
// applied to each Embed roundtrip.
//
// Returns an error if any argument is invalid; New deliberately does
// NOT probe the sidecar — the router builds at startup before the
// sidecar may be ready, and probing here would couple boot ordering.
// The first real Embed call surfaces sidecar unavailability via the
// returned error, which the router treats as ReasonUnknownIntent.
func New(baseURL, authToken string, timeout time.Duration) (*Embedder, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("sidecar.New: baseURL must be non-empty")
	}
	if strings.TrimSpace(authToken) == "" {
		return nil, errors.New("sidecar.New: authToken must be non-empty")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("sidecar.New: timeout must be > 0, got %s", timeout)
	}
	return &Embedder{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authToken:  authToken,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

type embedRequest struct {
	Text string `json:"text"`
}

type embedResponse struct {
	Vector []float32 `json:"vector"`
	Dim    int       `json:"dim"`
	Model  string    `json:"model"`
}

// Embed POSTs {text} to /embed and returns the parsed vector.
// Any HTTP error, non-2xx status, malformed body, empty vector, or
// dim mismatch returns a non-nil error.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{Text: text})
	if err != nil {
		return nil, fmt.Errorf("sidecar.Embed: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sidecar.Embed: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.authToken)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sidecar.Embed: POST /embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("sidecar.Embed: /embed returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var parsed embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("sidecar.Embed: decode response: %w", err)
	}
	if len(parsed.Vector) == 0 {
		return nil, errors.New("sidecar.Embed: /embed returned empty vector")
	}
	if parsed.Dim != 0 && parsed.Dim != len(parsed.Vector) {
		return nil, fmt.Errorf("sidecar.Embed: /embed dim=%d mismatches vector length %d", parsed.Dim, len(parsed.Vector))
	}
	return parsed.Vector, nil
}
