//go:build stress

package stress

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type notificationStressIngestResponse struct {
	NotificationID string `json:"notification_id"`
	IncidentID     string `json:"incident_id"`
	DecisionID     string `json:"decision_id"`
	ApprovalID     string `json:"approval_id"`
	Receipt        struct {
		Accepted bool `json:"Accepted"`
	} `json:"receipt"`
}

type notificationStressDetail struct {
	Decision *struct {
		DecisionType string `json:"DecisionType"`
	} `json:"Decision"`
}

type notificationStressOutput struct {
	DecisionID     string         `json:"DecisionID"`
	IncidentID     string         `json:"IncidentID"`
	PayloadHash    string         `json:"PayloadHash"`
	RedactionState map[string]any `json:"RedactionState"`
	Status         string         `json:"Status"`
	Channel        string         `json:"Channel"`
}

func notificationStressPrefix() string {
	return "notif-stress-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
}

func notificationStressIngest(t *testing.T, cfg stressConfig, payload map[string]any) notificationStressIngestResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal notification stress payload: %v", err)
	}
	status, responseBody, err := stressAPIPost(cfg, "/api/notifications/manual-ingest", body)
	if err != nil {
		t.Fatalf("notification stress ingest failed: %v", err)
	}
	if status != 201 {
		t.Fatalf("notification stress ingest returned %d: %s", status, string(responseBody))
	}
	var parsed notificationStressIngestResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		t.Fatalf("parse notification stress ingest response: %v; body=%s", err, string(responseBody))
	}
	if !parsed.Receipt.Accepted || parsed.NotificationID == "" || parsed.IncidentID == "" || parsed.DecisionID == "" {
		t.Fatalf("notification stress ingest missed durable identifiers: %+v", parsed)
	}
	return parsed
}

func notificationStressEventDetail(t *testing.T, cfg stressConfig, notificationID string) notificationStressDetail {
	t.Helper()
	status, body, err := stressAPIGet(cfg, "/api/notifications/events/"+notificationID)
	if err != nil {
		t.Fatalf("notification stress detail failed: %v", err)
	}
	if status != 200 {
		t.Fatalf("notification stress detail returned %d: %s", status, string(body))
	}
	var parsed notificationStressDetail
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse notification stress detail: %v; body=%s", err, string(body))
	}
	return parsed
}

func notificationStressOutputs(t *testing.T, cfg stressConfig) []notificationStressOutput {
	t.Helper()
	status, body, err := stressAPIGet(cfg, "/api/notifications/outputs")
	if err != nil {
		t.Fatalf("notification stress outputs failed: %v", err)
	}
	if status != 200 {
		t.Fatalf("notification stress outputs returned %d: %s", status, string(body))
	}
	var parsed struct {
		Outputs []notificationStressOutput `json:"outputs"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse notification stress outputs: %v; body=%s", err, string(body))
	}
	return parsed.Outputs
}

func notificationStressPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("stress: DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect notification stress database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func notificationStressCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("notification stress count query failed: %v", err)
	}
	return count
}

func notificationStressPayload(prefix string, sourceID string, index int, severity string, intent string) map[string]any {
	return map[string]any{
		"source_type":        "manual_fixture",
		"source_instance_id": sourceID,
		"title":              fmt.Sprintf("%s notification %d", prefix, index),
		"body":               fmt.Sprintf("%s notification %d body", prefix, index),
		"severity":           severity,
		"subject":            prefix,
		"service":            prefix,
		"domain":             "ops",
		"intent":             intent,
		"delivery_metadata":  map[string]string{"actor": "stress"},
	}
}