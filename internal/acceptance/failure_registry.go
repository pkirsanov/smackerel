// failure_registry.go declares the closed BUG-102-001-owned product-journey
// acceptance vocabularies — the aggregate verdicts, the per-journey outcomes,
// and the full E102-JOURNEY-* failure-code registry — that the manifest, result
// validator, verdict reducer, and the sibling production read-only guard
// (read_only_guard.go) consume. Every set is CLOSED and fail-closed: an
// unknown, duplicate, ownerless, or category-mismatched code makes the registry
// contract-invalid, and an unknown verdict, outcome, or code lookup returns
// not-ok rather than a guessed default.
//
// The closed E102-JOURNEY-* families and their one-category/one-owner mapping
// are taken from the design's "## Closed Failure-Code Registry" table in
// specs/102-target-deploy-hardening/bugs/BUG-102-001-product-journey-acceptance-gap/design.md.
// Category is the design error family; Owner is the single owning remediation
// role — the owning journey group for a product-journey family, or the
// acceptance-contract meta-owner for the contract-integrity family that this
// packet owns.

package acceptance

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// FailureCode is a closed E102-JOURNEY-* failure-code string. Only the codes in
// the canonical registry are valid; any other value is contract-invalid.
type FailureCode string

// FailureCategory is the closed error family a failure code belongs to.
type FailureCategory string

// FailureOwner is the single owning remediation role responsible for a failure
// code — the owning product-journey group, or the acceptance-contract meta-owner
// for the contract-integrity family.
type FailureOwner string

// AggregateVerdict is a closed aggregate acceptance verdict.
type AggregateVerdict string

// JourneyOutcome is a closed per-journey outcome.
type JourneyOutcome string

// Closed failure categories (the design's error families).
const (
	CategoryAuth             FailureCategory = "auth"
	CategorySearch           FailureCategory = "search"
	CategoryDigest           FailureCategory = "digest"
	CategoryAssistant        FailureCategory = "assistant"
	CategoryGraph            FailureCategory = "graph"
	CategoryRecommendations  FailureCategory = "recommendations"
	CategoryCards            FailureCategory = "cards"
	CategoryNotifications    FailureCategory = "notifications"
	CategoryCapabilityStatus FailureCategory = "capability-status"
	CategoryPhotos           FailureCategory = "photos"
	CategoryConnectors       FailureCategory = "connectors"
	CategoryModels           FailureCategory = "models"
	CategorySynthesis        FailureCategory = "synthesis"
	CategoryContract         FailureCategory = "contract"
)

// Closed owning remediation roles. Each product-journey family is owned by its
// journey group; the contract-integrity family is owned by this acceptance
// capability itself.
const (
	OwnerSession            FailureOwner = "session"
	OwnerSearch             FailureOwner = "search"
	OwnerDigest             FailureOwner = "digest"
	OwnerAssistant          FailureOwner = "assistant"
	OwnerGraph              FailureOwner = "graph"
	OwnerRecommendations    FailureOwner = "recommendations"
	OwnerCards              FailureOwner = "cards"
	OwnerNotifications      FailureOwner = "notifications"
	OwnerCapabilityStatus   FailureOwner = "capability-status"
	OwnerPhotos             FailureOwner = "photos"
	OwnerConnectors         FailureOwner = "connectors"
	OwnerModels             FailureOwner = "models"
	OwnerSynthesis          FailureOwner = "synthesis"
	OwnerAcceptanceContract FailureOwner = "acceptance-contract"
)

// Closed aggregate verdicts.
const (
	VerdictAccepted            AggregateVerdict = "accepted"
	VerdictAcceptedDegraded    AggregateVerdict = "accepted-degraded"
	VerdictBlockedPrerequisite AggregateVerdict = "blocked-prerequisite"
	VerdictRejected            AggregateVerdict = "rejected"
	VerdictContractInvalid     AggregateVerdict = "contract-invalid"
	VerdictTimedOut            AggregateVerdict = "timed-out"
)

// Closed per-journey outcomes.
const (
	OutcomePassed          JourneyOutcome = "passed"
	OutcomeAllowedEmpty    JourneyOutcome = "allowed-empty"
	OutcomeAllowedQuiet    JourneyOutcome = "allowed-quiet"
	OutcomeAllowedOptional JourneyOutcome = "allowed-optional"
	OutcomeAllowedDegraded JourneyOutcome = "allowed-degraded"
	OutcomeFailed          JourneyOutcome = "failed"
	OutcomeBlocked         JourneyOutcome = "blocked"
	OutcomeTimedOut        JourneyOutcome = "timed-out"
	OutcomeNotEvaluated    JourneyOutcome = "not-evaluated"
)

