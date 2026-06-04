// Spec 073 SCOPE-073-05 SCN-073-B02 — People index + detail.
import { validatePeopleList, validatePersonDetail, validateEdgesList } from "/pwa/generated/wiki_graph_v1.js";
import { apiGetJSON, clearChildren, el, markReady, renderError, renderCrossLinkList } from "/pwa/wiki_lib.js";

async function loadIndex() {
  const section = document.getElementById("wiki-people-index");
  const status = document.getElementById("wiki-people-status");
  const list = document.getElementById("wiki-people-list");
  try {
    const body = validatePeopleList(await apiGetJSON("/api/people?limit=50"));
    clearChildren(list);
    for (const p of body.items) {
      list.appendChild(el("li", { class: "wiki-list-item", "data-person-id": p.id },
        el("a", { href: "/pwa/wiki_people.html?id=" + encodeURIComponent(p.id), class: "wiki-list-name" }, p.displayName),
        el("span", { class: "wiki-list-counts", "data-artifact-count": p.artifactCount }, " — " + p.artifactCount + " artifacts"),
      ));
    }
    status.hidden = true; list.hidden = false; markReady(section);
  } catch (e) { renderError(status, e); markReady(section); }
}

async function loadDetail(id) {
  document.getElementById("wiki-people-index").hidden = true;
  const section = document.getElementById("wiki-person-detail");
  section.hidden = false;
  const status = document.getElementById("wiki-person-status");
  const heading = document.getElementById("wiki-person-detail-heading");
  try {
    const detail = validatePersonDetail(await apiGetJSON("/api/people/" + encodeURIComponent(id)));
    heading.textContent = detail.displayName;
    heading.setAttribute("data-person-id", detail.id);
    const tl = document.getElementById("wiki-person-timeline");
    clearChildren(tl);
    for (const entry of detail.artifactTimeline) {
      tl.appendChild(el("li", { class: "wiki-timeline-entry", "data-artifact-id": entry.artifactId, "data-captured-at": entry.capturedAt },
        el("a", { href: "/pwa/wiki_artifact.html?id=" + encodeURIComponent(entry.artifactId) }, entry.title || entry.artifactId),
        el("time", { datetime: entry.capturedAt, class: "wiki-timeline-date" }, " — " + entry.capturedAt),
      ));
    }
    renderCrossLinkList(document.getElementById("wiki-person-topics"), detail.relatedTopics);
    renderCrossLinkList(document.getElementById("wiki-person-places"), detail.relatedPlaces);
    try {
      const edges = validateEdgesList(await apiGetJSON("/api/graph/edges?source=person:" + encodeURIComponent(id) + "&limit=50"));
      renderCrossLinkList(document.getElementById("wiki-person-edges"), edges.items);
    } catch (e) { renderError(document.getElementById("wiki-person-edges"), e); }
    status.hidden = true; markReady(section);
  } catch (e) { renderError(status, e); markReady(section); }
}

const id = new URLSearchParams(window.location.search).get("id");
if (id) loadDetail(id); else loadIndex();
