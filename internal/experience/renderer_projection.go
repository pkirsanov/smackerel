package experience

// This file is the spec 106 (SCOPE-106-04, slice 1) renderer-neutral
// EXPERIENCE PROJECTION + SHADOW ADAPTERS foundation. It builds ONE
// content-free ExperienceProjection from the generated catalog (SCOPE-106-02),
// the resolved shell appearance (SCOPE-106-01), and the readiness-owned
// availability state contract (SCOPE-106-03), and renders that single
// projection through three independent SHADOW adapters — server template, PWA
// DOM, and Card chrome — into content-free comparison fixtures + a deterministic
// projection digest.
//
// SHADOW MODE ONLY. Nothing here changes an active link, page body, route
// authorization, or capability claim. The handwritten navigation authorities
// (internal/web/appshell.go, web/pwa/lib/appnav.js) remain ACTIVE and untouched;
// the shell cutover that replaces them is SCOPE-106-05. These adapters only
// render golden comparison fixtures + telemetry markers proving the three
// renderers consume ONE projection identically before any cutover.
//
// Hard invariants of this slice:
//
//   - CONTENT-FREE: the projection, digest, and every fixture carry surface
//     identity, hierarchy, order, wayfinding label, exact route binding,
//     current/parent-current state, audience, presented availability, and a
//     content-free action token ONLY. They never carry session scope, evidence
//     IDs, user content, or a raw readiness fact.
//   - AVAILABILITY FROM READINESS ONLY: every projected surface's availability
//     is PRESENTED (never derived) through ExperienceStatePresenter, which
//     refuses any source except a resolved readiness fact (SCOPE-106-03).
//   - FAIL CLOSED, NO FALLBACK: BuildExperienceProjection and every adapter
//     return a typed *F106PresentationError on ANY inconsistency and NEVER emit
//     an optimistic or static-ready fixture. Adapter failure stays visible in
//     the shadow evidence (explicit ShadowFailure + a non-settled fixture).
//   - SAFE DOM: the PWA adapter constructs nodes from a closed SAFE DOM-op
//     vocabulary (create element, set data-* attribute, set textContent, append
//     child, mark settled). The vocabulary makes innerHTML, arbitrary-attribute
//     injection, and bearer/credential ops UNREPRESENTABLE.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// ── Shell appearance (content-free; mirrors SCOPE-106-01 web appearance) ──────
//
// The projection consumes the resolved shell appearance as two closed,
// content-free enums. The caller derives these from the SCOPE-106-01 appearance
// codec (web.AppearancePreference.HTMLDataAttributes); they are duplicated as a
// tiny closed local enum ON PURPOSE so this renderer-neutral core never imports
// the server (web) renderer package and the shell-vs-renderer dependency
// direction is preserved.

// ShellTheme is the closed shell theme enum.
type ShellTheme string

const (
	ShellThemeSystem ShellTheme = "system"
	ShellThemeLight  ShellTheme = "light"
	ShellThemeDark   ShellTheme = "dark"
)

func (t ShellTheme) valid() bool {
	return t == ShellThemeSystem || t == ShellThemeLight || t == ShellThemeDark
}

// ShellDensity is the closed shell density enum.
type ShellDensity string

const (
	ShellDensityComfortable ShellDensity = "comfortable"
	ShellDensityCompact     ShellDensity = "compact"
)

func (d ShellDensity) valid() bool {
	return d == ShellDensityComfortable || d == ShellDensityCompact
}

// ShellAppearance is the resolved, content-free shell appearance shared by every
// renderer. It carries NO user id, session, route, prompt, card, provider, or
// readiness value.
type ShellAppearance struct {
	Theme   ShellTheme
	Density ShellDensity
}

// ── Content-free surface action token ────────────────────────────────────────

// SurfaceAction is the closed set of content-free action tokens a projected
// surface carries. It is derived from the catalog surface kind + readiness
// discoverability policy; it is NEVER user content and never an href.
type SurfaceAction string

