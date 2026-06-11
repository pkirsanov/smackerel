package cardrewards

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Sentinel errors let the REST handler map service failures to HTTP status
// codes without inspecting strings.
var (
	// ErrValidation indicates caller-supplied input failed validation (400).
	ErrValidation = errors.New("cardrewards: validation failed")
	// ErrCatalogNotFound indicates a referenced catalog card does not exist (404/422).
	ErrCatalogNotFound = errors.New("cardrewards: catalog card not found")
	// ErrUserCardNotFound indicates a referenced wallet entry does not exist (404).
	ErrUserCardNotFound = errors.New("cardrewards: user card not found")
	// ErrOfferNotFound indicates a referenced offer does not exist (404).
	ErrOfferNotFound = errors.New("cardrewards: offer not found")
	// ErrSelectionNotFound indicates a referenced selection does not exist (404).
	ErrSelectionNotFound = errors.New("cardrewards: selection not found")
	// ErrBonusNotFound indicates a referenced signup bonus does not exist (404).
	ErrBonusNotFound = errors.New("cardrewards: signup bonus not found")
	// ErrRotatingNotFound indicates a referenced rotating-category record does not exist (404).
	ErrRotatingNotFound = errors.New("cardrewards: rotating category not found")
	// ErrRecommendationNotFound indicates a referenced recommendation does not exist (404).
	ErrRecommendationNotFound = errors.New("cardrewards: recommendation not found")
)

// Service implements card-rewards business logic over a Store. It owns
// validation and ID generation; the Store owns persistence only.
type Service struct {
	store       *Store
	recommender *Recommender
}

// NewService creates a card-rewards service.
func NewService(store *Store) *Service {
	return &Service{store: store, recommender: NewRecommender(store)}
}

// validationErr wraps ErrValidation with a specific message.
func validationErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// CustomCardInput describes a manual (non-catalog) card to create.
type CustomCardInput struct {
	Name           string
	Issuer         string
	CardType       string
	AnnualFeeCents int
	Nickname       *string
	Note           *string
}

