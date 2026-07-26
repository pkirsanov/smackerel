// read_only_guard.go implements the BUG-102-001 production read-only static
// guard. It fails closed over a value-safe description of a production-readonly
// acceptance surface — the classified routes, the UI selectors the journeys
// activate, the result/evidence field identifiers the surface emits, and the raw
// runner/config text surfaces — and rejects any production write or
// state-changing selector, request interception, credential/cookie injection,
// direct datastore read, service-container exec, unclassified route, mutating
// method, concrete target literal, or forbidden evidence field. POST is allowed
// only for the closed session-establish or read-compute route classes; every
// other non-GET/HEAD method is a mutation.
//
// Every rejection carries one closed E102-JOURNEY-CONTRACT-* failure code from
// failure_registry.go, and the offending raw value is never echoed. Absent or
// ambiguous classification is a rejection, never a tolerated pass.

package acceptance

import (
	"fmt"
	"regexp"
	"strings"
)

// SideEffectClass is a closed route side-effect classification.
type SideEffectClass string

// Closed side-effect classes.
const (
	SideEffectStatic           SideEffectClass = "static"
	SideEffectRead             SideEffectClass = "read"
	SideEffectReadCompute      SideEffectClass = "read-compute"
	SideEffectSessionEstablish SideEffectClass = "session-establish"
	SideEffectTelemetryRead    SideEffectClass = "telemetry-read"
	SideEffectFixtureWrite     SideEffectClass = "fixture-write"
	SideEffectForbidden        SideEffectClass = "forbidden"
)

// closedSideEffects is the closed set of valid side-effect classes.
var closedSideEffects = map[SideEffectClass]bool{
	SideEffectStatic:           true,
	SideEffectRead:             true,
	SideEffectReadCompute:      true,
	SideEffectSessionEstablish: true,
	SideEffectTelemetryRead:    true,
	SideEffectFixtureWrite:     true,
	SideEffectForbidden:        true,
}

// productionAllowedSideEffects is the subset a production-readonly surface may
// declare. fixture-write and forbidden are validate-only / denied and can never
// appear in a production surface.
var productionAllowedSideEffects = map[SideEffectClass]bool{
	SideEffectStatic:           true,
	SideEffectRead:             true,
	SideEffectReadCompute:      true,
	SideEffectSessionEstablish: true,
	SideEffectTelemetryRead:    true,
}

// ProductionRoute is one classified route in a production-readonly surface. It
// carries a normalized route template (never a full URL with a host), the
// declared side-effect class, and the value-safe selector and evidence-field
// identifiers the route contributes.
type ProductionRoute struct {
	Method         string
	Template       string
	SideEffect     SideEffectClass
	Selectors      []string
	EvidenceFields []string
}

// ProductionSurface is the static, value-safe description of a
// production-readonly acceptance surface the guard validates before any run. It
// contains identifiers and normalized templates only — never a secret value.
type ProductionSurface struct {
	// Routes are the classified same-origin routes the surface observes.
	Routes []ProductionRoute
	// Selectors are surface-level UI control identifiers the journeys activate.
	Selectors []string
	// EvidenceFields are surface-level result/evidence field identifiers emitted.
	EvidenceFields []string
	// RunnerSources are raw runner/config text surfaces scanned for interception,
	// injection, direct datastore/container access, and concrete target literals.
	RunnerSources []string
}

// GuardViolation is a fail-closed rejection carrying one closed
// E102-JOURNEY-CONTRACT-* code and a value-safe reason. The offending raw value
// is never included.
type GuardViolation struct {
	Code   FailureCode
	Reason string
}

// Error implements error.
func (v *GuardViolation) Error() string {
	return fmt.Sprintf("read-only guard: %s: %s", v.Code, v.Reason)
}

// mutatingSelectorTokens are the closed state-changing action verbs a
// production-readonly surface may never activate (design production-mode item 5
// plus the scopes.md list). Matching is case-insensitive substring.
var mutatingSelectorTokens = []string{
	"create", "save", "update", "delete", "refresh", "sync",
	"test-provider", "test-connector", "provider-test", "reconnect",
	"replay", "snooze", "approve", "generate", "trigger", "schedule",
	"upload", "reveal", "confirm", "ingest", "rotate", "revoke",
}

// interceptionPatterns flag request interception / canned-response fabrication.
var interceptionPatterns = []string{
	"page.route", "context.route", "route.fulfill", "route.abort",
	"route.continue", "setrequestinterception", "page.unroute",
	"msw", "nock", "wiremock",
}

// injectionPatterns flag credential / cookie / bearer-header injection that
// bypasses the real login UI.
var injectionPatterns = []string{
	"addcookies", "setextrahttpheaders", "authorization:", "bearer ",
	"document.cookie", "localstorage.setitem", "sessionstorage.setitem",
}

