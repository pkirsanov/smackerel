// BUG-064-002 DEFECT 3a (user-visible render) — a delivered
// open_knowledge answer (Status=StatusAnswered) MUST render WITHOUT a
// "thinking…" header, with the synthesized body and the (capped) source
// list intact.
package assistant_adapter

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestBuildTelegramRendering_AnsweredNoThinkingHeader_BUG064002(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Status: contracts.StatusAnswered,
		Body:   "wa-town-A 06/11: high 7:42am 8.9 ft, low 1:55pm 0.3 ft.",
		Sources: []contracts.Source{{
			ID:    "https://tidetime.org/x",
			Title: "tidetime.org",
			Kind:  contracts.SourceWeb,
			Ref: contracts.WebSourceRef{
				URL:         "https://tidetime.org/x",
				Provider:    "searxng",
				ContentHash: "h1",
				Snippet:     "wa-town-A tide times",
			},
		}},
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(rendered, "thinking") {
		t.Fatalf("BUG-064-002 DEFECT 3a: delivered answer rendered a 'thinking…' header.\nrendered=%q", rendered)
	}
	if !strings.Contains(rendered, "high 7:42am 8.9 ft") {
		t.Fatalf("synthesized answer body missing from render: %q", rendered)
	}
}

// statusPrefix(StatusAnswered) MUST be empty — the terminal answered
// token carries no in-flight prefix.
func TestStatusPrefix_AnsweredIsEmpty_BUG064002(t *testing.T) {
	t.Parallel()
	if p := statusPrefix(contracts.AssistantResponse{Status: contracts.StatusAnswered}); p != "" {
		t.Fatalf("statusPrefix(StatusAnswered) = %q, want empty", p)
	}
}
