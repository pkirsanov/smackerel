package experience

import "strings"

// This file is the spec 106 (SCOPE-106-03) renderer-neutral CONTENT-STATE
// presentation foundation. It maps ABSTRACT, owner-classified read outcomes to
// a closed ViewState vocabulary and enforces the two hard invariants of the
// scope:
//
//   - Availability is an INDEPENDENT closed axis. A route existing, a flag being
//     enabled, an HTTP 200, a mounted handler, an empty array, or a health probe
//     CANNOT create availability. Only a resolved readiness fact can. This is
//     encoded as a closed type boundary (AvailabilitySignal): PresentAvailability
//     refuses every source except SignalReadinessResolved.
//   - Failure never becomes empty or success, and unknown/contradictory/unsafe
//     owner outcomes fail CLOSED to a typed error — never downgraded to empty,
//     ready, available, or success.
//
// The presenter NEVER queries a domain store, parses a raw error string, or
// infers health. Domain owners (Search, Digest, Assistant, Graph, Cards,
// Recommendations, Sources, Activity, readiness) keep their own typed outcomes
// and business rules; they pass an ABSTRACT OwnerReadOutcome across the seam and
// the presenter maps it. It imports no domain logic.

// ViewState is the closed set of page CONTENT states the shared presenter can
// render. It is INDEPENDENT of capability Availability and of MutationState.
type ViewState string

const (
	ViewLoading       ViewState = "loading"
	ViewReady         ViewState = "ready"
	ViewFirstUseEmpty ViewState = "first_use_empty"
	ViewFilteredEmpty ViewState = "filtered_empty"
	ViewStale         ViewState = "stale"
	ViewDegraded      ViewState = "degraded"
	ViewNeedsSetup    ViewState = "needs_setup"
	ViewDisabled      ViewState = "disabled"
	ViewUnauthorized  ViewState = "unauthorized"
	ViewAccessDenied  ViewState = "access_denied"
	ViewNotFound      ViewState = "not_found"
	ViewError         ViewState = "error"
)

func (s ViewState) valid() bool {
	switch s {
	case ViewLoading, ViewReady, ViewFirstUseEmpty, ViewFilteredEmpty, ViewStale,
		ViewDegraded, ViewNeedsSetup, ViewDisabled, ViewUnauthorized,
		ViewAccessDenied, ViewNotFound, ViewError:
		return true
	}
	return false
}

// Availability is the closed set of capability availability values. It is owned
// by the readiness resolver and merely PRESENTED here; the presenter never
// derives it from content, a route, a flag, health, or an HTTP status.
type Availability string

const (
	AvailabilityAvailable   Availability = "available"
	AvailabilityNeedsSetup  Availability = "needs_setup"
	AvailabilityDegraded    Availability = "degraded"
	AvailabilityUnavailable Availability = "unavailable"
)

func (a Availability) valid() bool {
	switch a {
	case AvailabilityAvailable, AvailabilityNeedsSetup, AvailabilityDegraded, AvailabilityUnavailable:
		return true
	}
	return false
}

// AvailabilitySignal is the closed set of inputs a caller might present as a
// SOURCE of availability. Only SignalReadinessResolved is a legitimate source.
// Every other value is a structural fact that MUST NOT create availability
// (SCN-106-005: a feature switch or route existing without a usable provider is
// NOT ready). This enum is the closed type boundary that makes availability
// independence mechanically enforceable.
type AvailabilitySignal string

const (
	// SignalReadinessResolved is the ONLY legitimate source of availability: an
	// authoritative value from the readiness owner.
	SignalReadinessResolved AvailabilitySignal = "readiness_resolved"
	// The following are structural facts that CANNOT create availability.
	SignalRouteRegistered AvailabilitySignal = "route_registered"
	SignalFlagEnabled     AvailabilitySignal = "flag_enabled"
	SignalHTTP200         AvailabilitySignal = "http_200"
	SignalHandlerMounted  AvailabilitySignal = "handler_mounted"
	SignalEmptyArray      AvailabilitySignal = "empty_array"
	SignalHealthOK        AvailabilitySignal = "health_ok"
)

// OwnerReadKind is the closed set of ABSTRACT read/content outcomes a domain
// owner reports to the shared presenter across the seam. The owner classifies
// its OWN outcome; the presenter never inspects raw error text, queries a store,
// or infers health from this value.
type OwnerReadKind string

const (
	ReadStarted       OwnerReadKind = "started"         // authorized operation started -> loading
	ReadPopulated     OwnerReadKind = "populated"       // successful authorized populated read -> ready
	ReadEmptyNoRecord OwnerReadKind = "empty_no_record" // successful zero-row read, no prior record -> first_use_empty
	ReadEmptyFiltered OwnerReadKind = "empty_filtered"  // successful read has data outside filter -> filtered_empty
	ReadStale         OwnerReadKind = "stale"           // last verified result exceeds owner freshness -> stale
	ReadDegraded      OwnerReadKind = "degraded_subset" // verified useful subset + named limitation -> degraded
	ReadNeedsSetup    OwnerReadKind = "needs_setup"     // owner readiness says optional setup allowed -> needs_setup
	ReadDisabled      OwnerReadKind = "disabled"        // explicit owner policy disables capability -> disabled
	ReadUnauthorized  OwnerReadKind = "unauthorized"    // session missing/expired/revoked/wrong purpose -> unauthorized
	ReadAccessDenied  OwnerReadKind = "access_denied"   // valid session lacks scope/role -> access_denied
	ReadNotFound      OwnerReadKind = "not_found"       // authorized lookup proves target absent -> not_found
	ReadFailed        OwnerReadKind = "failed"          // read/decode/schema/route/store/dependency failed -> error
)

