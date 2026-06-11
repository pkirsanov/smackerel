package cardrewards

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// mustConnect builds a connected card-rewards connector wired to the given
// sources. allowPrivateHosts is set so the SSRF guard permits the loopback
// httptest servers used throughout these tests (white-box seam).
func mustConnect(t *testing.T, sources []Source, timeoutSeconds int) *Connector {
	t.Helper()
	c := New()
	c.allowPrivateHosts = true
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]any{
			"sources":               sources,
			"fetch_timeout_seconds": timeoutSeconds,
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return c
}

// SCN-083-D01 — connector implements the Connector interface and ID()=="card-rewards".
func TestConnector_ImplementsInterfaceAndID_D01(t *testing.T) {
	var _ connector.Connector = New() // compile-time interface compliance

	c := New()
	if c.ID() != ConnectorID || c.ID() != "card-rewards" {
		t.Fatalf("ID() = %q, want %q", c.ID(), "card-rewards")
	}
}

// SCN-083-D06 — cursor advances to the last successful fetch timestamp.
func TestSync_CursorEncodesLastSuccessfulFetch_D06(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("page body"))
	}))
	defer srv.Close()

	c := mustConnect(t, []Source{{Name: "src", URL: srv.URL, IssuerHint: "issuer"}}, 5)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	arts, cursor, err := c.Sync(ctx, "")
	after := time.Now().UTC().Add(time.Second)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if cursor == "" {
		t.Fatalf("cursor must be non-empty after a successful sync")
	}
	parsed, perr := time.Parse(time.RFC3339Nano, cursor)
	if perr != nil {
		t.Fatalf("cursor %q is not RFC3339Nano: %v", cursor, perr)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Fatalf("cursor time %v not within [%v, %v]", parsed, before, after)
	}
}

// SCN-083-D02 — Sync emits one source-attributed RawArtifact per configured source.
func TestSync_EmitsSourceAttributedArtifactPerSource_D02(t *testing.T) {
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("body A"))
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("body B"))
	}))
	defer srvB.Close()

	sources := []Source{
		{Name: "Doctor of Credit", URL: srvA.URL, IssuerHint: "chase"},
		{Name: "Issuer Page", URL: srvB.URL, IssuerHint: "amex"},
	}
	c := mustConnect(t, sources, 5)

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts (one per source), got %d", len(arts))
	}

	byURL := map[string]connector.RawArtifact{}
	for _, a := range arts {
		if a.SourceID != ConnectorID {
			t.Errorf("SourceID = %q, want %q", a.SourceID, ConnectorID)
		}
		u, _ := a.Metadata["source_url"].(string)
		byURL[u] = a
	}
	for _, src := range sources {
		a, ok := byURL[src.URL]
		if !ok {
			t.Fatalf("no artifact emitted for source %q", src.URL)
		}
		if a.Metadata["source_name"] != src.Name {
			t.Errorf("source_name = %v, want %q", a.Metadata["source_name"], src.Name)
		}
		if a.Metadata["issuer_hint"] != src.IssuerHint {
			t.Errorf("issuer_hint = %v, want %q", a.Metadata["issuer_hint"], src.IssuerHint)
		}
	}
}

// SCN-083-D03 — connector applies no category parsing/regex: RawContent is
// verbatim and Metadata carries provenance only.
func TestSync_NoCategoryParsingRawContentVerbatim_D03(t *testing.T) {
	const body = "Discover it — 5% cash back at Grocery Stores and Wholesale Clubs, Q3 2026 (Jul-Sep). Activation required."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := mustConnect(t, []Source{{Name: "src", URL: srv.URL, IssuerHint: "discover"}}, 5)
	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].RawContent != body {
		t.Fatalf("RawContent must be the verbatim page text.\n got: %q\nwant: %q", arts[0].RawContent, body)
	}
	// Metadata must be provenance-only (no parsed categories/rates).
	if len(arts[0].Metadata) != 3 {
		t.Fatalf("expected exactly 3 metadata keys (provenance only), got %d: %v",
			len(arts[0].Metadata), arts[0].Metadata)
	}
	for _, banned := range []string{"categories", "category", "rate", "cashback", "rewards"} {
		if _, exists := arts[0].Metadata[banned]; exists {
			t.Errorf("connector must not parse categories; found parsed key %q in metadata", banned)
		}
	}
}

