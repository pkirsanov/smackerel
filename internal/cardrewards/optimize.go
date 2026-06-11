package cardrewards

// Card-rewards optimizer (spec 083 Scope 07, design §6 / FR-CR-013, Principle 8
// "explainability"). Replaces CCManager's optimizer.py.
//
// For a spend category and the current moment, the optimizer computes the best
// owned, active card by effective rate, taking the maximum over four benefit
// sources:
//
//   - base_benefit       — the card's fixed category (or catch-all) rate (G01)
//   - active rotating     — a lifecycle-active rotating_categories match, rated
//                           from the card's rotating_benefits (G02); EXPIRED and
//                           UPCOMING records are ignored (G03)
//   - active offer        — a date-windowed (and, if required, activated) offer
//                           match, with shared_limit_group pools treated as ONE
//                           combined pool, never double-counted (G04)
//   - active selection    — an enrolled selectable-category choice rated from
//                           the card's selectable_benefits
//
// Spend categories are normalized through category_aliases equivalents BEFORE
// matching, so "eating out" matches "Dining" (G05). Every pick records a
// human-readable reason (Principle 8). Ties are broken deterministically:
// no spend-limit beats a limited benefit, then a higher limit, then the
// lexicographically smaller user_card_id (a stable, reproducible final key —
// the design's "issuer preference" is not configured, so the optimizer never
// guesses one).
//
// The core Optimize function is PURE — a function of its inputs only — so every
// scenario SCN-083-G01..G05 is unit-testable with no database and no mocks
// (T-07-01/T-07-02). recommend.go wires it to a real Store for the per-period
// generation paths (T-07-03) and the REST surface (T-07-04).

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Effective-rate benefit-source labels, recorded on every recommendation for
// explainability (Principle 8). They mirror CCManager's recommendation "source"
// field so the imported history and freshly generated rows speak one vocabulary.
const (
	BenefitSourceBase      = "base"
	BenefitSourceRotating  = "rotating"
	BenefitSourceOffer     = "offer"
	BenefitSourceSelection = "selection"
	BenefitSourceNone      = "none"
)

// CardInputs is the snapshot of one owned wallet card the optimizer reasons
// over. Rotating carries ALL of the card's reconciled rotating-category records
// (every lifecycle state); the optimizer itself filters to the active ones, so
// an expired record fed in is provably ignored (SCN-083-G03 adversarial).
type CardInputs struct {
	UserCard   UserCard
	Catalog    *CatalogCard
	Offers     []Offer
	Selections []Selection
	Rotating   []RotatingCategory
}

// OptimizationResult is the optimizer's verdict for one spend category in one
// period. Category is the canonical (alias-normalized) name. RecommendedUserCardID
// is nil and Source is BenefitSourceNone when no owned card has any benefit for
// the category.
type OptimizationResult struct {
	Category              string  `json:"category"`
	RecommendedUserCardID *string `json:"recommended_user_card_id,omitempty"`
	CardName              string  `json:"card_name"`
	Rate                  float64 `json:"rate"`
	RateType              string  `json:"rate_type"`
	Source                string  `json:"source"`
	Reason                string  `json:"reason"`
	NeedsActivation       bool    `json:"needs_activation"`
	SharedLimitGroup      *string `json:"shared_limit_group,omitempty"`
	EffectiveLimitCents   *int    `json:"effective_limit_cents,omitempty"`
}

// ---- benefit JSON shapes ---------------------------------------------------
//
// The card_catalog jsonb columns are stored verbatim from the CCManager import
// (design §11 / import.go: benefits pass through unchanged), so the structures
// below tolerate both the canonical smackerel "limit_cents" form (design §2.1)
// and the CCManager source "limit" (whole dollars) form. A whole-dollar limit
// is converted to integer cents via dollarsToCents (import.go), keeping money
// math in cents everywhere.

type baseBenefit struct {
	Category   string   `json:"category"`
	Rate       float64  `json:"rate"`
	RateType   string   `json:"rate_type"`
	LimitCents *int     `json:"limit_cents"`
	Limit      *float64 `json:"limit"`
}

type rotatingBenefit struct {
	Type               string   `json:"type"`
	ActivationRequired bool     `json:"activation_required"`
	Rate               *float64 `json:"rate"`
	RateType           string   `json:"rate_type"`
	LimitCents         *int     `json:"limit_cents"`
	Limit              *float64 `json:"limit"`
}

type selectableTier struct {
	Name      string  `json:"name"`
	TierIndex int     `json:"tier_index"`
	Rate      float64 `json:"rate"`
	RateType  string  `json:"rate_type"`
}

