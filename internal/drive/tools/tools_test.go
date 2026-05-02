// Spec 038 Scope 7 — SCN-038-021 unit anchor.
//
// TestDriveToolsRegisterWithPolicyAndTraceContracts proves that:
//
//   - All four drive tools are registered with the agent registry from
//     this package's init().
//   - Each tool has the correct side-effect class, owning package, and
//     a non-empty input/output schema (the contract surface the spec
//     037 executor relies on).
//   - Tool input arguments are validated against the schemas the
//     handler advertises (executor-side compile + per-call validate).
//   - Calling a handler before SetToolServices returns the structured
//     `drive_tools_not_configured` payload — the LLM sees a tool error
//     trace, never a panic.
//   - Calling a handler with services wired routes through the
//     configured services and the policy engine refuses sensitive
//     content for drive_get_file (BS-025 — sensitive bytes never
//     leave Smackerel through this path either).
package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/drive/policy"
	"github.com/smackerel/smackerel/internal/drive/retrieve"
)

// fakeSearcher / fakeFetcher are the same shape as the unit test
// fakes in internal/drive/retrieve, restated here so tests/integration
// can run independently. We only need the interfaces, no plumbing.
type fakeSearcher struct {
	candidates []retrieve.RetrieveCandidate
	err        error
}

func (f *fakeSearcher) SearchDrive(_ context.Context, _ retrieve.RetrieveRequest) ([]retrieve.RetrieveCandidate, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.candidates, nil
}

type fakeFetcher struct {
	bytes []byte
	mime  string
	calls int
}

func (f *fakeFetcher) GetArtifactBytes(_ context.Context, _ string) ([]byte, string, error) {
	f.calls++
	return f.bytes, f.mime, nil
}

