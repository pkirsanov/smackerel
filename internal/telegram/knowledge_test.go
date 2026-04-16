package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestHandleConcept_NoArgs_ListsTopConcepts tests T7-01: /concept with no args returns top 10 list.
func TestHandleConcept_NoArgs_ListsTopConcepts(t *testing.T) {
	response := conceptListResponse{
		Concepts: []conceptListItem{
			{ID: "1", Title: "Leadership", CitationCount: 8, SourceTypes: []string{"article", "video"}},
			{ID: "2", Title: "Productivity", CitationCount: 5, SourceTypes: []string{"article"}},
			{ID: "3", Title: "Remote Work", CitationCount: 3, SourceTypes: []string{"podcast"}},
		},
		Total: 3,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/knowledge/concepts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("sort") != "citations" {
			t.Errorf("expected sort=citations, got %s", r.URL.Query().Get("sort"))
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("expected limit=10, got %s", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handleConcept(t.Context(), msg, "")

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	reply := (*replies)[0]
	if !strings.Contains(reply, "Concept Pages") {
		t.Errorf("expected heading, got: %s", reply)
	}
	if !strings.Contains(reply, "Leadership (8 citations)") {
		t.Errorf("expected Leadership with 8 citations, got: %s", reply)
	}
	if !strings.Contains(reply, "Productivity (5 citations)") {
		t.Errorf("expected Productivity, got: %s", reply)
	}
}

// TestHandleConcept_WithName_ShowsDetail tests T7-02: /concept Leadership shows concept detail.
func TestHandleConcept_WithName_ShowsDetail(t *testing.T) {
	claims, _ := json.Marshal([]struct {
		Text       string `json:"text"`
		SourceType string `json:"source_type"`
	}{
		{Text: "Leadership requires trust", SourceType: "article"},
		{Text: "Servant leadership is effective", SourceType: "video"},
	})

	listResp := conceptListResponse{
		Concepts: []conceptListItem{
			{ID: "abc-123", Title: "Leadership", CitationCount: 8},
		},
		Total: 1,
	}
	detailResp := conceptDetail{
		ID:                  "abc-123",
		Title:               "Leadership",
		Summary:             "A comprehensive overview of leadership styles and practices.",
		Claims:              claims,
		RelatedConceptIDs:   []string{"def-456"},
		SourceArtifactIDs:   []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"},
		SourceTypeDiversity: []string{"article", "video", "podcast"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/abc-123") {
			json.NewEncoder(w).Encode(detailResp)
		} else {
			json.NewEncoder(w).Encode(listResp)
		}
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handleConcept(t.Context(), msg, "Leadership")

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	reply := (*replies)[0]
	if !strings.Contains(reply, "# Leadership") {
		t.Errorf("expected title heading, got: %s", reply)
	}
	if !strings.Contains(reply, "comprehensive overview") {
		t.Errorf("expected summary, got: %s", reply)
	}
	if !strings.Contains(reply, "Leadership requires trust") {
		t.Errorf("expected claim, got: %s", reply)
	}
	if !strings.Contains(reply, "8 citations") {
		t.Errorf("expected citation count, got: %s", reply)
	}
}

// TestHandleConcept_NotFound tests T7-03: /concept nonexistent returns not-found message.
func TestHandleConcept_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(conceptListResponse{Concepts: []conceptListItem{}, Total: 0})
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handleConcept(t.Context(), msg, "nonexistent")

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	if !strings.Contains((*replies)[0], "No concept page found for 'nonexistent'") {
		t.Errorf("expected not-found message, got: %s", (*replies)[0])
	}
}

// TestHandlePerson_WithName_ShowsProfile tests T7-04: /person Sarah shows entity profile.
func TestHandlePerson_WithName_ShowsProfile(t *testing.T) {
	mentions, _ := json.Marshal([]struct {
		ArtifactTitle string `json:"artifact_title"`
		MentionedAt   string `json:"mentioned_at"`
	}{
		{ArtifactTitle: "Meeting Notes Q1", MentionedAt: "2024-01-15"},
		{ArtifactTitle: "Leadership Workshop", MentionedAt: "2024-02-20"},
	})

	listResp := entityListResponse{
		Entities: []entityListItem{
			{ID: "ent-001", Name: "Sarah Chen", EntityType: "person", InteractionCount: 12},
		},
		Total: 1,
	}
	detailResp := entityDetail{
		ID:               "ent-001",
		Name:             "Sarah Chen",
		EntityType:       "person",
		Summary:          "Senior engineering manager known for leadership initiatives.",
		Mentions:         mentions,
		SourceTypes:      []string{"article", "meeting_notes"},
		InteractionCount: 12,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/ent-001") {
			json.NewEncoder(w).Encode(detailResp)
		} else {
			json.NewEncoder(w).Encode(listResp)
		}
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handlePerson(t.Context(), msg, "Sarah")

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	reply := (*replies)[0]
	if !strings.Contains(reply, "# Sarah Chen") {
		t.Errorf("expected name heading, got: %s", reply)
	}
	if !strings.Contains(reply, "Senior engineering manager") {
		t.Errorf("expected summary, got: %s", reply)
	}
	if !strings.Contains(reply, "12 mentions") {
		t.Errorf("expected mention count, got: %s", reply)
	}
	if !strings.Contains(reply, "Meeting Notes Q1") {
		t.Errorf("expected recent mention, got: %s", reply)
	}
}

// TestHandlePerson_NotFound tests /person with nonexistent name.
func TestHandlePerson_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entityListResponse{Entities: []entityListItem{}, Total: 0})
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handlePerson(t.Context(), msg, "nobody")

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	if !strings.Contains((*replies)[0], "No entity profile found for 'nobody'") {
		t.Errorf("expected not-found message, got: %s", (*replies)[0])
	}
}

// TestHandleLint_ShowsReport tests T7-05: /lint shows latest lint report.
func TestHandleLint_ShowsReport(t *testing.T) {
	summary, _ := json.Marshal(lintSummary{Total: 5, High: 1, Medium: 2, Low: 2})
	findings, _ := json.Marshal([]lintFinding{
		{Type: "contradiction", Severity: "high", TargetTitle: "Leadership", Description: "Conflicting claims about delegation"},
		{Type: "stale_concept", Severity: "medium", TargetTitle: "Remote Work", Description: "Not updated in 90+ days"},
	})

	report := lintReportResponse{
		ID:         "lr-001",
		RunAt:      "2024-03-15T03:00:00Z",
		DurationMs: 1234,
		Findings:   findings,
		Summary:    summary,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/knowledge/lint" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handleLint(t.Context(), msg)

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	reply := (*replies)[0]
	if !strings.Contains(reply, "Knowledge Lint Report") {
		t.Errorf("expected lint report heading, got: %s", reply)
	}
	if !strings.Contains(reply, "High: 1") {
		t.Errorf("expected high count, got: %s", reply)
	}
	if !strings.Contains(reply, "Medium: 2") {
		t.Errorf("expected medium count, got: %s", reply)
	}
	if !strings.Contains(reply, "Conflicting claims") {
		t.Errorf("expected finding description, got: %s", reply)
	}
}

// TestHandleLint_NoReport tests /lint when no report exists.
func TestHandleLint_NoReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handleLint(t.Context(), msg)

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	if (*replies)[0] != "> No lint report yet" {
		t.Errorf("expected no-report message, got: %s", (*replies)[0])
	}
}

// TestHandleConcept_EmptyList tests /concept when no concepts exist.
func TestHandleConcept_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(conceptListResponse{Concepts: []conceptListItem{}, Total: 0})
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handleConcept(t.Context(), msg, "")

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	if (*replies)[0] != "> No concept pages yet" {
		t.Errorf("expected empty list message, got: %s", (*replies)[0])
	}
}