const (
	// ActionNavigate — an active leaf with an exact registered route.
	ActionNavigate SurfaceAction = "navigate"
	// ActionOpenGroup — a route-free grouping node (opens its child menu).
	ActionOpenGroup SurfaceAction = "open_group"
	// ActionUnavailable — an honestly-unavailable leaf (carries no route).
	ActionUnavailable SurfaceAction = "unavailable"
	// ActionComposeLocal — a local view composed within a parent's page.
	ActionComposeLocal SurfaceAction = "compose_local"
)

func (a SurfaceAction) valid() bool {
	switch a {
	case ActionNavigate, ActionOpenGroup, ActionUnavailable, ActionComposeLocal:
		return true
	}
	return false
}

// ── ExperienceProjection ─────────────────────────────────────────────────────

// ProjectedSurface is one renderer-neutral, content-free projected navigation
// surface. It carries EXACTLY the fields the shadow parity compares: identity,
// parent, order, wayfinding label, exact route binding, current/parent-current
// state, audience context, presented availability, and a content-free action
// token. It carries NO user content, session, evidence id, or readiness fact.
type ProjectedSurface struct {
	SurfaceID       string
	ParentSurfaceID string
	Order           int
	Label           string
	Href            string
	Current         bool
	ParentCurrent   bool
	Audience        string
	Availability    Availability
	Action          SurfaceAction
}

// ExperienceProjection is the single renderer-neutral projection every shadow
// adapter (server, PWA, card) consumes. It is content-free: it carries the
// generated experience version, the audience context, the resolved shell
// appearance, and the ordered projected surfaces. Its ProjectionDigest is a
// deterministic fingerprint over the shell + surface fields ONLY.
type ExperienceProjection struct {
	ExperienceVersion string
	Audience          string
	Appearance        ShellAppearance
	Surfaces          []ProjectedSurface
}

