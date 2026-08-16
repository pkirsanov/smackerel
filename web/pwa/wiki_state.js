// BUG-080-001 SCOPE-04 — the ONE closed activation/read state model for
// Knowledge Graph surfaces (GRAPH-ACT-004/005/006/009/010).
//
// Why this module exists: before it, every wiki page inferred state from
// the raw HTTP code at its own call site and funnelled every failure
// through one generic "Error: GET <path> HTTP <n>" string. A rejected
// session, a denied scope, an absent route, a downed store and a
// deliberately disabled capability were indistinguishable to the user,
// and a successful zero-row read rendered as a blank list with no
// guidance. scopes.md implementation step 1 forbids exactly that:
// projections MUST NOT infer state from an HTTP code or items.length
// independently.
//
// Containment rule: this module classifies EXACTLY as the server-side
// synthetic does (internal/graphsynthetic/synthetic.go classifyStatus)
// and names states with the server's own closed vocabulary. It reads
// ONLY the typed error code from the envelope — never the message, the
// field, or any other body content (CWE-200). The Go drift test at
// web/pwa/tests/graph_activation_state_test.go pins this file's
// vocabulary against the Go constants so the two cannot diverge.
//
// Auth: same-origin HttpOnly cookie. This module MUST NOT touch
// localStorage/sessionStorage/indexedDB/CacheStorage — the storage
// guard at web/pwa/tests/assistant_storage_guard_test.go fails on any
// such reference.

// ---------------------------------------------------------------------
// Closed vocabulary — mirrors internal/graphsynthetic/result.go and
// internal/api/graph_readiness.go. Adding a member here without adding
// it in Go (or vice versa) fails the drift test.
// ---------------------------------------------------------------------

// Aggregate states the product publishes (graphsynthetic.AggregateState).
export const AGGREGATE_AVAILABLE = "available";
export const AGGREGATE_DEGRADED = "degraded";
export const AGGREGATE_UNAVAILABLE = "unavailable";
export const AGGREGATE_POLICY_DISABLED = "policy_disabled";

// Closed per-read diagnostic codes (graphsynthetic Code*).
export const CODE_OK = "OK";
export const CODE_EMPTY_PERMITTED = "F080-SYNTH-EMPTY-PERMITTED";
export const CODE_EMPTY_NOT_PERMITTED = "F080-SYNTH-EMPTY-NOT-PERMITTED";
export const CODE_UNAUTHENTICATED = "F080-SYNTH-UNAUTHENTICATED";
export const CODE_FORBIDDEN = "F080-SYNTH-FORBIDDEN";
export const CODE_ROUTE_ABSENT = "F080-SYNTH-ROUTE-ABSENT";
export const CODE_CAPABILITY_DISABLED = "F080-SYNTH-CAPABILITY-DISABLED";
export const CODE_STORE_UNAVAILABLE = "F080-SYNTH-STORE-UNAVAILABLE";
export const CODE_SERVER_ERROR = "F080-SYNTH-SERVER-ERROR";
export const CODE_SCHEMA_INVALID = "F080-SYNTH-SCHEMA-INVALID";
export const CODE_CURSOR_INVALID = "F080-SYNTH-CURSOR-INVALID";
export const CODE_ROW_MISSING = "F080-SYNTH-ROW-MISSING";
export const CODE_TRANSPORT = "F080-SYNTH-TRANSPORT";
export const CODE_UNEXPECTED_STATUS = "F080-SYNTH-UNEXPECTED-STATUS";
export const CODE_POLICY_DISABLED = "F080-SYNTH-POLICY-DISABLED";
export const CODE_FAMILY_MISSING = "F080-SYNTH-FAMILY-MISSING";
export const CODE_OPTIONAL_OMITTED = "F080-SYNTH-OPTIONAL-OMITTED";

// Typed graphapi envelope codes this module is permitted to read.
const ENVELOPE_CAPABILITY_DISABLED = "capability_disabled";
const ENVELOPE_STORE_UNAVAILABLE = "store_unavailable";
const ENVELOPE_INVALID_CURSOR = "invalid_cursor";
const ENVELOPE_SCHEMA_ERROR = "schema_error";
// A DETAIL read whose row is gone. graphapi answers 404 with this typed
// code (internal/api/graphapi/{topics,people,places}.go), whereas a
// genuinely unmounted route yields a BARE 404 with no envelope at all.
// That difference is the only honest discriminator available here.
const ENVELOPE_NOT_FOUND = "not_found";