// CreateUserCard adds an existing catalog card to the wallet. It fails with
// ErrCatalogNotFound when the catalog id is unknown.
func (s *Service) CreateUserCard(ctx context.Context, catalogID string, nickname, note *string) (*UserCard, error) {
	catalogID = strings.TrimSpace(catalogID)
	if catalogID == "" {
		return nil, validationErr("catalog_id is required")
	}
	cat, err := s.store.GetCatalogCard(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	if cat == nil {
		return nil, fmt.Errorf("%w: %s", ErrCatalogNotFound, catalogID)
	}
	uc := &UserCard{
		ID:            uuid.NewString(),
		CardCatalogID: catalogID,
		Nickname:      trimPtr(nickname),
		Note:          trimPtr(note),
		Active:        true,
	}
	if err := s.store.CreateUserCard(ctx, uc); err != nil {
		return nil, err
	}
	return s.store.GetUserCard(ctx, uc.ID)
}

// CreateCustomCard creates a manual catalog entry (source="manual") plus its
// wallet entry, atomically (B04).
func (s *Service) CreateCustomCard(ctx context.Context, in CustomCardInput) (*UserCard, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Issuer = strings.TrimSpace(in.Issuer)
	in.CardType = strings.TrimSpace(in.CardType)
	if in.Name == "" {
		return nil, validationErr("custom.name is required")
	}
	if in.Issuer == "" {
		return nil, validationErr("custom.issuer is required")
	}
	if !ValidCardType(in.CardType) {
		return nil, validationErr("custom.card_type must be one of rotating|fixed|user-selected, got %q", in.CardType)
	}
	if in.AnnualFeeCents < 0 {
		return nil, validationErr("custom.annual_fee_cents must be >= 0")
	}

	cat := &CatalogCard{
		ID:             "manual-" + uuid.NewString(),
		Name:           in.Name,
		Issuer:         in.Issuer,
		CardType:       in.CardType,
		AnnualFeeCents: in.AnnualFeeCents,
		Source:         SourceManual,
	}
	uc := &UserCard{
		ID:            uuid.NewString(),
		CardCatalogID: cat.ID,
		Nickname:      trimPtr(in.Nickname),
		Note:          trimPtr(in.Note),
		Active:        true,
	}
	if err := s.store.CreateCustomCard(ctx, cat, uc); err != nil {
		return nil, err
	}
	return s.store.GetUserCard(ctx, uc.ID)
}

// GetUserCard returns a wallet entry or ErrUserCardNotFound.
func (s *Service) GetUserCard(ctx context.Context, id string) (*UserCard, error) {
	uc, err := s.store.GetUserCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if uc == nil {
		return nil, fmt.Errorf("%w: %s", ErrUserCardNotFound, id)
	}
	return uc, nil
}

// ListUserCards returns wallet entries (optionally active-only).
func (s *Service) ListUserCards(ctx context.Context, activeOnly bool) ([]UserCard, error) {
	return s.store.ListUserCards(ctx, activeOnly)
}

// UpdateUserCard updates the mutable wallet fields. Only non-nil arguments are
// applied; the others retain their current values. Returns ErrUserCardNotFound
// if the card does not exist.
func (s *Service) UpdateUserCard(ctx context.Context, id string, nickname, note *string, active *bool) (*UserCard, error) {
	current, err := s.store.GetUserCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("%w: %s", ErrUserCardNotFound, id)
	}
	if nickname != nil {
		current.Nickname = trimPtr(nickname)
	}
	if note != nil {
		current.Note = trimPtr(note)
	}
	if active != nil {
		current.Active = *active
	}
	ok, err := s.store.UpdateUserCard(ctx, current)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUserCardNotFound, id)
	}
	return s.store.GetUserCard(ctx, id)
}

// DeleteUserCard removes a wallet entry (cascading to its offers, selections,
// and bonuses). Returns ErrUserCardNotFound if no row matched.
func (s *Service) DeleteUserCard(ctx context.Context, id string) error {
	ok, err := s.store.DeleteUserCard(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrUserCardNotFound, id)
	}
	return nil
}

// ResolveCard returns ranked catalog candidates for free-text input.
func (s *Service) ResolveCard(ctx context.Context, text string) ([]Candidate, error) {
	catalog, err := s.store.ListCatalogCards(ctx)
	if err != nil {
		return nil, err
	}
	return ResolveCard(text, catalog), nil
}

// ListCatalog returns every master-catalog card. The web wallet uses it to
// resolve each wallet entry's card type for display (J01).
func (s *Service) ListCatalog(ctx context.Context) ([]CatalogCard, error) {
	return s.store.ListCatalogCards(ctx)
}

