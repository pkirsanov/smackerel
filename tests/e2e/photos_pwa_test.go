//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	for _, page := range []string{"photo-libraries.html", "photo-library-add.html"} {
		body := getE2EText(t, cfg.CoreURL+"/pwa/"+page)
		for _, expected := range []string{"Photo Libraries", "role=\"status\"", "/v1/photos/connectors"} {
			if !strings.Contains(body, expected) {
				t.Fatalf("%s missing %q", page, expected)
			}
		}
	}

	addJS := getE2EText(t, cfg.CoreURL+"/pwa/photo-library-add.js")
	for _, expected := range []string{"/v1/photos/connectors/test", "/v1/photos/connectors", `credentials: "same-origin"`, "included_albums"} {
		if !strings.Contains(addJS, expected) {
			t.Fatalf("photo-library-add.js missing %q", expected)
		}
	}
	if strings.Contains(addJS, `credentials: "omit"`) {
		t.Fatal("photo-library-add.js must not omit the same-origin auth cookie")
	}

	resp, err := apiGet(cfg, "/v1/photos/connectors")
	if err != nil {
		t.Fatalf("GET /v1/photos/connectors: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read connectors body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connectors status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Connectors []struct {
			Provider string `json:"provider"`
			Status   string `json:"status"`
		} `json:"connectors"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode connectors body: %v body=%s", err, string(body))
	}
	if len(parsed.Connectors) == 0 || parsed.Connectors[0].Provider != "immich" {
		t.Fatalf("connectors response missing Immich provider: %s", string(body))
	}
}

func TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	for _, page := range []string{"photo-library-detail.html", "photo-search.html", "photo-detail.html"} {
		body := getE2EText(t, cfg.CoreURL+"/pwa/"+page)
		for _, expected := range []string{"Photo", "role=\"status\"", "/v1/photos/"} {
			if !strings.Contains(body, expected) {
				t.Fatalf("%s missing %q", page, expected)
			}
		}
	}

	detailJS := getE2EText(t, cfg.CoreURL+"/pwa/photo-library-detail.js")
	for _, expected := range []string{"progress", "skips", "retry_token", "/v1/photos/connectors/"} {
		if !strings.Contains(detailJS, expected) {
			t.Fatalf("photo-library-detail.js missing %q", expected)
		}
	}
	searchJS := getE2EText(t, cfg.CoreURL+"/pwa/photo-search.js")
	for _, expected := range []string{"/v1/photos/search", "ocr_snippet", "match_confidence"} {
		if !strings.Contains(searchJS, expected) {
			t.Fatalf("photo-search.js missing %q", expected)
		}
	}
}

func getE2EText(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", url, resp.StatusCode, string(body))
	}
	return string(body)
}