// ProjectionDigest returns a deterministic sha256 fingerprint over the
// content-free shell + surface fields of the projection (experience version,
// audience, appearance, then each ordered surface's identity/parent/order/
// label/href/current/parent-current/audience/availability/action). It is
// computed over navigation structure ONLY — never over user content — so an
// identical projection yields an identical digest across every renderer, and any
// structural divergence changes it.
func (p ExperienceProjection) ProjectionDigest() string {
	var b strings.Builder
	b.WriteString("experience-version=")
	b.WriteString(p.ExperienceVersion)
	b.WriteString("\naudience=")
	b.WriteString(p.Audience)
	b.WriteString("\ntheme=")
	b.WriteString(string(p.Appearance.Theme))
	b.WriteString("\ndensity=")
	b.WriteString(string(p.Appearance.Density))
	for _, s := range p.Surfaces {
		b.WriteString("\nsurface\t")
		b.WriteString(s.SurfaceID)
		b.WriteByte('\t')
		b.WriteString(s.ParentSurfaceID)
		b.WriteByte('\t')
		b.WriteString(strconv.Itoa(s.Order))
		b.WriteByte('\t')
		b.WriteString(s.Label)
		b.WriteByte('\t')
		b.WriteString(s.Href)
		b.WriteByte('\t')
		b.WriteString(strconv.FormatBool(s.Current))
		b.WriteByte('\t')
		b.WriteString(strconv.FormatBool(s.ParentCurrent))
		b.WriteByte('\t')
		b.WriteString(s.Audience)
		b.WriteByte('\t')
		b.WriteString(string(s.Availability))
		b.WriteByte('\t')
		b.WriteString(string(s.Action))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ── BuildExperienceProjection ────────────────────────────────────────────────

// SurfaceAvailabilityOutcome is the owner-supplied availability outcome for one
// surface. The Signal MUST be SignalReadinessResolved — availability can ONLY
// come from a resolved readiness fact (SCOPE-106-03 invariant); any other signal
// fails the projection closed.
type SurfaceAvailabilityOutcome struct {
	Signal AvailabilitySignal
	Value  Availability
}

// ProjectionRequest is the content-free input to BuildExperienceProjection.
type ProjectionRequest struct {
	// Audience is the principal's audience token; only surfaces whose Audiences
	// include it are projected. Required (non-empty).
	Audience string
	// CurrentSurfaceID is the currently-active surface; "" means none active.
	// When set it MUST exist in the catalog.
	CurrentSurfaceID string
	// Appearance is the resolved, content-free shell appearance the caller
	// derives from the SCOPE-106-01 appearance codec.
	Appearance ShellAppearance
	// Availability maps a surface ID to its owner availability outcome. EVERY
	// audience-visible surface MUST have an entry, and each entry's Signal MUST
	// be SignalReadinessResolved.
	Availability map[string]SurfaceAvailabilityOutcome
}

// BuildExperienceProjection builds the single renderer-neutral projection from
// the catalog, request, and availability contract. It is PURE and DETERMINISTIC
// (a stable (parent, order, id) total order) and FAILS CLOSED to a typed
// *F106PresentationError on any inconsistency — an empty audience/catalog, an
// invalid appearance, a malformed catalog surface, an unknown current surface, a
// missing availability outcome, or an availability outcome not sourced from a
// resolved readiness fact. It NEVER emits an optimistic or static-ready
// fallback.
func BuildExperienceProjection(cat ProductExperienceCatalog, req ProjectionRequest) (ExperienceProjection, error) {
	var violations []string
	if strings.TrimSpace(req.Audience) == "" {
		violations = append(violations, "empty-audience")
	}
	if cat.SchemaVersion == "" {
		violations = append(violations, "empty-experience-version")
	}
	if len(cat.Surfaces) == 0 {
		violations = append(violations, "empty-catalog")
	}
	if !req.Appearance.Theme.valid() {
		violations = append(violations, "invalid-shell-theme:"+string(req.Appearance.Theme))
	}
	if !req.Appearance.Density.valid() {
		violations = append(violations, "invalid-shell-density:"+string(req.Appearance.Density))
	}

	// Index surfaces by ID and validate catalog surface consistency (mirrors the
	// generated-catalog validator so Build fails closed on a malformed catalog:
	// an active leaf MUST bind a route; a group / honestly-unavailable leaf MUST
	// NOT carry one).
	byID := make(map[string]Surface, len(cat.Surfaces))
	for _, s := range cat.Surfaces {
		if s.ID == "" {
			violations = append(violations, "surface-with-empty-id")
			continue
		}
		if _, dup := byID[s.ID]; dup {
			violations = append(violations, "duplicate-surface-id:"+s.ID)
		}
		byID[s.ID] = s
		if !s.Kind.valid() {
			violations = append(violations, "invalid-kind:"+s.ID)
		}
		if !s.ReadinessDiscoverabilityPolicy.valid() {
			violations = append(violations, "invalid-policy:"+s.ID)
		}
		switch {
		case s.ReadinessDiscoverabilityPolicy.active():
			if s.Href == "" {
				violations = append(violations, "active-leaf-without-href:"+s.ID)
			}
		case s.ReadinessDiscoverabilityPolicy == PolicyRouteFreeGroup:
			if s.Href != "" {
				violations = append(violations, "route-group-with-href:"+s.ID)
			}
		case s.ReadinessDiscoverabilityPolicy.unavailable():
			if s.Href != "" {
				violations = append(violations, "unavailable-leaf-with-href:"+s.ID)
			}
		}
	}
	if req.CurrentSurfaceID != "" {
		if _, ok := byID[req.CurrentSurfaceID]; !ok {
			violations = append(violations, "unknown-current-surface:"+req.CurrentSurfaceID)
		}
	}
	if len(violations) > 0 {
		return ExperienceProjection{}, &F106PresentationError{Surface: "projection", Violations: dedupeSorted(violations)}
	}

	currentParentID := ""
	if req.CurrentSurfaceID != "" {
		currentParentID = byID[req.CurrentSurfaceID].ParentID
	}

	sp := ExperienceStatePresenter{}
	var projected []ProjectedSurface
	for _, s := range cat.Surfaces {
		if !audienceIncludes(s.Audiences, req.Audience) {
			continue
		}
		// Availability is PRESENTED (never derived) through the state contract:
		// an outcome is REQUIRED for every visible surface and MUST originate
		// from a resolved readiness fact.
		out, ok := req.Availability[s.ID]
		if !ok {
			violations = append(violations, "missing-availability-outcome:"+s.ID)
			continue
		}
		av, err := sp.PresentAvailability(out.Signal, out.Value)
		if err != nil {
			violations = append(violations, "availability-not-readiness-resolved:"+s.ID)
			continue
		}
		projected = append(projected, ProjectedSurface{
			SurfaceID:       s.ID,
			ParentSurfaceID: s.ParentID,
			Order:           s.Order,
			Label:           s.Label,
			Href:            s.Href,
			Current:         s.ID == req.CurrentSurfaceID,
			ParentCurrent:   currentParentID != "" && s.ID == currentParentID,
			Audience:        req.Audience,
			Availability:    av,
			Action:          surfaceAction(s),
		})
	}
	if len(violations) > 0 {
		return ExperienceProjection{}, &F106PresentationError{Surface: "projection", Violations: dedupeSorted(violations)}
	}
	if len(projected) == 0 {
		return ExperienceProjection{}, &F106PresentationError{
			Surface:    "projection",
			Violations: []string{"no-audience-visible-surfaces:" + req.Audience},
		}
	}

	// Deterministic order: (parent, order, id) is a stable total order.
	sort.Slice(projected, func(i, j int) bool {
		a, b := projected[i], projected[j]
		if a.ParentSurfaceID != b.ParentSurfaceID {
			return a.ParentSurfaceID < b.ParentSurfaceID
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.SurfaceID < b.SurfaceID
	})

	return ExperienceProjection{
		ExperienceVersion: cat.SchemaVersion,
		Audience:          req.Audience,
		Appearance:        req.Appearance,
		Surfaces:          projected,
	}, nil
}

// audienceIncludes reports whether audience a is in the surface's audience set.
func audienceIncludes(auds []string, a string) bool {
	for _, x := range auds {
		if x == a {
			return true
		}
	}
	return false
}

// surfaceAction maps a validated catalog surface to its content-free action
// token. Build validates kind/policy consistency first, so this mapping is total
// over a well-formed catalog.
func surfaceAction(s Surface) SurfaceAction {
	switch {
	case s.Kind == KindRouteGroup || s.ReadinessDiscoverabilityPolicy == PolicyRouteFreeGroup:
		return ActionOpenGroup
	case s.Kind == KindLocalView:
		return ActionComposeLocal
	case s.ReadinessDiscoverabilityPolicy.unavailable():
		return ActionUnavailable
	case s.ReadinessDiscoverabilityPolicy.active():
		return ActionNavigate
	default:
		return ActionUnavailable
	}
}

// dedupeSorted returns the deterministic, de-duplicated, sorted violation list.
func dedupeSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// ── Safe DOM-op vocabulary + content-free contract markers ───────────────────

// DOMOpKind is the CLOSED vocabulary of SAFE DOM construction operations a
// shadow adapter may emit. There is deliberately NO innerHTML, no raw-HTML, no
// arbitrary-attribute injection, and no credential/bearer op: the vocabulary
// itself makes unsafe DOM construction UNREPRESENTABLE.
type DOMOpKind string

const (
	DOMCreateElement  DOMOpKind = "create_element"
	DOMSetDataAttr    DOMOpKind = "set_data_attr"    // data-* attributes ONLY
	DOMSetTextContent DOMOpKind = "set_text_content" // textContent (never innerHTML)
	DOMAppendChild    DOMOpKind = "append_child"
	DOMMarkSettled    DOMOpKind = "mark_settled"
)

// DOMOp is one safe DOM construction step. AttrKey is constrained to the data-*
// namespace; Text is assigned via textContent, never parsed as HTML.
type DOMOp struct {
	Kind    DOMOpKind
	Element string
	AttrKey string
	AttrVal string
	Text    string
}

// ContractMarker is one content-free data-* contract marker emitted into a
// shadow fixture. Keys are restricted to the closed ShadowMarkerKeys allowlist;
// values carry NO user content, session, evidence id, or readiness fact.
type ContractMarker struct {
	Key string
	Val string
}

// Closed content-free contract-marker key namespace.
const (
	MarkerExperienceVersion   = "data-experience-version"
	MarkerProductNavigation   = "data-product-navigation"
	MarkerProjectionDigest    = "data-projection-digest"
	MarkerShellTheme          = "data-theme"
	MarkerShellDensity        = "data-density"
	MarkerSurfaceID           = "data-surface-id"
	MarkerParentSurfaceID     = "data-parent-surface-id"
	MarkerSurfaceOrder        = "data-surface-order"
	MarkerSurfaceCurrent      = "data-surface-current"
	MarkerParentCurrent       = "data-parent-current"
	MarkerSurfaceAudience     = "data-surface-audience"
	MarkerSurfaceAvailability = "data-surface-availability"
	MarkerSurfaceAction       = "data-surface-action"
	MarkerSettled             = "data-shadow-settled"
)

// ShadowMarkerKeys is the closed allowlist of content-free data-* marker keys a
// shadow fixture may emit. A key outside this set means user content or an
// out-of-contract attribute leaked into the fixture.
func ShadowMarkerKeys() map[string]bool {
	return map[string]bool{
		MarkerExperienceVersion:   true,
		MarkerProductNavigation:   true,
		MarkerProjectionDigest:    true,
		MarkerShellTheme:          true,
		MarkerShellDensity:        true,
		MarkerSurfaceID:           true,
		MarkerParentSurfaceID:     true,
		MarkerSurfaceOrder:        true,
		MarkerSurfaceCurrent:      true,
		MarkerParentCurrent:       true,
		MarkerSurfaceAudience:     true,
		MarkerSurfaceAvailability: true,
		MarkerSurfaceAction:       true,
		MarkerSettled:             true,
	}
}

// ── Shadow fixtures + adapters ───────────────────────────────────────────────

// ShadowFailure is the explicit, content-free failure marker a shadow adapter
// emits when it detects an inconsistent projection. Its presence (never a
// settled/ready fixture) is how adapter failure stays VISIBLE without any
// optimistic or static-ready fallback.
type ShadowFailure struct {
	Renderer   Renderer
	Violations []string
}

// ShadowFixture is the content-free comparison fixture a shadow adapter renders.
// It embeds ONLY structural data-* markers + a safe DOM-op stream + the
// projection digest + a terminal settled flag. On success Failure is nil and
// Settled is true; on a detected inconsistency Failure is set, Settled is false,
// and NO marker claims readiness.
type ShadowFixture struct {
	Renderer          Renderer
	ExperienceVersion string
	ProjectionDigest  string
	Markers           []ContractMarker
	DOMOps            []DOMOp
	Settled           bool
	Failure           *ShadowFailure
}

// UsesOnlySafeDOMOps reports whether every DOM op in the fixture is drawn from
// the safe closed vocabulary and every attribute is in the data-* namespace. It
// is the mechanical proof that the shadow fixture constructs nodes without
// innerHTML, arbitrary-attribute injection, or bearer/credential ops.
func (f ShadowFixture) UsesOnlySafeDOMOps() bool {
	for _, op := range f.DOMOps {
		switch op.Kind {
		case DOMCreateElement, DOMSetTextContent, DOMAppendChild, DOMMarkSettled:
			// inherently safe
		case DOMSetDataAttr:
			if !strings.HasPrefix(op.AttrKey, "data-") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ShadowRenderer is the shared contract every shadow adapter implements. The
// server, PWA, and Card adapters are three renderers over ONE projection; their
// only difference is their renderer identity, which is what guarantees golden
// parity by construction.
type ShadowRenderer interface {
	RendererID() Renderer
	RenderShadow(p ExperienceProjection) (ShadowFixture, error)
}

// ServerShadowRenderer is the server-template shadow adapter.
type ServerShadowRenderer struct{}

// PWAShadowRenderer is the PWA DOM shadow adapter. It constructs nodes ONLY from
// the safe DOM-op vocabulary — no innerHTML, no bearer injection, no
// static-ready fallback.
type PWAShadowRenderer struct{}

// CardShadowRenderer is the Card-chrome shadow adapter (a consumer of the shared
// shell contract that preserves every existing Card route/behavior).
type CardShadowRenderer struct{}

func (ServerShadowRenderer) RendererID() Renderer { return RendererServer }
func (PWAShadowRenderer) RendererID() Renderer    { return RendererPWA }
func (CardShadowRenderer) RendererID() Renderer   { return RendererCard }

func (r ServerShadowRenderer) RenderShadow(p ExperienceProjection) (ShadowFixture, error) {
	return assembleShadowFixture(p, r.RendererID())
}

func (r PWAShadowRenderer) RenderShadow(p ExperienceProjection) (ShadowFixture, error) {
	return assembleShadowFixture(p, r.RendererID())
}

func (r CardShadowRenderer) RenderShadow(p ExperienceProjection) (ShadowFixture, error) {
	return assembleShadowFixture(p, r.RendererID())
}

// assembleShadowFixture renders the shared renderer-neutral projection into a
// content-free shadow fixture for the given renderer. It is the SINGLE source of
// the shadow contract every adapter emits: the three adapters differ only in
// their renderer identity, guaranteeing golden parity by construction. It
// re-validates the projection first (defense in depth — an adapter never trusts
// Build) and FAILS CLOSED on any inconsistency: it returns a non-settled fixture
// carrying an explicit ShadowFailure plus a typed error, and NEVER an optimistic
// or static-ready fixture.
func assembleShadowFixture(p ExperienceProjection, renderer Renderer) (ShadowFixture, error) {
	if v := validateProjection(p); len(v) > 0 {
		return ShadowFixture{
				Renderer: renderer,
				Settled:  false,
				Failure:  &ShadowFailure{Renderer: renderer, Violations: v},
			},
			&F106PresentationError{Surface: "shadow", Violations: v}
	}

	digest := p.ProjectionDigest()

	markers := []ContractMarker{
		{MarkerExperienceVersion, p.ExperienceVersion},
		{MarkerProductNavigation, "shadow"},
		{MarkerProjectionDigest, digest},
		{MarkerShellTheme, string(p.Appearance.Theme)},
		{MarkerShellDensity, string(p.Appearance.Density)},
	}
	ops := []DOMOp{
		{Kind: DOMCreateElement, Element: "product-navigation"},
		{Kind: DOMSetDataAttr, AttrKey: MarkerExperienceVersion, AttrVal: p.ExperienceVersion},
		{Kind: DOMSetDataAttr, AttrKey: MarkerProductNavigation, AttrVal: "shadow"},
		{Kind: DOMSetDataAttr, AttrKey: MarkerProjectionDigest, AttrVal: digest},
		{Kind: DOMSetDataAttr, AttrKey: MarkerShellTheme, AttrVal: string(p.Appearance.Theme)},
		{Kind: DOMSetDataAttr, AttrKey: MarkerShellDensity, AttrVal: string(p.Appearance.Density)},
	}

	for _, s := range p.Surfaces {
		markers = append(markers,
			ContractMarker{MarkerSurfaceID, s.SurfaceID},
			ContractMarker{MarkerParentSurfaceID, s.ParentSurfaceID},
			ContractMarker{MarkerSurfaceOrder, strconv.Itoa(s.Order)},
			ContractMarker{MarkerSurfaceCurrent, strconv.FormatBool(s.Current)},
			ContractMarker{MarkerParentCurrent, strconv.FormatBool(s.ParentCurrent)},
			ContractMarker{MarkerSurfaceAudience, s.Audience},
			ContractMarker{MarkerSurfaceAvailability, string(s.Availability)},
			ContractMarker{MarkerSurfaceAction, string(s.Action)},
		)
		ops = append(ops,
			DOMOp{Kind: DOMCreateElement, Element: "surface"},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerSurfaceID, AttrVal: s.SurfaceID},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerParentSurfaceID, AttrVal: s.ParentSurfaceID},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerSurfaceOrder, AttrVal: strconv.Itoa(s.Order)},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerSurfaceCurrent, AttrVal: strconv.FormatBool(s.Current)},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerParentCurrent, AttrVal: strconv.FormatBool(s.ParentCurrent)},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerSurfaceAudience, AttrVal: s.Audience},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerSurfaceAvailability, AttrVal: string(s.Availability)},
			DOMOp{Kind: DOMSetDataAttr, AttrKey: MarkerSurfaceAction, AttrVal: string(s.Action)},
			// The wayfinding label is assigned as a SAFE text node (textContent),
			// never concatenated into an HTML string / innerHTML.
			DOMOp{Kind: DOMSetTextContent, Text: s.Label},
			DOMOp{Kind: DOMAppendChild, Element: "surface"},
		)
	}

	markers = append(markers, ContractMarker{MarkerSettled, "true"})
	ops = append(ops, DOMOp{Kind: DOMMarkSettled})

	return ShadowFixture{
		Renderer:          renderer,
		ExperienceVersion: p.ExperienceVersion,
		ProjectionDigest:  digest,
		Markers:           markers,
		DOMOps:            ops,
		Settled:           true,
		Failure:           nil,
	}, nil
}

