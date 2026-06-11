// Spec 083 Scope 10 — server-rendered card-rewards Web UI.
//
// CardRewardsWebHandler serves the wallet, offers, selections, bonuses, and
// categories pages with full CRUD parity to the standalone CCManager app
// (FR-CR-016). It follows the existing internal/web paradigm: Go html/template
// + go-chi, mounted behind the same bearer/session auth (webAuthMiddleware) and
// global CSP as the rest of the web UI (NFR-CR-006). All mutations use the
// Post/Redirect/Get pattern so a reload re-renders the persisted state.
//
// CSP discipline: the global securityHeadersMiddleware ships a strict CSP whose
// script-src allows only 'self', the pinned htmx unpkg bundle, and one hashed
// inline theme script (in the shared "head" template). These pages therefore
// add NO new inline <script> and NO inline event handlers (onclick/onchange) —
// interactivity is plain <form> submits. Styling uses the shared design-token
// palette (var(--…)); no hardcoded colors.
//
// Routes are registered via RegisterRoutes and wired in internal/api/router.go
// through the CardRewardsWebUI interface (see health.go), mirroring the
// AgentAdminUI precedent.

package web

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/cardrewards"
)

// CardRewardsWebHandler renders the spec 083 card-rewards pages.
type CardRewardsWebHandler struct {
	Service   *cardrewards.Service
	Templates *template.Template
}

// NewCardRewardsWebHandler builds the handler with a self-contained template
// set (cardRewardsTemplates) that defines its own script-free "head"/"foot"
// chrome using the shared design-token palette (var(--…)). It deliberately does
// NOT reuse the shared allTemplates "head", because that head loads the htmx
// bundle from a URL the global CSP does not allow-list (script-src lists
// ".../htmx.org@1.9.12/" with a trailing slash; the bundle URL has none), which
// the e2e-ui CSP guard flags as a violation. The card-rewards pages use plain
// Post/Redirect/Get forms and need no client JS, so a script-free head keeps
// them strictly CSP-clean. (The shared-head/CSP mismatch is a pre-existing,
// out-of-scope inconsistency noted in the spec 083 Scope 10 report.)
func NewCardRewardsWebHandler(svc *cardrewards.Service) *CardRewardsWebHandler {
	fm := template.FuncMap{
		// Card-rewards display helpers.
		"cents": func(c int) string { return fmt.Sprintf("$%.2f", float64(c)/100) },
		"centsPtr": func(c *int) string {
			if c == nil {
				return "—"
			}
			return fmt.Sprintf("$%.2f", float64(*c)/100)
		},
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"intPtr": func(i *int) string {
			if i == nil {
				return ""
			}
			return strconv.Itoa(*i)
		},
		"csv": func(ss []string) string { return strings.Join(ss, ", ") },
		"date": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"pct": func(progress int, required *int) int {
			if required == nil || *required <= 0 {
				return 0
			}
			p := progress * 100 / *required
			if p > 100 {
				return 100
			}
			if p < 0 {
				return 0
			}
			return p
		},
	}
	t := template.Must(template.New("cardrewards").Funcs(fm).Parse(cardRewardsTemplates))
	return &CardRewardsWebHandler{Service: svc, Templates: t}
}

