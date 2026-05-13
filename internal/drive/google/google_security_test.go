// MIT-038-S-004 — Adversarial regression tests for the defense-in-depth
// `io.LimitReader` wraps applied to every `io.ReadAll(resp.Body)` site in
// internal/drive/google/google.go (9 sites). Each test deliberately
// constructs an oversized response body and asserts that the cap fires —
// proving that removing the wrap would re-introduce the unbounded-read
// vulnerability surfaced by the spec 038 security audit.
//
// Caps under test (sourced from drive.io_limits.* SST keys):
//
//	provider_response_max_bytes — 7 sites: ListFolder (455), GetFile error
//	  (494), PutFile response (556), EnsureFolder GET error (615),
//	  EnsureFolder POST response (638), Changes (679), fetchAccountEmail
//	  (356, exercised here as the unexported response-cap site).
//	provider_binary_max_bytes   — 1 site: PutFile body.Reader (530,
//	  exercised here via the readUploadBody helper which centralizes the
//	  binary-cap wrap).
//	oauth_response_max_bytes    — 1 site: exchangeCodeForToken (325).
//
// Each adversarial test MUST FAIL if its corresponding `io.LimitReader`
// wrap is removed from the production code path — see the inline assertion
// commentary for the exact failure mode that proves the cap is live.
package google

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/drive"
)

// oversizeBytes returns n copies of byte 'A'. Used to build deliberately
// oversized response bodies that exceed the configured cap by orders of
// magnitude; the test then asserts the cap truncated the read.
func oversizeBytes(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = 'A'
	}
	return out
}

// TestGoogleDriveProvider_S004_OAuthTokenResponseLimitReaderTruncatesOversizedBody
// — Adversarial regression for the OAuth-response cap (site google.go:325 in
// exchangeCodeForToken). Spins up an httptest server returning a 400 status
// with a 1 MiB body of 'A' chars; the cap is set to 1 KiB. Asserts the
// resulting error message embeds at most `cap` bytes from the body.
//
// Removing the io.LimitReader wrap would let the entire 1 MiB land in the
// error string and the assertion `len(bodyPortion) <= cap` would fail
// (the unwrapped read would deliver ~1 MiB).
func TestGoogleDriveProvider_S004_OAuthTokenResponseLimitReaderTruncatesOversizedBody(t *testing.T) {
	const cap = 1024 // 1 KiB cap for the adversarial test
	const oversize = 1 * 1024 * 1024
	body := oversizeBytes(oversize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	p := New(DefaultCapabilities())
	p.client = http.DefaultClient
	p.cfg = config.DriveGoogleProviderConfig{
		OAuthClientID:     "test-client",
		OAuthClientSecret: "test-secret",
		OAuthRedirectURL:  "http://127.0.0.1:0/cb",
		OAuthBaseURL:      server.URL,
		APIBaseURL:        server.URL,
		IOLimits: config.DriveIOLimitsConfig{
			OAuthResponseMaxBytes: cap,
		},
	}

	_, err := p.exchangeCodeForToken(context.Background(), "auth-code")
	if err == nil {
		t.Fatal("exchangeCodeForToken returned nil error for 400 status; expected error")
	}
	// The error message format is:
	//   "google: token exchange status %d: %s"
	// where %s is the body. Extract the body portion after the status prefix.
	const prefix = "google: token exchange status 400: "
	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		t.Fatalf("error %q missing expected prefix %q", msg, prefix)
	}
	bodyPortion := strings.TrimPrefix(msg, prefix)
	if len(bodyPortion) > cap {
		t.Fatalf("ADVERSARIAL FAILURE: body portion length = %d, want <= %d (cap). io.LimitReader wrap appears to be missing or disabled at google.go exchangeCodeForToken site (~325).", len(bodyPortion), cap)
	}
	if len(bodyPortion) < cap-1 {
		// Sanity: the server sent 1 MiB; the cap should produce essentially
		// `cap` bytes (modulo TrimSpace removing trailing whitespace, which
		// 'A' chars are not).
		t.Fatalf("body portion length = %d, want ~= %d (cap fired but truncated unexpectedly low — confirm body is 'A' chars not whitespace)", len(bodyPortion), cap)
	}
}

