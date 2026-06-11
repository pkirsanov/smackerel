package cardrewards

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles PostgreSQL operations for the card-rewards domain (spec 083
// design §2). It owns no business logic — validation and ID generation live in
// Service. Mirrors internal/mealplan.Store.
type Store struct {
	Pool *pgxpool.Pool
}

// NewStore creates a new card-rewards store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

// jsonbOrEmpty returns the raw JSON as a string for a NOT NULL jsonb column,
// substituting the SQL-side default literal when the value is absent. The
// caller MUST cast the bind parameter with ::jsonb so pgx encodes it as jsonb
// rather than bytea.
func jsonbOrEmpty(raw json.RawMessage, dflt string) string {
	if len(raw) == 0 {
		return dflt
	}
	return string(raw)
}

// nullableJSONB returns a string (jsonb) bind value, or nil for a SQL NULL.
func nullableJSONB(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

// nonNilStrings normalizes a nil slice to an empty (non-nil) slice. The TEXT[]
// columns (aliases, equivalents) are NOT NULL: pgx encodes a nil []string as
// SQL NULL, which violates the constraint when the column is named explicitly
// in the INSERT (the DEFAULT '{}' only applies when the column is omitted).
// Passing []string{} encodes as '{}'.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ---- card_catalog ----------------------------------------------------------

// CreateCatalogCard inserts a new card_catalog row.
func (s *Store) CreateCatalogCard(ctx context.Context, c *CatalogCard) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO card_catalog
		   (id, name, issuer, card_type, annual_fee_cents, requires,
		    base_benefits, rotating_benefits, selectable_benefits, perks,
		    aliases, source, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,$11,$12,NOW(),NOW())`,
		c.ID, c.Name, c.Issuer, c.CardType, c.AnnualFeeCents, c.Requires,
		jsonbOrEmpty(c.BaseBenefits, "[]"), nullableJSONB(c.RotatingBenefits),
		nullableJSONB(c.SelectableBenefits), jsonbOrEmpty(c.Perks, "[]"),
		nonNilStrings(c.Aliases), c.Source,
	)
	return err
}

// CreateCustomCard atomically inserts a manual (non-catalog) card and its
// wallet entry in a single transaction (B04). Either both rows are written or
// neither — no orphan catalog row on partial failure.
func (s *Store) CreateCustomCard(ctx context.Context, c *CatalogCard, u *UserCard) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO card_catalog
		   (id, name, issuer, card_type, annual_fee_cents, requires,
		    base_benefits, rotating_benefits, selectable_benefits, perks,
		    aliases, source, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,$11,$12,NOW(),NOW())`,
		c.ID, c.Name, c.Issuer, c.CardType, c.AnnualFeeCents, c.Requires,
		jsonbOrEmpty(c.BaseBenefits, "[]"), nullableJSONB(c.RotatingBenefits),
		nullableJSONB(c.SelectableBenefits), jsonbOrEmpty(c.Perks, "[]"),
		nonNilStrings(c.Aliases), c.Source,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO user_cards (id, card_catalog_id, nickname, note, active, added_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NOW(),NOW(),NOW())`,
		u.ID, u.CardCatalogID, u.Nickname, u.Note, u.Active,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpsertCatalogCard inserts or updates a card_catalog row keyed on the stable
// text id. Used by the one-time CCManager import (Scope 03) for idempotent
// reseed; available now so the store surface is complete.
func (s *Store) UpsertCatalogCard(ctx context.Context, c *CatalogCard) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO card_catalog
		   (id, name, issuer, card_type, annual_fee_cents, requires,
		    base_benefits, rotating_benefits, selectable_benefits, perks,
		    aliases, source, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,$11,$12,NOW(),NOW())
		 ON CONFLICT (id) DO UPDATE SET
		   name=EXCLUDED.name, issuer=EXCLUDED.issuer, card_type=EXCLUDED.card_type,
		   annual_fee_cents=EXCLUDED.annual_fee_cents, requires=EXCLUDED.requires,
		   base_benefits=EXCLUDED.base_benefits, rotating_benefits=EXCLUDED.rotating_benefits,
		   selectable_benefits=EXCLUDED.selectable_benefits, perks=EXCLUDED.perks,
		   aliases=EXCLUDED.aliases, source=EXCLUDED.source, updated_at=NOW()`,
		c.ID, c.Name, c.Issuer, c.CardType, c.AnnualFeeCents, c.Requires,
		jsonbOrEmpty(c.BaseBenefits, "[]"), nullableJSONB(c.RotatingBenefits),
		nullableJSONB(c.SelectableBenefits), jsonbOrEmpty(c.Perks, "[]"),
		nonNilStrings(c.Aliases), c.Source,
	)
	return err
}

