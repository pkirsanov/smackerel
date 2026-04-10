package imap

import (
	"context"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- Chaos: Malformed message parsing ---

func TestChaos_ParseMessages_AllFieldsEmpty(t *testing.T) {
	c := New("chaos-imap")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{}, // completely empty message
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Empty UID message: cursor is "", code checks msg.UID <= cursor && cursor != "" → false → proceeds.
	// So the message with empty UID should produce an artifact with empty SourceRef
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact for empty-field message, got %d", len(artifacts))
	}
	// The artifact has empty SourceRef which is a data quality issue
	if artifacts[0].SourceRef != "" {
		t.Errorf("expected empty SourceRef for empty UID, got %q", artifacts[0].SourceRef)
	}
}

func TestChaos_ParseMessages_WrongTypes(t *testing.T) {
	// Fields with wrong types: numbers where strings expected, etc.
	c := New("chaos-types")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":     12345,      // int instead of string
					"from":    true,       // bool instead of string
					"subject": []int{1},   // array instead of string
					"body":    42,         // int instead of string
					"date":    12345,      // int instead of string
					"labels":  "not-list", // string instead of array
				},
			},
		},
	})

	// Should not panic — type assertions should fail gracefully
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync should not error on wrong types: %v", err)
	}
	// Message with all wrong types still gets created — fields are just empty
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact (defaults for wrong types), got %d", len(artifacts))
	}
}

func TestChaos_ParseMessages_NonArrayInput(t *testing.T) {
	c := New("chaos-nonarray")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": "not-an-array",
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when messages is not an array")
	}
}

func TestChaos_ParseMessages_NilMessages(t *testing.T) {
	c := New("chaos-nil")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": nil,
		},
	})

	// nil messages value should return empty results, not an error
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync with nil messages should not error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for nil messages, got %d", len(artifacts))
	}
}

func TestChaos_ParseMessages_MixedValidInvalid(t *testing.T) {
	c := New("chaos-mixed")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				"not-a-map",                                            // invalid
				42,                                                     // invalid
				nil,                                                    // invalid
				map[string]interface{}{"uid": "valid", "from": "a@b"}, // valid
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Only the valid message should produce an artifact
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from mixed input, got %d", len(artifacts))
	}
}

// --- Chaos: Email body edge cases ---

func TestChaos_Sync_VeryLargeEmailBody(t *testing.T) {
	// 5MB email body — no size limit is enforced on email body processing
	largeBody := strings.Repeat("Important information. ", 200000)
	c := New("chaos-large")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":  "big-1",
					"from": "sender@example.com",
					"body": largeBody,
					"date": "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync with large body: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact for large body")
	}
	// RawContent is the body — verify it's preserved
	if len(artifacts[0].RawContent) != len(largeBody) {
		t.Errorf("expected body length %d, got %d", len(largeBody), len(artifacts[0].RawContent))
	}
}

func TestChaos_Sync_EmptyBodyFallsBackToSubject(t *testing.T) {
	c := New("chaos-nob")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":     "nob-1",
					"from":    "sender@example.com",
					"subject": "Subject as content",
					"body":    "",
					"date":    "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact")
	}
	if artifacts[0].RawContent != "Subject as content" {
		t.Errorf("expected subject as fallback content, got %q", artifacts[0].RawContent)
	}
}

func TestChaos_Sync_BothBodyAndSubjectEmpty(t *testing.T) {
	c := New("chaos-empty-content")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":  "empty-1",
					"from": "sender@example.com",
					"date": "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Message with no body and no subject should still produce artifact with empty content
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].RawContent != "" {
		t.Errorf("expected empty content, got %q", artifacts[0].RawContent)
	}
}

// --- Chaos: Unicode in email fields ---

func TestChaos_Sync_UnicodeSubjectAndBody(t *testing.T) {
	c := New("chaos-unicode")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"uid":     "uni-1",
					"from":    "münchen@café.de",
					"subject": "🚀 Résumé — 日本語テスト",
					"body":    "Ñoño αβγδε مرحبا 你好世界 🌍\nAction: Bitte überprüfen by Freitag",
					"date":    "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact")
	}
	if !utf8.ValidString(artifacts[0].Title) {
		t.Error("title should be valid UTF-8")
	}
	if !utf8.ValidString(artifacts[0].RawContent) {
		t.Error("content should be valid UTF-8")
	}
}

// --- Chaos: Action item extraction edge cases ---

func TestChaos_ExtractActionItems_OnlyWhitespace(t *testing.T) {
	items := ExtractActionItems("   \n   \n   ")
	if len(items) != 0 {
		t.Errorf("expected 0 items from whitespace-only text, got %d", len(items))
	}
}

func TestChaos_ExtractActionItems_VeryLongLines(t *testing.T) {
	// Very long line that matches "please...by" pattern
	longLine := "Please " + strings.Repeat("process this very important request ", 1000) + "by Friday"
	items := ExtractActionItems(longLine)
	if len(items) != 1 {
		t.Errorf("expected 1 action item from long line, got %d", len(items))
	}
}