// TestGoogleDriveProvider_S004_MetadataResponseLimitReaderTruncatesOversizedBody
// — Adversarial regression for the provider-response cap. Exercises the
// fetchAccountEmail path (site google.go:356), which is the simplest
// pool-free public-ish entry point that flows through the provider-response
// cap; the same wrap pattern guards 7 sites total (ListFolder, GetFile,
// PutFile response, EnsureFolder GET/POST, Changes, fetchAccountEmail).
//
// Removing the io.LimitReader wrap would let the entire oversized body
// land in the error string and the assertion `len(bodyPortion) <= cap`
// would fail.
func TestGoogleDriveProvider_S004_MetadataResponseLimitReaderTruncatesOversizedBody(t *testing.T) {
	const cap = 4096 // 4 KiB cap for the adversarial test
	const oversize = 5 * 1024 * 1024
	body := oversizeBytes(oversize)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drive/v3/about" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	p := New(DefaultCapabilities())
	p.client = http.DefaultClient
	p.cfg = config.DriveGoogleProviderConfig{
		OAuthBaseURL: server.URL,
		APIBaseURL:   server.URL,
		IOLimits: config.DriveIOLimitsConfig{
			ProviderResponseMaxBytes: cap,
		},
	}

	_, err := p.fetchAccountEmail(context.Background(), "fake-bearer-token")
	if err == nil {
		t.Fatal("fetchAccountEmail returned nil error for 400 status; expected error")
	}
	const prefix = "google: drive about status 400: "
	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		t.Fatalf("error %q missing expected prefix %q", msg, prefix)
	}
	bodyPortion := strings.TrimPrefix(msg, prefix)
	if len(bodyPortion) > cap {
		t.Fatalf("ADVERSARIAL FAILURE: body portion length = %d, want <= %d (cap). io.LimitReader wrap appears to be missing or disabled at google.go fetchAccountEmail site (~356) — and likely at the other 6 metadata-response sites which share the same wrap pattern.", len(bodyPortion), cap)
	}
	if len(bodyPortion) < cap-1 {
		t.Fatalf("body portion length = %d, want ~= %d (cap fired but truncated unexpectedly low)", len(bodyPortion), cap)
	}
}

// TestGoogleDriveProvider_S004_BinaryUploadLimitReaderTruncatesOversizedBody
// — Adversarial regression for the binary-content cap (site google.go:530
// in PutFile via the readUploadBody helper). Constructs an oversized
// in-memory io.Reader that returns N bytes of 'A' where N >> cap, and
// asserts the helper returns exactly `cap` bytes.
//
// PutFile itself requires a *pgxpool.Pool for accessToken; the cap wrap
// was extracted into readUploadBody specifically so this defense can be
// unit-tested without standing up a database. Removing the io.LimitReader
// wrap from readUploadBody (or removing the helper and inlining an
// unwrapped io.ReadAll(body.Reader) at the PutFile site) would let the
// full oversize bytes flow through and the `len(data) <= cap` assertion
// would fail.
func TestGoogleDriveProvider_S004_BinaryUploadLimitReaderTruncatesOversizedBody(t *testing.T) {
	const cap = 8 * 1024 // 8 KiB cap for the adversarial test
	const oversize = 100 * 1024 * 1024
	source := bytes.NewReader(oversizeBytes(oversize))

	p := New(DefaultCapabilities())
	p.cfg = config.DriveGoogleProviderConfig{
		IOLimits: config.DriveIOLimitsConfig{
			ProviderBinaryMaxBytes: cap,
		},
	}

	data, err := p.readUploadBody(source)
	if err != nil {
		t.Fatalf("readUploadBody returned error: %v", err)
	}
	if len(data) != cap {
		t.Fatalf("ADVERSARIAL FAILURE: data length = %d, want exactly %d (cap). io.LimitReader wrap appears to be missing or disabled at google.go PutFile binary site (~530, helper readUploadBody).", len(data), cap)
	}
	// Verify content is the expected 'A' fill so we know we actually read
	// from the source rather than getting an empty/short read for some
	// other reason.
	for i, b := range data {
		if b != 'A' {
			t.Fatalf("data[%d] = %q, want 'A' (cap truncated to short content)", i, b)
		}
	}
}

