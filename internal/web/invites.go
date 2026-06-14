// Spec 093 SCOPE-03 — operator admin invites UI (generate / list / revoke).
//
// These are methods on the EXISTING CardRewardsWebHandler so they inherit the
// spec-092 head/cardrewards-nav/foot chrome + design-token CSS + the template
// FuncMap verbatim, with zero shared-template edits and zero FuncMap
// duplication. The three routes mount inside the existing /cards/admin
// webAuthMiddleware block (see RegisterRoutes), so they inherit the binding
// authorization (logged-in operator — NOT callerIsAdmin) for free.
//
// VALUE-SAFETY (AC-11): the one-time plaintext invite token leaves the process
// ONLY in the AdminInviteGenerate 200 response body (the reveal template). It
// is never logged, never stored (the repo holds only the SHA-256), never put in
// a redirect Location/query/cookie, and never appears in the list view (which
// renders metadata only). The error path re-renders WITHOUT the token.
package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/webinvite"
)

const (
	// inviteTTL is the v1 default invite lifetime at generation (design (b)).
	inviteTTL = 7 * 24 * time.Hour
	// maxInviteLabelLen defensively caps the optional operator label.
	maxInviteLabelLen = 120
	// inviteRaceNotice is the non-enumerating banner shown when a revoke (or a
	// stale-page action) was a no-op.
	inviteRaceNotice = "That invite was already used or revoked — nothing to do."
	// inviteGenerateError is the value-safe error banner (never echoes a token).
	inviteGenerateError = "Could not generate an invite. Please try again."
)

// inviteListVM backs cardrewards-invites.html. NO token, NO hash.
type inviteListVM struct {
	Title   string
	Invites []inviteRowView
	Notice  string
}

// inviteRevealVM backs cardrewards-invite-reveal.html. Token exists ONLY here.
type inviteRevealVM struct {
	Title   string
	Token   string
	Label   string
	Invites []inviteRowView
}

// inviteRowView is the pre-rendered metadata projection used by the templates.
// Status badge fields are computed in Go so the templates carry no fragile
// typed-string comparisons; it deliberately excludes the token hash entirely.
type inviteRowView struct {
	ID         string
	Label      *string
	CreatedBy  string
	CreatedAt  time.Time
	UsedBy     *string
	Status     string // string form for data-* hooks + display
	BadgeClass string
	BadgeGlyph string
	BadgeLabel string
	CanRevoke  bool
}

func inviteRowViews(rows []webinvite.InviteRow) []inviteRowView {
	out := make([]inviteRowView, 0, len(rows))
	for _, r := range rows {
		v := inviteRowView{
			ID:        r.ID,
			Label:     r.Label,
			CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt,
			UsedBy:    r.UsedBy,
			Status:    string(r.Status),
		}
		switch r.Status {
		case webinvite.StatusOutstanding:
			v.BadgeClass, v.BadgeGlyph, v.BadgeLabel, v.CanRevoke = "badge-info", "\u25CF", "Outstanding", true
		case webinvite.StatusUsed:
			v.BadgeClass, v.BadgeGlyph, v.BadgeLabel = "badge-success", "\u2713", "Used"
		case webinvite.StatusExpired:
			v.BadgeClass, v.BadgeGlyph, v.BadgeLabel = "badge-warning", "\u26A0", "Expired"
		case webinvite.StatusRevoked:
			v.BadgeClass, v.BadgeGlyph, v.BadgeLabel = "badge-danger", "\u2715", "Revoked"
		}
		out = append(out, v)
	}
	return out
}

// SetInvites late-wires the spec-093 invite repo (mirrors SetTriggers). nil ⇒
// the invite sub-pages return 503. Set in cmd/core/wiring.go after construction.
func (h *CardRewardsWebHandler) SetInvites(r webinvite.Repo) { h.Invites = r }

// AdminInvitesPage handles GET /cards/admin/invites — the metadata-only list +
// the generate form. Title "Admin" lights the Admin nav pill.
func (h *CardRewardsWebHandler) AdminInvitesPage(w http.ResponseWriter, r *http.Request) {
	if h.Invites == nil {
		http.Error(w, "invites unavailable", http.StatusServiceUnavailable)
		return
	}
	rows, err := h.Invites.List(r.Context())
	if err != nil {
		h.fail(w, "list invites", err)
		return
	}
	notice := ""
	if r.URL.Query().Get("notice") == "race" {
		notice = inviteRaceNotice
	}
	h.render(w, "cardrewards-invites.html", inviteListVM{
		Title:   "Admin",
		Invites: inviteRowViews(rows),
		Notice:  notice,
	})
}

// AdminInviteGenerate handles POST /cards/admin/invites — mints an invite and
// renders the one-time reveal at HTTP 200 (NOT a redirect: the plaintext must
// never travel via a Location/query/log).
func (h *CardRewardsWebHandler) AdminInviteGenerate(w http.ResponseWriter, r *http.Request) {
	if h.Invites == nil {
		http.Error(w, "invites unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.fail(w, "parse invite form", err)
		return
	}
	label := strings.TrimSpace(r.PostFormValue("label"))
	if len(label) > maxInviteLabelLen {
		label = label[:maxInviteLabelLen]
	}
	createdBy := sessionIdentity(r)

	plaintext, err := h.Invites.Generate(r.Context(), createdBy, label, inviteTTL)
	if err != nil {
		// Value-safe error path: re-render the list with an alert; NEVER echo
		// the token (there is none to echo — generation failed).
		rows, listErr := h.Invites.List(r.Context())
		if listErr != nil {
			h.fail(w, "generate invite", err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = h.Templates.ExecuteTemplate(w, "cardrewards-invites.html", inviteListVM{
			Title:   "Admin",
			Invites: inviteRowViews(rows),
			Notice:  inviteGenerateError,
		})
		return
	}

	rows, err := h.Invites.List(r.Context())
	if err != nil {
		h.fail(w, "list invites after generate", err)
		return
	}
	// HTTP 200 render-once: the plaintext appears ONLY in this body.
	h.render(w, "cardrewards-invite-reveal.html", inviteRevealVM{
		Title:   "Admin",
		Token:   plaintext,
		Label:   label,
		Invites: inviteRowViews(rows),
	})
}

// AdminInviteRevoke handles POST /cards/admin/invites/{id}/revoke — revokes then
// 303-redirects back to the list (PRG). A no-op (stale-page race) adds
// ?notice=race so the list shows the non-enumerating banner.
func (h *CardRewardsWebHandler) AdminInviteRevoke(w http.ResponseWriter, r *http.Request) {
	if h.Invites == nil {
		http.Error(w, "invites unavailable", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	outcome, err := h.Invites.Revoke(r.Context(), id)
	if err != nil {
		h.fail(w, "revoke invite", err)
		return
	}
	dest := "/cards/admin/invites"
	if outcome == webinvite.RevokeNoop {
		dest += "?notice=race"
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// sessionIdentity returns the operator's display identity for created_by:
// auth.Session.UserID when a per-user session is present and non-empty, else
// "operator" (shared-token sessions carry no distinct username — spec 070 "any
// web user = full admin"). Value-safe: never a secret; metadata only.
func sessionIdentity(r *http.Request) string {
	if sess, ok := auth.SessionFromContext(r.Context()); ok && sess.UserID != "" {
		return sess.UserID
	}
	return "operator"
}