// Named contract-integrity failure codes the read-only guard emits. They are
// declared here so the guard and the registry share one source of truth.
const (
	// CodeContractUnsafeMutation flags a production write, a state-changing
	// selector, an unclassified or mutating request, request interception,
	// credential/cookie injection, a direct datastore read, or a
	// service-container exec in a production-readonly surface.
	CodeContractUnsafeMutation FailureCode = "E102-JOURNEY-CONTRACT-UNSAFE-MUTATION"
	// CodeContractEvidenceUnsafe flags a concrete target literal or a forbidden
	// result/evidence field in a production-readonly surface.
	CodeContractEvidenceUnsafe FailureCode = "E102-JOURNEY-CONTRACT-EVIDENCE-UNSAFE"
)

// FailureMeta is the closed one-category/one-owner metadata for a failure code.
type FailureMeta struct {
	Category FailureCategory
	Owner    FailureOwner
}

// FailureDecl is one declared registry row: a code with its category and owner.
// It is the input to registry validation so a duplicate, ownerless, or
// category-mismatched declaration is detectable before the code map is built.
type FailureDecl struct {
	Code     FailureCode
	Category FailureCategory
	Owner    FailureOwner
}

// ErrFailureRegistryInvalid is the fail-closed sentinel returned whenever the
// failure registry is contract-invalid: an empty set, an empty or malformed
// code, an unknown category or owner, a duplicate code, a code whose declared
// category contradicts its E102-JOURNEY family prefix, or a code whose owner is
// not its category's canonical owner. It corresponds to the VerdictContractInvalid
// aggregate verdict.
var ErrFailureRegistryInvalid = errors.New("failure registry: contract-invalid")

// closedCategories is the closed set of valid failure categories.
var closedCategories = map[FailureCategory]bool{
	CategoryAuth:             true,
	CategorySearch:           true,
	CategoryDigest:           true,
	CategoryAssistant:        true,
	CategoryGraph:            true,
	CategoryRecommendations:  true,
	CategoryCards:            true,
	CategoryNotifications:    true,
	CategoryCapabilityStatus: true,
	CategoryPhotos:           true,
	CategoryConnectors:       true,
	CategoryModels:           true,
	CategorySynthesis:        true,
	CategoryContract:         true,
}

// closedOwners is the closed set of valid owning remediation roles.
var closedOwners = map[FailureOwner]bool{
	OwnerSession:            true,
	OwnerSearch:             true,
	OwnerDigest:             true,
	OwnerAssistant:          true,
	OwnerGraph:              true,
	OwnerRecommendations:    true,
	OwnerCards:              true,
	OwnerNotifications:      true,
	OwnerCapabilityStatus:   true,
	OwnerPhotos:             true,
	OwnerConnectors:         true,
	OwnerModels:             true,
	OwnerSynthesis:          true,
	OwnerAcceptanceContract: true,
}

// categoryToOwner binds each category to its single canonical owner. A declared
// code whose owner is not its category's canonical owner is contract-invalid.
var categoryToOwner = map[FailureCategory]FailureOwner{
	CategoryAuth:             OwnerSession,
	CategorySearch:           OwnerSearch,
	CategoryDigest:           OwnerDigest,
	CategoryAssistant:        OwnerAssistant,
	CategoryGraph:            OwnerGraph,
	CategoryRecommendations:  OwnerRecommendations,
	CategoryCards:            OwnerCards,
	CategoryNotifications:    OwnerNotifications,
	CategoryCapabilityStatus: OwnerCapabilityStatus,
	CategoryPhotos:           OwnerPhotos,
	CategoryConnectors:       OwnerConnectors,
	CategoryModels:           OwnerModels,
	CategorySynthesis:        OwnerSynthesis,
	CategoryContract:         OwnerAcceptanceContract,
}

// closedVerdicts is the closed set of valid aggregate verdicts.
var closedVerdicts = map[AggregateVerdict]bool{
	VerdictAccepted:            true,
	VerdictAcceptedDegraded:    true,
	VerdictBlockedPrerequisite: true,
	VerdictRejected:            true,
	VerdictContractInvalid:     true,
	VerdictTimedOut:            true,
}

