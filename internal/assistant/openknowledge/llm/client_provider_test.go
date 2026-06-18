// Spec 096 SCOPE-03 — SCN-096-A02: the Go llm.ChatRequest gains the four
// provider-credential-seam fields ADDITIVELY. A zero-value request (the spec
// 064/088/089 no-override Ollama caller) MUST serialize byte-for-byte the
// pre-096 wire shape so the sidecar takes the unchanged Ollama branch; a hosted
// request carries the new fields without disturbing the existing ones.
package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestChatRequest_ProviderFieldsAdditive_Spec096 — ADVERSARIAL. The new
// Provider/APIBase/APIKey/ProviderParams fields are omitempty: a request that
// leaves them zero marshals to the EXACT pre-096 JSON, so no existing caller's
// wire bytes change. A second assertion proves the fields DO appear when set
// (so the additive contract is real, not a dead struct tag).
func TestChatRequest_ProviderFieldsAdditive_Spec096(t *testing.T) {
	maxTokens := 256
	temp := 0.0

	t.Run("zero_value_provider_fields_are_byte_for_byte_pre096", func(t *testing.T) {
		req := ChatRequest{
			Model: "gemma3:4b",
			Messages: []ChatMessage{
				{Role: RoleSystem, Content: "You are the planner."},
				{Role: RoleUser, Content: "Convert 5 km to miles."},
			},
			MaxTokens:   &maxTokens,
			Temperature: &temp,
		}
		got, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// The exact pre-096 wire shape (model, messages, max_tokens,
		// temperature) — NONE of the four spec-096 keys present.
		const wantPre096 = `{"model":"gemma3:4b","messages":[{"role":"system","content":"You are the planner."},{"role":"user","content":"Convert 5 km to miles."}],"max_tokens":256,"temperature":0}`
		if string(got) != wantPre096 {
			t.Fatalf("zero-value request wire shape drifted from pre-096:\n got: %s\nwant: %s", got, wantPre096)
		}
		// Adversarial belt-and-suspenders: assert each spec-096 key is ABSENT.
		for _, k := range []string{`"provider":`, `"api_base":`, `"api_key":`, `"provider_params":`} {
			if strings.Contains(string(got), k) {
				t.Fatalf("zero-value request leaked spec-096 key %s into the pre-096 wire shape: %s", k, got)
			}
		}
	})

	t.Run("hosted_request_carries_the_new_fields", func(t *testing.T) {
		base := "https://api.anthropic.test"
		key := "sk-synthetic-096" // gitleaks:allow
		req := ChatRequest{
			Model:          "claude-3-5-sonnet", // BARE backend id
			Messages:       []ChatMessage{{Role: RoleUser, Content: "hi"}},
			Provider:       "anthropic",
			APIBase:        &base,
			APIKey:         &key,
			ProviderParams: map[string]any{"organization": "acme"},
		}
		got, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// Round-trip back so the assertion is structural, not string-order
		// fragile.
		var back map[string]any
		if err := json.Unmarshal(got, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if back["provider"] != "anthropic" {
			t.Fatalf("provider not carried: %v", back["provider"])
		}
		if back["api_base"] != base {
			t.Fatalf("api_base not carried: %v", back["api_base"])
		}
		if back["api_key"] != key {
			t.Fatalf("api_key not carried: %v", back["api_key"])
		}
		if back["model"] != "claude-3-5-sonnet" {
			t.Fatalf("model MUST be the bare backend id, got %v", back["model"])
		}
		pp, ok := back["provider_params"].(map[string]any)
		if !ok || pp["organization"] != "acme" {
			t.Fatalf("provider_params not carried: %v", back["provider_params"])
		}
	})
}
