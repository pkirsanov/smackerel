// Spec 073 SCOPE-073-02 — Web Chat Vertical Slice.
//
// Same-origin HttpOnly cookie session (ratified 2026-06-01).
// This module MUST NOT touch localStorage, sessionStorage, indexedDB,
// IDBFactory, caches.open, caches.match, or CacheStorage — the static
// guard at web/pwa/tests/assistant_storage_guard_test.go (TP-073-06,
// TP-073-12) fails on any such reference.
//
// SCN-073-A01: POST /api/assistant/turn with credentials: 'same-origin'.
// SCN-073-A03: retry reuses the original transport_message_id.
// SCN-073-A09: ARIA live region announces responses; keyboard order is
//   composer (tabindex=1) → send (2) → disambig choices (3) → confirm
//   accept/decline (4) → retry (5).

import {
  SCHEMA_VERSION,
  validateTurnRequest,
  validateTurnResponse,
} from "/pwa/generated/assistant_turn_v1.js";

const ENDPOINT = "/api/assistant/turn";
const TRANSPORT_HINT_WEB = "web";

// In-memory only. Never persisted. Holds the most-recent attempted
// turn so the retry button reuses the same transport_message_id and
// the same request body until the user edits the composer.
let pendingTurn = null;

// Guards against overlapping turns: only ONE turn may be in flight at a
// time so a rapid double-submit (Send button or Enter key) cannot fire
// two turns with different transport_message_ids — the SCN-073
// idempotency contract is "one logical turn per user action". Also
// drives the composer busy state (disabled Send + aria-busy).
let inFlight = false;

// Client-side request timeout (ms). A hung or unreachable assistant
// endpoint must not leave the composer frozen with no feedback; the
// fetch is aborted after this budget and surfaced as a retryable error.
// This is a client UX constant, not environment-specific runtime config.
const TURN_TIMEOUT_MS = 30000;

function el(id) {
  const node = document.getElementById(id);
  if (!node) {
    throw new Error("assistant: required DOM node missing: #" + id);
  }
  return node;
}

function newTransportMessageID() {
  // RFC4122-ish stable per-attempt id. crypto.randomUUID is available
  // in all supported browsers; same-origin only, no external CDN.
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return "web-" + crypto.randomUUID();
  }
  // Fallback: time + random bits. Still locally unique.
  return "web-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 10);
}

function clearChildren(node) {
  while (node.firstChild) {
    node.removeChild(node.firstChild);
  }
}

// Security (spec 076 security sweep): citation URLs come from web-search
// tool results, which the server treats as UNTRUSTED — it sanitises
// snippet text (internal/assistant/openknowledge/web/sanitize.go) but
// passes result URLs through verbatim (web_search.go: "this tool does not
// re-validate URLs"; the cite-back verifier only lower-cases the scheme).
// A "javascript:" / "data:" / "vbscript:" URL assigned to <a href> would
// execute in the authenticated PWA origin on click (DOM-XSS, OWASP A03).
// safeHref returns the URL only when it carries an http(s) scheme; any
// other scheme yields "" so the caller renders plain text (attribution
// preserved, link neutralised). Locked by
// tests/assistant_source_href_security_guard_test.go.
function safeHref(rawURL) {
  if (typeof rawURL !== "string" || rawURL.length === 0) {
    return "";
  }
  let parsed;
  try {
    parsed = new URL(rawURL, window.location.origin);
  } catch (_e) {
    return "";
  }
  if (parsed.protocol === "https:" || parsed.protocol === "http:") {
    return rawURL;
  }
  return "";
}

function appendTranscriptRow(role, text) {
  const li = document.createElement("li");
  li.className = "assistant-transcript-row assistant-transcript-" + role;
  const label = document.createElement("span");
  label.className = "assistant-transcript-role";
  label.textContent = role === "user" ? "You" : "Assistant";
  const body = document.createElement("span");
  body.className = "assistant-transcript-body";
  body.textContent = text;
  li.appendChild(label);
  li.appendChild(body);
  el("assistant-transcript").appendChild(li);
}