// closedJourneyOutcomes is the closed set of valid per-journey outcomes.
var closedJourneyOutcomes = map[JourneyOutcome]bool{
	OutcomePassed:          true,
	OutcomeAllowedEmpty:    true,
	OutcomeAllowedQuiet:    true,
	OutcomeAllowedOptional: true,
	OutcomeAllowedDegraded: true,
	OutcomeFailed:          true,
	OutcomeBlocked:         true,
	OutcomeTimedOut:        true,
	OutcomeNotEvaluated:    true,
}

// categoryPrefix binds a category to the exact E102-JOURNEY-* code prefix every
// code in that category must carry. The list is ordered so the more specific
// capability prefix is tested before the contract prefix it shares a stem with;
// categoryForCode relies on this order.
type categoryPrefix struct {
	prefix   string
	category FailureCategory
}

// orderedCategoryPrefixes derives a code's category from its family prefix. The
// capability family ("E102-JOURNEY-CONTRACT-CAPABILITY-") MUST precede the
// contract family ("E102-JOURNEY-CONTRACT-") so a capability code is never
// misread as a contract code. No other family prefix is a prefix of another.
var orderedCategoryPrefixes = []categoryPrefix{
	{"E102-JOURNEY-CONTRACT-CAPABILITY-", CategoryCapabilityStatus},
	{"E102-JOURNEY-AUTH-", CategoryAuth},
	{"E102-JOURNEY-SEARCH-", CategorySearch},
	{"E102-JOURNEY-DIGEST-", CategoryDigest},
	{"E102-JOURNEY-ASSISTANT-", CategoryAssistant},
	{"E102-JOURNEY-GRAPH-", CategoryGraph},
	{"E102-JOURNEY-RECOMMEND-", CategoryRecommendations},
	{"E102-JOURNEY-CARDS-", CategoryCards},
	{"E102-JOURNEY-NOTIFICATIONS-", CategoryNotifications},
	{"E102-JOURNEY-PHOTOS-", CategoryPhotos},
	{"E102-JOURNEY-CONNECTORS-", CategoryConnectors},
	{"E102-JOURNEY-MODELS-", CategoryModels},
	{"E102-JOURNEY-SYNTH-", CategorySynthesis},
	{"E102-JOURNEY-CONTRACT-", CategoryContract},
}

// categoryForCode returns the closed category implied by a code's family prefix,
// or (_, false) when the code matches no closed family. It gives the registry an
// independent second opinion on every code's category so a declaration filed
// under the wrong category is caught (category-mismatch detection).
func categoryForCode(code FailureCode) (FailureCategory, bool) {
	s := string(code)
	for _, cp := range orderedCategoryPrefixes {
		if strings.HasPrefix(s, cp.prefix) && len(s) > len(cp.prefix) {
			return cp.category, true
		}
	}
	return "", false
}

// canonicalFailureFamily groups a family's codes with their shared category and
// owner so the canonical declaration lists each code exactly once.
type canonicalFailureFamily struct {
	category FailureCategory
	owner    FailureOwner
	codes    []FailureCode
}