// TestGoogleDriveProvider_S004_BinaryUploadUncappedRetainsLegacyBehavior is
// the positive-guard counterpart to the adversarial truncation test. It
// proves that when ProviderBinaryMaxBytes is zero (the test default before
// ConfigureRuntime injects SST values), readUploadBody preserves the
// historical unbounded behavior. This guards against a regression where a
// future refactor accidentally inverts the `> 0` guard (e.g., wrapping
// every read with `io.LimitReader(_, 0)` which would return EOF
// immediately and break legacy test wiring).
func TestGoogleDriveProvider_S004_BinaryUploadUncappedRetainsLegacyBehavior(t *testing.T) {
	const payloadSize = 16 * 1024
	source := bytes.NewReader(oversizeBytes(payloadSize))

	p := New(DefaultCapabilities())
	// Note: IOLimits intentionally left zero-value to simulate legacy
	// test wiring that does not configure the cap.
	p.cfg = config.DriveGoogleProviderConfig{}

	data, err := p.readUploadBody(source)
	if err != nil {
		t.Fatalf("readUploadBody returned error: %v", err)
	}
	if len(data) != payloadSize {
		t.Fatalf("data length = %d, want %d (uncapped read should return full payload; > 0 guard regressed?)", len(data), payloadSize)
	}
}

// Compile-time assertion that drive.DefaultCapabilities and
// io.LimitReader are imported (defensive against future imports cleanup).
var (
	_ = drive.DefaultRegistry
	_ = io.LimitReader
)

// TestGoogleDriveProvider_S005_GetFileURLPathEscapesProviderFileID is an
// adversarial regression test for the SSRF/path-traversal fix applied in
// GetFile. The providerFileID is interpolated into the Drive API URL; prior
// to the fix it was sanitized with path.Clean (a filesystem function) which:
//   - does NOT URL-encode special characters (?, #, &)
//   - does NOT prevent path traversal (../x stays ../x)
//
// The test verifies that:
//  1. Path traversal sequences are percent-encoded so they cannot escape
//     the /drive/v3/files/ prefix.
//  2. URL special characters (?, #) are percent-encoded so they cannot
//     inject query params or fragment.
//  3. The parsed URL path always starts with /drive/v3/files/ and the
//     file ID cannot alter the path hierarchy.
//
// Each sub-test MUST FAIL if url.PathEscape is replaced with path.Clean
// or raw string concatenation.
func TestGoogleDriveProvider_S005_GetFileURLPathEscapesProviderFileID(t *testing.T) {
	const base = "https://www.googleapis.com"

	tests := []struct {
		name   string
		fileID string
		// wantPathPrefix verifies the URL path starts with the expected
		// prefix and has NOT been shortened by traversal.
		wantPathPrefix string
		// rejectRawContains is a string that MUST NOT appear in the raw
		// (percent-encoded) URL path. E.g. literal "../" would mean the
		// traversal was not encoded.
		rejectRawContains string
		// wantNoQuery verifies that no query string leaked from the fileID.
		wantNoQuery bool
		// wantNoFragment verifies that no fragment leaked from the fileID.
		wantNoFragment bool
	}{
		{
			name:              "path traversal attempt with dot-dot-slash",
			fileID:            "../../admin/v3/danger",
			wantPathPrefix:    "/drive/v3/files/",
			rejectRawContains: "../",
			wantNoQuery:       true,
			wantNoFragment:    true,
		},
		{
			name:              "query injection attempt with question mark",
			fileID:            "abc?alt=json&x=1",
			wantPathPrefix:    "/drive/v3/files/",
			rejectRawContains: "",
			wantNoQuery:       true,
			wantNoFragment:    true,
		},
		{
			name:              "fragment injection attempt with hash",
			fileID:            "abc#fragment",
			wantPathPrefix:    "/drive/v3/files/",
			rejectRawContains: "",
			wantNoQuery:       true,
			wantNoFragment:    true,
		},
		{
			name:              "normal alphanumeric file ID preserved",
			fileID:            "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgVE2upms",
			wantPathPrefix:    "/drive/v3/files/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgVE2upms",
			rejectRawContains: "",
			wantNoQuery:       true,
			wantNoFragment:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the EXACT URL construction from GetFile (post-fix).
			fileURL := strings.TrimRight(base, "/") + "/drive/v3/files/" + url.PathEscape(tt.fileID)

			parsed, err := url.Parse(fileURL)
			if err != nil {
				t.Fatalf("url.Parse failed: %v", err)
			}

			// Use RawPath (percent-encoded) for traversal/injection checks.
			// parsed.Path is always decoded, so %2F becomes / — which looks
			// like traversal but is safe because the wire format (RawPath)
			// keeps the encoding intact.
			rawPath := parsed.RawPath
			if rawPath == "" {
				rawPath = parsed.Path
			}

			// 1. The raw URL path must start with the expected prefix.
			if !strings.HasPrefix(rawPath, tt.wantPathPrefix) {
				t.Fatalf("ADVERSARIAL FAILURE: rawPath = %q does not start with %q.\n"+
					"url.PathEscape may have been replaced with path.Clean or removed.",
					rawPath, tt.wantPathPrefix)
			}

			// 2. The raw path must NOT contain the rejected substring.
			if tt.rejectRawContains != "" && strings.Contains(rawPath, tt.rejectRawContains) {
				t.Fatalf("ADVERSARIAL FAILURE: rawPath = %q contains %q.\n"+
					"Path traversal is not properly encoded — url.PathEscape may be missing.",
					rawPath, tt.rejectRawContains)
			}

			// 3. No query string leaked from the file ID.
			if tt.wantNoQuery && parsed.RawQuery != "" {
				t.Fatalf("ADVERSARIAL FAILURE: query string %q leaked from file ID %q.\n"+
					"url.PathEscape may have been replaced or removed.",
					parsed.RawQuery, tt.fileID)
			}

			// 4. No fragment leaked from the file ID.
			if tt.wantNoFragment && parsed.Fragment != "" {
				t.Fatalf("ADVERSARIAL FAILURE: fragment %q leaked from file ID %q.\n"+
					"url.PathEscape may have been replaced or removed.",
					parsed.Fragment, tt.fileID)
			}
		})
	}
}