// SCN-083-D04 — a slow source is skipped (recorded failed) while the healthy
// source still emits an artifact.
func TestSync_SlowSourceDegradesOnlyThatSource_D04(t *testing.T) {
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client cancels (its per-source deadline fires) or a
		// generous ceiling elapses; respecting r.Context() avoids a leaked
		// goroutine and keeps the test fast.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			_, _ = w.Write([]byte("slow body"))
		}
	}))
	defer slowSrv.Close()
	fastSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fast body"))
	}))
	defer fastSrv.Close()

	c := mustConnect(t, []Source{
		{Name: "slow-source", URL: slowSrv.URL, IssuerHint: "slow-issuer"},
		{Name: "fast-source", URL: fastSrv.URL, IssuerHint: "fast-issuer"},
	}, 5)
	// White-box: shrink the per-source deadline so the slow source trips it
	// quickly. Config granularity is whole seconds; this keeps the test fast
	// without changing the behavior under test (per-source isolation).
	c.fetchTimeout = 150 * time.Millisecond

	ctx := context.Background()
	arts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected exactly 1 artifact (healthy source), got %d", len(arts))
	}
	if got, _ := arts[0].Metadata["source_url"].(string); got != fastSrv.URL {
		t.Fatalf("emitted artifact should be from the fast source %q, got %q", fastSrv.URL, got)
	}
	emitted, failed := c.LastSyncStats()
	if emitted != 1 || failed != 1 {
		t.Fatalf("expected emitted=1 failed=1, got emitted=%d failed=%d", emitted, failed)
	}
	if got := c.Health(ctx); got != connector.HealthDegraded {
		t.Fatalf("partial success should be degraded, got %s", got)
	}
	if cursor == "" {
		t.Fatalf("cursor should advance on partial success")
	}
}

// SCN-083-D05 — health reflects consecutive failures via HealthFromErrorCount
// thresholds (0-4 healthy, 5-9 degraded, 10+ failing).
func TestHealth_ReflectsConsecutiveErrors_D05(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	c := mustConnect(t, []Source{{Name: "always-fails", URL: failSrv.URL, IssuerHint: "x"}}, 5)
	ctx := context.Background()

	// 1-4 consecutive failures → still healthy per HealthFromErrorCount.
	for i := 1; i <= 4; i++ {
		arts, cursor, err := c.Sync(ctx, "")
		if err != nil {
			t.Fatalf("sync %d: unexpected error %v", i, err)
		}
		if len(arts) != 0 {
			t.Fatalf("sync %d: expected 0 artifacts, got %d", i, len(arts))
		}
		if cursor != "" {
			t.Fatalf("sync %d: cursor must stay unchanged on total failure, got %q", i, cursor)
		}
		if got := c.Health(ctx); got != connector.HealthHealthy {
			t.Fatalf("after %d failures: health=%s, want %s", i, got, connector.HealthHealthy)
		}
	}
	// 5th consecutive failure → degraded.
	c.Sync(ctx, "")
	if got := c.Health(ctx); got != connector.HealthDegraded {
		t.Fatalf("after 5 failures: health=%s, want %s", got, connector.HealthDegraded)
	}
	// Drive to 10 consecutive failures → failing.
	for i := 6; i <= 10; i++ {
		c.Sync(ctx, "")
	}
	if got := c.Health(ctx); got != connector.HealthFailing {
		t.Fatalf("after 10 failures: health=%s, want %s", got, connector.HealthFailing)
	}
}

// A successful Sync resets the consecutive-failure streak (recovery path).
func TestSync_SuccessResetsConsecutiveErrors(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n <= 6 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := mustConnect(t, []Source{{Name: "flaky", URL: srv.URL, IssuerHint: "x"}}, 5)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		c.Sync(ctx, "")
	}
	if got := c.Health(ctx); got != connector.HealthDegraded {
		t.Fatalf("precondition: want degraded after 6 failures, got %s", got)
	}

	arts, cursor, err := c.Sync(ctx, "") // 7th call → HTTP 200
	if err != nil || len(arts) != 1 {
		t.Fatalf("recovery sync: arts=%d err=%v", len(arts), err)
	}
	if cursor == "" {
		t.Fatalf("recovery sync should advance the cursor")
	}
	if got := c.Health(ctx); got != connector.HealthHealthy {
		t.Fatalf("after recovery: health=%s, want healthy", got)
	}
	if _, failed := c.LastSyncStats(); failed != 0 {
		t.Fatalf("recovery sync should report 0 failures, got %d", failed)
	}
}

// Total failure leaves the cursor unchanged so the next run retries the window.
func TestSync_TotalFailureKeepsCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := mustConnect(t, []Source{{Name: "down", URL: srv.URL, IssuerHint: "x"}}, 5)
	arts, cursor, err := c.Sync(context.Background(), "prev-cursor")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(arts) != 0 {
		t.Fatalf("expected 0 artifacts, got %d", len(arts))
	}
	if cursor != "prev-cursor" {
		t.Fatalf("cursor must be unchanged on total failure, got %q", cursor)
	}
	emitted, failed := c.LastSyncStats()
	if emitted != 0 || failed != 1 {
		t.Fatalf("expected emitted=0 failed=1, got %d/%d", emitted, failed)
	}
}

