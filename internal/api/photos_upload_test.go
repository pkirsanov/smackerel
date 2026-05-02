package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosUpload_PreservesSourceAndProviderRefs covers the upload
// validation contract for SCN-040-010 — the unified pipeline MUST
// accept Telegram/mobile/web channels, reject the reserved
// `provider` channel, require a source_ref, and only allow the
// `single` and `document` modes. The test exercises the handler with
// an unconfigured store so we focus on validation boundaries; the
// integration test owns the full multipart→DB→pipeline assertion.
func TestPhotosUpload_PreservesSourceAndProviderRefs(t *testing.T) {
	tests := []struct {
		name           string
		fields         map[string]string
		filename       string
		body           []byte
		wantStatus     int
		wantErrorCode  string
		wantErrorMatch string
	}{
		{
			name: "missing source_channel rejected",
			fields: map[string]string{
				"source_ref": "tg:1:1",
			},
			filename:      "x.jpg",
			body:          []byte("\x89PNG"),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_source_channel",
		},
		{
			name: "provider channel reserved",
			fields: map[string]string{
				"source_channel": "provider",
				"source_ref":     "internal:1",
			},
			filename:      "x.jpg",
			body:          []byte("\x89PNG"),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_source_channel",
		},
		{
			name: "missing source_ref rejected",
			fields: map[string]string{
				"source_channel": "telegram",
			},
			filename:      "x.jpg",
			body:          []byte("\x89PNG"),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_source_ref",
		},
		{
			name: "invalid mode rejected",
			fields: map[string]string{
				"source_channel": "mobile",
				"source_ref":     "device:1",
				"mode":           "weird",
			},
			filename:      "x.jpg",
			body:          []byte("\x89PNG"),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_mode",
		},
		{
			name: "document mode requires group id",
			fields: map[string]string{
				"source_channel": "mobile",
				"source_ref":     "device:1",
				"mode":           "document",
			},
			filename:      "x.jpg",
			body:          []byte("\x89PNG"),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_document_group",
		},
		{
			name: "non-positive page index rejected",
			fields: map[string]string{
				"source_channel":      "web",
				"source_ref":          "session:42",
				"mode":                "document",
				"document_group_id":   "scan-1",
				"document_page_index": "0",
			},
			filename:      "x.jpg",
			body:          []byte("\x89PNG"),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_page_index",
		},
		{
			name: "missing file part rejected",
			fields: map[string]string{
				"source_channel": "telegram",
				"source_ref":     "tg:1:1",
			},
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "missing_file",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, contentType := buildMultipartUpload(t, tc.fields, tc.filename, tc.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/photos/upload", body)
			req.Header.Set("Content-Type", contentType)
			req.Header.Set("X-Actor-Id", "test-actor")
			rec := httptest.NewRecorder()
			handlers := &PhotosHandlers{}
			handlers.Upload(rec, req)
			if got := rec.Result().StatusCode; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", got, tc.wantStatus, rec.Body.String())
			}
			if tc.wantErrorCode != "" {
				if !strings.Contains(rec.Body.String(), tc.wantErrorCode) {
					t.Fatalf("body missing error code %q: %s", tc.wantErrorCode, rec.Body.String())
				}
			}
		})
	}
}

// TestPhotosUpload_HelperFunctionsCarryChannelMetadata anchors the
// helpers used by the multipart handler. They MUST embed the channel
// and source ref so audit replays can prove every upload kept its
// origin metadata (FR-019, SCN-040-010).
func TestPhotosUpload_HelperFunctionsCarryChannelMetadata(t *testing.T) {
	ref := buildProviderRef(photolib.SourceChannelTelegram, "tg:42:99", "", 0)
	if !strings.HasPrefix(ref, "telegram:upload:tg:42:99:") {
		t.Fatalf("provider_ref must encode channel + source ref, got %q", ref)
	}
	docRef := buildProviderRef(photolib.SourceChannelWeb, "session:1", "scan-XYZ", 3)
	if docRef != "web:doc:scan-XYZ:p3" {
		t.Fatalf("document provider_ref shape drift: %q", docRef)
	}
	hash := contentHashForUpload("internal", ref, []byte("payload"))
	if !strings.HasPrefix(hash, "sha256:upload:internal:") {
		t.Fatalf("content hash must include upload provenance, got %q", hash)
	}
	if got := fallbackFilename("", "telegram:upload:abc", "image/jpeg"); got == "" {
		t.Fatalf("fallback filename must always return non-empty value")
	}
	if got := fallbackFilename("photo.jpg", "ref", "image/jpeg"); got != "photo.jpg" {
		t.Fatalf("fallback filename should preserve provided name, got %q", got)
	}
}

// TestPhotosUpload_ResponseSchemaStable guards against silent shape
// drift in the API contract. The DTO is consumed by Telegram, the
// PWA, and the agent tools; renaming a field would silently break
// each.
func TestPhotosUpload_ResponseSchemaStable(t *testing.T) {
	resp := PhotoUploadResponse{
		PhotoID:         "photo-1",
		ArtifactID:      "art-1",
		ConnectorID:     "photos-upload-mobile",
		Provider:        "internal",
		ProviderRef:     "mobile:upload:device:1:abc",
		SourceChannel:   photolib.SourceChannelMobile,
		SourceRef:       "device:1",
		DocumentGroupID: "doc-1",
		PageIndex:       2,
		Pipeline: photoUploadPipelineEcho{
			Name:   "photos.unified_pipeline",
			Stages: []string{"persist", "classify", "routing", "sensitivity_gate"},
		},
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	for _, want := range []string{
		`"photo_id":"photo-1"`,
		`"artifact_id":"art-1"`,
		`"source_channel":"mobile"`,
		`"document_group_id":"doc-1"`,
		`"document_page_index":2`,
		`"stages":["persist","classify","routing","sensitivity_gate"]`,
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("response shape missing %q: %s", want, encoded)
		}
	}
}

func buildMultipartUpload(t *testing.T, fields map[string]string, filename string, contents []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(contents); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return buf, writer.FormDataContentType()
}
