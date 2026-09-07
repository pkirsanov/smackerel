#!/usr/bin/env python3
"""Provider-neutral IMP-054 research runtime built on ECF v2 and MBE.

Research semantics live here. Canonical bytes, immutable objects, append-only
records, locking, recovery, admission, budgets, occurrences, and usage remain
owned by the imported ECF and MBE modules.
"""
from __future__ import annotations

import argparse
import datetime as dt
import importlib.util
import json
import os
import re
import selectors
import signal
import stat
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, NoReturn

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent


def _module(name: str, path: Path) -> Any:
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"required module unavailable: {name}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


ecf = _module("bubbles_research_ecf_v2", HERE / "execution-control-store.py")
mbe = _module("bubbles_research_mbe", HERE / "measured-budget-runtime.py")

VERSION = 1
RUNTIME_VERSION = "imp-054-v1"
REGISTRY_PATH = ROOT / "registry" / "research-runtime.json"
STAGES_PATH = ROOT / "registry" / "research-stages.yaml"
MAX_SOURCE_BYTES = 1_048_576
MAX_CANDIDATE_BYTES = 262_144
MAX_RECORDS = 1024
ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
TIMESTAMP = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$")
CLASSIFICATIONS = ("public", "internal", "confidential", "restricted")
MATERIALITIES = ("routine", "material", "high-stakes", "consequentially-prohibited")
COVERAGE_STATES = ("satisfied", "missing", "stale", "conflicted", "unavailable")
VERDICTS = ("proceed", "high-assurance", "human", "refuse")
RELATIONS = ("support", "rebuttal", "context")
SELECTORS = ("segment-byte-range", "line-range", "json-pointer", "table-cell-range")
STAGES = (
    "question-contract", "deterministic-plan", "bounded-acquisition",
    "normalization-extraction", "claim-evidence-ledger",
    "conflict-coverage-analysis", "materiality-risk-gate",
    "optional-synthesis", "deterministic-validation", "immutable-publication",
)
PREFIXES = {
    "ResearchQuestion": "rq", "ResearchRun": "rr", "SourceRecord": "src",
    "NormalizedSource": "nrm", "ClaimRecord": "clm", "EvidenceLink": "evl",
    "ConflictSet": "cfs", "CoverageAssessment": "cov",
    "MaterialityAssessment": "mat", "StageReceipt": "str",
    "ModelRouteDecision": "mrd", "ResearchArtifact": "art",
    "ValidationReceipt": "val", "RunManifest": "rrm",
}

IDENTITY_FIELDS = {
    "ResearchQuestion": "questionId",
    "ResearchRun": "runId",
    "SourceRecord": "sourceId",
    "NormalizedSource": "normalizedSourceId",
    "ClaimRecord": "claimId",
    "EvidenceLink": "linkId",
    "ConflictSet": "conflictId",
    "CoverageAssessment": "coverageId",
    "MaterialityAssessment": "materialityAssessmentId",
    "StageReceipt": "stageReceiptId",
    "ModelRouteDecision": "modelRouteDecisionId",
    "ResearchArtifact": "artifactId",
    "ValidationReceipt": "validationReceiptId",
    "RunManifest": "manifestId",
}
RECORD_FIELDS = {
    "ResearchQuestion": {"contractType","schemaVersion","questionId","question","decisionContext","scope","exclusions","materiality","coveragePolicy","sourcePolicy","dataClassification","egressPolicy","publicationPolicy","consequenceBoundary","asOf","policies"},
    "ResearchRun": {"contractType","schemaVersion","runId","questionId","policyDigest","planDigest","budgetId","state","createdAt","manifestId","runtimeVersion"},
    "SourceRecord": {"contractType","schemaVersion","sourceId","runId","locator","publisher","retrievedAt","transport","mediaType","contentDigest","normalizedDigest","freshness","classification","acquisitionReceiptId"},
    "NormalizedSource": {"contractType","schemaVersion","normalizedSourceId","sourceId","contentDigest","mediaType","canonicalContent","segments"},
    "ClaimRecord": {"contractType","schemaVersion","claimId","questionId","statement","subject","predicate","object","polarity","time","scope","unit","scale","modality","materiality","originStageId","statusBasis"},
    "EvidenceLink": {"contractType","schemaVersion","linkId","claimId","sourceId","relation","selector","excerptDigest","extractionMethod","confidenceBasis","boundSlots","authorityClass","freshnessState"},
    "ConflictSet": {"contractType","schemaVersion","conflictId","claimIds","evidenceLinkIds","conflictType","materiality","resolutionBasis","state"},
    "CoverageAssessment": {"contractType","schemaVersion","coverageId","questionId","dimensions","materialClaimIds","state","findingCodes"},
    "MaterialityAssessment": {"contractType","schemaVersion","materialityAssessmentId","questionId","coverageId","conflictIds","classification","materiality","verdict","reasonCodes"},
    "StageReceipt": {"contractType","schemaVersion","stageReceiptId","runId","stageId","inputFingerprint","registryDigest","runtimeDigest","dependencyDigests","questionId","policyDigest","mbeBudgetId","admissionDecisionId","adapterDigest","freshnessDigest","inputManifestDigest","status","outputDigests","usageReceiptIds","occurrenceId","attemptId","startedAt","finishedAt","error"},
    "ModelRouteDecision": {"contractType","schemaVersion","modelRouteDecisionId","runId","stageId","adapterId","routeClass","classification","materiality","mbeBudgetId","admissionDecisionId","permitId","occurrenceId","attemptId","usageReceiptIds","accountingState","unresolvedHold","verdict","reasonCode"},
    "ResearchArtifact": {"contractType","schemaVersion","artifactId","runId","questionId","claimIds","conflictIds","sourceIds","citationIndex","coverageId","materialityAssessmentId","limitations","body","bodyDigest","validationReceiptId","publicationTarget","consequenceBoundary"},
    "ValidationReceipt": {"contractType","schemaVersion","validationReceiptId","runId","artifactDigest","manifestDigest","checks","verdict","findingCodes","validatedAt"},
    "RunManifest": {"contractType","schemaVersion","manifestId","runId","recordDigests","stageReceiptIds","policyDigest","budgetSummaryDigest","artifactId","predecessorManifestId","state","sealedAt"},
}
FORBIDDEN_MODEL_KEYS = {"command","commands","argv","url","urls","tool","tools","provider","providerId","endpoint","routeSwitch","budget","publicationTarget"}
TELEMETRY_FORBIDDEN = ("question","source","claim","prompt","output","url","endpoint","credential","provider","privatepath","private_path","locator","statement","body")
PROJECT_CONFIG_PATHS = (Path(".github/bubbles-project.yaml"), Path("bubbles-project.yaml"))


class ResearchError(RuntimeError):
    def __init__(self, code: str, kind: str, message: str, exit_code: int = 1) -> None:
        super().__init__(message)
        self.code, self.kind, self.exit_code = code, kind, exit_code


def fail(code: str, kind: str, message: str, exit_code: int = 1) -> NoReturn:
    raise ResearchError(code, kind, message, exit_code)


def malformed(message: str) -> NoReturn:
    fail("RER-INPUT-INVALID", "input", message, 2)