// RegisterRoutes registers the card-rewards web routes on r. The caller mounts
// this inside the webAuthMiddleware group (see internal/api/router.go) so the
// pages share the same auth + CSP posture as the rest of the web UI.
func (h *CardRewardsWebHandler) RegisterRoutes(r chi.Router) {
	r.Route("/cards/wallet", func(r chi.Router) {
		r.Get("/", h.WalletPage)
		r.Get("/add", h.WalletAddPage)
		r.Post("/", h.WalletAdd)
		r.Get("/add-custom", h.WalletAddCustomPage)
		r.Post("/custom", h.WalletAddCustom)
		r.Get("/{id}/edit", h.WalletEditPage)
		r.Post("/{id}", h.WalletUpdate)
		r.Post("/{id}/toggle", h.WalletToggle)
		r.Post("/{id}/delete", h.WalletDelete)
	})
	r.Route("/cards/offers", func(r chi.Router) {
		r.Get("/", h.OffersPage)
		r.Post("/", h.OfferCreate)
		r.Get("/{id}/edit", h.OfferEditPage)
		r.Post("/{id}", h.OfferUpdate)
		r.Post("/{id}/toggle", h.OfferToggle)
		r.Post("/{id}/delete", h.OfferDelete)
	})
	r.Route("/cards/selections", func(r chi.Router) {
		r.Get("/", h.SelectionsPage)
		r.Post("/", h.SelectionCreate)
		r.Get("/{id}/edit", h.SelectionEditPage)
		r.Post("/{id}", h.SelectionUpdate)
	})
	r.Route("/cards/bonuses", func(r chi.Router) {
		r.Get("/", h.BonusesPage)
		r.Post("/", h.BonusCreate)
		r.Post("/{id}/progress", h.BonusProgress)
	})
	r.Route("/cards/categories", func(r chi.Router) {
		r.Get("/", h.CategoriesPage)
		r.Post("/", h.CategoryUpsert)
	})
}

// ---- view models -----------------------------------------------------------

type cardOption struct {
	ID   string
	Name string
}

type walletCardRow struct {
	cardrewards.UserCard
	CardType string
}

type offerRow struct {
	cardrewards.Offer
	CardName string
}

type selectionRow struct {
	cardrewards.Selection
	CardName string
}

type bonusRow struct {
	cardrewards.SignupBonus
	CardName string
}

// ---- wallet -----------------------------------------------------------------

// WalletPage handles GET /cards/wallet (SCN-083-J01): list owned cards with
// nickname, type, note, and active state.
func (h *CardRewardsWebHandler) WalletPage(w http.ResponseWriter, r *http.Request) {
	cards, err := h.Service.ListUserCards(r.Context(), false)
	if err != nil {
		h.fail(w, "load wallet", err)
		return
	}
	catalog, err := h.Service.ListCatalog(r.Context())
	if err != nil {
		h.fail(w, "load catalog", err)
		return
	}
	typeByID := make(map[string]string, len(catalog))
	for _, c := range catalog {
		typeByID[c.ID] = c.CardType
	}
	rows := make([]walletCardRow, 0, len(cards))
	for _, c := range cards {
		rows = append(rows, walletCardRow{UserCard: c, CardType: typeByID[c.CardCatalogID]})
	}
	h.render(w, "cardrewards-wallet.html", map[string]any{"Title": "My Cards", "Cards": rows})
}

// WalletAddPage handles GET /cards/wallet/add (SCN-083-J02): catalog discovery
// search. With ?q=… it renders ranked candidates, each with a confirm form.
func (h *CardRewardsWebHandler) WalletAddPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var candidates []cardrewards.Candidate
	if q != "" {
		var err error
		candidates, err = h.Service.ResolveCard(r.Context(), q)
		if err != nil {
			h.fail(w, "resolve card", err)
			return
		}
	}
	h.render(w, "cardrewards-wallet-add.html", map[string]any{
		"Title": "Add Card", "Query": q, "Candidates": candidates,
	})
}

// WalletAdd handles POST /cards/wallet (SCN-083-J02 confirm): add a catalog card
// to the wallet by catalog_id.
func (h *CardRewardsWebHandler) WalletAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	catalogID := strings.TrimSpace(r.FormValue("catalog_id"))
	nickname := optStr(r.FormValue("nickname"))
	if _, err := h.Service.CreateUserCard(r.Context(), catalogID, nickname, nil); err != nil {
		h.fail(w, "add card", err)
		return
	}
	seeOther(w, r, "/cards/wallet")
}

// WalletAddCustomPage handles GET /cards/wallet/add-custom (SCN-083-J03).
func (h *CardRewardsWebHandler) WalletAddCustomPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "cardrewards-wallet-add-custom.html", map[string]any{"Title": "Add Custom Card"})
}

