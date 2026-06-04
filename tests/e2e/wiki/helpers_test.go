//go:build e2e

// Spec 073 SCOPE-073-05 wiki browse surface e2e helpers.
// Mirrors the lightweight helper shape used by tests/e2e/drive and
// the top-level e2e package; subpackage isolation keeps wiki test
// state self-contained.
package wiki

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type e2eConfig struct {
	CoreURL   string
	AuthToken string
	DBURL     string
}

func loadE2EConfig(t *testing.T) e2eConfig {
	t.Helper()
	core := os.Getenv("CORE_EXTERNAL_URL")
	if core == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set")
	}
	auth := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if auth == "" {
		t.Skip("e2e: SMACKEREL_AUTH_TOKEN not set")
	}
	return e2eConfig{
		CoreURL:   core,
		AuthToken: auth,
		DBURL:     os.Getenv("DATABASE_URL"),
	}
}

func waitForHealth(t *testing.T, cfg e2eConfig, maxWait time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, err := client.Get(cfg.CoreURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("e2e: services not healthy after %s at %s", maxWait, cfg.CoreURL)
}

func getText(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", url, resp.StatusCode, string(body))
	}
	return string(body)
}

func apiGetJSON(t *testing.T, cfg e2eConfig, path string, out any) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest %s: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("decode %s: %v body=%s", path, err, string(body))
		}
	}
	return resp, body
}

func mustContain(t *testing.T, label, body string, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if !strings.Contains(body, f) {
			t.Fatalf("%s missing fragment %q", label, f)
		}
	}
}

func newPrefix(label string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "wiki-e2e-" + label + "-" + hex.EncodeToString(buf)
}

func connectDB(t *testing.T, cfg e2eConfig) *pgx.Conn {
	t.Helper()
	if cfg.DBURL == "" {
		t.Skip("e2e: DATABASE_URL not set — wiki e2e needs Postgres to seed fixtures")
	}
	conn, err := pgx.Connect(context.Background(), cfg.DBURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	return conn
}

type crossLink struct {
	TargetKind  string `json:"targetKind"`
	TargetID    string `json:"targetId"`
	TargetLabel string `json:"targetLabel"`
	Reason      string `json:"reason"`
}
