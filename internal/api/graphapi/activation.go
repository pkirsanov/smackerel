package graphapi

// activation.go — fail-soft runtime activation for the spec 080
// Knowledge Graph Public API.
//
// BUG-080-001 (operator-directed fail-soft contract). On the live
// product the Wiki/Graph public APIs were ABSENT because the cursor
// HMAC secret resolved empty: cmd/core/wiring.go logged a warning,
// left every graph handler field nil, and internal/api/router.go then
// omitted the routes so callers saw an ordinary Chi 404 — a SILENT
// ABSENCE indistinguishable from "the feature does not exist".
//
// This file supplies the disjoint, hermetically unit-testable core of
// the fix: read the EXISTING cursor-secret config; when it is empty or
// missing, resolve a TYPED runtime-DISABLED state that answers every
// known graph path with a typed 503 `capability_disabled` envelope —
// never a silent 404, never an opaque 500, never a panic. When the
// secret is present, activation is ENABLED and the guard delegates to
// the operating handler unchanged (typed errors on the operating path
// flow through transparently).
//
// It also models the single-operator-owned GLOBAL-corpus authorization
// matrix (GRAPH-ACT-005 / GRAPH-ACT-011): operator and grant-holder
// identities are authorized to read the SAME global rows (the grant,
// not a row partition, differentiates the projection), and an ungranted
// authenticated identity receives a leak-free `unauthorized-scope`
// (403 `forbidden`) denial. No read path here adds a tenant or per-user
// row predicate and no outcome claims per-user row isolation.
//
// Scope boundary: this file is intentionally decoupled — it imports no
// datastore, no router, and no auth package, so its behavior is proven
// by pure httptest unit tests. Router/core wiring of Guard and the
// auth.Session -> GraphIdentity adapter is a separate integration step
// (see the BUG-080-001 report for the deferred residuals).

import (
	"net/http"
	"os"
	"slices"
)

// CodeCapabilityDisabled is the closed error code returned for every
// graph path while the capability is in the fail-soft runtime-DISABLED
// state. It is a TYPED, honest "graph disabled" outcome — distinct from
// a missing route (404), an opaque failure (500), or a silent absence.
const CodeCapabilityDisabled = "capability_disabled"

// Value-safe activation diagnostic codes. These name only the closed
// condition and the non-secret config identity; a secret value, its
// length, hash, or any derivative NEVER appears in a code.
const (
	// CodeActivationOK marks an ENABLED activation (secret present).
	CodeActivationOK = "OK"
	// CodeCursorSecretEmpty marks the DISABLED activation reached when
	// the named cursor-secret env var resolves to an empty value.
	CodeCursorSecretEmpty = "F080-CURSOR-SECRET-EMPTY"  //gitleaks:allow
	// CodeCursorSecretMissing marks the DISABLED activation reached
	// when the cursor-secret indirection is unset (no env name, or the
	// named env var is absent).
	CodeCursorSecretMissing = "F080-CURSOR-SECRET-MISSING"  //gitleaks:allow
)

// ErrCapabilityDisabled is the canonical typed response for the
// fail-soft runtime-disabled state. Its wire shape is the uniform
// graphapi envelope:
//
//	{"error":{"code":"capability_disabled",
//	          "message":"connected knowledge is disabled for this deployment"}}
//
// with HTTP status 503 Service Unavailable — NOT 404 (which would
// re-create the silent-absence bug) and NOT 500 (which would be an
// opaque failure). Handlers reuse this singleton via WriteDisabled.
var ErrCapabilityDisabled = &APIError{
	Status:  http.StatusServiceUnavailable,
	Code:    CodeCapabilityDisabled,
	Message: "connected knowledge is disabled for this deployment",
}

// SecretPresence is the value-safe classification of the cursor-secret
// configuration. It carries NO secret bytes, length, or hash.
type SecretPresence string

const (
	// SecretPresent — the named env var is set to a non-empty value.
	SecretPresent SecretPresence = "present"
	// SecretEmpty — the named env var is set but its value is "".
	SecretEmpty SecretPresence = "empty"
	// SecretMissing — no env name is configured, or the named env var
	// is not set at all.
	SecretMissing SecretPresence = "missing"
)

// ClassifyCursorSecret reports the value-safe presence class of the
// cursor HMAC secret named by CursorSecretEnv, WITHOUT returning,
// logging, or otherwise exposing the secret value. It reuses the same
// os.LookupEnv indirection that LoadCursorSecret uses so the fail-soft
// decision reads the EXISTING secret configuration rather than a new
// config key.
func (c Config) ClassifyCursorSecret() SecretPresence {
	if c.CursorSecretEnv == "" {
		return SecretMissing
	}
	v, ok := os.LookupEnv(c.CursorSecretEnv)
	if !ok {
		return SecretMissing
	}
	if v == "" {
		return SecretEmpty
	}
	return SecretPresent
}

// ActivationState is the closed fail-soft activation enum.
type ActivationState string

const (
	// ActivationEnabled — the graph public API operates normally.
	ActivationEnabled ActivationState = "enabled"
	// ActivationDisabled — the graph public API answers every known
	// path with the typed capability_disabled response.
	ActivationDisabled ActivationState = "disabled"
)

// Activation is the resolved, value-safe runtime activation outcome.
// Every field is safe to log, emit as a metric label, or surface in a
// status projection: it names the state, the secret presence class, and
// a closed non-secret code — never the secret itself.
type Activation struct {
	// State is the resolved enabled/disabled decision.
	State ActivationState
	// SecretPresence is why the decision was reached, value-safe.
	SecretPresence SecretPresence
	// Code is the closed value-safe activation diagnostic code.
	Code string
}

