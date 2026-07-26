// product_journeys.go is the Go-literal single source of truth for Smackerel's
// product journey manifest and its compiled train/environment policy. It is the
// SST for requiredness: every journey declares an explicit Requiredness, an
// explicit allowed-outcome set, its route/side-effect steps, its assertions, its
// dependency evidence classes, and its category-matched E102-JOURNEY-* failure
// codes. CanonicalProductJourneyManifest() and DefaultPolicyConfig() together
// compile into a complete CompiledAcceptancePolicy with no default and no
// fallback (see manifest.go Compile).
//
// The data covers all fourteen closed journey groups: session (auth), search,
// digest, assistant, wiki, graph, cards, recommendations, notifications,
// capability-status (status/health), photos, connectors, models, and synthesis —
// and references every route authority in DefaultRouteSideEffectRegistry
// (read_only_guard.go).

package acceptance

import "time"

// safeEvidenceFields is the closed, value-safe evidence-field set every journey
// emits. None of these identifiers is a forbidden evidence token or a target
// literal, so a journey using them passes the reused read-only static guard.
var safeEvidenceFields = []string{"status", "route-id", "state-enum", "duration-ms", "body-digest", "outcome"}

// browserAssertions returns a complete assertion set for a browser journey: a
// status assertion, a schema and DOM assertion, and accessibility assertions.
func browserAssertions() JourneyAssertions {
	return JourneyAssertions{
		Status:        []string{"status-in-expected-set"},
		Schema:        []string{"schema-required-fields"},
		DOM:           []string{"dom-terminal-state", "dom-no-contradictory-state"},
		Accessibility: []string{"a11y-landmarks", "a11y-keyboard-operable", "a11y-live-region"},
		Telemetry:     []string{"telemetry-outcome-delta"},
		Freshness:     []string{"freshness-within-threshold"},
	}
}

// apiAssertions returns an assertion set for an api-only journey.
func apiAssertions() JourneyAssertions {
	return JourneyAssertions{
		Status:    []string{"status-in-expected-set"},
		Schema:    []string{"schema-required-fields"},
		Freshness: []string{"freshness-within-threshold"},
	}
}

func dataModeRead() map[Mode]DataSelection {
	return map[Mode]DataSelection{ModeProductionReadonly: DataExistingAuthorized, ModeSeededValidate: DataFixtureSet}
}

func dataModeSession() map[Mode]DataSelection {
	return map[Mode]DataSelection{ModeProductionReadonly: DataNone, ModeSeededValidate: DataFixtureSet}
}