// ---------------------------------------------------------------------
// Closed EXCLUSIVE UI states. Every read resolves to exactly one. The
// resolver is total (see resolveReadState) so no read can fall through
// into a neighbouring state.
// ---------------------------------------------------------------------
export const STATE_LOADING = "loading";
export const STATE_READY = "ready";
export const STATE_TRUE_EMPTY = "true-empty";
export const STATE_PARTIAL = "partial";
export const STATE_DEGRADED = "degraded";
export const STATE_DISABLED = "disabled";
export const STATE_UNAUTHENTICATED = "unauthenticated";
export const STATE_FORBIDDEN = "forbidden";
export const STATE_ROUTE_ABSENT = "route-absent";
export const STATE_STORE_UNAVAILABLE = "store-unavailable";
export const STATE_SCHEMA_INVALID = "schema-invalid";

export const ALL_STATES = [
  STATE_LOADING,
  STATE_READY,
  STATE_TRUE_EMPTY,
  STATE_PARTIAL,
  STATE_DEGRADED,
  STATE_DISABLED,
  STATE_UNAUTHENTICATED,
  STATE_FORBIDDEN,
  STATE_ROUTE_ABSENT,
  STATE_STORE_UNAVAILABLE,
  STATE_SCHEMA_INVALID,
];

// States that mean "the caller's identity failed", after which any
// previously rendered private content MUST be gone before paint.
const PRIVACY_CLEARING_STATES = new Set([
  STATE_UNAUTHENTICATED,
  STATE_FORBIDDEN,
]);

// Exact user-facing copy per state. Kept here (not at call sites) so two
// surfaces cannot describe the same state differently. No copy embeds a
// request path, a status line, or any server message — a value-safe
// closed code is the only diagnostic surfaced.
const STATE_COPY = {
  [STATE_LOADING]: {
    message: "Loading your knowledge graph…",
    action: "",
  },
  [STATE_READY]: {
    message: "",
    action: "",
  },
  [STATE_TRUE_EMPTY]: {
    message:
      "Nothing has been synthesized into your knowledge graph yet. Connect a source or capture something, and topics, people, places and time will appear here.",
    action: "Add a source",
    actionHref: "/pwa/connectors.html",
  },
  [STATE_PARTIAL]: {
    message:
      "Some optional parts of your knowledge graph were not included in this view. What is shown below is complete and verified.",
    action: "",
  },
  [STATE_DEGRADED]: {
    message:
      "Part of your knowledge graph could not be read, so this view is incomplete. Nothing shown below is invented — it is only what was verified.",
    action: "Try this view again",
  },
  [STATE_DISABLED]: {
    message:
      "The knowledge graph is turned off for this deployment. This is a deliberate configuration, not a fault, and there is nothing to retry.",
    action: "",
  },
  [STATE_UNAUTHENTICATED]: {
    message: "Your session has ended. Sign in again to view your knowledge graph.",
    action: "Sign in",
    actionHref: "/pwa/index.html",
  },
  [STATE_FORBIDDEN]: {
    message:
      "This account is not permitted to read the knowledge graph. Ask the operator to grant knowledge graph access.",
    action: "",
  },
  [STATE_ROUTE_ABSENT]: {
    message:
      "This knowledge graph view is not available in the running build. It is not empty — it is not deployed.",
    action: "",
  },
  [STATE_STORE_UNAVAILABLE]: {
    message:
      "The knowledge graph store is temporarily unreachable. Your data is not lost; this view can be retried.",
    action: "Try this view again",
  },
  [STATE_SCHEMA_INVALID]: {
    message:
      "The knowledge graph returned a response this build cannot read. Retrying will not help until the mismatch is resolved.",
    action: "",
  },
};

// ---------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------

// classifyStatus mirrors internal/graphsynthetic/synthetic.go
// classifyStatus exactly, including its typed-envelope disambiguation of
// 503 and 4xx/5xx. `envelopeCode` is the already-extracted
// `error.code`, or "" when the body was absent or unparseable.
export function classifyStatus(status, envelopeCode) {
  const typed = envelopeCode || "";
  switch (status) {
    case 401:
      return CODE_UNAUTHENTICATED;
    case 403:
      return CODE_FORBIDDEN;
    case 404:
      // A missing ROW is not an absent ROUTE. When the server sends the
      // typed `not_found` envelope the route plainly exists — it just
      // answered that this id is gone. Reporting that as route-absent
      // told the user "it is not deployed", which is false, and offered
      // no recovery action because route-absent deliberately has none.
      // Bare 404s (no envelope) still mean the route really is absent.
      // This mirrors the 503 disambiguation below, and the identical
      // correction the Go model already makes in
      // internal/graphsynthetic/synthetic.go ("A populated list whose
      // own first row 404s is a missing row, not an absent route.").
      return typed === ENVELOPE_NOT_FOUND ? CODE_ROW_MISSING : CODE_ROUTE_ABSENT;
    case 400:
      return typed === ENVELOPE_INVALID_CURSOR ? CODE_CURSOR_INVALID : CODE_SCHEMA_INVALID;
    case 503:
      if (typed === ENVELOPE_CAPABILITY_DISABLED) return CODE_CAPABILITY_DISABLED;
      if (typed === ENVELOPE_STORE_UNAVAILABLE) return CODE_STORE_UNAVAILABLE;
      return CODE_SERVER_ERROR;
    default:
      break;
  }
  if (status >= 500) {
    return typed === ENVELOPE_SCHEMA_ERROR ? CODE_SCHEMA_INVALID : CODE_SERVER_ERROR;
  }
  return CODE_UNEXPECTED_STATUS;
}