type selectableBenefit struct {
	Rate       *float64         `json:"rate"`
	RateType   string           `json:"rate_type"`
	LimitCents *int             `json:"limit_cents"`
	Limit      *float64         `json:"limit"`
	Tiers      []selectableTier `json:"tiers"`
}

// limitToCents resolves the optional limit of a benefit, preferring the
// canonical limit_cents and falling back to the CCManager whole-dollar limit.
// (This is tolerant JSON shape handling, NOT a config default.)
func limitToCents(cents *int, dollars *float64) *int {
	if cents != nil {
		return cents
	}
	if dollars != nil {
		c := dollarsToCents(*dollars)
		return &c
	}
	return nil
}

// catchAllBaseCategories are the base-benefit category labels that apply to
// every spend category (the "all other purchases" floor). Compared lowercased.
var catchAllBaseCategories = map[string]bool{
	"everything":      true,
	"everything else": true,
	"all":             true,
	"all purchases":   true,
	"other":           true,
}

// ---- category canonicalization (G05) ---------------------------------------

// categoryCanonicalizer normalizes spend-category names to their canonical form
// using category_aliases equivalents. Matching is case-insensitive. A name with
// no alias entry canonicalizes to its trimmed self (so unknown categories still
// match identically-spelled benefits).
type categoryCanonicalizer struct {
	toCanonical map[string]string
}

func newCanonicalizer(aliases []CategoryAlias) *categoryCanonicalizer {
	m := make(map[string]string, len(aliases)*3)
	for _, a := range aliases {
		canon := strings.TrimSpace(a.CanonicalCategory)
		if canon == "" {
			continue
		}
		m[strings.ToLower(canon)] = canon
		for _, eq := range a.Equivalents {
			eq = strings.TrimSpace(eq)
			if eq == "" {
				continue
			}
			m[strings.ToLower(eq)] = canon
		}
	}
	return &categoryCanonicalizer{toCanonical: m}
}

// canonical returns the canonical form of cat, or cat trimmed when unknown.
func (c *categoryCanonicalizer) canonical(cat string) string {
	if canon, ok := c.toCanonical[strings.ToLower(strings.TrimSpace(cat))]; ok {
		return canon
	}
	return strings.TrimSpace(cat)
}

// matches reports whether two category names share a canonical form.
func (c *categoryCanonicalizer) matches(a, b string) bool {
	return strings.EqualFold(c.canonical(a), c.canonical(b))
}

// ---- candidate model -------------------------------------------------------

// benefitCandidate is one (rate, source) option for a single card. specificity
// distinguishes an exact category match (1) from a catch-all base floor (0) so
// an exact 1% beats a catch-all 1% deterministically.
type benefitCandidate struct {
	rate            float64
	rateType        string
	source          string
	reason          string
	needsActivation bool
	sharedGroup     *string
	limitCents      *int
	specificity     int
}

