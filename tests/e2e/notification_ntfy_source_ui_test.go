//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
	ntfysource "github.com/smackerel/smackerel/internal/notification/source/ntfy"
)

func TestNtfyOperatorWorkflowSourceListDetailDLQReplayTroubleshooting(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	validResp, err := apiPostRaw(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", []byte(`{"id":"evt-e2e-ntfy-ui","event":"message","topic":"home-lab-alerts","title":"UI ntfy","message":"operator workflow"}`))
	if err != nil {
		t.Fatalf("valid ntfy UI setup webhook failed: %v", err)
	}
	validBody, err := readBody(validResp)
	if err != nil {
		t.Fatalf("read valid UI setup body: %v", err)
	}
	if validResp.StatusCode != http.StatusAccepted {
		t.Fatalf("valid UI setup webhook status/body = %d %s", validResp.StatusCode, string(validBody))
	}
	malformedResp, err := apiPostRaw(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", []byte(`{"event":"message","message":"token=secret-token"`))
	if err != nil {
		t.Fatalf("malformed ntfy UI setup webhook failed: %v", err)
	}
	malformedBody, err := readBody(malformedResp)
	if err != nil {
		t.Fatalf("read malformed UI setup body: %v", err)
	}
	if malformedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed UI setup webhook status/body = %d %s", malformedResp.StatusCode, string(malformedBody))
	}

	pages := map[string][]string{
		"/notifications/sources":                                 {"Notification Sources", "ntfy-local-webhook", "Dead letters", "source/output boundary"},
		"/notifications/sources/ntfy-local-webhook":              {"ntfy Source", "Topic Health And Troubleshooting", "Refresh Health", "Reconnect Source", "Last Accepted Event", "SourceEventSink"},
		"/notifications/sources/ntfy-local-webhook/dead-letters": {"ntfy Dead Letters", "malformed_json", "replay_through_source_sink"},
	}
	deadLetterPage := ""
	for path, markers := range pages {
		body := notificationWebPage(t, cfg, path)
		if path == "/notifications/sources/ntfy-local-webhook/dead-letters" {
			deadLetterPage = body
		}
		for _, marker := range markers {
			if !strings.Contains(body, marker) {
				t.Fatalf("web page %s missing marker %q: %s", path, marker, body)
			}
		}
		if strings.Contains(body, "secret-token") || strings.Contains(body, "Telegram") || strings.Contains(body, "telegram") {
			t.Fatalf("web page %s leaked secret or source/output coupling: %s", path, body)
		}
	}
	deadLetterID := extractNtfyDeadLetterID(t, deadLetterPage)
	detailBody := notificationWebPage(t, cfg, "/notifications/sources/ntfy-local-webhook/dead-letters/"+deadLetterID)
	for _, marker := range []string{"ntfy Dead Letter", deadLetterID, "Replay Confirmation", "replay_through_source_sink", "SourceEventSink", "does not perform output dispatch"} {
		if !strings.Contains(detailBody, marker) {
			t.Fatalf("dead-letter detail page missing marker %q: %s", marker, detailBody)
		}
	}
	if strings.Contains(detailBody, "secret-token") || strings.Contains(detailBody, "Telegram") || strings.Contains(detailBody, "telegram") {
		t.Fatalf("dead-letter detail page leaked secret or source/output coupling: %s", detailBody)
	}
}

func TestNtfySourceListShowsDisconnectedRedactedHealthWithoutSecrets(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	store, pool, cleanup := notificationE2EStore(t)
	t.Cleanup(cleanup)
	prefix := notificationE2EPrefix()
	sourceID := prefix + "-ntfy-ui-auth-failed"
	ntfyCfg := ntfyE2EConfig(sourceID)
	seedNtfyE2ESource(t, store, ntfyCfg)
	t.Cleanup(func() { cleanupNtfyE2EArtifacts(t, pool, prefix) })
	now := time.Date(2026, 5, 24, 21, 0, 0, 0, time.UTC)
	report := notification.SourceHealthReport{SourceType: ntfysource.SourceType, SourceInstanceID: sourceID, SourceForm: ntfyCfg.SourceForm, State: notification.SourceHealthDisconnected, RetryCount: 2, LastErrorKind: ntfysource.ErrorAuthFailed, LastErrorRedacted: "token=secret-token-123 password=hunter2", ObservedAt: now}
	if err := store.RecordSourceHealth(context.Background(), report); err != nil {
		t.Fatalf("record ntfy UI auth failure health: %v", err)
	}

	body := notificationWebPage(t, cfg, "/notifications/sources")
	for _, marker := range []string{sourceID, "ntfy", "disconnected", "source authentication failed", "auth bearer_token", "Dead letters", "source/output boundary"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("source list missing marker %q for disconnected ntfy row: %s", marker, body)
		}
	}
	if strings.Contains(body, "secret-token-123") || strings.Contains(body, "hunter2") || strings.Contains(body, "Telegram") || strings.Contains(body, "telegram") {
		t.Fatalf("source list leaked credential material or output coupling: %s", body)
	}
}

func extractNtfyDeadLetterID(t *testing.T, html string) string {
	t.Helper()
	marker := "ntfy_dlq_"
	start := strings.Index(html, marker)
	if start < 0 {
		t.Fatalf("dead-letter page did not include an ntfy dead-letter id: %s", html)
	}
	end := start + len(marker)
	for end < len(html) {
		ch := html[end]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			end++
			continue
		}
		break
	}
	return html[start:end]
}

