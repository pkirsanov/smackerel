//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPhotosContractCanary_ConfigNATSDBAndMLAgree(t *testing.T) {
	t.Run("config_PHOTOS_env_vars_present", canaryConfigPhotosEnvVars)
	t.Run("nats_PHOTOS_stream_in_jetstream", canaryNATSPhotosStream)
	t.Run("migration_025_photos_present", canaryMigration025Photos)
	t.Run("ml_sidecar_photos_contract_response", canaryMLPhotosContract)
}

func canaryMLPhotosContract(t *testing.T) {
	nc := testNATSConn(t)
	sub, err := nc.SubscribeSync("photos.classified")
	if err != nil {
		t.Fatalf("subscribe photos.classified: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush NATS subscription: %v", err)
	}

	requestID := strings.ReplaceAll("scope-040-"+testID(t), "/", "-")
	payload := map[string]any{
		"request_id":  requestID,
		"photo_id":    "photo-" + requestID,
		"artifact_id": "artifact-" + requestID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal photo classify canary payload: %v", err)
	}
	if err := nc.Publish("photos.classify", encoded); err != nil {
		t.Fatalf("publish photos.classify canary: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush photos.classify canary: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := sub.NextMsg(time.Until(deadline))
		if err != nil {
			t.Fatalf("wait for photos.classified canary response: %v", err)
		}
		var response struct {
			RequestID  string `json:"request_id"`
			PhotoID    string `json:"photo_id"`
			ArtifactID string `json:"artifact_id"`
			Result     struct {
				Decision   string  `json:"decision"`
				Confidence float64 `json:"confidence"`
				Rationale  string  `json:"rationale"`
			} `json:"result"`
		}
		if err := json.Unmarshal(msg.Data, &response); err != nil {
			t.Fatalf("decode photos.classified canary response %q: %v", string(msg.Data), err)
		}
		if response.RequestID != requestID {
			continue
		}
		if response.PhotoID != payload["photo_id"] || response.ArtifactID != payload["artifact_id"] {
			t.Fatalf("identity mismatch in ML response: %+v", response)
		}
		if response.Result.Decision != "needs_model" {
			t.Fatalf("decision = %q, want needs_model", response.Result.Decision)
		}
		if response.Result.Confidence != 0 {
			t.Fatalf("confidence = %v, want 0 for Scope-1 canary", response.Result.Confidence)
		}
		if strings.TrimSpace(response.Result.Rationale) == "" {
			t.Fatalf("ML response omitted required rationale: %+v", response.Result)
		}
		return
	}
	t.Fatalf("did not receive matching photos.classified response for request_id=%s", requestID)
}