// readEnvelopeCode extracts ONLY `error.code` from a response body. Any
// parse failure yields "" — never a message fragment.
async function readEnvelopeCode(resp) {
  try {
    const body = await resp.clone().json();
    if (body && body.error && typeof body.error.code === "string") return body.error.code;
  } catch (_) {
    // Unparseable or empty body: the status alone drives classification.
  }
  return "";
}

// resolveReadState maps a closed code plus the read's own row count to
// exactly one exclusive UI state. It is TOTAL: the final return is a
// conservative degraded rather than a fall-through to ready, so an
// unrecognised code can never be presented as a working graph.
export function resolveReadState(code, itemCount, emptyPermitted) {
  switch (code) {
    case CODE_UNAUTHENTICATED:
      return STATE_UNAUTHENTICATED;
    case CODE_FORBIDDEN:
      return STATE_FORBIDDEN;
    case CODE_ROUTE_ABSENT:
      return STATE_ROUTE_ABSENT;
    case CODE_CAPABILITY_DISABLED:
    case CODE_POLICY_DISABLED:
      return STATE_DISABLED;
    case CODE_STORE_UNAVAILABLE:
      return STATE_STORE_UNAVAILABLE;
    case CODE_SCHEMA_INVALID:
    case CODE_CURSOR_INVALID:
      return STATE_SCHEMA_INVALID;
    case CODE_OPTIONAL_OMITTED:
      return STATE_PARTIAL;
    case CODE_EMPTY_NOT_PERMITTED:
    case CODE_FAMILY_MISSING:
    case CODE_ROW_MISSING:
    case CODE_SERVER_ERROR:
    case CODE_TRANSPORT:
    case CODE_UNEXPECTED_STATUS:
      return STATE_DEGRADED;
    case CODE_EMPTY_PERMITTED:
      return STATE_TRUE_EMPTY;
    case CODE_OK:
      if (itemCount > 0) return STATE_READY;
      return emptyPermitted ? STATE_TRUE_EMPTY : STATE_DEGRADED;
    default:
      return STATE_DEGRADED;
  }
}

// ---------------------------------------------------------------------
// Activation — the truthful published aggregate
// ---------------------------------------------------------------------

// readGraphActivation returns the `graph` section of /api/health, which
// is the ONE aggregate every surface reads. Returning null means "no
// authoritative activation available" (the section is inside the
// authenticated branch, so an unauthenticated caller never sees it);
// callers then fall back to their own read outcome rather than
// inventing an activation claim.
export async function readGraphActivation() {
  try {
    const resp = await fetch("/api/health", {
      method: "GET",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });
    if (!resp.ok) return null;
    const body = await resp.json();
    if (!body || !body.graph) return null;
    return body.graph;
  } catch (_) {
    return null;
  }
}

// activationDisablesJourney reports whether the published aggregate says
// the Graph journey is deliberately off. Only an explicit
// policy_disabled aggregate counts — an unproven or degraded deployment
// is NOT the same claim and must not be rendered as "turned off".
export function activationDisablesJourney(graphSection) {
  return !!graphSection && graphSection.state === AGGREGATE_POLICY_DISABLED;
}

// ---------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------

// purgePrivateRegion synchronously removes prior personal graph content —
// DOM nodes, accessibility records, counts and labels — from a region.
// Called BEFORE an identity-failure state paints so no prior private
// label survives behind the recovery UI (SCN-080-001-06).
export function purgePrivateRegion(region) {
  if (!region) return;
  while (region.firstChild) region.removeChild(region.firstChild);
  region.removeAttribute("aria-label");
  region.removeAttribute("data-linked-artifact-count");
  region.removeAttribute("data-people-count");
  region.removeAttribute("data-place-count");
  region.hidden = true;
}