function renderResponse(response) {
  const responseNode = el("assistant-response");
  const sourcesNode = el("assistant-sources");
  const controlsNode = el("assistant-controls");
  const errorNode = el("assistant-error");
  const retryBtn = el("assistant-retry-btn");

  clearChildren(responseNode);
  clearChildren(sourcesNode);
  clearChildren(controlsNode);
  errorNode.classList.add("hidden");
  errorNode.textContent = "";

  // Body text. Render-by-shape: nothing branches on scenario id,
  // capture_route, or transport_hint.
  if (typeof response.body === "string" && response.body.length > 0) {
    const p = document.createElement("p");
    p.className = "assistant-response-body";
    p.textContent = response.body;
    responseNode.appendChild(p);
  }

  // Sources (citations).
  const sources = Array.isArray(response.sources) ? response.sources : [];
  if (sources.length > 0) {
    const ul = document.createElement("ul");
    ul.className = "assistant-sources-list";
    for (const s of sources) {
      const li = document.createElement("li");
      const title = (s && typeof s.title === "string") ? s.title : "(untitled source)";
      const href = safeHref(s && s.url);
      if (href) {
        const a = document.createElement("a");
        a.href = href;
        a.textContent = title;
        a.rel = "noopener noreferrer";
        li.appendChild(a);
      } else {
        li.textContent = title;
      }
      ul.appendChild(li);
    }
    sourcesNode.appendChild(ul);
  }

  // Disambiguation choices.
  let nextTabIndex = 3;
  const dis = response.disambiguation_prompt;
  if (dis && typeof dis === "object" && Array.isArray(dis.choices)) {
    const ref = String(dis.disambiguation_ref || "");
    for (const c of dis.choices) {
      if (!c || typeof c !== "object") continue;
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "btn btn-secondary assistant-choice-btn";
      btn.dataset.disambigRef = ref;
      btn.dataset.choiceNumber = String(c.number);
      btn.tabIndex = nextTabIndex;
      btn.textContent = String(c.label || ("Choice " + c.number));
      btn.addEventListener("click", () => submitDisambiguationChoice(ref, c.number));
      controlsNode.appendChild(btn);
    }
  }

  // Confirm card (accept/decline pair, in canonical order).
  const conf = response.confirm_card;
  if (conf && typeof conf === "object") {
    const ref = String(conf.confirm_ref || "");
    const accept = document.createElement("button");
    accept.type = "button";
    accept.className = "btn btn-primary assistant-confirm-accept";
    accept.dataset.confirmRef = ref;
    accept.tabIndex = 4;
    accept.textContent = String(conf.positive_label || "Confirm");
    accept.addEventListener("click", () => submitConfirm(ref, true));
    controlsNode.appendChild(accept);

    const decline = document.createElement("button");
    decline.type = "button";
    decline.className = "btn btn-secondary assistant-confirm-decline";
    decline.dataset.confirmRef = ref;
    decline.tabIndex = 4;
    decline.textContent = String(conf.negative_label || "Cancel");
    decline.addEventListener("click", () => submitConfirm(ref, false));
    controlsNode.appendChild(decline);
  }

  // Error / retry affordance.
  if (typeof response.error_cause === "string" && response.error_cause.length > 0) {
    errorNode.textContent = "Assistant error: " + response.error_cause;
    errorNode.classList.remove("hidden");
    retryBtn.classList.remove("hidden");
  } else {
    retryBtn.classList.add("hidden");
  }

  // Capture-as-fallback acknowledgement is conveyed through body/copy
  // already populated above; capture_route is server-decided telemetry.
  if (typeof response.capture_route === "string" && response.capture_route.length > 0) {
    responseNode.dataset.captureRoute = response.capture_route;
  }

  // Spec 075 / 076 SCOPE-6c — legacy-retirement notice rendered as a
  // one-line addendum AFTER the primary body. Never replaces the body;
  // never branches on scenario id. Same canonical copy shape as the
  // WhatsApp renderer (LegacyRetirementNoticeAddendum): the descriptor
  // projection at web/pwa/lib/render_descriptor_v1.js carries the
  // replacement_example verbatim, and this consumer matches the
  // transport-side phrasing the server-owned ledger dedupes against.
  const notice = response.notice;
  if (notice && typeof notice === "object" && !Array.isArray(notice)) {
    const cmd = typeof notice.command === "string" ? notice.command.trim() : "";
    const ex = typeof notice.replacement_example === "string" ? notice.replacement_example.trim() : "";
    if (cmd.length > 0 && ex.length > 0) {
      const p = document.createElement("p");
      p.className = "assistant-notice";
      p.dataset.copyKey = typeof notice.copy_key === "string" ? notice.copy_key : "";
      p.dataset.windowId = typeof notice.window_id === "string" ? notice.window_id : "";
      p.textContent = "Heads up: " + cmd + " is retiring \u2014 try \"" + ex + "\" instead.";
      responseNode.appendChild(p);
    }
  }
}

function showLocalError(message) {
  const errorNode = el("assistant-error");
  const retryBtn = el("assistant-retry-btn");
  errorNode.textContent = message;
  errorNode.classList.remove("hidden");
  retryBtn.classList.remove("hidden");
}

async function postTurn(requestBody) {
  // credentials: 'same-origin' carries the spec 070 HttpOnly session
  // cookie. The bearer token is never read or sent by this client.
  // An AbortController bounds the request (TURN_TIMEOUT_MS) so a hung or
  // unreachable endpoint cannot leave the turn pending forever.
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), TURN_TIMEOUT_MS);
  try {
    const resp = await fetch(ENDPOINT, {
      method: "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "Accept": "application/json",
      },
      body: JSON.stringify(requestBody),
      signal: controller.signal,
    });
    const text = await resp.text();
    if (!resp.ok) {
      throw new Error("HTTP " + resp.status + ": " + text.slice(0, 256));
    }
    let parsed;
    try {
      parsed = JSON.parse(text);
    } catch (err) {
      throw new Error("response is not valid JSON: " + err.message);
    }
    validateTurnResponse(parsed);
    return parsed;
  } catch (err) {
    if (err && err.name === "AbortError") {
      throw new Error("request timed out after " + Math.round(TURN_TIMEOUT_MS / 1000) + "s");
    }
    throw err;
  } finally {
    clearTimeout(timer);
  }
}

