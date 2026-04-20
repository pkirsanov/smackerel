package bookmarks

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

// T-2-01
func TestNormalizeURL_Lowercase(t *testing.T) {
	got := NormalizeURL("HTTPS://Example.COM/Page")
	want := "https://example.com/Page"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-02
func TestNormalizeURL_StripTrailingSlash(t *testing.T) {
	got := NormalizeURL("https://example.com/page/")
	want := "https://example.com/page"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-03
func TestNormalizeURL_StripUTMParams(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "utm_source with other param",
			in:   "https://example.com/page?utm_source=twitter&id=123",
			want: "https://example.com/page?id=123",
		},
		{
			name: "fbclid only",
			in:   "https://example.com/page?fbclid=abc123",
			want: "https://example.com/page",
		},
		{
			name: "gclid only",
			in:   "https://example.com/page?gclid=xyz",
			want: "https://example.com/page",
		},
		{
			name: "multiple utm params",
			in:   "https://example.com/p?utm_source=a&utm_medium=b&utm_campaign=c&keep=1",
			want: "https://example.com/p?keep=1",
		},
		{
			name: "ref param",
			in:   "https://example.com/article?ref=homepage&id=5",
			want: "https://example.com/article?id=5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// T-2-04
func TestNormalizeURL_PreservesPath(t *testing.T) {
	got := NormalizeURL("https://Example.COM/CamelCase/Path?id=1")
	want := "https://example.com/CamelCase/Path?id=1"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-05
func TestNormalizeURL_InvalidURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"no scheme", "://broken"},
		{"garbage", "not a url at all"},
		{"just path", "/some/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if got != tt.in {
				t.Errorf("NormalizeURL(%q) = %q, want %q (returned as-is)", tt.in, got, tt.in)
			}
		})
	}
}

// T-2-R1 Regression: invalid URL does not crash normalizer
func TestNormalizeURL_Regression_NoPanic(t *testing.T) {
	// Should not panic on any input
	inputs := []string{
		"",
		"://",
		"http://",
		"ftp://broken",
		string([]byte{0x00, 0x01, 0x02}),
		"https://example.com/path?utm_source=&utm_medium=",
	}

	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("NormalizeURL(%q) panicked: %v", in, r)
				}
			}()
			_ = NormalizeURL(in)
		}()
	}
}

// T-2-09: Fragment is stripped from normalized URLs.
func TestNormalizeURL_StripFragment(t *testing.T) {
	got := NormalizeURL("https://example.com/page#section-2")
	want := "https://example.com/page"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q (fragment stripped)", got, want)
	}
}

// T-2-10: Combination of all normalizations applied together.
func TestNormalizeURL_CombinedNormalization(t *testing.T) {
	got := NormalizeURL("HTTPS://Example.COM/Page/?utm_source=twitter&id=5#footer")
	want := "https://example.com/Page?id=5"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-11: Root path "/" is preserved (not stripped to empty).
func TestNormalizeURL_RootPath(t *testing.T) {
	got := NormalizeURL("https://example.com/")
	want := "https://example.com/"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q (root path preserved)", got, want)
	}
}

// T-2-06 through T-2-08 test FilterNew which requires a live DB pool.
// These are integration-test paths that need PostgreSQL — not yet implemented
// (tracked as spec 009 test gap). Here we test nil-pool graceful degradation.
func TestFilterNew_NilPool(t *testing.T) {
	d := NewURLDeduplicator(nil)
	in := []connector.RawArtifact{
		{URL: "https://example.com", SourceRef: "https://example.com"},
	}

	result, dupes, err := d.FilterNew(context.Background(), in)
	if err != nil {
		t.Fatalf("FilterNew() error: %v", err)
	}
	if dupes != 0 {
		t.Errorf("dupes = %d, want 0", dupes)
	}
	if len(result) != len(in) {
		t.Errorf("result len = %d, want %d", len(result), len(in))
	}
}

func TestIsKnown_NilPool(t *testing.T) {
	d := NewURLDeduplicator(nil)
	known, err := d.IsKnown(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("IsKnown() error: %v", err)
	}
	if known {
		t.Error("IsKnown() = true with nil pool, want false")
	}
}

