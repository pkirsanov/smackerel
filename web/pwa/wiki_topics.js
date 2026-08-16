// Spec 073 SCOPE-073-05 SCN-073-B01 — Topics index + detail.
// Hits GET /api/topics, GET /api/topics/{id}, GET /api/graph/edges.
// Cross-link reasons are projected verbatim from the server.
//
// BUG-080-001 SCOPE-04: state resolution moved OUT of this file and into
// the shared closed model in /pwa/wiki_state.js. This page no longer
// branches on an HTTP code or on items.length — it renders whichever
// single exclusive state the model resolved.
import {
  validateTopicsList,
  validateTopicDetail,
  validateEdgesList,
} from "/pwa/generated/wiki_graph_v1.js";
import { clearChildren, el, markReady, renderCrossLinkList } from "/pwa/wiki_lib.js";
import {
  readGraphJSON,
  readGraphActivation,
  activationDisablesJourney,
  renderReadState,
  STATE_READY,
  STATE_DISABLED,
} from "/pwa/wiki_state.js";

async function loadIndex() {
  const section = document.getElementById("wiki-topics-index");
  const status = document.getElementById("wiki-topics-status");
  const list = document.getElementById("wiki-topics-list");

  // The published aggregate is authoritative for "deliberately off", so a
  // disabled deployment says so instead of reporting an empty graph.
  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    clearChildren(list);
    renderReadState(status, STATE_DISABLED, { privateRegions: [list] });
    markReady(section);
    return;
  }

  const result = await readGraphJSON("/api/topics?limit=50", {
    validate: validateTopicsList,
    countOf: (body) => (body && body.items ? body.items.length : 0),
  });

  if (result.state !== STATE_READY) {
    clearChildren(list);
    list.hidden = true;
    renderReadState(status, result.state, {
      code: result.code,
      privateRegions: [list],
      onRetry: loadIndex,
    });
    markReady(section);
    return;
  }

  clearChildren(list);
  for (const t of result.body.items) {
    const li = el("li", { class: "wiki-list-item", "data-topic-id": t.id },
      el("a", { class: "wiki-list-name", href: "/pwa/wiki_topics.html?id=" + encodeURIComponent(t.id) }, t.label),
      el("span", { class: "wiki-list-counts", "data-linked-artifact-count": t.linkedArtifactCount, "data-people-count": t.peopleCount, "data-place-count": t.placeCount },
        " — " + t.linkedArtifactCount + " artifacts · " + t.peopleCount + " people · " + t.placeCount + " places")
    );
    list.appendChild(li);
  }
  list.hidden = false;
  renderReadState(status, STATE_READY, { code: result.code });
  markReady(section);
}

async function loadDetail(id) {
  document.getElementById("wiki-topics-index").hidden = true;
  const section = document.getElementById("wiki-topic-detail");
  section.hidden = false;
  const status = document.getElementById("wiki-topic-status");
  const heading = document.getElementById("wiki-topic-detail-heading");
  const artifacts = document.getElementById("wiki-topic-artifacts");
  const people = document.getElementById("wiki-topic-people");
  const places = document.getElementById("wiki-topic-places");
  const edges = document.getElementById("wiki-topic-edges");
  // Every region holding personal graph content, so an identity failure
  // purges all of them before the recovery UI paints.
  const privateRegions = [artifacts, people, places, edges];

  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    heading.textContent = "";
    heading.removeAttribute("data-topic-id");
    renderReadState(status, STATE_DISABLED, { privateRegions });
    markReady(section);
    return;
  }

  const result = await readGraphJSON("/api/topics/" + encodeURIComponent(id), {
    validate: validateTopicDetail,
    countOf: () => 1,
  });

  if (result.state !== STATE_READY) {
    // The heading carries the topic label, which is personal content and
    // must not survive an identity failure.
    heading.textContent = "";
    heading.removeAttribute("data-topic-id");
    renderReadState(status, result.state, {
      code: result.code,
      privateRegions,
      onRetry: () => loadDetail(id),
    });
    markReady(section);
    return;
  }

  const detail = result.body;
  heading.textContent = detail.label;
  heading.setAttribute("data-topic-id", detail.id);
  for (const region of privateRegions) region.hidden = false;
  renderCrossLinkList(artifacts, detail.linkedArtifacts);
  renderCrossLinkList(people, detail.relatedPeople);
  renderCrossLinkList(places, detail.relatedPlaces);

  // Universal cross-links via /api/graph/edges (SCN-073-B05). This is a
  // second, independent read: its own outcome renders in its own region
  // so an edges failure cannot mislabel the topic itself.
  const edgesResult = await readGraphJSON(
    "/api/graph/edges?source=topic:" + encodeURIComponent(id) + "&limit=50",
    { validate: validateEdgesList, countOf: (b) => (b && b.items ? b.items.length : 0) },
  );
  if (edgesResult.state === STATE_READY) {
    renderCrossLinkList(edges, edgesResult.body.items);
  } else {
    clearChildren(edges);
    renderReadState(edges, edgesResult.state, {
      code: edgesResult.code,
      onRetry: () => loadDetail(id),
    });
  }

  renderReadState(status, STATE_READY, { code: result.code });
  markReady(section);
}

const params = new URLSearchParams(window.location.search);
const topicID = params.get("id");
if (topicID) {
  loadDetail(topicID);
} else {
  loadIndex();
}
