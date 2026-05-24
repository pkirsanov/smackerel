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

// T-IMP-009-001: www. prefix is stripped for consistent dedup across www/non-www variants.
func TestNormalizeURL_StripWWWPrefix(t *testing.T) {
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
			name: "non-www unchanged",
			in:   "https://example.com/page",
			want: "https://example.com/page",
		},
		{
			name: "www with path and query",
			in:   "https://www.example.com/path?id=1",
			want: "https://example.com/path?id=1",
		},
		{
			name: "www with uppercase host",
			in:   "https://WWW.Example.COM/page",
			want: "https://example.com/page",
		},
		{
			name: "subdomain starting with www not stripped",
			in:   "https://www2.example.com/page",
			want: "https://www2.example.com/page",
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

// ============================================================================
// CHAOS R30 — Adversarial regression tests for NormalizeURL
// Stochastic sweep round 14 (sweep-2026-05-23-r30) found three normalization
// gaps that allowed dedup misses and DB-failure / log-injection vectors. Each
// of these tests MUST FAIL if the corresponding fix in dedup.go is reverted.
// ============================================================================

// T-CHAOS-R30-001: NormalizeURL MUST strip ASCII control characters from URLs
// before they become SourceRefs.
//
// Before the fix, embedded NUL (0x00) would survive into the source_ref TEXT
// column and cause `INSERT INTO artifacts` to fail (PostgreSQL rejects NUL in
// text). Embedded LF/CR/TAB would survive into structured log fields and
// enable log injection. Attacker-introduced control-char variants of the same
// URL would also dedup as distinct rows.
func TestChaosR30_NormalizeURLStripsControlChars(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "embedded NUL in path",
			in:   "http://example.com/path\x00more",
			want: "http://example.com/pathmore",
		},
		{
			name: "embedded LF in path",
			in:   "http://example.com/path\nmore",
			want: "http://example.com/pathmore",
		},
		{
			name: "embedded CR in path",
			in:   "http://example.com/path\rmore",
			want: "http://example.com/pathmore",
		},
		{
			name: "embedded TAB in path",
			in:   "http://example.com/\tpath",
			want: "http://example.com/path",
		},
		{
			name: "embedded DEL (0x7F) in path",
			in:   "http://example.com/path\x7Fmore",
			want: "http://example.com/pathmore",
		},
		{
			name: "mixed control chars across host and path",
			in:   "http://exa\nmple.com/p\rath",
			want: "http://example.com/path",
		},
		{
			name: "leading control chars before scheme",
			in:   "\r\nhttp://example.com/page",
			want: "http://example.com/page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if got != tt.want {
				t.Errorf("CHAOS R30-001: NormalizeURL(%q) = %q, want %q — control chars leaked into SourceRef", tt.in, got, tt.want)
			}
			// Belt-and-braces: result must not contain any control byte.
			for i := 0; i < len(got); i++ {
				if c := got[i]; c < 0x20 || c == 0x7F {
					t.Errorf("CHAOS R30-001: NormalizeURL(%q) = %q contains control byte 0x%02X at offset %d", tt.in, got, c, i)
					break
				}
			}
		})
	}
}

// T-CHAOS-R30-002: NormalizeURL MUST elide default ports (:80 for http,
// :443 for https, :21 for ftp) so canonically-equivalent URLs share a
// SourceRef.
//
// Before the fix, "https://example.com/" and "https://example.com:443/" were
// stored as two distinct artifacts even though every browser treats them as
// the same page. Same for "http://...:80/" and "ftp://...:21/".
func TestChaosR30_NormalizeURLElidesDefaultPorts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "https default port 443",
			in:   "https://example.com:443/page",
			want: "https://example.com/page",
		},
		{
			name: "http default port 80",
			in:   "http://example.com:80/page",
			want: "http://example.com/page",
		},
		{
			name: "ftp default port 21",
			in:   "ftp://files.example.com:21/pub",
			want: "ftp://files.example.com/pub",
		},
		{
			name: "https non-default port preserved",
			in:   "https://example.com:8443/api",
			want: "https://example.com:8443/api",
		},
		{
			name: "http non-default port preserved",
			in:   "http://example.com:8080/page",
			want: "http://example.com:8080/page",
		},
		{
			name: "https default port with userinfo (both stripped)",
			in:   "https://user:pass@example.com:443/admin",
			want: "https://example.com/admin",
		},
		{
			name: "https default port with www prefix (both stripped)",
			in:   "https://www.example.com:443/page",
			want: "https://example.com/page",
		},
		{
			name: "https default port with tracking params",
			in:   "https://example.com:443/p?utm_source=x&id=1",
			want: "https://example.com/p?id=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if got != tt.want {
				t.Errorf("CHAOS R30-002: NormalizeURL(%q) = %q, want %q — default port not elided", tt.in, got, tt.want)
			}
		})
	}
}

