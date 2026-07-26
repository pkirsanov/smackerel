// manifest.go declares the BUG-102-001 product-owned journey manifest
// (apiVersion smackerel.io/product-journeys/v1) and its fail-closed compiler.
// The manifest is the single source of truth for which product journeys exist,
// which are required, which routes/side-effects/assertions/failure-codes each
// declares, and which dependency evidence each is gated on. Compile() turns the
// declarative manifest plus an explicit compiled train/environment policy into
// an immutable CompiledAcceptancePolicy that the runner, verdict reducer, and
// result validator consume.
//
// Every check is fail-closed with no default and no fallback: a missing
// declared journey, a dropped dependency or assertion, an implicit (unset)
// requiredness, an unknown/duplicate/mismatched enum or E102-JOURNEY-* code, an
// unresolvable timeout/freshness policy reference, a health-only "success", or a
// production-unsafe field each abort compilation with one closed
// E102-JOURNEY-CONTRACT-* code (see failure_registry.go). The closed vocabularies
// and the reused route/side-effect static guard come from failure_registry.go
// and read_only_guard.go; this file never redefines them.

package acceptance

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Manifest/result API versions this package implements. A manifest or result
// carrying any other version is unsupported.
const (
	manifestAPIVersion = "smackerel.io/product-journeys/v1"
	manifestKind       = "ProductJourneyManifest"
	resultSchema       = "smackerel.io/product-acceptance-result/v1"
)

// Named E102-JOURNEY-CONTRACT-* codes the manifest compiler, result validator,
// verdict reducer, and evidence sanitizer emit. Each value is a registered
// canonical code from failure_registry.go; the two guard codes are reused from
// there rather than redeclared. Defining them once here keeps the contract
// layer's call sites readable without a second source of truth.
const (
	CodeMissing          FailureCode = "E102-JOURNEY-CONTRACT-MISSING"
	CodeUnsupported      FailureCode = "E102-JOURNEY-CONTRACT-UNSUPPORTED"
	CodeMalformed        FailureCode = "E102-JOURNEY-CONTRACT-MALFORMED"
	CodeReleaseMismatch  FailureCode = "E102-JOURNEY-CONTRACT-RELEASE-MISMATCH"
	CodeManifestMismatch FailureCode = "E102-JOURNEY-CONTRACT-MANIFEST-MISMATCH"
	CodePolicyMismatch   FailureCode = "E102-JOURNEY-CONTRACT-POLICY-MISMATCH"
	CodeMissingJourney   FailureCode = "E102-JOURNEY-CONTRACT-MISSING-JOURNEY"
	CodeDuplicateJourney FailureCode = "E102-JOURNEY-CONTRACT-DUPLICATE-JOURNEY"
	CodeUnknownEnum      FailureCode = "E102-JOURNEY-CONTRACT-UNKNOWN-ENUM"
	CodeIdentityMissing  FailureCode = "E102-JOURNEY-CONTRACT-IDENTITY-MISSING"
	CodeFixtureMissing   FailureCode = "E102-JOURNEY-CONTRACT-FIXTURE-MISSING"
	CodeStaleResult      FailureCode = "E102-JOURNEY-CONTRACT-STALE-RESULT"
	CodeStepTimeout      FailureCode = "E102-JOURNEY-CONTRACT-STEP-TIMEOUT"
	CodeOverallTimeout   FailureCode = "E102-JOURNEY-CONTRACT-OVERALL-TIMEOUT"
	CodeSignature        FailureCode = "E102-JOURNEY-CONTRACT-SIGNATURE"

	// CodeUnsafeMutation and CodeEvidenceUnsafe alias the two guard codes already
	// declared in failure_registry.go so the contract layer shares one source of
	// truth for them.
	CodeUnsafeMutation = CodeContractUnsafeMutation
	CodeEvidenceUnsafe = CodeContractEvidenceUnsafe
)