// CreateOffer validates and persists an offer. When o.UserCardID is set the
// referenced wallet entry must exist.
func (s *Service) CreateOffer(ctx context.Context, o Offer) (*Offer, error) {
	o.Title = strings.TrimSpace(o.Title)
	o.Category = strings.TrimSpace(o.Category)
	if o.Title == "" {
		return nil, validationErr("offer title is required")
	}
	if o.Category == "" {
		return nil, validationErr("offer category is required")
	}
	if !ValidRateType(o.RateType) {
		return nil, validationErr("offer rate_type must be one of percent|points|multiplier, got %q", o.RateType)
	}
	if err := s.ensureUserCardExists(ctx, o.UserCardID); err != nil {
		return nil, err
	}
	o.ID = uuid.NewString()
	if err := s.store.CreateOffer(ctx, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOffersByUserCard returns offers for one wallet entry.
func (s *Service) ListOffersByUserCard(ctx context.Context, userCardID string) ([]Offer, error) {
	return s.store.ListOffersByUserCard(ctx, userCardID)
}

// ListOffersBySharedLimitGroup returns offers sharing a combined-limit pool.
func (s *Service) ListOffersBySharedLimitGroup(ctx context.Context, group string) ([]Offer, error) {
	return s.store.ListOffersBySharedLimitGroup(ctx, group)
}

// GetOffer returns one offer or ErrOfferNotFound.
func (s *Service) GetOffer(ctx context.Context, id string) (*Offer, error) {
	o, err := s.store.GetOffer(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("%w: %s", ErrOfferNotFound, id)
	}
	return o, nil
}

// UpdateOffer validates and persists changes to an existing offer (J06 edit /
// activation toggle). The referenced wallet entry (when set) must exist.
func (s *Service) UpdateOffer(ctx context.Context, o Offer) (*Offer, error) {
	o.ID = strings.TrimSpace(o.ID)
	if o.ID == "" {
		return nil, validationErr("offer id is required")
	}
	o.Title = strings.TrimSpace(o.Title)
	o.Category = strings.TrimSpace(o.Category)
	if o.Title == "" {
		return nil, validationErr("offer title is required")
	}
	if o.Category == "" {
		return nil, validationErr("offer category is required")
	}
	if !ValidRateType(o.RateType) {
		return nil, validationErr("offer rate_type must be one of percent|points|multiplier, got %q", o.RateType)
	}
	if err := s.ensureUserCardExists(ctx, o.UserCardID); err != nil {
		return nil, err
	}
	ok, err := s.store.UpdateOffer(ctx, &o)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrOfferNotFound, o.ID)
	}
	return s.store.GetOffer(ctx, o.ID)
}

// DeleteOffer removes an offer. Returns ErrOfferNotFound if no row matched.
func (s *Service) DeleteOffer(ctx context.Context, id string) error {
	ok, err := s.store.DeleteOffer(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrOfferNotFound, id)
	}
	return nil
}

// ListOffers returns every offer (web offers index).
func (s *Service) ListOffers(ctx context.Context) ([]Offer, error) {
	return s.store.ListOffers(ctx)
}

// CreateSelection validates and persists a selectable-category choice. The
// referenced wallet entry must exist.
func (s *Service) CreateSelection(ctx context.Context, sel Selection) (*Selection, error) {
	sel.Category = strings.TrimSpace(sel.Category)
	sel.PeriodLabel = strings.TrimSpace(sel.PeriodLabel)
	if sel.UserCardID == "" {
		return nil, validationErr("selection user_card_id is required")
	}
	if sel.Category == "" {
		return nil, validationErr("selection category is required")
	}
	if sel.PeriodLabel == "" {
		return nil, validationErr("selection period_label is required")
	}
	if sel.Tier != nil && *sel.Tier <= 0 {
		return nil, validationErr("selection tier must be > 0 when set")
	}
	uc := sel.UserCardID
	if err := s.ensureUserCardExists(ctx, &uc); err != nil {
		return nil, err
	}
	sel.ID = uuid.NewString()
	if err := s.store.CreateSelection(ctx, &sel); err != nil {
		return nil, err
	}
	return &sel, nil
}

// ListSelectionsByUserCard returns selections for one wallet entry.
func (s *Service) ListSelectionsByUserCard(ctx context.Context, userCardID string) ([]Selection, error) {
	return s.store.ListSelectionsByUserCard(ctx, userCardID)
}

// GetSelection returns one selection or ErrSelectionNotFound.
func (s *Service) GetSelection(ctx context.Context, id string) (*Selection, error) {
	sel, err := s.store.GetSelection(ctx, id)
	if err != nil {
		return nil, err
	}
	if sel == nil {
		return nil, fmt.Errorf("%w: %s", ErrSelectionNotFound, id)
	}
	return sel, nil
}

