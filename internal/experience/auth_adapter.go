package experience

// This file is the spec 106 (SCOPE-106-03) renderer-neutral AUTHENTICATED-
// REQUEST presentation adapter. It standardizes ONLY the cross-surface
// PRESENTATION response to an already-classified auth outcome:
//
//   - 401 (session ended): synchronously CLEAR every protected presentation
//     target (protected DOM markers, accessible labels, in-memory business
//     state, pending work, graph pixels) BEFORE re-authentication, retaining no
//     session, and offer a safe re-authentication path.
//   - 403 (access denied): RETAIN the valid session, clear nothing, and show an
//     access-denied state without looping through login.
//
// PRESENTATION CONTRACT ONLY. This adapter does NOT issue, read, verify, or
// parse an auth token, cookie, or middleware result. Its input is an ABSTRACT,
// already-classified AuthOutcome (authorized / 401 / 403) produced by the
// session owner's RequestAuthenticator — never a raw HTTP response. Token
// issuance and middleware verification are owned elsewhere (BUG-070-001) and are
// explicitly out of this scope's Change Boundary.

// AuthOutcome is the closed set of ABSTRACT, already-classified authenticated-
// request outcomes the adapter presents.
type AuthOutcome string

const (
	AuthAuthorized   AuthOutcome = "authorized"    // valid session, request authorized
	AuthSessionEnded AuthOutcome = "session_ended" // 401: session missing/expired/revoked/wrong purpose
	AuthAccessDenied AuthOutcome = "access_denied" // 403: valid session lacks scope/role
)

// ProtectedPresentationTarget is the closed set of protected presentation
// surfaces that MUST be cleared synchronously on a 401 before re-authentication.
type ProtectedPresentationTarget string

const (
	TargetProtectedDOM          ProtectedPresentationTarget = "protected_dom"
	TargetAccessibleLabels      ProtectedPresentationTarget = "accessible_labels"
	TargetInMemoryBusinessState ProtectedPresentationTarget = "in_memory_business_state"
	TargetPendingWork           ProtectedPresentationTarget = "pending_work"
	TargetGraphPixels           ProtectedPresentationTarget = "graph_pixels"
)

// allProtectedTargets is the closed set cleared synchronously on a 401, in a
// stable/deterministic order so callers and tests can assert on it.
var allProtectedTargets = []ProtectedPresentationTarget{
	TargetProtectedDOM,
	TargetAccessibleLabels,
	TargetInMemoryBusinessState,
	TargetPendingWork,
	TargetGraphPixels,
}

// AuthPresentation is the renderer-neutral presentation directive an adapter
// applies for an auth outcome. It carries NO raw content, token, cookie, user
// id, scope, or route fragment.
type AuthPresentation struct {
	// ViewState is the auth-driven content state, set ONLY for the failure
	// outcomes (401 -> unauthorized, 403 -> access_denied). For an authorized
	// outcome it is the empty string: auth imposes no content override, so the
	// content axis stays owned by ExperienceStatePresenter (axis independence).
	ViewState ViewState
	// RetainSession is false for 401 (session ended) and true for 403 (access
	// denied) and authorized.
	RetainSession bool
	// ClearedTargets is the closed set of protected presentation surfaces
	// cleared synchronously before re-authentication. It is fully populated on
	// 401 and empty on 403 / authorized.
	ClearedTargets []ProtectedPresentationTarget
	// OffersSafeReauthentication is true only on 401. A 403 never loops through
	// login.
	OffersSafeReauthentication bool
}

// AuthenticatedRequestAdapter presents an already-classified auth outcome. It is
// presentation-only and never touches token issuance or middleware verification.
type AuthenticatedRequestAdapter struct{}

// Present maps an abstract, already-classified AuthOutcome to its renderer-
// neutral AuthPresentation. It fails closed to *F106PresentationError on an
// unknown outcome; it never guesses, never retains protected presentation after
// a 401, and never loops a 403 through login.
func (AuthenticatedRequestAdapter) Present(o AuthOutcome) (AuthPresentation, error) {
	switch o {
	case AuthSessionEnded: // 401
		return AuthPresentation{
			ViewState:                  ViewUnauthorized,
			RetainSession:              false,
			ClearedTargets:             append([]ProtectedPresentationTarget(nil), allProtectedTargets...),
			OffersSafeReauthentication: true,
		}, nil
	case AuthAccessDenied: // 403
		return AuthPresentation{
			ViewState:                  ViewAccessDenied,
			RetainSession:              true,
			ClearedTargets:             nil,
			OffersSafeReauthentication: false,
		}, nil
	case AuthAuthorized:
		return AuthPresentation{
			ViewState:                  "",
			RetainSession:              true,
			ClearedTargets:             nil,
			OffersSafeReauthentication: false,
		}, nil
	default:
		return AuthPresentation{}, &F106PresentationError{Surface: "auth", Violations: []string{"unknown-auth-outcome:" + string(o)}}
	}
}