// Mode is a closed execution mode. Only production-readonly evidence satisfies
// deploy acceptance; seeded-validate evidence is validation-only.
type Mode string

const (
	// ModeSeededValidate runs against a disposable env=test* stack and may create
	// fixture-owned records; its evidence is validation-only.
	ModeSeededValidate Mode = "seeded-validate"
	// ModeProductionReadonly runs read-only against the deployed release; it is
	// the only mode accepted by deployment promotion.
	ModeProductionReadonly Mode = "production-readonly"
)

var closedModes = map[Mode]bool{
	ModeSeededValidate:     true,
	ModeProductionReadonly: true,
}

// Requiredness is a closed per-journey requiredness. It MUST be declared
// explicitly on every manifest journey; an empty value is implicit requiredness
// and fails compilation.
type Requiredness string

const (
	RequirednessRequired          Requiredness = "required"
	RequirednessOptional          Requiredness = "optional"
	RequirednessDependencyBlocked Requiredness = "dependency-blocked"
	RequirednessNotApplicable     Requiredness = "not-applicable"
)

var closedRequiredness = map[Requiredness]bool{
	RequirednessRequired:          true,
	RequirednessOptional:          true,
	RequirednessDependencyBlocked: true,
	RequirednessNotApplicable:     true,
}

// JourneyGroup is a closed product journey group.
type JourneyGroup string

const (
	GroupSession          JourneyGroup = "session"
	GroupSearch           JourneyGroup = "search"
	GroupDigest           JourneyGroup = "digest"
	GroupAssistant        JourneyGroup = "assistant"
	GroupWiki             JourneyGroup = "wiki"
	GroupGraph            JourneyGroup = "graph"
	GroupCards            JourneyGroup = "cards"
	GroupRecommendations  JourneyGroup = "recommendations"
	GroupNotifications    JourneyGroup = "notifications"
	GroupCapabilityStatus JourneyGroup = "capability-status"
	GroupPhotos           JourneyGroup = "photos"
	GroupConnectors       JourneyGroup = "connectors"
	GroupModels           JourneyGroup = "models"
	GroupSynthesis        JourneyGroup = "synthesis"
)

// closedJourneyGroups is the closed set of journey groups the manifest must
// cover in full. Coverage of every group is required (see CoveredGroups).
var closedJourneyGroups = map[JourneyGroup]bool{
	GroupSession:          true,
	GroupSearch:           true,
	GroupDigest:           true,
	GroupAssistant:        true,
	GroupWiki:             true,
	GroupGraph:            true,
	GroupCards:            true,
	GroupRecommendations:  true,
	GroupNotifications:    true,
	GroupCapabilityStatus: true,
	GroupPhotos:           true,
	GroupConnectors:       true,
	GroupModels:           true,
	GroupSynthesis:        true,
}

// groupToCategory binds each journey group to the failure-registry category its
// declared failure codes must belong to. Wiki shares the graph category, and
// capability-status maps to the capability-status category whose codes carry the
// E102-JOURNEY-CONTRACT-CAPABILITY-* prefix.
var groupToCategory = map[JourneyGroup]FailureCategory{
	GroupSession:          CategoryAuth,
	GroupSearch:           CategorySearch,
	GroupDigest:           CategoryDigest,
	GroupAssistant:        CategoryAssistant,
	GroupWiki:             CategoryGraph,
	GroupGraph:            CategoryGraph,
	GroupCards:            CategoryCards,
	GroupRecommendations:  CategoryRecommendations,
	GroupNotifications:    CategoryNotifications,
	GroupCapabilityStatus: CategoryCapabilityStatus,
	GroupPhotos:           CategoryPhotos,
	GroupConnectors:       CategoryConnectors,
	GroupModels:           CategoryModels,
	GroupSynthesis:        CategorySynthesis,
}

// Audience is a closed journey audience.
type Audience string

const (
	AudienceAuthenticatedUser Audience = "authenticated-user"
	AudienceOperator          Audience = "operator"
)