func TestChaos_ExtractActionItems_CaseSensitivity(t *testing.T) {
	text := "ACTION: uppercase action\nTODO: uppercase todo\nPlease review by Monday"
	items := ExtractActionItems(text)
	if len(items) != 3 {
		t.Errorf("expected 3 items (case-insensitive), got %d: %v", len(items), items)
	}
}

func TestChaos_ExtractActionItems_FalsePositives(t *testing.T) {
	// "please" + "by" in the same line but not really an action item
	text := "The painting was done by the artist please note this is for the gallery"
	items := ExtractActionItems(text)
	// This will match because it contains both "please" and "by"
	// This is a known false positive of the simple heuristic — document it
	if len(items) == 0 {
		t.Log("no false positives detected for 'please...by' heuristic — pattern may have been improved")
	}
}

func TestChaos_ExtractActionItems_NullBytes(t *testing.T) {
	text := "Action: do this\x00\nTodo: do that"
	items := ExtractActionItems(text)
	// Null bytes in text should not crash the extractor
	if len(items) < 1 {
		t.Error("expected at least 1 action item even with null bytes")
	}
}

func TestChaos_ExtractActionItems_CheckboxSyntax(t *testing.T) {
	text := "- [ ] Buy groceries\n- [x] Already done\n- [ ] Call dentist"
	items := ExtractActionItems(text)
	// Should match "- [ ]" prefix items
	if len(items) != 2 {
		t.Errorf("expected 2 unchecked checkbox items, got %d: %v", len(items), items)
	}
}

// --- Chaos: Concurrent Sync ---

func TestChaos_ConcurrentIMAPSync(t *testing.T) {
	c := New("chaos-concurrent")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{"uid": "100", "from": "a@b.com", "subject": "Test", "date": "2026-04-08T10:00:00Z"},
			},
		},
	})

	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, errs[idx] = c.Sync(context.Background(), "")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: sync error: %v", i, err)
		}
	}
}

// --- Chaos: Duplicate UIDs in message list ---

func TestChaos_Sync_DuplicateUIDs(t *testing.T) {
	c := New("chaos-dup-uid")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{"uid": "100", "from": "a@b.com", "subject": "First", "date": "2026-04-08T10:00:00Z"},
				map[string]interface{}{"uid": "100", "from": "a@b.com", "subject": "Duplicate", "date": "2026-04-08T11:00:00Z"},
				map[string]interface{}{"uid": "101", "from": "a@b.com", "subject": "Third", "date": "2026-04-08T12:00:00Z"},
			},
		},
	})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Both UID=100 messages pass through — connector has no dedup for source UIDs
	// This shows the connector relies on downstream pipeline dedup
	if len(artifacts) < 2 {
		t.Errorf("expected at least 2 artifacts (no connector-level dedup), got %d", len(artifacts))
	}
	if cursor != "101" {
		t.Errorf("expected cursor 101, got %q", cursor)
	}
}

// --- Chaos: Qualifier edge cases ---

func TestChaos_AssignTier_FromWithMultipleAtSigns(t *testing.T) {
	// Email with multiple @ signs (technically invalid but seen in wild)
	q := QualifierConfig{SkipDomains: []string{"spam.com"}}
	tier := AssignTier("weird@address@spam.com", nil, q)
	if tier != "skip" {
		t.Errorf("expected skip for domain match with multiple @, got %q", tier)
	}
}

func TestChaos_AssignTier_EmptyFrom(t *testing.T) {
	q := QualifierConfig{
		PrioritySenders: []string{"boss@example.com"},
		SkipDomains:     []string{"spam.com"},
	}
	tier := AssignTier("", nil, q)
	if tier != "standard" {
		t.Errorf("expected standard for empty from, got %q", tier)
	}
}

func TestChaos_AssignTier_NilLabels(t *testing.T) {
	q := QualifierConfig{SkipLabels: []string{"spam"}}
	// nil labels should not panic
	tier := AssignTier("user@example.com", nil, q)
	if tier != "standard" {
		t.Errorf("expected standard for nil labels, got %q", tier)
	}
}

func TestChaos_ParseQualifiers_EmptyMap(t *testing.T) {
	cfg := ParseQualifiers(map[string]interface{}{})
	if len(cfg.PrioritySenders) != 0 || len(cfg.SkipLabels) != 0 || len(cfg.PriorityLabels) != 0 {
		t.Error("empty qualifier map should produce empty config")
	}
}

func TestChaos_ParseQualifiers_WrongTypes(t *testing.T) {
	cfg := ParseQualifiers(map[string]interface{}{
		"priority_senders": "not-an-array",
		"skip_labels":      42,
		"priority_labels":  true,
	})
	// Should not panic on wrong types
	if len(cfg.PrioritySenders) != 0 {
		t.Error("wrong type should produce empty config")
	}
}

func TestChaos_ParseQualifiers_NilMap(t *testing.T) {
	cfg := ParseQualifiers(nil)
	if len(cfg.PrioritySenders) != 0 {
		t.Error("nil map should produce empty config")
	}
}
