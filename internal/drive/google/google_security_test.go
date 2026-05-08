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
