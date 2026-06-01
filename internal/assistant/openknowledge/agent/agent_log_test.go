// Spec 064 SCOPE-14 — agent redacted trace-logging test.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
)

// TestAgentTurnLog_RedactsSecrets drives the agent through one
// successful turn whose system prompt, user prompt, tool args, and
// tool result all contain values that MUST NOT appear in the log.
func TestAgentTurnLog_RedactsSecrets(t *testing.T) {
	const apiKey = "sk-supersecret-DO-NOT-LOG-9b3f7e1c"
	const rawPrompt = "How much is 2+2? My internal note: " + apiKey
	const fullURL = "https://example.test/secret-path?token=" + apiKey
	const snippetBody = "this snippet body must not appear in the log"

	web := fakeWebTool{url: fullURL, hash: "deadbeef", snippet: snippetBody}

	final := "The answer is 4.<CITATIONS>[{\"kind\":\"web\",\"url\":\"" + fullURL + "\",\"content_hash\":\"deadbeef\"}]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w1", "fake_web", `{"query":"`+apiKey+`","k":1}`, 10),
		endTurn(final, 5),
	}}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 })
	cfg.SystemPrompt = "system prompt mentioning " + apiKey
	cfg.Logger = logger

	r := ok.NewRegistry([]string{"calculator", "fake_web"})
	if err := r.Register(tools.NewCalculator()); err != nil {
		t.Fatalf("register calc: %v", err)
	}
	if err := r.Register(web); err != nil {
		t.Fatalf("register web: %v", err)
	}

	a, err := New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Run(context.Background(), rawPrompt); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "openknowledge.turn") {
		t.Fatalf("log missing openknowledge.turn marker; got=%q", out)
	}
	if strings.Count(out, "openknowledge.turn") != 1 {
		t.Errorf("openknowledge.turn marker count = %d want 1", strings.Count(out, "openknowledge.turn"))
	}
	if strings.Contains(out, apiKey) {
		t.Errorf("LOG LEAK: API key %q appears in log output:\n%s", apiKey, out)
	}
	if strings.Contains(out, "secret-path") {
		t.Errorf("LOG LEAK: full URL path appears in log output:\n%s", out)
	}
	if strings.Contains(out, snippetBody) {
		t.Errorf("LOG LEAK: web snippet body appears in log output:\n%s", out)
	}
	if strings.Contains(out, rawPrompt) {
		t.Errorf("LOG LEAK: raw user prompt appears in log output:\n%s", out)
	}
	if strings.Contains(out, "system prompt mentioning") {
		t.Errorf("LOG LEAK: system prompt text appears in log output:\n%s", out)
	}

	var rec map[string]any
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse log JSON: %v\nraw=%s", err, out)
	}
	wantFields := []string{
		"turn_id", "prompt_sha256", "iterations", "tokens_used",
		"usd_spent", "status", "termination_reason", "num_sources",
		"compaction_signaled", "tool_calls",
	}
	for _, f := range wantFields {
		if _, ok := rec[f]; !ok {
			t.Errorf("log record missing field %q (record=%v)", f, rec)
		}
	}
	got, _ := rec["prompt_sha256"].(string)
	want := sha256Hex(rawPrompt)
	if got != want {
		t.Errorf("prompt_sha256 = %q want %q", got, want)
	}
	calls, _ := rec["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("tool_calls len = %d want 1: %+v", len(calls), calls)
	}
	first, _ := calls[0].(map[string]any)
	if first["name"] != "fake_web" {
		t.Errorf("tool_calls[0].name = %v want fake_web", first["name"])
	}
	if first["outcome"] != "success" {
		t.Errorf("tool_calls[0].outcome = %v want success", first["outcome"])
	}
	for k := range first {
		if k != "name" && k != "outcome" {
			t.Errorf("tool_calls entry leaked unexpected field %q = %v", k, first[k])
		}
	}
}

// TestAgentTurnLog_EmittedOnRefusal proves the log is written even
// when the agent refuses (so operators can correlate refusal
// volume).
func TestAgentTurnLog_EmittedOnRefusal(t *testing.T) {
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("ignored", 100)}}
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := baseCfg(5, 50 /*tokens*/, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 })
	cfg.Logger = logger
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Run(context.Background(), "big"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), "openknowledge.turn") {
		t.Fatalf("expected openknowledge.turn log entry on refusal; got=%q", buf.String())
	}
	if !strings.Contains(buf.String(), `"termination_reason":"cap_tokens"`) {
		t.Errorf("expected termination_reason=cap_tokens; got=%q", buf.String())
	}
}
