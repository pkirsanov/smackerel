package imap

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnector_Interface(t *testing.T) {
	var _ connector.Connector = New("test-imap")
}

func TestConnector_Connect(t *testing.T) {
	c := New("gmail-imap")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy after connect")
	}
}

func TestConnector_Connect_InvalidAuth(t *testing.T) {
	c := New("test-imap")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "invalid",
	})
	if err == nil {
		t.Error("expected error for invalid auth type")
	}
}

func TestConnector_Close(t *testing.T) {
	c := New("test-imap")
	c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})
	c.Close()
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("expected disconnected after close")
	}
}

func TestSync_WithMessages(t *testing.T) {
	c := New("gmail-imap")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":     "100",
					"from":    "alice@example.com",
					"subject": "Project Update",
					"body":    "Here is the Q4 report.\nAction: Please review by Friday",
					"date":    "2026-04-01T10:00:00Z",
					"labels":  []interface{}{"work", "important"},
				},
				map[string]interface{}{
					"uid":     "101",
					"from":    "newsletter@spam.com",
					"subject": "Weekly Deals",
					"body":    "50% off everything",
					"date":    "2026-04-01T11:00:00Z",
					"labels":  []interface{}{"promotions"},
				},
				map[string]interface{}{
					"uid":     "102",
					"from":    "boss@example.com",
					"subject": "Urgent: Budget Review",
					"body":    "We need to discuss the budget.\nDeadline: Monday morning",
					"date":    "2026-04-01T12:00:00Z",
					"labels":  []interface{}{"work"},
				},
			},
		},
		Qualifiers: map[string]interface{}{
			"priority_senders": []interface{}{"boss@example.com"},
			"skip_labels":      []interface{}{"promotions"},
		},
	})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	// SCN-003-008: promotions email is skipped entirely (skip_labels → "skip" tier)
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts (promotions skipped), got %d", len(artifacts))
	}
	if cursor != "102" {
		t.Errorf("expected cursor 102, got %q", cursor)
	}

	// Verify boss email gets full tier
	bossArtifact := artifacts[1]
	if bossArtifact.Metadata["processing_tier"] != "full" {
		t.Errorf("expected full tier for boss email, got %v", bossArtifact.Metadata["processing_tier"])
	}

	// Verify standard email gets standard tier
	stdArtifact := artifacts[0]
	if stdArtifact.Metadata["processing_tier"] != "standard" {
		t.Errorf("expected standard tier for regular email, got %v", stdArtifact.Metadata["processing_tier"])
	}
}

func TestSync_CursorAdvancement(t *testing.T) {
	c := New("test-imap")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{"uid": "100", "from": "a@b.com", "subject": "Old", "date": "2026-04-01T10:00:00Z"},
				map[string]interface{}{"uid": "101", "from": "a@b.com", "subject": "New", "date": "2026-04-01T11:00:00Z"},
			},
		},
	})

	artifacts, cursor, err := c.Sync(context.Background(), "100")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact after cursor, got %d", len(artifacts))
	}
	if cursor != "101" {
		t.Errorf("expected cursor 101, got %q", cursor)
	}
	if artifacts[0].Title != "New" {
		t.Errorf("expected 'New', got %q", artifacts[0].Title)
	}
}

func TestSync_EmptySource(t *testing.T) {
	c := New("test-imap")
	c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}
}

func TestExtractActionItems_Patterns(t *testing.T) {
	text := "Let's discuss the project.\nAction: Review the proposal by Wednesday\nTodo: Update budget spreadsheet\nPlease send the report by Friday\nNo action needed here."
	items := ExtractActionItems(text)
	if len(items) != 3 {
		t.Fatalf("expected 3 action items, got %d: %v", len(items), items)
	}

	// SCN-003-007: Verify extracted content matches the actual commitment text.
	found := map[string]bool{
		"action":   false,
		"todo":     false,
		"sendverb": false,
	}
	for _, item := range items {
		lower := strings.ToLower(item)
		switch {
		case strings.Contains(lower, "review the proposal"):
			found["action"] = true
		case strings.Contains(lower, "update budget"):
			found["todo"] = true
		case strings.Contains(lower, "send the report"):
			found["sendverb"] = true
		}
	}
	for tag, ok := range found {
		if !ok {
			t.Errorf("expected action item %q to be extracted from text; got items: %v", tag, items)
		}
	}
}

