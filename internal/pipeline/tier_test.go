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

func TestAssignTier_ExactBoundary200(t *testing.T) {
	// Content of exactly 200 characters should get standard tier (>=200 threshold)
	tier := AssignTier(TierSignals{ContentLen: 200, SourceID: "gmail"})
	if tier != TierStandard {
		t.Errorf("exactly 200-char content should get standard tier, got %q", tier)
	}
}

func TestAssignTier_Boundary199(t *testing.T) {
	// Content of 199 characters should get light tier (<200 threshold)
	tier := AssignTier(TierSignals{ContentLen: 199, SourceID: "gmail"})
	if tier != TierLight {
		t.Errorf("199-char content should get light tier, got %q", tier)
	}
}

func TestAssignTier_ZeroContentLength(t *testing.T) {
	tier := AssignTier(TierSignals{ContentLen: 0, SourceID: "gmail"})
	if tier != TierLight {
		t.Errorf("zero-length content should get light tier, got %q", tier)
	}
}

func TestAssignTier_UnknownSourceID(t *testing.T) {
	// Unknown source IDs should not get full tier privilege
	tier := AssignTier(TierSignals{ContentLen: 500, SourceID: "unknown-source"})
	if tier != TierStandard {
		t.Errorf("unknown source ID should get standard tier, got %q", tier)
	}
}

func TestAssignTier_EmptySourceID(t *testing.T) {
	tier := AssignTier(TierSignals{ContentLen: 500, SourceID: ""})
	if tier != TierStandard {
		t.Errorf("empty source ID should get standard tier, got %q", tier)
	}
}

func TestAssignTier_PriorityOrder_StarredOverridesShortContent(t *testing.T) {
	// Starred flag should override content length, producing full tier
	tier := AssignTier(TierSignals{UserStarred: true, ContentLen: 10, SourceID: "gmail"})
	if tier != TierFull {
		t.Errorf("starred short content should get full tier, got %q", tier)
	}
}

func TestAssignTier_LargeContent(t *testing.T) {
	tier := AssignTier(TierSignals{ContentLen: 100000, SourceID: "gmail"})
	if tier != TierStandard {
		t.Errorf("large content from passive source should get standard tier, got %q", tier)
	}
}
