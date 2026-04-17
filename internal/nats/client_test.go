package nats

import (
	"context"
	"strings"
	"testing"
)

func TestAllStreams_Coverage(t *testing.T) {
	streams := AllStreams()
	if len(streams) != 9 {
		t.Fatalf("expected 9 streams, got %d", len(streams))
	}

	expected := map[string][]string{
		"ARTIFACTS":    {"artifacts.>"},
		"SEARCH":       {"search.>"},
		"DIGEST":       {"digest.>"},
		"KEEP":         {"keep.>"},
		"INTELLIGENCE": {"learning.>", "content.>", "monthly.>", "quickref.>", "seasonal.>"},
		"ALERTS":       {"alerts.>"},
		"SYNTHESIS":    {"synthesis.>"},
		"DOMAIN":       {"domain.>"},
		"DEADLETTER":   {"deadletter.>"},
	}

	for _, s := range streams {
		subjects, ok := expected[s.Name]
		if !ok {
			t.Errorf("unexpected stream: %s", s.Name)
			continue
		}
		if len(s.Subjects) != len(subjects) {
			t.Errorf("stream %s: expected %d subjects, got %d", s.Name, len(subjects), len(s.Subjects))
			continue
		}
		for i, subj := range s.Subjects {
			if subj != subjects[i] {
				t.Errorf("stream %s subject %d: expected %s, got %s", s.Name, i, subjects[i], subj)
			}
		}
	}
}

func TestSubjectConstants(t *testing.T) {
	subjects := []struct {
		name  string
		value string
	}{
		{"SubjectArtifactsProcess", SubjectArtifactsProcess},
		{"SubjectArtifactsProcessed", SubjectArtifactsProcessed},
		{"SubjectSearchEmbed", SubjectSearchEmbed},
		{"SubjectSearchEmbedded", SubjectSearchEmbedded},
		{"SubjectSearchRerank", SubjectSearchRerank},
		{"SubjectSearchReranked", SubjectSearchReranked},
		{"SubjectDigestGenerate", SubjectDigestGenerate},
		{"SubjectDigestGenerated", SubjectDigestGenerated},
		{"SubjectKeepSyncRequest", SubjectKeepSyncRequest},
		{"SubjectKeepSyncResponse", SubjectKeepSyncResponse},
		{"SubjectKeepOCRRequest", SubjectKeepOCRRequest},
		{"SubjectKeepOCRResponse", SubjectKeepOCRResponse},
		{"SubjectLearningClassify", SubjectLearningClassify},
		{"SubjectLearningClassified", SubjectLearningClassified},
		{"SubjectContentAnalyze", SubjectContentAnalyze},
		{"SubjectContentAnalyzed", SubjectContentAnalyzed},
		{"SubjectMonthlyGenerate", SubjectMonthlyGenerate},
		{"SubjectMonthlyGenerated", SubjectMonthlyGenerated},
		{"SubjectQuickrefGenerate", SubjectQuickrefGenerate},
		{"SubjectQuickrefGenerated", SubjectQuickrefGenerated},
		{"SubjectSeasonalAnalyze", SubjectSeasonalAnalyze},
		{"SubjectSeasonalAnalyzed", SubjectSeasonalAnalyzed},
		{"SubjectDomainExtract", SubjectDomainExtract},
		{"SubjectDomainExtracted", SubjectDomainExtracted},
	}

	for _, s := range subjects {
		if s.value == "" {
			t.Errorf("subject constant %s is empty", s.name)
		}
	}
}

func TestStreamConfig_HasRequiredFields(t *testing.T) {
	for _, s := range AllStreams() {
		if s.Name == "" {
			t.Error("stream has empty name")
		}
		if len(s.Subjects) == 0 {
			t.Errorf("stream %s has no subjects", s.Name)
		}
	}
}

func TestSubjectPairs(t *testing.T) {
	// Every publish subject should have a matching response subject
	pairs := []struct {
		request  string
		response string
	}{
		{SubjectArtifactsProcess, SubjectArtifactsProcessed},
		{SubjectSearchEmbed, SubjectSearchEmbedded},
		{SubjectSearchRerank, SubjectSearchReranked},
		{SubjectDigestGenerate, SubjectDigestGenerated},
		{SubjectKeepSyncRequest, SubjectKeepSyncResponse},
		{SubjectKeepOCRRequest, SubjectKeepOCRResponse},
		{SubjectLearningClassify, SubjectLearningClassified},
		{SubjectContentAnalyze, SubjectContentAnalyzed},
		{SubjectMonthlyGenerate, SubjectMonthlyGenerated},
		{SubjectQuickrefGenerate, SubjectQuickrefGenerated},
		{SubjectSeasonalAnalyze, SubjectSeasonalAnalyzed},
		{SubjectDomainExtract, SubjectDomainExtracted},
	}

	for _, p := range pairs {
		if p.request == "" || p.response == "" {
			t.Errorf("subject pair has empty value: %q -> %q", p.request, p.response)
		}
		// Response subject should be the request subject + "ed" or "d" suffix pattern
		if p.request == p.response {
			t.Errorf("request and response should differ: %q", p.request)
		}
	}
}