func TestExtractActionItems_Empty(t *testing.T) {
	items := ExtractActionItems("")
	if items != nil {
		t.Errorf("expected nil for empty text, got %v", items)
	}
}

func TestAssignTier_PrioritySender(t *testing.T) {
	q := QualifierConfig{PrioritySenders: []string{"boss@example.com"}}
	tier := AssignTier("boss@example.com", nil, q)
	if tier != "full" {
		t.Errorf("expected full tier for priority sender, got %q", tier)
	}
}

func TestAssignTier_SkipLabel(t *testing.T) {
	q := QualifierConfig{SkipLabels: []string{"promotions"}}
	tier := AssignTier("someone@example.com", []string{"promotions"}, q)
	if tier != "skip" {
		t.Errorf("expected skip tier for skip label, got %q", tier)
	}
}

func TestAssignTier_SkipDomain(t *testing.T) {
	q := QualifierConfig{SkipDomains: []string{"spam.com"}}
	tier := AssignTier("newsletter@spam.com", nil, q)
	if tier != "skip" {
		t.Errorf("expected skip tier for skip domain, got %q", tier)
	}
}

func TestAssignTier_Standard(t *testing.T) {
	q := QualifierConfig{}
	tier := AssignTier("someone@example.com", nil, q)
	if tier != "standard" {
		t.Errorf("expected standard tier, got %q", tier)
	}
}

func TestAssignTier_Default(t *testing.T) {
	tier := AssignTier("someone@example.com", nil, QualifierConfig{})
	if tier != "standard" {
		t.Errorf("expected standard tier by default, got %q", tier)
	}
}

func TestAssignTier_PriorityLabel(t *testing.T) {
	q := QualifierConfig{PriorityLabels: []string{"important", "urgent"}}
	tier := AssignTier("someone@example.com", []string{"urgent"}, q)
	if tier != "full" {
		t.Errorf("expected full tier for priority label, got %q", tier)
	}
}

func TestAssignTier_SkipDomainTakesPrecedence(t *testing.T) {
	q := QualifierConfig{
		PrioritySenders: []string{"vip@spam.com"},
		SkipDomains:     []string{"spam.com"},
	}
	tier := AssignTier("vip@spam.com", nil, q)
	if tier != "skip" {
		t.Errorf("skip domain should override priority sender, got %q", tier)
	}
}

func TestAssignTier_SkipLabelBeforePrioritySender(t *testing.T) {
	q := QualifierConfig{
		PrioritySenders: []string{"boss@example.com"},
		SkipLabels:      []string{"promotions"},
	}
	tier := AssignTier("boss@example.com", []string{"promotions"}, q)
	if tier != "skip" {
		t.Errorf("skip label should override priority sender, got %q", tier)
	}
}

func TestConnect_PasswordAuth(t *testing.T) {
	c := New("imap-pw")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "password",
	})
	if err != nil {
		t.Fatalf("expected password auth to succeed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy after password connect")
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New("imap-1")
	if c.ID() != "imap-1" {
		t.Errorf("expected ID 'imap-1', got %q", c.ID())
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected before connect, got %v", c.Health(context.Background()))
	}
}

func TestParseQualifiers_AllFields(t *testing.T) {
	q := ParseQualifiers(map[string]interface{}{
		"priority_senders": []interface{}{"a@b.com"},
		"skip_labels":      []interface{}{"spam", "promos"},
		"priority_labels":  []interface{}{"important"},
	})
	if len(q.PrioritySenders) != 1 {
		t.Errorf("expected 1 priority sender, got %d", len(q.PrioritySenders))
	}
	if len(q.SkipLabels) != 2 {
		t.Errorf("expected 2 skip labels, got %d", len(q.SkipLabels))
	}
	if len(q.PriorityLabels) != 1 {
		t.Errorf("expected 1 priority label, got %d", len(q.PriorityLabels))
	}
}

func TestParseQualifiers_Empty(t *testing.T) {
	q := ParseQualifiers(map[string]interface{}{})
	if len(q.PrioritySenders) != 0 || len(q.SkipLabels) != 0 || len(q.PriorityLabels) != 0 {
		t.Error("expected empty qualifiers")
	}
}

func TestParseQualifiers_NilMap(t *testing.T) {
	q := ParseQualifiers(nil)
	if len(q.PrioritySenders) != 0 {
		t.Error("expected empty qualifiers for nil map")
	}
}