// T-2-12: NormalizeURL preserves port numbers.
func TestNormalizeURL_WithPort(t *testing.T) {
	got := NormalizeURL("HTTPS://Example.COM:8080/page")
	want := "https://example.com:8080/page"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-13: NormalizeURL strips all tracking params leaving clean URL.
func TestNormalizeURL_AllTrackingParamsStripped(t *testing.T) {
	got := NormalizeURL("https://example.com/page?utm_source=a&utm_medium=b&utm_campaign=c&utm_term=d&utm_content=e&fbclid=f&gclid=g&ref=h")
	want := "https://example.com/page"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-14: NormalizeURL strips auth credentials from URLs (F-CHAOS-R24-002).
func TestNormalizeURL_WithCredentials(t *testing.T) {
	got := NormalizeURL("https://user:pass@Example.COM/page")
	want := "https://example.com/page"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-15: NormalizeURL with query params but no tracking params preserves them.
func TestNormalizeURL_NoTrackingParams(t *testing.T) {
	got := NormalizeURL("https://example.com/search?q=test&page=2")
	// No tracking params to strip → URL is returned unchanged (no re-encoding)
	want := "https://example.com/search?q=test&page=2"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-16: NormalizeURL handles FTP scheme.
func TestNormalizeURL_FTPScheme(t *testing.T) {
	got := NormalizeURL("FTP://Files.Example.COM/readme.txt")
	want := "ftp://files.example.com/readme.txt"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// T-2-17: FilterNew with empty artifacts and nil pool returns empty.
func TestFilterNew_EmptyArtifacts(t *testing.T) {
	d := NewURLDeduplicator(nil)
	result, dupes, err := d.FilterNew(context.Background(), []connector.RawArtifact{})
	if err != nil {
		t.Fatalf("FilterNew() error: %v", err)
	}
	if dupes != 0 {
		t.Errorf("dupes = %d, want 0", dupes)
	}
	if len(result) != 0 {
		t.Errorf("result len = %d, want 0", len(result))
	}
}

// T-2-18: FilterNew with nil artifacts and nil pool returns nil.
func TestFilterNew_NilArtifacts(t *testing.T) {
	d := NewURLDeduplicator(nil)
	result, dupes, err := d.FilterNew(context.Background(), nil)
	if err != nil {
		t.Fatalf("FilterNew() error: %v", err)
	}
	if dupes != 0 {
		t.Errorf("dupes = %d, want 0", dupes)
	}
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
}

// T-2-19: NormalizeURL with only a fragment and no query.
func TestNormalizeURL_OnlyFragment(t *testing.T) {
	got := NormalizeURL("https://example.com/page#top")
	want := "https://example.com/page"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// ============================================================================
// IMP-009-R-002 — www. prefix normalization for cross-variant dedup
// ============================================================================

// T-IMP-009-R-002-A: www. prefix is stripped so www and non-www normalize to same URL.
func TestNormalizeURL_StripWWW(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "www prefix stripped",
			in:   "https://www.example.com/page",
			want: "https://example.com/page",
		},
		{
			name: "www with port",
			in:   "https://www.example.com:8080/page",
			want: "https://example.com:8080/page",
		},
		{
			name: "www uppercase",
			in:   "HTTPS://WWW.Example.COM/page",
			want: "https://example.com/page",
		},
		{
			name: "already without www",
			in:   "https://example.com/page",
			want: "https://example.com/page",
		},
		{
			name: "www2 not stripped",
			in:   "https://www2.example.com/page",
			want: "https://www2.example.com/page",
		},
		{
			name: "wwwexample not stripped",
			in:   "https://wwwexample.com/page",
			want: "https://wwwexample.com/page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// T-IMP-009-R-002-B: Adversarial — www and non-www variants normalize identically.
// Without the fix, these would produce different normalized URLs, causing
// duplicate artifacts when the same page is bookmarked from different browsers
// that add or omit the www prefix.
func TestNormalizeURL_WWWDedup(t *testing.T) {
	www := NormalizeURL("https://www.example.com/article?id=42")
	noWWW := NormalizeURL("https://example.com/article?id=42")
	if www != noWWW {
		t.Errorf("IMP-009-R-002: www variant %q != non-www variant %q — "+
			"same page would create duplicate artifacts", www, noWWW)
	}
}

// T-2-20: NormalizeURL with empty path (just host).
func TestNormalizeURL_HostOnly(t *testing.T) {
	got := NormalizeURL("https://example.com")
	want := "https://example.com"
	if got != want {
		t.Errorf("NormalizeURL() = %q, want %q", got, want)
	}
}

// ============================================================================
// CHAOS R24 — Adversarial regression tests for NormalizeURL
// ============================================================================

// T-CHAOS-R24-002: NormalizeURL MUST strip userinfo to prevent credential
// leaks into SourceRef. Before the fix, "https://user:pass@host/p" would
// store the credentials in the database.
func TestChaosR24_NormalizeURLStripsUserinfo(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "user:pass basic auth",
			in:   "https://user:pass@example.com/page",
			want: "https://example.com/page",
		},
		{
			name: "user-only (no password)",
			in:   "https://admin@example.com/path",
			want: "https://example.com/path",
		},
		{
			name: "encoded credentials",
			in:   "https://u%40ser:p%40ss@example.com/path",
			want: "https://example.com/path",
		},
		{
			name: "userinfo with port",
			in:   "https://root:secret@db.example.com:5432/admin",
			want: "https://db.example.com:5432/admin",
		},
		{
			name: "ftp with credentials",
			in:   "ftp://anonymous:email@files.example.com/pub",
			want: "ftp://files.example.com/pub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if got != tt.want {
				t.Errorf("CHAOS R24-002: NormalizeURL(%q) = %q, want %q — userinfo leaked", tt.in, got, tt.want)
			}
		})
	}
}