// renderReadState paints exactly one state into `statusNode`, replacing
// whatever was there. Replacement (not append) is what keeps a retry
// from stacking stale alerts, and what guarantees a single announcement.
//
// `onRetry`, when supplied AND permitted for the state, wires a
// read-only single-flight retry button. States with no honest recovery
// (disabled, forbidden, route-absent, schema-invalid) never get one.
export function renderReadState(statusNode, state, options) {
  if (!statusNode) return state;
  const opts = options || {};
  const copy = STATE_COPY[state] || STATE_COPY[STATE_DEGRADED];

  if (PRIVACY_CLEARING_STATES.has(state)) {
    for (const region of opts.privateRegions || []) purgePrivateRegion(region);
  }

  while (statusNode.firstChild) statusNode.removeChild(statusNode.firstChild);

  // A ready view has no status message to announce at all.
  if (state === STATE_READY) {
    statusNode.hidden = true;
    statusNode.removeAttribute("role");
    statusNode.removeAttribute("aria-live");
    statusNode.setAttribute("data-graph-state", state);
    return state;
  }

  statusNode.hidden = false;
  statusNode.setAttribute("data-graph-state", state);
  if (opts.code) statusNode.setAttribute("data-graph-code", opts.code);

  // Exactly one live region per paint. Faults alert; non-faults are
  // polite status. Two roles are never set at once, so a screen reader
  // receives a single announcement.
  const isFault =
    state === STATE_DEGRADED ||
    state === STATE_STORE_UNAVAILABLE ||
    state === STATE_SCHEMA_INVALID ||
    state === STATE_UNAUTHENTICATED ||
    state === STATE_FORBIDDEN ||
    state === STATE_ROUTE_ABSENT;
  statusNode.setAttribute("role", isFault ? "alert" : "status");
  statusNode.setAttribute("aria-live", isFault ? "assertive" : "polite");
  statusNode.className = isFault ? "status error" : "status";

  const message = document.createElement("p");
  message.className = "graph-state-message";
  message.textContent = copy.message;
  statusNode.appendChild(message);

  if (copy.actionHref) {
    const link = document.createElement("a");
    link.className = "btn graph-state-action";
    link.setAttribute("href", copy.actionHref);
    link.textContent = copy.action;
    statusNode.appendChild(link);
  } else if (copy.action && typeof opts.onRetry === "function") {
    const button = document.createElement("button");
    button.className = "btn graph-state-action";
    button.setAttribute("type", "button");
    button.setAttribute("data-graph-retry", state);
    button.textContent = copy.action;
    // Single-flight: the button disables itself for the duration of the
    // read it triggers, so repeated activation cannot stack reads.
    button.addEventListener("click", async () => {
      if (button.disabled) return;
      button.disabled = true;
      button.setAttribute("aria-disabled", "true");
      try {
        await opts.onRetry();
      } finally {
        if (button.isConnected) {
          button.disabled = false;
          button.setAttribute("aria-disabled", "false");
        }
      }
    });
    statusNode.appendChild(button);
  }

  return state;
}

// ---------------------------------------------------------------------
// The single read entry point
// ---------------------------------------------------------------------

// readGraphJSON performs one authorized graph read and returns
// { state, code, body }. It never throws for a server-expressed
// outcome: a non-OK response is classified, not raised, which is what
// stops call sites from re-deriving state from an exception string.
// A transport failure is the one genuinely client-side outcome and maps
// to the closed transport code.
export async function readGraphJSON(path, opts) {
  const options = opts || {};
  let resp;
  try {
    resp = await fetch(path, {
      method: "GET",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });
  } catch (_) {
    return { state: resolveReadState(CODE_TRANSPORT, 0, false), code: CODE_TRANSPORT, body: null };
  }

  if (!resp.ok) {
    const envelopeCode = await readEnvelopeCode(resp);
    const code = classifyStatus(resp.status, envelopeCode);
    return { state: resolveReadState(code, 0, false), code, body: null };
  }

  let body = null;
  try {
    body = await resp.json();
  } catch (_) {
    return {
      state: resolveReadState(CODE_SCHEMA_INVALID, 0, false),
      code: CODE_SCHEMA_INVALID,
      body: null,
    };
  }

  // A 200 that the typed validator rejects is a schema mismatch, not an
  // empty result — collapsing the two is how a broken contract used to
  // read as "you have no data".
  if (typeof options.validate === "function") {
    try {
      body = options.validate(body);
    } catch (_) {
      return {
        state: resolveReadState(CODE_SCHEMA_INVALID, 0, false),
        code: CODE_SCHEMA_INVALID,
        body: null,
      };
    }
  }

  const count = typeof options.countOf === "function" ? options.countOf(body) : 0;
  const emptyPermitted = options.emptyPermitted !== false;
  return { state: resolveReadState(CODE_OK, count, emptyPermitted), code: CODE_OK, body };
}