func TestExtractActionItems_CheckboxPattern(t *testing.T) {
	text := "Tasks:\n- [ ] Complete the review\n- [x] Already done"
	items := ExtractActionItems(text)
	if len(items) != 1 {
		t.Errorf("expected 1 checkbox action item, got %d: %v", len(items), items)
	}
}

func TestExtractActionItems_DeadlinePattern(t *testing.T) {
	text := "Report due.\nDeadline: Friday end of day\nNothing else."
	items := ExtractActionItems(text)
	if len(items) != 1 {
		t.Errorf("expected 1 deadline item, got %d: %v", len(items), items)
	}
}

func TestExtractActionItems_TodoPattern(t *testing.T) {
	text := "Notes:\nTodo: file the expense report\nRegular text."
	items := ExtractActionItems(text)
	if len(items) != 1 {
		t.Errorf("expected 1 todo item, got %d: %v", len(items), items)
	}
}

func TestSync_ThreadDetection(t *testing.T) {
	c := New("imap-thread")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":         "200",
					"from":        "dev@example.com",
					"subject":     "Re: Design Discussion",
					"body":        "I agree with the changes.",
					"date":        "2026-04-08T10:00:00Z",
					"in_reply_to": "msg-100@example.com",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	m := artifacts[0].Metadata
	if m["in_reply_to"] != "msg-100@example.com" {
		t.Errorf("in_reply_to: %v", m["in_reply_to"])
	}
	if m["is_thread"] != true {
		t.Errorf("expected is_thread=true, got %v", m["is_thread"])
	}
}

func TestSync_SkipDomainFiltering(t *testing.T) {
	c := New("imap-skip")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":     "300",
					"from":    "bot@noreply.example.com",
					"subject": "Automated notification",
					"date":    "2026-04-08T10:00:00Z",
				},
				map[string]interface{}{
					"uid":     "301",
					"from":    "human@example.com",
					"subject": "Real email",
					"date":    "2026-04-08T11:00:00Z",
				},
			},
		},
		Qualifiers: map[string]interface{}{
			"skip_domains": []interface{}{"noreply.example.com"},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// skip_domains should filter out bot@noreply.example.com via AssignTier → "skip"
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact (skip domain filtered), got %d", len(artifacts))
	}
	if artifacts[0].Title != "Real email" {
		t.Errorf("expected 'Real email', got %q", artifacts[0].Title)
	}
}

func TestSync_ActionItemsInMetadata(t *testing.T) {
	c := New("imap-actions")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":     "400",
					"from":    "pm@example.com",
					"subject": "Project Tasks",
					"body":    "Update:\nAction: Complete the design doc\nTodo: Review PR #42",
					"date":    "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	items, ok := artifacts[0].Metadata["action_items"].([]string)
	if !ok {
		t.Fatal("expected action_items in metadata")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 action items, got %d: %v", len(items), items)
	}
}

func TestSync_EmptyBodyFallsBackToSubject(t *testing.T) {
	c := New("imap-nobody")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":     "500",
					"from":    "sender@example.com",
					"subject": "Subject as Content",
					"date":    "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if artifacts[0].RawContent != "Subject as Content" {
		t.Errorf("expected subject as content, got %q", artifacts[0].RawContent)
	}
}

func TestParseEmailMessages_InvalidInput(t *testing.T) {
	_, err := parseEmailMessages("not-an-array")
	if err == nil {
		t.Error("expected error for non-array input")
	}
}

