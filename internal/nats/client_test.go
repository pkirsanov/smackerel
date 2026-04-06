package nats

import (
	"testing"
)

func TestAllStreams_Coverage(t *testing.T) {
	streams := AllStreams()
	if len(streams) != 3 {
		t.Fatalf("expected 3 streams, got %d", len(streams))
	}

	expected := map[string][]string{
		"ARTIFACTS": {"artifacts.>"},
		"SEARCH":    {"search.>"},
		"DIGEST":    {"digest.>"},
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
	// All subjects should follow domain.action pattern
	subjects := []string{
		SubjectArtifactsProcess, SubjectArtifactsProcessed,
		SubjectSearchEmbed, SubjectSearchEmbedded,
		SubjectSearchRerank, SubjectSearchReranked,
		SubjectDigestGenerate, SubjectDigestGenerated,
	}

	for _, s := range subjects {
		dotCount := 0
		for _, c := range s {
			if c == '.' {
				dotCount++
			}
		}
		if dotCount != 1 {
			t.Errorf("subject %q should have exactly 1 dot separator, got %d", s, dotCount)
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