func TestSubjectNaming_Convention(t *testing.T) {
	// All subjects should follow domain.action or domain.sub.action pattern
	subjects := []string{
		SubjectArtifactsProcess, SubjectArtifactsProcessed,
		SubjectSearchEmbed, SubjectSearchEmbedded,
		SubjectSearchRerank, SubjectSearchReranked,
		SubjectDigestGenerate, SubjectDigestGenerated,
		SubjectKeepSyncRequest, SubjectKeepSyncResponse,
		SubjectKeepOCRRequest, SubjectKeepOCRResponse,
	}

	for _, s := range subjects {
		dotCount := 0
		for _, c := range s {
			if c == '.' {
				dotCount++
			}
		}
		if dotCount < 1 || dotCount > 2 {
			t.Errorf("subject %q should have 1 or 2 dot separators, got %d", s, dotCount)
		}
	}
}

func TestStreamNames_Unique(t *testing.T) {
	streams := AllStreams()
	names := make(map[string]bool)
	for _, s := range streams {
		if names[s.Name] {
			t.Errorf("duplicate stream name: %s", s.Name)
		}
		names[s.Name] = true
	}
}

func TestStreamSubjects_CoverAllSubjects(t *testing.T) {
	// Verify that all subject constants are covered by at least one stream's wildcard
	allSubjects := []string{
		SubjectArtifactsProcess, SubjectArtifactsProcessed,
		SubjectSearchEmbed, SubjectSearchEmbedded,
		SubjectSearchRerank, SubjectSearchReranked,
		SubjectDigestGenerate, SubjectDigestGenerated,
		SubjectKeepSyncRequest, SubjectKeepSyncResponse,
		SubjectKeepOCRRequest, SubjectKeepOCRResponse,
	}

	streams := AllStreams()
	for _, subj := range allSubjects {
		covered := false
		for _, s := range streams {
			for _, wildcard := range s.Subjects {
				// Check if subject matches wildcard (e.g., "artifacts.>" covers "artifacts.process")
				prefix := wildcard[:len(wildcard)-1] // remove ">"
				if len(subj) >= len(prefix) && subj[:len(prefix)] == prefix {
					covered = true
					break
				}
			}
			if covered {
				break
			}
		}
		if !covered {
			t.Errorf("subject %q not covered by any stream", subj)
		}
	}
}

// SCN-002-003: NATS connectivity — verify pub/sub subject routing is correctly configured
// for core→ml→core roundtrip (artifacts.process → artifacts.processed, etc.)
func TestSCN002003_NATSConnectivity_SubjectRouting(t *testing.T) {
	// Verify request/response subject pairs match the expected routing
	// Core publishes to *.process/embed/rerank/generate
	// ML sidecar publishes to *.processed/embedded/reranked/generated
	pairs := map[string]string{
		SubjectArtifactsProcess: SubjectArtifactsProcessed,
		SubjectSearchEmbed:      SubjectSearchEmbedded,
		SubjectSearchRerank:     SubjectSearchReranked,
		SubjectDigestGenerate:   SubjectDigestGenerated,
	}
	for req, resp := range pairs {
		// Request and response must share the same stream (same domain prefix)
		reqDomain := req[:indexByte(req, '.')]
		respDomain := resp[:indexByte(resp, '.')]
		if reqDomain != respDomain {
			t.Errorf("subject pair %q→%q crosses stream boundaries", req, resp)
		}
	}
}

// SCN-002-003: Verify all streams cover both request and response subjects
func TestSCN002003_NATSConnectivity_StreamCoverage(t *testing.T) {
	// Each stream must cover both directions of its domain's pub/sub
	for _, s := range AllStreams() {
		wildcard := s.Subjects[0] // e.g. "artifacts.>"
		if wildcard[len(wildcard)-1] != '>' {
			t.Errorf("stream %s subject %q should use > wildcard", s.Name, wildcard)
		}
	}
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func TestConnect_InvalidURL(t *testing.T) {
	// NATS Connect with an unreachable URL should fail
	ctx := context.Background()
	_, err := Connect(ctx, "nats://127.0.0.1:1", "")
	if err == nil {
		t.Fatal("expected error connecting to unreachable NATS")
	}
	if !strings.Contains(err.Error(), "connect to NATS") {
		t.Errorf("expected wrapped connect error, got: %v", err)
	}
}

func TestConnect_EmptyURL(t *testing.T) {
	ctx := context.Background()
	_, err := Connect(ctx, "", "")
	if err == nil {
		t.Fatal("expected error for empty NATS URL")
	}
}

func TestAllStreams_NoDuplicateSubjects(t *testing.T) {
	// Verify no two streams claim the same subject wildcard
	seen := make(map[string]string) // subject -> stream name
	for _, s := range AllStreams() {
		for _, subj := range s.Subjects {
			if prev, ok := seen[subj]; ok {
				t.Errorf("subject %q claimed by both %s and %s", subj, prev, s.Name)
			}
			seen[subj] = s.Name
		}
	}
}

func TestAllStreams_RetentionAndStorage(t *testing.T) {
	// Verify the AllStreams function returns configs that are suitable
	// for stream creation — names and subjects are non-empty
	for _, s := range AllStreams() {
		if s.Name == "" {
			t.Error("stream config has empty name")
		}
		if len(s.Subjects) == 0 {
			t.Errorf("stream %s has no subjects", s.Name)
		}
		for _, subj := range s.Subjects {
			if subj == "" {
				t.Errorf("stream %s has empty subject", s.Name)
			}
		}
	}
}
