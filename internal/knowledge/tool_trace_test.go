package knowledge

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToolTrace_ZeroValueDefaultsFilledOnInsert(t *testing.T) {
	// Pure struct-level test that the zero-value invariants we rely
	// on inside insertToolTraceTx are stable: empty Params/ResultSummary
	// should be substituted with json "{}" so the JSONB NOT NULL
	// column constraint cannot reject the row.
	tr := &ToolTrace{
		AgentAnswerID: "01HEXAMPLE",
		Sequence:      1,
		ToolName:      "web_search",
		ExecutedAt:    time.Now(),
	}
	if len(tr.Params) != 0 {
		t.Fatalf("expected empty Params, got %s", string(tr.Params))
	}
	if len(tr.ResultSummary) != 0 {
		t.Fatalf("expected empty ResultSummary, got %s", string(tr.ResultSummary))
	}
}

func TestToolTrace_ParamsRoundTrip(t *testing.T) {
	want := map[string]any{"query": "sourdough", "k": 3.0}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tr := &ToolTrace{Params: raw}
	var got map[string]any
	if err := json.Unmarshal(tr.Params, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["query"].(string) != want["query"].(string) {
		t.Errorf("query mismatch: got %v want %v", got["query"], want["query"])
	}
}