// TestHandlePerson_NoArgs_ListsTopEntities tests /person with no args returns top 10 list.
func TestHandlePerson_NoArgs_ListsTopEntities(t *testing.T) {
	response := entityListResponse{
		Entities: []entityListItem{
			{ID: "1", Name: "Sarah Chen", EntityType: "person", InteractionCount: 12},
			{ID: "2", Name: "Google", EntityType: "organization", InteractionCount: 8},
		},
		Total: 2,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/knowledge/entities" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("sort") != "mentions" {
			t.Errorf("expected sort=mentions, got %s", r.URL.Query().Get("sort"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handlePerson(t.Context(), msg, "")

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	reply := (*replies)[0]
	if !strings.Contains(reply, "Entity Profiles") {
		t.Errorf("expected heading, got: %s", reply)
	}
	if !strings.Contains(reply, "Sarah Chen (person) - 12 mentions") {
		t.Errorf("expected Sarah entry, got: %s", reply)
	}
}

// TestHandleLint_ServiceUnavailable tests /lint when knowledge layer is disabled.
func TestHandleLint_ServiceUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	bot, replies := newTestKnowledgeBot(t, srv.URL)
	msg := newTestMessage(123, "")
	bot.handleLint(t.Context(), msg)

	if len(*replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(*replies))
	}
	if !strings.Contains((*replies)[0], "Knowledge layer is not enabled") {
		t.Errorf("expected service unavailable message, got: %s", (*replies)[0])
	}
}

// TestFormatConceptDetail_Structure tests T7-07: formatConceptDetail produces correct structure.
func TestFormatConceptDetail_Structure(t *testing.T) {
	claims, _ := json.Marshal([]struct {
		Text       string `json:"text"`
		SourceType string `json:"source_type"`
	}{
		{Text: "Leaders build trust", SourceType: "article"},
		{Text: "Delegation is key", SourceType: "video"},
	})

	c := conceptDetail{
		ID:                  "c1",
		Title:               "Leadership",
		Summary:             "Overview of leadership.",
		Claims:              claims,
		RelatedConceptIDs:   []string{"c2"},
		SourceArtifactIDs:   []string{"a1", "a2", "a3"},
		SourceTypeDiversity: []string{"article", "video"},
	}

	result := formatConceptDetail(c)

	expectations := []string{
		"# Leadership",
		"> Overview of leadership.",
		"Key Claims",
		"Leaders build trust (article)",
		"Delegation is key (video)",
		"1 related concepts",
		"Sources: article, video",
		"3 citations",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("expected %q in output, got:\n%s", exp, result)
		}
	}
}

