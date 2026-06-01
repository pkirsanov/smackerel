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
      if (s && typeof s.url === "string" && s.url.length > 0) {
        const a = document.createElement("a");
        a.href = s.url;
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
  const resp = await fetch(ENDPOINT, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "Accept": "application/json",
    },
    body: JSON.stringify(requestBody),
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
}

async function dispatchTurn(requestBody) {
  validateTurnRequest(requestBody);
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