func scanCatalogCard(row pgx.Row) (*CatalogCard, error) {
	var c CatalogCard
	var base, rotating, selectable, perks []byte
	err := row.Scan(&c.ID, &c.Name, &c.Issuer, &c.CardType, &c.AnnualFeeCents,
		&c.Requires, &base, &rotating, &selectable, &perks, &c.Aliases,
		&c.Source, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.BaseBenefits = json.RawMessage(base)
	c.RotatingBenefits = json.RawMessage(rotating)
	c.SelectableBenefits = json.RawMessage(selectable)
	c.Perks = json.RawMessage(perks)
	return &c, nil
}

const catalogCols = `id, name, issuer, card_type, annual_fee_cents, requires,
	base_benefits, rotating_benefits, selectable_benefits, perks, aliases,
	source, created_at, updated_at`

// GetCatalogCard returns a catalog card by id, or (nil, nil) if absent.
func (s *Store) GetCatalogCard(ctx context.Context, id string) (*CatalogCard, error) {
	c, err := scanCatalogCard(s.Pool.QueryRow(ctx,
		`SELECT `+catalogCols+` FROM card_catalog WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// ListCatalogCards returns every catalog card ordered by name (used by the
// resolver and the web catalog views).
func (s *Store) ListCatalogCards(ctx context.Context) ([]CatalogCard, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+catalogCols+` FROM card_catalog ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CatalogCard
	for rows.Next() {
		c, err := scanCatalogCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ---- user_cards ------------------------------------------------------------

// CreateUserCard inserts a wallet entry.
func (s *Store) CreateUserCard(ctx context.Context, u *UserCard) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO user_cards (id, card_catalog_id, nickname, note, active, added_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NOW(),NOW(),NOW())`,
		u.ID, u.CardCatalogID, u.Nickname, u.Note, u.Active,
	)
	return err
}

func scanUserCard(row pgx.Row) (*UserCard, error) {
	var u UserCard
	err := row.Scan(&u.ID, &u.CardCatalogID, &u.Nickname, &u.Note, &u.Active,
		&u.AddedAt, &u.CreatedAt, &u.UpdatedAt, &u.CatalogName)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

const userCardSelect = `SELECT uc.id, uc.card_catalog_id, uc.nickname, uc.note, uc.active,
	uc.added_at, uc.created_at, uc.updated_at,
	COALESCE(cc.name, '') AS catalog_name
	FROM user_cards uc
	LEFT JOIN card_catalog cc ON cc.id = uc.card_catalog_id`

// GetUserCard returns a wallet entry (with resolved catalog name) by id, or
// (nil, nil) if absent.
func (s *Store) GetUserCard(ctx context.Context, id string) (*UserCard, error) {
	u, err := scanUserCard(s.Pool.QueryRow(ctx, userCardSelect+` WHERE uc.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

// ListUserCards returns wallet entries ordered by add time (newest first).
// When activeOnly is true only active cards are returned.
func (s *Store) ListUserCards(ctx context.Context, activeOnly bool) ([]UserCard, error) {
	q := userCardSelect
	if activeOnly {
		q += ` WHERE uc.active = true`
	}
	q += ` ORDER BY uc.added_at DESC`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserCard
	for rows.Next() {
		u, err := scanUserCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateUserCard updates the mutable wallet fields (nickname, note, active) and
// refreshes updated_at. Returns false if no row matched.
func (s *Store) UpdateUserCard(ctx context.Context, u *UserCard) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE user_cards SET nickname=$1, note=$2, active=$3, updated_at=NOW() WHERE id=$4`,
		u.Nickname, u.Note, u.Active, u.ID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteUserCard removes a wallet entry. Dependent offers, selections, and
// signup bonuses are removed by ON DELETE CASCADE (migration 057). Returns
// false if no row matched.
func (s *Store) DeleteUserCard(ctx context.Context, id string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM user_cards WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ---- card_offers -----------------------------------------------------------

// CreateOffer inserts an offer.
func (s *Store) CreateOffer(ctx context.Context, o *Offer) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO card_offers
		   (id, user_card_id, title, category, rate, rate_type, limit_cents, limit_period,
		    shared_limit_group, starts_on, ends_on, activation_required, activated, notes,
		    created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW())`,
		o.ID, o.UserCardID, o.Title, o.Category, o.Rate, o.RateType, o.LimitCents,
		o.LimitPeriod, o.SharedLimitGroup, o.StartsOn, o.EndsOn, o.ActivationRequired,
		o.Activated, o.Notes,
	)
	return err
}

func scanOffer(row pgx.Row) (*Offer, error) {
	var o Offer
	err := row.Scan(&o.ID, &o.UserCardID, &o.Title, &o.Category, &o.Rate, &o.RateType,
		&o.LimitCents, &o.LimitPeriod, &o.SharedLimitGroup, &o.StartsOn, &o.EndsOn,
		&o.ActivationRequired, &o.Activated, &o.Notes, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

const offerCols = `id, user_card_id, title, category, rate, rate_type, limit_cents,
	limit_period, shared_limit_group, starts_on, ends_on, activation_required,
	activated, notes, created_at, updated_at`

func (s *Store) queryOffers(ctx context.Context, where string, arg any) ([]Offer, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+offerCols+` FROM card_offers WHERE `+where+` ORDER BY created_at`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Offer
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

// ListOffersByUserCard returns offers for one wallet entry.
func (s *Store) ListOffersByUserCard(ctx context.Context, userCardID string) ([]Offer, error) {
	return s.queryOffers(ctx, "user_card_id = $1", userCardID)
}

// ListOffersBySharedLimitGroup returns offers that share a combined-limit pool.
func (s *Store) ListOffersBySharedLimitGroup(ctx context.Context, group string) ([]Offer, error) {
	return s.queryOffers(ctx, "shared_limit_group = $1", group)
}

// GetOffer returns one offer by id, or (nil, nil) if absent.
func (s *Store) GetOffer(ctx context.Context, id string) (*Offer, error) {
	o, err := scanOffer(s.Pool.QueryRow(ctx, `SELECT `+offerCols+` FROM card_offers WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return o, err
}

// UpdateOffer rewrites the mutable offer columns and refreshes updated_at.
// Returns false if no row matched.
func (s *Store) UpdateOffer(ctx context.Context, o *Offer) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE card_offers SET title=$1, category=$2, rate=$3, rate_type=$4,
		   limit_cents=$5, limit_period=$6, shared_limit_group=$7, starts_on=$8,
		   ends_on=$9, activation_required=$10, activated=$11, notes=$12, updated_at=NOW()
		 WHERE id=$13`,
		o.Title, o.Category, o.Rate, o.RateType, o.LimitCents, o.LimitPeriod,
		o.SharedLimitGroup, o.StartsOn, o.EndsOn, o.ActivationRequired, o.Activated,
		o.Notes, o.ID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteOffer removes an offer. Returns false if no row matched.
func (s *Store) DeleteOffer(ctx context.Context, id string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM card_offers WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListOffers returns every offer (across all wallet entries and general
// offers), newest first. Backs the web offers index page.
func (s *Store) ListOffers(ctx context.Context) ([]Offer, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+offerCols+` FROM card_offers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Offer
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

// ---- card_selections -------------------------------------------------------

// CreateSelection inserts a selectable-category choice.
func (s *Store) CreateSelection(ctx context.Context, sel *Selection) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO card_selections
		   (id, user_card_id, category, tier, period_label, enrolled, enrolled_at,
		    effective_start, effective_end, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW())`,
		sel.ID, sel.UserCardID, sel.Category, sel.Tier, sel.PeriodLabel, sel.Enrolled,
		sel.EnrolledAt, sel.EffectiveStart, sel.EffectiveEnd,
	)
	return err
}

// ListSelectionsByUserCard returns the selections for one wallet entry.
func (s *Store) ListSelectionsByUserCard(ctx context.Context, userCardID string) ([]Selection, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, user_card_id, category, tier, period_label, enrolled, enrolled_at,
		        effective_start, effective_end, created_at, updated_at
		 FROM card_selections WHERE user_card_id = $1 ORDER BY period_label, category`, userCardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Selection
	for rows.Next() {
		var sel Selection
		if err := rows.Scan(&sel.ID, &sel.UserCardID, &sel.Category, &sel.Tier,
			&sel.PeriodLabel, &sel.Enrolled, &sel.EnrolledAt, &sel.EffectiveStart,
			&sel.EffectiveEnd, &sel.CreatedAt, &sel.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sel)
	}
	return out, rows.Err()
}

// GetSelection returns one selection by id, or (nil, nil) if absent.
func (s *Store) GetSelection(ctx context.Context, id string) (*Selection, error) {
	var sel Selection
	err := s.Pool.QueryRow(ctx,
		`SELECT id, user_card_id, category, tier, period_label, enrolled, enrolled_at,
		        effective_start, effective_end, created_at, updated_at
		 FROM card_selections WHERE id = $1`, id).Scan(
		&sel.ID, &sel.UserCardID, &sel.Category, &sel.Tier, &sel.PeriodLabel,
		&sel.Enrolled, &sel.EnrolledAt, &sel.EffectiveStart, &sel.EffectiveEnd,
		&sel.CreatedAt, &sel.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sel, nil
}

// UpdateSelection rewrites the mutable selection columns and refreshes
// updated_at. Returns false if no row matched.
func (s *Store) UpdateSelection(ctx context.Context, sel *Selection) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE card_selections SET category=$1, tier=$2, period_label=$3, enrolled=$4,
		   enrolled_at=$5, effective_start=$6, effective_end=$7, updated_at=NOW()
		 WHERE id=$8`,
		sel.Category, sel.Tier, sel.PeriodLabel, sel.Enrolled, sel.EnrolledAt,
		sel.EffectiveStart, sel.EffectiveEnd, sel.ID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListSelections returns every selection across all wallet entries, ordered by
// period then category. Backs the web selections index page.
func (s *Store) ListSelections(ctx context.Context) ([]Selection, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, user_card_id, category, tier, period_label, enrolled, enrolled_at,
		        effective_start, effective_end, created_at, updated_at
		 FROM card_selections ORDER BY period_label DESC, tier NULLS FIRST, category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Selection
	for rows.Next() {
		var sel Selection
		if err := rows.Scan(&sel.ID, &sel.UserCardID, &sel.Category, &sel.Tier,
			&sel.PeriodLabel, &sel.Enrolled, &sel.EnrolledAt, &sel.EffectiveStart,
			&sel.EffectiveEnd, &sel.CreatedAt, &sel.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sel)
	}
	return out, rows.Err()
}

// ListPendingReEnrollments returns selectable-card selections whose enrollment
// window is open at `now` (effective_start has passed and effective_end has
// not) but which are not yet enrolled, joined to the wallet entry's catalog
// name for display. These are the pending re-enrollment actions surfaced for
// the dashboard (SCN-083-F06 / UC-003 A2) — the smackerel equivalent of
// CCManager's "pending selections". Only active wallet cards are considered.
func (s *Store) ListPendingReEnrollments(ctx context.Context, now time.Time) ([]PendingReEnrollment, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT cs.user_card_id, COALESCE(cc.name, ''), cs.category, cs.tier,
		        cs.period_label, cs.effective_start, cs.effective_end
		   FROM card_selections cs
		   JOIN user_cards uc ON uc.id = cs.user_card_id
		   LEFT JOIN card_catalog cc ON cc.id = uc.card_catalog_id
		  WHERE cs.enrolled = false
		    AND uc.active = true
		    AND cs.effective_start IS NOT NULL
		    AND cs.effective_start <= $1::date
		    AND (cs.effective_end IS NULL OR cs.effective_end >= $1::date)
		  ORDER BY cs.effective_start, cs.category`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingReEnrollment
	for rows.Next() {
		var p PendingReEnrollment
		if err := rows.Scan(&p.UserCardID, &p.CatalogName, &p.Category, &p.Tier,
			&p.PeriodLabel, &p.EffectiveStart, &p.EffectiveEnd); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- signup_bonuses --------------------------------------------------------

// CreateSignupBonus inserts a signup-bonus tracker.
func (s *Store) CreateSignupBonus(ctx context.Context, b *SignupBonus) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO signup_bonuses
		   (id, user_card_id, bonus_type, description, spend_required_cents,
		    spend_progress_cents, reward_description, deadline, met, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW())`,
		b.ID, b.UserCardID, b.BonusType, b.Description, b.SpendRequiredCents,
		b.SpendProgressCents, b.RewardDescription, b.Deadline, b.Met,
	)
	return err
}

// ListBonusesByUserCard returns the signup bonuses for one wallet entry.
func (s *Store) ListBonusesByUserCard(ctx context.Context, userCardID string) ([]SignupBonus, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, user_card_id, bonus_type, description, spend_required_cents,
		        spend_progress_cents, reward_description, deadline, met, created_at, updated_at
		 FROM signup_bonuses WHERE user_card_id = $1 ORDER BY created_at`, userCardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SignupBonus
	for rows.Next() {
		var b SignupBonus
		if err := rows.Scan(&b.ID, &b.UserCardID, &b.BonusType, &b.Description,
			&b.SpendRequiredCents, &b.SpendProgressCents, &b.RewardDescription,
			&b.Deadline, &b.Met, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetSignupBonus returns one signup bonus by id, or (nil, nil) if absent.
func (s *Store) GetSignupBonus(ctx context.Context, id string) (*SignupBonus, error) {
	var b SignupBonus
	err := s.Pool.QueryRow(ctx,
		`SELECT id, user_card_id, bonus_type, description, spend_required_cents,
		        spend_progress_cents, reward_description, deadline, met, created_at, updated_at
		 FROM signup_bonuses WHERE id = $1`, id).Scan(
		&b.ID, &b.UserCardID, &b.BonusType, &b.Description, &b.SpendRequiredCents,
		&b.SpendProgressCents, &b.RewardDescription, &b.Deadline, &b.Met,
		&b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// UpdateSignupBonus rewrites the mutable bonus columns (spend progress, met,
// deadline, …) and refreshes updated_at. Returns false if no row matched.
func (s *Store) UpdateSignupBonus(ctx context.Context, b *SignupBonus) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE signup_bonuses SET bonus_type=$1, description=$2, spend_required_cents=$3,
		   spend_progress_cents=$4, reward_description=$5, deadline=$6, met=$7, updated_at=NOW()
		 WHERE id=$8`,
		b.BonusType, b.Description, b.SpendRequiredCents, b.SpendProgressCents,
		b.RewardDescription, b.Deadline, b.Met, b.ID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListBonuses returns every signup bonus across all wallet entries, newest
// first. Backs the web bonuses index page.
func (s *Store) ListBonuses(ctx context.Context) ([]SignupBonus, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, user_card_id, bonus_type, description, spend_required_cents,
		        spend_progress_cents, reward_description, deadline, met, created_at, updated_at
		 FROM signup_bonuses ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SignupBonus
	for rows.Next() {
		var b SignupBonus
		if err := rows.Scan(&b.ID, &b.UserCardID, &b.BonusType, &b.Description,
			&b.SpendRequiredCents, &b.SpendProgressCents, &b.RewardDescription,
			&b.Deadline, &b.Met, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---- category_aliases ------------------------------------------------------

// UpsertCategoryAlias inserts or updates a category alias keyed on the unique
// canonical_category. Used by the CCManager import (Scope 03) and web category
// management; idempotent by design.
func (s *Store) UpsertCategoryAlias(ctx context.Context, a *CategoryAlias) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO category_aliases
		   (id, canonical_category, equivalents, starred, priority, built_in, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())
		 ON CONFLICT (canonical_category) DO UPDATE SET
		   equivalents=EXCLUDED.equivalents, starred=EXCLUDED.starred,
		   priority=EXCLUDED.priority, built_in=EXCLUDED.built_in, updated_at=NOW()`,
		a.ID, a.CanonicalCategory, nonNilStrings(a.Equivalents), a.Starred, a.Priority, a.BuiltIn,
	)
	return err
}

// ListCategoryAliases returns every category alias ordered by priority then name.
func (s *Store) ListCategoryAliases(ctx context.Context) ([]CategoryAlias, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, canonical_category, equivalents, starred, priority, built_in, created_at, updated_at
		 FROM category_aliases ORDER BY COALESCE(priority, 2147483647), canonical_category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CategoryAlias
	for rows.Next() {
		var a CategoryAlias
		if err := rows.Scan(&a.ID, &a.CanonicalCategory, &a.Equivalents, &a.Starred,
			&a.Priority, &a.BuiltIn, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- import idempotency helpers (Scope 03) ---------------------------------
//
// The one-time CCManager JSON import (design §11) must be safe to re-run: a
// second run creates zero duplicate rows (SCN-083-C02). card_catalog and
// category_aliases already have UpsertXxx methods keyed on their unique
// columns. The remaining wallet/offer/selection/bonus/run tables either have
// no usable unique constraint or a nullable key column (nickname, tier,
// user_card_id, started_at) where a UNIQUE constraint treats each NULL as
// distinct — so ON CONFLICT cannot dedupe them. These helpers use
// `INSERT … SELECT … WHERE NOT EXISTS (… IS NOT DISTINCT FROM …)`, which
// treats NULL = NULL as equal and is therefore correctly idempotent. The
// importer is single-threaded (one-time migration), so the check-then-insert
// has no concurrency hazard.

// GetOrCreateUserCardByCatalog returns the id of the wallet entry matching
// (card_catalog_id, nickname); if none exists it inserts the supplied UserCard
// (whose ID the caller has already generated) and returns it. The second
// return value reports whether a new row was inserted. Idempotent on the
// (card_catalog_id, nickname) natural key, treating a NULL nickname as equal
// to a NULL nickname.
func (s *Store) GetOrCreateUserCardByCatalog(ctx context.Context, u *UserCard) (string, bool, error) {
	var existing string
	err := s.Pool.QueryRow(ctx,
		`SELECT id FROM user_cards
		 WHERE card_catalog_id = $1 AND nickname IS NOT DISTINCT FROM $2
		 ORDER BY added_at LIMIT 1`,
		u.CardCatalogID, u.Nickname,
	).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	if _, err := s.Pool.Exec(ctx,
		`INSERT INTO user_cards (id, card_catalog_id, nickname, note, active, added_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NOW(),NOW(),NOW())`,
		u.ID, u.CardCatalogID, u.Nickname, u.Note, u.Active,
	); err != nil {
		return "", false, err
	}
	return u.ID, true, nil
}

// InsertOfferIfAbsent inserts an offer only when no row with the same
// (title, category, user_card_id) natural key already exists. Returns true
// when a row was inserted.
func (s *Store) InsertOfferIfAbsent(ctx context.Context, o *Offer) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO card_offers
		   (id, user_card_id, title, category, rate, rate_type, limit_cents, limit_period,
		    shared_limit_group, starts_on, ends_on, activation_required, activated, notes,
		    created_at, updated_at)
		 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM card_offers
		   WHERE title = $3 AND category = $4 AND user_card_id IS NOT DISTINCT FROM $2
		 )`,
		o.ID, o.UserCardID, o.Title, o.Category, o.Rate, o.RateType, o.LimitCents,
		o.LimitPeriod, o.SharedLimitGroup, o.StartsOn, o.EndsOn, o.ActivationRequired,
		o.Activated, o.Notes,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// InsertSelectionIfAbsent inserts a selection only when no row with the same
// (user_card_id, period_label, tier, category) natural key already exists,
// treating a NULL tier as equal to a NULL tier. Returns true when inserted.
func (s *Store) InsertSelectionIfAbsent(ctx context.Context, sel *Selection) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO card_selections
		   (id, user_card_id, category, tier, period_label, enrolled, enrolled_at,
		    effective_start, effective_end, created_at, updated_at)
		 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM card_selections
		   WHERE user_card_id = $2 AND period_label = $5
		     AND tier IS NOT DISTINCT FROM $4 AND category = $3
		 )`,
		sel.ID, sel.UserCardID, sel.Category, sel.Tier, sel.PeriodLabel, sel.Enrolled,
		sel.EnrolledAt, sel.EffectiveStart, sel.EffectiveEnd,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// InsertSignupBonusIfAbsent inserts a signup bonus only when no row with the
// same (user_card_id, bonus_type, description) natural key already exists.
// Returns true when inserted.
func (s *Store) InsertSignupBonusIfAbsent(ctx context.Context, b *SignupBonus) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO signup_bonuses
		   (id, user_card_id, bonus_type, description, spend_required_cents,
		    spend_progress_cents, reward_description, deadline, met, created_at, updated_at)
		 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),NOW()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM signup_bonuses
		   WHERE user_card_id = $2 AND bonus_type = $3 AND description = $4
		 )`,
		b.ID, b.UserCardID, b.BonusType, b.Description, b.SpendRequiredCents,
		b.SpendProgressCents, b.RewardDescription, b.Deadline, b.Met,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ---- rotating_categories ---------------------------------------------------

// UpsertRotatingCategory inserts or updates a reconciled rotating-category
// record keyed on (card_catalog_id, period_label). Used by the CCManager
// import (Scope 03, seeds ManualOverride=true) and later the reconciler
// (Scope 06). Idempotent.
func (s *Store) UpsertRotatingCategory(ctx context.Context, rc *RotatingCategory) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO rotating_categories
		   (id, card_catalog_id, period_label, period_start, period_end, categories,
		    limit_cents, activation_required, lifecycle_state, confidence,
		    needs_verification, manual_override, source_count, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW(),NOW())
		 ON CONFLICT (card_catalog_id, period_label) DO UPDATE SET
		   period_start=EXCLUDED.period_start, period_end=EXCLUDED.period_end,
		   categories=EXCLUDED.categories, limit_cents=EXCLUDED.limit_cents,
		   activation_required=EXCLUDED.activation_required,
		   lifecycle_state=EXCLUDED.lifecycle_state, confidence=EXCLUDED.confidence,
		   needs_verification=EXCLUDED.needs_verification,
		   manual_override=EXCLUDED.manual_override, source_count=EXCLUDED.source_count,
		   updated_at=NOW()`,
		rc.ID, rc.CardCatalogID, rc.PeriodLabel, rc.PeriodStart, rc.PeriodEnd,
		nonNilStrings(rc.Categories), rc.LimitCents, rc.ActivationRequired,
		rc.LifecycleState, rc.Confidence, rc.NeedsVerification, rc.ManualOverride,
		rc.SourceCount,
	)
	return err
}

// ListRotatingCategoriesByCard returns the rotating-category records for one
// catalog card ordered by period label (used by tests and later scopes).
func (s *Store) ListRotatingCategoriesByCard(ctx context.Context, catalogID string) ([]RotatingCategory, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, card_catalog_id, period_label, period_start, period_end, categories,
		        limit_cents, activation_required, lifecycle_state, confidence,
		        needs_verification, manual_override, source_count, created_at, updated_at
		 FROM rotating_categories WHERE card_catalog_id = $1 ORDER BY period_label`, catalogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RotatingCategory
	for rows.Next() {
		var rc RotatingCategory
		if err := rows.Scan(&rc.ID, &rc.CardCatalogID, &rc.PeriodLabel, &rc.PeriodStart,
			&rc.PeriodEnd, &rc.Categories, &rc.LimitCents, &rc.ActivationRequired,
			&rc.LifecycleState, &rc.Confidence, &rc.NeedsVerification, &rc.ManualOverride,
			&rc.SourceCount, &rc.CreatedAt, &rc.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// GetRotatingCategory returns the reconciled rotating-category record for a
// (card_catalog_id, period_label), or (nil, nil) when absent. Used by the
// extractor to detect an existing record that must be flagged (not overwritten)
// when an extraction fails to validate (SCN-083-E03), and by tests.
func (s *Store) GetRotatingCategory(ctx context.Context, catalogID, periodLabel string) (*RotatingCategory, error) {
	var rc RotatingCategory
	err := s.Pool.QueryRow(ctx,
		`SELECT id, card_catalog_id, period_label, period_start, period_end, categories,
		        limit_cents, activation_required, lifecycle_state, confidence,
		        needs_verification, manual_override, source_count, created_at, updated_at
		 FROM rotating_categories WHERE card_catalog_id = $1 AND period_label = $2`,
		catalogID, periodLabel,
	).Scan(&rc.ID, &rc.CardCatalogID, &rc.PeriodLabel, &rc.PeriodStart, &rc.PeriodEnd,
		&rc.Categories, &rc.LimitCents, &rc.ActivationRequired, &rc.LifecycleState,
		&rc.Confidence, &rc.NeedsVerification, &rc.ManualOverride, &rc.SourceCount,
		&rc.CreatedAt, &rc.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rc, nil
}

const rotatingCategoryCols = `id, card_catalog_id, period_label, period_start, period_end, categories,
	limit_cents, activation_required, lifecycle_state, confidence,
	needs_verification, manual_override, source_count, created_at, updated_at`

func scanRotatingCategory(row pgx.Row) (*RotatingCategory, error) {
	var rc RotatingCategory
	if err := row.Scan(&rc.ID, &rc.CardCatalogID, &rc.PeriodLabel, &rc.PeriodStart,
		&rc.PeriodEnd, &rc.Categories, &rc.LimitCents, &rc.ActivationRequired,
		&rc.LifecycleState, &rc.Confidence, &rc.NeedsVerification, &rc.ManualOverride,
		&rc.SourceCount, &rc.CreatedAt, &rc.UpdatedAt); err != nil {
		return nil, err
	}
	return &rc, nil
}

// ListAllRotatingCategories returns every reconciled rotating-category record
// ordered by (card, period). Used by the daily lifecycle pass (Scope 06) to
// advance lifecycle_state by date (Principle 3).
func (s *Store) ListAllRotatingCategories(ctx context.Context) ([]RotatingCategory, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+rotatingCategoryCols+` FROM rotating_categories ORDER BY card_catalog_id, period_label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RotatingCategory
	for rows.Next() {
		rc, err := scanRotatingCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rc)
	}
	return out, rows.Err()
}

// ListActiveRotatingCategories returns only the records currently in the
// `active` lifecycle state — the set eligible for current recommendations.
// Expired and upcoming records are excluded (SCN-083-F05).
func (s *Store) ListActiveRotatingCategories(ctx context.Context) ([]RotatingCategory, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+rotatingCategoryCols+` FROM rotating_categories
		 WHERE lifecycle_state = $1 ORDER BY card_catalog_id, period_label`, LifecycleActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RotatingCategory
	for rows.Next() {
		rc, err := scanRotatingCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rc)
	}
	return out, rows.Err()
}

// UpdateRotatingLifecycle sets a record's lifecycle_state and refreshes
// updated_at. Returns false if no row matched. It NEVER touches categories,
// confidence, manual_override, or needs_verification — lifecycle is a pure
// date-derived transition (Principle 3 / FR-CR-012).
func (s *Store) UpdateRotatingLifecycle(ctx context.Context, id, state string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE rotating_categories SET lifecycle_state = $1, updated_at = NOW() WHERE id = $2`,
		state, id,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CountRotatingCategoriesByCardPeriod returns the number of rotating_categories
// rows for a (card_catalog_id, period_label). The UNIQUE constraint guarantees
// this is 0 or 1; the reconciler's idempotency test asserts it stays exactly 1
// across repeated runs (SCN-083-F07).
func (s *Store) CountRotatingCategoriesByCardPeriod(ctx context.Context, catalogID, periodLabel string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM rotating_categories WHERE card_catalog_id = $1 AND period_label = $2`,
		catalogID, periodLabel,
	).Scan(&n)
	return n, err
}

// ---- rotating_category_observations ----------------------------------------

const observationCols = `id, card_catalog_id, period_label, period_start, period_end,
	categories, limit_cents, activation_required, confidence, source_name,
	source_url, source_evidence, extraction_run_id, observed_at`

func scanObservation(row pgx.Row) (*RotatingCategoryObservation, error) {
	var o RotatingCategoryObservation
	if err := row.Scan(&o.ID, &o.CardCatalogID, &o.PeriodLabel, &o.PeriodStart,
		&o.PeriodEnd, &o.Categories, &o.LimitCents, &o.ActivationRequired,
		&o.Confidence, &o.SourceName, &o.SourceURL, &o.SourceEvidence,
		&o.ExtractionRunID, &o.ObservedAt); err != nil {
		return nil, err
	}
	return &o, nil
}

// ListObservationsByCardPeriod returns the per-source observations for a
// (card_catalog_id, period_label) ordered by observed_at. Used by the
// reconciler (Scope 06) and tests.
func (s *Store) ListObservationsByCardPeriod(ctx context.Context, catalogID, periodLabel string) ([]RotatingCategoryObservation, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+observationCols+`
		 FROM rotating_category_observations
		 WHERE card_catalog_id = $1 AND period_label = $2
		 ORDER BY observed_at`, catalogID, periodLabel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RotatingCategoryObservation
	for rows.Next() {
		o, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

// ListObservationRefs returns the distinct (card_catalog_id, period_label)
// pairs that have at least one rotating_category_observation, ordered
// deterministically. The daily-refresh pipeline (Scope 09) drives
// reconciliation over every ref this returns, so a refresh that adds NO new
// extraction still reconciles previously-stored observations into the
// authoritative rotating_categories record — which is what makes a re-run
// idempotent (SCN-083-I06): the same observations upsert the same single row
// per (card, period), never a duplicate.
func (s *Store) ListObservationRefs(ctx context.Context) ([]CardPeriodRef, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT DISTINCT card_catalog_id, period_label
		 FROM rotating_category_observations
		 ORDER BY card_catalog_id, period_label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CardPeriodRef
	for rows.Next() {
		var ref CardPeriodRef
		if err := rows.Scan(&ref.CardCatalogID, &ref.PeriodLabel); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// PersistExtractionRun atomically writes one extraction audit run and its
// validated observations, and flags any existing reconciled records that an
// extraction failed to validate (SCN-083-E03/E08). All three writes share one
// transaction so the observations' extraction_run_id FK is always satisfied and
// a mid-batch failure leaves no partial audit trail. Returns the number of
// existing rotating_categories rows flagged needs_verification.
//
// Flagging is a pure UPDATE of needs_verification + updated_at; it NEVER
// rewrites categories/confidence/limit — the reconciled record is preserved,
// never overwritten with stale or placeholder data (design §4, the CCManager
// silent-fallback failure mode this scope replaces).
func (s *Store) PersistExtractionRun(ctx context.Context, run *CardRun, observations []RotatingCategoryObservation, flags []CardPeriodRef) (int, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO card_runs
		   (id, run_type, trigger, status, sources_attempted, sources_succeeded,
		    categories_extracted, events_written, error_detail, started_at, finished_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())`,
		run.ID, run.RunType, run.Trigger, run.Status, run.SourcesAttempted,
		run.SourcesSucceeded, run.CategoriesExtracted, run.EventsWritten,
		run.ErrorDetail, run.StartedAt, run.FinishedAt,
	); err != nil {
		return 0, err
	}

	for i := range observations {
		o := &observations[i]
		if _, err := tx.Exec(ctx,
			`INSERT INTO rotating_category_observations
			   (id, card_catalog_id, period_label, period_start, period_end, categories,
			    limit_cents, activation_required, confidence, source_name, source_url,
			    source_evidence, extraction_run_id, observed_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			o.ID, o.CardCatalogID, o.PeriodLabel, o.PeriodStart, o.PeriodEnd,
			nonNilStrings(o.Categories), o.LimitCents, o.ActivationRequired, o.Confidence,
			o.SourceName, o.SourceURL, o.SourceEvidence, o.ExtractionRunID, o.ObservedAt,
		); err != nil {
			return 0, err
		}
	}

	flagged := 0
	for _, f := range flags {
		tag, err := tx.Exec(ctx,
			`UPDATE rotating_categories
			    SET needs_verification = true, updated_at = NOW()
			  WHERE card_catalog_id = $1 AND period_label = $2`,
			f.CardCatalogID, f.PeriodLabel,
		)
		if err != nil {
			return 0, err
		}
		flagged += int(tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return flagged, nil
}

// ---- card_recommendations --------------------------------------------------

// UpsertRecommendation inserts or updates a recommendation keyed on
// (period_label, category). Idempotent; used by the import (Scope 03) and the
// optimizer (Scope 07).
func (s *Store) UpsertRecommendation(ctx context.Context, r *CardRecommendation) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO card_recommendations
		   (id, period_label, category, recommended_user_card_id, rate, reason,
		    starred, starred_override, calendar_event_uid, generated_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())
		 ON CONFLICT (period_label, category) DO UPDATE SET
		   recommended_user_card_id=EXCLUDED.recommended_user_card_id, rate=EXCLUDED.rate,
		   reason=EXCLUDED.reason, starred=EXCLUDED.starred,
		   starred_override=EXCLUDED.starred_override,
		   calendar_event_uid=EXCLUDED.calendar_event_uid,
		   generated_at=EXCLUDED.generated_at, updated_at=NOW()`,
		r.ID, r.PeriodLabel, r.Category, r.RecommendedUserCardID, r.Rate, r.Reason,
		r.Starred, r.StarredOverride, r.CalendarEventUID, r.GeneratedAt,
	)
	return err
}

const recommendationCols = `id, period_label, category, recommended_user_card_id, rate,
	reason, starred, starred_override, calendar_event_uid, generated_at, created_at, updated_at`

func scanRecommendation(row pgx.Row) (*CardRecommendation, error) {
	var rec CardRecommendation
	if err := row.Scan(&rec.ID, &rec.PeriodLabel, &rec.Category, &rec.RecommendedUserCardID,
		&rec.Rate, &rec.Reason, &rec.Starred, &rec.StarredOverride, &rec.CalendarEventUID,
		&rec.GeneratedAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	return &rec, nil
}

// GetRecommendation returns the recommendation for a (period_label, category),
// or (nil, nil) when absent. Used by the optimizer to detect a starred_override
// row that must be preserved (SCN-083-G07) and by tests.
func (s *Store) GetRecommendation(ctx context.Context, period, category string) (*CardRecommendation, error) {
	rec, err := scanRecommendation(s.Pool.QueryRow(ctx,
		`SELECT `+recommendationCols+` FROM card_recommendations
		 WHERE period_label = $1 AND category = $2`, period, category))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// ListRecommendationsByPeriod returns every recommendation for a period ordered
// by category (used by the recommendations endpoint, SCN-083-G08).
func (s *Store) ListRecommendationsByPeriod(ctx context.Context, period string) ([]CardRecommendation, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+recommendationCols+` FROM card_recommendations
		 WHERE period_label = $1 ORDER BY category`, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CardRecommendation
	for rows.Next() {
		rec, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// ---- card_runs -------------------------------------------------------------

// CreateRun inserts a single audit run row (always a fresh insert — each
// invocation is its own audit record per Principle 8). Used for the import's
// own run_type="migration" record (SCN-083-C06).
func (s *Store) CreateRun(ctx context.Context, r *CardRun) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO card_runs
		   (id, run_type, trigger, status, sources_attempted, sources_succeeded,
		    categories_extracted, events_written, error_detail, started_at, finished_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())`,
		r.ID, r.RunType, r.Trigger, r.Status, r.SourcesAttempted, r.SourcesSucceeded,
		r.CategoriesExtracted, r.EventsWritten, r.ErrorDetail, r.StartedAt, r.FinishedAt,
	)
	return err
}

// InsertRunIfAbsent inserts a historical run only when no row with the same
// (run_type, started_at) natural key already exists, treating a NULL
// started_at as equal to a NULL started_at. Returns true when inserted. Used
// to idempotently import the mappable subset of CCManager run-history.json.
func (s *Store) InsertRunIfAbsent(ctx context.Context, r *CardRun) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO card_runs
		   (id, run_type, trigger, status, sources_attempted, sources_succeeded,
		    categories_extracted, events_written, error_detail, started_at, finished_at, created_at)
		 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM card_runs WHERE run_type = $2 AND started_at IS NOT DISTINCT FROM $10
		 )`,
		r.ID, r.RunType, r.Trigger, r.Status, r.SourcesAttempted, r.SourcesSucceeded,
		r.CategoriesExtracted, r.EventsWritten, r.ErrorDetail, r.StartedAt, r.FinishedAt,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CountRunsByType returns the number of card_runs rows with the given run_type
// (used by the importer's idempotency test to separate the always-appended
// migration audit row from the idempotent historical-run import).
func (s *Store) CountRunsByType(ctx context.Context, runType string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM card_runs WHERE run_type = $1`, runType).Scan(&n)
	return n, err
}