func TestParseEmailMessages_SkipsNonMapEntries(t *testing.T) {
	msgs, err := parseEmailMessages([]interface{}{
		"not-a-map",
		map[string]interface{}{"uid": "valid", "from": "a@b.com", "subject": "Valid"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestParseEmailMessages_AllFields(t *testing.T) {
	msgs, err := parseEmailMessages([]interface{}{
		map[string]interface{}{
			"uid":         "msg-1",
			"message_id":  "mid-1",
			"from":        "sender@test.com",
			"subject":     "Full Message",
			"body":        "Body text",
			"date":        "2026-04-08T10:00:00Z",
			"labels":      []interface{}{"inbox", "work"},
			"to":          []interface{}{"recv@test.com"},
			"in_reply_to": "prev-msg",
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := msgs[0]
	if m.MessageID != "mid-1" {
		t.Errorf("message_id: %q", m.MessageID)
	}
	if m.InReplyTo != "prev-msg" {
		t.Errorf("in_reply_to: %q", m.InReplyTo)
	}
	if len(m.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(m.Labels))
	}
	if len(m.To) != 1 {
		t.Errorf("expected 1 To, got %d", len(m.To))
	}
}

func TestGetCredential_NilMap(t *testing.T) {
	if v := getCredential(nil, "key"); v != "" {
		t.Errorf("expected empty for nil map, got %q", v)
	}
}

func TestGetStr_NonStringValue(t *testing.T) {
	m := map[string]interface{}{"num": 42}
	if v := getStr(m, "num"); v != "" {
		t.Errorf("expected empty for non-string, got %q", v)
	}
}

func TestSync_ErrorOnInvalidMessages(t *testing.T) {
	c := New("imap-err")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": "not-an-array",
		},
	})
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for invalid messages format")
	}
}

func TestSync_HealthStaysErrorOnFailure(t *testing.T) {
	c := New("imap-health")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": "not-an-array", // triggers parse error
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected sync error for invalid messages")
	}

	// Health must remain at Error, not reset to Healthy by the defer
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("health should be Error after failed sync, got %v", c.Health(context.Background()))
	}
}

func TestSync_BeforeConnect(t *testing.T) {
	c := New("imap-no-connect")
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when Sync called before Connect")
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("health should remain disconnected, got %v", c.Health(context.Background()))
	}
}

func TestParseQualifiers_SkipDomains(t *testing.T) {
	q := ParseQualifiers(map[string]interface{}{
		"skip_domains": []interface{}{"spam.com", "noreply.example.com"},
	})
	if len(q.SkipDomains) != 2 {
		t.Errorf("expected 2 skip domains, got %d", len(q.SkipDomains))
	}
	if q.SkipDomains[0] != "spam.com" {
		t.Errorf("expected skip domain 'spam.com', got %q", q.SkipDomains[0])
	}
}

func TestParseEmailMessages_SkipsEmptyUID(t *testing.T) {
	msgs, err := parseEmailMessages([]interface{}{
		map[string]interface{}{"uid": "", "from": "a@b.com", "subject": "No UID"},
		map[string]interface{}{"uid": "valid", "from": "a@b.com", "subject": "Valid"},
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (empty UID skipped), got %d", len(msgs))
	}
	if msgs[0].UID != "valid" {
		t.Errorf("expected UID 'valid', got %q", msgs[0].UID)
	}
}

// I-IMPROVE-001: compareUIDs uses numeric ordering for IMAP UIDs.
func TestCompareUIDs_Numeric(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1", "2", -1},
		{"10", "2", 1},   // string compare would say "10" < "2"
		{"9", "100", -1}, // string compare would say "9" > "100"
		{"100", "100", 0},
		{"999", "1000", -1},
	}
	for _, tt := range tests {
		got := compareUIDs(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareUIDs(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareUIDs_NonNumericFallback(t *testing.T) {
	// Gmail API message IDs are hex-like strings; fall back to lexicographic
	if got := compareUIDs("abc", "def"); got >= 0 {
		t.Errorf("expected abc < def, got %d", got)
	}
	if got := compareUIDs("same", "same"); got != 0 {
		t.Errorf("expected 0 for equal strings, got %d", got)
	}
}

// I-IMPROVE-002: AssignTier is case-insensitive for labels and senders.
func TestAssignTier_CaseInsensitiveSender(t *testing.T) {
	q := QualifierConfig{PrioritySenders: []string{"Boss@Example.COM"}}
	tier := AssignTier("boss@example.com", nil, q)
	if tier != "full" {
		t.Errorf("expected full for case-insensitive sender match, got %q", tier)
	}
}

func TestAssignTier_CaseInsensitiveLabel(t *testing.T) {
	q := QualifierConfig{SkipLabels: []string{"Promotions"}}
	tier := AssignTier("a@b.com", []string{"PROMOTIONS"}, q)
	if tier != "skip" {
		t.Errorf("expected skip for case-insensitive label match, got %q", tier)
	}
}

func TestAssignTier_CaseInsensitiveDomain(t *testing.T) {
	q := QualifierConfig{SkipDomains: []string{"Spam.COM"}}
	tier := AssignTier("newsletter@spam.com", nil, q)
	if tier != "skip" {
		t.Errorf("expected skip for case-insensitive domain match, got %q", tier)
	}
}