var closedAudiences = map[Audience]bool{
	AudienceAuthenticatedUser: true,
	AudienceOperator:          true,
}

// Plane is a closed assertion/step plane.
type Plane string

const (
	PlaneBrowser        Plane = "browser"
	PlaneBrowserNetwork Plane = "browser-network"
	PlaneAPI            Plane = "api"
	PlaneTelemetry      Plane = "telemetry"
	PlaneAccessibility  Plane = "accessibility"
)

var closedPlanes = map[Plane]bool{
	PlaneBrowser:        true,
	PlaneBrowserNetwork: true,
	PlaneAPI:            true,
	PlaneTelemetry:      true,
	PlaneAccessibility:  true,
}

// DataSelection is a closed data-selection class.
type DataSelection string

const (
	DataExistingAuthorized DataSelection = "existing-authorized-records"
	DataFixtureSet         DataSelection = "fixture-set"
	DataNone               DataSelection = "none"
)

var closedDataSelections = map[DataSelection]bool{
	DataExistingAuthorized: true,
	DataFixtureSet:         true,
	DataNone:               true,
}

// closedDependencyEvidenceClasses is the closed set of dependency evidence
// classes a journey may require of an owning packet.
var closedDependencyEvidenceClasses = map[string]bool{
	"certified-current-journey":  true,
	"scope-bounded-evidence":     true,
	"activated-runtime-contract": true,
	"none-required":              true,
}

// DependencyRef declares a delivery dependency on an owning packet and the
// evidence class that dependency must provide. Both fields are required.
type DependencyRef struct {
	Packet        string
	EvidenceClass string
}

// ManifestStep is one declared journey step: a plane, an HTTP method, a
// normalized route template, its closed side-effect class, and the exact set of
// accepted statuses. No field has a default.
type ManifestStep struct {
	ID             string
	Plane          Plane
	Method         string
	Route          string
	SideEffect     SideEffectClass
	ExpectedStatus []int
}

// JourneyAssertions declares the closed assertion planes a journey proves. A
// required or browser journey must declare a status assertion, at least one
// schema-or-DOM assertion, and at least one accessibility assertion; dropping
// any of these fails compilation.
type JourneyAssertions struct {
	Status        []string
	Schema        []string
	DOM           []string
	Accessibility []string
	Telemetry     []string
	Freshness     []string
}

// ManifestJourney is one declared product journey. Requiredness is explicit
// (never inferred), assertions and dependencies are complete, and the selectors
// and evidence fields feed the reused production read-only static guard.
type ManifestJourney struct {
	ID              string
	Group           JourneyGroup
	Audience        Audience
	Requiredness    Requiredness
	AllowedOutcomes []JourneyOutcome
	Dependencies    []DependencyRef
	DataMode        map[Mode]DataSelection
	Steps           []ManifestStep
	Assertions      JourneyAssertions
	Selectors       []string
	EvidenceFields  []string
	TimeoutRef      string
	FreshnessRef    string
	FailureCodes    []FailureCode
}

// ProductJourneyManifest is the product-owned manifest root.
type ProductJourneyManifest struct {
	APIVersion       string
	Kind             string
	ManifestID       string
	ManifestRevision int
	ResultSchema     string
	SupportedModes   []Mode
	PolicyRefs       map[string]string
	Journeys         []ManifestJourney
}

// CompiledPolicyConfig is the explicit compiled train/environment policy the
// manifest is compiled against. Timeouts and freshness are resolved by policy
// reference with NO fallback: an unresolvable reference aborts compilation.
type CompiledPolicyConfig struct {
	Train       string
	Environment string
	Timeouts    map[string]time.Duration
	Freshness   map[string]time.Duration
}

