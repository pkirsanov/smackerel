package keep

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// ===========================================================================
// REG-001: Archived+pinned notes must NOT be skipped under default config.
//
// R-008 evaluation order: trashed→skip, pinned→full, labeled→full, ...
// The shouldSkip filter must honour this priority: pinned overrides archived.
// If this test fails, it means shouldSkip is dropping high-signal notes that
// the user explicitly pinned or labeled before archiving.
// ===========================================================================

func TestRegression_PinnedArchivedNotesNotSkipped(t *testing.T) {
	// Default config has IncludeArchived=false
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title:                   "Pinned and Archived",
		TextContent:             "Important note that was pinned then archived — should NOT be dropped",
		IsPinned:                true,
		IsArchived:              true,
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "pinned-archived", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("REGRESSION: pinned+archived note was skipped — pinned should override archived exclusion per R-008")
	}
	if artifact.Metadata["processing_tier"] != string(TierFull) {
		t.Errorf("REGRESSION: pinned+archived note tier = %q, want full", artifact.Metadata["processing_tier"])
	}
}

func TestRegression_LabeledArchivedNotesNotSkipped(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title:                   "Labeled and Archived",
		TextContent:             "Work note that was labeled then archived — should NOT be dropped",
		IsArchived:              true,
		Labels:                  []TakeoutLabel{{Name: "Work"}, {Name: "Important"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "labeled-archived", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("REGRESSION: labeled+archived note was skipped — labels should override archived exclusion per R-008")
	}
	if artifact.Metadata["processing_tier"] != string(TierFull) {
		t.Errorf("REGRESSION: labeled+archived note tier = %q, want full", artifact.Metadata["processing_tier"])
	}
}

func TestRegression_ImageArchivedNotesNotSkipped(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title:      "Photo Note Archived",
		IsArchived: true,
		Attachments: []TakeoutAttachment{
			{FilePath: "whiteboard.jpg", MimeType: "image/jpeg"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "image-archived", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("REGRESSION: image+archived note was skipped — image attachments should override archived exclusion per R-008")
	}
	if artifact.Metadata["processing_tier"] != string(TierFull) {
		t.Errorf("REGRESSION: image+archived note tier = %q, want full", artifact.Metadata["processing_tier"])
	}
}

// Plain archived notes without pinned/labeled/image flags SHOULD still be skipped
// when IncludeArchived=false. This is the regression counterpart: removing the
// archived filter entirely would cause this to fail.
func TestRegression_PlainArchivedNotesStillSkipped(t *testing.T) {
	n := NewNormalizer(KeepConfig{IncludeArchived: false})

	note := &TakeoutNote{
		Title:                   "Boring Archived Note",
		TextContent:             "Just an old archived note with no special signals",
		IsArchived:              true,
		UserEditedTimestampUsec: time.Now().Add(-90 * 24 * time.Hour).UnixMicro(),
		CreatedTimestampUsec:    time.Now().Add(-180 * 24 * time.Hour).UnixMicro(),
	}

	artifact, err := n.Normalize(note, "plain-archived", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact != nil {
		t.Fatal("REGRESSION: plain archived note (no pin/label/image) should be skipped when IncludeArchived=false")
	}
}

// Trashed notes must ALWAYS be skipped regardless of other flags.
// This is the highest-priority rule in R-008.
func TestRegression_TrashedPinnedLabeledAlwaysSkipped(t *testing.T) {
	n := NewNormalizer(KeepConfig{IncludeArchived: true})

	note := &TakeoutNote{
		Title:                   "Trashed but Pinned and Labeled",
		TextContent:             "Should still be skipped because trashed overrides everything",
		IsTrashed:               true,
		IsPinned:                true,
		Labels:                  []TakeoutLabel{{Name: "Important"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "trashed-pinned", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact != nil {
		t.Fatal("REGRESSION: trashed note should ALWAYS be skipped, even if pinned and labeled")
	}
}

// ===========================================================================
// REG-002: Cursor boundary — exactly-at-cursor notes are correctly excluded.
//
// FilterByCursor uses strict After() to prevent duplicate processing. Notes
// modified at exactly the cursor timestamp must be excluded. If someone
// changes After() to >= without adding dedup, this test catches it.
// ===========================================================================

func TestRegression_CursorBoundaryExcludesExactMatch(t *testing.T) {
	parser := NewTakeoutParser()

	// All notes share the same timestamp
	ts := int64(1712000000000000) // 2024-04-02 approx
	notes := []TakeoutNote{
		{TextContent: "Note A", UserEditedTimestampUsec: ts, SourceFile: "a.json"},
		{TextContent: "Note B", UserEditedTimestampUsec: ts, SourceFile: "b.json"},
		{TextContent: "Note C", UserEditedTimestampUsec: ts, SourceFile: "c.json"},
	}

	// First sync: empty cursor returns all
	filtered, cursor := parser.FilterByCursor(notes, "")
	if len(filtered) != 3 {
		t.Fatalf("first sync: filtered = %d, want 3", len(filtered))
	}
	if cursor == "" {
		t.Fatal("cursor should not be empty after first sync")
	}

	// Second sync with the returned cursor — all notes are at cursor time, so 0 returned
	filtered2, _ := parser.FilterByCursor(notes, cursor)
	if len(filtered2) != 0 {
		t.Errorf("REGRESSION: re-sync returned %d notes, want 0 (cursor boundary exclusion)", len(filtered2))
	}
}

func TestRegression_CursorAdvancesMonotonically(t *testing.T) {
	parser := NewTakeoutParser()

	notes := []TakeoutNote{
		{TextContent: "Old", UserEditedTimestampUsec: 1711000000000000, SourceFile: "old.json"},
		{TextContent: "New", UserEditedTimestampUsec: 1713000000000000, SourceFile: "new.json"},
	}

	_, cursor1 := parser.FilterByCursor(notes, "")

	// Add even newer note
	notes = append(notes, TakeoutNote{
		TextContent:             "Newest",
		UserEditedTimestampUsec: 1714000000000000,
		SourceFile:              "newest.json",
	})

	filtered, cursor2 := parser.FilterByCursor(notes, cursor1)
	if len(filtered) != 1 {
		t.Errorf("incremental sync: filtered = %d, want 1 (only newest)", len(filtered))
	}
	if cursor2 <= cursor1 {
		t.Errorf("REGRESSION: cursor did not advance: %q <= %q", cursor2, cursor1)
	}
}

// ===========================================================================
// REG-003: NormalizeGkeep preserves content type classification.
//
// GkeepNote → TakeoutNote conversion must carry list items so that
// gkeepapi-sourced checklist notes get classified as note/checklist.
// ===========================================================================

func TestRegression_NormalizeGkeepChecklistType(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	gkeep := &GkeepNote{
		NoteID:      "gkeep-checklist-1",
		Title:       "Shopping List",
		TextContent: "",
		ListItems: []struct {
			Text      string `json:"text"`
			IsChecked bool   `json:"is_checked"`
		}{
			{Text: "Milk", IsChecked: false},
			{Text: "Eggs", IsChecked: true},
			{Text: "Bread", IsChecked: false},
		},
		ModifiedUsec: time.Now().UnixMicro(),
		CreatedUsec:  time.Now().Add(-24 * time.Hour).UnixMicro(),
	}

	artifact, err := n.NormalizeGkeep(gkeep)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	if artifact == nil {
		t.Fatal("REGRESSION: gkeepapi checklist note produced nil artifact")
	}
	if artifact.ContentType != "note/checklist" {
		t.Errorf("REGRESSION: gkeepapi checklist ContentType = %q, want note/checklist", artifact.ContentType)
	}
	if !strings.Contains(artifact.RawContent, "- [x] Eggs") {
		t.Errorf("REGRESSION: checked item missing from gkeepapi content: %q", artifact.RawContent)
	}
	if !strings.Contains(artifact.RawContent, "- [ ] Milk") {
		t.Errorf("REGRESSION: unchecked item missing from gkeepapi content: %q", artifact.RawContent)
	}
}

func TestRegression_NormalizeGkeepPreservesLabels(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	gkeep := &GkeepNote{
		NoteID:       "gkeep-labeled-1",
		Title:        "Labeled Note from API",
		TextContent:  "This note has labels from the gkeepapi bridge",
		Labels:       []string{"Work", "ML", "Ideas"},
		ModifiedUsec: time.Now().UnixMicro(),
		CreatedUsec:  time.Now().Add(-24 * time.Hour).UnixMicro(),
	}

	artifact, err := n.NormalizeGkeep(gkeep)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	labels, ok := artifact.Metadata["labels"].([]string)
	if !ok {
		t.Fatal("REGRESSION: labels metadata is not []string")
	}
	if len(labels) != 3 {
		t.Errorf("REGRESSION: gkeepapi labels = %v, want [Work ML Ideas]", labels)
	}
	if artifact.Metadata["processing_tier"] != string(TierFull) {
		t.Errorf("REGRESSION: labeled gkeepapi note tier = %q, want full", artifact.Metadata["processing_tier"])
	}
}

func TestRegression_NormalizeGkeepSourcePath(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	gkeep := &GkeepNote{
		NoteID:       "gkeep-src-1",
		Title:        "Source Path Test",
		TextContent:  "Should have source_path = gkeepapi",
		ModifiedUsec: time.Now().UnixMicro(),
		CreatedUsec:  time.Now().UnixMicro(),
	}

	artifact, err := n.NormalizeGkeep(gkeep)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if artifact.Metadata["source_path"] != "gkeepapi" {
		t.Errorf("REGRESSION: gkeepapi source_path = %q, want gkeepapi", artifact.Metadata["source_path"])
	}
	if artifact.SourceRef != "gkeep-src-1" {
		t.Errorf("REGRESSION: gkeepapi SourceRef = %q, want gkeep-src-1", artifact.SourceRef)
	}
}

// ===========================================================================
// REG-004: shouldSkip and qualifier priority alignment.
//
// The shouldSkip filter and the qualifier engine must agree on priority:
// trashed > pinned > labeled > images > archived > recency.
// If shouldSkip evaluates archived before pinned/labeled, high-signal
// notes get silently dropped.
// ===========================================================================

func TestRegression_ShouldSkipRespectsQualifierPriority(t *testing.T) {
	cases := []struct {
		name       string
		note       TakeoutNote
		config     KeepConfig
		wantSkip   bool
		wantReason string
	}{
		{
			name:       "trashed overrides everything",
			note:       TakeoutNote{IsTrashed: true, IsPinned: true, TextContent: "x", UserEditedTimestampUsec: time.Now().UnixMicro()},
			config:     KeepConfig{},
			wantSkip:   true,
			wantReason: "trashed is highest priority",
		},
		{
			name:       "pinned overrides archived exclusion",
			note:       TakeoutNote{IsPinned: true, IsArchived: true, TextContent: "pinned content", UserEditedTimestampUsec: time.Now().UnixMicro()},
			config:     KeepConfig{IncludeArchived: false},
			wantSkip:   false,
			wantReason: "pinned notes must survive archived filter",
		},
		{
			name:       "labeled overrides archived exclusion",
			note:       TakeoutNote{IsArchived: true, Labels: []TakeoutLabel{{Name: "Work"}}, TextContent: "labeled content", UserEditedTimestampUsec: time.Now().UnixMicro()},
			config:     KeepConfig{IncludeArchived: false},
			wantSkip:   false,
			wantReason: "labeled notes must survive archived filter",
		},
		{
			name:       "plain archived skipped when IncludeArchived=false",
			note:       TakeoutNote{IsArchived: true, TextContent: "plain archived", UserEditedTimestampUsec: time.Now().UnixMicro()},
			config:     KeepConfig{IncludeArchived: false},
			wantSkip:   true,
			wantReason: "plain archived notes should be excluded by default",
		},
		{
			name:       "archived included when IncludeArchived=true",
			note:       TakeoutNote{IsArchived: true, TextContent: "archived but included", UserEditedTimestampUsec: time.Now().UnixMicro()},
			config:     KeepConfig{IncludeArchived: true},
			wantSkip:   false,
			wantReason: "archived notes pass through when config allows",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNormalizer(tt.config)
			content := n.buildContent(&tt.note)
			skipped := n.shouldSkip(&tt.note, content)
			if skipped != tt.wantSkip {
				t.Errorf("REGRESSION: shouldSkip = %v, want %v (%s)", skipped, tt.wantSkip, tt.wantReason)
			}
		})
	}
}

// ===========================================================================
// REG-005: Full sync cycle regression — end-to-end connector behavior.
//
// Verifies the full Takeout sync flow respects shouldSkip priority rules,
// confirming that the connector-level Sync doesn't introduce additional
// filtering that could mask the normalizer's decisions.
// ===========================================================================

func TestRegression_SyncPreservesPinnedArchivedNotes(t *testing.T) {
	dir := t.TempDir()
	// Pinned + archived note — must appear in results under default config
	writeTestJSON(t, dir, "pinned-archived.json", `{
		"color": "RED", "isTrashed": false, "isPinned": true, "isArchived": true,
		"textContent": "Critical idea that was pinned then archived — must not be lost",
		"title": "Pinned Archived Idea", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [{"name": "Ideas"}], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)
	// Plain archived note — must NOT appear in results under default config
	writeTestJSON(t, dir, "plain-archived.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": true,
		"textContent": "Boring old archived note with no special flags at all",
		"title": "Old Note", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)
	// Active note — must appear
	writeTestJSON(t, dir, "active.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Regular active note that should always appear in sync results",
		"title": "Active Note", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	if err := c.Connect(t.Context(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(t.Context(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Expect 2 artifacts: pinned-archived (survives) + active (always included)
	// Plain-archived should be excluded
	if len(artifacts) != 2 {
		names := make([]string, len(artifacts))
		for i, a := range artifacts {
			names[i] = a.Title
		}
		t.Fatalf("REGRESSION: artifacts = %d (%v), want 2 (pinned-archived + active)", len(artifacts), names)
	}

	foundPinnedArchived := false
	for _, a := range artifacts {
		if a.Title == "Pinned Archived Idea" {
			foundPinnedArchived = true
		}
		if a.Title == "Old Note" {
			t.Error("REGRESSION: plain archived note should NOT appear in sync results")
		}
	}
	if !foundPinnedArchived {
		t.Error("REGRESSION: pinned+archived note missing from sync results — shouldSkip dropped it")
	}
}

// ===========================================================================
// REG-006: URL scheme sanitization — isSafeURL must block injection vectors.
//
// Annotations can contain arbitrary URLs from Takeout exports. The normalizer
// must strip unsafe schemes (javascript:, data:, vbscript:) to prevent XSS
// if content is rendered downstream (CWE-79). If isSafeURL is weakened or
// the allowlist is expanded, these tests catch the regression.
// ===========================================================================

func TestRegression_URLSanitizationBlocksJavascript(t *testing.T) {
	dangerous := []string{
		"javascript:alert('xss')",
		"javascript:void(0)",
		"data:text/html,<script>alert(1)</script>",
		"vbscript:msgbox",
		"JAVASCRIPT:alert('case-bypass')",
		"Data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==",
	}
	for _, u := range dangerous {
		if isSafeURL(u) {
			t.Errorf("REGRESSION: isSafeURL(%q) = true, must block unsafe scheme", u)
		}
	}
}

func TestRegression_URLSanitizationAllowsSafe(t *testing.T) {
	safe := []string{
		"https://example.com/article",
		"http://localhost:8080/test",
		"mailto:user@example.com",
	}
	for _, u := range safe {
		if !isSafeURL(u) {
			t.Errorf("REGRESSION: isSafeURL(%q) = false, safe schemes must be allowed", u)
		}
	}
}

func TestRegression_UnsafeURLStrippedFromContent(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:       "XSS Probe",
		TextContent: "Legit note content",
		Annotations: []TakeoutAnnotation{
			{URL: "javascript:alert(1)", Title: "Evil", Description: "XSS"},
			{URL: "https://safe.example.com", Title: "Good", Description: "Safe"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "xss-probe", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if strings.Contains(artifact.RawContent, "javascript:") {
		t.Error("REGRESSION: javascript: URL leaked into artifact content")
	}
	if !strings.Contains(artifact.RawContent, "https://safe.example.com") {
		t.Error("REGRESSION: safe URL was incorrectly stripped from content")
	}
}

// ===========================================================================
// REG-007: Attachment path traversal — sanitizeAttachmentPath must strip
// directory traversal sequences to prevent CWE-22.
// ===========================================================================

func TestRegression_AttachmentPathTraversal(t *testing.T) {
	cases := []struct {
		input string
		want  string // expected sanitized output (base name only)
	}{
		{"../../etc/passwd", "passwd"},
		{"../../../secret.txt", "secret.txt"},
		{"/absolute/path/photo.jpg", "photo.jpg"},
		{"safe-image.png", "safe-image.png"},
		{"", ""},
		{"subdir/nested/image.jpg", "image.jpg"},
	}

	for _, tt := range cases {
		got := sanitizeAttachmentPath(tt.input)
		if got != tt.want {
			t.Errorf("REGRESSION: sanitizeAttachmentPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRegression_TraversalPathStrippedFromMetadata(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title: "Path Traversal Probe",
		Attachments: []TakeoutAttachment{
			{FilePath: "../../../etc/passwd", MimeType: "image/jpeg"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "traversal-probe", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	attachments, ok := artifact.Metadata["attachments"].([]map[string]string)
	if !ok || len(attachments) == 0 {
		t.Fatal("expected attachments in metadata")
	}
	if strings.Contains(attachments[0]["file_path"], "..") {
		t.Error("REGRESSION: path traversal sequence leaked into attachment metadata")
	}
	if attachments[0]["file_path"] != "passwd" {
		t.Errorf("REGRESSION: attachment file_path = %q, want base name only", attachments[0]["file_path"])
	}
}

// ===========================================================================
// REG-008: Metadata array cap — maxMetadataArrayLen protects against resource
// exhaustion from adversarial Takeout exports with excessive labels/annotations
// (CWE-400). If the cap is removed or raised without bound, this test fails.
// ===========================================================================

func TestRegression_MetadataArrayCap(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	// Create a note with 1000 labels — well over the 500 cap
	note := &TakeoutNote{
		Title:                   "Label Bomb",
		TextContent:             "Note with excessive labels for resource exhaustion probe",
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}
	for i := 0; i < 1000; i++ {
		note.Labels = append(note.Labels, TakeoutLabel{Name: "label-" + strings.Repeat("x", 10)})
	}

	artifact, err := n.Normalize(note, "label-bomb", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	labels, ok := artifact.Metadata["labels"].([]string)
	if !ok {
		t.Fatal("REGRESSION: labels metadata missing or wrong type")
	}
	if len(labels) > maxMetadataArrayLen {
		t.Errorf("REGRESSION: labels count = %d, exceeds cap %d — resource exhaustion possible", len(labels), maxMetadataArrayLen)
	}
	if len(labels) != maxMetadataArrayLen {
		t.Errorf("REGRESSION: labels count = %d, want exactly %d (cap)", len(labels), maxMetadataArrayLen)
	}
}

// ===========================================================================
// REG-009: Config validation strictness — parseKeepConfig must reject invalid
// sync modes and enforce minimum poll interval. If validation is weakened,
// silent misconfiguration becomes possible.
// ===========================================================================

func TestRegression_ConfigRejectsInvalidSyncMode(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode": "invalid-mode",
		},
	}
	_, err := parseKeepConfig(cfg)
	if err == nil {
		t.Fatal("REGRESSION: parseKeepConfig accepted invalid sync_mode — must reject unknown modes")
	}
	if !strings.Contains(err.Error(), "invalid sync_mode") {
		t.Errorf("REGRESSION: error message should mention invalid sync_mode, got: %v", err)
	}
}

func TestRegression_ConfigEnforcesMinPollInterval(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":     "takeout",
			"import_dir":    "/tmp",
			"poll_interval": "5m",
		},
	}
	_, err := parseKeepConfig(cfg)
	if err == nil {
		t.Fatal("REGRESSION: parseKeepConfig accepted poll_interval < 15m — minimum must be enforced")
	}
	if !strings.Contains(err.Error(), "15m") {
		t.Errorf("REGRESSION: error should mention 15m minimum, got: %v", err)
	}
}

// ===========================================================================
// REG-010: Health state machine — consecutive sync errors must escalate
// through degraded → failing → error. If thresholds change, health
// reporting is misleading and upstream clients make wrong decisions.
// ===========================================================================

func TestRegression_HealthEscalationThresholds(t *testing.T) {
	dir := t.TempDir()
	// Write one corrupted file — every sync will produce errors with 0 artifacts
	writeTestJSON(t, dir, "corrupted.json", `{invalid json`)

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Run syncs and check health transitions
	// The parser logs parse errors. With one corrupt file and no valid files,
	// syncTakeout returns 0 artifacts but no fatal error – it counts parse errors.
	// After sync: lastSyncErrors > 0 AND lastSyncCount == 0 → consecutive error escalation.

	for i := 1; i <= 12; i++ {
		_, _, _ = c.Sync(context.Background(), "")

		h := c.Health(context.Background())
		switch {
		case i < 5:
			if h != connector.HealthDegraded {
				t.Errorf("REGRESSION: after %d consecutive-error syncs, health = %q, want degraded", i, h)
			}
		case i < 10:
			if h != connector.HealthFailing {
				t.Errorf("REGRESSION: after %d consecutive-error syncs, health = %q, want failing", i, h)
			}
		default:
			if h != connector.HealthError {
				t.Errorf("REGRESSION: after %d consecutive-error syncs, health = %q, want error", i, h)
			}
		}
	}
}

func TestRegression_HealthResetsOnSuccess(t *testing.T) {
	dir := t.TempDir()

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Empty dir → 0 artifacts, 0 errors → full success
	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if h := c.Health(context.Background()); h != connector.HealthHealthy {
		t.Errorf("REGRESSION: after clean sync, health = %q, want healthy", h)
	}
}

// ===========================================================================
// REG-011: Label cascade order — TopicMapper must resolve in strict order:
// exact → abbreviation → fuzzy → created. If the order changes, label
// matching silently degrades.
// ===========================================================================

func TestRegression_LabelCascadeExactBeforeAbbreviation(t *testing.T) {
	tm := NewTopicMapper()
	existingTopics := []string{"Machine Learning", "ML"}

	matches := tm.MapLabels([]string{"Machine Learning"}, existingTopics)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("REGRESSION: 'Machine Learning' resolved as %q, want exact", matches[0].MatchType)
	}
}

func TestRegression_LabelCascadeAbbreviationResolves(t *testing.T) {
	tm := NewTopicMapper()
	existingTopics := []string{"Machine Learning"}

	matches := tm.MapLabels([]string{"ml"}, existingTopics)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].MatchType != "abbreviation" {
		t.Errorf("REGRESSION: 'ml' resolved as %q, want abbreviation", matches[0].MatchType)
	}
	if matches[0].TopicName != "Machine Learning" {
		t.Errorf("REGRESSION: 'ml' mapped to %q, want Machine Learning", matches[0].TopicName)
	}
}

func TestRegression_LabelCascadeFuzzyFallback(t *testing.T) {
	tm := NewTopicMapper()
	existingTopics := []string{"Machine Learning", "Artificial Intelligence"}

	matches := tm.MapLabels([]string{"Machne Lerning"}, existingTopics) // typo
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].MatchType != "fuzzy" {
		t.Errorf("REGRESSION: typo 'Machne Lerning' resolved as %q, want fuzzy", matches[0].MatchType)
	}
}

func TestRegression_LabelCascadeCreateNew(t *testing.T) {
	tm := NewTopicMapper()
	existingTopics := []string{"Cooking", "Gardening"}

	matches := tm.MapLabels([]string{"Quantum Physics"}, existingTopics)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].MatchType != "created" {
		t.Errorf("REGRESSION: novel label resolved as %q, want created", matches[0].MatchType)
	}
}

// ===========================================================================
// REG-012: NoteID stability — NoteID must produce deterministic IDs from
// filenames so that the same export re-synced doesn't create duplicate
// artifacts. If ID derivation changes, dedup breaks silently.
// ===========================================================================

func TestRegression_NoteIDDeterministic(t *testing.T) {
	parser := NewTakeoutParser()
	note := &TakeoutNote{Title: "test"}

	id1 := parser.NoteID(note, "my-important-note.json")
	id2 := parser.NoteID(note, "my-important-note.json")

	if id1 != id2 {
		t.Errorf("REGRESSION: NoteID not deterministic: %q != %q", id1, id2)
	}
	if id1 != "my-important-note" {
		t.Errorf("REGRESSION: NoteID = %q, want my-important-note (strip .json extension)", id1)
	}
}

func TestRegression_NoteIDDistinctForDifferentFiles(t *testing.T) {
	parser := NewTakeoutParser()
	note := &TakeoutNote{Title: "Same Title"}

	id1 := parser.NoteID(note, "note-001.json")
	id2 := parser.NoteID(note, "note-002.json")

	if id1 == id2 {
		t.Errorf("REGRESSION: NoteID collision for different filenames: %q == %q", id1, id2)
	}
}

// ===========================================================================
// REG-013: Gkeepapi warning gate — Connect must reject gkeepapi or hybrid
// mode when gkeep_enabled=true but warning_acknowledged=false. If this gate
// is removed, users unknowingly use an unofficial, breakable API.
// ===========================================================================

func TestRegression_GkeepWarningGateEnforced(t *testing.T) {
	dir := t.TempDir()

	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig(dir, "gkeepapi", true, false))
	if err == nil {
		t.Fatal("REGRESSION: Connect accepted gkeepapi mode without warning_acknowledged — gate bypassed")
	}
	if !strings.Contains(err.Error(), "warning_acknowledged") {
		t.Errorf("REGRESSION: error should mention warning_acknowledged, got: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Error("REGRESSION: health should be error after gkeep gate rejection")
	}
}

func TestRegression_GkeepWarningGatePassesWithAck(t *testing.T) {
	// With ack=true, gkeepapi mode Connect should not fail on the gate
	// (it may fail later on missing bridge, but the gate itself passes)
	dir := t.TempDir()
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig(dir, "gkeepapi", true, true))
	// gkeepapi mode doesn't require import_dir, so no directory validation failure
	if err != nil && strings.Contains(err.Error(), "warning_acknowledged") {
		t.Errorf("REGRESSION: Connect rejected gkeepapi with warning_acknowledged=true: %v", err)
	}
}

// ===========================================================================
// REG-014: Email sanitization — isBasicEmail must reject injection-susceptible
// formats in collaborator emails. If weakened, CWE-79 through metadata.
// ===========================================================================

func TestRegression_EmailSanitizationBlocksInjection(t *testing.T) {
	dangerous := []string{
		"<script>@evil.com",
		"user @example.com",
		"user\t@example.com",
		"user\n@example.com",
		"user@@double.com",
		"@bare-domain.com",
		"local-only@",
		"",
	}
	for _, e := range dangerous {
		if isBasicEmail(e) {
			t.Errorf("REGRESSION: isBasicEmail(%q) = true, must reject injection-susceptible format", e)
		}
	}
}

func TestRegression_EmailSanitizationAllowsValid(t *testing.T) {
	valid := []string{
		"alice@example.com",
		"bob.smith@company.org",
		"user+tag@gmail.com",
	}
	for _, e := range valid {
		if !isBasicEmail(e) {
			t.Errorf("REGRESSION: isBasicEmail(%q) = false, valid emails must be accepted", e)
		}
	}
}

// ===========================================================================
// REG-015: Timestamp zero guards — buildMetadata must replace zero/epoch
// timestamps with current time so downstream consumers don't see 1970-01-01.
// If the guards are removed, metadata contains misleading timestamps.
// ===========================================================================

func TestRegression_ZeroTimestampGuards(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:                   "Zero Timestamp Note",
		TextContent:             "Note with zero timestamps",
		UserEditedTimestampUsec: 0,
		CreatedTimestampUsec:    0,
	}

	artifact, err := n.Normalize(note, "zero-ts", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	modifiedAt, ok := artifact.Metadata["modified_at"].(string)
	if !ok {
		t.Fatal("REGRESSION: modified_at missing from metadata")
	}
	if strings.HasPrefix(modifiedAt, "1970-01-01") {
		t.Error("REGRESSION: zero timestamp leaked as 1970-01-01 into modified_at metadata")
	}

	createdAt, ok := artifact.Metadata["created_at"].(string)
	if !ok {
		t.Fatal("REGRESSION: created_at missing from metadata")
	}
	if strings.HasPrefix(createdAt, "1970-01-01") {
		t.Error("REGRESSION: zero timestamp leaked as 1970-01-01 into created_at metadata")
	}

	// The artifact CapturedAt should also not be epoch
	if artifact.CapturedAt.Year() < 2020 {
		t.Errorf("REGRESSION: artifact CapturedAt = %v, should be recent (zero guard)", artifact.CapturedAt)
	}
}

// ===========================================================================
// REG-016: DiffLabels correctness — must correctly detect added and removed
// labels across sync cycles. If broken, topic graph edge updates silently
// skip additions or deletions.
// ===========================================================================

func TestRegression_DiffLabelsDetectsAddedAndRemoved(t *testing.T) {
	previous := []string{"Work", "Ideas"}
	current := []string{"Work", "ML", "Research"}

	added, removed := DiffLabels(current, previous)

	wantAdded := map[string]bool{"ML": true, "Research": true}
	wantRemoved := map[string]bool{"Ideas": true}

	for _, a := range added {
		if !wantAdded[a] {
			t.Errorf("REGRESSION: unexpected label in added: %q", a)
		}
		delete(wantAdded, a)
	}
	if len(wantAdded) > 0 {
		t.Errorf("REGRESSION: missing added labels: %v", wantAdded)
	}

	for _, r := range removed {
		if !wantRemoved[r] {
			t.Errorf("REGRESSION: unexpected label in removed: %q", r)
		}
		delete(wantRemoved, r)
	}
	if len(wantRemoved) > 0 {
		t.Errorf("REGRESSION: missing removed labels: %v", wantRemoved)
	}
}

func TestRegression_DiffLabelsEmptyTransitions(t *testing.T) {
	// From no labels to some labels (first sync)
	added, removed := DiffLabels([]string{"New"}, nil)
	if len(added) != 1 || added[0] != "New" {
		t.Errorf("REGRESSION: first-sync added = %v, want [New]", added)
	}
	if len(removed) != 0 {
		t.Errorf("REGRESSION: first-sync removed = %v, want empty", removed)
	}

	// From some labels to no labels (all deleted)
	added2, removed2 := DiffLabels(nil, []string{"Old"})
	if len(added2) != 0 {
		t.Errorf("REGRESSION: clear-all added = %v, want empty", added2)
	}
	if len(removed2) != 1 || removed2[0] != "Old" {
		t.Errorf("REGRESSION: clear-all removed = %v, want [Old]", removed2)
	}
}
