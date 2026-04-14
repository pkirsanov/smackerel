package keep

import (
	"testing"
	"time"
)

func TestQualifierEvaluationOrder(t *testing.T) {
	q := NewQualifier()
	// Pinned AND archived — pinned should win
	note := &TakeoutNote{
		IsPinned:                true,
		IsArchived:              true,
		Labels:                  []TakeoutLabel{{Name: "Work"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierFull {
		t.Errorf("tier = %q, want full (pinned evaluated first)", result.Tier)
	}
	if result.Reason != "pinned" {
		t.Errorf("reason = %q, want pinned", result.Reason)
	}
}

func TestQualifierPinnedOverridesAll(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		IsPinned:                true,
		IsArchived:              true,
		UserEditedTimestampUsec: time.Now().Add(-90 * 24 * time.Hour).UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierFull {
		t.Errorf("tier = %q, want full", result.Tier)
	}
}

func TestQualifierLabeledGetsFull(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		Labels:                  []TakeoutLabel{{Name: "Work"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierFull {
		t.Errorf("tier = %q, want full", result.Tier)
	}
}

func TestQualifierImageGetsFull(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		Attachments:             []TakeoutAttachment{{MimeType: "image/jpeg"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierFull {
		t.Errorf("tier = %q, want full", result.Tier)
	}
}

func TestQualifierRecentGetsStandard(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		TextContent:             "Recent note",
		UserEditedTimestampUsec: time.Now().Add(-10 * 24 * time.Hour).UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierStandard {
		t.Errorf("tier = %q, want standard", result.Tier)
	}
}

func TestQualifierOldGetsLight(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		TextContent:             "Old note",
		UserEditedTimestampUsec: time.Now().Add(-60 * 24 * time.Hour).UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierLight {
		t.Errorf("tier = %q, want light", result.Tier)
	}
}

func TestQualifierArchivedGetsLight(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		IsArchived:              true,
		UserEditedTimestampUsec: time.Now().Add(-60 * 24 * time.Hour).UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierLight {
		t.Errorf("tier = %q, want light", result.Tier)
	}
}

func TestQualifierTrashedGetsSkip(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{IsTrashed: true}
	result := q.Evaluate(note)
	if result.Tier != TierSkip {
		t.Errorf("tier = %q, want skip", result.Tier)
	}
}

// Regression: archived notes must get light tier even when recently modified.
// R-008 says "Archived note → light" without any recency qualification.
// Archiving is an intentional user deprioritization signal that overrides recency.
func TestQualifierRecentArchivedGetsLight(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		IsArchived:              true,
		TextContent:             "Recently archived note",
		UserEditedTimestampUsec: time.Now().Add(-5 * 24 * time.Hour).UnixMicro(), // 5 days ago — recent
	}
	result := q.Evaluate(note)
	if result.Tier != TierLight {
		t.Errorf("tier = %q, want light (archived overrides recent per R-008)", result.Tier)
	}
	if result.Reason != "archived" {
		t.Errorf("reason = %q, want archived", result.Reason)
	}
}

func TestEvaluateBatch(t *testing.T) {
	q := NewQualifier()
	notes := []TakeoutNote{
		{IsPinned: true, UserEditedTimestampUsec: time.Now().UnixMicro()},
		{IsTrashed: true},
		{UserEditedTimestampUsec: time.Now().Add(-10 * 24 * time.Hour).UnixMicro()},
		{UserEditedTimestampUsec: time.Now().Add(-60 * 24 * time.Hour).UnixMicro()},
	}

	counts := q.EvaluateBatch(notes)
	if counts[TierFull] != 1 {
		t.Errorf("full = %d, want 1", counts[TierFull])
	}
	if counts[TierSkip] != 1 {
		t.Errorf("skip = %d, want 1", counts[TierSkip])
	}
	if counts[TierStandard] != 1 {
		t.Errorf("standard = %d, want 1", counts[TierStandard])
	}
	if counts[TierLight] != 1 {
		t.Errorf("light = %d, want 1", counts[TierLight])
	}
}

// --- Trashed overrides all other properties ---

func TestQualifierTrashedOverridesPinned(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		IsTrashed:               true,
		IsPinned:                true,
		Labels:                  []TakeoutLabel{{Name: "Important"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierSkip {
		t.Errorf("tier = %q, want skip (trashed overrides pinned)", result.Tier)
	}
	if result.Reason != "trashed" {
		t.Errorf("reason = %q, want trashed", result.Reason)
	}
}

func TestQualifierTrashedOverridesLabeled(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		IsTrashed:               true,
		Labels:                  []TakeoutLabel{{Name: "Work"}, {Name: "Urgent"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierSkip {
		t.Errorf("tier = %q, want skip (trashed overrides labels)", result.Tier)
	}
}

// --- Audio-only attachment does NOT trigger full tier ---

func TestQualifierAudioOnlyGetsDefaultNotFull(t *testing.T) {
	q := NewQualifier()
	note := &TakeoutNote{
		Attachments:             []TakeoutAttachment{{MimeType: "audio/3gpp"}},
		UserEditedTimestampUsec: time.Now().Add(-10 * 24 * time.Hour).UnixMicro(),
	}
	result := q.Evaluate(note)
	// Audio attachments do NOT trigger full — only image/ prefix does
	if result.Tier == TierFull {
		t.Errorf("tier = %q — audio-only should not get full tier", result.Tier)
	}
}

// --- EvaluateBatch edge cases ---

func TestEvaluateBatchEmpty(t *testing.T) {
	q := NewQualifier()
	counts := q.EvaluateBatch([]TakeoutNote{})
	total := counts[TierFull] + counts[TierStandard] + counts[TierLight] + counts[TierSkip]
	if total != 0 {
		t.Errorf("empty batch total = %d, want 0", total)
	}
}

// --- Boundary: note modified exactly 30 days ago ---

func TestQualifierJustUnder30DaysOld(t *testing.T) {
	q := NewQualifier()
	// Use 29 days + 23 hours to be safely within the 30-day threshold
	note := &TakeoutNote{
		TextContent:             "Note from just under 30 days ago",
		UserEditedTimestampUsec: time.Now().Add(-29*24*time.Hour - 23*time.Hour).UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierStandard {
		t.Errorf("tier = %q, want standard (within 30d)", result.Tier)
	}
}

func TestQualifierJustOver30DaysOld(t *testing.T) {
	q := NewQualifier()
	// Use 31 days to be clearly past the 30-day threshold
	note := &TakeoutNote{
		TextContent:             "Note from just over 30 days ago",
		UserEditedTimestampUsec: time.Now().Add(-31 * 24 * time.Hour).UnixMicro(),
	}
	result := q.Evaluate(note)
	if result.Tier != TierLight {
		t.Errorf("tier = %q, want light (>30d)", result.Tier)
	}
}