// TestFormatLintReport_Structure tests formatLintReport produces correct structure.
func TestFormatLintReport_Structure(t *testing.T) {
	summary, _ := json.Marshal(lintSummary{Total: 3, High: 1, Medium: 1, Low: 1})
	findings, _ := json.Marshal([]lintFinding{
		{Type: "contradiction", Severity: "high", TargetTitle: "Topic A", Description: "Conflicting data"},
		{Type: "stale", Severity: "medium", TargetTitle: "Topic B", Description: "Old data"},
		{Type: "orphan", Severity: "low", TargetTitle: "Topic C", Description: "No connections"},
	})

	r := lintReportResponse{
		ID:       "lr-1",
		RunAt:    "2024-03-15T03:00:00Z",
		Summary:  summary,
		Findings: findings,
	}

	result := formatLintReport(r)

	expectations := []string{
		"Knowledge Lint Report",
		"Total findings: 3",
		"High: 1",
		"Medium: 1",
		"Low: 1",
		"[high] Topic A: Conflicting data",
		"[medium] Topic B: Old data",
		"[low] Topic C: No connections",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("expected %q in output, got:\n%s", exp, result)
		}
	}
}

// TestFormatEntityProfile_Structure tests formatEntityProfile produces correct structure.
func TestFormatEntityProfile_Structure(t *testing.T) {
	mentions, _ := json.Marshal([]struct {
		ArtifactTitle string `json:"artifact_title"`
		MentionedAt   string `json:"mentioned_at"`
	}{
		{ArtifactTitle: "Meeting Notes", MentionedAt: "2024-01-15"},
	})

	e := entityDetail{
		ID:               "e1",
		Name:             "Sarah",
		EntityType:       "person",
		Summary:          "A person.",
		Mentions:         mentions,
		SourceTypes:      []string{"article"},
		InteractionCount: 5,
	}

	result := formatEntityProfile(e)

	expectations := []string{
		"# Sarah (person)",
		"> A person.",
		"Source types: article",
		"5 mentions",
		"Recent Mentions",
		"Meeting Notes",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("expected %q in output, got:\n%s", exp, result)
		}
	}
}

// TestFormatKnowledgeMatch tests the knowledge match formatting for /find.
func TestFormatKnowledgeMatch(t *testing.T) {
	km := knowledgeMatchResponse{
		Title:         "Negotiation",
		Summary:       "Techniques for effective negotiation.",
		CitationCount: 4,
		SourceTypes:   []string{"article", "video"},
	}

	result := formatKnowledgeMatch(km)

	expectations := []string{
		"From Knowledge Layer: Negotiation",
		"Techniques for effective negotiation.",
		"4 citations from article, video",
		"/concept Negotiation for full page",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("expected %q in output, got:\n%s", exp, result)
		}
	}
}

// TestFormatConceptList_Structure tests formatConceptList produces correct structure.
func TestFormatConceptList_Structure(t *testing.T) {
	concepts := []conceptListItem{
		{ID: "1", Title: "Leadership", CitationCount: 8, SourceTypes: []string{"article", "video"}},
		{ID: "2", Title: "Productivity", CitationCount: 5, SourceTypes: []string{"article"}},
	}

	result := formatConceptList(concepts)

	expectations := []string{
		"Concept Pages",
		"1. Leadership (8 citations) [article, video]",
		"2. Productivity (5 citations) [article]",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("expected %q in output, got:\n%s", exp, result)
		}
	}
}

