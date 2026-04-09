package pipeline

import (
	"testing"
)

func TestAssignTier_UserStarred(t *testing.T) {
	tier := AssignTier(TierSignals{UserStarred: true})
	if tier != TierFull {
		t.Errorf("starred content should get full tier, got %q", tier)
	}
}

func TestAssignTier_ActiveCapture(t *testing.T) {
	for _, source := range []string{"capture", "telegram", "browser"} {
		tier := AssignTier(TierSignals{SourceID: source})
		if tier != TierFull {
			t.Errorf("source %q should get full tier, got %q", source, tier)
		}
	}
}

func TestAssignTier_WithContext(t *testing.T) {
	tier := AssignTier(TierSignals{HasContext: true, SourceID: "gmail"})
	if tier != TierFull {
		t.Errorf("content with context should get full tier, got %q", tier)
	}
}

func TestAssignTier_ShortContent(t *testing.T) {
	tier := AssignTier(TierSignals{ContentLen: 50, SourceID: "gmail"})
	if tier != TierLight {
		t.Errorf("short content should get light tier, got %q", tier)
	}
}

func TestAssignTier_Default(t *testing.T) {
	tier := AssignTier(TierSignals{ContentLen: 500, SourceID: "gmail"})
	if tier != TierStandard {
		t.Errorf("default should be standard tier, got %q", tier)
	}
}

func TestAssignTier_BrowserHistorySourceID(t *testing.T) {
	// R001 regression: "browser-history" SourceID must get full-tier processing
	// just like "browser", "capture", and "telegram".
	tier := AssignTier(TierSignals{SourceID: "browser-history"})
	if tier != TierFull {
		t.Errorf("browser-history source should get full tier, got %q", tier)
	}
}