// validateProjection re-checks the projection's internal consistency at render
// time (defense in depth: an adapter never trusts that Build produced it). It
// returns a deterministic, content-free violation list; a non-empty list means
// the adapter MUST fail closed.
func validateProjection(p ExperienceProjection) []string {
	var v []string
	if p.ExperienceVersion == "" {
		v = append(v, "empty-experience-version")
	}
	if strings.TrimSpace(p.Audience) == "" {
		v = append(v, "empty-audience")
	}
	if !p.Appearance.Theme.valid() {
		v = append(v, "invalid-shell-theme")
	}
	if !p.Appearance.Density.valid() {
		v = append(v, "invalid-shell-density")
	}
	if len(p.Surfaces) == 0 {
		v = append(v, "no-projected-surfaces")
	}

	seen := make(map[string]bool, len(p.Surfaces))
	for _, s := range p.Surfaces {
		if s.SurfaceID == "" {
			v = append(v, "surface-with-empty-id")
			continue
		}
		if seen[s.SurfaceID] {
			v = append(v, "duplicate-surface-id:"+s.SurfaceID)
		}
		seen[s.SurfaceID] = true
		if s.Audience != p.Audience {
			v = append(v, "surface-audience-mismatch:"+s.SurfaceID)
		}
		if !s.Availability.valid() {
			v = append(v, "invalid-availability:"+s.SurfaceID)
		}
		if !s.Action.valid() {
			v = append(v, "invalid-action:"+s.SurfaceID)
		}
		// Action/href consistency — the exact inconsistency the fail-closed test
		// induces: a navigate action MUST carry a route; a group / unavailable
		// action MUST NOT.
		switch s.Action {
		case ActionNavigate:
			if s.Href == "" {
				v = append(v, "navigate-without-href:"+s.SurfaceID)
			}
		case ActionOpenGroup, ActionUnavailable:
			if s.Href != "" {
				v = append(v, "route-on-nonnavigable:"+s.SurfaceID)
			}
		}
		if s.Current && s.ParentCurrent {
			v = append(v, "current-and-parent-current:"+s.SurfaceID)
		}
	}
	return dedupeSorted(v)
}