// dataAccessPatterns flag a direct datastore read that bypasses the product.
var dataAccessPatterns = []string{
	"select * from", "insert into", "update set", "delete from",
	"pg_", "psql ", "database/sql", "sql.open(",
}

// containerExecPatterns flag a service-container exec that bypasses the product.
var containerExecPatterns = []string{
	"docker exec", "docker-compose exec", "kubectl exec",
	`exec.command("docker`, `exec.command("kubectl`, "nsenter",
}

// targetLiteralPatterns flag a concrete target host, operator path, tailnet
// identity, or private-key literal that must never enter product source or
// evidence.
var targetLiteralPatterns = []string{
	"http://", "https://", ".ts.net", "/home/", "/users/",
	"age-secret-key", "begin openssh private key", "begin rsa private key",
	"begin private key",
}

// ipv4Re matches a dotted-quad IPv4 literal (a concrete target address).
var ipv4Re = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

// forbiddenEvidenceTokens are result/evidence field identifiers that would carry
// a credential, personal content, or target detail. Matching is
// case-insensitive substring.
var forbiddenEvidenceTokens = []string{
	"password", "passwd", "token", "cookie", "authorization", "bearer",
	"secret", "credential", "apikey", "api-key", "query-text", "querytext",
	"prompt", "answer", "response-body", "responsebody", "raw-body", "rawbody",
	"full-url", "url-with-query", "graph-label", "graphlabel", "card-fact",
	"cardfact", "prose", "stacktrace", "stack-trace", "preview",
	"provider-payload", "providerpayload", "username", "fqdn",
}

// RouteSideEffectRegistry classifies known normalized route templates. The
// static scanner uses it to detect declared-versus-canonical side-effect drift.
type RouteSideEffectRegistry map[string]SideEffectClass

// DefaultRouteSideEffectRegistry is the canonical classification of the product
// routes named in the design journey inventory. It is the drift baseline: a
// surface that declares a different class for a known route is rejected.
func DefaultRouteSideEffectRegistry() RouteSideEffectRegistry {
	return RouteSideEffectRegistry{
		"/":                                  SideEffectRead,
		"/login":                             SideEffectRead,
		"/v1/web/login":                      SideEffectSessionEstablish,
		"/search":                            SideEffectReadCompute,
		"/digest":                            SideEffectRead,
		"/api/digest":                        SideEffectRead,
		"/assistant":                         SideEffectRead,
		"/api/assistant/turn":                SideEffectReadCompute,
		"/pwa/wiki.html":                     SideEffectRead,
		"/pwa/wiki_topics.html":              SideEffectRead,
		"/api/topics":                        SideEffectRead,
		"/api/graph/edges":                   SideEffectRead,
		"/api/graph/query":                   SideEffectReadCompute,
		"/knowledge/graph":                   SideEffectRead,
		"/cards":                             SideEffectRead,
		"/api/cards/":                        SideEffectRead,
		"/api/card-optimization-report":      SideEffectRead,
		"/recommendations":                   SideEffectRead,
		"/api/recommendations/availability":  SideEffectRead,
		"/api/recommendations/providers":     SideEffectRead,
		"/notifications":                     SideEffectRead,
		"/api/notifications/status":          SideEffectRead,
		"/api/notifications/events":          SideEffectRead,
		"/settings":                          SideEffectRead,
		"/status":                            SideEffectRead,
		"/api/health":                        SideEffectTelemetryRead,
		"/pwa/photo-health.html":             SideEffectRead,
		"/v1/photos/health":                  SideEffectRead,
		"/v1/photos/connectors":              SideEffectRead,
		"/pwa/connectors.html":               SideEffectRead,
		"/v1/connectors/drive":               SideEffectRead,
		"/pwa/model-connections.html":        SideEffectRead,
		"/v1/admin/model-connections":        SideEffectRead,
		"/api/intelligence/synthesis/latest": SideEffectRead,
	}
}

// ScanProductionSurface validates a production-readonly surface and returns the
// first fail-closed *GuardViolation, or nil when the surface is safe. The scan
// order is deterministic: route structure, then selectors, then runner-source
// text, then evidence fields.
func ScanProductionSurface(surface ProductionSurface, registry RouteSideEffectRegistry) error {
	if registry == nil {
		registry = DefaultRouteSideEffectRegistry()
	}
	for _, r := range surface.Routes {
		if v := scanRoute(r, registry); v != nil {
			return v
		}
	}
	if v := scanSelectors(surface.Selectors); v != nil {
		return v
	}
	for _, src := range surface.RunnerSources {
		if v := scanRunnerSource(src); v != nil {
			return v
		}
	}
	if v := scanEvidenceFields(surface.EvidenceFields); v != nil {
		return v
	}
	return nil
}

