package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/cardrewards"
)

// CardRewardsHandler serves the card-rewards CRUD API (spec 083 Scope 02):
// wallet (user cards), offers, selections, signup bonuses, card-name
// resolution, and category aliases. It is mounted inside the authenticated
// /api group (bearer auth, spec 044/070). JSON field names are snake_case to
// match the rest of the smackerel API surface.
type CardRewardsHandler struct {
	Service *cardrewards.Service
}

// NewCardRewardsHandler creates a card-rewards API handler.
func NewCardRewardsHandler(svc *cardrewards.Service) *CardRewardsHandler {
	return &CardRewardsHandler{Service: svc}
}

// RegisterRoutes registers card-rewards routes on the given Chi router. The
// prefix is relative; the production router mounts this handler inside the
// outer r.Route("/api", ...) authenticated group (see internal/api/router.go),
// so an absolute "/api/cards" prefix here would double the prefix (BUG-034-003).
func (h *CardRewardsHandler) RegisterRoutes(r chi.Router) {
	r.Route("/cards", func(r chi.Router) {
		r.Post("/", h.CreateCard)
		r.Get("/", h.ListCards)
		r.Post("/resolve", h.ResolveCard)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetCard)
			r.Put("/", h.UpdateCard)
			r.Delete("/", h.DeleteCard)
			r.Post("/offers", h.CreateOffer)
			r.Get("/offers", h.ListOffers)
			r.Post("/selections", h.CreateSelection)
			r.Get("/selections", h.ListSelections)
			r.Post("/bonuses", h.CreateBonus)
			r.Get("/bonuses", h.ListBonuses)
		})
	})
	r.Get("/card-offers/shared/{group}", h.ListOffersBySharedLimitGroup)
	r.Get("/card-category-aliases", h.ListCategoryAliases)
	r.Post("/card-category-aliases", h.CreateCategoryAlias)
	r.Route("/card-recommendations", func(r chi.Router) {
		r.Get("/", h.ListRecommendations)
		r.Post("/generate", h.GenerateRecommendations)
	})
	r.Get("/card-optimization-report", h.OptimizationReport)
	// Spec 083 Scope 11 — rotating-category read + observation/reconcile seed
	// surface (the rotating-verify web page's backing data + the e2e-ui seam
	// that produces a needs_verification record from disagreeing observations).
	r.Route("/card-rotating", func(r chi.Router) {
		r.Get("/", h.ListRotatingCategories)
		r.Post("/observations", h.CreateRotatingObservation)
		r.Post("/reconcile", h.ReconcileRotating)
	})
}

// ---- request DTOs (snake_case) ---------------------------------------------

type customCardInput struct {
	Name           string  `json:"name"`
	Issuer         string  `json:"issuer"`
	CardType       string  `json:"card_type"`
	AnnualFeeCents int     `json:"annual_fee_cents"`
	Nickname       *string `json:"nickname"`
	Note           *string `json:"note"`
}

type createCardRequest struct {
	CatalogID string           `json:"catalog_id"`
	Nickname  *string          `json:"nickname"`
	Note      *string          `json:"note"`
	Custom    *customCardInput `json:"custom"`
}

type updateCardRequest struct {
	Nickname *string `json:"nickname"`
	Note     *string `json:"note"`
	Active   *bool   `json:"active"`
}

type resolveCardRequest struct {
	Text string `json:"text"`
}

type createOfferRequest struct {
	Title              string  `json:"title"`
	Category           string  `json:"category"`
	Rate               float64 `json:"rate"`
	RateType           string  `json:"rate_type"`
	LimitCents         *int    `json:"limit_cents"`
	LimitPeriod        *string `json:"limit_period"`
	SharedLimitGroup   *string `json:"shared_limit_group"`
	StartsOn           string  `json:"starts_on"`
	EndsOn             string  `json:"ends_on"`
	ActivationRequired bool    `json:"activation_required"`
	Activated          bool    `json:"activated"`
	Notes              *string `json:"notes"`
}

type createSelectionRequest struct {
	Category       string `json:"category"`
	Tier           *int   `json:"tier"`
	PeriodLabel    string `json:"period_label"`
	Enrolled       bool   `json:"enrolled"`
	EffectiveStart string `json:"effective_start"`
	EffectiveEnd   string `json:"effective_end"`
}