// UpdateSelection validates and persists changes to an existing selection
// (J07 edit). The referenced wallet entry must exist.
func (s *Service) UpdateSelection(ctx context.Context, sel Selection) (*Selection, error) {
	sel.ID = strings.TrimSpace(sel.ID)
	if sel.ID == "" {
		return nil, validationErr("selection id is required")
	}
	sel.Category = strings.TrimSpace(sel.Category)
	sel.PeriodLabel = strings.TrimSpace(sel.PeriodLabel)
	if sel.UserCardID == "" {
		return nil, validationErr("selection user_card_id is required")
	}
	if sel.Category == "" {
		return nil, validationErr("selection category is required")
	}
	if sel.PeriodLabel == "" {
		return nil, validationErr("selection period_label is required")
	}
	if sel.Tier != nil && *sel.Tier <= 0 {
		return nil, validationErr("selection tier must be > 0 when set")
	}
	uc := sel.UserCardID
	if err := s.ensureUserCardExists(ctx, &uc); err != nil {
		return nil, err
	}
	ok, err := s.store.UpdateSelection(ctx, &sel)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSelectionNotFound, sel.ID)
	}
	return s.store.GetSelection(ctx, sel.ID)
}

// ListSelections returns every selection (web selections index).
func (s *Service) ListSelections(ctx context.Context) ([]Selection, error) {
	return s.store.ListSelections(ctx)
}

// CreateSignupBonus validates and persists a signup-bonus tracker. The
// referenced wallet entry must exist.
func (s *Service) CreateSignupBonus(ctx context.Context, b SignupBonus) (*SignupBonus, error) {
	b.Description = strings.TrimSpace(b.Description)
	if b.UserCardID == "" {
		return nil, validationErr("signup bonus user_card_id is required")
	}
	if !ValidBonusType(b.BonusType) {
		return nil, validationErr("signup bonus bonus_type must be one of spend|first_year_rate, got %q", b.BonusType)
	}
	if b.Description == "" {
		return nil, validationErr("signup bonus description is required")
	}
	if b.SpendProgressCents < 0 {
		return nil, validationErr("signup bonus spend_progress_cents must be >= 0")
	}
	uc := b.UserCardID
	if err := s.ensureUserCardExists(ctx, &uc); err != nil {
		return nil, err
	}
	b.ID = uuid.NewString()
	if err := s.store.CreateSignupBonus(ctx, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBonusesByUserCard returns signup bonuses for one wallet entry.
func (s *Service) ListBonusesByUserCard(ctx context.Context, userCardID string) ([]SignupBonus, error) {
	return s.store.ListBonusesByUserCard(ctx, userCardID)
}

// GetBonus returns one signup bonus or ErrBonusNotFound.
func (s *Service) GetBonus(ctx context.Context, id string) (*SignupBonus, error) {
	b, err := s.store.GetSignupBonus(ctx, id)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("%w: %s", ErrBonusNotFound, id)
	}
	return b, nil
}

// UpdateBonus validates and persists changes to an existing signup bonus —
// notably manual spend-progress entry (bonuses "progress"). For spend-type
// bonuses with a known requirement, Met is recomputed from the new progress.
// The referenced wallet entry must exist.
func (s *Service) UpdateBonus(ctx context.Context, b SignupBonus) (*SignupBonus, error) {
	b.ID = strings.TrimSpace(b.ID)
	if b.ID == "" {
		return nil, validationErr("signup bonus id is required")
	}
	b.Description = strings.TrimSpace(b.Description)
	if b.UserCardID == "" {
		return nil, validationErr("signup bonus user_card_id is required")
	}
	if !ValidBonusType(b.BonusType) {
		return nil, validationErr("signup bonus bonus_type must be one of spend|first_year_rate, got %q", b.BonusType)
	}
	if b.Description == "" {
		return nil, validationErr("signup bonus description is required")
	}
	if b.SpendProgressCents < 0 {
		return nil, validationErr("signup bonus spend_progress_cents must be >= 0")
	}
	if b.SpendRequiredCents != nil {
		b.Met = b.SpendProgressCents >= *b.SpendRequiredCents
	}
	uc := b.UserCardID
	if err := s.ensureUserCardExists(ctx, &uc); err != nil {
		return nil, err
	}
	ok, err := s.store.UpdateSignupBonus(ctx, &b)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrBonusNotFound, b.ID)
	}
	return s.store.GetSignupBonus(ctx, b.ID)
}

