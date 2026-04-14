package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

// mockBookmarkPublisher records published artifacts.
type mockBookmarkPublisher struct {
	published []connector.RawArtifact
	err       error
}

func (m *mockBookmarkPublisher) PublishRawArtifact(_ context.Context, a connector.RawArtifact) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.published = append(m.published, a)
	return fmt.Sprintf("art-%d", len(m.published)), nil
}

func makeMultipartRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestBookmarkImportHandler_ChromeJSON(t *testing.T) {
	pub := &mockBookmarkPublisher{}
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		BookmarkPub: pub,
	}

	chromeJSON := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder", "name": "Bar",
				"children": [
					{"type": "url", "name": "Example", "url": "https://example.com"},
					{"type": "url", "name": "Go", "url": "https://go.dev"}
				]
			}
		}
	}`)

	req := makeMultipartRequest(t, "Bookmarks", chromeJSON)
	rec := httptest.NewRecorder()
	deps.BookmarkImportHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BookmarkImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Imported != 2 {
		t.Errorf("expected 2 imported, got %d", resp.Imported)
	}
	if len(pub.published) != 2 {
		t.Errorf("expected 2 published artifacts, got %d", len(pub.published))
	}
}

func TestBookmarkImportHandler_NetscapeHTML(t *testing.T) {
	pub := &mockBookmarkPublisher{}
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		BookmarkPub: pub,
	}

	netscapeHTML := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><H3>Reading</H3>
<DL>
<DT><A HREF="https://example.com/article">Article</A>
</DL>
<DT><A HREF="https://go.dev">Go</A>
</DL>`)

	req := makeMultipartRequest(t, "bookmarks.html", netscapeHTML)
	rec := httptest.NewRecorder()
	deps.BookmarkImportHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BookmarkImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Imported != 2 {
		t.Errorf("expected 2 imported, got %d", resp.Imported)
	}
}

func TestBookmarkImportHandler_UnsupportedFormat(t *testing.T) {
	deps := &Dependencies{
		DB:   &mockDB{healthy: true},
		NATS: &mockNATS{healthy: true},
	}

	req := makeMultipartRequest(t, "random.txt", []byte("not a bookmark file at all"))
	rec := httptest.NewRecorder()
	deps.BookmarkImportHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Error.Code != "UNSUPPORTED_FORMAT" {
		t.Errorf("expected UNSUPPORTED_FORMAT, got %q", resp.Error.Code)
	}
}

func TestBookmarkImportHandler_EmptyFile(t *testing.T) {
	deps := &Dependencies{
		DB:   &mockDB{healthy: true},
		NATS: &mockNATS{healthy: true},
	}

	req := makeMultipartRequest(t, "empty.json", []byte(""))
	rec := httptest.NewRecorder()
	deps.BookmarkImportHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBookmarkImportHandler_MissingFile(t *testing.T) {
	deps := &Dependencies{
		DB:   &mockDB{healthy: true},
		NATS: &mockNATS{healthy: true},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks/import",
		bytes.NewBufferString("not multipart"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	deps.BookmarkImportHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBookmarkImportHandler_NoPublisher(t *testing.T) {
	deps := &Dependencies{
		DB:   &mockDB{healthy: true},
		NATS: &mockNATS{healthy: true},
		// BookmarkPub is nil — should still return parsed count
	}

	chromeJSON := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder", "name": "Bar",
				"children": [
					{"type": "url", "name": "Test", "url": "https://test.com"}
				]
			}
		}
	}`)

	req := makeMultipartRequest(t, "Bookmarks", chromeJSON)
	rec := httptest.NewRecorder()
	deps.BookmarkImportHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BookmarkImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Imported != 1 {
		t.Errorf("expected 1 imported, got %d", resp.Imported)
	}
}

func TestBookmarkImportHandler_PublishError(t *testing.T) {
	pub := &mockBookmarkPublisher{err: fmt.Errorf("db connection refused")}
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		BookmarkPub: pub,
	}

	chromeJSON := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder", "name": "Bar",
				"children": [
					{"type": "url", "name": "Test", "url": "https://test.com"}
				]
			}
		}
	}`)

	req := makeMultipartRequest(t, "Bookmarks", chromeJSON)
	rec := httptest.NewRecorder()
	deps.BookmarkImportHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (with errors), got %d", rec.Code)
	}

	var resp BookmarkImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Imported != 0 {
		t.Errorf("expected 0 imported, got %d", resp.Imported)
	}
	if len(resp.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(resp.Errors))
	}
}