// CompiledJourney is one resolved journey in a CompiledAcceptancePolicy.
type CompiledJourney struct {
	ID              string
	Group           JourneyGroup
	Audience        Audience
	Requiredness    Requiredness
	AllowedOutcomes map[JourneyOutcome]bool
	Dependencies    []DependencyRef
	Steps           []ManifestStep
	FailureCodes    []FailureCode
	Timeout         time.Duration
	Freshness       time.Duration
}

// CompiledAcceptancePolicy is the immutable compiled policy the reducer and
// validator consume. It is produced only by a fully-validated Compile().
type CompiledAcceptancePolicy struct {
	ManifestID       string
	ManifestRevision int
	ResultSchema     string
	SupportedModes   []Mode
	Journeys         []CompiledJourney
	byID             map[string]CompiledJourney
	groups           map[JourneyGroup]bool
}

// Journey returns the compiled journey with the given ID and true, or the zero
// journey and false. It never guesses a default.
func (p CompiledAcceptancePolicy) Journey(id string) (CompiledJourney, bool) {
	j, ok := p.byID[id]
	return j, ok
}

// CoversGroup reports whether the compiled policy contains a journey in group g.
func (p CompiledAcceptancePolicy) CoversGroup(g JourneyGroup) bool { return p.groups[g] }

// ContractError is a fail-closed contract violation carrying exactly one closed
// E102-JOURNEY-CONTRACT-* code and a value-safe reason. It never echoes a raw
// secret, credential, or target literal.
type ContractError struct {
	Code   FailureCode
	Reason string
}

// Error implements error.
func (e *ContractError) Error() string {
	return fmt.Sprintf("acceptance contract: %s: %s", e.Code, e.Reason)
}

// contractErr builds a *ContractError with a formatted value-safe reason.
func contractErr(code FailureCode, format string, args ...any) *ContractError {
	return &ContractError{Code: code, Reason: fmt.Sprintf(format, args...)}
}

// canonicalRequiredJourneyIDs is the closed minimum the manifest must always
// declare. Removing any of these makes Compile return
// E102-JOURNEY-CONTRACT-MISSING-JOURNEY.
var canonicalRequiredJourneyIDs = []string{
	"session.login-reuse",
	"search.read",
	"digest.current-read",
	"assistant.grounded-read",
	"wiki.browse",
	"graph.explorer",
	"cards.representative-read",
	"recommendations.readiness",
	"notifications.read",
	"capability-status.read",
	"photos.status-read",
	"connectors.status-read",
	"models.status-read",
	"synthesis.status-read",
}

