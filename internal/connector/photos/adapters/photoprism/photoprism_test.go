package photoprism

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotoprismAdapterMapsProviderMediaToPhotoEvent proves the
// adapter normalises the PhotoPrism on-the-wire shape into the
// provider-neutral PhotoEvent without leaking provider-specific keys.
func TestPhotoprismAdapterMapsProviderMediaToPhotoEvent(t *testing.T) {
	photo := samplePhoto("vacation-001", "album-vacation", "Vacation 2026")
	event, skip, err := MapPhoto(photo)
	if err != nil {
		t.Fatalf("MapPhoto: %v", err)
	}
	if skip != nil {
		t.Fatalf("unexpected skip: %+v", skip)
	}
	if event.ProviderRef != photo.UID {
		t.Fatalf("ProviderRef = %q, want %q", event.ProviderRef, photo.UID)
	}
	if event.Operation != photolib.PhotoOpUpsert {
		t.Fatalf("Operation = %q, want upsert", event.Operation)
	}
	if event.MIMEType != photo.MIME {
		t.Fatalf("MIMEType = %q, want %q", event.MIMEType, photo.MIME)
	}
	if event.ContentHash != "sha1:"+photo.Hash {
		t.Fatalf("ContentHash = %q, want sha1:%s", event.ContentHash, photo.Hash)
	}
	if event.MediaRole != photolib.MediaRoleCameraOriginal {
		t.Fatalf("MediaRole = %q, want camera_original", event.MediaRole)
	}
	if event.RawProvider["provider"] != providerName {
		t.Fatalf("RawProvider provider = %v, want %q", event.RawProvider["provider"], providerName)
	}
	if event.Sensitivity.Source != "photoprism:inferred-locally" {
		t.Fatalf("Sensitivity.Source = %q, want photoprism:inferred-locally", event.Sensitivity.Source)
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("PhotoEvent.Validate failed: %v", err)
	}
}

// TestPhotoprismAdapterEnumerateScopeExcludesAlbums proves the
// album-scope filter respects the user's exclusion list.
func TestPhotoprismAdapterEnumerateScopeExcludesAlbums(t *testing.T) {
	included := samplePhoto("public-001", "album-public", "Public")
	excluded := samplePhoto("private-001", "album-private", "Private")
	fixture := newPhotoprismFixtureServer(t, photoprismFixtureData{Photos: []Photo{included, excluded}})
	client := NewClient(fixture.Client())
	connectFixture(t, client, fixture)

	events, errs := client.EnumerateScope(context.Background(), photolib.Scope{
		IncludedAlbums: []string{"album-public"},
		ExcludedAlbums: []string{"album-private"},
	})
	collected := []photolib.PhotoEvent{}
	for event := range events {
		collected = append(collected, event)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("EnumerateScope error: %v", err)
		}
	}
	if len(collected) != 1 {
		t.Fatalf("collected %d events, want 1: %+v", len(collected), collected)
	}
	if collected[0].ProviderRef != "public-001" {
		t.Fatalf("collected ProviderRef = %q, want public-001", collected[0].ProviderRef)
	}
}

// TestPhotoprismWriterEnforcesCapabilityLimitation proves the writer
// returns a typed `ProviderLimitationError` for capabilities the
// provider marks unsupported. The API layer relies on the typed error
// to translate into 409 PROVIDER_LIMITATION.
func TestPhotoprismWriterEnforcesCapabilityLimitation(t *testing.T) {
	fixture := newPhotoprismFixtureServer(t, photoprismFixtureData{})
	client := NewClient(fixture.Client())
	connectFixture(t, client, fixture)

	writer := client.Writer()
	if writer == nil {
		t.Fatalf("Writer returned nil")
	}
	err := writer.RenameFaceCluster(context.Background(), "subj-001", "Maria")
	if err == nil {
		t.Fatalf("RenameFaceCluster expected ProviderLimitationError, got nil")
	}
	var limit *ProviderLimitationError
	if !errors.As(err, &limit) {
		t.Fatalf("RenameFaceCluster error type = %T, want *ProviderLimitationError: %v", err, err)
	}
	if limit.LimitationCode != photolib.LimitationFacesWriteNotSupported {
		t.Fatalf("LimitationCode = %q, want %q", limit.LimitationCode, photolib.LimitationFacesWriteNotSupported)
	}
	if limit.Capability != photolib.CapFacesWrite {
		t.Fatalf("Capability = %q, want %q", limit.Capability, photolib.CapFacesWrite)
	}
	if limit.Provider != providerName {
		t.Fatalf("Provider = %q, want %q", limit.Provider, providerName)
	}

	// Adversarial: writer methods backed by SUPPORTED capabilities
	// MUST NOT return the limitation error. AddToAlbum is supported
	// for PhotoPrism — the fixture server returns 2xx for that path.
	if err := writer.AddToAlbum(context.Background(), "vacation-001", "album-vacation"); err != nil {
		t.Fatalf("AddToAlbum returned unexpected error for supported capability: %v", err)
	}
}