// Connect is fail-loud (Gate G028 / smackerel-no-defaults): no defaults are
// supplied for a missing/empty source list or a non-positive fetch timeout.
func TestConnect_FailsLoudOnInvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		sc   map[string]any
	}{
		{"nil source_config", nil},
		{"missing sources", map[string]any{"fetch_timeout_seconds": 5}},
		{"empty sources", map[string]any{"sources": []Source{}, "fetch_timeout_seconds": 5}},
		{"source missing url", map[string]any{"sources": []Source{{Name: "x", URL: ""}}, "fetch_timeout_seconds": 5}},
		{"source missing name", map[string]any{"sources": []Source{{Name: "", URL: "https://e.com"}}, "fetch_timeout_seconds": 5}},
		{"missing timeout", map[string]any{"sources": []Source{{Name: "x", URL: "https://e.com"}}}},
		{"zero timeout", map[string]any{"sources": []Source{{Name: "x", URL: "https://e.com"}}, "fetch_timeout_seconds": 0}},
		{"negative timeout", map[string]any{"sources": []Source{{Name: "x", URL: "https://e.com"}}, "fetch_timeout_seconds": -3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			err := c.Connect(context.Background(), connector.ConnectorConfig{SourceConfig: tc.sc})
			if err == nil {
				t.Fatalf("expected fail-loud error for %q, got nil", tc.name)
			}
			if c.Health(context.Background()) != connector.HealthDisconnected {
				t.Errorf("a failed Connect must leave the connector disconnected")
			}
		})
	}
}

// Sync before Connect is a well-defined error (not a silent no-op).
func TestSync_BeforeConnectErrors(t *testing.T) {
	c := New()
	if _, _, err := c.Sync(context.Background(), ""); err == nil {
		t.Fatal("Sync before Connect should return an error")
	}
}

// Close marks the connector disconnected and blocks further syncs.
func TestClose_SetsDisconnectedAndBlocksSync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := mustConnect(t, []Source{{Name: "s", URL: srv.URL, IssuerHint: "x"}}, 5)
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Fatalf("Close should set health=disconnected")
	}
	if _, _, err := c.Sync(context.Background(), ""); err == nil {
		t.Fatal("Sync after Close should error")
	}
}

// SSRF guard: scheme allowlist + private/loopback rejection (defense in depth).
func TestValidateSourceURL_SSRFGuard(t *testing.T) {
	for _, bad := range []string{
		"file:///etc/passwd",
		"ftp://internal/data",
		"gopher://evil/x",
		"http://", // empty host
	} {
		if err := validateSourceURL(bad, false); err == nil {
			t.Errorf("validateSourceURL(%q) should be rejected", bad)
		}
	}
	// Literal private/reserved IPs are rejected when private hosts are not
	// allowed (literal IPs resolve offline — no real DNS).
	for _, priv := range []string{
		"http://127.0.0.1/x",
		"http://10.0.0.5/x",
		"http://169.254.169.254/latest/meta-data",
	} {
		if err := validateSourceURL(priv, false); err == nil {
			t.Errorf("validateSourceURL(%q) should be rejected as private/reserved", priv)
		}
		if err := validateSourceURL(priv, true); err != nil {
			t.Errorf("validateSourceURL(%q, allowPrivate=true) should pass, got %v", priv, err)
		}
	}
	// A public literal IP is allowed (offline-safe: net.LookupHost short-circuits literals).
	if err := validateSourceURL("https://8.8.8.8/x", false); err != nil {
		t.Errorf("public IP should be allowed, got %v", err)
	}
}

// The generic []any / []map[string]any config forms (JSON/YAML round-trip)
// parse identically to the typed []Source form used by wiring.
func TestParseSources_AcceptsGenericConfigForms(t *testing.T) {
	sc := map[string]any{
		"sources": []any{
			map[string]any{"name": "A", "url": "https://a.com", "issuer_hint": "ha"},
			map[string]any{"name": "B", "url": "https://b.com", "issuer_hint": "hb"},
		},
	}
	got, err := parseSources(sc)
	if err != nil {
		t.Fatalf("parseSources([]any): %v", err)
	}
	if len(got) != 2 || got[0].Name != "A" || got[1].URL != "https://b.com" || got[0].IssuerHint != "ha" {
		t.Fatalf("unexpected parse result: %+v", got)
	}

	sc2 := map[string]any{"sources": []map[string]any{{"name": "C", "url": "https://c.com"}}}
	got2, err := parseSources(sc2)
	if err != nil || len(got2) != 1 || got2[0].Name != "C" {
		t.Fatalf("parseSources([]map): %+v err=%v", got2, err)
	}
}

func TestParseFetchTimeout_AcceptsNumericForms(t *testing.T) {
	for _, raw := range []any{int(20), int64(20), float64(20)} {
		d, err := parseFetchTimeout(map[string]any{"fetch_timeout_seconds": raw})
		if err != nil {
			t.Fatalf("parseFetchTimeout(%T): %v", raw, err)
		}
		if d != 20*time.Second {
			t.Fatalf("parseFetchTimeout(%T) = %v, want 20s", raw, d)
		}
	}
	if _, err := parseFetchTimeout(map[string]any{"fetch_timeout_seconds": "20"}); err == nil {
		t.Fatal("string timeout should be rejected (no silent coercion)")
	}
}