func TestDriveToolsRegisterWithPolicyAndTraceContracts(t *testing.T) {
	// Registration contract -------------------------------------------------
	t.Run("all_four_tools_registered_with_correct_side_effect_class", func(t *testing.T) {
		expected := map[string]agent.SideEffectClass{
			"drive_search":     agent.SideEffectRead,
			"drive_get_file":   agent.SideEffectExternal,
			"drive_save_file":  agent.SideEffectExternal,
			"drive_list_rules": agent.SideEffectRead,
		}
		for name, want := range expected {
			tool, ok := agent.ByName(name)
			if !ok {
				t.Fatalf("tool %q not registered (init() did not run?)", name)
			}
			if tool.SideEffectClass != want {
				t.Fatalf("tool %q SideEffectClass = %q, want %q", name, tool.SideEffectClass, want)
			}
			if tool.OwningPackage != "internal/drive/tools" {
				t.Fatalf("tool %q OwningPackage = %q, want internal/drive/tools", name, tool.OwningPackage)
			}
			if len(tool.InputSchema) == 0 {
				t.Fatalf("tool %q InputSchema is empty", name)
			}
			if len(tool.OutputSchema) == 0 {
				t.Fatalf("tool %q OutputSchema is empty", name)
			}
			if tool.PerCallTimeoutMs <= 0 {
				t.Fatalf("tool %q PerCallTimeoutMs = %d, want > 0", name, tool.PerCallTimeoutMs)
			}
		}
	})

	t.Run("tool_names_constant_matches_registry", func(t *testing.T) {
		// Adversarial: a regression that adds/removes a tool without
		// updating the ToolNames constant would silently desync; this
		// asserts they line up.
		seen := map[string]bool{}
		for _, n := range ToolNames {
			seen[n] = true
		}
		for _, n := range []string{"drive_search", "drive_get_file", "drive_save_file", "drive_list_rules"} {
			if !seen[n] {
				t.Fatalf("ToolNames missing %q", n)
			}
		}
		if len(ToolNames) != 4 {
			t.Fatalf("len(ToolNames) = %d, want 4", len(ToolNames))
		}
	})

	t.Run("input_schemas_compile_and_reject_invalid_args", func(t *testing.T) {
		tool, ok := agent.ByName("drive_search")
		if !ok {
			t.Fatalf("drive_search not registered")
		}
		// Compile schema and validate a bad arg payload (missing
		// required "query"); the schema MUST reject it. This proves
		// the executor's per-call validation path will refuse the call
		// before the handler runs.
		schema, err := agent.CompileSchema(tool.InputSchema)
		if err != nil {
			t.Fatalf("compile drive_search input schema: %v", err)
		}
		if err := schema.ValidateBytes(json.RawMessage(`{}`)); err == nil {
			t.Fatalf("drive_search input schema accepted empty args; required[query] missing")
		}
		if err := schema.ValidateBytes(json.RawMessage(`{"query":"boarding pass"}`)); err != nil {
			t.Fatalf("drive_search input schema rejected valid args: %v", err)
		}
	})

	// Trace + policy contract ------------------------------------------------
	t.Run("handlers_return_not_configured_envelope_before_setservices", func(t *testing.T) {
		ResetForTest()
		t.Cleanup(ResetForTest)

		for _, name := range ToolNames {
			tool, ok := agent.ByName(name)
			if !ok {
				t.Fatalf("tool %q not registered", name)
			}
			out, err := tool.Handler(context.Background(), validArgsFor(name))
			if err != nil {
				t.Fatalf("handler %q returned error before wiring: %v", name, err)
			}
			var payload map[string]any
			if err := json.Unmarshal(out, &payload); err != nil {
				t.Fatalf("handler %q output not JSON: %v", name, err)
			}
			if payload["ok"] != false {
				t.Fatalf("handler %q before wiring: ok = %v, want false", name, payload["ok"])
			}
			if payload["error"] != "drive_tools_not_configured" {
				t.Fatalf("handler %q before wiring: error = %v, want drive_tools_not_configured", name, payload["error"])
			}
		}
	})

	t.Run("drive_get_file_with_sensitive_candidate_returns_secure_link_no_bytes", func(t *testing.T) {
		ResetForTest()
		t.Cleanup(ResetForTest)

		searcher := &fakeSearcher{candidates: []retrieve.RetrieveCandidate{{
			ArtifactID:  "drive:google:conn:medical-record",
			Title:       "Medical record.pdf",
			Folder:      "Medical",
			Sensitivity: "medical",
			SizeBytes:   8_000,
			Provider:    "google",
			ProviderURL: "https://drive.example/medical",
		}}}
		fetcher := &fakeFetcher{bytes: []byte("LEAK"), mime: "application/pdf"}
		retriever := retrieve.NewService(searcher, fetcher, policy.NewEngine(), 5_000_000, retrieve.DefaultReasonTable())

		SetToolServices(&ToolServices{
			Retriever: retriever,
			Policy:    policy.NewEngine(),
		})

		tool, ok := agent.ByName("drive_get_file")
		if !ok {
			t.Fatalf("drive_get_file not registered")
		}
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"query":"medical record"}`))
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("handler output not JSON: %v", err)
		}
		if payload["ok"] != true {
			t.Fatalf("ok = %v, want true (got payload=%+v)", payload["ok"], payload)
		}
		if mode, _ := payload["mode"].(string); mode != string(retrieve.ModeSecureLink) {
			t.Fatalf("mode = %q, want %q (sensitive must downgrade)", mode, retrieve.ModeSecureLink)
		}
		if _, present := payload["bytes_base64"]; present {
			t.Fatalf("BS-025 VIOLATION: bytes_base64 present for sensitive download (payload=%+v)", payload)
		}
		if fetcher.calls != 0 {
			t.Fatalf("BS-025 VIOLATION: fetcher.calls = %d for sensitive content; must be 0", fetcher.calls)
		}
		if reason, _ := payload["policy_reason"].(string); reason == "" {
			t.Fatalf("policy_reason empty; agent trace must explain the downgrade")
		}
	})

	t.Run("drive_search_returns_provider_neutral_candidates", func(t *testing.T) {
		ResetForTest()
		t.Cleanup(ResetForTest)

		searcher := &fakeSearcher{candidates: []retrieve.RetrieveCandidate{{
			ArtifactID:  "drive:google:conn:lisbon",
			Title:       "Lisbon boarding pass.pdf",
			Folder:      "Travel/Portugal",
			Sensitivity: "none",
			SizeBytes:   8_000,
			Provider:    "google",
			ProviderURL: "https://drive.example/lisbon",
		}}}
		fetcher := &fakeFetcher{}
		retriever := retrieve.NewService(searcher, fetcher, policy.NewEngine(), 5_000_000, retrieve.DefaultReasonTable())
		SetToolServices(&ToolServices{Retriever: retriever, Policy: policy.NewEngine()})

		tool, _ := agent.ByName("drive_search")
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"query":"lisbon"}`))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		var payload struct {
			OK         bool             `json:"ok"`
			Candidates []map[string]any `json:"candidates"`
		}
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if !payload.OK || len(payload.Candidates) != 1 {
			t.Fatalf("payload wrong: %+v", payload)
		}
		c := payload.Candidates[0]
		for _, k := range []string{"artifact_id", "title", "folder", "provider", "sensitivity", "provider_url"} {
			if v, _ := c[k].(string); v == "" {
				t.Fatalf("candidate missing %q label: %+v", k, c)
			}
		}
		// drive_search MUST not branch on provider; it just surfaces
		// the value. Adversarial guard: a regression that returned a
		// hard-coded provider would fail this if the fixture changed.
		if c["provider"] != "google" {
			t.Fatalf("provider = %v, want google (passed through)", c["provider"])
		}
	})

	t.Run("output_schema_validates_drive_search_payload", func(t *testing.T) {
		// Round-trip: take a real handler output and prove the
		// declared output schema accepts it. Catches regressions where
		// the handler shape drifts from the schema.
		ResetForTest()
		t.Cleanup(ResetForTest)
		retriever := retrieve.NewService(
			&fakeSearcher{candidates: []retrieve.RetrieveCandidate{{
				ArtifactID:  "drive:google:conn:x",
				Title:       "x",
				Folder:      "f",
				Sensitivity: "none",
				SizeBytes:   1,
				Provider:    "google",
				ProviderURL: "https://x",
			}}},
			&fakeFetcher{},
			policy.NewEngine(),
			5_000_000,
			retrieve.DefaultReasonTable(),
		)
		SetToolServices(&ToolServices{Retriever: retriever, Policy: policy.NewEngine()})

		tool, _ := agent.ByName("drive_search")
		out, err := tool.Handler(context.Background(), json.RawMessage(`{"query":"x"}`))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		schema, err := agent.CompileSchema(tool.OutputSchema)
		if err != nil {
			t.Fatalf("compile output schema: %v", err)
		}
		if err := schema.ValidateBytes(out); err != nil {
			t.Fatalf("output schema rejected handler output %s: %v", string(out), err)
		}
	})
}

// validArgsFor returns a JSON arg payload that satisfies the tool's
// input schema so the not-configured envelope test exercises the
// "passed validation, hit handler" path uniformly across tools.
func validArgsFor(tool string) json.RawMessage {
	switch tool {
	case "drive_search":
		return json.RawMessage(`{"query":"x"}`)
	case "drive_get_file":
		return json.RawMessage(`{"query":"x"}`)
	case "drive_save_file":
		return json.RawMessage(`{"artifact_id":"a","title":"t","classification":"c","sensitivity":"none","confidence":0.9,"content_base64":"` + base64.StdEncoding.EncodeToString([]byte("x")) + `"}`)
	case "drive_list_rules":
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(`{}`)
}

// Compile-time guard: the tools package MUST keep the canonical names
// identical to what scenario allowlists reference. A regression that
// renamed a tool in the registry but forgot to update ToolNames would
// fail the registration test above; this constant pins the four names
// in source for grep-based audits.
var _ = strings.Join(ToolNames, ",") // unused; suppresses lint when ToolNames is the only reference