func scanRoute(r ProductionRoute, registry RouteSideEffectRegistry) *GuardViolation {
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if method == "" {
		return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: "route declares no HTTP method"}
	}
	if strings.TrimSpace(r.Template) == "" {
		return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: "route declares no normalized template"}
	}
	// A concrete target literal (full URL, host, tailnet identity, operator path)
	// must never appear in a route template.
	if code, why, bad := targetLiteral(r.Template); bad {
		return &GuardViolation{Code: code, Reason: "route template carries a concrete target literal (" + why + ")"}
	}
	// An unclassified or unknown side-effect class fails closed.
	if r.SideEffect == "" {
		return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: "unclassified route: no side-effect class"}
	}
	if !closedSideEffects[r.SideEffect] {
		return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("route declares unknown side-effect class %q", r.SideEffect)}
	}
	if !productionAllowedSideEffects[r.SideEffect] {
		return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("side-effect class %q is not permitted in a production-readonly surface", r.SideEffect)}
	}
	// Canonical drift: a known route must keep its canonical classification.
	if want, known := registry[r.Template]; known && r.SideEffect != want {
		return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("route declares side-effect %q but the registry classifies it %q", r.SideEffect, want)}
	}
	// Method rules: POST only for session-establish or read-compute; every other
	// non-GET/HEAD method is a mutation.
	switch method {
	case "GET", "HEAD":
		switch r.SideEffect {
		case SideEffectStatic, SideEffectRead, SideEffectReadCompute, SideEffectTelemetryRead:
			// safe read
		default:
			return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("%s route may not carry side-effect %q", method, r.SideEffect)}
		}
	case "POST":
		if r.SideEffect != SideEffectSessionEstablish && r.SideEffect != SideEffectReadCompute {
			return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("POST is allowed only for session-establish or read-compute, got %q", r.SideEffect)}
		}
	default:
		return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("mutating HTTP method %q is forbidden in a production-readonly surface", method)}
	}
	// Route-scoped selectors and evidence fields.
	if v := scanSelectors(r.Selectors); v != nil {
		return v
	}
	return scanEvidenceFields(r.EvidenceFields)
}

func scanSelectors(selectors []string) *GuardViolation {
	for _, s := range selectors {
		low := strings.ToLower(s)
		for _, tok := range mutatingSelectorTokens {
			if strings.Contains(low, tok) {
				return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("selector activates a state-changing action (%q)", tok)}
			}
		}
	}
	return nil
}

func scanRunnerSource(src string) *GuardViolation {
	low := strings.ToLower(src)
	for _, p := range interceptionPatterns {
		if strings.Contains(low, p) {
			return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("runner source uses request interception (%q)", p)}
		}
	}
	for _, p := range injectionPatterns {
		if strings.Contains(low, p) {
			return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("runner source injects credential/cookie state (%q)", p)}
		}
	}
	for _, p := range dataAccessPatterns {
		if strings.Contains(low, p) {
			return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("runner source performs a direct datastore read (%q)", p)}
		}
	}
	for _, p := range containerExecPatterns {
		if strings.Contains(low, p) {
			return &GuardViolation{Code: CodeContractUnsafeMutation, Reason: fmt.Sprintf("runner source performs a service-container exec (%q)", p)}
		}
	}
	if code, why, bad := targetLiteral(src); bad {
		return &GuardViolation{Code: code, Reason: "runner source carries a concrete target literal (" + why + ")"}
	}
	return nil
}

func scanEvidenceFields(fields []string) *GuardViolation {
	for _, f := range fields {
		low := strings.ToLower(f)
		for _, tok := range forbiddenEvidenceTokens {
			if strings.Contains(low, tok) {
				return &GuardViolation{Code: CodeContractEvidenceUnsafe, Reason: fmt.Sprintf("evidence field carries forbidden content (%q)", tok)}
			}
		}
		if code, why, bad := targetLiteral(f); bad {
			return &GuardViolation{Code: code, Reason: "evidence field carries a concrete target literal (" + why + ")"}
		}
	}
	return nil
}

// targetLiteral reports whether s carries a concrete target literal and, if so,
// the closed code and a value-safe reason token. It never echoes the literal.
func targetLiteral(s string) (FailureCode, string, bool) {
	low := strings.ToLower(s)
	for _, p := range targetLiteralPatterns {
		if strings.Contains(low, p) {
			return CodeContractEvidenceUnsafe, p, true
		}
	}
	if ipv4Re.MatchString(s) {
		return CodeContractEvidenceUnsafe, "ipv4-literal", true
	}
	return "", "", false
}
