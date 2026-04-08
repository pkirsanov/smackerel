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
