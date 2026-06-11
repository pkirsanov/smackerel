// Package cardrewards implements the Card Rewards Companion domain (spec 083):
// the PostgreSQL-backed wallet/catalog/offers/selections/bonuses/category-alias
// store, card-name resolution (replacing CCManager's card_resolver.py), and the
// service wiring consumed by the REST API, scheduler, and web layers.
//
// Storage is PostgreSQL-only via jackc/pgx (Principle 5 / NFR-CR-002); there is
// no ORM and no second store. JSON field names are snake_case to match every
// other smackerel API surface (connector.RawArtifact, mealplan, recommendations
// …) — the codebase-wide convention. Monetary amounts are integer cents.
package cardrewards

import (
	"encoding/json"
	"time"
)

// Card type and source enums mirror the migration 057 CHECK constraints
// (design §2.1). Keeping them as typed string constants avoids magic strings
// in the service and handler layers.
const (
	CardTypeRotating     = "rotating"
	CardTypeFixed        = "fixed"
	CardTypeUserSelected = "user-selected"

	SourceSeed      = "seed"
	SourceDiscovery = "discovery"
	SourceManual    = "manual"

	RateTypePercent    = "percent"
	RateTypePoints     = "points"
	RateTypeMultiplier = "multiplier"

	BonusTypeSpend         = "spend"
	BonusTypeFirstYearRate = "first_year_rate"
)

// CatalogCard is a master-database card (table card_catalog). The id is a
// stable text slug (e.g. "citi-custom-cash") so the one-time CCManager JSON
// import (Scope 03) can reseed idempotently.
type CatalogCard struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Issuer             string          `json:"issuer"`
	CardType           string          `json:"card_type"`
	AnnualFeeCents     int             `json:"annual_fee_cents"`
	Requires           *string         `json:"requires,omitempty"`
	BaseBenefits       json.RawMessage `json:"base_benefits"`
	RotatingBenefits   json.RawMessage `json:"rotating_benefits,omitempty"`
	SelectableBenefits json.RawMessage `json:"selectable_benefits,omitempty"`
	Perks              json.RawMessage `json:"perks"`
	Aliases            []string        `json:"aliases"`
	Source             string          `json:"source"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// UserCard is a wallet entry (table user_cards) referencing a CatalogCard.
type UserCard struct {
	ID            string    `json:"id"`
	CardCatalogID string    `json:"card_catalog_id"`
	Nickname      *string   `json:"nickname,omitempty"`
	Note          *string   `json:"note,omitempty"`
	Active        bool      `json:"active"`
	AddedAt       time.Time `json:"added_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// CatalogName is resolved at query time from card_catalog; not stored on
	// user_cards. Mirrors mealplan.Slot.RecipeTitle.
	CatalogName string `json:"catalog_name,omitempty"`
}