// dateOf truncates a timestamp to its UTC calendar date for inclusive
// date-window comparisons (an offer ending 2026-06-30 is active all day).
func dateOf(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// offerActive reports whether an offer applies at now: not requiring an
// un-done activation, and inside its (inclusive, date-granular) window.
func offerActive(o Offer, now time.Time) bool {
	if o.ActivationRequired && !o.Activated {
		return false
	}
	nd := dateOf(now)
	if o.StartsOn != nil && nd.Before(dateOf(*o.StartsOn)) {
		return false
	}
	if o.EndsOn != nil && nd.After(dateOf(*o.EndsOn)) {
		return false
	}
	return true
}

// selectionActive reports whether an enrolled selection applies at now.
func selectionActive(sel Selection, now time.Time) bool {
	if !sel.Enrolled {
		return false
	}
	nd := dateOf(now)
	if sel.EffectiveStart != nil && nd.Before(dateOf(*sel.EffectiveStart)) {
		return false
	}
	if sel.EffectiveEnd != nil && nd.After(dateOf(*sel.EffectiveEnd)) {
		return false
	}
	return true
}

// poolLimitCents returns the single combined limit of a shared_limit_group,
// counted ONCE across every offer that references the group (SCN-083-G04). The
// offers in a pool share one cap, so the pool limit is the maximum stated cap,
// never the sum — summing would double-count the shared pool.
func poolLimitCents(offers []Offer, group string) *int {
	var pool *int
	for i := range offers {
		o := offers[i]
		if o.SharedLimitGroup == nil || *o.SharedLimitGroup != group {
			continue
		}
		lc := o.LimitCents
		if lc == nil {
			continue
		}
		if pool == nil || *lc > *pool {
			v := *lc
			pool = &v
		}
	}
	return pool
}

// ---- per-card evaluation ---------------------------------------------------

// cardCatalogName resolves a display name for a wallet card.
func cardCatalogName(in CardInputs) string {
	if in.UserCard.CatalogName != "" {
		return in.UserCard.CatalogName
	}
	if in.Catalog != nil && in.Catalog.Name != "" {
		return in.Catalog.Name
	}
	if in.UserCard.Nickname != nil && *in.UserCard.Nickname != "" {
		return *in.UserCard.Nickname
	}
	return in.UserCard.ID
}

// evaluateCard returns the single best benefit candidate this card offers for
// the canonical query category, or ok=false when the card has no applicable
// benefit. PURE.
func evaluateCard(in CardInputs, query string, canon *categoryCanonicalizer, now time.Time) (benefitCandidate, bool) {
	var cands []benefitCandidate
	name := cardCatalogName(in)

	// --- base benefits (G01) ---
	if in.Catalog != nil {
		var bases []baseBenefit
		if len(in.Catalog.BaseBenefits) > 0 {
			_ = json.Unmarshal(in.Catalog.BaseBenefits, &bases)
		}
		for _, b := range bases {
			if b.RateType == "" {
				b.RateType = RateTypePercent
			}
			switch {
			case canon.matches(b.Category, query):
				cands = append(cands, benefitCandidate{
					rate: b.Rate, rateType: b.RateType, source: BenefitSourceBase,
					reason:      fmt.Sprintf("%s earns %s base on %s", name, formatRate(b.Rate, b.RateType), canon.canonical(query)),
					limitCents:  limitToCents(b.LimitCents, b.Limit),
					specificity: 1,
				})
			case catchAllBaseCategories[strings.ToLower(strings.TrimSpace(b.Category))]:
				cands = append(cands, benefitCandidate{
					rate: b.Rate, rateType: b.RateType, source: BenefitSourceBase,
					reason:      fmt.Sprintf("%s earns %s on all purchases", name, formatRate(b.Rate, b.RateType)),
					limitCents:  limitToCents(b.LimitCents, b.Limit),
					specificity: 0,
				})
			}
		}
	}

	// --- active rotating category (G02; expired/upcoming ignored — G03) ---
	if in.Catalog != nil {
		var rb rotatingBenefit
		hasRB := len(in.Catalog.RotatingBenefits) > 0 && json.Unmarshal(in.Catalog.RotatingBenefits, &rb) == nil
		if hasRB && rb.Rate != nil {
			if rb.RateType == "" {
				rb.RateType = RateTypePercent
			}
			for _, rc := range in.Rotating {
				if rc.LifecycleState != LifecycleActive {
					continue // G03: expired/upcoming benefits are not used
				}
				if !rotatingCovers(rc, query, canon) {
					continue
				}
				lim := rc.LimitCents
				if lim == nil {
					lim = limitToCents(rb.LimitCents, rb.Limit)
				}
				cands = append(cands, benefitCandidate{
					rate: *rb.Rate, rateType: rb.RateType, source: BenefitSourceRotating,
					reason: fmt.Sprintf("%s has an active rotating %s category for %s (%s)",
						name, formatRate(*rb.Rate, rb.RateType), canon.canonical(query), rc.PeriodLabel),
					needsActivation: rb.ActivationRequired || rc.ActivationRequired,
					limitCents:      lim,
					specificity:     1,
				})
			}
		}
	}

	// --- active selection (selectable-category choice) ---
	if in.Catalog != nil && len(in.Selections) > 0 {
		var sb selectableBenefit
		hasSB := len(in.Catalog.SelectableBenefits) > 0 && json.Unmarshal(in.Catalog.SelectableBenefits, &sb) == nil
		if hasSB {
			for _, sel := range in.Selections {
				if !selectionActive(sel, now) || !canon.matches(sel.Category, query) {
					continue
				}
				rate, rateType, ok := selectionRate(sb, sel)
				if !ok {
					continue
				}
				cands = append(cands, benefitCandidate{
					rate: rate, rateType: rateType, source: BenefitSourceSelection,
					reason:      fmt.Sprintf("%s has %s selected on %s", name, formatRate(rate, rateType), canon.canonical(query)),
					limitCents:  limitToCents(sb.LimitCents, sb.Limit),
					specificity: 1,
				})
			}
		}
	}

	// --- active offers (G04 shared-limit pool) ---
	for i := range in.Offers {
		o := in.Offers[i]
		if !offerActive(o, now) || !canon.matches(o.Category, query) {
			continue
		}
		lim := o.LimitCents
		if o.SharedLimitGroup != nil {
			lim = poolLimitCents(in.Offers, *o.SharedLimitGroup)
		}
		cands = append(cands, benefitCandidate{
			rate: o.Rate, rateType: o.RateType, source: BenefitSourceOffer,
			reason:          fmt.Sprintf("%s has an active offer: %s on %s (%s)", name, formatRate(o.Rate, o.RateType), canon.canonical(query), o.Title),
			needsActivation: o.ActivationRequired && !o.Activated,
			sharedGroup:     o.SharedLimitGroup,
			limitCents:      lim,
			specificity:     1,
		})
	}

	if len(cands) == 0 {
		return benefitCandidate{}, false
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].rate != cands[j].rate {
			return cands[i].rate > cands[j].rate
		}
		return cands[i].specificity > cands[j].specificity
	})
	return cands[0], true
}

