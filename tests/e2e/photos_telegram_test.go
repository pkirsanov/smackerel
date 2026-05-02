//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestPhotosTelegram_E2E_UploadClassifySearchAndRetrieve covers
// SCN-040-010 against the live stack. The Telegram, mobile, and web
// channels MUST share one upload pipeline: each upload returns 201
// with the channel + source_ref preserved AND the photo MUST be
// retrievable through GET /v1/photos/{id} with the same channel
// metadata. The retrieval surface confirms the unified pipeline did
// not strip or rewrite the inbound channel data.
func TestPhotosTelegram_E2E_UploadClassifySearchAndRetrieve(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	pool := photosE2EPool(t)
	uniqueRef := "e2e-upload-" + uuid.NewString()
	for _, channel := range []string{"telegram", "mobile", "web"} {
		channel := channel
		t.Run(channel, func(t *testing.T) {
			sourceRef := channel + ":" + uniqueRef
			resp := uploadPhoto(t, cfg, uploadFields{
				channel:   channel,
				sourceRef: sourceRef,
				filename:  channel + "-page.jpg",
				contents:  syntheticJPEG(channel + uniqueRef),
			})
			if resp.SourceChannel != channel {
				t.Fatalf("source_channel echoed = %q, want %q", resp.SourceChannel, channel)
			}
			if resp.SourceRef != sourceRef {
				t.Fatalf("source_ref echoed = %q, want %q", resp.SourceRef, sourceRef)
			}
			if resp.PhotoID == "" || resp.ArtifactID == "" {
				t.Fatalf("missing ids in upload response: %+v", resp)
			}

			cleanupE2EPhoto(t, pool, resp.ArtifactID)

			detail, err := apiGet(cfg, "/v1/photos/"+resp.PhotoID)
			if err != nil {
				t.Fatalf("get photo: %v", err)
			}
			body, err := readBody(detail)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if detail.StatusCode != http.StatusOK {
				t.Fatalf("photo detail status=%d body=%s", detail.StatusCode, body)
			}
			if !strings.Contains(string(body), `"source_channel":"`+channel+`"`) {
				t.Fatalf("photo detail missing source_channel %q: %s", channel, body)
			}
			if !strings.Contains(string(body), `"source_ref":"`+sourceRef+`"`) {
				t.Fatalf("photo detail missing source_ref %q: %s", sourceRef, body)
			}
		})
	}
}

// uploadFields + uploadResponse + helpers shared with the routing and
// sensitivity e2e files.
type uploadFields struct {
	channel       string
	sourceRef     string
	mode          string
	documentGroup string
	documentPage  int
	filename      string
	contents      []byte
}

type uploadResponse struct {
	PhotoID         string `json:"photo_id"`
	ArtifactID      string `json:"artifact_id"`
	SourceChannel   string `json:"source_channel"`
	SourceRef       string `json:"source_ref"`
	DocumentGroupID string `json:"document_group_id,omitempty"`
	PageIndex       int    `json:"document_page_index,omitempty"`
}

func uploadPhoto(t *testing.T, cfg e2eConfig, fields uploadFields) uploadResponse {
	t.Helper()
	body, contentType := buildE2EMultipart(t, fields)
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/v1/photos/upload", body)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", contentType)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	respBody, err := readBody(resp)
	if err != nil {
		t.Fatalf("read upload body: %v", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", resp.StatusCode, respBody)
	}
	var out uploadResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		t.Fatalf("decode upload response: %v body=%s", err, respBody)
	}
	return out
}

func buildE2EMultipart(t *testing.T, fields uploadFields) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	if err := writer.WriteField("source_channel", fields.channel); err != nil {
		t.Fatalf("write source_channel: %v", err)
	}
	if err := writer.WriteField("source_ref", fields.sourceRef); err != nil {
		t.Fatalf("write source_ref: %v", err)
	}
	if fields.mode != "" {
		if err := writer.WriteField("mode", fields.mode); err != nil {
			t.Fatalf("write mode: %v", err)
		}
	}
	if fields.documentGroup != "" {
		if err := writer.WriteField("document_group_id", fields.documentGroup); err != nil {
			t.Fatalf("write document_group_id: %v", err)
		}
		if fields.documentPage > 0 {
			if err := writer.WriteField("document_page_index", fmt.Sprintf("%d", fields.documentPage)); err != nil {
				t.Fatalf("write document_page_index: %v", err)
			}
		}
	}
	part, err := writer.CreateFormFile("file", fields.filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(fields.contents)); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return buf, writer.FormDataContentType()
}

// syntheticJPEG returns a minimal JPEG-shaped byte slice. The bytes
// are intentionally NOT a real image — the upload endpoint only
// validates size + content-type metadata; downstream classification
// in the live stack is mocked at the model boundary for this scope.
func syntheticJPEG(seed string) []byte {
	header := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
	footer := []byte{0xFF, 0xD9}
	body := []byte("smackerel-e2e-fixture:" + seed)
	out := make([]byte, 0, len(header)+len(body)+len(footer))
	out = append(out, header...)
	out = append(out, body...)
	out = append(out, footer...)
	return out
}