// WalletAddCustom handles POST /cards/wallet/custom (SCN-083-J03): create a
// manual (non-catalog) card plus its wallet entry.
func (h *CardRewardsWebHandler) WalletAddCustom(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	in := cardrewards.CustomCardInput{
		Name:           r.FormValue("name"),
		Issuer:         r.FormValue("issuer"),
		CardType:       r.FormValue("card_type"),
		AnnualFeeCents: atoiDefault(r.FormValue("annual_fee_cents"), 0),
		Nickname:       optStr(r.FormValue("nickname")),
		Note:           optStr(r.FormValue("note")),
	}
	if _, err := h.Service.CreateCustomCard(r.Context(), in); err != nil {
		h.fail(w, "add custom card", err)
		return
	}
	seeOther(w, r, "/cards/wallet")
}

// WalletEditPage handles GET /cards/wallet/{id}/edit (SCN-083-J04).
func (h *CardRewardsWebHandler) WalletEditPage(w http.ResponseWriter, r *http.Request) {
	card, err := h.Service.GetUserCard(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, "load card", err)
		return
	}
	h.render(w, "cardrewards-wallet-edit.html", map[string]any{"Title": "Edit Card", "Card": card})
}

// WalletUpdate handles POST /cards/wallet/{id} (SCN-083-J04): persist nickname +
// per-card note edits.
func (h *CardRewardsWebHandler) WalletUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	id := chi.URLParam(r, "id")
	nickname := r.FormValue("nickname")
	note := r.FormValue("note")
	if _, err := h.Service.UpdateUserCard(r.Context(), id, &nickname, &note, nil); err != nil {
		h.fail(w, "update card", err)
		return
	}
	seeOther(w, r, "/cards/wallet")
}

// WalletToggle handles POST /cards/wallet/{id}/toggle (SCN-083-J05): flip the
// active state so the optimizer includes/excludes the card.
func (h *CardRewardsWebHandler) WalletToggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cur, err := h.Service.GetUserCard(r.Context(), id)
	if err != nil {
		h.fail(w, "load card", err)
		return
	}
	next := !cur.Active
	if _, err := h.Service.UpdateUserCard(r.Context(), id, nil, nil, &next); err != nil {
		h.fail(w, "toggle card", err)
		return
	}
	seeOther(w, r, "/cards/wallet")
}

// WalletDelete handles POST /cards/wallet/{id}/delete: remove a wallet entry
// (cascades to its offers, selections, and bonuses).
func (h *CardRewardsWebHandler) WalletDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.DeleteUserCard(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.fail(w, "remove card", err)
		return
	}
	seeOther(w, r, "/cards/wallet")
}

// ---- offers -----------------------------------------------------------------

// OffersPage handles GET /cards/offers (SCN-083-J06): list offers across all
// wallet entries plus the add form.
func (h *CardRewardsWebHandler) OffersPage(w http.ResponseWriter, r *http.Request) {
	offers, err := h.Service.ListOffers(r.Context())
	if err != nil {
		h.fail(w, "load offers", err)
		return
	}
	names, opts, err := h.cardNameIndex(r)
	if err != nil {
		h.fail(w, "load cards", err)
		return
	}
	rows := make([]offerRow, 0, len(offers))
	for _, o := range offers {
		rows = append(rows, offerRow{Offer: o, CardName: offerCardName(o.UserCardID, names)})
	}
	h.render(w, "cardrewards-offers.html", map[string]any{
		"Title": "Offers", "Offers": rows, "Cards": opts,
	})
}

// OfferCreate handles POST /cards/offers (SCN-083-J06 add): create an offer,
// including a shared_limit_group.
func (h *CardRewardsWebHandler) OfferCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	o := cardrewards.Offer{
		UserCardID:         optStr(r.FormValue("user_card_id")),
		Title:              r.FormValue("title"),
		Category:           r.FormValue("category"),
		Rate:               atofDefault(r.FormValue("rate"), 0),
		RateType:           r.FormValue("rate_type"),
		LimitCents:         optInt(r.FormValue("limit_cents")),
		SharedLimitGroup:   optStr(r.FormValue("shared_limit_group")),
		ActivationRequired: r.FormValue("activation_required") == "on",
		Notes:              optStr(r.FormValue("notes")),
	}
	if _, err := h.Service.CreateOffer(r.Context(), o); err != nil {
		h.fail(w, "add offer", err)
		return
	}
	seeOther(w, r, "/cards/offers")
}

