// Spec 073 SCOPE-073-05 Knowledge Graph Browse Surface — minimal
// JSON validators for the spec 080 Knowledge Graph Public API wire
// shapes. Hand-written (not yet covered by the assistant-turn codegen
// pipeline). Source of truth is internal/api/graphapi/{topics,people,
// places,time,edges}.go — see also tests/e2e/graphapi_e2e_test.go for
// the live contract. If the server contract changes, update this file
// AND the assertions in tests/e2e/wiki/* together; the storage guard
// scans this generated dir to ensure no bearer/session material is
// stashed in browser storage.

// Contract version. Bumped when any wire field is renamed/added/removed.
export const GRAPH_SCHEMA_VERSION = "v1";

function isObj(v) { return v !== null && typeof v === "object" && !Array.isArray(v); }
function isStr(v) { return typeof v === "string"; }
function isNum(v) { return typeof v === "number" && Number.isFinite(v); }
function isArr(v) { return Array.isArray(v); }

function requireFields(obj, fields, label) {
  if (!isObj(obj)) throw new Error(label + ": not an object");
  for (const f of fields) {
    if (!(f in obj)) throw new Error(label + ": missing field " + f);
  }
}

export function validateCrossLink(x) {
  requireFields(x, ["targetKind", "targetId", "targetLabel", "reason"], "CrossLink");
  if (!isStr(x.targetKind) || !x.targetKind) throw new Error("CrossLink.targetKind: empty");
  if (!isStr(x.targetId) || !x.targetId) throw new Error("CrossLink.targetId: empty");
  if (!isStr(x.targetLabel)) throw new Error("CrossLink.targetLabel: not string");
  if (!isStr(x.reason) || !x.reason) throw new Error("CrossLink.reason: empty");
  return x;
}

function validateList(body, itemValidator, label) {
  if (!isObj(body)) throw new Error(label + ": not object");
  if (!isArr(body.items)) throw new Error(label + ".items: not array");
  body.items.forEach((it, i) => {
    try { itemValidator(it); } catch (e) { throw new Error(label + ".items[" + i + "]: " + e.message); }
  });
  if ("nextCursor" in body && body.nextCursor !== "" && !isStr(body.nextCursor)) {
    throw new Error(label + ".nextCursor: not string");
  }
  return body;
}

export function validateTopicRow(t) {
  requireFields(t, ["id", "label", "linkedArtifactCount", "peopleCount", "placeCount"], "TopicRow");
  if (!isStr(t.id)) throw new Error("TopicRow.id");
  if (!isStr(t.label)) throw new Error("TopicRow.label");
  if (!isNum(t.linkedArtifactCount)) throw new Error("TopicRow.linkedArtifactCount");
  if (!isNum(t.peopleCount)) throw new Error("TopicRow.peopleCount");
  if (!isNum(t.placeCount)) throw new Error("TopicRow.placeCount");
  return t;
}
export const validateTopicsList = (b) => validateList(b, validateTopicRow, "TopicsList");

export function validateTopicDetail(d) {
  requireFields(d, ["id", "label", "linkedArtifacts", "relatedPeople", "relatedPlaces"], "TopicDetail");
  d.linkedArtifacts.forEach(validateCrossLink);
  d.relatedPeople.forEach(validateCrossLink);
  d.relatedPlaces.forEach(validateCrossLink);
  return d;
}

export function validatePersonRow(p) {
  requireFields(p, ["id", "displayName", "artifactCount"], "PersonRow");
  if (!isStr(p.id) || !isStr(p.displayName) || !isNum(p.artifactCount)) {
    throw new Error("PersonRow fields");
  }
  return p;
}
export const validatePeopleList = (b) => validateList(b, validatePersonRow, "PeopleList");

export function validatePersonDetail(d) {
  requireFields(d, ["id", "displayName", "artifactTimeline", "relatedTopics", "relatedPlaces"], "PersonDetail");
  d.relatedTopics.forEach(validateCrossLink);
  d.relatedPlaces.forEach(validateCrossLink);
  d.artifactTimeline.forEach((e, i) => {
    if (!isStr(e.artifactId) || !isStr(e.title) || !isStr(e.capturedAt)) {
      throw new Error("PersonDetail.artifactTimeline[" + i + "]");
    }
  });
  return d;
}

export function validatePlaceRow(p) {
  requireFields(p, ["id", "displayName", "artifactCount", "source"], "PlaceRow");
  if (!isStr(p.id) || !isStr(p.displayName) || !isNum(p.artifactCount) || !isStr(p.source)) {
    throw new Error("PlaceRow fields");
  }
  return p;
}
export const validatePlacesList = (b) => validateList(b, validatePlaceRow, "PlacesList");

export function validatePlaceDetail(d) {
  requireFields(d, ["id", "displayName", "linkedArtifacts"], "PlaceDetail");
  // location is nullable
  if (d.location !== null && d.location !== undefined) {
    if (!isNum(d.location.lat) || !isNum(d.location.lon)) {
      throw new Error("PlaceDetail.location fields");
    }
  }
  d.linkedArtifacts.forEach(validateCrossLink);
  return d;
}

export function validateTimeResponse(b) {
  requireFields(b, ["days"], "TimeResponse");
  if (!isArr(b.days)) throw new Error("TimeResponse.days: not array");
  b.days.forEach((d, i) => {
    if (!isStr(d.date)) throw new Error("TimeResponse.days[" + i + "].date");
    if (!isArr(d.artifacts)) throw new Error("TimeResponse.days[" + i + "].artifacts");
  });
  return b;
}

export const validateEdgesList = (b) => validateList(b, validateCrossLink, "EdgesList");