// ListBonuses returns every signup bonus (web bonuses index).
func (s *Service) ListBonuses(ctx context.Context) ([]SignupBonus, error) {
	return s.store.ListBonuses(ctx)
}

// ListCategoryAliases returns every category alias.
func (s *Service) ListCategoryAliases(ctx context.Context) ([]CategoryAlias, error) {
	return s.store.ListCategoryAliases(ctx)
}

// CreateCategoryAlias validates and upserts a tracked spend category (and its
// equivalents) keyed on the canonical name. Idempotent on canonical_category,
// so re-posting the same category updates its equivalents/star/priority. Used
// by web category management and to declare the categories recommendation
// generation tracks (Scope 07).
func (s *Service) CreateCategoryAlias(ctx context.Context, a CategoryAlias) (*CategoryAlias, error) {
	a.CanonicalCategory = strings.TrimSpace(a.CanonicalCategory)
	if a.CanonicalCategory == "" {
		return nil, validationErr("canonical_category is required")
	}
	if a.Priority != nil && *a.Priority < 0 {
		return nil, validationErr("priority must be >= 0 when set")
	}
	cleaned := make([]string, 0, len(a.Equivalents))
	for _, e := range a.Equivalents {
		if e = strings.TrimSpace(e); e != "" {
			cleaned = append(cleaned, e)
		}
	}
	a.Equivalents = cleaned
	a.ID = uuid.NewString()
	if err := s.store.UpsertCategoryAlias(ctx, &a); err != nil {
		return nil, err
	}
	aliases, err := s.store.ListCategoryAliases(ctx)
	if err != nil {
		return nil, err
	}
	for i := range aliases {
		if strings.EqualFold(aliases[i].CanonicalCategory, a.CanonicalCategory) {
			return &aliases[i], nil
		}
	}
	return &a, nil
}

// GenerateRecommendations runs the optimizer across the tracked categories and
// writes one card_recommendations row per (period, category), preserving
// starred overrides (Scope 07 / SCN-083-G06, G07). An empty period means the
// current monthly period.
func (s *Service) GenerateRecommendations(ctx context.Context, period, trigger string) (RecommendationReport, error) {
	return s.recommender.GenerateRecommendations(ctx, period, trigger)
}

// ListRecommendations returns the persisted recommendations for a period. An
// empty period means the current monthly period (SCN-083-G08).
func (s *Service) ListRecommendations(ctx context.Context, period string) (string, []CardRecommendation, error) {
	if period == "" {
		period = s.recommender.CurrentPeriod()
	}
	recs, err := s.store.ListRecommendationsByPeriod(ctx, period)
	if err != nil {
		return period, nil, err
	}
	return period, recs, nil
}

// OptimizationReport returns the read-only optimizer breakdown for a period. An
// empty period means the current monthly period (SCN-083-G08).
func (s *Service) OptimizationReport(ctx context.Context, period string) (OptimizationReport, error) {
	return s.recommender.BuildOptimizationReport(ctx, period)
}

// ensureUserCardExists returns ErrUserCardNotFound when id is non-nil and the
// referenced wallet entry is absent. A nil id (general offer) is allowed.
func (s *Service) ensureUserCardExists(ctx context.Context, id *string) error {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	uc, err := s.store.GetUserCard(ctx, *id)
	if err != nil {
		return err
	}
	if uc == nil {
		return fmt.Errorf("%w: %s", ErrUserCardNotFound, *id)
	}
	return nil
}

// trimPtr trims a string pointer, returning nil for nil or whitespace-only.
func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}