def exact(value: Any, names: set[str], label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        malformed(f"{label} must be an object")
    actual = set(value)
    if actual != names:
        malformed(f"{label} field mismatch; missing={sorted(names-actual)}, unknown={sorted(actual-names)}")
    return value

def telemetry_projection(run_id: str, stage_id: str, status: str, receipt_ids: list[str]) -> dict[str, Any]:
    projection = {"contractType":"research-telemetry","schemaVersion":1,"runId":run_id,"stageId":stage_id,
                  "stageClass":"research-runtime","adapterClass":"closed","routeClass":"repository",
                  "status":status,"durationMs":0,"counts":[len(receipt_ids)],
                  "digests":[typed("research-stage-receipt-set", sorted(receipt_ids))],"errorCode":None}
    validate_telemetry(projection)
    return projection


def bridge_projection(bridge_id: str, artifact: Any) -> dict[str, Any]:
    bridges = load_registry()["bridges"]
    enum(bridge_id, tuple(bridges), "bridgeId")
    record = validate_record(artifact)
    if record["contractType"] != "ResearchArtifact" or record["validationReceiptId"] is None:
        fail("RER-BRIDGE-INVALID", "bridge", "bridge input must be a validation-bound research artifact")
    return {"contractType":"research-bridge-result","schemaVersion":1,"bridgeId":bridge_id,
            "state":bridges[bridge_id],"artifactId":record["artifactId"],"activation":"unavailable"}


def identifier(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.isascii() or not ID.fullmatch(value):
        malformed(f"{label} violates the closed identifier grammar")
    return value


def digest(value: Any, label: str) -> str:
    if not isinstance(value, str) or not DIGEST.fullmatch(value):
        malformed(f"{label} must be a lowercase sha256 digest")
    return value


def timestamp(value: Any, label: str) -> str:
    if not isinstance(value, str) or not TIMESTAMP.fullmatch(value):
        malformed(f"{label} must use YYYY-MM-DDTHH:MM:SS.mmmZ")
    try:
        dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%S.%fZ")
    except ValueError:
        malformed(f"{label} is not a valid UTC instant")
    return value


def instant(value: str) -> dt.datetime:
    return dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=dt.timezone.utc)


def enum(value: Any, allowed: tuple[str, ...], label: str) -> str:
    if not isinstance(value, str) or value not in allowed:
        malformed(f"{label} is outside its closed vocabulary")
    return value


def integer(value: Any, label: str, low: int = 0, high: int = 9_007_199_254_740_991) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < low or value > high:
        malformed(f"{label} must be an integer in [{low}, {high}]")
    return value


def typed(kind: str, value: Any) -> str:
    return ecf.typed_digest(kind, value)


def identity_material(kind: str, record: dict[str, Any], identity: str) -> dict[str, Any]:
    material = {**record, identity: None}
    if kind == "ClaimRecord":
        material["statusBasis"] = []
    if kind == "ResearchArtifact":
        material["validationReceiptId"] = None
    return material


def identified(kind: str, payload: dict[str, Any], identity: str) -> dict[str, Any]:
    prefix = PREFIXES[kind]
    material = {"contractType": kind, "schemaVersion": VERSION, **payload, identity: None}
    material[identity] = f"{prefix}:{typed(kind, identity_material(kind, material, identity))[7:]}"
    return validate_record(material)


def load_registry() -> dict[str, Any]:
    try:
        value = json.loads(REGISTRY_PATH.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        fail("RER-INTERNAL", "registry", f"research runtime registry unavailable: {exc}", 2)
    exact(value, {"adapters","bridges","classifications","contractType","coverageStates","evidenceRelations","materialityVerdicts","operations","recordTypes","schemaVersion","selectorTypes","stageRegistry","telemetryFields","validationCheckOutcomes","validationChecks","version"}, "runtime registry")
    if value["contractType"] != "research-runtime-registry" or value["schemaVersion"] != 1:
        fail("RER-INTERNAL", "registry", "runtime registry contract mismatch", 2)
    if value["recordTypes"] != list(RECORD_FIELDS) or value["operations"] != list(parser_operations()):
        fail("RER-INTERNAL", "registry", "runtime registry is not executable-authority exact", 2)
    return value


def load_stages() -> list[dict[str, Any]]:
    lines = STAGES_PATH.read_text(encoding="utf-8").splitlines()
    rows: list[dict[str, Any]] = []
    current: dict[str, Any] | None = None
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("- id: "):
            if current is not None:
                rows.append(current)
            current = {"id": stripped[6:]}
        elif current is not None and stripped.startswith("ordinal: "):
            current["ordinal"] = int(stripped[9:])
        elif current is not None and stripped.startswith("class: "):
            current["class"] = stripped[7:]
        elif current is not None and stripped.startswith("predecessors: "):
            raw = stripped[14:].strip()
            current["predecessors"] = [] if raw == "[]" else [item.strip() for item in raw[1:-1].split(",")]
    if current is not None:
        rows.append(current)
    if len(rows) != 10 or tuple(row.get("id") for row in rows) != STAGES or [row.get("ordinal") for row in rows] != list(range(1, 11)):
        fail("RER-INTERNAL", "registry", "stage registry must contain the exact ten ordered stages", 2)
    known: set[str] = set()
    for row in rows:
        predecessors = row.get("predecessors")
        if not isinstance(predecessors, list) or any(item not in known for item in predecessors):
            fail("RER-INTERNAL", "registry", "stage registry is cyclic or forward-referencing", 2)
        known.add(row["id"])
    return rows


def validate_record(record: Any) -> dict[str, Any]:
    if not isinstance(record, dict) or record.get("contractType") not in RECORD_FIELDS:
        malformed("record contractType is outside the closed research vocabulary")
    kind = record["contractType"]
    exact(record, RECORD_FIELDS[kind], kind)
    if record["schemaVersion"] != VERSION:
        malformed(f"{kind}.schemaVersion is unsupported")
    identity_name = IDENTITY_FIELDS[kind]
    expected = f"{PREFIXES[kind]}:{typed(kind, identity_material(kind, record, identity_name))[7:]}"
    # Identities are derived with the identity field absent, represented by None during hashing.
    if record[identity_name] != expected:
        malformed(f"{kind} identity is not content-derived")
    return record


def _normalized_key(value: str) -> str:
    return re.sub(r"[^a-z0-9]", "", value.casefold())


def validate_telemetry(value: Any) -> None:
    if isinstance(value, dict):
        allowed = set(load_registry()["telemetryFields"])
        for key, item in value.items():
            if key not in allowed or any(token in _normalized_key(key) for token in TELEMETRY_FORBIDDEN):
                fail("RER-TELEMETRY-PRIVATE", "security", f"telemetry field is forbidden: {key}")
            validate_telemetry(item)
    elif isinstance(value, list):
        for item in value:
            validate_telemetry(item)
    elif isinstance(value, str):
        if value.startswith(("/home/", "/Users/")) or "://" in value or "-----BEGIN " in value:
            fail("RER-TELEMETRY-PRIVATE", "security", "telemetry contains private or endpoint-shaped data")


def resolve_project_config(project_root: str | None) -> dict[str, Any]:
    if project_root is None:
        return {"enabled": False, "configPath": None, "reason": "absent"}
    root = Path(project_root)
    if not root.is_absolute() or not root.is_dir():
        malformed("project root must be an existing absolute directory")
    candidates = [root / relative for relative in PROJECT_CONFIG_PATHS if (root / relative).is_file()]
    if len(candidates) > 1:
        fail("RER-ROUTE-AMBIGUOUS", "config", "multiple project configuration authorities contain a research runtime candidate")
    if not candidates:
        return {"enabled": False, "configPath": None, "reason": "absent"}
    path = candidates[0]
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeError) as exc:
        fail("RER-CONFIG-INVALID", "config", f"project configuration is unreadable: {exc}", 2)
    starts = [index for index, line in enumerate(lines) if line.startswith("researchRuntime:")]
    if not starts:
        return {"enabled": False, "configPath": str(path), "reason": "absent"}
    if len(starts) != 1 or lines[starts[0]].strip() != "researchRuntime:":
        fail("RER-CONFIG-INVALID", "config", "researchRuntime must be one top-level mapping", 2)
    block = []
    for line in lines[starts[0] + 1:]:
        if line and not line[0].isspace():
            break
        if line.strip() and not line.lstrip().startswith("#"):
            block.append(line)
    scalars: dict[str, str] = {}
    for line in block:
        if not line.startswith("  ") or line.startswith("   "):
            continue
        key, separator, raw = line.strip().partition(":")
        if separator and key in {"schemaVersion", "enabled"}:
            if key in scalars or not raw.strip():
                fail("RER-CONFIG-INVALID", "config", "researchRuntime activation fields are duplicate or empty", 2)
            scalars[key] = raw.strip()
    if set(scalars) != {"schemaVersion", "enabled"} or scalars["schemaVersion"] != "1" or scalars["enabled"] not in {"true", "false"}:
        fail("RER-CONFIG-INVALID", "config", "researchRuntime requires schemaVersion 1 and explicit boolean enabled", 2)
    return {"enabled": scalars["enabled"] == "true", "configPath": str(path), "reason": "configured"}


def validate_question(value: Any) -> dict[str, Any]:
    fields = RECORD_FIELDS["ResearchQuestion"] - {"contractType","schemaVersion","questionId"}
    question = exact(value, fields, "question")
    for name in ("question","decisionContext"):
        if not isinstance(question[name], str) or not question[name].strip():
            fail("RER-QUESTION-INVALID", "question", f"{name} is required")
    if question["consequenceBoundary"] != "no-direct-consequential-action":
        fail("RER-QUESTION-INVALID", "question", "consequence boundary must prohibit direct action")
    timestamp(question["asOf"], "asOf")
    enum(question["dataClassification"], CLASSIFICATIONS, "dataClassification")
    enum(question["materiality"], MATERIALITIES, "materiality")
    for name in ("scope","coveragePolicy","sourcePolicy","egressPolicy","publicationPolicy","policies"):
        if not isinstance(question[name], dict) or not question[name]:
            fail("RER-QUESTION-INVALID", "question", f"explicit non-empty {name} is required")
    if not isinstance(question["exclusions"], list):
        fail("RER-QUESTION-INVALID", "question", "exclusions must be an array")
    source_policy = question["sourcePolicy"]
    if set(source_policy) != {"allowedClasses","maximumAgeSecondsByClass","classificationByClass"}:
        fail("RER-QUESTION-INVALID", "question", "sourcePolicy requires explicit freshness and classification for each class")
    classes = source_policy["allowedClasses"]
    if not isinstance(classes, list) or not classes or len(classes) != len(set(classes)):
        fail("RER-QUESTION-INVALID", "question", "source classes must be a unique non-empty array")
    if set(source_policy["maximumAgeSecondsByClass"]) != set(classes) or set(source_policy["classificationByClass"]) != set(classes):
        fail("RER-QUESTION-INVALID", "question", "each source class requires exact freshness and classification policy")
    for source_class in classes:
        integer(source_policy["maximumAgeSecondsByClass"][source_class], "maximum source age", 0)
        enum(source_policy["classificationByClass"][source_class], CLASSIFICATIONS, "source classification")
    egress = question["egressPolicy"]
    if set(egress) != set(CLASSIFICATIONS) or any(not isinstance(egress[name], list) for name in CLASSIFICATIONS):
        fail("RER-QUESTION-INVALID", "question", "egressPolicy must explicitly map every classification")
    publication = question["publicationPolicy"]
    required_publication = {"audience","retention","citationPolicy","allowLedgerOnly","allowIncomplete","target"}
    if set(publication) != required_publication or not all(name in publication for name in required_publication):
        fail("RER-QUESTION-INVALID", "question", "publicationPolicy is incomplete")
    coverage = question["coveragePolicy"]
    if set(coverage) != {"requiredDimensions","contradictionTreatment"} or not isinstance(coverage["requiredDimensions"], list) or not coverage["requiredDimensions"]:
        fail("RER-QUESTION-INVALID", "question", "coveragePolicy is incomplete")
    return identified("ResearchQuestion", dict(question), "questionId")


class Runtime:
    def __init__(self, store_root: str) -> None:
        self.store = ecf.Store(store_root)
        self.registry = load_registry()
        self.stages = load_stages()
        self.registry_digest = typed("research-runtime-registry", self.registry)
        self.stage_registry_digest = typed("research-stage-registry", self.stages)
        source_digest = ecf.digest_bytes(Path(__file__).read_bytes())
        self.runtime_digest = typed("research-runtime-source", source_digest)

    def _records_locked(self) -> list[dict[str, Any]]:
        records = []
        for event in self.store.verified().events:
            extension = [item for item in event["extensions"] if item["namespace"] == "org.bubbles.research"]
            if extension:
                if len(extension) != 1 or extension[0]["payloadDigest"] != event["objectDigest"]:
                    fail("RER-INTERNAL", "integrity", "research extension binding is invalid", 2)
                records.append(self.store.require_object(event["objectDigest"]))
        return records

    def records(self) -> list[dict[str, Any]]:
        with self.store.locked():
            return self._records_locked()

    def put(self, record: dict[str, Any], subject_kind: str = "record") -> dict[str, Any]:
        validate_record(record)
        with self.store.locked():
            existing = self._records_locked()
            identity = record[IDENTITY_FIELDS[record["contractType"]]]
            same = [item for item in existing if item.get("contractType") == record["contractType"] and identity in item.values()]
            if same:
                if record in same:
                    return record
                identity_field = IDENTITY_FIELDS[record["contractType"]]
                if any(identity_material(record["contractType"], item, identity_field) != identity_material(record["contractType"], record, identity_field) for item in same):
                    fail("RER-PUBLICATION-CONFLICT", "conflict", "content identity already maps to different bytes", 4)
            stored = self.store.put_object(record)
            head = self.store.load_head()
            suffix = stored["objectDigest"][7:55]
            at = next((record.get(name) for name in ("finishedAt","validatedAt","sealedAt","createdAt","asOf","retrievedAt") if record.get(name)), "1970-01-01T00:00:00.000Z")
            proposal = {"attemptId":f"att:research.{suffix}","contractType":"execution-control-event","eventId":f"evt:research.{suffix}","eventType":"RECORD","extensions":[{"namespace":"org.bubbles.research","payloadDigest":stored["objectDigest"],"schemaVersion":1}],"objectDigest":stored["objectDigest"],"occurrenceId":f"occ:research.{suffix}","posture":"shadow","recordedAt":at,"schemaVersion":2,"subject":{"id":identity,"kind":f"x.org.bubbles.research-{subject_kind}"},"supersedesEventId":None}
            self.store.append(head["sequence"], head["eventDigest"], proposal)
            return record

    def find(self, kind: str, field: str, value: str) -> dict[str, Any]:
        rows = [row for row in self.records() if row.get("contractType") == kind and row.get(field) == value]
        if len(rows) != 1:
            fail("RER-IDENTITY-MISSING", "identity", f"exact {kind} identity unavailable")
        return rows[0]

    def plan(self, question: dict[str, Any], policies: dict[str, Any], budget_id: str, adapter: dict[str, Any]) -> dict[str, Any]:
        if not isinstance(policies, dict) or not policies:
            malformed("explicit policies are required")
        identifier(budget_id, "budgetId")
        exact(adapter, {"adapterId","mode","capabilityDigest","configPath"}, "adapter")
        identifier(adapter["adapterId"], "adapterId")
        enum(adapter["mode"], ("disabled","local-command"), "adapter.mode")
        digest(adapter["capabilityDigest"], "adapter.capabilityDigest")
        if adapter["mode"] == "local-command" and not isinstance(adapter["configPath"], str):
            malformed("local-command requires an explicit configPath")
        if adapter["mode"] == "disabled" and adapter["configPath"] is not None:
            malformed("disabled adapter cannot carry configPath")
        policy_digest = typed("research-policies", policies)
        stage_plan = [{"stageId":row["id"],"ordinal":row["ordinal"],"stageClass":row["class"],"predecessors":row["predecessors"]} for row in self.stages]
        plan = {"questionId":question["questionId"],"policyDigest":policy_digest,"budgetId":budget_id,"adapterId":adapter["adapterId"],"adapterMode":adapter["mode"],"stagePlan":stage_plan,"runtimeVersion":RUNTIME_VERSION}
        plan["planDigest"] = typed("research-plan", plan)
        plan["runId"] = "rr:" + typed("ResearchRun-key", {"questionId":question["questionId"],"policyDigest":policy_digest,"planDigest":plan["planDigest"],"budgetId":budget_id,"runtimeVersion":RUNTIME_VERSION})[7:]
        return plan

    def fingerprint(self, stage_id: str, question: dict[str, Any], plan: dict[str, Any], adapter: dict[str, Any], mbe_context: dict[str, Any], freshness: Any, input_manifest: list[str]) -> str:
        material = {"stageId":stage_id,"registryDigest":self.stage_registry_digest,"runtimeDigest":self.runtime_digest,"dependencyDigests":sorted(input_manifest),"questionId":question["questionId"],"policyDigest":plan["policyDigest"],"mbeBudgetId":plan["budgetId"],"admissionDecisionId":mbe_context["admissionDecisionId"],"adapterDigest":adapter["capabilityDigest"],"freshnessDigest":typed("research-freshness", freshness),"schemaVersion":VERSION,"inputManifestDigest":typed("research-input-manifest", input_manifest)}
        return typed("research-stage-fingerprint", material)

    def reusable(self, run_id: str, stage_id: str, fingerprint: str, as_of: str) -> dict[str, Any] | None:
        receipts = [row for row in self.records() if row.get("contractType") == "StageReceipt" and row["runId"] == run_id and row["stageId"] == stage_id and row["inputFingerprint"] == fingerprint and row["status"] == "accepted"]
        for receipt in receipts:
            try:
                for object_digest in receipt["outputDigests"]:
                    self.store.require_object(object_digest)
            except (OSError, ecf.EcfError):
                continue
            if stage_id == "bounded-acquisition":
                sources = [self.store.require_object(item) for item in receipt["outputDigests"]]
                if any(source["freshness"]["state"] != freshness_state(source["retrievedAt"], as_of, source["freshness"]["maximumAgeSeconds"]) for source in sources):
                    continue
            if receipt["error"] is None:
                return receipt
        return None

    def receipt(self, run_id: str, stage_id: str, fingerprint: str, question: dict[str, Any], plan: dict[str, Any], adapter: dict[str, Any], mbe_context: dict[str, Any], freshness: Any, inputs: list[str], outputs: list[dict[str, Any]], status: str = "accepted", error: str | None = None) -> dict[str, Any]:
        output_digests = []
        for output in outputs:
            self.put(output)
            output_digests.append(typed("execution-control-object", output))
        payload = {"runId":run_id,"stageId":stage_id,"inputFingerprint":fingerprint,"registryDigest":self.stage_registry_digest,"runtimeDigest":self.runtime_digest,"dependencyDigests":sorted(inputs),"questionId":question["questionId"],"policyDigest":plan["policyDigest"],"mbeBudgetId":plan["budgetId"],"admissionDecisionId":mbe_context["admissionDecisionId"],"adapterDigest":adapter["capabilityDigest"],"freshnessDigest":typed("research-freshness", freshness),"inputManifestDigest":typed("research-input-manifest", inputs),"status":status,"outputDigests":output_digests,"usageReceiptIds":mbe_context["usageReceiptIds"],"occurrenceId":mbe_context["occurrenceId"],"attemptId":mbe_context["attemptId"],"startedAt":question["asOf"],"finishedAt":question["asOf"],"error":error}
        result = identified("StageReceipt", payload, "stageReceiptId")
        return self.put(result, "stage")

    def _mbe_context(self, value: Any, adapter_mode: str) -> dict[str, Any]:
        context = exact(value, {"budgetId","admissionDecisionId","permitId","occurrenceId","attemptId","usageReceiptIds","accountingState","unresolvedHold"}, "mbeContext")
        identifier(context["budgetId"], "mbeContext.budgetId")
        for name in ("occurrenceId","attemptId"):
            identifier(context[name], name)
        if context["admissionDecisionId"] is not None:
            identifier(context["admissionDecisionId"], "admissionDecisionId")
        if context["permitId"] is not None:
            identifier(context["permitId"], "permitId")
        if not isinstance(context["usageReceiptIds"], list) or any(not isinstance(item, str) for item in context["usageReceiptIds"]):
            malformed("usageReceiptIds must be an identifier array")
        enum(context["accountingState"], ("terminal","unmeasured","unresolved"), "accountingState")
        if not isinstance(context["unresolvedHold"], bool):
            malformed("unresolvedHold must be boolean")
        if adapter_mode == "local-command":
            if context["admissionDecisionId"] is None or context["permitId"] is None:
                fail("RER-MBE-PREREQUISITE", "admission", "local model dispatch requires MBE decision and permit")
            runtime = mbe.MeasuredBudgetRuntime(str(self.store.root))
            records = runtime.records()

            def one(kind: str, field: str, target: str) -> dict[str, Any]:
                matches = [row for row in records if row.get("contractType") == kind and row.get(field) == target]
                if len(matches) != 1:
                    fail("RER-MBE-PREREQUISITE", "admission", f"exact persisted {kind} record is unavailable")
                return matches[0]

            policy = one("goal-budget-policy", "budgetId", context["budgetId"])
            permit = one("dispatch-permit", "permitId", context["permitId"])
            decision = one("admission-decision", "decisionId", context["admissionDecisionId"])
            intent = one("dispatch-intent", "intentId", permit["intentId"])
            epoch = one("session-epoch", "epochId", permit["epochId"])
            epoch_verification = one("epoch-verification", "epochVerificationId", decision["epochVerificationId"])
            negotiation = one("usage-negotiation", "negotiationId", decision["negotiationId"])
            quote = one("usage-quote", "quoteId", permit["quoteId"])
            reservation = one("budget-reservation", "reservationId", permit["reservationId"])
            consumption = one("permit-consumption", "permitId", permit["permitId"])
            expected = {name: permit[name] for name in ("goalId","budgetId","epochId","sessionIdentityId","occurrenceId","attemptId","actionDigest")}
            if policy["goalId"] != permit["goalId"] or context["occurrenceId"] != permit["occurrenceId"] or context["attemptId"] != permit["attemptId"]:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE goal, occurrence, or attempt binding differs")
            for record in (intent, decision, reservation, quote, consumption):
                if any(record.get(name) != expected_value for name, expected_value in expected.items()):
                    fail("RER-MBE-PREREQUISITE", "admission", "persisted MBE graph has cross-bound lineage")
            if decision["verdict"] != "permit-eligible" or permit["decisionId"] != decision["decisionId"] or decision["reservationId"] != reservation["reservationId"]:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE decision, reservation, and permit lineage differs")
            if negotiation["intentId"] != intent["intentId"] or quote["negotiationId"] != negotiation["negotiationId"] or reservation["quoteId"] != quote["quoteId"]:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE negotiation and quote lineage differs")
            if epoch["goalId"] != permit["goalId"] or epoch["budgetId"] != permit["budgetId"] or epoch["hostSessionIdentityId"] != permit["sessionIdentityId"]:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE epoch lineage differs")
            if epoch_verification["verdict"] != "verified" or epoch["epochVerificationId"] != epoch_verification["epochVerificationId"] or epoch["openedByBoundaryId"] != epoch_verification["boundaryId"]:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE epoch verification is not current and verified")
            for fact_ref in decision["factRefs"]:
                fact = one("admission-fact", "factId", fact_ref["factId"])
                if fact["intentId"] != intent["intentId"] or any(fact.get(name) != fact_ref.get(name) for name in fact_ref):
                    fail("RER-MBE-PREREQUISITE", "admission", "MBE admission fact lineage differs")
            receipts = [row for row in records if row.get("contractType") == "usage-receipt" and row.get("permitId") == permit["permitId"]]
            superseded_receipts = {row["supersedesReceiptId"] for row in receipts if row["supersedesReceiptId"] is not None}
            receipt_leaves = [row for row in receipts if row["usageReceiptId"] not in superseded_receipts]
            if len(receipt_leaves) != 1:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE usage receipt has no unique current leaf")
            receipt = receipt_leaves[0]
            if any(receipt.get(name) != expected_value for name, expected_value in expected.items()) or receipt["measurementStatus"] != "measured":
                fail("RER-MBE-PREREQUISITE", "admission", "MBE usage receipt is unmeasured or cross-bound")
            verifications = [row for row in records if row.get("contractType") == "usage-receipt-verification" and row.get("usageReceiptId") == receipt["usageReceiptId"] and row.get("revision") == receipt["revision"]]
            valid_verifications = [row for row in verifications if row.get("verdict") == "valid"]
            if len(valid_verifications) != 1:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE usage receipt lacks one valid verification")
            settlement_rows = [row for row in records if row.get("contractType") == "budget-settlement" and row.get("reservationId") == reservation["reservationId"]]
            superseded_settlements = {row["supersedesSettlementId"] for row in settlement_rows if row["supersedesSettlementId"] is not None}
            settlement_leaves = [row for row in settlement_rows if row["settlementId"] not in superseded_settlements]
            if len(settlement_leaves) != 1:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE settlement has no unique current leaf")
            settlement = settlement_leaves[0]
            if settlement["usageReceiptId"] != receipt["usageReceiptId"] or settlement["receiptVerificationId"] != valid_verifications[0]["verificationId"] or settlement["consumptionId"] != consumption["consumptionId"]:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE terminal settlement lineage differs")
            _, budget_state = runtime._budget_state(records, context["budgetId"])
            unresolved_hold = bool(budget_state["holds"])
            accounting_state = "terminal" if budget_state["reservations"].get(reservation["reservationId"], {}).get("state") == "reconciled" and not unresolved_hold else "unresolved"
            derived_usage_ids = [receipt["usageReceiptId"]]
            if context["usageReceiptIds"] != derived_usage_ids or context["accountingState"] != accounting_state or context["unresolvedHold"] != unresolved_hold:
                fail("RER-MBE-PREREQUISITE", "admission", "caller MBE assertions differ from persisted accounting")
            if accounting_state != "terminal" or unresolved_hold:
                fail("RER-MBE-PREREQUISITE", "admission", "MBE accounting must be terminal without a hold")
            return {**context, "usageReceiptIds": derived_usage_ids, "accountingState": accounting_state, "unresolvedHold": unresolved_hold}
        elif context["permitId"] is not None:
            fail("RER-MBE-PREREQUISITE", "admission", "disabled adapter cannot consume a permit")
        return dict(context)

    def run(self, request: Any, stop_after: str | None = None, required_validation_receipt_id: str | None = None) -> dict[str, Any]:
        req = exact(request, {"question","policies","budgetId","adapter","mbeContext","sources","claims","telemetry"}, "run request")
        question = validate_question(req["question"])
        self.put(question, "question")
        plan = self.plan(question, req["policies"], req["budgetId"], req["adapter"])
        mbe_context = self._mbe_context(req["mbeContext"], req["adapter"]["mode"])
        if mbe_context["budgetId"] != plan["budgetId"]:
            fail("RER-MBE-PREREQUISITE", "admission", "run and MBE budget identities differ")
        if not isinstance(req["sources"], list) or not isinstance(req["claims"], list):
            malformed("sources and claims must be arrays")
        validate_telemetry(req["telemetry"])
        run = identified("ResearchRun", {"questionId":question["questionId"],"policyDigest":plan["policyDigest"],"planDigest":plan["planDigest"],"budgetId":plan["budgetId"],"state":"running","createdAt":question["asOf"],"manifestId":None,"runtimeVersion":RUNTIME_VERSION}, "runId")
        # ResearchRun's normative key excludes mutable run state.
        run["runId"] = plan["runId"]
        validate_record_with_external_identity(run, "runId", plan["runId"])
        self.put_external_identity(run, "runId", plan["runId"], "run")
        stage_receipts: list[dict[str, Any]] = []
        manifest_inputs: list[str] = []
        stage_outputs: dict[str, list[dict[str, Any]]] = {}
        freshness_material: Any = question["sourcePolicy"]
        for stage in STAGES:
            if stage == "bounded-acquisition":
                freshness_material = [{"locatorDigest":typed("source-locator", source.get("locator")),"retrievedAt":source.get("retrievedAt"),"contentDigest":typed("source-input", source.get("content"))} for source in req["sources"]]
            fingerprint = self.fingerprint(stage, question, plan, req["adapter"], mbe_context, freshness_material, manifest_inputs)
            if stage == "immutable-publication" and required_validation_receipt_id is not None:
                validation = stage_outputs["deterministic-validation"][0]
                if validation["validationReceiptId"] != required_validation_receipt_id or validation["verdict"] != "accepted":
                    fail("RER-VALIDATION-FAILED", "validation", "publication requires the accepted validation receipt for the exact candidate")
            cached = self.reusable(plan["runId"], stage, fingerprint, question["asOf"])
            if cached is not None:
                outputs = [self.store.require_object(item) for item in cached["outputDigests"]]
                receipt = cached
            else:
                outputs = self.execute_stage(stage, question, plan, req, mbe_context, stage_outputs, manifest_inputs)
                receipt = self.receipt(plan["runId"], stage, fingerprint, question, plan, req["adapter"], mbe_context, freshness_material, manifest_inputs, outputs)
            stage_receipts.append(receipt)
            stage_outputs[stage] = outputs
            manifest_inputs = receipt["outputDigests"]
            if stop_after == stage:
                if stage == "deterministic-validation":
                    validation = outputs[0]
                    artifact = next(item for item in stage_outputs["optional-synthesis"] if item["contractType"] == "ResearchArtifact")
                    receipt_ids = [row["stageReceiptId"] for row in stage_receipts]
                    return {"contractType":"research-run-result","schemaVersion":1,"runId":plan["runId"],"state":"validated","artifactId":artifact["artifactId"],"validationReceiptId":validation["validationReceiptId"],"stageReceiptIds":receipt_ids,"telemetry":telemetry_projection(plan["runId"],stage,"validated",receipt_ids)}
                receipt_ids = [row["stageReceiptId"] for row in stage_receipts]
                return {"contractType":"research-run-result","schemaVersion":1,"runId":plan["runId"],"state":"paused","stageId":stage,"stageReceiptIds":receipt_ids,"telemetry":telemetry_projection(plan["runId"],stage,"paused",receipt_ids)}
        manifests = stage_outputs["immutable-publication"]
        publication_manifest = next(item for item in manifests if item["contractType"] == "RunManifest")
        artifact = next(item for item in manifests if item["contractType"] == "ResearchArtifact")
        receipt_ids = [row["stageReceiptId"] for row in stage_receipts]
        prior_terminal = [row for row in self.records() if row.get("contractType") == "RunManifest" and row.get("runId") == plan["runId"] and row.get("state") == "published" and row.get("stageReceiptIds") == sorted(receipt_ids)]
        if prior_terminal:
            manifest = prior_terminal[0]
        else:
            record_digests = sorted({item for receipt in stage_receipts for item in receipt["outputDigests"]})
            manifest = identified("RunManifest", {"runId":plan["runId"],"recordDigests":record_digests,"stageReceiptIds":sorted(receipt_ids),"policyDigest":plan["policyDigest"],"budgetSummaryDigest":typed("mbe-budget-summary", {"budgetId":mbe_context["budgetId"],"accountingState":mbe_context["accountingState"],"usageReceiptIds":mbe_context["usageReceiptIds"]}),"artifactId":artifact["artifactId"],"predecessorManifestId":publication_manifest["manifestId"],"state":"published","sealedAt":question["asOf"]}, "manifestId")
            self.put(manifest, "publication")
        receipt_ids = [row["stageReceiptId"] for row in stage_receipts]
        return {"contractType":"research-run-result","schemaVersion":1,"runId":plan["runId"],"state":"published","artifactId":artifact["artifactId"],"manifestId":manifest["manifestId"],"stageReceiptIds":receipt_ids,"telemetry":telemetry_projection(plan["runId"],"immutable-publication","published",receipt_ids),"parkedActivation":{"hostedProviders":"unavailable","downstreamBridges":"unavailable"}}

    def execute_stage(self, stage: str, question: dict[str, Any], plan: dict[str, Any], req: dict[str, Any], mbe_context: dict[str, Any], outputs: dict[str, list[dict[str, Any]]], inputs: list[str]) -> list[dict[str, Any]]:
        if stage == "question-contract":
            return [question]
        if stage == "deterministic-plan":
            return []
        if stage == "bounded-acquisition":
            return acquire_sources(plan["runId"], req["sources"], question, self)
        if stage == "normalization-extraction":
            return normalize_sources(req["sources"], outputs["bounded-acquisition"])
        if stage == "claim-evidence-ledger":
            normalized = outputs["normalization-extraction"]
            sources = outputs["bounded-acquisition"]
            return build_ledger(question, req["claims"], sources, normalized)
        if stage == "conflict-coverage-analysis":
            return analyze(question, outputs["claim-evidence-ledger"])
        if stage == "materiality-risk-gate":
            coverage = next(item for item in outputs["conflict-coverage-analysis"] if item["contractType"] == "CoverageAssessment")
            conflicts = [item for item in outputs["conflict-coverage-analysis"] if item["contractType"] == "ConflictSet"]
            return [materiality_gate(question, coverage, conflicts)]
        if stage == "optional-synthesis":
            assessment = outputs["materiality-risk-gate"][0]
            route = route_decision(plan["runId"], req["adapter"], question, assessment, mbe_context)
            if assessment["verdict"] == "refuse":
                return [route, candidate_artifact(plan["runId"], question, outputs, None, "refused")]
            if req["adapter"]["mode"] == "disabled":
                return [route, candidate_artifact(plan["runId"], question, outputs, None, "ledger-only")]
            candidate = local_command(req["adapter"]["configPath"], candidate_input(plan["runId"], question, outputs))
            return [route, candidate_artifact(plan["runId"], question, outputs, candidate, "synthesized")]
        if stage == "deterministic-validation":
            artifact = next(item for item in outputs["optional-synthesis"] if item["contractType"] == "ResearchArtifact")
            return [validate_artifact(question, artifact, outputs)]
        if stage == "immutable-publication":
            artifact = next(item for item in outputs["optional-synthesis"] if item["contractType"] == "ResearchArtifact")
            validation = outputs["deterministic-validation"][0]
            if validation["verdict"] != "accepted":
                fail("RER-VALIDATION-FAILED", "validation", ",".join(validation["findingCodes"]))
            artifact = {**artifact, "validationReceiptId": validation["validationReceiptId"]}
            validate_record(artifact)
            self.put(artifact, "publication")
            previous = [row for row in self.records() if row.get("contractType") == "RunManifest" and row["runId"] == plan["runId"]]
            record_digests = sorted({item for receipt in self.records() if receipt.get("contractType") == "StageReceipt" and receipt["runId"] == plan["runId"] for item in receipt["outputDigests"]})
            manifest = identified("RunManifest", {"runId":plan["runId"],"recordDigests":record_digests,"stageReceiptIds":sorted(row["stageReceiptId"] for row in self.records() if row.get("contractType") == "StageReceipt" and row["runId"] == plan["runId"]),"policyDigest":plan["policyDigest"],"budgetSummaryDigest":typed("mbe-budget-summary", {"budgetId":mbe_context["budgetId"],"accountingState":mbe_context["accountingState"],"usageReceiptIds":mbe_context["usageReceiptIds"]}),"artifactId":artifact["artifactId"],"predecessorManifestId":previous[-1]["manifestId"] if previous else None,"state":"publication-pending-terminal-snapshot","sealedAt":question["asOf"]}, "manifestId")
            self.put(manifest, "publication")
            return [artifact, manifest]
        fail("RER-INTERNAL", "stage", f"unsupported stage: {stage}", 2)

    def put_external_identity(self, record: dict[str, Any], field: str, expected: str, subject: str) -> dict[str, Any]:
        if record[field] != expected:
            malformed(f"{field} mismatch")
        # ECF still content-addresses complete bytes. The domain run key follows IMP-054's normative formula.
        with self.store.locked():
            existing = [row for row in self._records_locked() if row.get("contractType") == record["contractType"] and row.get(field) == expected]
            if existing:
                return existing[0]
            stored = self.store.put_object(record)
            head = self.store.load_head(); suffix = stored["objectDigest"][7:55]
            proposal={"attemptId":f"att:research.{suffix}","contractType":"execution-control-event","eventId":f"evt:research.{suffix}","eventType":"RECORD","extensions":[{"namespace":"org.bubbles.research","payloadDigest":stored["objectDigest"],"schemaVersion":1}],"objectDigest":stored["objectDigest"],"occurrenceId":f"occ:research.{suffix}","posture":"shadow","recordedAt":record["createdAt"],"schemaVersion":2,"subject":{"id":expected,"kind":f"x.org.bubbles.research-{subject}"},"supersedesEventId":None}
            self.store.append(head["sequence"],head["eventDigest"],proposal)
            return record


def validate_record_with_external_identity(record: dict[str, Any], field: str, expected: str) -> None:
    exact(record, RECORD_FIELDS[record["contractType"]], record["contractType"])
    if record[field] != expected:
        malformed(f"{field} is invalid")


def reidentify(record: dict[str, Any], field: str) -> dict[str, Any]:
    payload = {key:value for key,value in record.items() if key not in {"contractType","schemaVersion",field}}
    return identified(record["contractType"], payload, field)


def freshness_state(retrieved: str, as_of: str, maximum_age: int) -> str:
    timestamp(retrieved, "retrievedAt"); timestamp(as_of, "asOf"); integer(maximum_age, "maximumAgeSeconds")
    age = int((instant(as_of) - instant(retrieved)).total_seconds())
    return "current" if 0 <= age <= maximum_age else "stale"


def acquire_sources(run_id: str, rows: list[Any], question: dict[str, Any], runtime: Runtime) -> list[dict[str, Any]]:
    if len(rows) > 128:
        fail("RER-SOURCE-LIMIT", "limit", "source count exceeds 128")
    result=[]
    allowed=question["sourcePolicy"]["allowedClasses"]
    for index, raw in enumerate(rows):
        row=exact(raw,{"locator","publisher","retrievedAt","transport","mediaType","content","sourceClass","classification","acquisitionReceiptId"},f"sources[{index}]")
        if row["sourceClass"] not in allowed:
            fail("RER-SOURCE-UNTRUSTED","policy","source class is not allowed")
        expected_class=question["sourcePolicy"]["classificationByClass"][row["sourceClass"]]
        if row["classification"] != expected_class or CLASSIFICATIONS.index(row["classification"]) > CLASSIFICATIONS.index(question["dataClassification"]):
            fail("RER-SOURCE-UNTRUSTED","policy","source classification violates exact policy or run maximum")
        if not isinstance(row["content"],str) or len(row["content"].encode("utf-8")) > MAX_SOURCE_BYTES:
            fail("RER-SOURCE-LIMIT","limit","source content exceeds bounded bytes")
        content_digest=typed("research-source-bytes",row["content"])
        maximum=question["sourcePolicy"]["maximumAgeSecondsByClass"][row["sourceClass"]]
        normalized=normalize_content(row["content"],row["mediaType"])
        source=identified("SourceRecord",{"runId":run_id,"locator":row["locator"],"publisher":row["publisher"],"retrievedAt":row["retrievedAt"],"transport":row["transport"],"mediaType":row["mediaType"],"contentDigest":content_digest,"normalizedDigest":typed("research-normalized-bytes",normalized),"freshness":{"maximumAgeSeconds":maximum,"state":freshness_state(row["retrievedAt"],question["asOf"],maximum),"sourceClass":row["sourceClass"]},"classification":row["classification"],"acquisitionReceiptId":row["acquisitionReceiptId"]},"sourceId")
        result.append(source)
    return result


def normalize_content(content: str, media_type: str) -> str:
    if media_type == "application/json":
        try: return ecf.canonical_bytes(json.loads(content)).decode("utf-8")
        except (json.JSONDecodeError, UnicodeError): fail("RER-EXTRACTION-INVALID","source","JSON source is malformed")
    return content.replace("\r\n","\n").replace("\r","\n")


def segments(content: str) -> list[dict[str, Any]]:
    encoded=content.encode("utf-8"); result=[]; offset=0
    for number,line in enumerate(content.splitlines(keepends=True),1):
        size=len(line.encode("utf-8")); result.append({"index":number,"byteStart":offset,"byteEnd":offset+size}); offset+=size
    if not result: result=[{"index":1,"byteStart":0,"byteEnd":len(encoded)}]
    return result


def normalize_sources(raw_sources: list[Any], sources: list[dict[str, Any]]) -> list[dict[str, Any]]:
    by_locator = {source["locator"]: source for source in sources}
    normalized_sources = []
    for raw in raw_sources:
        source = by_locator.get(raw["locator"])
        if source is None:
            fail("RER-EXTRACTION-INVALID", "source", "acquired source is unavailable for normalization")
        content = normalize_content(raw["content"], raw["mediaType"])
        content_digest = typed("research-normalized-bytes", content)
        if content_digest != source["normalizedDigest"]:
            fail("RER-EXTRACTION-INVALID", "integrity", "normalized source digest changed after acquisition")
        normalized_sources.append(identified("NormalizedSource", {
            "sourceId": source["sourceId"],
            "contentDigest": content_digest,
            "mediaType": raw["mediaType"],
            "canonicalContent": content,
            "segments": segments(content),
        }, "normalizedSourceId"))
    return normalized_sources


def extract_selector(content: str, selector: Any) -> bytes:
    value=exact(selector,{"type","start","end","pointer","rowStart","rowEnd","columnStart","columnEnd"},"selector")
    kind=enum(value["type"],SELECTORS,"selector.type")
    data=content.encode("utf-8")
    if kind == "segment-byte-range":
        start=integer(value["start"],"selector.start"); end=integer(value["end"],"selector.end")
        if start>=end or end>len(data): fail("RER-EXTRACTION-INVALID","selector","byte selector is out of bounds")
        return data[start:end]
    if kind == "line-range":
        lines=content.splitlines(keepends=True); start=integer(value["start"],"selector.start",1); end=integer(value["end"],"selector.end",start)
        if end>len(lines): fail("RER-EXTRACTION-INVALID","selector","line selector is out of bounds")
        return "".join(lines[start-1:end]).encode("utf-8")
    if kind == "json-pointer":
        if not isinstance(value["pointer"],str) or not value["pointer"].startswith("/"): fail("RER-EXTRACTION-INVALID","selector","JSON pointer is invalid")
        try:
            node=json.loads(content)
            for token in value["pointer"].split("/")[1:]:
                key=token.replace("~1","/").replace("~0","~"); node=node[int(key)] if isinstance(node,list) else node[key]
        except (ValueError,KeyError,IndexError,TypeError,json.JSONDecodeError): fail("RER-EXTRACTION-INVALID","selector","JSON pointer is unresolved")
        return ecf.canonical_bytes(node)
    rows=[line.split("\t") for line in content.splitlines()]
    rs=integer(value["rowStart"],"rowStart",1); re_=integer(value["rowEnd"],"rowEnd",rs); cs=integer(value["columnStart"],"columnStart",1); ce=integer(value["columnEnd"],"columnEnd",cs)
    if re_>len(rows) or any(ce>len(row) for row in rows[rs-1:re_]): fail("RER-EXTRACTION-INVALID","selector","table selector is out of bounds")
    return "\n".join("\t".join(row[cs-1:ce]) for row in rows[rs-1:re_]).encode("utf-8")


def proposition(raw: Any, question_id: str) -> dict[str, Any]:
    fields={"statement","subject","predicate","object","polarity","time","scope","unit","scale","modality","materiality","originStageId"}
    value=exact(raw,fields,"claim")
    enum(value["polarity"],("positive","negative"),"polarity"); enum(value["modality"],("asserted","estimated","interpreted"),"modality"); enum(value["materiality"],MATERIALITIES,"claim.materiality")
    for name in ("subject","predicate","object","time","scope","unit","scale"):
        if not isinstance(value[name],str): malformed(f"claim.{name} must be a string")
    return identified("ClaimRecord",{"questionId":question_id,**value,"statusBasis":[]},"claimId")


def semantic_bound_slots(claim: dict[str, Any], excerpt: bytes) -> list[str]:
    try:
        text = excerpt.decode("utf-8")
    except UnicodeDecodeError:
        return []
    normalized_excerpt = " ".join(re.findall(r"[a-z0-9]+", text.casefold()))
    normalized_statement = " ".join(re.findall(r"[a-z0-9]+", claim["statement"].casefold()))
    if not normalized_statement or normalized_statement not in normalized_excerpt:
        return []
    bound = []
    for slot in ("subject", "predicate", "object"):
        normalized_value = " ".join(re.findall(r"[a-z0-9]+", claim[slot].casefold()))
        if normalized_value and normalized_value in normalized_excerpt:
            bound.append(slot)
    negative = bool({"no", "not", "never", "without", "unavailable", "unsupported"} & set(normalized_excerpt.split()))
    if (claim["polarity"] == "negative") == negative:
        bound.append("polarity")
    for slot in ("time", "scope", "unit", "scale", "modality", "materiality"):
        normalized_value = " ".join(re.findall(r"[a-z0-9]+", claim[slot].casefold()))
        if normalized_value and normalized_value in normalized_excerpt:
            bound.append(slot)
    return sorted(bound)


def build_ledger(question: dict[str, Any], rows: list[Any], sources: list[dict[str, Any]], normalized_refs: list[dict[str, Any]]) -> list[dict[str, Any]]:
    source_by_locator={row["locator"]:row for row in sources}; output=[]
    normalized_by_source={row["sourceId"]:row for row in normalized_refs}
    for index, raw in enumerate(rows):
        value=exact(raw,{"claim","evidence"},f"claims[{index}]"); claim=proposition(value["claim"],question["questionId"]); links=[]
        if not isinstance(value["evidence"],list): malformed("evidence must be an array")
        for number,evidence_raw in enumerate(value["evidence"]):
            ev=exact(evidence_raw,{"sourceLocator","relation","selector","excerptDigest","extractionMethod","confidenceBasis","boundSlots","authorityClass"},f"evidence[{number}]")
            relation=enum(ev["relation"],RELATIONS,"relation")
            source=source_by_locator.get(ev["sourceLocator"])
            if source is None: fail("RER-EXTRACTION-INVALID","evidence","evidence source is unavailable")
            normalized = normalized_by_source.get(source["sourceId"])
            if normalized is None:
                fail("RER-EXTRACTION-INVALID", "evidence", "normalized evidence source is unavailable")
            excerpt = extract_selector(normalized["canonicalContent"], ev["selector"])
            expected_excerpt_digest = typed("research-evidence-excerpt", excerpt.hex())
            if ev["excerptDigest"] != expected_excerpt_digest:
                fail("RER-EXTRACTION-INVALID", "evidence", "evidence excerpt digest does not match selected immutable bytes")
            if not isinstance(ev["boundSlots"],list) or any(slot not in {"subject","predicate","object","polarity","time","scope","unit","scale","modality","materiality"} for slot in ev["boundSlots"]): malformed("boundSlots is outside atomic proposition fields")
            derived_slots = semantic_bound_slots(claim, excerpt)
            required={"subject","predicate","object","polarity"}
            compatible=required.issubset(set(derived_slots))
            if relation=="support" and not compatible: relation="context"
            link=identified("EvidenceLink",{"claimId":claim["claimId"],"sourceId":source["sourceId"],"relation":relation,"selector":ev["selector"],"excerptDigest":ev["excerptDigest"],"extractionMethod":ev["extractionMethod"],"confidenceBasis":ev["confidenceBasis"],"boundSlots":derived_slots,"authorityClass":ev["authorityClass"],"freshnessState":source["freshness"]["state"]},"linkId")
            links.append(link)
        claim["statusBasis"]=sorted(link["linkId"] for link in links); claim=reidentify(claim,"claimId")
        # claim identity change requires links to bind final claim identity.
        rebound=[]
        for link in links:
            link["claimId"]=claim["claimId"]; rebound.append(reidentify(link,"linkId"))
        claim["statusBasis"]=sorted(link["linkId"] for link in rebound); claim=reidentify(claim,"claimId")
        output.extend([claim,*rebound])
    return output


def analyze(question: dict[str, Any], ledger: list[dict[str, Any]]) -> list[dict[str, Any]]:
    claims=[row for row in ledger if row["contractType"]=="ClaimRecord"]; links=[row for row in ledger if row["contractType"]=="EvidenceLink"]
    conflicts=[]
    for i,left in enumerate(claims):
        for right in claims[i+1:]:
            if all(left[key]==right[key] for key in ("subject","predicate","time","scope","unit","scale")) and left["polarity"]!=right["polarity"]:
                related=[link["linkId"] for link in links if link["claimId"] in {left["claimId"],right["claimId"]} and link["relation"] in {"support","rebuttal"}]
                conflicts.append(identified("ConflictSet",{"claimIds":sorted([left["claimId"],right["claimId"]]),"evidenceLinkIds":sorted(related),"conflictType":"opposed-polarity","materiality":"material" if "material" in {left["materiality"],right["materiality"]} else "routine","resolutionBasis":None,"state":"open"},"conflictId"))
    dimensions=[]; findings=[]
    material_ids=[]
    for claim in claims:
        applicable=[link for link in links if link["claimId"]==claim["claimId"]]
        supports=[link for link in applicable if link["relation"]=="support"]
        is_conflicted=any(claim["claimId"] in conflict["claimIds"] for conflict in conflicts)
        state="conflicted" if is_conflicted else "satisfied" if any(link["freshnessState"]=="current" for link in supports) else "stale" if supports else "missing"
        if claim["materiality"] in {"material","high-stakes","consequentially-prohibited"}: material_ids.append(claim["claimId"])
        dimensions.append({"dimension":claim["claimId"],"state":state,"evidenceLinkIds":sorted(link["linkId"] for link in applicable)})
        if state!="satisfied": findings.append({"missing":"RER-CLAIM-UNSUPPORTED","stale":"RER-COVERAGE-INCOMPLETE","conflicted":"RER-EVIDENCE-CONFLICT"}[state])
    for required in question["coveragePolicy"]["requiredDimensions"]:
        if required not in {claim["subject"] for claim in claims}:
            dimensions.append({"dimension":required,"state":"missing","evidenceLinkIds":[]}); findings.append("RER-COVERAGE-INCOMPLETE")
    states={row["state"] for row in dimensions}
    overall="conflicted" if "conflicted" in states else "stale" if "stale" in states else "missing" if "missing" in states else "unavailable" if not dimensions else "satisfied"
    coverage=identified("CoverageAssessment",{"questionId":question["questionId"],"dimensions":sorted(dimensions,key=lambda row:row["dimension"]),"materialClaimIds":sorted(material_ids),"state":overall,"findingCodes":sorted(set(findings))},"coverageId")
    return [*conflicts,coverage]


def materiality_gate(question: dict[str, Any], coverage: dict[str, Any], conflicts: list[dict[str, Any]]) -> dict[str, Any]:
    reasons=[]; verdict="proceed"
    if question["materiality"]=="consequentially-prohibited": reasons.append("RER-CONSEQUENCE-PROHIBITED"); verdict="refuse"
    if coverage["state"] in {"missing","stale","unavailable"} and coverage["materialClaimIds"]: reasons.append("RER-CLAIM-UNSUPPORTED"); verdict="refuse"
    if coverage["state"]=="conflicted" or any(item["state"]=="open" and item["materiality"]!="routine" for item in conflicts): reasons.append("RER-EVIDENCE-CONFLICT"); verdict="human" if question["materiality"]=="routine" else "refuse"
    if question["materiality"] in {"material","high-stakes"} and verdict=="proceed": verdict="high-assurance"
    return identified("MaterialityAssessment",{"questionId":question["questionId"],"coverageId":coverage["coverageId"],"conflictIds":sorted(item["conflictId"] for item in conflicts),"classification":question["dataClassification"],"materiality":question["materiality"],"verdict":verdict,"reasonCodes":sorted(set(reasons))},"materialityAssessmentId")


def route_decision(run_id: str, adapter: dict[str, Any], question: dict[str, Any], assessment: dict[str, Any], context: dict[str, Any]) -> dict[str, Any]:
    if adapter["mode"]=="disabled": verdict="unavailable"; reason="RER-ROUTE-UNAVAILABLE"; route="deterministic-only"
    elif assessment["verdict"] in {"refuse","human"}: verdict="refused"; reason="RER-ESCALATION-REQUIRED"; route="local-command"
    else: verdict="selected"; reason="RER-ROUTE-SELECTED"; route="local-command"
    return identified("ModelRouteDecision",{"runId":run_id,"stageId":"optional-synthesis","adapterId":adapter["adapterId"],"routeClass":route,"classification":question["dataClassification"],"materiality":question["materiality"],"mbeBudgetId":context["budgetId"],"admissionDecisionId":context["admissionDecisionId"],"permitId":context["permitId"],"occurrenceId":context["occurrenceId"],"attemptId":context["attemptId"],"usageReceiptIds":context["usageReceiptIds"],"accountingState":context["accountingState"],"unresolvedHold":context["unresolvedHold"],"verdict":verdict,"reasonCode":reason},"modelRouteDecisionId")


def candidate_input(run_id: str, question: dict[str, Any], outputs: dict[str,list[dict[str,Any]]]) -> dict[str,Any]:
    claims=[row for row in outputs["claim-evidence-ledger"] if row["contractType"]=="ClaimRecord"]
    return {"contractType":"research-model-input","schemaVersion":1,"runId":run_id,"questionId":question["questionId"],"claimIds":[row["claimId"] for row in claims],"coverageId":next(row["coverageId"] for row in outputs["conflict-coverage-analysis"] if row["contractType"]=="CoverageAssessment"),"materialityAssessmentId":outputs["materiality-risk-gate"][0]["materialityAssessmentId"]}


def candidate_artifact(run_id: str, question: dict[str, Any], outputs: dict[str,list[dict[str,Any]]], candidate: dict[str,Any]|None, mode: str) -> dict[str,Any]:
    claims=[row for row in outputs["claim-evidence-ledger"] if row["contractType"]=="ClaimRecord"]; links=[row for row in outputs["claim-evidence-ledger"] if row["contractType"]=="EvidenceLink"]
    conflicts=[row for row in outputs["conflict-coverage-analysis"] if row["contractType"]=="ConflictSet"]; coverage=next(row for row in outputs["conflict-coverage-analysis"] if row["contractType"]=="CoverageAssessment"); assessment=outputs["materiality-risk-gate"][0]
    body={"mode":mode,"claimIds":[row["claimId"] for row in claims],"narrative":None if candidate is None else candidate["narrative"]}
    return identified("ResearchArtifact",{"runId":run_id,"questionId":question["questionId"],"claimIds":sorted(row["claimId"] for row in claims),"conflictIds":sorted(row["conflictId"] for row in conflicts),"sourceIds":sorted(set(row["sourceId"] for row in links)),"citationIndex":sorted([{"claimId":row["claimId"],"linkId":row["linkId"],"sourceId":row["sourceId"]} for row in links],key=lambda row:(row["claimId"],row["linkId"])),"coverageId":coverage["coverageId"],"materialityAssessmentId":assessment["materialityAssessmentId"],"limitations":sorted(set(coverage["findingCodes"]+assessment["reasonCodes"])),"body":body,"bodyDigest":typed("research-artifact-body",body),"validationReceiptId":None,"publicationTarget":question["publicationPolicy"]["target"],"consequenceBoundary":question["consequenceBoundary"]},"artifactId")


def validate_artifact(question: dict[str,Any], artifact: dict[str,Any], outputs: dict[str,list[dict[str,Any]]]) -> dict[str,Any]:
    coverage=next(row for row in outputs["conflict-coverage-analysis"] if row["contractType"]=="CoverageAssessment"); assessment=outputs["materiality-risk-gate"][0]; findings=[]
    if artifact["bodyDigest"]!=typed("research-artifact-body",artifact["body"]): findings.append("RER-VALIDATION-FAILED")
    if assessment["verdict"] in {"refuse","human"}: findings.extend(assessment["reasonCodes"] or ["RER-ESCALATION-REQUIRED"])
    if coverage["state"]!="satisfied" and not question["publicationPolicy"]["allowIncomplete"]: findings.extend(coverage["findingCodes"] or ["RER-COVERAGE-INCOMPLETE"])
    if artifact["body"]["mode"]=="ledger-only" and not question["publicationPolicy"]["allowLedgerOnly"]: findings.append("RER-ESCALATION-REQUIRED")
    route=next((row for row in outputs["optional-synthesis"] if row.get("contractType")=="ModelRouteDecision"),None)
    source_records=[row for row in outputs["bounded-acquisition"] if row.get("contractType")=="SourceRecord"]
    if route is None:
        budget_outcome="not-evaluated"; budget_evidence=[]
    elif route["routeClass"]=="deterministic-only" and route["verdict"]=="unavailable":
        budget_outcome="satisfied"; budget_evidence=[route["modelRouteDecisionId"]]
    elif route["unresolvedHold"] or route["accountingState"]=="unresolved":
        budget_outcome="failed"; budget_evidence=[route["modelRouteDecisionId"],*route["usageReceiptIds"]]
    elif route["accountingState"]=="terminal" and route["usageReceiptIds"]:
        budget_outcome="satisfied"; budget_evidence=[route["modelRouteDecisionId"],*route["usageReceiptIds"]]
    else:
        budget_outcome="not-evaluated"; budget_evidence=[route["modelRouteDecisionId"]]
    allowed_local=set(question["egressPolicy"].get(question["dataClassification"],[]))
    if route is None or not source_records:
        privacy_outcome="not-evaluated"; privacy_evidence=[]
    elif route["routeClass"]=="deterministic-only" and route["verdict"]=="unavailable":
        privacy_outcome="satisfied"; privacy_evidence=[route["modelRouteDecisionId"],*[row["sourceId"] for row in source_records]]
    elif route["routeClass"]=="local-command" and "local" in allowed_local and all(row["classification"]==question["dataClassification"] for row in source_records):
        privacy_outcome="satisfied"; privacy_evidence=[route["modelRouteDecisionId"],*[row["sourceId"] for row in source_records]]
    else:
        privacy_outcome="failed"; privacy_evidence=([route["modelRouteDecisionId"]] if route else [])+[*[row["sourceId"] for row in source_records]]
    if budget_outcome!="satisfied": findings.append("RER-BUDGET-NOT-EVALUATED")
    if privacy_outcome!="satisfied" and question["dataClassification"]!="public": findings.append("RER-PRIVACY-NOT-EVALUATED")
    check=lambda outcome,evidence: {"outcome":outcome,"evidenceIds":sorted(set(evidence))}
    checks={"identity":check("satisfied",[artifact["artifactId"]]),"citations":check("satisfied" if coverage["state"]=="satisfied" else "failed",[coverage["coverageId"]]),"coverage":check("satisfied" if coverage["state"]=="satisfied" else "failed",[coverage["coverageId"]]),"conflicts":check("satisfied" if assessment["verdict"] not in {"human","refuse"} else "failed",[assessment["materialityAssessmentId"]]),"privacy":check(privacy_outcome,privacy_evidence),"freshness":check("failed" if "stale" in {row["state"] for row in coverage["dimensions"]} else "satisfied",[coverage["coverageId"]]),"budget":check(budget_outcome,budget_evidence)}
    return identified("ValidationReceipt",{"runId":artifact["runId"],"artifactDigest":typed("execution-control-object",artifact),"manifestDigest":typed("research-validation-input",outputs["optional-synthesis"]),"checks":checks,"verdict":"accepted" if not findings else "rejected","findingCodes":sorted(set(findings)),"validatedAt":question["asOf"]},"validationReceiptId")


def _private_json(path_text: str) -> dict[str,Any]:
    path=Path(path_text)
    if not path.is_absolute() or ".." in path.parts: malformed("adapter config path must be lexical absolute")
    current=Path(path.anchor)
    for part in path.parts[1:]:
        current/=part
        metadata=os.lstat(current)
        if stat.S_ISLNK(metadata.st_mode): fail("RER-ROUTE-UNAVAILABLE","adapter","adapter config symlinks are forbidden")
    metadata=os.stat(path,follow_symlinks=False)
    if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid!=os.geteuid() or stat.S_IMODE(metadata.st_mode)!=0o600 or metadata.st_nlink!=1:
        fail("RER-ROUTE-UNAVAILABLE","adapter","adapter config must be owner-private regular no-hardlink JSON")
    data=path.read_bytes()
    if len(data)>65_536: fail("RER-SOURCE-LIMIT","limit","adapter config exceeds 65536 bytes")
    try: value=json.loads(data)
    except json.JSONDecodeError: malformed("adapter config is malformed JSON")
    return exact(value,{"contractType","schemaVersion","argv","environment","environmentAllowlist","maxInputBytes","maxOutputBytes","maxErrorBytes","timeoutMs","cancelFile"},"adapter config")


def _closed_candidate(value: Any) -> dict[str,Any]:
    candidate=exact(value,{"contractType","schemaVersion","narrative","claimIds","limitations"},"model candidate")
    if candidate["contractType"]!="research-model-candidate" or candidate["schemaVersion"]!=1: fail("RER-MODEL-OUTPUT-INVALID","adapter","candidate contract mismatch")
    if any(key in FORBIDDEN_MODEL_KEYS for key in candidate): fail("RER-MODEL-OUTPUT-INVALID","security","candidate contains control fields")
    if not isinstance(candidate["narrative"],str) or not isinstance(candidate["claimIds"],list) or not isinstance(candidate["limitations"],list): fail("RER-MODEL-OUTPUT-INVALID","adapter","candidate field types are invalid")
    return candidate


def local_command(config_path: str, input_value: dict[str,Any]) -> dict[str,Any]:
    config=_private_json(config_path)
    if config["contractType"]!="research-local-command-config" or config["schemaVersion"]!=1: malformed("local-command config contract mismatch")
    argv=config["argv"]
    if not isinstance(argv,list) or not argv or any(not isinstance(item,str) or "\x00" in item for item in argv): malformed("argv must be a non-empty string array")
    allow=config["environmentAllowlist"]
    if not isinstance(allow,list) or len(allow)!=len(set(allow)) or any(not isinstance(item,str) for item in allow): malformed("environmentAllowlist must be unique strings")
    environment=config["environment"]
    if not isinstance(environment,dict) or set(environment)-set(allow) or any(not isinstance(value,str) for value in environment.values()): malformed("environment exceeds explicit allowlist")
    stdin=ecf.canonical_line(input_value)
    max_input=integer(config["maxInputBytes"],"maxInputBytes",1,MAX_CANDIDATE_BYTES); max_output=integer(config["maxOutputBytes"],"maxOutputBytes",1,MAX_CANDIDATE_BYTES); max_error=integer(config["maxErrorBytes"],"maxErrorBytes",0,MAX_CANDIDATE_BYTES); timeout_ms=integer(config["timeoutMs"],"timeoutMs",1,300_000)
    if len(stdin)>max_input: fail("RER-SOURCE-LIMIT","limit","model input exceeds configured bound")
    cancel=config["cancelFile"]
    run_id=None
    if cancel is not None:
        run_id=input_value.get("runId")
        if not isinstance(run_id,str): malformed("cancellable model input requires runId")
        try: cancellation=_private_cancellation(cancel,run_id)
        except FileNotFoundError: cancellation=None
        if cancellation is not None: fail("RER-ROUTE-UNAVAILABLE","cancelled","dispatch cancelled before start")
    process=subprocess.Popen(argv,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=environment,cwd="/",shell=False,close_fds=True,start_new_session=True)
    assert process.stdin is not None and process.stdout is not None and process.stderr is not None
    stdout,stderr=_bounded_process_io(process,stdin,max_output,max_error,timeout_ms,cancel,run_id)
    if process.returncode!=0: fail("RER-ROUTE-UNAVAILABLE","adapter",f"local command failed with exit {process.returncode}")
    try: value=json.loads(stdout)
    except (json.JSONDecodeError,UnicodeError): fail("RER-MODEL-OUTPUT-INVALID","adapter","candidate output is not UTF-8 JSON")
    return _closed_candidate(value)


def _terminate_group(process: subprocess.Popen[bytes]) -> None:
    try: os.killpg(process.pid,signal.SIGTERM)
    except ProcessLookupError: pass
    try: process.wait(timeout=1)
    except subprocess.TimeoutExpired: pass
    try: os.killpg(process.pid,signal.SIGKILL)
    except ProcessLookupError: pass
    if process.poll() is None: process.wait()


def _bounded_process_io(process: subprocess.Popen[bytes], stdin: bytes, max_output: int, max_error: int, timeout_ms: int, cancel: str | None, run_id: str | None) -> tuple[bytes,bytes]:
    selector=selectors.DefaultSelector(); outputs={"stdout":bytearray(),"stderr":bytearray()}
    input_offset=0; stdin_open=True
    assert process.stdin is not None and process.stdout is not None and process.stderr is not None
    for name,stream in (("stdout",process.stdout),("stderr",process.stderr)):
        os.set_blocking(stream.fileno(),False); selector.register(stream,selectors.EVENT_READ,name)
    os.set_blocking(process.stdin.fileno(),False); selector.register(process.stdin,selectors.EVENT_WRITE,"stdin")
    deadline=time.monotonic()+timeout_ms/1000
    try:
        while selector.get_map() or process.poll() is None:
            remaining=deadline-time.monotonic()
            if remaining<=0: _terminate_group(process); fail("RER-ROUTE-UNAVAILABLE","timeout","local command exceeded deadline")
            if cancel is not None and run_id is not None:
                try: cancellation=_private_cancellation(cancel,run_id)
                except FileNotFoundError: cancellation=None
                if cancellation is not None: _terminate_group(process); fail("RER-ROUTE-UNAVAILABLE","cancelled","dispatch cancelled during execution")
            if process.poll() is not None and stdin_open:
                selector.unregister(process.stdin); process.stdin.close(); stdin_open=False
            events=selector.select(min(remaining,0.05) if cancel is not None else remaining)
            for key,_mask in events:
                if key.data=="stdin":
                    try: written=os.write(key.fileobj.fileno(),stdin[input_offset:input_offset+65536])
                    except BrokenPipeError: written=0
                    input_offset+=written
                    if written==0 or input_offset==len(stdin):
                        selector.unregister(key.fileobj); key.fileobj.close(); stdin_open=False
                    continue
                limit=max_output if key.data=="stdout" else max_error
                chunk=os.read(key.fileobj.fileno(),min(65536,limit-len(outputs[key.data])+1))
                if not chunk: selector.unregister(key.fileobj); key.fileobj.close(); continue
                outputs[key.data].extend(chunk)
                if len(outputs[key.data])>limit: _terminate_group(process); fail("RER-SOURCE-LIMIT","limit",f"adapter {key.data} exceeded configured bound")
    finally:
        selector.close()
    return bytes(outputs["stdout"]),bytes(outputs["stderr"])


def _private_cancellation(path_text: str, expected_run_id: str) -> dict[str,Any]:
    value=_private_file_json(path_text,"cancellation")
    result=exact(value,{"contractType","schemaVersion","runId","state","at","reasonDigest"},"cancellation")
    if result["contractType"]!="research-cancellation" or result["schemaVersion"]!=1 or result["state"]!="cancelled": malformed("cancellation contract mismatch")
    identifier(result["runId"],"cancellation.runId"); timestamp(result["at"],"cancellation.at"); digest(result["reasonDigest"],"cancellation.reasonDigest")
    if result["runId"]!=expected_run_id: fail("RER-ROUTE-UNAVAILABLE","security","cancellation runId does not match dispatched run")
    return result


def _private_file_json(path_text: str, label: str) -> dict[str,Any]:
    path=Path(path_text)
    if not path.is_absolute() or ".." in path.parts: malformed(f"{label} path must be lexical absolute")
    current=Path(path.anchor)
    for part in path.parts[1:]:
        current/=part
        metadata=os.lstat(current)
        if stat.S_ISLNK(metadata.st_mode): fail("RER-ROUTE-UNAVAILABLE","security",f"{label} symlinks are forbidden")
    metadata=os.stat(path,follow_symlinks=False)
    if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid!=os.geteuid() or stat.S_IMODE(metadata.st_mode)!=0o600 or metadata.st_nlink!=1: fail("RER-ROUTE-UNAVAILABLE","security",f"{label} must be owner-private regular no-hardlink JSON")
    data=path.read_bytes()
    if len(data)>65_536: fail("RER-SOURCE-LIMIT","limit",f"{label} exceeds 65536 bytes")
    try: value=json.loads(data)
    except json.JSONDecodeError: malformed(f"{label} is malformed JSON")
    if not isinstance(value,dict): malformed(f"{label} must be an object")
    return value


def parser_operations() -> tuple[str,...]:
    return ("capabilities","validate-question","plan","run","resume","inspect","validate","publish","bridge","cancel","schema","check","adapter-disabled","adapter-local-command")


def parser() -> argparse.ArgumentParser:
    result=argparse.ArgumentParser(description="IMP-054 research runtime")
    result.add_argument("operation",choices=parser_operations())
    result.add_argument("--store-root")
    result.add_argument("--input")
    result.add_argument("--run-id")
    result.add_argument("--stop-after",choices=STAGES)
    result.add_argument("--project-root")
    result.add_argument("--bridge",choices=("bubbles-envelope","downstream-consumer"))
    return result


def read_input(path_text: str|None) -> Any:
    if path_text is None: malformed("--input is required")
    raw,_=ecf.read_external(ecf.absolute_path(path_text,"input"),ecf.MAX_INPUT_BYTES,"research input")
    return ecf.parse_json(raw,"research input",canonical=False)


def execute(args: argparse.Namespace) -> Any:
    registry=load_registry(); load_stages()
    if args.operation=="capabilities":
        config = resolve_project_config(args.project_root)
        return {"contractType":"research-capabilities","schemaVersion":1,"enabled":config["enabled"],"configPath":config["configPath"],"adapters":registry["adapters"] if config["enabled"] else [],"hostedRoutes":[],"parkedActivation":True}
    if args.operation=="schema": return {"contractType":"research-schema","schemaVersion":1,"records":{key:sorted(value) for key,value in RECORD_FIELDS.items()},"stages":list(STAGES)}
    if args.operation=="check": return {"contractType":"research-check","schemaVersion":1,"registryDigest":typed("research-runtime-registry",registry),"stageDigest":typed("research-stage-registry",load_stages()),"ecfVersion":ecf.VERSION,"mbeVersion":mbe.VERSION,"status":"valid"}
    if args.operation=="validate-question": return validate_question(read_input(args.input))
    if args.operation=="adapter-disabled": return {"contractType":"research-adapter-result","schemaVersion":1,"adapter":"disabled","status":"unavailable","measurement":"unmeasured","candidate":None}
    if args.operation=="adapter-local-command":
        value=exact(read_input(args.input),{"configPath","input"},"adapter request"); return local_command(value["configPath"],value["input"])
    if args.store_root is None: malformed("--store-root is required")
    runtime=Runtime(args.store_root)
    if args.operation=="plan":
        value=exact(read_input(args.input),{"question","policies","budgetId","adapter"},"plan request"); question=validate_question(value["question"]); return runtime.plan(question,value["policies"],value["budgetId"],value["adapter"])
    if args.operation in {"run","resume"}: return runtime.run(read_input(args.input),args.stop_after)
    if args.operation=="inspect":
        if args.run_id is None: malformed("--run-id is required")
        rows=[row for row in runtime.records() if row.get("runId")==args.run_id or row.get("questionId")==args.run_id]
        return {"contractType":"research-inspection","schemaVersion":1,"runId":args.run_id,"records":rows}
    if args.operation=="validate":
        return runtime.run(read_input(args.input), "deterministic-validation")
    if args.operation=="publish":
        value=exact(read_input(args.input), {"request","validationReceiptId"}, "publish request")
        identifier(value["validationReceiptId"], "validationReceiptId")
        return runtime.run(value["request"], required_validation_receipt_id=value["validationReceiptId"])
    if args.operation=="bridge":
        if args.bridge is None: malformed("bridge requires --bridge")
        return bridge_projection(args.bridge,read_input(args.input))
    if args.operation=="cancel":
        value=exact(read_input(args.input),{"runId","at","reason"},"cancel request"); identifier(value["runId"],"runId"); timestamp(value["at"],"at")
        return {"contractType":"research-cancellation","schemaVersion":1,"runId":value["runId"],"state":"cancelled","at":value["at"],"reasonDigest":typed("research-cancel-reason",value["reason"])}
    malformed("unsupported operation")


def main() -> int:
    try:
        result=execute(parser().parse_args()); sys.stdout.buffer.write(ecf.canonical_line(result)); return 0
    except ResearchError as exc:
        sys.stderr.buffer.write(ecf.canonical_line({"code":exc.code,"contractType":"research-error","errorClass":exc.kind,"message":str(exc)[:512],"schemaVersion":1})); return exc.exit_code
    except (ecf.EcfError,mbe.MbeError) as exc:
        code=getattr(exc,"code","RER-INTERNAL"); kind=getattr(exc,"kind",getattr(exc,"error_class","dependency")); exit_code=getattr(exc,"exit_code",2)
        sys.stderr.buffer.write(ecf.canonical_line({"code":code,"contractType":"research-error","errorClass":kind,"message":str(exc)[:512],"schemaVersion":1})); return exit_code
    except (OSError,TypeError,ValueError,KeyError) as exc:
        sys.stderr.buffer.write(ecf.canonical_line({"code":"RER-INTERNAL","contractType":"research-error","errorClass":"internal","message":str(exc)[:512],"schemaVersion":1})); return 2


if __name__=="__main__": raise SystemExit(main())