func (k OwnerReadKind) valid() bool {
	_, ok := readKindToView[k]
	return ok
}

// readKindToView is the total, closed mapping from an abstract owner read
// outcome to the presented ViewState. There is NO entry that maps a failure or
// auth-loss kind to ready/first_use_empty/filtered_empty: failure can never
// become empty or success.
var readKindToView = map[OwnerReadKind]ViewState{
	ReadStarted:       ViewLoading,
	ReadPopulated:     ViewReady,
	ReadEmptyNoRecord: ViewFirstUseEmpty,
	ReadEmptyFiltered: ViewFilteredEmpty,
	ReadStale:         ViewStale,
	ReadDegraded:      ViewDegraded,
	ReadNeedsSetup:    ViewNeedsSetup,
	ReadDisabled:      ViewDisabled,
	ReadUnauthorized:  ViewUnauthorized,
	ReadAccessDenied:  ViewAccessDenied,
	ReadNotFound:      ViewNotFound,
	ReadFailed:        ViewError,
}

// OwnerReadOutcome is the ABSTRACT, content-free read outcome an owner passes
// across the seam. It carries NO raw content: only a typed Kind, a content-free
// Owner identity for a safe correlation reference, and OPTIONAL content-free
// LimitationCode / ActionCode tokens. It deliberately carries no Availability
// field — availability is a separate axis reached through PresentAvailability.
type OwnerReadOutcome struct {
	Kind           OwnerReadKind
	Owner          string
	LimitationCode string
	ActionCode     string
}

// F106PresentationError is the single typed, fail-closed presentation error for
// the spec 106 shared presenters (state, mutation, auth). It is returned for an
// unknown, malformed, contradictory, or unsafe owner outcome. Its Violations
// slice is deterministic so callers and tests can assert on it. A presenter
// NEVER downgrades one of these to empty, ready, available, or success.
type F106PresentationError struct {
	Surface    string // "state" | "mutation" | "auth"
	Violations []string
}

func (e *F106PresentationError) Error() string {
	return "F106PresentationError[" + e.Surface + "]: " + strings.Join(e.Violations, "; ")
}

// ExperienceStatePresenter maps abstract owner outcomes to the shared content
// ViewState and presents (never derives) the independent Availability axis.
type ExperienceStatePresenter struct{}

// PresentRead maps an abstract owner read outcome to a ViewState. It fails
// closed to *F106PresentationError when the Kind is unknown, when an optional
// code carries unsafe (non content-free) text, or when the outcome is
// contradictory (a degraded/stale outcome without a named limitation, or a
// successful populated/empty read that also names a limitation). It never
// converts a failure into empty or success and never inspects a raw error
// string.
func (ExperienceStatePresenter) PresentRead(o OwnerReadOutcome) (ViewState, error) {
	if !o.Kind.valid() {
		return "", &F106PresentationError{Surface: "state", Violations: []string{"unknown-read-kind:" + string(o.Kind)}}
	}

	var v []string
	// Redaction contract: an optional code, when present, must be content-free.
	if o.LimitationCode != "" && !safeCode(o.LimitationCode) {
		v = append(v, "unsafe-limitation-code")
	}
	if o.ActionCode != "" && !safeCode(o.ActionCode) {
		v = append(v, "unsafe-action-code")
	}
	// Contradiction contract (grounded in design.md content-state table):
	//   - degraded/stale claim useful-with-a-limitation, so a named limitation
	//     is required to be truthful;
	//   - a successful populated/empty read must NOT smuggle a limitation.
	switch o.Kind {
	case ReadDegraded, ReadStale:
		if o.LimitationCode == "" {
			v = append(v, "missing-limitation-for:"+string(o.Kind))
		}
	case ReadPopulated, ReadEmptyNoRecord, ReadEmptyFiltered:
		if o.LimitationCode != "" {
			v = append(v, "limitation-on-success:"+string(o.Kind))
		}
	}
	if len(v) > 0 {
		return "", &F106PresentationError{Surface: "state", Violations: v}
	}
	return readKindToView[o.Kind], nil
}

// PresentAvailability presents an availability value ONLY when its source is a
// resolved readiness fact. Any other source (a registered route, an enabled
// flag, an HTTP 200, a mounted handler, an empty array, a health probe) fails
// closed to *F106PresentationError: structural facts cannot create
// availability. An unknown Availability value also fails closed.
func (ExperienceStatePresenter) PresentAvailability(src AvailabilitySignal, a Availability) (Availability, error) {
	var v []string
	if src != SignalReadinessResolved {
		v = append(v, "availability-not-from-readiness:"+string(src))
	}
	if !a.valid() {
		v = append(v, "unknown-availability:"+string(a))
	}
	if len(v) > 0 {
		return "", &F106PresentationError{Surface: "state", Violations: v}
	}
	return a, nil
}

// safeCode reports whether s is a content-free presentation code: a short
// lowercase token of [a-z0-9_.-] only. It rejects whitespace, uppercase,
// punctuation, path separators, and any other shape that would indicate a raw
// error string, stack frame, secret, URL, query, or personal content reaching a
// presented value. It is the mechanical core of the redaction contract shared
// by every spec 106 presenter.
func safeCode(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '-':
		default:
			return false
		}
	}
	return true
}
