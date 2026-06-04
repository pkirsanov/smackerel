// Spec 073 SCOPE-073-05 SCN-073-B03 — Places index + detail.
import { validatePlacesList, validatePlaceDetail, validateEdgesList } from "/pwa/generated/wiki_graph_v1.js";
import { apiGetJSON, clearChildren, el, markReady, renderError, renderCrossLinkList } from "/pwa/wiki_lib.js";

async function loadIndex() {
  const section = document.getElementById("wiki-places-index");
  const status = document.getElementById("wiki-places-status");
  const list = document.getElementById("wiki-places-list");
  try {
    const body = validatePlacesList(await apiGetJSON("/api/places?limit=50"));
    clearChildren(list);
    for (const p of body.items) {
      list.appendChild(el("li", { class: "wiki-list-item", "data-place-id": p.id, "data-source": p.source },
        el("a", { href: "/pwa/wiki_places.html?id=" + encodeURIComponent(p.id), class: "wiki-list-name" }, p.displayName),
        el("span", { class: "wiki-list-counts", "data-artifact-count": p.artifactCount }, " — " + p.artifactCount + " artifacts · source: " + p.source),
      ));
    }
    status.hidden = true; list.hidden = false; markReady(section);
  } catch (e) { renderError(status, e); markReady(section); }
}

async function loadDetail(id) {
  document.getElementById("wiki-places-index").hidden = true;
  const section = document.getElementById("wiki-place-detail");
  section.hidden = false;
  const status = document.getElementById("wiki-place-status");
  const heading = document.getElementById("wiki-place-detail-heading");
  try {
    const detail = validatePlaceDetail(await apiGetJSON("/api/places/" + encodeURIComponent(id)));
    heading.textContent = detail.displayName;
    heading.setAttribute("data-place-id", detail.id);
    const loc = document.getElementById("wiki-place-location");
    if (detail.location) {
      loc.textContent = "Location: " + detail.location.lat + ", " + detail.location.lon;
      loc.setAttribute("data-lat", String(detail.location.lat));
      loc.setAttribute("data-lon", String(detail.location.lon));
      loc.hidden = false;
    }
    renderCrossLinkList(document.getElementById("wiki-place-artifacts"), detail.linkedArtifacts);
    try {
      const edges = validateEdgesList(await apiGetJSON("/api/graph/edges?source=place:" + encodeURIComponent(id) + "&limit=50"));
      renderCrossLinkList(document.getElementById("wiki-place-edges"), edges.items);
    } catch (e) { renderError(document.getElementById("wiki-place-edges"), e); }
    status.hidden = true; markReady(section);
  } catch (e) { renderError(status, e); markReady(section); }
}

const id = new URLSearchParams(window.location.search).get("id");
if (id) loadDetail(id); else loadIndex();