// Offer is a promo/elevated-rate offer (table card_offers). UserCardID is nil
// for a general (non-card-specific) offer.
type Offer struct {
	ID                 string     `json:"id"`
	UserCardID         *string    `json:"user_card_id,omitempty"`
	Title              string     `json:"title"`
	Category           string     `json:"category"`
	Rate               float64    `json:"rate"`
	RateType           string     `json:"rate_type"`
	LimitCents         *int       `json:"limit_cents,omitempty"`
	LimitPeriod        *string    `json:"limit_period,omitempty"`
	SharedLimitGroup   *string    `json:"shared_limit_group,omitempty"`
	StartsOn           *time.Time `json:"starts_on,omitempty"`
	EndsOn             *time.Time `json:"ends_on,omitempty"`
	ActivationRequired bool       `json:"activation_required"`
	Activated          bool       `json:"activated"`
	Notes              *string    `json:"notes,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Selection is a selectable-category choice (table card_selections). Tier is
// nil for non-tiered cards and set (e.g. 1) for tiered cards like US Bank Cash+.
type Selection struct {
	ID             string     `json:"id"`
	UserCardID     string     `json:"user_card_id"`
	Category       string     `json:"category"`
	Tier           *int       `json:"tier,omitempty"`
	PeriodLabel    string     `json:"period_label"`
	Enrolled       bool       `json:"enrolled"`
	EnrolledAt     *time.Time `json:"enrolled_at,omitempty"`
	EffectiveStart *time.Time `json:"effective_start,omitempty"`
	EffectiveEnd   *time.Time `json:"effective_end,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// SignupBonus is a spend/first-year bonus tracker (table signup_bonuses).
// Spend progress is manually entered (there is no card feed).
type SignupBonus struct {
	ID                 string     `json:"id"`
	UserCardID         string     `json:"user_card_id"`
	BonusType          string     `json:"bonus_type"`
	Description        string     `json:"description"`
	SpendRequiredCents *int       `json:"spend_required_cents,omitempty"`
	SpendProgressCents int        `json:"spend_progress_cents"`
	RewardDescription  *string    `json:"reward_description,omitempty"`
	Deadline           *time.Time `json:"deadline,omitempty"`
	Met                bool       `json:"met"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// CategoryAlias is a canonical spend-category name with equivalents (table
// category_aliases), imported from CCManager config.json (Scope 03) and used
// by the optimizer + web category management.
type CategoryAlias struct {
	ID                string    `json:"id"`
	CanonicalCategory string    `json:"canonical_category"`
	Equivalents       []string  `json:"equivalents"`
	Starred           bool      `json:"starred"`
	Priority          *int      `json:"priority,omitempty"`
	BuiltIn           bool      `json:"built_in"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ValidCardType reports whether t is an allowed card_catalog.card_type value
// (matches the migration CHECK constraint).
func ValidCardType(t string) bool {
	return t == CardTypeRotating || t == CardTypeFixed || t == CardTypeUserSelected
}

// ValidRateType reports whether t is an allowed card_offers.rate_type value.
func ValidRateType(t string) bool {
	return t == RateTypePercent || t == RateTypePoints || t == RateTypeMultiplier
}

// ValidBonusType reports whether t is an allowed signup_bonuses.bonus_type value.
func ValidBonusType(t string) bool {
	return t == BonusTypeSpend || t == BonusTypeFirstYearRate
}

// Lifecycle states for a reconciled rotating-category record (table
// rotating_categories, design §2.7) and run types/triggers/statuses for the
// audit table (table card_runs, design §2.10). They mirror the migration 057
// CHECK constraints so the importer (Scope 03) and later scopes avoid magic
// strings.
const (
	LifecycleUpcoming = "upcoming"
	LifecycleActive   = "active"
	LifecycleExpired  = "expired"

	RunTypeScrape       = "scrape"
	RunTypeExtract      = "extract"
	RunTypeReconcile    = "reconcile"
	RunTypeOptimize     = "optimize"
	RunTypeCalendarSync = "calendar_sync"
	RunTypeMigration    = "migration"
	RunTypeDiscovery    = "discovery"

	RunTriggerScheduled = "scheduled"
	RunTriggerManual    = "manual"

	RunStatusSuccess = "success"
	RunStatusPartial = "partial"
	RunStatusFailed  = "failed"
)

// RotatingCategory is a reconciled, lifecycle-aware rotating-category record
// (table rotating_categories). The CCManager JSON import (Scope 03) seeds these
// with ManualOverride=true and Confidence=1 so the first live LLM extraction
// augments rather than overwrites the imported historical truth (SCN-083-C03).
type RotatingCategory struct {
	ID                 string     `json:"id"`
	CardCatalogID      string     `json:"card_catalog_id"`
	PeriodLabel        string     `json:"period_label"`
	PeriodStart        *time.Time `json:"period_start,omitempty"`
	PeriodEnd          *time.Time `json:"period_end,omitempty"`
	Categories         []string   `json:"categories"`
	LimitCents         *int       `json:"limit_cents,omitempty"`
	ActivationRequired bool       `json:"activation_required"`
	LifecycleState     string     `json:"lifecycle_state"`
	Confidence         float64    `json:"confidence"`
	NeedsVerification  bool       `json:"needs_verification"`
	ManualOverride     bool       `json:"manual_override"`
	SourceCount        int        `json:"source_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// CardRecommendation is a per-period, per-category card recommendation (table
// card_recommendations). RecommendedUserCardID is nil when the recommended
// card is not (or no longer) in the wallet.
type CardRecommendation struct {
	ID                    string    `json:"id"`
	PeriodLabel           string    `json:"period_label"`
	Category              string    `json:"category"`
	RecommendedUserCardID *string   `json:"recommended_user_card_id,omitempty"`
	Rate                  float64   `json:"rate"`
	Reason                string    `json:"reason"`
	Starred               bool      `json:"starred"`
	StarredOverride       bool      `json:"starred_override"`
	CalendarEventUID      *string   `json:"calendar_event_uid,omitempty"`
	GeneratedAt           time.Time `json:"generated_at"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// CardRun is a single audit record for a refresh/extract/reconcile/optimize/
// calendar-sync/migration run (table card_runs, Principle 8). The importer
// (Scope 03) writes one row with RunType="migration" and imports the mappable
// subset of CCManager run-history.json (SCN-083-C06).
type CardRun struct {
	ID                  string     `json:"id"`
	RunType             string     `json:"run_type"`
	Trigger             string     `json:"trigger"`
	Status              string     `json:"status"`
	SourcesAttempted    int        `json:"sources_attempted"`
	SourcesSucceeded    int        `json:"sources_succeeded"`
	CategoriesExtracted int        `json:"categories_extracted"`
	EventsWritten       int        `json:"events_written"`
	ErrorDetail         *string    `json:"error_detail,omitempty"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// ValidLifecycleState reports whether s is an allowed rotating_categories
// lifecycle_state value (matches the migration CHECK constraint).
func ValidLifecycleState(s string) bool {
	return s == LifecycleUpcoming || s == LifecycleActive || s == LifecycleExpired
}

// ValidRunType reports whether t is an allowed card_runs.run_type value.
func ValidRunType(t string) bool {
	switch t {
	case RunTypeScrape, RunTypeExtract, RunTypeReconcile, RunTypeOptimize,
		RunTypeCalendarSync, RunTypeMigration, RunTypeDiscovery:
		return true
	default:
		return false
	}
}

// RotatingCategoryObservation is a single per-source, strict-schema LLM
// extraction (table rotating_category_observations, design §2.6 / §4). Each row
// is one source's validated claim about a card's rotating categories for a
// period, retaining full provenance (SourceName/SourceURL/SourceEvidence —
// Principle 4) and the audit run that produced it (ExtractionRunID). The
// reconciler (Scope 06) later merges observations into the authoritative
// rotating_categories record; an extraction MUST never overwrite that record
// directly (design §4 — the CCManager silent-fallback failure mode this scope
// replaces). LimitCents is integer cents (the sidecar reports a whole-dollar
// spend cap which the orchestrator converts ×100).
type RotatingCategoryObservation struct {
	ID                 string     `json:"id"`
	CardCatalogID      string     `json:"card_catalog_id"`
	PeriodLabel        string     `json:"period_label"`
	PeriodStart        *time.Time `json:"period_start,omitempty"`
	PeriodEnd          *time.Time `json:"period_end,omitempty"`
	Categories         []string   `json:"categories"`
	LimitCents         *int       `json:"limit_cents,omitempty"`
	ActivationRequired *bool      `json:"activation_required,omitempty"`
	Confidence         float64    `json:"confidence"`
	SourceName         string     `json:"source_name"`
	SourceURL          string     `json:"source_url"`
	SourceEvidence     *string    `json:"source_evidence,omitempty"`
	ExtractionRunID    string     `json:"extraction_run_id"`
	ObservedAt         time.Time  `json:"observed_at"`
}

// CardPeriodRef identifies one reconciled rotating-category record by its
// natural key (card_catalog_id, period_label). The extractor uses it to flag an
// existing record needs_verification when an extraction fails to validate —
// without touching its categories/confidence (design §4 step 2).
type CardPeriodRef struct {
	CardCatalogID string
	PeriodLabel   string
}