// Compile validates the manifest against config and routeRegistry and returns
// an immutable CompiledAcceptancePolicy, or a *ContractError. It fails closed on
// any incompleteness, unknown/duplicate/mismatched enum or code, implicit
// requiredness, unresolvable policy reference, health-only success, or
// production-unsafe field.
func (m ProductJourneyManifest) Compile(config CompiledPolicyConfig, routeRegistry RouteSideEffectRegistry) (CompiledAcceptancePolicy, error) {
	if routeRegistry == nil {
		routeRegistry = DefaultRouteSideEffectRegistry()
	}
	if m.APIVersion != manifestAPIVersion {
		return CompiledAcceptancePolicy{}, contractErr(CodeUnsupported, "manifest apiVersion %q is unsupported", m.APIVersion)
	}
	if m.Kind != manifestKind {
		return CompiledAcceptancePolicy{}, contractErr(CodeMalformed, "manifest kind %q is not %q", m.Kind, manifestKind)
	}
	if m.ResultSchema != resultSchema {
		return CompiledAcceptancePolicy{}, contractErr(CodeUnsupported, "manifest resultSchema %q is unsupported", m.ResultSchema)
	}
	if strings.TrimSpace(m.ManifestID) == "" {
		return CompiledAcceptancePolicy{}, contractErr(CodeMalformed, "manifest has no manifestId")
	}
	if m.ManifestRevision < 1 {
		return CompiledAcceptancePolicy{}, contractErr(CodeMalformed, "manifest revision must be >= 1")
	}
	if len(m.SupportedModes) == 0 {
		return CompiledAcceptancePolicy{}, contractErr(CodeMalformed, "manifest declares no supported modes")
	}
	for _, mode := range m.SupportedModes {
		if !closedModes[mode] {
			return CompiledAcceptancePolicy{}, contractErr(CodeUnknownEnum, "unknown supported mode %q", mode)
		}
	}
	for _, key := range []string{"requiredness", "timeouts", "freshness", "dataPolicy"} {
		if strings.TrimSpace(m.PolicyRefs[key]) == "" {
			return CompiledAcceptancePolicy{}, contractErr(CodeMalformed, "manifest policyRefs missing required key %q", key)
		}
	}
	reg, err := DefaultFailureRegistry()
	if err != nil {
		return CompiledAcceptancePolicy{}, contractErr(CodeMalformed, "failure registry is contract-invalid")
	}
	if len(m.Journeys) == 0 {
		return CompiledAcceptancePolicy{}, contractErr(CodeMissingJourney, "manifest declares no journeys")
	}

	seen := make(map[string]bool, len(m.Journeys))
	groups := make(map[JourneyGroup]bool, len(closedJourneyGroups))
	compiled := make([]CompiledJourney, 0, len(m.Journeys))
	byID := make(map[string]CompiledJourney, len(m.Journeys))

	for _, j := range m.Journeys {
		cj, cerr := compileJourney(j, config, routeRegistry, reg)
		if cerr != nil {
			return CompiledAcceptancePolicy{}, cerr
		}
		if seen[j.ID] {
			return CompiledAcceptancePolicy{}, contractErr(CodeDuplicateJourney, "duplicate journey id %q", j.ID)
		}
		seen[j.ID] = true
		groups[j.Group] = true
		compiled = append(compiled, cj)
		byID[cj.ID] = cj
	}

	for _, id := range canonicalRequiredJourneyIDs {
		if !seen[id] {
			return CompiledAcceptancePolicy{}, contractErr(CodeMissingJourney, "manifest is missing required journey %q", id)
		}
	}

	return CompiledAcceptancePolicy{
		ManifestID:       m.ManifestID,
		ManifestRevision: m.ManifestRevision,
		ResultSchema:     m.ResultSchema,
		SupportedModes:   append([]Mode(nil), m.SupportedModes...),
		Journeys:         compiled,
		byID:             byID,
		groups:           groups,
	}, nil
}