// OfferEditPage handles GET /cards/offers/{id}/edit (SCN-083-J06 edit).
func (h *CardRewardsWebHandler) OfferEditPage(w http.ResponseWriter, r *http.Request) {
	offer, err := h.Service.GetOffer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, "load offer", err)
		return
	}
	_, opts, err := h.cardNameIndex(r)
	if err != nil {
		h.fail(w, "load cards", err)
		return
	}
	h.render(w, "cardrewards-offer-edit.html", map[string]any{
		"Title": "Edit Offer", "Offer": offer, "Cards": opts,
	})
}

// OfferUpdate handles POST /cards/offers/{id} (SCN-083-J06): persist edits so
// they round-trip. Fields not exposed by the form (dates, limit period) are
// preserved from the current row.
func (h *CardRewardsWebHandler) OfferUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	id := chi.URLParam(r, "id")
	cur, err := h.Service.GetOffer(r.Context(), id)
	if err != nil {
		h.fail(w, "load offer", err)
		return
	}
	cur.UserCardID = optStr(r.FormValue("user_card_id"))
	cur.Title = r.FormValue("title")
	cur.Category = r.FormValue("category")
	cur.Rate = atofDefault(r.FormValue("rate"), cur.Rate)
	cur.RateType = r.FormValue("rate_type")
	cur.LimitCents = optInt(r.FormValue("limit_cents"))
	cur.SharedLimitGroup = optStr(r.FormValue("shared_limit_group"))
	cur.ActivationRequired = r.FormValue("activation_required") == "on"
	cur.Activated = r.FormValue("activated") == "on"
	cur.Notes = optStr(r.FormValue("notes"))
	if _, err := h.Service.UpdateOffer(r.Context(), *cur); err != nil {
		h.fail(w, "update offer", err)
		return
	}
	seeOther(w, r, "/cards/offers")
}

// OfferToggle handles POST /cards/offers/{id}/toggle: flip the activated flag.
func (h *CardRewardsWebHandler) OfferToggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cur, err := h.Service.GetOffer(r.Context(), id)
	if err != nil {
		h.fail(w, "load offer", err)
		return
	}
	cur.Activated = !cur.Activated
	if _, err := h.Service.UpdateOffer(r.Context(), *cur); err != nil {
		h.fail(w, "toggle offer", err)
		return
	}
	seeOther(w, r, "/cards/offers")
}

// OfferDelete handles POST /cards/offers/{id}/delete.
func (h *CardRewardsWebHandler) OfferDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.DeleteOffer(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.fail(w, "remove offer", err)
		return
	}
	seeOther(w, r, "/cards/offers")
}

// ---- selections -------------------------------------------------------------

// SelectionsPage handles GET /cards/selections: list selections plus the
// add/tiered-save form.
func (h *CardRewardsWebHandler) SelectionsPage(w http.ResponseWriter, r *http.Request) {
	sels, err := h.Service.ListSelections(r.Context())
	if err != nil {
		h.fail(w, "load selections", err)
		return
	}
	names, opts, err := h.cardNameIndex(r)
	if err != nil {
		h.fail(w, "load cards", err)
		return
	}
	rows := make([]selectionRow, 0, len(sels))
	for _, s := range sels {
		rows = append(rows, selectionRow{Selection: s, CardName: names[s.UserCardID]})
	}
	h.render(w, "cardrewards-selections.html", map[string]any{
		"Title": "Selections", "Selections": rows, "Cards": opts,
	})
}

