package web

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordingTransport records whether RoundTrip was invoked. Used to
// prove that deny decisions do NOT reach the inner transport
// (adversarial: an implementation that calls inner first then
// inspects the request would still leak DNS / SYN packets to the
// disallowed host).
type recordingTransport struct {
	called int
	inner  http.RoundTripper
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.called++
	if r.inner != nil {
		return r.inner.RoundTrip(req)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
}

func TestNewEgressAllowlistTransport_RejectsMalformedEntries(t *testing.T) {
	cases := []struct {
		name    string
		entries []string
	}{
		{"empty_entry", []string{""}},
		{"whitespace_entry", []string{"   "}},
		{"with_scheme", []string{"https://example.com"}},
		{"with_path", []string{"example.com/foo"}},
		{"with_userinfo", []string{"user:pass@example.com"}},
		{"with_port", []string{"example.com:8080"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewEgressAllowlistTransport(c.entries, http.DefaultTransport)
			if err == nil {
				t.Fatalf("expected ErrInvalidConfig for %v", c.entries)
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestNewEgressAllowlistTransport_NilInner(t *testing.T) {
	_, err := NewEgressAllowlistTransport([]string{"example.com"}, nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for nil inner, got %v", err)
	}
}

// TestEgressAllowlistTransport_DenyByDefault_Adversarial proves G021
// invariant: an empty allowlist denies every request — there is no
// silent fallback to allow-all. A regression that flipped the
// default would let any host pass.
func TestEgressAllowlistTransport_DenyByDefault_Adversarial(t *testing.T) {
	inner := &recordingTransport{}
	tr, err := NewEgressAllowlistTransport(nil, inner)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	_, err = tr.RoundTrip(req)
	if !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("expected ErrEgressDenied, got %v", err)
	}
	if inner.called != 0 {
		t.Fatalf("inner transport MUST NOT be called on deny; got %d calls", inner.called)
	}
}

func TestEgressAllowlistTransport_AllowedHostPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// httptest binds 127.0.0.1; allowlist it.
	tr, err := NewEgressAllowlistTransport([]string{"127.0.0.1"}, http.DefaultTransport)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("allowed request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestEgressAllowlistTransport_DisallowedHostDenied(t *testing.T) {
	inner := &recordingTransport{}
	tr, err := NewEgressAllowlistTransport([]string{"allowed.example"}, inner)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://blocked.example/", nil)
	_, err = tr.RoundTrip(req)
	if !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("expected ErrEgressDenied for disallowed host, got %v", err)
	}
	if inner.called != 0 {
		t.Fatalf("inner transport MUST NOT be called on deny; got %d calls", inner.called)
	}
}

// TestEgressAllowlistTransport_NormalizesMixedCaseHost proves that
// host comparison is case-insensitive — an attacker cannot bypass
// allowlist {"example.com"} by requesting "Example.COM".
func TestEgressAllowlistTransport_NormalizesMixedCaseHost(t *testing.T) {
	inner := &recordingTransport{}
	tr, err := NewEgressAllowlistTransport([]string{"Example.COM"}, inner)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://EXAMPLE.com/path", nil)
	_, err = tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("mixed-case host should be allowed, got %v", err)
	}
	if inner.called != 1 {
		t.Fatalf("expected inner called once, got %d", inner.called)
	}
}

// TestEgressAllowlistTransport_UserinfoDoesNotBypass proves that a
// URL with embedded userinfo (https://user:pass@allowed.example/)
// has its host extracted from .Hostname() — userinfo is stripped
// before comparison, so an attacker cannot inject credentials to
// confuse allowlist matching.
func TestEgressAllowlistTransport_UserinfoDoesNotBypass(t *testing.T) {
	inner := &recordingTransport{}
	tr, err := NewEgressAllowlistTransport([]string{"allowed.example"}, inner)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	// userinfo present + allowed host → allowed.
	req, _ := http.NewRequest(http.MethodGet, "https://user:pass@allowed.example/", nil)
	_, err = tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("userinfo + allowed host should pass, got %v", err)
	}
	// userinfo present + disallowed host → denied (no smuggling).
	req2, _ := http.NewRequest(http.MethodGet, "https://allowed.example:pass@blocked.example/", nil)
	_, err = tr.RoundTrip(req2)
	if !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("userinfo containing allowed-host string MUST NOT bypass allowlist on blocked.example, got %v", err)
	}
}

func TestEgressAllowlistTransport_RejectsNonHTTPScheme(t *testing.T) {
	inner := &recordingTransport{}
	tr, err := NewEgressAllowlistTransport([]string{"example.com"}, inner)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	cases := []string{"file:///etc/passwd", "ftp://example.com/", "gopher://example.com/"}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, raw, nil)
			_, err := tr.RoundTrip(req)
			if !errors.Is(err, ErrEgressDenied) {
				t.Fatalf("expected ErrEgressDenied for %q, got %v", raw, err)
			}
			if inner.called != 0 {
				t.Fatalf("inner transport MUST NOT be called for non-http(s) scheme; got %d", inner.called)
			}
		})
	}
}

// TestEgressAllowlistTransport_AllowsHTTPForLoopback covers the
// in-cluster SearxNG path: provider_endpoint is http://searxng:8080
// inside the compose network. The host-based allowlist is what
// authorises it; scheme is permitted as long as it is http or https.
func TestEgressAllowlistTransport_AllowsHTTPForAllowedHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	tr, err := NewEgressAllowlistTransport([]string{"127.0.0.1"}, http.DefaultTransport)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	client := &http.Client{Transport: tr}
	// httptest.NewServer uses http://; verify it goes through.
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("http on allowed host: %v", err)
	}
	resp.Body.Close()
}
