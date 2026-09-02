//go:build integration

// SearxNG integration test for spec 064 SCOPE-07.
//
// This test drives the real SearxNG container against the
// disposable test compose. Missing configuration, an unreachable
// container, a non-2xx response, malformed JSON, or an invalid mapped
// snippet is a hard failure. A well-formed response with an explicit
// empty results array is an upstream no-hit state: it proves the live
// transport and protocol contract, but not content availability.
//
// Run through the repository integration lane:
//
//	./smackerel.sh test integration --go-run '^TestSearxNGIntegration_Smoke$'
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/web"
)

type liveSearxNGResult struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type liveSearxNGExchange struct {
	method      string
	path        string
	query       url.Values
	accept      string
	statusCode  int
	contentType string
	body        []byte
}

// liveSearxNGProtocolObserver delegates every request to the real network
// transport and records the exact exchange consumed by web.SearxNG.Search.
// It never manufactures a response or answers on SearxNG's behalf.
type liveSearxNGProtocolObserver struct {
	next         *http.Transport
	requestCount int
	exchange     liveSearxNGExchange
}

func (o *liveSearxNGProtocolObserver) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := o.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read live SearxNG response: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close live SearxNG response: %w", closeErr)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	o.requestCount++
	o.exchange = liveSearxNGExchange{
		method:      req.Method,
		path:        req.URL.Path,
		query:       req.URL.Query(),
		accept:      req.Header.Get("Accept"),
		statusCode:  resp.StatusCode,
		contentType: resp.Header.Get("Content-Type"),
		body:        body,
	}
	return resp, nil
}

func requireLiveSearxNGProtocol(t *testing.T, observer *liveSearxNGProtocolObserver, query string, count int) []liveSearxNGResult {
	t.Helper()
	if observer.requestCount != 1 {
		t.Fatalf("expected exactly one real SearxNG request, got %d", observer.requestCount)
	}
	exchange := observer.exchange
	if exchange.method != http.MethodGet {
		t.Fatalf("SearxNG method=%q, want GET", exchange.method)
	}
	if exchange.path != "/search" {
		t.Fatalf("SearxNG path=%q, want /search", exchange.path)
	}
	if got := exchange.query.Get("q"); got != query {
		t.Fatalf("SearxNG q=%q, want %q", got, query)
	}
	if got := exchange.query.Get("format"); got != "json" {
		t.Fatalf("SearxNG format=%q, want json", got)
	}
	if got := exchange.query.Get("count"); got != strconv.Itoa(count) {
		t.Fatalf("SearxNG count=%q, want %d", got, count)
	}
	if exchange.accept != "application/json" {
		t.Fatalf("SearxNG Accept=%q, want application/json", exchange.accept)
	}
	if exchange.statusCode < 200 || exchange.statusCode >= 300 {
		t.Fatalf("SearxNG /search returned HTTP %d", exchange.statusCode)
	}
	mediaType, _, err := mime.ParseMediaType(exchange.contentType)
	if err != nil {
		t.Fatalf("SearxNG Content-Type %q is malformed: %v", exchange.contentType, err)
	}
	if mediaType != "application/json" {
		t.Fatalf("SearxNG Content-Type=%q, want application/json", exchange.contentType)
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(exchange.body, &envelope); err != nil {
		t.Fatalf("SearxNG /search response is malformed JSON: %v", err)
	}
	rawResults, ok := envelope["results"]
	if !ok {
		t.Fatal("SearxNG /search response is missing required results array")
	}
	if bytes.Equal(bytes.TrimSpace(rawResults), []byte("null")) {
		t.Fatal("SearxNG /search response has results=null, want an array")
	}
	var results []liveSearxNGResult
	if err := json.Unmarshal(rawResults, &results); err != nil {
		t.Fatalf("SearxNG /search results field is not a valid array: %v", err)
	}
	if results == nil {
		t.Fatal("SearxNG /search results did not decode as an explicit array")
	}
	for i, result := range results {
		rawURL := strings.TrimSpace(result.URL)
		parsedURL, parseErr := url.Parse(rawURL)
		if parseErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			t.Fatalf("SearxNG result[%d] has invalid URL %q", i, result.URL)
		}
		if strings.TrimSpace(result.Title) == "" {
			t.Fatalf("SearxNG result[%d] has empty title", i)
		}
		if strings.TrimSpace(result.Content) == "" {
			t.Fatalf("SearxNG result[%d] has empty content", i)
		}
	}
	return results
}

