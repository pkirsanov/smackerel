package nats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// contractFile holds the parsed nats_contract.json structure.
type contractFile struct {
	Subjects             map[string]contractSubject `json:"subjects"`
	Streams              map[string]contractStream  `json:"streams"`
	RequestResponsePairs []contractPair             `json:"request_response_pairs"`
}

type contractSubject struct {
	Direction string `json:"direction"`
	Response  string `json:"response,omitempty"`
	Request   string `json:"request,omitempty"`
	Stream    string `json:"stream"`
	Critical  bool   `json:"critical"`
}

type contractStream struct {
	SubjectsPattern string `json:"subjects_pattern"`
}

type contractPair struct {
	Request  string `json:"request"`
	Response string `json:"response"`
}

func loadContract(t *testing.T) contractFile {
	t.Helper()
	// Resolve path relative to this test file
	_, thisFile, _, _ := runtime.Caller(0)
	contractPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "nats_contract.json")
	data, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("failed to read nats_contract.json: %v", err)
	}
	var c contractFile
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("failed to parse nats_contract.json: %v", err)
	}
	return c
}

// TestSCN002054_GoSubjectsMatchContract verifies every Go subject constant
// matches the shared NATS contract file.
func TestSCN002054_GoSubjectsMatchContract(t *testing.T) {
	contract := loadContract(t)

	// Map Go constants to their expected string values
	goSubjects := map[string]string{
		"SubjectArtifactsProcess":   SubjectArtifactsProcess,
		"SubjectArtifactsProcessed": SubjectArtifactsProcessed,
		"SubjectSearchEmbed":        SubjectSearchEmbed,
		"SubjectSearchEmbedded":     SubjectSearchEmbedded,
		"SubjectSearchRerank":       SubjectSearchRerank,
		"SubjectSearchReranked":     SubjectSearchReranked,
		"SubjectDigestGenerate":     SubjectDigestGenerate,
		"SubjectDigestGenerated":    SubjectDigestGenerated,
		"SubjectKeepSyncRequest":    SubjectKeepSyncRequest,
		"SubjectKeepSyncResponse":   SubjectKeepSyncResponse,
		"SubjectKeepOCRRequest":     SubjectKeepOCRRequest,
		"SubjectKeepOCRResponse":    SubjectKeepOCRResponse,
		"SubjectLearningClassify":   SubjectLearningClassify,
		"SubjectLearningClassified": SubjectLearningClassified,
		"SubjectContentAnalyze":     SubjectContentAnalyze,
		"SubjectContentAnalyzed":    SubjectContentAnalyzed,
		"SubjectMonthlyGenerate":    SubjectMonthlyGenerate,
		"SubjectMonthlyGenerated":   SubjectMonthlyGenerated,
		"SubjectQuickrefGenerate":   SubjectQuickrefGenerate,
		"SubjectQuickrefGenerated":  SubjectQuickrefGenerated,
		"SubjectSeasonalAnalyze":    SubjectSeasonalAnalyze,
		"SubjectSeasonalAnalyzed":   SubjectSeasonalAnalyzed,
		"SubjectAlertsNotify":       SubjectAlertsNotify,
	}

	// Every Go constant must exist in the contract
	for name, value := range goSubjects {
		if _, ok := contract.Subjects[value]; !ok {
			t.Errorf("Go constant %s = %q not found in nats_contract.json subjects", name, value)
		}
	}

	// Every contract subject must have a matching Go constant
	for subject := range contract.Subjects {
		found := false
		for _, v := range goSubjects {
			if v == subject {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("contract subject %q has no matching Go constant in internal/nats/client.go", subject)
		}
	}
}

// TestSCN002054_GoStreamsMatchContract verifies every Go stream config
// matches the shared NATS contract file.
func TestSCN002054_GoStreamsMatchContract(t *testing.T) {
	contract := loadContract(t)
	goStreams := AllStreams()

	goStreamMap := make(map[string][]string)
	for _, s := range goStreams {
		goStreamMap[s.Name] = s.Subjects
	}

	// Every contract stream must exist in Go
	for name, cs := range contract.Streams {
		goSubjects, ok := goStreamMap[name]
		if !ok {
			t.Errorf("contract stream %q not found in Go AllStreams()", name)
			continue
		}
		found := false
		for _, s := range goSubjects {
			if s == cs.SubjectsPattern {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("contract stream %q pattern %q not found in Go stream subjects %v", name, cs.SubjectsPattern, goSubjects)
		}
	}

	// Every Go stream must exist in contract
	for name := range goStreamMap {
		if _, ok := contract.Streams[name]; !ok {
			t.Errorf("Go stream %q not found in nats_contract.json streams", name)
		}
	}
}

// TestSCN002054_GoSubjectPairsMatchContract verifies the request/response
// pairs in the contract match the Go constant pairs.
func TestSCN002054_GoSubjectPairsMatchContract(t *testing.T) {
	contract := loadContract(t)

	goPairs := []struct{ request, response string }{
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
	}

	if len(contract.RequestResponsePairs) != len(goPairs) {
		t.Fatalf("contract has %d pairs, Go has %d pairs", len(contract.RequestResponsePairs), len(goPairs))
	}

	contractPairSet := make(map[string]string)
	for _, p := range contract.RequestResponsePairs {
		contractPairSet[p.Request] = p.Response
	}

	for _, gp := range goPairs {
		resp, ok := contractPairSet[gp.request]
		if !ok {
			t.Errorf("Go pair %q -> %q: request not found in contract", gp.request, gp.response)
			continue
		}
		if resp != gp.response {
			t.Errorf("Go pair %q -> %q: contract says response should be %q", gp.request, gp.response, resp)
		}
	}
}
