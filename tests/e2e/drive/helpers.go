//go:build e2e

package drive

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// e2eConfig holds live-stack connection details resolved from environment.
type e2eConfig struct {
	CoreURL   string
	AuthToken string
}

// loadE2EConfig reads live-stack connection details from environment.
// Mirrors the helpers in tests/e2e/browser_history_e2e_test.go so the
// drive e2e suite stays self-contained.
func loadE2EConfig(t *testing.T) e2eConfig {
	t.Helper()
	coreURL := os.Getenv("CORE_EXTERNAL_URL")
	if coreURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	return e2eConfig{CoreURL: coreURL, AuthToken: authToken}
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

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