func TestSearxNGIntegration_Smoke(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("OPEN_KNOWLEDGE_SEARXNG_URL"))
	if endpoint == "" {
		t.Fatal("OPEN_KNOWLEDGE_SEARXNG_URL must be set by the integration runner")
	}

	healthClient := &http.Client{Timeout: 10 * time.Second}
	healthReq, err := http.NewRequest(http.MethodGet, strings.TrimRight(endpoint, "/")+"/healthz", nil)
	if err != nil {
		t.Fatalf("construct SearxNG health request: %v", err)
	}
	healthResp, err := healthClient.Do(healthReq)
	if err != nil {
		t.Fatalf("SearxNG endpoint %s is unreachable: %v", endpoint, err)
	}
	if closeErr := healthResp.Body.Close(); closeErr != nil {
		t.Fatalf("close SearxNG health response: %v", closeErr)
	}
	if healthResp.StatusCode < 200 || healthResp.StatusCode >= 300 {
		t.Fatalf("SearxNG health endpoint returned HTTP %d", healthResp.StatusCode)
	}

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("http.DefaultTransport is not an *http.Transport")
	}
	observer := &liveSearxNGProtocolObserver{next: baseTransport.Clone()}
	t.Cleanup(observer.next.CloseIdleConnections)
	client := &http.Client{Timeout: 10 * time.Second, Transport: observer}
	p, err := web.NewSearxNG(endpoint, client)
	if err != nil {
		t.Fatalf("NewSearxNG: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const query = "kale recipes"
	const count = 3
	searchStartedAt := time.Now().UTC()
	snips, err := p.Search(ctx, query, count)
	searchFinishedAt := time.Now().UTC()
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results := requireLiveSearxNGProtocol(t, observer, query, count)
	if len(results) == 0 {
		if len(snips) != 0 {
			t.Fatalf("SearxNG mapped %d snippets from an explicit empty results array", len(snips))
		}
		t.Log("SEARXNG_UPSTREAM_NO_HIT: live /search returned HTTP 2xx application/json with results=[]; transport and protocol are healthy, content availability is not proven")
		return
	}
	if len(snips) != len(results) {
		t.Fatalf("SearxNG mapped %d snippets from %d valid upstream rows", len(snips), len(results))
	}
	for i, s := range snips {
		if s.URL != results[i].URL || s.Title != results[i].Title {
			t.Errorf("[%d] mapped identity mismatch: got URL=%q title=%q, upstream URL=%q title=%q", i, s.URL, s.Title, results[i].URL, results[i].Title)
		}
		if strings.TrimSpace(s.Snippet) == "" {
			t.Errorf("[%d] mapped snippet is empty", i)
		}
		if s.ContentHash != web.CanonicalContentHash(s.URL, s.Title, s.Snippet) {
			t.Errorf("[%d] ContentHash not canonical", i)
		}
		if s.Provider != "searxng" {
			t.Errorf("[%d] Provider=%q", i, s.Provider)
		}
		if s.FetchedAt.IsZero() {
			t.Errorf("[%d] FetchedAt zero", i)
		} else if s.FetchedAt.Before(searchStartedAt) || s.FetchedAt.After(searchFinishedAt) {
			t.Errorf("[%d] FetchedAt=%s outside live search window [%s, %s]", i, s.FetchedAt, searchStartedAt, searchFinishedAt)
		}
	}
	t.Logf("SEARXNG_CONTENT_AVAILABLE: live /search returned and mapped %d valid snippet(s)", len(snips))
}