// compileJourney validates and resolves one journey.
func compileJourney(j ManifestJourney, config CompiledPolicyConfig, routeRegistry RouteSideEffectRegistry, reg *FailureRegistry) (CompiledJourney, *ContractError) {
	if strings.TrimSpace(j.ID) == "" {
		return CompiledJourney{}, contractErr(CodeMalformed, "a journey has no id")
	}
	if !closedJourneyGroups[j.Group] {
		return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q declares unknown group %q", j.ID, j.Group)
	}
	if !closedAudiences[j.Audience] {
		return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q declares unknown audience %q", j.ID, j.Audience)
	}
	// Requiredness must be explicit. An empty value is implicit requiredness.
	if strings.TrimSpace(string(j.Requiredness)) == "" {
		return CompiledJourney{}, contractErr(CodeMalformed, "journey %q has implicit (unset) requiredness", j.ID)
	}
	if !closedRequiredness[j.Requiredness] {
		return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q declares unknown requiredness %q", j.ID, j.Requiredness)
	}
	// Allowed outcomes: non-empty and closed. A required journey must permit a
	// clean pass; it may never be declared with only limitation outcomes.
	if len(j.AllowedOutcomes) == 0 {
		return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares no allowed outcomes", j.ID)
	}
	allowed := make(map[JourneyOutcome]bool, len(j.AllowedOutcomes))
	for _, o := range j.AllowedOutcomes {
		if !IsClosedJourneyOutcome(o) {
			return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q allows unknown outcome %q", j.ID, o)
		}
		allowed[o] = true
	}
	if j.Requiredness == RequirednessRequired && !allowed[OutcomePassed] {
		return CompiledJourney{}, contractErr(CodeMalformed, "required journey %q does not permit a clean pass", j.ID)
	}
	// Dependencies: every declared dependency is complete; a dependency-blocked
	// journey must declare at least one.
	for _, d := range j.Dependencies {
		if strings.TrimSpace(d.Packet) == "" {
			return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares a dependency with no packet", j.ID)
		}
		if !closedDependencyEvidenceClasses[d.EvidenceClass] {
			return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q dependency declares unknown evidence class %q", j.ID, d.EvidenceClass)
		}
	}
	if j.Requiredness == RequirednessDependencyBlocked && len(j.Dependencies) == 0 {
		return CompiledJourney{}, contractErr(CodeMalformed, "dependency-blocked journey %q declares no dependency", j.ID)
	}
	// Data mode: every declared mode/selection is closed.
	for mode, sel := range j.DataMode {
		if !closedModes[mode] {
			return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q dataMode declares unknown mode %q", j.ID, mode)
		}
		if !closedDataSelections[sel] {
			return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q dataMode declares unknown selection %q", j.ID, sel)
		}
	}
	// Steps: non-empty, each with a closed plane and non-health real behavior for
	// a required/optional journey.
	if len(j.Steps) == 0 {
		return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares no steps", j.ID)
	}
	hasBrowser := false
	hasRealBehavior := false
	for _, s := range j.Steps {
		if !closedPlanes[s.Plane] {
			return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q step %q declares unknown plane %q", j.ID, s.ID, s.Plane)
		}
		if len(s.ExpectedStatus) == 0 {
			return CompiledJourney{}, contractErr(CodeMalformed, "journey %q step %q declares no expected status", j.ID, s.ID)
		}
		if s.Plane == PlaneBrowser || s.Plane == PlaneBrowserNetwork {
			hasBrowser = true
		}
		// Real product behavior is a read/read-compute/session-establish step, not
		// a static asset or a telemetry-only health read.
		switch s.SideEffect {
		case SideEffectRead, SideEffectReadCompute, SideEffectSessionEstablish, SideEffectFixtureWrite:
			hasRealBehavior = true
		}
	}
	// Health-only success guard: a required/optional journey must observe real
	// product behavior, not only infrastructure health/telemetry.
	if (j.Requiredness == RequirednessRequired || j.Requiredness == RequirednessOptional) && !hasRealBehavior {
		return CompiledJourney{}, contractErr(CodeMalformed, "journey %q proves only health/telemetry, not product behavior (health-only success)", j.ID)
	}
	// Assertions: a required journey (and any browser journey) must declare a
	// status assertion, a schema-or-DOM assertion, and an accessibility assertion.
	if j.Requiredness == RequirednessRequired || hasBrowser {
		if len(j.Assertions.Status) == 0 {
			return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares no status assertion", j.ID)
		}
		if len(j.Assertions.Schema) == 0 && len(j.Assertions.DOM) == 0 {
			return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares no schema or DOM assertion", j.ID)
		}
		if hasBrowser && len(j.Assertions.Accessibility) == 0 {
			return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares no accessibility assertion", j.ID)
		}
	}
	// Failure codes: non-empty, registered, and category-matched to the group.
	if len(j.FailureCodes) == 0 {
		return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares no failure codes", j.ID)
	}
	wantCategory := groupToCategory[j.Group]
	for _, code := range j.FailureCodes {
		meta, ok := reg.LookupFailure(code)
		if !ok {
			return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q declares unregistered failure code %q", j.ID, code)
		}
		if meta.Category != wantCategory {
			return CompiledJourney{}, contractErr(CodeUnknownEnum, "journey %q code %q category %q does not match group category %q", j.ID, code, meta.Category, wantCategory)
		}
	}
	// Resolve timeout/freshness by policy reference with no fallback.
	if strings.TrimSpace(j.TimeoutRef) == "" {
		return CompiledJourney{}, contractErr(CodeMalformed, "journey %q declares no timeout reference", j.ID)
	}
	timeout, ok := config.Timeouts[j.TimeoutRef]
	if !ok || timeout <= 0 {
		return CompiledJourney{}, contractErr(CodeMalformed, "journey %q timeout reference %q does not resolve in policy", j.ID, j.TimeoutRef)
	}
	var freshness time.Duration
	if strings.TrimSpace(j.FreshnessRef) != "" {
		fr, fok := config.Freshness[j.FreshnessRef]
		if !fok || fr <= 0 {
			return CompiledJourney{}, contractErr(CodeMalformed, "journey %q freshness reference %q does not resolve in policy", j.ID, j.FreshnessRef)
		}
		freshness = fr
	}
	// Production read-only static guard reuse: every production-readonly step must
	// pass the sibling read-only guard (unclassified route, mutating method,
	// state-changing selector, target literal, or unsafe evidence field).
	surface := ProductionSurface{
		Routes:         make([]ProductionRoute, 0, len(j.Steps)),
		Selectors:      j.Selectors,
		EvidenceFields: j.EvidenceFields,
	}
	for _, s := range j.Steps {
		surface.Routes = append(surface.Routes, ProductionRoute{
			Method:     s.Method,
			Template:   s.Route,
			SideEffect: s.SideEffect,
		})
	}
	if gerr := ScanProductionSurface(surface, routeRegistry); gerr != nil {
		var gv *GuardViolation
		if asGuardViolation(gerr, &gv) {
			return CompiledJourney{}, contractErr(gv.Code, "journey %q production surface is unsafe: %s", j.ID, gv.Reason)
		}
		return CompiledJourney{}, contractErr(CodeUnsafeMutation, "journey %q production surface is unsafe", j.ID)
	}

	return CompiledJourney{
		ID:              j.ID,
		Group:           j.Group,
		Audience:        j.Audience,
		Requiredness:    j.Requiredness,
		AllowedOutcomes: allowed,
		Dependencies:    append([]DependencyRef(nil), j.Dependencies...),
		Steps:           append([]ManifestStep(nil), j.Steps...),
		FailureCodes:    append([]FailureCode(nil), j.FailureCodes...),
		Timeout:         timeout,
		Freshness:       freshness,
	}, nil
}

