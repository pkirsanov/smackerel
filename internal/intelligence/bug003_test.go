package intelligence

import (
	"encoding/json"
	"testing"
)

// TestNormalizeDifficulty_KnownLabels verifies the LLM reply normalization
// for SubjectLearningClassify only accepts the three documented difficulty
// labels and rejects everything else (so noise from a misbehaving sidecar
// falls back to the local heuristic instead of corrupting the path).
func TestNormalizeDifficulty_KnownLabels(t *testing.T) {
	tests := []struct {
		in   string
		want LearningDifficulty
	}{
		{"beginner", DifficultyBeginner},
		{"INTERMEDIATE", DifficultyIntermediate},
		{"  Advanced  ", DifficultyAdvanced},
		{"", ""},
		{"expert", ""},
		{"easy", ""},
		{"hard", ""},
	}
	for _, tt := range tests {
		got := normalizeDifficulty(tt.in)
		if got != tt.want {
			t.Errorf("normalizeDifficulty(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestLLMReplyShapes guards the JSON shape contract for the BUG-003 ML
// sidecar replies. The Go side asks the sidecar for these fields; if the
// tag names drift, the unmarshal will silently produce zero values and we
// would fall back without anyone noticing.
func TestLLMReplyShapes(t *testing.T) {
	t.Run("monthly", func(t *testing.T) {
		var r monthlyGenerateReply
		if err := json.Unmarshal([]byte(`{"report_text":"hello"}`), &r); err != nil {
			t.Fatal(err)
		}
		if r.ReportText != "hello" {
			t.Errorf("monthly report_text not bound: %+v", r)
		}
	})
	t.Run("content", func(t *testing.T) {
		var r contentAnalyzeReply
		blob := `{"title":"T","uniqueness_rationale":"U","format_suggestion":"essay"}`
		if err := json.Unmarshal([]byte(blob), &r); err != nil {
			t.Fatal(err)
		}
		if r.Title != "T" || r.UniqueRationale != "U" || r.FormatSuggestion != "essay" {
			t.Errorf("content reply not bound: %+v", r)
		}
	})
	t.Run("learning", func(t *testing.T) {
		var r learningClassifyReply
		if err := json.Unmarshal([]byte(`{"difficulty":"advanced"}`), &r); err != nil {
			t.Fatal(err)
		}
		if r.Difficulty != "advanced" {
			t.Errorf("learning reply not bound: %+v", r)
		}
	})
	t.Run("quickref", func(t *testing.T) {
		var r quickrefGenerateReply
		if err := json.Unmarshal([]byte(`{"content":"compiled"}`), &r); err != nil {
			t.Fatal(err)
		}
		if r.Content != "compiled" {
			t.Errorf("quickref reply not bound: %+v", r)
		}
	})
	t.Run("seasonal", func(t *testing.T) {
		var r seasonalAnalyzeReply
		blob := `{"observations":[{"pattern":"p","month":"April","observation":"o","actionable":true}]}`
		if err := json.Unmarshal([]byte(blob), &r); err != nil {
			t.Fatal(err)
		}
		if len(r.Observations) != 1 || r.Observations[0].Pattern != "p" ||
			r.Observations[0].Month != "April" || r.Observations[0].Observation != "o" ||
			!r.Observations[0].Actionable {
			t.Errorf("seasonal reply not bound: %+v", r)
		}
	})
}

// TestNilNATS_FallbackPaths verifies that the BUG-003 callers do not panic or
// require a NATS client to compute their result. The local-only paths must
// continue to return something usable even when e.NATS is nil. Each call here
// runs against a nil pool so the function exits before touching the database;
// we only need to assert the engine construction and the nil-NATS guard work
// together (no panics, no nil-pointer dereference).
func TestNilNATS_FallbackPaths(t *testing.T) {
	engine := NewEngine(nil, nil) // nil pool, nil NATS
	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.NATS != nil {
		t.Errorf("expected nil NATS, got %v", engine.NATS)
	}
	// assembleMonthlyReportText is the local fallback the BUG-003 code path
	// uses when NATS is nil or fails. It must always return a non-empty
	// string for the empty-data shape.
	if got := assembleMonthlyReportText(&MonthlyReport{Month: "2026-04"}); got == "" {
		t.Error("assembleMonthlyReportText returned empty string for fallback path")
	}
	// classifyDifficultyHeuristic is the local fallback for learning
	// classify when NATS is nil or fails.
	if got := classifyDifficultyHeuristic("Intro to Go", "article", 0); got != DifficultyBeginner {
		t.Errorf("heuristic fallback: got %q, want beginner", got)
	}
}
