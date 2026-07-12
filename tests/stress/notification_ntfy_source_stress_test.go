//go:build stress

package stress

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNtfyWebhookBurstUsesRuntimeReceiverWithoutDuplicateRejection(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	const burstSize = 8
	var wg sync.WaitGroup
	for index := 0; index < burstSize; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := []byte(fmt.Sprintf(`{"id":"evt-stress-ntfy-%d","event":"message","topic":"self-hosted-alerts","title":"stress ntfy %d","message":"runtime webhook receiver burst"}`, index, index))
			status, body, err := stressAPIPost(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", payload)
			if err != nil {
				t.Errorf("ntfy webhook burst request %d failed: %v", index, err)
				return
			}
			if status != http.StatusAccepted || !strings.Contains(string(body), `"accepted":true`) {
				t.Errorf("ntfy webhook burst request %d status/body = %d %s", index, status, string(body))
			}
		}()
	}
	wg.Wait()
	status, detailBody, err := stressAPIGet(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy")
	if err != nil {
		t.Fatalf("ntfy detail after burst failed: %v", err)
	}
	if status != http.StatusOK || !strings.Contains(string(detailBody), "SourceEventSink") {
		t.Fatalf("ntfy detail after burst status/body = %d %s", status, string(detailBody))
	}
}

func TestNtfyMalformedReconnectAndDuplicateBurstCreatesBoundedOperationalRecords(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	const malformedBurst = 5
	for index := 0; index < malformedBurst; index++ {
		status, body, err := stressAPIPost(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", []byte(fmt.Sprintf(`{"event":"message","message":"malformed token=secret-%d"`, index)))
		if err != nil {
			t.Fatalf("malformed ntfy burst request %d failed: %v", index, err)
		}
		if status != http.StatusBadRequest || !strings.Contains(string(body), "invalid_ntfy_webhook_payload") {
			t.Fatalf("malformed ntfy burst request %d status/body = %d %s", index, status, string(body))
		}
	}
	for index := 0; index < 3; index++ {
		status, body, err := stressAPIPost(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", []byte(fmt.Sprintf(`{"id":"evt-stress-ntfy-dup","event":"message","topic":"self-hosted-alerts","title":"dup %d","message":"duplicate upstream id preserves source provenance"}`, index)))
		if err != nil {
			t.Fatalf("duplicate ntfy burst request %d failed: %v", index, err)
		}
		if status != http.StatusAccepted || !strings.Contains(string(body), `"accepted":true`) {
			t.Fatalf("duplicate ntfy burst request %d status/body = %d %s", index, status, string(body))
		}
	}
	status, reconnectBody, err := stressAPIPost(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/reconnect", []byte(`{}`))
	if err != nil {
		t.Fatalf("ntfy reconnect stress request failed: %v", err)
	}
	if status != http.StatusAccepted || !strings.Contains(string(reconnectBody), `"created_notification":false`) {
		t.Fatalf("ntfy reconnect stress status/body = %d %s", status, string(reconnectBody))
	}
	status, dlqBody, err := stressAPIGet(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/dead-letters?limit=5")
	if err != nil {
		t.Fatalf("ntfy DLQ stress query failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("ntfy DLQ stress status/body = %d %s", status, string(dlqBody))
	}
	var parsed struct {
		DeadLetters []struct {
			CauseKind string `json:"cause_kind"`
		} `json:"dead_letters"`
	}
	if err := json.Unmarshal(dlqBody, &parsed); err != nil {
		t.Fatalf("parse ntfy DLQ stress body: %v; body=%s", err, string(dlqBody))
	}
	if len(parsed.DeadLetters) < malformedBurst {
		t.Fatalf("ntfy malformed burst created %d dead letters, want at least %d", len(parsed.DeadLetters), malformedBurst)
	}
	if strings.Contains(string(dlqBody), "secret-") {
		t.Fatalf("ntfy dead-letter stress body leaked malformed secret: %s", string(dlqBody))
	}
}

func TestNtfyConfigValidationBurstDoesNotFabricateConnectedHealth(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	const reconnectBurst = 4
	for index := 0; index < reconnectBurst; index++ {
		status, body, err := stressAPIPost(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/reconnect", []byte(`{}`))
		if err != nil {
			t.Fatalf("ntfy reconnect burst request %d failed: %v", index, err)
		}
		if status != http.StatusAccepted || !strings.Contains(string(body), `"created_notification":false`) {
			t.Fatalf("ntfy reconnect burst request %d status/body = %d %s", index, status, string(body))
		}
	}
	status, detailBody, err := stressAPIGet(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy")
	if err != nil {
		t.Fatalf("ntfy detail after reconnect burst failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("ntfy detail after reconnect burst status/body = %d %s", status, string(detailBody))
	}
	if !strings.Contains(string(detailBody), "reconnecting") || !strings.Contains(string(detailBody), "operator requested reconnect") || strings.Contains(string(detailBody), "created_notification\":true") {
		t.Fatalf("ntfy reconnect burst fabricated connected health or notification side effect: %s", string(detailBody))
	}
}
