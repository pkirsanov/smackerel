package imap

import (
	"context"
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
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}
	if cursor != "102" {
		t.Errorf("expected cursor 102, got %q", cursor)
	}

	// Verify boss email gets full tier
	bossArtifact := artifacts[2]
	if bossArtifact.Metadata["processing_tier"] != "full" {
		t.Errorf("expected full tier for boss email, got %v", bossArtifact.Metadata["processing_tier"])
	}

	// Verify promotions email gets metadata tier
	promoArtifact := artifacts[1]
	if promoArtifact.Metadata["processing_tier"] != "metadata" {
		t.Errorf("expected metadata tier for promotions, got %v", promoArtifact.Metadata["processing_tier"])
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
		t.Errorf("expected 3 action items, got %d: %v", len(items), items)
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
	if tier != "metadata" {
		t.Errorf("expected metadata tier for skip label, got %q", tier)
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