type createBonusRequest struct {
	BonusType          string  `json:"bonus_type"`
	Description        string  `json:"description"`
	SpendRequiredCents *int    `json:"spend_required_cents"`
	SpendProgressCents int     `json:"spend_progress_cents"`
	RewardDescription  *string `json:"reward_description"`
	Deadline           string  `json:"deadline"`
	Met                bool    `json:"met"`
}

type createCategoryAliasRequest struct {
	CanonicalCategory string   `json:"canonical_category"`
	Equivalents       []string `json:"equivalents"`
	Starred           bool     `json:"starred"`
	Priority          *int     `json:"priority"`
	BuiltIn           bool     `json:"built_in"`
}

type generateRecommendationsRequest struct {
	Period string `json:"period"`
}

// ---- handlers --------------------------------------------------------------

// CreateCard creates a wallet entry from either a catalog id or a custom card.
func (h *CardRewardsHandler) CreateCard(w http.ResponseWriter, r *http.Request) {
	var req createCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}

	var (
		card *cardrewards.UserCard
		err  error
	)
	switch {
	case req.Custom != nil:
		card, err = h.Service.CreateCustomCard(r.Context(), cardrewards.CustomCardInput{
			Name:           req.Custom.Name,
			Issuer:         req.Custom.Issuer,
			CardType:       req.Custom.CardType,
			AnnualFeeCents: req.Custom.AnnualFeeCents,
			Nickname:       req.Custom.Nickname,
			Note:           req.Custom.Note,
		})
	case strings.TrimSpace(req.CatalogID) != "":
		card, err = h.Service.CreateUserCard(r.Context(), req.CatalogID, req.Nickname, req.Note)
	default:
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "either catalog_id or custom is required")
		return
	}
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, card)
}