// SelectionCreate handles POST /cards/selections (SCN-083-J07): save a non-tiered
// category and/or tier-1 + tier-2 categories for the period in one submit.
func (h *CardRewardsWebHandler) SelectionCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	userCardID := strings.TrimSpace(r.FormValue("user_card_id"))
	period := strings.TrimSpace(r.FormValue("period_label"))

	type tierCat struct {
		category string
		tier     *int
	}
	var toCreate []tierCat
	if c := strings.TrimSpace(r.FormValue("category")); c != "" {
		toCreate = append(toCreate, tierCat{category: c, tier: nil})
	}
	if c := strings.TrimSpace(r.FormValue("category_tier1")); c != "" {
		t := 1
		toCreate = append(toCreate, tierCat{category: c, tier: &t})
	}
	if c := strings.TrimSpace(r.FormValue("category_tier2")); c != "" {
		t := 2
		toCreate = append(toCreate, tierCat{category: c, tier: &t})
	}
	if len(toCreate) == 0 {
		h.fail(w, "save selection", fmt.Errorf("%w: at least one category is required", cardrewards.ErrValidation))
		return
	}
	for _, tc := range toCreate {
		if _, err := h.Service.CreateSelection(r.Context(), cardrewards.Selection{
			UserCardID:  userCardID,
			Category:    tc.category,
			Tier:        tc.tier,
			PeriodLabel: period,
			Enrolled:    true,
		}); err != nil {
			h.fail(w, "save selection", err)
			return
		}
	}
	seeOther(w, r, "/cards/selections")
}

// SelectionEditPage handles GET /cards/selections/{id}/edit.
func (h *CardRewardsWebHandler) SelectionEditPage(w http.ResponseWriter, r *http.Request) {
	sel, err := h.Service.GetSelection(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, "load selection", err)
		return
	}
	_, opts, err := h.cardNameIndex(r)
	if err != nil {
		h.fail(w, "load cards", err)
		return
	}
	h.render(w, "cardrewards-selection-edit.html", map[string]any{
		"Title": "Edit Selection", "Selection": sel, "Cards": opts,
	})
}

// SelectionUpdate handles POST /cards/selections/{id}.
func (h *CardRewardsWebHandler) SelectionUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	id := chi.URLParam(r, "id")
	cur, err := h.Service.GetSelection(r.Context(), id)
	if err != nil {
		h.fail(w, "load selection", err)
		return
	}
	cur.Category = r.FormValue("category")
	cur.PeriodLabel = r.FormValue("period_label")
	cur.Tier = optInt(r.FormValue("tier"))
	cur.Enrolled = r.FormValue("enrolled") == "on"
	if _, err := h.Service.UpdateSelection(r.Context(), *cur); err != nil {
		h.fail(w, "update selection", err)
		return
	}
	seeOther(w, r, "/cards/selections")
}

// ---- bonuses ----------------------------------------------------------------

// BonusesPage handles GET /cards/bonuses: list signup bonuses + add form.
func (h *CardRewardsWebHandler) BonusesPage(w http.ResponseWriter, r *http.Request) {
	bonuses, err := h.Service.ListBonuses(r.Context())
	if err != nil {
		h.fail(w, "load bonuses", err)
		return
	}
	names, opts, err := h.cardNameIndex(r)
	if err != nil {
		h.fail(w, "load cards", err)
		return
	}
	rows := make([]bonusRow, 0, len(bonuses))
	for _, b := range bonuses {
		rows = append(rows, bonusRow{SignupBonus: b, CardName: names[b.UserCardID]})
	}
	h.render(w, "cardrewards-bonuses.html", map[string]any{
		"Title": "Sign-up Bonuses", "Bonuses": rows, "Cards": opts,
	})
}

// BonusCreate handles POST /cards/bonuses.
func (h *CardRewardsWebHandler) BonusCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	b := cardrewards.SignupBonus{
		UserCardID:         strings.TrimSpace(r.FormValue("user_card_id")),
		BonusType:          r.FormValue("bonus_type"),
		Description:        r.FormValue("description"),
		SpendRequiredCents: optInt(r.FormValue("spend_required_cents")),
		SpendProgressCents: atoiDefault(r.FormValue("spend_progress_cents"), 0),
		RewardDescription:  optStr(r.FormValue("reward_description")),
		Deadline:           optDate(r.FormValue("deadline")),
	}
	if _, err := h.Service.CreateSignupBonus(r.Context(), b); err != nil {
		h.fail(w, "add bonus", err)
		return
	}
	seeOther(w, r, "/cards/bonuses")
}