// canonicalFailureDecls is the immutable, closed E102-JOURNEY-* code set from
// design.md, each mapped to exactly one category and one owner. It is the sole
// source of truth for the canonical registry.
func canonicalFailureDecls() []FailureDecl {
	families := []canonicalFailureFamily{
		{CategoryAuth, OwnerSession, []FailureCode{
			"E102-JOURNEY-AUTH-LOGIN",
			"E102-JOURNEY-AUTH-COOKIE",
			"E102-JOURNEY-AUTH-REUSE",
			"E102-JOURNEY-AUTH-EXPIRED",
			"E102-JOURNEY-AUTH-AUTHZ",
			"E102-JOURNEY-AUTH-PRIVACY",
		}},
		{CategorySearch, OwnerSearch, []FailureCode{
			"E102-JOURNEY-SEARCH-NO-REQUEST",
			"E102-JOURNEY-SEARCH-DUPLICATE-REQUEST",
			"E102-JOURNEY-SEARCH-HTTP",
			"E102-JOURNEY-SEARCH-SCHEMA",
			"E102-JOURNEY-SEARCH-FALSE-EMPTY",
			"E102-JOURNEY-SEARCH-DOM",
			"E102-JOURNEY-SEARCH-A11Y",
			"E102-JOURNEY-SEARCH-TIMEOUT",
		}},
		{CategoryDigest, OwnerDigest, []FailureCode{
			"E102-JOURNEY-DIGEST-HTTP",
			"E102-JOURNEY-DIGEST-SCHEMA",
			"E102-JOURNEY-DIGEST-FALSE-EMPTY",
			"E102-JOURNEY-DIGEST-STALE",
			"E102-JOURNEY-DIGEST-DOM",
			"E102-JOURNEY-DIGEST-A11Y",
			"E102-JOURNEY-DIGEST-TIMEOUT",
		}},
		{CategoryAssistant, OwnerAssistant, []FailureCode{
			"E102-JOURNEY-ASSISTANT-DEPENDENCY",
			"E102-JOURNEY-ASSISTANT-HTTP",
			"E102-JOURNEY-ASSISTANT-SCHEMA",
			"E102-JOURNEY-ASSISTANT-BLANK",
			"E102-JOURNEY-ASSISTANT-FALSE-CAPTURE",
			"E102-JOURNEY-ASSISTANT-NO-RETRY",
			"E102-JOURNEY-ASSISTANT-FRESHNESS",
			"E102-JOURNEY-ASSISTANT-A11Y",
			"E102-JOURNEY-ASSISTANT-TIMEOUT",
		}},
		{CategoryGraph, OwnerGraph, []FailureCode{
			"E102-JOURNEY-GRAPH-DEPENDENCY",
			"E102-JOURNEY-GRAPH-ROUTE",
			"E102-JOURNEY-GRAPH-HTTP",
			"E102-JOURNEY-GRAPH-QUERY",
			"E102-JOURNEY-GRAPH-SCHEMA",
			"E102-JOURNEY-GRAPH-FALSE-EMPTY",
			"E102-JOURNEY-GRAPH-PARTIAL",
			"E102-JOURNEY-GRAPH-UNBOUNDED",
			"E102-JOURNEY-GRAPH-PROJECTION",
			"E102-JOURNEY-GRAPH-FRESHNESS",
			"E102-JOURNEY-GRAPH-DOM",
			"E102-JOURNEY-GRAPH-A11Y",
			"E102-JOURNEY-GRAPH-TIMEOUT",
		}},
		{CategoryRecommendations, OwnerRecommendations, []FailureCode{
			"E102-JOURNEY-RECOMMEND-DEPENDENCY",
			"E102-JOURNEY-RECOMMEND-ZERO-PROVIDER",
			"E102-JOURNEY-RECOMMEND-STALE",
			"E102-JOURNEY-RECOMMEND-MISMATCH",
			"E102-JOURNEY-RECOMMEND-DOM",
			"E102-JOURNEY-RECOMMEND-TIMEOUT",
		}},
		{CategoryCards, OwnerCards, []FailureCode{
			"E102-JOURNEY-CARDS-DEPENDENCY",
			"E102-JOURNEY-CARDS-HTTP",
			"E102-JOURNEY-CARDS-SCHEMA",
			"E102-JOURNEY-CARDS-FALSE-EMPTY",
			"E102-JOURNEY-CARDS-STALE",
			"E102-JOURNEY-CARDS-DOM",
			"E102-JOURNEY-CARDS-A11Y",
			"E102-JOURNEY-CARDS-TIMEOUT",
		}},
		{CategoryNotifications, OwnerNotifications, []FailureCode{
			"E102-JOURNEY-NOTIFICATIONS-HTTP",
			"E102-JOURNEY-NOTIFICATIONS-SCHEMA",
			"E102-JOURNEY-NOTIFICATIONS-FALSE-EMPTY",
			"E102-JOURNEY-NOTIFICATIONS-STALE",
			"E102-JOURNEY-NOTIFICATIONS-DOM",
			"E102-JOURNEY-NOTIFICATIONS-A11Y",
			"E102-JOURNEY-NOTIFICATIONS-TIMEOUT",
		}},
		{CategoryCapabilityStatus, OwnerCapabilityStatus, []FailureCode{
			"E102-JOURNEY-CONTRACT-CAPABILITY-POLICY",
			"E102-JOURNEY-CONTRACT-CAPABILITY-STATUS-MISMATCH",
			"E102-JOURNEY-CONTRACT-CAPABILITY-STALE",
			"E102-JOURNEY-CONTRACT-CAPABILITY-DOM",
			"E102-JOURNEY-CONTRACT-CAPABILITY-A11Y",
		}},
		{CategoryPhotos, OwnerPhotos, []FailureCode{
			"E102-JOURNEY-PHOTOS-HTTP",
			"E102-JOURNEY-PHOTOS-SCHEMA",
			"E102-JOURNEY-PHOTOS-FALSE-EMPTY",
			"E102-JOURNEY-PHOTOS-LIMITATION",
			"E102-JOURNEY-PHOTOS-STALE",
			"E102-JOURNEY-PHOTOS-DOM",
			"E102-JOURNEY-PHOTOS-A11Y",
			"E102-JOURNEY-PHOTOS-TIMEOUT",
		}},
		{CategoryConnectors, OwnerConnectors, []FailureCode{
			"E102-JOURNEY-CONNECTORS-HTTP",
			"E102-JOURNEY-CONNECTORS-SCHEMA",
			"E102-JOURNEY-CONNECTORS-FALSE-EMPTY",
			"E102-JOURNEY-CONNECTORS-STALE",
			"E102-JOURNEY-CONNECTORS-DOM",
			"E102-JOURNEY-CONNECTORS-A11Y",
			"E102-JOURNEY-CONNECTORS-TIMEOUT",
		}},
		{CategoryModels, OwnerModels, []FailureCode{
			"E102-JOURNEY-MODELS-AUTHZ",
			"E102-JOURNEY-MODELS-HTTP",
			"E102-JOURNEY-MODELS-SCHEMA",
			"E102-JOURNEY-MODELS-FALSE-READY",
			"E102-JOURNEY-MODELS-STALE",
			"E102-JOURNEY-MODELS-DOM",
			"E102-JOURNEY-MODELS-A11Y",
			"E102-JOURNEY-MODELS-TIMEOUT",
		}},
		{CategorySynthesis, OwnerSynthesis, []FailureCode{
			"E102-JOURNEY-SYNTH-DEPENDENCY",
			"E102-JOURNEY-SYNTH-NEVER-RUN-UP",
			"E102-JOURNEY-SYNTH-NO-DURABLE-OUTPUT",
			"E102-JOURNEY-SYNTH-STALE",
			"E102-JOURNEY-SYNTH-DOM",
			"E102-JOURNEY-SYNTH-TIMEOUT",
		}},
		{CategoryContract, OwnerAcceptanceContract, []FailureCode{
			"E102-JOURNEY-CONTRACT-MISSING",
			"E102-JOURNEY-CONTRACT-UNSUPPORTED",
			"E102-JOURNEY-CONTRACT-MALFORMED",
			"E102-JOURNEY-CONTRACT-RELEASE-MISMATCH",
			"E102-JOURNEY-CONTRACT-MANIFEST-MISMATCH",
			"E102-JOURNEY-CONTRACT-POLICY-MISMATCH",
			"E102-JOURNEY-CONTRACT-MISSING-JOURNEY",
			"E102-JOURNEY-CONTRACT-DUPLICATE-JOURNEY",
			"E102-JOURNEY-CONTRACT-UNKNOWN-ENUM",
			CodeContractUnsafeMutation,
			CodeContractEvidenceUnsafe,
			"E102-JOURNEY-CONTRACT-IDENTITY-MISSING",
			"E102-JOURNEY-CONTRACT-FIXTURE-MISSING",
			"E102-JOURNEY-CONTRACT-STALE-RESULT",
			"E102-JOURNEY-CONTRACT-STEP-TIMEOUT",
			"E102-JOURNEY-CONTRACT-OVERALL-TIMEOUT",
			"E102-JOURNEY-CONTRACT-SIGNATURE",
		}},
	}
	var decls []FailureDecl
	for _, f := range families {
		for _, c := range f.codes {
			decls = append(decls, FailureDecl{Code: c, Category: f.category, Owner: f.owner})
		}
	}
	return decls
}