// ListCards returns wallet entries. ?active=true filters to active cards.
func (h *CardRewardsHandler) ListCards(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"
	cards, err := h.Service.ListUserCards(r.Context(), activeOnly)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if cards == nil {
		cards = []cardrewards.UserCard{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": cards})
}

// GetCard returns a single wallet entry.
func (h *CardRewardsHandler) GetCard(w http.ResponseWriter, r *http.Request) {
	card, err := h.Service.GetUserCard(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// UpdateCard edits a wallet entry's nickname/note/active.
func (h *CardRewardsHandler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	var req updateCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	card, err := h.Service.UpdateUserCard(r.Context(), chi.URLParam(r, "id"), req.Nickname, req.Note, req.Active)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// DeleteCard removes a wallet entry (cascading to its offers/selections/bonuses).
func (h *CardRewardsHandler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.DeleteUserCard(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ResolveCard returns ranked catalog candidates for free-text input.
func (h *CardRewardsHandler) ResolveCard(w http.ResponseWriter, r *http.Request) {
	var req resolveCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "text is required")
		return
	}
	candidates, err := h.Service.ResolveCard(r.Context(), req.Text)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if candidates == nil {
		candidates = []cardrewards.Candidate{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

// CreateOffer creates an offer for a wallet entry.
func (h *CardRewardsHandler) CreateOffer(w http.ResponseWriter, r *http.Request) {
	var req createOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	startsOn, err := parseOptionalDate(req.StartsOn)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "starts_on must be YYYY-MM-DD")
		return
	}
	endsOn, err := parseOptionalDate(req.EndsOn)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "ends_on must be YYYY-MM-DD")
		return
	}
	userCardID := chi.URLParam(r, "id")
	offer, err := h.Service.CreateOffer(r.Context(), cardrewards.Offer{
		UserCardID:         &userCardID,
		Title:              req.Title,
		Category:           req.Category,
		Rate:               req.Rate,
		RateType:           req.RateType,
		LimitCents:         req.LimitCents,
		LimitPeriod:        req.LimitPeriod,
		SharedLimitGroup:   req.SharedLimitGroup,
		StartsOn:           startsOn,
		EndsOn:             endsOn,
		ActivationRequired: req.ActivationRequired,
		Activated:          req.Activated,
		Notes:              req.Notes,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, offer)
}

// ListOffers returns the offers for a wallet entry.
func (h *CardRewardsHandler) ListOffers(w http.ResponseWriter, r *http.Request) {
	offers, err := h.Service.ListOffersByUserCard(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if offers == nil {
		offers = []cardrewards.Offer{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"offers": offers})
}

// ListOffersBySharedLimitGroup returns offers sharing a combined-limit pool.
func (h *CardRewardsHandler) ListOffersBySharedLimitGroup(w http.ResponseWriter, r *http.Request) {
	offers, err := h.Service.ListOffersBySharedLimitGroup(r.Context(), chi.URLParam(r, "group"))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if offers == nil {
		offers = []cardrewards.Offer{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"offers": offers})
}

// CreateSelection creates a selectable-category choice for a wallet entry.
func (h *CardRewardsHandler) CreateSelection(w http.ResponseWriter, r *http.Request) {
	var req createSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	effStart, err := parseOptionalDate(req.EffectiveStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "effective_start must be YYYY-MM-DD")
		return
	}
	effEnd, err := parseOptionalDate(req.EffectiveEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "effective_end must be YYYY-MM-DD")
		return
	}
	sel, err := h.Service.CreateSelection(r.Context(), cardrewards.Selection{
		UserCardID:     chi.URLParam(r, "id"),
		Category:       req.Category,
		Tier:           req.Tier,
		PeriodLabel:    req.PeriodLabel,
		Enrolled:       req.Enrolled,
		EffectiveStart: effStart,
		EffectiveEnd:   effEnd,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sel)
}

// ListSelections returns the selections for a wallet entry.
func (h *CardRewardsHandler) ListSelections(w http.ResponseWriter, r *http.Request) {
	sels, err := h.Service.ListSelectionsByUserCard(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if sels == nil {
		sels = []cardrewards.Selection{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"selections": sels})
}

// CreateBonus creates a signup-bonus tracker for a wallet entry.
func (h *CardRewardsHandler) CreateBonus(w http.ResponseWriter, r *http.Request) {
	var req createBonusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	deadline, err := parseOptionalDate(req.Deadline)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "deadline must be YYYY-MM-DD")
		return
	}
	bonus, err := h.Service.CreateSignupBonus(r.Context(), cardrewards.SignupBonus{
		UserCardID:         chi.URLParam(r, "id"),
		BonusType:          req.BonusType,
		Description:        req.Description,
		SpendRequiredCents: req.SpendRequiredCents,
		SpendProgressCents: req.SpendProgressCents,
		RewardDescription:  req.RewardDescription,
		Deadline:           deadline,
		Met:                req.Met,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, bonus)
}

// ListBonuses returns the signup bonuses for a wallet entry.
func (h *CardRewardsHandler) ListBonuses(w http.ResponseWriter, r *http.Request) {
	bonuses, err := h.Service.ListBonusesByUserCard(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if bonuses == nil {
		bonuses = []cardrewards.SignupBonus{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"bonuses": bonuses})
}

// ListCategoryAliases returns the configured category aliases.
func (h *CardRewardsHandler) ListCategoryAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := h.Service.ListCategoryAliases(r.Context())
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if aliases == nil {
		aliases = []cardrewards.CategoryAlias{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"category_aliases": aliases})
}

// CreateCategoryAlias upserts a tracked spend category (and its equivalents).
func (h *CardRewardsHandler) CreateCategoryAlias(w http.ResponseWriter, r *http.Request) {
	var req createCategoryAliasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	alias, err := h.Service.CreateCategoryAlias(r.Context(), cardrewards.CategoryAlias{
		CanonicalCategory: req.CanonicalCategory,
		Equivalents:       req.Equivalents,
		Starred:           req.Starred,
		Priority:          req.Priority,
		BuiltIn:           req.BuiltIn,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, alias)
}

// GenerateRecommendations runs the optimizer across the tracked categories and
// writes one recommendation per (period, category), preserving starred
// overrides (SCN-083-G06, G07). An empty period means the current month.
func (h *CardRewardsHandler) GenerateRecommendations(w http.ResponseWriter, r *http.Request) {
	var req generateRecommendationsRequest
	// An empty body is valid (generate for the current period).
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
			return
		}
	}
	report, err := h.Service.GenerateRecommendations(r.Context(), strings.TrimSpace(req.Period), "manual")
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ListRecommendations returns the persisted recommendations for a period
// (?period=YYYY-MM, default current month) (SCN-083-G08).
func (h *CardRewardsHandler) ListRecommendations(w http.ResponseWriter, r *http.Request) {
	period, recs, err := h.Service.ListRecommendations(r.Context(), strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if recs == nil {
		recs = []cardrewards.CardRecommendation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"period": period, "recommendations": recs})
}

// OptimizationReport returns the read-only optimizer breakdown for a period
// (?period=YYYY-MM, default current month) (SCN-083-G08).
func (h *CardRewardsHandler) OptimizationReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.OptimizationReport(r.Context(), strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ---- rotating categories (spec 083 Scope 11) -------------------------------

type createObservationRequest struct {
	CardCatalogID string   `json:"card_catalog_id"`
	PeriodLabel   string   `json:"period_label"`
	PeriodStart   string   `json:"period_start"`
	PeriodEnd     string   `json:"period_end"`
	Categories    []string `json:"categories"`
	Confidence    float64  `json:"confidence"`
	SourceName    string   `json:"source_name"`
	SourceURL     string   `json:"source_url"`
	LimitCents    *int     `json:"limit_cents"`
}

type reconcileRequest struct {
	Threshold float64 `json:"threshold"`
	Trigger   string  `json:"trigger"`
}

// ListRotatingCategories returns every reconciled rotating-category record
// (all lifecycle states). Read surface behind the same /api auth as the rest
// of the card-rewards API.
func (h *CardRewardsHandler) ListRotatingCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.Service.ListRotatingCategories(r.Context())
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	if cats == nil {
		cats = []cardrewards.RotatingCategory{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rotating_categories": cats})
}

// CreateRotatingObservation persists a single per-source rotating-category
// observation under a fresh extract audit run. It is the seam the rotating-
// verify e2e-ui test uses to seed the observations the reconciler merges.
func (h *CardRewardsHandler) CreateRotatingObservation(w http.ResponseWriter, r *http.Request) {
	var req createObservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	start, err := parseOptionalDate(req.PeriodStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid period_start (want YYYY-MM-DD)")
		return
	}
	end, err := parseOptionalDate(req.PeriodEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid period_end (want YYYY-MM-DD)")
		return
	}
	obs, err := h.Service.CreateObservation(r.Context(), cardrewards.RotatingCategoryObservation{
		CardCatalogID: req.CardCatalogID,
		PeriodLabel:   req.PeriodLabel,
		PeriodStart:   start,
		PeriodEnd:     end,
		Categories:    req.Categories,
		Confidence:    req.Confidence,
		SourceName:    req.SourceName,
		SourceURL:     req.SourceURL,
		LimitCents:    req.LimitCents,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, obs)
}

// ReconcileRotating merges every stored observation into its authoritative
// rotating-category record via the Scope 06 reconciler. The confidence
// threshold is a REQUIRED request field (no hidden default).
func (h *CardRewardsHandler) ReconcileRotating(w http.ResponseWriter, r *http.Request) {
	var req reconcileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", "invalid JSON body")
		return
	}
	res, err := h.Service.Reconcile(r.Context(), req.Threshold, req.Trigger)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleServiceError maps card-rewards service sentinel errors to HTTP status.
func (h *CardRewardsHandler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cardrewards.ErrValidation):
		writeError(w, http.StatusBadRequest, "CARD_REWARDS_VALIDATION", err.Error())
	case errors.Is(err, cardrewards.ErrUserCardNotFound):
		writeError(w, http.StatusNotFound, "CARD_NOT_FOUND", err.Error())
	case errors.Is(err, cardrewards.ErrRotatingNotFound):
		writeError(w, http.StatusNotFound, "ROTATING_NOT_FOUND", err.Error())
	case errors.Is(err, cardrewards.ErrRecommendationNotFound):
		writeError(w, http.StatusNotFound, "RECOMMENDATION_NOT_FOUND", err.Error())
	case errors.Is(err, cardrewards.ErrCatalogNotFound):
		writeError(w, http.StatusUnprocessableEntity, "CATALOG_NOT_FOUND", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "CARD_REWARDS_INTERNAL", "internal error")
	}
}

// parseOptionalDate parses an optional YYYY-MM-DD date; "" → nil.
func parseOptionalDate(s string) (*time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
