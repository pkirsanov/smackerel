// Spec 073 SCOPE-073-05 SCN-073-B02 — People index + detail.
//
// BUG-080-001 SCOPE-04: state resolution lives in the shared closed model
// (/pwa/wiki_state.js), not at this call site.
import { validatePeopleList, validatePersonDetail, validateEdgesList } from "/pwa/generated/wiki_graph_v1.js";
import { clearChildren, el, markReady, renderCrossLinkList } from "/pwa/wiki_lib.js";
import {
  readGraphJSON, readGraphActivation, activationDisablesJourney, renderReadState,
  STATE_READY, STATE_DISABLED,
} from "/pwa/wiki_state.js";

async function loadIndex() {
  const section = document.getElementById("wiki-people-index");
  const status = document.getElementById("wiki-people-status");
  const list = document.getElementById("wiki-people-list");

  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    clearChildren(list);
    renderReadState(status, STATE_DISABLED, { privateRegions: [list] });
    markReady(section);
    return;
  }

  const result = await readGraphJSON("/api/people?limit=50", {
    validate: validatePeopleList,
    countOf: (b) => (b && b.items ? b.items.length : 0),
  });
  if (result.state !== STATE_READY) {
    clearChildren(list);
    list.hidden = true;
    renderReadState(status, result.state, { code: result.code, privateRegions: [list], onRetry: loadIndex });
    markReady(section);
    return;
  }

  clearChildren(list);
  for (const p of result.body.items) {
    list.appendChild(el("li", { class: "wiki-list-item", "data-person-id": p.id },
      el("a", { href: "/pwa/wiki_people.html?id=" + encodeURIComponent(p.id), class: "wiki-list-name" }, p.displayName),
      el("span", { class: "wiki-list-counts", "data-artifact-count": p.artifactCount }, " — " + p.artifactCount + " artifacts"),
    ));
  }
  list.hidden = false;
  renderReadState(status, STATE_READY, { code: result.code });
  markReady(section);
}

async function loadDetail(id) {
  document.getElementById("wiki-people-index").hidden = true;
  const section = document.getElementById("wiki-person-detail");
  section.hidden = false;
  const status = document.getElementById("wiki-person-status");
  const heading = document.getElementById("wiki-person-detail-heading");
  const tl = document.getElementById("wiki-person-timeline");
  const topics = document.getElementById("wiki-person-topics");
  const places = document.getElementById("wiki-person-places");
  const edges = document.getElementById("wiki-person-edges");
  const privateRegions = [tl, topics, places, edges];

  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    heading.textContent = "";
    heading.removeAttribute("data-person-id");
    renderReadState(status, STATE_DISABLED, { privateRegions });
    markReady(section);
    return;
  }

  const result = await readGraphJSON("/api/people/" + encodeURIComponent(id), {
    validate: validatePersonDetail,
    countOf: () => 1,
  });
  if (result.state !== STATE_READY) {
    // The heading carries a person's display name — personal content that
    // must not survive an identity failure.
    heading.textContent = "";
    heading.removeAttribute("data-person-id");
    renderReadState(status, result.state, { code: result.code, privateRegions, onRetry: () => loadDetail(id) });
    markReady(section);
    return;
  }

  const detail = result.body;
  heading.textContent = detail.displayName;
  heading.setAttribute("data-person-id", detail.id);
  for (const region of privateRegions) region.hidden = false;
  clearChildren(tl);
  for (const entry of detail.artifactTimeline) {
    tl.appendChild(el("li", { class: "wiki-timeline-entry", "data-artifact-id": entry.artifactId, "data-captured-at": entry.capturedAt },
      el("a", { href: "/pwa/wiki_artifact.html?id=" + encodeURIComponent(entry.artifactId) }, entry.title || entry.artifactId),
      el("time", { datetime: entry.capturedAt, class: "wiki-timeline-date" }, " — " + entry.capturedAt),
    ));
  }
  renderCrossLinkList(topics, detail.relatedTopics);
  renderCrossLinkList(places, detail.relatedPlaces);

  const edgesResult = await readGraphJSON("/api/graph/edges?source=person:" + encodeURIComponent(id) + "&limit=50", {
    validate: validateEdgesList,
    countOf: (b) => (b && b.items ? b.items.length : 0),
  });
  if (edgesResult.state === STATE_READY) {
    renderCrossLinkList(edges, edgesResult.body.items);
  } else {
    clearChildren(edges);
    renderReadState(edges, edgesResult.state, { code: edgesResult.code, onRetry: () => loadDetail(id) });
  }

  renderReadState(status, STATE_READY, { code: result.code });
  markReady(section);
}

const id = new URLSearchParams(window.location.search).get("id");
if (id) loadDetail(id); else loadIndex();