// BonusProgress handles POST /cards/bonuses/{id}/progress: record manual
// spend-progress entry; Met is recomputed by the service for spend bonuses.
func (h *CardRewardsWebHandler) BonusProgress(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	id := chi.URLParam(r, "id")
	cur, err := h.Service.GetBonus(r.Context(), id)
	if err != nil {
		h.fail(w, "load bonus", err)
		return
	}
	cur.SpendProgressCents = atoiDefault(r.FormValue("spend_progress_cents"), cur.SpendProgressCents)
	if _, err := h.Service.UpdateBonus(r.Context(), *cur); err != nil {
		h.fail(w, "update bonus progress", err)
		return
	}
	seeOther(w, r, "/cards/bonuses")
}

// ---- categories -------------------------------------------------------------

// CategoriesPage handles GET /cards/categories (SCN-083-J08): manage canonical
// names, equivalents, starred, and priority.
func (h *CardRewardsWebHandler) CategoriesPage(w http.ResponseWriter, r *http.Request) {
	aliases, err := h.Service.ListCategoryAliases(r.Context())
	if err != nil {
		h.fail(w, "load categories", err)
		return
	}
	h.render(w, "cardrewards-categories.html", map[string]any{"Title": "Categories", "Aliases": aliases})
}

// CategoryUpsert handles POST /cards/categories (SCN-083-J08): idempotent upsert
// on the canonical name — add equivalents, star, set priority.
func (h *CardRewardsWebHandler) CategoryUpsert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse form", err)
		return
	}
	a := cardrewards.CategoryAlias{
		CanonicalCategory: r.FormValue("canonical_category"),
		Equivalents:       splitCSV(r.FormValue("equivalents")),
		Starred:           r.FormValue("starred") == "on",
		Priority:          optInt(r.FormValue("priority")),
	}
	if _, err := h.Service.CreateCategoryAlias(r.Context(), a); err != nil {
		h.fail(w, "save category", err)
		return
	}
	seeOther(w, r, "/cards/categories")
}

// ---- helpers ----------------------------------------------------------------

// cardNameIndex returns a userCardID→display-name map and the matching select
// options for the add/edit forms.
func (h *CardRewardsWebHandler) cardNameIndex(r *http.Request) (map[string]string, []cardOption, error) {
	cards, err := h.Service.ListUserCards(r.Context(), false)
	if err != nil {
		return nil, nil, err
	}
	names := make(map[string]string, len(cards))
	opts := make([]cardOption, 0, len(cards))
	for _, c := range cards {
		name := cardDisplayName(c)
		names[c.ID] = name
		opts = append(opts, cardOption{ID: c.ID, Name: name})
	}
	return names, opts, nil
}

func cardDisplayName(c cardrewards.UserCard) string {
	if c.Nickname != nil && strings.TrimSpace(*c.Nickname) != "" {
		if c.CatalogName != "" {
			return *c.Nickname + " (" + c.CatalogName + ")"
		}
		return *c.Nickname
	}
	if c.CatalogName != "" {
		return c.CatalogName
	}
	return c.ID
}

func offerCardName(userCardID *string, names map[string]string) string {
	if userCardID == nil || *userCardID == "" {
		return "General"
	}
	if n, ok := names[*userCardID]; ok {
		return n
	}
	return *userCardID
}

func (h *CardRewardsWebHandler) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("cardrewards web: render failed", "template", name, "error", err)
		http.Error(w, "render failed", http.StatusInternalServerError)
	}
}

func (h *CardRewardsWebHandler) fail(w http.ResponseWriter, what string, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, cardrewards.ErrValidation):
		status = http.StatusBadRequest
	case errors.Is(err, cardrewards.ErrUserCardNotFound),
		errors.Is(err, cardrewards.ErrCatalogNotFound),
		errors.Is(err, cardrewards.ErrOfferNotFound),
		errors.Is(err, cardrewards.ErrSelectionNotFound),
		errors.Is(err, cardrewards.ErrBonusNotFound):
		status = http.StatusNotFound
	}
	slog.Error("cardrewards web: "+what, "error", err)
	http.Error(w, what+": "+err.Error(), status)
}

func seeOther(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func optStr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func optInt(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func atofDefault(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return f
}

func optDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
