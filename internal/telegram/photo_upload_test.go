package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDownloadTelegramFile_LimitReaderTruncatesOversizedBody is the
// adversarial regression for MIT-040-S-006 covering
// (b *Bot).downloadTelegramFile at internal/telegram/photo_upload.go.
//
// The test stands up an httptest server that returns 16 KiB of bytes
// for a Telegram-file URL, configures the Bot with a 1 KiB cap via
// `photoDownloadMaxBytes`, and asserts the returned body is truncated
// to exactly 1 KiB (LimitReader semantics).
//
// If the io.LimitReader wrap is removed from downloadTelegramFile,
// this test fails with `body length = 16384, want 1024`.
func TestDownloadTelegramFile_LimitReaderTruncatesOversizedBody(t *testing.T) {
	const cap = int64(1024)
	const responseLen = 16 * 1024

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(make([]byte, responseLen))
	}))
	t.Cleanup(server.Close)

	bot := &Bot{
		httpClient:            &http.Client{Timeout: 5 * time.Second},
		photoDownloadMaxBytes: cap,
	}

	body, contentType, err := bot.downloadTelegramFile(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("downloadTelegramFile: %v", err)
	}
	if int64(len(body)) != cap {
		t.Fatalf("body length = %d, want %d (LimitReader truncation not honored)", len(body), cap)
	}
	if contentType != "image/jpeg" {
		t.Fatalf("contentType = %q, want image/jpeg", contentType)
	}
}

// TestPostPhotoUpload_LimitReaderTruncatesOversizedResponse is the
// adversarial regression for MIT-040-S-006 covering
// (b *Bot).postPhotoUpload at internal/telegram/photo_upload.go:152.
//
// The test stands up an httptest server that returns a JSON body
// MUCH larger than the configured cap. With the LimitReader wrap in
// place, ReadAll truncates the response to `uploadResponseMaxBytes`
// bytes, after which JSON decoding fails (truncated payload). Without
// the wrap, the full response is read and JSON decoding succeeds.
//
// If the io.LimitReader wrap is removed from postPhotoUpload, this
// test fails because Unmarshal of the full response would succeed.
func TestPostPhotoUpload_LimitReaderTruncatesOversizedResponse(t *testing.T) {
	const cap = int64(64) // intentionally tiny so JSON parse fails after truncation

	// Construct a server response substantially larger than the cap.
	// The response is valid JSON in full but invalid after truncation.
	padding := strings.Repeat("x", 4096)
	fullResponse := []byte(`{"photo_id":"p-test","artifact_id":"a-test","padding":"` + padding + `"}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/photos/upload" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(fullResponse)
	}))
	t.Cleanup(server.Close)

	bot := &Bot{
		httpClient:             &http.Client{Timeout: 5 * time.Second},
		baseURL:                server.URL,
		uploadResponseMaxBytes: cap,
	}

	_, err := bot.postPhotoUpload(context.Background(), telegramPhotoUploadRequest{
		Filename:    "telegram-test.jpg",
		ContentType: "image/jpeg",
		File:        []byte("fake image bytes"),
		Channel:     "telegram",
		SourceRef:   "1:2",
	})
	if err == nil {
		t.Fatalf("expected decode error after LimitReader truncation, got nil — LimitReader not honored")
	}
	if !strings.Contains(err.Error(), "decode upload response") &&
		!strings.Contains(err.Error(), "missing photo_id") {
		t.Fatalf("error = %v, want a decode/missing-id failure caused by truncation", err)
	}

	// Sanity: confirm the unwrapped server response is itself well-formed
	// — i.e., the failure above is from truncation, NOT from a malformed
	// fixture. This makes the test diagnostically clear: removing the
	// LimitReader wrap would let postPhotoUpload read the full body and
	// succeed.
	var sane struct {
		PhotoID    string `json:"photo_id"`
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.Unmarshal(fullResponse, &sane); err != nil || sane.PhotoID == "" {
		t.Fatalf("fixture response itself is malformed (sanity guard): %v / %+v", err, sane)
	}
}

// TestPostPhotoUpload_MultipartFormStillWorksUnderCap is a positive
// guard that the LimitReader wrap does NOT regress healthy responses
// — i.e., a small response that fits inside the cap is decoded
// normally. This pairs with the truncation test above so the cap can
// be tuned without silently breaking healthy uploads.
func TestPostPhotoUpload_MultipartFormStillWorksUnderCap(t *testing.T) {
	const cap = int64(10 * 1024) // generous cap
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sanity-check the multipart form arrives intact (covers the
		// regression where LimitReader is mistakenly applied to the
		// REQUEST body instead of the RESPONSE body).
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("source_channel") != "telegram" {
			t.Errorf("source_channel = %q, want telegram", r.FormValue("source_channel"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"photo_id":"p-good","artifact_id":"a-good"}`))
	}))
	t.Cleanup(server.Close)

	bot := &Bot{
		httpClient:             &http.Client{Timeout: 5 * time.Second},
		baseURL:                server.URL,
		uploadResponseMaxBytes: cap,
	}

	resp, err := bot.postPhotoUpload(context.Background(), telegramPhotoUploadRequest{
		Filename:    "telegram-test.jpg",
		ContentType: "image/jpeg",
		File:        []byte("fake image bytes"),
		Channel:     "telegram",
		SourceRef:   "1:2",
	})
	if err != nil {
		t.Fatalf("postPhotoUpload (under cap): %v", err)
	}
	if resp.PhotoID != "p-good" {
		t.Fatalf("PhotoID = %q, want p-good", resp.PhotoID)
	}
}