// T-CHAOS-R30-003: NormalizeURL MUST strip the trailing DNS-root dot from the
// hostname so "example.com." and "example.com" dedup as one SourceRef.
//
// Before the fix, "http://example.com./foo" and "http://example.com/foo"
// produced two distinct SourceRefs even though they resolve to the same
// origin in every browser.
func TestChaosR30_NormalizeURLStripsTrailingDot(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trailing dot in host",
			in:   "http://example.com./foo",
			want: "http://example.com/foo",
		},
		{
			name: "trailing dot with uppercase",
			in:   "https://EXAMPLE.COM./bar",
			want: "https://example.com/bar",
		},
		{
			name: "trailing dot with www prefix",
			in:   "https://www.example.com./baz",
			want: "https://example.com/baz",
		},
		{
			name: "trailing dot with default port",
			in:   "https://example.com.:443/page",
			want: "https://example.com/page",
		},
		{
			name: "no trailing dot (control)",
			in:   "https://example.com/page",
			want: "https://example.com/page",
		},
		{
			name: "multiple trailing dots collapse",
			in:   "https://example.com.../page",
			want: "https://example.com/page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if got != tt.want {
				t.Errorf("CHAOS R30-003: NormalizeURL(%q) = %q, want %q — trailing dot not stripped", tt.in, got, tt.want)
			}
		})
	}
}

// T-CHAOS-R30-004: SourceRef from ToRawArtifacts MUST never contain a NUL
// byte. PostgreSQL TEXT columns reject NUL (0x00), which would block the
// INSERT and prevent the artifact from ever entering the dedup table — a
// silent capture loss.
func TestChaosR30_ToRawArtifactsRejectsNULInSourceRef(t *testing.T) {
	books := []Bookmark{
		{Title: "evil", URL: "http://example.com/\x00danger"},
		{Title: "good", URL: "http://example.com/safe"},
	}
	got := ToRawArtifacts(books)
	if len(got) != 2 {
		t.Fatalf("ToRawArtifacts: got %d artifacts, want 2", len(got))
	}
	for i, a := range got {
		for j := 0; j < len(a.SourceRef); j++ {
			if a.SourceRef[j] == 0x00 {
				t.Errorf("CHAOS R30-004: artifact[%d].SourceRef = %q contains NUL byte at offset %d — would fail PG insert", i, a.SourceRef, j)
			}
		}
	}
}

// T-CHAOS-R30-005: Two URLs that differ ONLY by an embedded control character
// MUST normalize to the same SourceRef so dedup catches the duplicate.
func TestChaosR30_ControlCharVariantsDedup(t *testing.T) {
	pairs := []struct {
		clean, dirty string
	}{
		{"http://example.com/page", "http://example.com/\npage"},
		{"http://example.com/page", "http://example.com/page\r"},
		{"http://example.com/page", "http://example.com/pa\tge"},
		{"http://example.com/page", "http://example.com/pa\x00ge"},
	}
	ctx := context.Background()
	_ = ctx // silence unused if dedup not invoked
	for i, p := range pairs {
		a := NormalizeURL(p.clean)
		b := NormalizeURL(p.dirty)
		if a != b {
			t.Errorf("CHAOS R30-005[%d]: NormalizeURL(%q)=%q ≠ NormalizeURL(%q)=%q — control-char variant dedup miss", i, p.clean, a, p.dirty, b)
		}
	}
	// Also verify FilterNew nil-pool path stays correct under chaos input.
	d := NewURLDeduplicator(nil)
	artifacts := []connector.RawArtifact{
		{SourceID: "bookmarks", URL: "http://example.com/page", SourceRef: NormalizeURL("http://example.com/\npage")},
	}
	out, dupes, err := d.FilterNew(context.Background(), artifacts)
	if err != nil {
		t.Fatalf("FilterNew nil-pool error: %v", err)
	}
	if dupes != 0 || len(out) != 1 {
		t.Fatalf("FilterNew nil-pool: out=%d dupes=%d, want 1/0", len(out), dupes)
	}
	for _, a := range out {
		for j := 0; j < len(a.SourceRef); j++ {
			if a.SourceRef[j] == 0x00 || a.SourceRef[j] == 0x0A {
				t.Errorf("CHAOS R30-005: FilterNew passed through SourceRef with control byte: %q", a.SourceRef)
				break
			}
		}
	}
}

// T-CHAOS-R30-006: stripURLControlChars unit-level fast path — strings that
// contain no control characters must be returned unchanged (no allocation).
func TestChaosR30_StripURLControlCharsFastPath(t *testing.T) {
	in := "https://example.com/perfectly/normal?q=1&r=2"
	out := stripURLControlChars(in)
	if out != in {
		t.Errorf("stripURLControlChars(%q) = %q, want unchanged", in, out)
	}
	// Empty input passes through.
	if got := stripURLControlChars(""); got != "" {
		t.Errorf("stripURLControlChars(\"\") = %q, want \"\"", got)
	}
}
