package pipeline

// Tier represents the processing tier for an artifact.
type Tier string

const (
	TierFull     Tier = "full"     // summary + entities + action items + connections + embedding
	TierStandard Tier = "standard" // summary + entities + embedding
	TierLight    Tier = "light"    // summary + embedding only
	TierMetadata Tier = "metadata" // title + source only (fallback)
)

// TierSignals contains the signals used to determine processing tier.
type TierSignals struct {
	UserStarred bool   // user explicitly starred this item
	SourceID    string // source that sent this (gmail, telegram, etc.)
	HasContext  bool   // user provided additional context
	ContentLen  int    // length of extracted content
}

// AssignTier determines the processing tier based on input signals.
func AssignTier(s TierSignals) Tier {
	// User-starred content always gets full processing
	if s.UserStarred {
		return TierFull
	}

	// Active captures (user-initiated) get full processing
	if s.SourceID == "capture" || s.SourceID == "telegram" || s.SourceID == "browser" || s.SourceID == "browser-history" {
		return TierFull
	}

	// Content with user context gets full processing
	if s.HasContext {
		return TierFull
	}

	// Short content gets light processing
	if s.ContentLen < 200 {
		return TierLight
	}

	// Default: standard
	return TierStandard
}
