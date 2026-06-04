// Spec 073 SCOPE-073-05 SCN-073-B01 — Topics index + detail.
// Hits GET /api/topics, GET /api/topics/{id}, GET /api/graph/edges.
// Cross-link reasons are projected verbatim from the server.
import {
  validateTopicsList,
  validateTopicDetail,
  validateEdgesList,
} from "/pwa/generated/wiki_graph_v1.js";
import {
  apiGetJSON, clearChildren, el, markReady, renderError,
  renderCrossLinkList,
} from "/pwa/wiki_lib.js";

async function loadIndex() {
  const section = document.getElementById("wiki-topics-index");
  const status = document.getElementById("wiki-topics-status");
  const list = document.getElementById("wiki-topics-list");
  try {
    const raw = await apiGetJSON("/api/topics?limit=50");
    const body = validateTopicsList(raw);
    clearChildren(list);
    for (const t of body.items) {
      const li = el("li", { class: "wiki-list-item", "data-topic-id": t.id },
        el("a", { class: "wiki-list-name", href: "/pwa/wiki_topics.html?id=" + encodeURIComponent(t.id) }, t.label),
        el("span", { class: "wiki-list-counts", "data-linked-artifact-count": t.linkedArtifactCount, "data-people-count": t.peopleCount, "data-place-count": t.placeCount },
          " — " + t.linkedArtifactCount + " artifacts · " + t.peopleCount + " people · " + t.placeCount + " places")
      );
      list.appendChild(li);
    }
    status.hidden = true;
    list.hidden = false;
    markReady(section);
  } catch (e) {
    renderError(status, e);
    markReady(section);
  }
}

async function loadDetail(id) {
  document.getElementById("wiki-topics-index").hidden = true;
  const section = document.getElementById("wiki-topic-detail");
  section.hidden = false;
  const status = document.getElementById("wiki-topic-status");
  const heading = document.getElementById("wiki-topic-detail-heading");
  try {
    const raw = await apiGetJSON("/api/topics/" + encodeURIComponent(id));
    const detail = validateTopicDetail(raw);
    heading.textContent = detail.label;
    heading.setAttribute("data-topic-id", detail.id);
    renderCrossLinkList(document.getElementById("wiki-topic-artifacts"), detail.linkedArtifacts);
    renderCrossLinkList(document.getElementById("wiki-topic-people"), detail.relatedPeople);
    renderCrossLinkList(document.getElementById("wiki-topic-places"), detail.relatedPlaces);
    // Universal cross-links via /api/graph/edges (SCN-073-B05).
    try {
      const edgesRaw = await apiGetJSON("/api/graph/edges?source=topic:" + encodeURIComponent(id) + "&limit=50");
      const edges = validateEdgesList(edgesRaw);
      renderCrossLinkList(document.getElementById("wiki-topic-edges"), edges.items);
    } catch (e) {
      renderError(document.getElementById("wiki-topic-edges"), e);
    }
    status.hidden = true;
    markReady(section);
  } catch (e) {
    renderError(status, e);
    markReady(section);
  }
}

const params = new URLSearchParams(window.location.search);
const topicID = params.get("id");
if (topicID) {
  loadDetail(topicID);
} else {
  loadIndex();
}