// setComposerBusy toggles the in-flight UI affordance: the Send button is
// disabled and aria-busy is announced while a turn is in flight, then
// restored when it settles. This is the visible half of the inFlight
// guard (the correctness half is the early-return in dispatchTurn).
function setComposerBusy(busy) {
  const sendBtn = el("assistant-send-btn");
  sendBtn.disabled = busy;
  if (busy) {
    sendBtn.setAttribute("aria-busy", "true");
  } else {
    sendBtn.removeAttribute("aria-busy");
  }
}

async function dispatchTurn(requestBody) {
  validateTurnRequest(requestBody);
  // Single-flight: ignore a new dispatch while one is already running so
  // overlapping turns with distinct transport_message_ids cannot be
  // created (SCN-073 idempotency: one logical turn per user action).
  if (inFlight) {
    return;
  }
  inFlight = true;
  setComposerBusy(true);
  pendingTurn = requestBody;
  try {
    const response = await postTurn(requestBody);
    renderResponse(response);
    // Successful turn — clear pendingTurn only after we have a definitive
    // server response. Transport-level failures (timeout, 5xx) leave
    // pendingTurn intact so retry reuses the same transport_message_id.
    pendingTurn = null;
    el("assistant-retry-btn").classList.add("hidden");
  } catch (err) {
    showLocalError("Network or server error. You can retry the same turn.");
    // pendingTurn intentionally retained for the retry button.
    throw err;
  } finally {
    inFlight = false;
    setComposerBusy(false);
  }
}

function buildTextRequest(text, transportMessageID) {
  return {
    schema_version: SCHEMA_VERSION,
    transport_message_id: transportMessageID,
    kind: "text",
    transport_hint: TRANSPORT_HINT_WEB,
    text: text,
    confirm_ref: "",
    confirm_choice: "",
    disambiguation_ref: "",
    disambiguation_choice: 0,
    client_context: "",
  };
}

function buildConfirmRequest(ref, accepted, transportMessageID) {
  return {
    schema_version: SCHEMA_VERSION,
    transport_message_id: transportMessageID,
    kind: "confirm",
    transport_hint: TRANSPORT_HINT_WEB,
    text: "",
    confirm_ref: ref,
    confirm_choice: accepted ? "accept" : "decline",
    disambiguation_ref: "",
    disambiguation_choice: 0,
    client_context: "",
  };
}

function buildDisambiguationRequest(ref, number, transportMessageID) {
  return {
    schema_version: SCHEMA_VERSION,
    transport_message_id: transportMessageID,
    kind: "disambiguation",
    transport_hint: TRANSPORT_HINT_WEB,
    text: "",
    confirm_ref: "",
    confirm_choice: "",
    disambiguation_ref: ref,
    disambiguation_choice: number,
    client_context: "",
  };
}

async function submitText(text) {
  appendTranscriptRow("user", text);
  const req = buildTextRequest(text, newTransportMessageID());
  try {
    await dispatchTurn(req);
  } catch (_err) {
    // Already surfaced via showLocalError; retry button reuses pendingTurn.
  }
}

async function submitConfirm(ref, accepted) {
  const req = buildConfirmRequest(ref, accepted, newTransportMessageID());
  try {
    await dispatchTurn(req);
  } catch (_err) {
    // surfaced
  }
}

async function submitDisambiguationChoice(ref, number) {
  const req = buildDisambiguationRequest(ref, number, newTransportMessageID());
  try {
    await dispatchTurn(req);
  } catch (_err) {
    // surfaced
  }
}

async function retryPending() {
  if (!pendingTurn) return;
  // Same transport_message_id, same body. Server deduplicates.
  try {
    await dispatchTurn(pendingTurn);
  } catch (_err) {
    // surfaced
  }
}

function wireEvents() {
  const form = el("assistant-composer-form");
  const input = el("assistant-composer-input");
  const retryBtn = el("assistant-retry-btn");

  form.addEventListener("submit", (ev) => {
    ev.preventDefault();
    // A turn is already in flight — ignore the submit without clearing the
    // composer, so the user does not silently lose their typed text and no
    // overlapping turn is dispatched.
    if (inFlight) return;
    const text = String(input.value || "").trim();
    if (text.length === 0) return;
    input.value = "";
    submitText(text);
  });

  // User edits to the composer invalidate the pending retry: a freshly
  // typed message must mint a new transport_message_id.
  input.addEventListener("input", () => {
    pendingTurn = null;
    retryBtn.classList.add("hidden");
  });

  retryBtn.addEventListener("click", () => {
    retryPending();
  });
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", wireEvents);
} else {
  wireEvents();
}