// Disabled reports whether the graph public API is in the fail-soft
// runtime-disabled state.
func (a Activation) Disabled() bool { return a.State == ActivationDisabled }

// ResolveActivation derives the fail-soft activation decision from the
// EXISTING cursor-secret configuration. It NEVER returns an error and
// NEVER panics: a present secret yields ENABLED; an empty or missing
// secret yields the typed DISABLED state (fail-soft) rather than a boot
// refusal, a 500, or a silent nil-handler absence.
func ResolveActivation(cfg Config) Activation {
	switch p := cfg.ClassifyCursorSecret(); p {
	case SecretPresent:
		return Activation{State: ActivationEnabled, SecretPresence: p, Code: CodeActivationOK}
	case SecretEmpty:
		return Activation{State: ActivationDisabled, SecretPresence: p, Code: CodeCursorSecretEmpty}
	default: // SecretMissing
		return Activation{State: ActivationDisabled, SecretPresence: SecretMissing, Code: CodeCursorSecretMissing}
	}
}

// GraphCapability is the fail-soft graph public-API service. It owns a
// single resolved Activation and the typed responses that make the
// runtime-disabled state honest and observable instead of a silent 404.
type GraphCapability struct {
	activation Activation
}

// NewGraphCapability resolves the fail-soft activation from the supplied
// (already-validated) Config and returns the capability service. It does
// not touch the datastore, the router, or the secret value.
func NewGraphCapability(cfg Config) *GraphCapability {
	return &GraphCapability{activation: ResolveActivation(cfg)}
}

// Activation returns the resolved value-safe activation outcome.
func (c *GraphCapability) Activation() Activation { return c.activation }

// Disabled reports whether the capability is in the runtime-disabled
// state.
func (c *GraphCapability) Disabled() bool { return c.activation.Disabled() }

// WriteDisabled emits the typed 503 capability_disabled envelope. It is
// the ONLY response the disabled capability serves for a known graph
// path — never a bare 404 or an opaque 500.
func (c *GraphCapability) WriteDisabled(w http.ResponseWriter) {
	WriteAPIError(w, ErrCapabilityDisabled)
}

// Guard wraps an operating graph handler with the fail-soft activation
// gate. When the capability is DISABLED it answers with the typed 503
// capability_disabled envelope for every wrapped path (so an empty
// secret can never resurface as a silent Chi 404 nor as an opaque 500).
// When ENABLED it delegates to next unchanged, so the operating path's
// typed errors flow through transparently.
func (c *GraphCapability) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.Disabled() {
			c.WriteDisabled(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GraphReadScope is the single graph-read grant. It is the same scope
// already enforced by auth.RequireScope("knowledge-graph:read") in
// internal/api/router.go; this file reframes its meaning under the
// single-operator-owned global-corpus model rather than renaming it.
const GraphReadScope = "knowledge-graph:read"

// GraphGrant is the caller's authorization class against the single
// operator-owned GLOBAL corpus. It is derived from the grant the caller
// holds, NOT from any tenant or per-user row partition.
type GraphGrant string

const (
	// GrantOperator reads all private graph content plus operational
	// metadata (the auth bootstrap/shared-token admin sources).
	GrantOperator GraphGrant = "operator"
	// GrantHolder reads the authorized global-corpus projection granted
	// by the knowledge-graph:read scope. It sees the SAME global rows
	// every other grant-holder sees.
	GrantHolder GraphGrant = "grant_holder"
	// GrantNone is an ungranted identity: an authenticated principal
	// without the knowledge-graph:read grant, or an unauthenticated
	// caller. It receives a leak-free denial only.
	GrantNone GraphGrant = "ungranted"
)

// GraphIdentity is the value-safe projection of the authenticated
// principal used to classify graph-read authorization. It deliberately
// carries NO tenant id, no owner id, and no row selector: the corpus is
// one global corpus and the projection differs by GRANT, not by a
// per-identity row predicate.
//
// The router adapter (deferred integration) builds a GraphIdentity from
// an auth.Session: Operator = session.IsAdmin() (bootstrap/shared-token
// sources); Grants = session.Scopes; Authenticated = a session exists.
type GraphIdentity struct {
	// Authenticated reports whether a valid session is present.
	Authenticated bool
	// Operator reports whether the principal reads all private graph
	// content plus operational metadata.
	Operator bool
	// Grants is the principal's scope set; GraphReadScope authorizes
	// the global-corpus projection.
	Grants []string
}

// ClassifyGraphGrant maps an identity to its grant class against the
// single global corpus. It inspects ONLY the operator flag and the
// grant set — never a tenant or row identifier — so it structurally
// cannot introduce per-user row isolation.
func ClassifyGraphGrant(id GraphIdentity) GraphGrant {
	if id.Operator {
		return GrantOperator
	}
	if id.Authenticated && slices.Contains(id.Grants, GraphReadScope) {
		return GrantHolder
	}
	return GrantNone
}

// AuthorizeGraphRead returns nil when the grant class may read the
// global corpus (operator OR grant-holder), or the leak-free
// ErrMissingScope (403 `forbidden`) for an ungranted identity. The
// denial discloses no labels, nodes, edges, counts, route-family
// existence, source titles, or graph-existence hints, so an ungranted
// caller cannot distinguish "denied" from "empty" from "absent". This
// function introduces NO tenant or per-user row predicate; a granted
// read sees the same global rows regardless of identity.
func AuthorizeGraphRead(grant GraphGrant) *APIError {
	switch grant {
	case GrantOperator, GrantHolder:
		return nil
	default:
		return ErrMissingScope
	}
}