// rotatingCovers reports whether a rotating record lists the query category
// (canonicalized) among its categories.
func rotatingCovers(rc RotatingCategory, query string, canon *categoryCanonicalizer) bool {
	for _, c := range rc.Categories {
		if canon.matches(c, query) {
			return true
		}
	}
	return false
}

// selectionRate resolves the reward rate for an enrolled selection: a tiered
// card's tier-specific rate (matched by the selection's Tier), otherwise the
// card's flat selectable rate. Returns ok=false when neither is defined.
func selectionRate(sb selectableBenefit, sel Selection) (float64, string, bool) {
	if sel.Tier != nil && len(sb.Tiers) > 0 {
		for _, t := range sb.Tiers {
			if t.TierIndex == *sel.Tier {
				rt := t.RateType
				if rt == "" {
					rt = RateTypePercent
				}
				return t.Rate, rt, true
			}
		}
	}
	if sb.Rate != nil {
		rt := sb.RateType
		if rt == "" {
			rt = RateTypePercent
		}
		return *sb.Rate, rt, true
	}
	return 0, "", false
}

// formatRate renders a rate for a human-readable reason ("5%", "3x points",
// "2 points").
func formatRate(rate float64, rateType string) string {
	switch rateType {
	case RateTypePercent:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.2f", rate), "0"), "0") + "%"
	case RateTypeMultiplier:
		return fmt.Sprintf("%gx", rate)
	case RateTypePoints:
		return fmt.Sprintf("%g points", rate)
	default:
		return fmt.Sprintf("%g %s", rate, rateType)
	}
}

// ---- top-level optimizer ---------------------------------------------------

// Optimize computes the best owned card for a spend category at the given
// moment. The query is normalized through category_aliases equivalents before
// matching (G05). The result always carries a reason (Principle 8); when no
// card has a benefit it reports BenefitSourceNone with a zero rate and a nil
// recommended_user_card_id. PURE — no I/O.
func Optimize(query string, cards []CardInputs, aliases []CategoryAlias, now time.Time) OptimizationResult {
	canon := newCanonicalizer(aliases)
	canonical := canon.canonical(query)

	type ranked struct {
		in   CardInputs
		cand benefitCandidate
	}
	var picks []ranked
	for _, in := range cards {
		if !in.UserCard.Active {
			continue
		}
		if cand, ok := evaluateCard(in, query, canon, now); ok {
			picks = append(picks, ranked{in: in, cand: cand})
		}
	}

	if len(picks) == 0 {
		return OptimizationResult{
			Category: canonical,
			Rate:     0,
			Source:   BenefitSourceNone,
			Reason:   fmt.Sprintf("No owned card has a benefit for %s", canonical),
		}
	}

	sort.SliceStable(picks, func(i, j int) bool {
		a, b := picks[i], picks[j]
		if a.cand.rate != b.cand.rate {
			return a.cand.rate > b.cand.rate // higher effective rate wins
		}
		// Tie-break 1: a benefit with no spend limit beats a capped one.
		aNoLimit, bNoLimit := a.cand.limitCents == nil, b.cand.limitCents == nil
		if aNoLimit != bNoLimit {
			return aNoLimit
		}
		// Tie-break 2: higher cap beats a lower cap.
		if !aNoLimit && !bNoLimit && *a.cand.limitCents != *b.cand.limitCents {
			return *a.cand.limitCents > *b.cand.limitCents
		}
		// Tie-break 3: stable, reproducible final key (no guessed issuer order).
		return a.in.UserCard.ID < b.in.UserCard.ID
	})

	best := picks[0]
	id := best.in.UserCard.ID
	return OptimizationResult{
		Category:              canonical,
		RecommendedUserCardID: &id,
		CardName:              cardCatalogName(best.in),
		Rate:                  best.cand.rate,
		RateType:              best.cand.rateType,
		Source:                best.cand.source,
		Reason:                best.cand.reason,
		NeedsActivation:       best.cand.needsActivation,
		SharedLimitGroup:      best.cand.sharedGroup,
		EffectiveLimitCents:   best.cand.limitCents,
	}
}