// asGuardViolation reports whether err is a *GuardViolation and, if so, binds it.
func asGuardViolation(err error, out **GuardViolation) bool {
	gv, ok := err.(*GuardViolation)
	if ok {
		*out = gv
	}
	return ok
}

// CoveredGroups returns the set of journey groups the manifest declares at least
// one journey for. TP-102-01-06 asserts this equals the full closed group set.
func (m ProductJourneyManifest) CoveredGroups() map[JourneyGroup]bool {
	groups := make(map[JourneyGroup]bool, len(closedJourneyGroups))
	for _, j := range m.Journeys {
		groups[j.Group] = true
	}
	return groups
}

// CoveredRoutes returns the set of normalized route templates the manifest
// references across every journey step. TP-102-01-06 asserts this covers every
// route authority in DefaultRouteSideEffectRegistry.
func (m ProductJourneyManifest) CoveredRoutes() map[string]bool {
	routes := make(map[string]bool)
	for _, j := range m.Journeys {
		for _, s := range j.Steps {
			routes[s.Route] = true
		}
	}
	return routes
}

// ClosedJourneyGroups returns a sorted copy of the closed journey-group set so
// tests can assert full coverage without touching the unexported map.
func ClosedJourneyGroups() []JourneyGroup {
	groups := make([]JourneyGroup, 0, len(closedJourneyGroups))
	for g := range closedJourneyGroups {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i] < groups[j] })
	return groups
}