// FailureRegistry is the immutable, closed E102-JOURNEY-* code registry. It is
// built from an ordered declaration slice so Validate can detect a duplicate
// code before the lookup map exists; LookupFailure serves only a validated
// registry.
type FailureRegistry struct {
	decls  []FailureDecl
	byCode map[FailureCode]FailureMeta
}

// newFailureRegistry wraps a declaration slice without validating it, so a test
// can inject an adversarial (duplicate, ownerless, or mismatched) declaration
// set and then observe Validate fail closed.
func newFailureRegistry(decls []FailureDecl) *FailureRegistry {
	return &FailureRegistry{decls: decls}
}

// NewFailureRegistry builds and validates a registry from a declaration slice,
// returning ErrFailureRegistryInvalid (contract-invalid) on any violation.
func NewFailureRegistry(decls []FailureDecl) (*FailureRegistry, error) {
	r := newFailureRegistry(decls)
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

// DefaultFailureRegistry returns the validated canonical registry. It returns
// ErrFailureRegistryInvalid only if the canonical declarations are internally
// inconsistent (a programming error), so callers still fail closed.
func DefaultFailureRegistry() (*FailureRegistry, error) {
	return NewFailureRegistry(canonicalFailureDecls())
}

// Validate fails closed on any contract violation and, on success, builds the
// code lookup map. Every declared code must be non-empty, carry a known closed
// category and owner, match its E102-JOURNEY family prefix (category-mismatch
// detection), be owned by its category's canonical owner (owner-mismatch
// detection), and appear exactly once (duplicate detection).
func (r *FailureRegistry) Validate() error {
	if len(r.decls) == 0 {
		return fmt.Errorf("%w: empty failure registry", ErrFailureRegistryInvalid)
	}
	seen := make(map[FailureCode]bool, len(r.decls))
	built := make(map[FailureCode]FailureMeta, len(r.decls))
	for _, d := range r.decls {
		if strings.TrimSpace(string(d.Code)) == "" {
			return fmt.Errorf("%w: empty failure code", ErrFailureRegistryInvalid)
		}
		if seen[d.Code] {
			return fmt.Errorf("%w: duplicate failure code %q", ErrFailureRegistryInvalid, d.Code)
		}
		seen[d.Code] = true
		if !closedCategories[d.Category] {
			return fmt.Errorf("%w: code %q declares unknown category %q", ErrFailureRegistryInvalid, d.Code, d.Category)
		}
		if !closedOwners[d.Owner] {
			return fmt.Errorf("%w: code %q has unknown or missing owner %q", ErrFailureRegistryInvalid, d.Code, d.Owner)
		}
		derived, ok := categoryForCode(d.Code)
		if !ok {
			return fmt.Errorf("%w: code %q matches no closed E102-JOURNEY family prefix", ErrFailureRegistryInvalid, d.Code)
		}
		if derived != d.Category {
			return fmt.Errorf("%w: code %q declares category %q but its family prefix implies %q",
				ErrFailureRegistryInvalid, d.Code, d.Category, derived)
		}
		if want := categoryToOwner[d.Category]; d.Owner != want {
			return fmt.Errorf("%w: code %q category %q must be owned by %q, got %q",
				ErrFailureRegistryInvalid, d.Code, d.Category, want, d.Owner)
		}
		built[d.Code] = FailureMeta{Category: d.Category, Owner: d.Owner}
	}
	r.byCode = built
	return nil
}

// LookupFailure returns the closed metadata for a code and true, or the zero
// metadata and false for an unknown code. It never guesses a default and never
// serves an unvalidated registry (a nil lookup map returns not-ok).
func (r *FailureRegistry) LookupFailure(code FailureCode) (FailureMeta, bool) {
	meta, ok := r.byCode[code]
	return meta, ok
}

// Codes returns the sorted list of registered failure codes in a validated
// registry, or nil when the registry has not been validated.
func (r *FailureRegistry) Codes() []FailureCode {
	if r.byCode == nil {
		return nil
	}
	codes := make([]FailureCode, 0, len(r.byCode))
	for c := range r.byCode {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	return codes
}

// IsClosedVerdict reports whether v is a valid closed aggregate verdict. An
// unknown value is not-ok, never a guessed default.
func IsClosedVerdict(v AggregateVerdict) bool { return closedVerdicts[v] }

// IsClosedJourneyOutcome reports whether o is a valid closed per-journey
// outcome. An unknown value is not-ok, never a guessed default.
func IsClosedJourneyOutcome(o JourneyOutcome) bool { return closedJourneyOutcomes[o] }

// IsClosedCategory reports whether c is a valid closed failure category.
func IsClosedCategory(c FailureCategory) bool { return closedCategories[c] }

// IsClosedOwner reports whether o is a valid closed owning remediation role.
func IsClosedOwner(o FailureOwner) bool { return closedOwners[o] }