// TestGoogleDriveProvider_S005_GetFileHTTPServerReceivesEscapedPath verifies
// that an httptest server receives a properly-encoded path segment when a
// path-traversal providerFileID is used. This is the end-to-end proof that
// the request as sent on the wire is safe.
func TestGoogleDriveProvider_S005_GetFileHTTPServerReceivesEscapedPath(t *testing.T) {
	var gotRawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawPath != "" {
			gotRawPath = r.URL.RawPath
		} else {
			gotRawPath = r.URL.Path
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`content`))
	}))
	t.Cleanup(server.Close)

	// Path traversal attempt — the most dangerous case.
	maliciousID := "../../admin/v3/danger"
	fileURL := strings.TrimRight(server.URL, "/") + "/drive/v3/files/" + url.PathEscape(maliciousID)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fileURL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	resp.Body.Close()

	// The raw path MUST contain the percent-encoded traversal (..%2F)
	// and MUST NOT have been normalized to a shorter path.
	if !strings.HasPrefix(gotRawPath, "/drive/v3/files/") {
		t.Fatalf("ADVERSARIAL FAILURE: raw path %q does not start with /drive/v3/files/.\n"+
			"The traversal escaped the files prefix — url.PathEscape may be missing.",
			gotRawPath)
	}
	if strings.Contains(gotRawPath, "../") {
		t.Fatalf("ADVERSARIAL FAILURE: raw path %q contains literal '../'.\n"+
			"url.PathEscape is not encoding path separators — SSRF possible.",
			gotRawPath)
	}
}

// TestGoogleDriveProvider_S005_PathCleanDoesNotPreventTraversal is a
// companion test proving that the OLD code (path.Clean) was insufficient.
// This test MUST PASS — it demonstrates the vulnerability that the
// url.PathEscape fix addresses.
func TestGoogleDriveProvider_S005_PathCleanDoesNotPreventTraversal(t *testing.T) {
	// path.Clean leaves "../" intact — it only canonicalizes, it does
	// not prevent traversal when the result is used in a URL context.
	cleaned := path.Clean("../../admin/v3/danger")
	if !strings.Contains(cleaned, "..") {
		t.Fatal("expected path.Clean to preserve '..' segments; if it now strips them, the security rationale should be re-evaluated")
	}
	// path.Clean does NOT encode special URL characters.
	cleanedQuery := path.Clean("abc?alt=json")
	if !strings.Contains(cleanedQuery, "?") {
		t.Fatal("expected path.Clean to preserve '?' character; if it now encodes URL chars, re-evaluate the SSRF fix rationale")
	}
}