// TestFormatEntityList_Structure tests formatEntityList produces correct structure.
func TestFormatEntityList_Structure(t *testing.T) {
	entities := []entityListItem{
		{ID: "1", Name: "Sarah", EntityType: "person", InteractionCount: 12},
		{ID: "2", Name: "Google", EntityType: "organization", InteractionCount: 8},
	}

	result := formatEntityList(entities)

	expectations := []string{
		"Entity Profiles",
		"1. Sarah (person) - 12 mentions",
		"2. Google (organization) - 8 mentions",
	}

	for _, exp := range expectations {
		if !strings.Contains(result, exp) {
			t.Errorf("expected %q in output, got:\n%s", exp, result)
		}
	}
}

// TestHandleFind_WithKnowledgeMatch tests T7-06: /find with knowledge_match prepends provenance.
func TestHandleFind_WithKnowledgeMatch(t *testing.T) {
	searchResp := map[string]interface{}{
		"knowledge_match": map[string]interface{}{
			"title":          "Negotiation",
			"summary":        "Techniques for effective negotiation.",
			"citation_count": float64(4),
			"source_types":   []interface{}{"article", "video"},
		},
		"results": []interface{}{
			map[string]interface{}{
				"title":         "Negotiation Tips Article",
				"artifact_type": "article",
				"summary":       "Top negotiation strategies.",
			},
		},
		"total_candidates": float64(1),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResp)
	}))
	defer srv.Close()

	var replies []string
	bot := &Bot{
		searchURL:  srv.URL + "/api/search",
		authToken:  "test-token",
		httpClient: http.DefaultClient,
		replyFunc: func(chatID int64, text string) {
			replies = append(replies, text)
		},
	}

	msg := newTestMessage(123, "/find negotiation")
	bot.handleFind(t.Context(), msg, "negotiation")

	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	reply := replies[0]
	if !strings.Contains(reply, "From Knowledge Layer: Negotiation") {
		t.Errorf("expected knowledge match header, got: %s", reply)
	}
	if !strings.Contains(reply, "/concept Negotiation for full page") {
		t.Errorf("expected concept reference, got: %s", reply)
	}
	if !strings.Contains(reply, "Negotiation Tips Article") {
		t.Errorf("expected vector result, got: %s", reply)
	}
	// Knowledge match should appear BEFORE vector results
	kmIdx := strings.Index(reply, "From Knowledge Layer")
	vecIdx := strings.Index(reply, "Negotiation Tips Article")
	if kmIdx >= vecIdx {
		t.Errorf("knowledge match should appear before vector results: km=%d, vec=%d", kmIdx, vecIdx)
	}
}

// TestHandleFind_WithoutKnowledgeMatch tests /find without knowledge_match works as before.
func TestHandleFind_WithoutKnowledgeMatch(t *testing.T) {
	searchResp := map[string]interface{}{
		"results": []interface{}{
			map[string]interface{}{
				"title":         "Some Article",
				"artifact_type": "article",
				"summary":       "Article summary.",
			},
		},
		"total_candidates": float64(1),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResp)
	}))
	defer srv.Close()

	var replies []string
	bot := &Bot{
		searchURL:  srv.URL + "/api/search",
		authToken:  "test-token",
		httpClient: http.DefaultClient,
		replyFunc: func(chatID int64, text string) {
			replies = append(replies, text)
		},
	}

	msg := newTestMessage(123, "/find something")
	bot.handleFind(t.Context(), msg, "something")

	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	reply := replies[0]
	if strings.Contains(reply, "From Knowledge Layer") {
		t.Errorf("should not contain knowledge match, got: %s", reply)
	}
	if !strings.Contains(reply, "Some Article") {
		t.Errorf("expected regular result, got: %s", reply)
	}
}

// newTestKnowledgeBot creates a Bot wired to the given test server for knowledge handler testing.
// Returns the bot and a pointer to a string slice that captures all reply messages.
func newTestKnowledgeBot(t *testing.T, baseURL string) (*Bot, *[]string) {
	t.Helper()
	var replies []string
	bot := &Bot{
		knowledgeURL: baseURL + "/api/knowledge",
		authToken:    "test-token",
		httpClient:   http.DefaultClient,
		replyFunc: func(chatID int64, text string) {
			replies = append(replies, text)
		},
	}
	return bot, &replies
}

// newTestMessage creates a minimal tgbotapi.Message for testing.
func newTestMessage(chatID int64, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		Text: text,
	}
}
