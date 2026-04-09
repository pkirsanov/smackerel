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

// T-2-06 through T-2-08 test FilterNew which requires a DB pool.
// These are tested as integration tests (they need postgres).
// Here we test the nil-pool graceful handling.
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