func connectFixture(t *testing.T, client *Client, fixture *photoprismFixtureServer) {
	t.Helper()
	config := connector.ConnectorConfig{
		AuthType:     "api_token",
		Credentials:  map[string]string{"api_token": fixture.APIToken()},
		SourceConfig: map[string]any{"base_url": fixture.URL()},
	}
	if err := client.Connect(context.Background(), config); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if status := client.Health(context.Background()); status != connector.HealthHealthy {
		t.Fatalf("Health = %q, want healthy", status)
	}
}

// TestPhotoprismCapabilityProbeUsesLimitedSurface proves the capability
// report for PhotoPrism marks faces_write UNSUPPORTED and sensitivity
// LIMITED with the canonical limitation codes.
func TestPhotoprismCapabilityProbeUsesLimitedSurface(t *testing.T) {
	fixture := newPhotoprismFixtureServer(t, photoprismFixtureData{})
	client := NewClient(fixture.Client())
	connectFixture(t, client, fixture)

	report := client.Capabilities()
	if report.Provider != providerName {
		t.Fatalf("report.Provider = %q, want %q", report.Provider, providerName)
	}
	facesWrite, ok := report.Capabilities[photolib.CapFacesWrite]
	if !ok {
		t.Fatalf("capability %q missing from report", photolib.CapFacesWrite)
	}
	if facesWrite.Status != photolib.CapabilityUnsupported {
		t.Fatalf("faces_write.Status = %q, want unsupported", facesWrite.Status)
	}
	if facesWrite.LimitationCode != string(photolib.LimitationFacesWriteNotSupported) {
		t.Fatalf("faces_write.LimitationCode = %q, want %q", facesWrite.LimitationCode, photolib.LimitationFacesWriteNotSupported)
	}
	sensitivity := report.Capabilities[photolib.CapSensitivity]
	if sensitivity.Status != photolib.CapabilityLimited {
		t.Fatalf("sensitivity.Status = %q, want limited", sensitivity.Status)
	}
	if sensitivity.LimitationCode != string(photolib.LimitationSensitivityNotInferred) {
		t.Fatalf("sensitivity.LimitationCode = %q, want %q", sensitivity.LimitationCode, photolib.LimitationSensitivityNotInferred)
	}

	// Sanity guard: capabilities the writer USES are still
	// SUPPORTED so AddToAlbum/Tag/Favorite/Archive/Delete don't
	// false-positive on the limitation check.
	for _, capability := range []photolib.Capability{photolib.CapWriteAlbum, photolib.CapWriteTag, photolib.CapWriteFavorite, photolib.CapArchive, photolib.CapDelete, photolib.CapUpload, photolib.CapRead, photolib.CapMonitor, photolib.CapFacesRead} {
		entry, ok := report.Capabilities[capability]
		if !ok {
			t.Fatalf("expected capability %q in report", capability)
		}
		if entry.Status != photolib.CapabilitySupported {
			t.Fatalf("capability %q status = %q, want supported", capability, entry.Status)
		}
	}
}

// TestPhotoprismRequestUsesXSessionIDHeader proves the adapter sends
// the api_token in the standard PhotoPrism header. Drift here would
// silently cause auth failures in production.
func TestPhotoprismRequestUsesXSessionIDHeader(t *testing.T) {
	request, err := buildPhotoprismRequest(context.Background(), "https://photoprism.example", "fixture-token", http.MethodGet, "/api/v1/photos", nil)
	if err != nil {
		t.Fatalf("buildPhotoprismRequest: %v", err)
	}
	if got := request.Header.Get("X-Session-ID"); got != "fixture-token" {
		t.Fatalf("X-Session-ID header = %q, want fixture-token", got)
	}
}

// TestPhotoprismUpload_LimitReaderTruncatesOversizedSource is the
// adversarial regression for MIT-040-S-006 on the photoprism Upload
// path.
//
// The test sets `uploadMaxBytes = 1024` and feeds the writer a 16 KiB
// `io.Reader`. With the LimitReader wrap in place, the JSON body
// posted to /api/v1/upload contains exactly 1024 bytes in the `bytes`
// field (LimitReader silently truncates after `max` bytes). Without
// the wrap, the body would contain all 16 KiB.
//
// If the LimitReader call inside (writer).Upload is removed, this
// test fails with `body bytes = 16384, want 1024`.
func TestPhotoprismUpload_LimitReaderTruncatesOversizedSource(t *testing.T) {
	const cap = int64(1024)
	const sourceLen = 16 * 1024

	var capturedBytes []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/server":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "v240601-test"})
			return
		case "/api/v1/upload":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read upload body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var parsed struct {
				Bytes []byte `json:"bytes"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Errorf("decode upload body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			capturedBytes = parsed.Bytes
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"UID": "photo-test"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.Client())
	client.SetUploadMaxBytes(cap)
	if err := client.Connect(context.Background(), connector.ConnectorConfig{
		AuthType:     "api_token",
		Credentials:  map[string]string{"api_token": "test"},
		SourceConfig: map[string]any{"base_url": server.URL},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	src := bytes.NewReader(make([]byte, sourceLen))
	if _, err := client.Writer().Upload(context.Background(), src, photolib.UploadMeta{
		Filename: "test.jpg",
		MIMEType: "image/jpeg",
	}); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if int64(len(capturedBytes)) != cap {
		t.Fatalf("body bytes = %d, want %d (LimitReader truncation not honored)", len(capturedBytes), cap)
	}
}