// CanonicalProductJourneyManifest returns the immutable product-owned manifest.
// It is the SST for the journey inventory and requiredness.
func CanonicalProductJourneyManifest() ProductJourneyManifest {
	return ProductJourneyManifest{
		APIVersion:       manifestAPIVersion,
		Kind:             manifestKind,
		ManifestID:       "smackerel-primary-journeys",
		ManifestRevision: 1,
		ResultSchema:     resultSchema,
		SupportedModes:   []Mode{ModeSeededValidate, ModeProductionReadonly},
		PolicyRefs: map[string]string{
			"requiredness": "acceptance.journeys.requiredness",
			"timeouts":     "acceptance.journeys.timeouts",
			"freshness":    "acceptance.journeys.freshness",
			"dataPolicy":   "acceptance.journeys.data_policy",
		},
		Journeys: []ManifestJourney{
			{
				ID:              "session.login-reuse",
				Group:           GroupSession,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessRequired,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed},
				DataMode:        dataModeSession(),
				Steps: []ManifestStep{
					{ID: "open-login", Plane: PlaneBrowser, Method: "GET", Route: "/login", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "submit-login", Plane: PlaneBrowserNetwork, Method: "POST", Route: "/v1/web/login", SideEffect: SideEffectSessionEstablish, ExpectedStatus: []int{303}},
					{ID: "authenticated-probe", Plane: PlaneAPI, Method: "GET", Route: "/api/digest", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "login-form", "login-heading"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.session",
				FreshnessRef:   "acceptance.journeys.freshness.session",
				FailureCodes:   []FailureCode{"E102-JOURNEY-AUTH-LOGIN", "E102-JOURNEY-AUTH-COOKIE", "E102-JOURNEY-AUTH-REUSE"},
			},
			{
				ID:              "search.read",
				Group:           GroupSearch,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessRequired,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedEmpty},
				Dependencies:    []DependencyRef{{Packet: "specs/002-phase1-foundation/bugs/BUG-002-006-search-htmx-sri-blocks-submit", EvidenceClass: "certified-current-journey"}},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-home", Plane: PlaneBrowser, Method: "GET", Route: "/", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "submit-search", Plane: PlaneBrowserNetwork, Method: "POST", Route: "/search", SideEffect: SideEffectReadCompute, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "search-input", "search-submit", "results-list"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.search",
				FreshnessRef:   "acceptance.journeys.freshness.search",
				FailureCodes:   []FailureCode{"E102-JOURNEY-SEARCH-NO-REQUEST", "E102-JOURNEY-SEARCH-HTTP", "E102-JOURNEY-SEARCH-FALSE-EMPTY"},
			},
			{
				ID:              "digest.current-read",
				Group:           GroupDigest,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessRequired,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedEmpty, OutcomeAllowedQuiet},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-digest", Plane: PlaneBrowser, Method: "GET", Route: "/digest", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "digest-api", Plane: PlaneAPI, Method: "GET", Route: "/api/digest", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "digest-heading", "digest-date"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.digest",
				FreshnessRef:   "acceptance.journeys.freshness.digest",
				FailureCodes:   []FailureCode{"E102-JOURNEY-DIGEST-HTTP", "E102-JOURNEY-DIGEST-FALSE-EMPTY", "E102-JOURNEY-DIGEST-STALE"},
			},
			{
				ID:              "assistant.grounded-read",
				Group:           GroupAssistant,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessDependencyBlocked,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeBlocked, OutcomeNotEvaluated},
				Dependencies: []DependencyRef{
					{Packet: "specs/073-web-mobile-assistant-frontend/bugs/BUG-073-006-auth-rejection-blank-assistant-response", EvidenceClass: "certified-current-journey"},
					{Packet: "specs/104-universal-ask-self-knowledge#scope-08", EvidenceClass: "scope-bounded-evidence"},
				},
				DataMode: dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-assistant", Plane: PlaneBrowser, Method: "GET", Route: "/assistant", SideEffect: SideEffectRead, ExpectedStatus: []int{200, 302}},
					{ID: "assistant-turn", Plane: PlaneBrowserNetwork, Method: "POST", Route: "/api/assistant/turn", SideEffect: SideEffectReadCompute, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "assistant-composer", "assistant-send", "assistant-transcript"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.assistant",
				FreshnessRef:   "acceptance.journeys.freshness.assistant",
				FailureCodes:   []FailureCode{"E102-JOURNEY-ASSISTANT-DEPENDENCY", "E102-JOURNEY-ASSISTANT-BLANK", "E102-JOURNEY-ASSISTANT-FALSE-CAPTURE"},
			},
			{
				ID:              "wiki.browse",
				Group:           GroupWiki,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessRequired,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedEmpty},
				Dependencies:    []DependencyRef{{Packet: "specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable", EvidenceClass: "certified-current-journey"}},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-wiki", Plane: PlaneBrowser, Method: "GET", Route: "/pwa/wiki.html", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "open-wiki-topics", Plane: PlaneBrowser, Method: "GET", Route: "/pwa/wiki_topics.html", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "topics-api", Plane: PlaneAPI, Method: "GET", Route: "/api/topics", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "edges-api", Plane: PlaneAPI, Method: "GET", Route: "/api/graph/edges", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "wiki-heading", "wiki-topic-list"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.wiki",
				FreshnessRef:   "acceptance.journeys.freshness.wiki",
				FailureCodes:   []FailureCode{"E102-JOURNEY-GRAPH-ROUTE", "E102-JOURNEY-GRAPH-HTTP", "E102-JOURNEY-GRAPH-FALSE-EMPTY"},
			},
			{
				ID:              "graph.explorer",
				Group:           GroupGraph,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessDependencyBlocked,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeBlocked, OutcomeNotEvaluated},
				Dependencies:    []DependencyRef{{Packet: "specs/105-connected-knowledge-graph-explorer", EvidenceClass: "activated-runtime-contract"}},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-graph", Plane: PlaneBrowser, Method: "GET", Route: "/knowledge/graph", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "graph-query", Plane: PlaneBrowserNetwork, Method: "POST", Route: "/api/graph/query", SideEffect: SideEffectReadCompute, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "graph-canvas", "graph-outline", "graph-table"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.graph",
				FreshnessRef:   "acceptance.journeys.freshness.graph",
				FailureCodes:   []FailureCode{"E102-JOURNEY-GRAPH-DEPENDENCY", "E102-JOURNEY-GRAPH-QUERY", "E102-JOURNEY-GRAPH-UNBOUNDED"},
			},
			{
				ID:              "cards.representative-read",
				Group:           GroupCards,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessOptional,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedOptional, OutcomeAllowedEmpty, OutcomeAllowedDegraded},
				Dependencies:    []DependencyRef{{Packet: "specs/083-card-rewards-companion/bugs/BUG-083-002-ccmanager-parity-runtime-drift", EvidenceClass: "certified-current-journey"}},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-cards", Plane: PlaneBrowser, Method: "GET", Route: "/cards", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "cards-api", Plane: PlaneAPI, Method: "GET", Route: "/api/cards/", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "cards-report", Plane: PlaneAPI, Method: "GET", Route: "/api/card-optimization-report", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "cards-heading", "cards-wallet-list"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.cards",
				FreshnessRef:   "acceptance.journeys.freshness.cards",
				FailureCodes:   []FailureCode{"E102-JOURNEY-CARDS-DEPENDENCY", "E102-JOURNEY-CARDS-HTTP", "E102-JOURNEY-CARDS-FALSE-EMPTY"},
			},
			{
				ID:              "recommendations.readiness",
				Group:           GroupRecommendations,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessOptional,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedOptional, OutcomeAllowedDegraded},
				Dependencies:    []DependencyRef{{Packet: "specs/039-recommendations-engine/bugs/BUG-039-005-enabled-with-zero-providers-false-ready", EvidenceClass: "certified-current-journey"}},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-recommendations", Plane: PlaneBrowser, Method: "GET", Route: "/recommendations", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "availability-api", Plane: PlaneAPI, Method: "GET", Route: "/api/recommendations/availability", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "providers-api", Plane: PlaneAPI, Method: "GET", Route: "/api/recommendations/providers", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "recommendations-heading", "recommendations-status"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.recommendations",
				FreshnessRef:   "acceptance.journeys.freshness.recommendations",
				FailureCodes:   []FailureCode{"E102-JOURNEY-RECOMMEND-DEPENDENCY", "E102-JOURNEY-RECOMMEND-ZERO-PROVIDER", "E102-JOURNEY-RECOMMEND-STALE"},
			},
			{
				ID:              "notifications.read",
				Group:           GroupNotifications,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessOptional,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedOptional, OutcomeAllowedEmpty, OutcomeAllowedDegraded},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-notifications", Plane: PlaneBrowser, Method: "GET", Route: "/notifications", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "notifications-status-api", Plane: PlaneAPI, Method: "GET", Route: "/api/notifications/status", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "notifications-events-api", Plane: PlaneAPI, Method: "GET", Route: "/api/notifications/events", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "notifications-heading", "notifications-status"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.notifications",
				FreshnessRef:   "acceptance.journeys.freshness.notifications",
				FailureCodes:   []FailureCode{"E102-JOURNEY-NOTIFICATIONS-HTTP", "E102-JOURNEY-NOTIFICATIONS-FALSE-EMPTY", "E102-JOURNEY-NOTIFICATIONS-STALE"},
			},
			{
				ID:              "capability-status.read",
				Group:           GroupCapabilityStatus,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessRequired,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedDegraded},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-settings", Plane: PlaneBrowser, Method: "GET", Route: "/settings", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "open-status", Plane: PlaneBrowser, Method: "GET", Route: "/status", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "health-supporting", Plane: PlaneTelemetry, Method: "GET", Route: "/api/health", SideEffect: SideEffectTelemetryRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "status-heading", "capability-row"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.capability-status",
				FreshnessRef:   "acceptance.journeys.freshness.capability-status",
				FailureCodes:   []FailureCode{"E102-JOURNEY-CONTRACT-CAPABILITY-POLICY", "E102-JOURNEY-CONTRACT-CAPABILITY-STATUS-MISMATCH", "E102-JOURNEY-CONTRACT-CAPABILITY-STALE"},
			},
			{
				ID:              "photos.status-read",
				Group:           GroupPhotos,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessOptional,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedOptional, OutcomeAllowedEmpty, OutcomeAllowedDegraded},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-photo-health", Plane: PlaneBrowser, Method: "GET", Route: "/pwa/photo-health.html", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "photos-health-api", Plane: PlaneAPI, Method: "GET", Route: "/v1/photos/health", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "photos-connectors-api", Plane: PlaneAPI, Method: "GET", Route: "/v1/photos/connectors", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "photo-health-summary", "photo-limitation-code"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.photos",
				FreshnessRef:   "acceptance.journeys.freshness.photos",
				FailureCodes:   []FailureCode{"E102-JOURNEY-PHOTOS-HTTP", "E102-JOURNEY-PHOTOS-FALSE-EMPTY", "E102-JOURNEY-PHOTOS-LIMITATION"},
			},
			{
				ID:              "connectors.status-read",
				Group:           GroupConnectors,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessOptional,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedOptional, OutcomeAllowedEmpty},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-connectors", Plane: PlaneBrowser, Method: "GET", Route: "/pwa/connectors.html", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "connectors-drive-api", Plane: PlaneAPI, Method: "GET", Route: "/v1/connectors/drive", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "connectors-heading", "connector-row"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.connectors",
				FreshnessRef:   "acceptance.journeys.freshness.connectors",
				FailureCodes:   []FailureCode{"E102-JOURNEY-CONNECTORS-HTTP", "E102-JOURNEY-CONNECTORS-FALSE-EMPTY", "E102-JOURNEY-CONNECTORS-STALE"},
			},
			{
				ID:              "models.status-read",
				Group:           GroupModels,
				Audience:        AudienceOperator,
				Requiredness:    RequirednessOptional,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeAllowedOptional, OutcomeAllowedDegraded},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "open-model-connections", Plane: PlaneBrowser, Method: "GET", Route: "/pwa/model-connections.html", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
					{ID: "model-connections-api", Plane: PlaneAPI, Method: "GET", Route: "/v1/admin/model-connections", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     browserAssertions(),
				Selectors:      []string{"main-landmark", "models-heading", "model-slot-row"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.models",
				FreshnessRef:   "acceptance.journeys.freshness.models",
				FailureCodes:   []FailureCode{"E102-JOURNEY-MODELS-AUTHZ", "E102-JOURNEY-MODELS-HTTP", "E102-JOURNEY-MODELS-FALSE-READY"},
			},
			{
				ID:              "synthesis.status-read",
				Group:           GroupSynthesis,
				Audience:        AudienceAuthenticatedUser,
				Requiredness:    RequirednessDependencyBlocked,
				AllowedOutcomes: []JourneyOutcome{OutcomePassed, OutcomeBlocked, OutcomeNotEvaluated},
				Dependencies:    []DependencyRef{{Packet: "specs/004-phase3-intelligence/bugs/BUG-004-004-synthesis-persistence-and-health-truth", EvidenceClass: "certified-current-journey"}},
				DataMode:        dataModeRead(),
				Steps: []ManifestStep{
					{ID: "synthesis-latest-api", Plane: PlaneAPI, Method: "GET", Route: "/api/intelligence/synthesis/latest", SideEffect: SideEffectRead, ExpectedStatus: []int{200}},
				},
				Assertions:     apiAssertions(),
				Selectors:      []string{"main-landmark", "synthesis-status"},
				EvidenceFields: safeEvidenceFields,
				TimeoutRef:     "acceptance.journeys.timeouts.synthesis",
				FreshnessRef:   "acceptance.journeys.freshness.synthesis",
				FailureCodes:   []FailureCode{"E102-JOURNEY-SYNTH-DEPENDENCY", "E102-JOURNEY-SYNTH-NEVER-RUN-UP", "E102-JOURNEY-SYNTH-NO-DURABLE-OUTPUT"},
			},
		},
	}
}

// DefaultPolicyConfig returns the compiled train/environment policy the
// canonical manifest is compiled against. It resolves every journey timeout and
// freshness reference explicitly; there is no in-code fallback, so a manifest
// journey whose reference is absent here fails compilation.
func DefaultPolicyConfig() CompiledPolicyConfig {
	groups := []JourneyGroup{
		GroupSession, GroupSearch, GroupDigest, GroupAssistant, GroupWiki, GroupGraph,
		GroupCards, GroupRecommendations, GroupNotifications, GroupCapabilityStatus,
		GroupPhotos, GroupConnectors, GroupModels, GroupSynthesis,
	}
	timeouts := make(map[string]time.Duration, len(groups))
	freshness := make(map[string]time.Duration, len(groups))
	for _, g := range groups {
		timeouts["acceptance.journeys.timeouts."+string(g)] = 30 * time.Second
		freshness["acceptance.journeys.freshness."+string(g)] = 15 * time.Minute
	}
	return CompiledPolicyConfig{
		Train:       "mvp",
		Environment: "production-readonly",
		Timeouts:    timeouts,
		Freshness:   freshness,
	}
}
