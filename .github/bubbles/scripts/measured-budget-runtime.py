#!/usr/bin/env python3
"""IMP-055 MBE-1 provider-neutral budget, admission, epoch, and corpus domain engine.

This module deliberately delegates canonical JSON, content-addressed objects,
locking, append, recovery, and checkpoint mechanics to Execution-Control
Foundation v2. It is a reference domain engine, not a host dispatch broker.
Host-native interception remains unsupported until an independently enforcing
host adapter consumes a one-use permit.
"""
from __future__ import annotations

import argparse
import datetime as dt
import importlib.util
import json
import re
import sys
from pathlib import Path
from typing import Any, NoReturn

HERE = Path(__file__).resolve().parent
ECF_PATH = HERE / "execution-control-store.py"
DIMENSIONS_PATH = HERE.parent / "registry" / "admission-dimensions.yaml"
MODEL_CLASSES_PATH = HERE.parent / "registry" / "model-classes.yaml"
CONTRACTS_PATH = HERE / "measured-budget-contracts.py"
_spec = importlib.util.spec_from_file_location("bubbles_execution_control_store", ECF_PATH)
if _spec is None or _spec.loader is None:
    raise RuntimeError("ECF v2 module is unavailable")
ecf = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(ecf)
_contracts_spec = importlib.util.spec_from_file_location("bubbles_measured_budget_contracts", CONTRACTS_PATH)
if _contracts_spec is None or _contracts_spec.loader is None:
    raise RuntimeError("MBE contract authority is unavailable")
contracts = importlib.util.module_from_spec(_contracts_spec)
_contracts_spec.loader.exec_module(contracts)
_security_spec = importlib.util.spec_from_file_location("bubbles_security_authority", HERE / "security-authority.py")
if _security_spec is None or _security_spec.loader is None:
    raise RuntimeError("security authority is unavailable")
security = importlib.util.module_from_spec(_security_spec)
_security_spec.loader.exec_module(security)
CONTRACT_REGISTRY = contracts.load_registry()

VERSION = 1
MAX_AMOUNT = 9_007_199_254_740_991
MAX_ROWS = 256
MAX_DOMAIN_RECORD_BYTES = 131_072
ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
CURRENCY = re.compile(r"^[A-Z]{3}$")
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
TIMESTAMP = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$")
DIMENSION_STATES = frozenset(("configured", "unconfigured"))
CAPABILITY_MODES = frozenset(("native", "trusted-derived", "bounded-only", "unsupported"))
ROLLOUT_POSTURES = frozenset(("shadow", "reference-enforce"))
EPOCH_CLASSES = ("planning", "implementation", "verification", "certification")
BUDGET_EVENTS = frozenset(("OPEN", "RESERVE", "DEBIT", "RELEASE", "HOLD", "CORRECT", "CLOSE"))
SINGLE_USE_DIMENSIONS = frozenset(("modelRequestCount", "subagentDispatches", "webCalls", "browserCalls", "toolCalls", "retries", "concurrency"))
HOST_PROOF_TYPES = frozenset(("host-checkpoint", "new-session"))
FORBIDDEN_KEYS = frozenset(
    re.sub(r"[^a-z0-9]", "", alias.casefold())
    for alias in CONTRACT_REGISTRY["sensitiveKeyAliases"]
)


class MbeError(RuntimeError):
    """Stable typed MBE refusal or malformed-input error."""

    def __init__(self, code: str, error_class: str, message: str, exit_code: int) -> None:
        super().__init__(message)
        self.code = code
        self.error_class = error_class
        self.exit_code = exit_code


def fail(code: str, error_class: str, message: str, exit_code: int) -> NoReturn:
    raise MbeError(code, error_class, message, exit_code)


def malformed(message: str) -> NoReturn:
    fail("MBE-INPUT-INVALID", "input", message, 2)


def refused(code: str, message: str) -> NoReturn:
    fail(code, "policy", message, 1)


def unsupported(code: str, message: str) -> NoReturn:
    fail(code, "unsupported", message, 3)


def outstanding(code: str, message: str) -> NoReturn:
    fail(code, "reconciliation", message, 4)


def exact_fields(value: Any, expected: set[str] | frozenset[str], label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        malformed(f"{label} must be an object")
    actual = set(value)
    if actual != set(expected):
        malformed(f"{label} field mismatch; missing={sorted(set(expected)-actual)}, unknown={sorted(actual-set(expected))}")
    reject_sensitive(value)
    return value


def reject_sensitive(value: Any) -> None:
    if isinstance(value, dict):
        for key, item in value.items():
            normalized_key = re.sub(r"[^a-z0-9]", "", str(key).casefold())
            if normalized_key in FORBIDDEN_KEYS:
                malformed(f"forbidden private telemetry field: {key}")
            reject_sensitive(item)
    elif isinstance(value, list):
        for item in value:
            reject_sensitive(item)
    elif isinstance(value, str):
        if value.startswith(("/home/", "/Users/")) or "-----BEGIN " in value:
            malformed("private paths and secret material are forbidden")


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
        malformed(f"{label} is not a valid UTC timestamp")
    return value


def instant(value: str) -> dt.datetime:
    return dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=dt.timezone.utc)


def integer(value: Any, label: str, minimum: int = 0, maximum: int = MAX_AMOUNT) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < minimum or value > maximum:
        malformed(f"{label} must be an integer in [{minimum}, {maximum}]")
    return value


def boolean(value: Any, label: str) -> bool:
    if not isinstance(value, bool):
        malformed(f"{label} must be boolean")
    return value


def enum(value: Any, allowed: set[str] | frozenset[str] | tuple[str, ...], label: str) -> str:
    if not isinstance(value, str) or value not in allowed:
        malformed(f"{label} is outside its closed vocabulary")
    return value


def load_json_registry(path: Path, contract_type: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        fail("MBE-INTERNAL", "internal", f"registry unavailable or malformed: {exc}", 2)
    if not isinstance(value, dict) or value.get("contractType") != contract_type or value.get("schemaVersion") != 1:
        fail("MBE-INTERNAL", "internal", "registry contract is invalid", 2)
    return value


_DIMENSION_REGISTRY = load_json_registry(DIMENSIONS_PATH, "admission-dimension-registry")
DIMENSION_ROWS = _DIMENSION_REGISTRY.get("dimensions")
if not isinstance(DIMENSION_ROWS, list) or len(DIMENSION_ROWS) != 15:
    raise RuntimeError("admission dimension registry must contain exactly 15 rows")
DIMENSION_DEFINITIONS = {row["name"]: row for row in DIMENSION_ROWS}
if len(DIMENSION_DEFINITIONS) != 15:
    raise RuntimeError("admission dimension names must be unique")
DIMENSIONS = tuple(row["name"] for row in DIMENSION_ROWS)
_MODEL_REGISTRY = load_json_registry(MODEL_CLASSES_PATH, "model-class-registry")
MODEL_CLASSES = frozenset(row["name"] for row in _MODEL_REGISTRY["classes"])


def identified(contract_type: str, payload: dict[str, Any], field: str, prefix: str) -> dict[str, Any]:
    material = {"contractType": contract_type, "schemaVersion": VERSION, **payload}
    material[field] = f"{prefix}:{ecf.typed_digest(contract_type, material)[7:]}"
    return material


def lineaged(contract_type: str, payload: dict[str, Any], field: str, prefix: str, predecessor_digest: str) -> dict[str, Any]:
    digest(predecessor_digest, "predecessorDigest")
    with_predecessor = {**payload, "predecessorDigest": predecessor_digest}
    with_domain = {**with_predecessor, "domainDigest": ecf.typed_digest(f"{contract_type}-domain", with_predecessor)}
    return identified(contract_type, with_domain, field, prefix)


def validate_amount(row: Any, label: str) -> dict[str, Any]:
    base = {"dimension", "amount", "unit", "currency", "scale"}
    value = exact_fields(row, base, label)
    name = enum(value["dimension"], DIMENSIONS, f"{label}.dimension")
    definition = DIMENSION_DEFINITIONS[name]
    integer(value["amount"], f"{label}.amount")
    if value["unit"] != definition["unit"]:
        malformed(f"{label}.unit does not match the dimension registry")
    if definition["amountShape"] == "currency-scale-integer":
        if not isinstance(value["currency"], str) or not CURRENCY.fullmatch(value["currency"]):
            malformed(f"{label}.currency must be ISO-4217 uppercase")
        integer(value["scale"], f"{label}.scale", 0, 9)
    elif value["currency"] is not None or value["scale"] is not None:
        malformed(f"{label} cannot carry currency or scale")
    return value


def amount_key(row: dict[str, Any]) -> str:
    if row["dimension"] == "monetaryMinorUnits":
        return f"{row['dimension']}:{row['currency']}:{row['scale']}"
    return row["dimension"]


def validate_amounts(rows: Any, label: str, allow_empty: bool = False) -> list[dict[str, Any]]:
    if not isinstance(rows, list) or len(rows) > MAX_ROWS or (not rows and not allow_empty):
        malformed(f"{label} must be a bounded non-empty array")
    result: list[dict[str, Any]] = []
    seen: set[str] = set()
    for index, row in enumerate(rows):
        item = validate_amount(row, f"{label}[{index}]")
        key = amount_key(item)
        if key in seen:
            malformed(f"{label} contains a duplicate counter")
        seen.add(key)
        result.append(dict(item))
    return sorted(result, key=amount_key)


def validate_dimensions(rows: Any) -> list[dict[str, Any]]:
    if not isinstance(rows, list) or len(rows) != len(DIMENSIONS):
        malformed("dimensions must contain exactly the 15 registry dimensions")
    result = []
    seen = set()
    for index, row in enumerate(rows):
        value = exact_fields(row, {"dimension", "state", "limit", "unit", "currency", "scale"}, f"dimensions[{index}]")
        name = enum(value["dimension"], DIMENSIONS, "dimension")
        if name in seen:
            malformed("dimension policy rows must be unique")
        seen.add(name)
        state = enum(value["state"], DIMENSION_STATES, "dimension.state")
        definition = DIMENSION_DEFINITIONS[name]
        if value["unit"] != definition["unit"]:
            malformed("dimension unit does not match registry")
        if state == "configured":
            integer(value["limit"], "dimension.limit")
        elif value["limit"] is not None:
            malformed("unconfigured dimension limit must be null")
        if name == "monetaryMinorUnits":
            if state == "configured":
                if not isinstance(value["currency"], str) or not CURRENCY.fullmatch(value["currency"]):
                    malformed("configured money requires ISO currency")
                integer(value["scale"], "dimension.scale", 0, 9)
            elif value["currency"] is not None or value["scale"] is not None:
                malformed("unconfigured money has no currency counter")
        elif value["currency"] is not None or value["scale"] is not None:
            malformed("non-money dimensions cannot carry currency or scale")
        result.append(dict(value))
    if seen != set(DIMENSIONS):
        malformed("dimension policy vocabulary is incomplete")
    return sorted(result, key=lambda row: DIMENSIONS.index(row["dimension"]))


def validate_capabilities(rows: Any) -> dict[str, dict[str, Any]]:
    if not isinstance(rows, list) or len(rows) != len(DIMENSIONS):
        malformed("adapter capabilities must contain all 15 dimensions")
    result: dict[str, dict[str, Any]] = {}
    for index, row in enumerate(rows):
        value = exact_fields(row, {"dimension", "mode", "preDispatchBound", "postDispatchActual"}, f"capabilities[{index}]")
        name = enum(value["dimension"], DIMENSIONS, "capability.dimension")
        if name in result:
            malformed("capability dimensions must be unique")
        enum(value["mode"], CAPABILITY_MODES, "capability.mode")
        boolean(value["preDispatchBound"], "capability.preDispatchBound")
        boolean(value["postDispatchActual"], "capability.postDispatchActual")
        result[name] = dict(value)
    if set(result) != set(DIMENSIONS):
        malformed("capability vocabulary is incomplete")
    return result


def validate_host_proof(value: Any, authority_path: str, verified_at: str) -> dict[str, Any]:
    proof = exact_fields(value, {"contractType", "schemaVersion", "proofType", "proofId", "issuer", "verifierId", "trustRootId", "repositoryDecisionId", "previousSessionIdentity", "nextSessionIdentity", "continuationDigest", "issuedAt", "expiresAt", "authenticator"}, "hostProof")
    if proof["contractType"] != "authenticated-host-proof" or proof["schemaVersion"] != 1:
        refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "host proof contract is unsupported")
    enum(proof["proofType"], HOST_PROOF_TYPES, "hostProof.proofType")
    identifier(proof["proofId"], "hostProof.proofId")
    identifier(proof["issuer"], "hostProof.issuer")
    identifier(proof["verifierId"], "hostProof.verifierId")
    identifier(proof["trustRootId"], "hostProof.trustRootId")
    identifier(proof["repositoryDecisionId"], "hostProof.repositoryDecisionId")
    identifier(proof["previousSessionIdentity"], "hostProof.previousSessionIdentity")
    identifier(proof["nextSessionIdentity"], "hostProof.nextSessionIdentity")
    digest(proof["continuationDigest"], "hostProof.continuationDigest")
    timestamp(proof["issuedAt"], "hostProof.issuedAt")
    timestamp(proof["expiresAt"], "hostProof.expiresAt")
    timestamp(verified_at, "verifiedAt")
    try:
        authority = security.load(authority_path, "host-proof")
    except (security.AuthorityError, OSError) as exc:
        refused("MBE-EPOCH-AUTHORITY-UNAVAILABLE", str(exc))
    unsigned = {key: item for key, item in proof.items() if key != "authenticator"}
    if proof["verifierId"] != authority["authorityId"] or proof["trustRootId"] != authority["trustRootId"] or not security.verify(authority, unsigned, proof["authenticator"]):
        refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "host proof authority or authenticator is invalid")
    if instant(verified_at) < instant(proof["issuedAt"]) or instant(verified_at) > instant(proof["expiresAt"]):
        refused("MBE-EPOCH-PROOF-STALE", "host proof is outside its authenticated freshness window")
    if proof["proofType"] == "new-session" and proof["previousSessionIdentity"] == proof["nextSessionIdentity"]:
        refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "new-session proof must change exact session identity")
    return dict(proof)


class MeasuredBudgetRuntime:
    """Deterministic MBE domain state derived solely from ECF v2 events."""

    def __init__(self, store_root: str) -> None:
        self.store = ecf.Store(store_root)

    def _records_locked(self) -> list[dict[str, Any]]:
        verification = self.store.verified()
        records = []
        for event in verification.events:
            matches = [extension for extension in event.get("extensions", []) if extension.get("namespace") == "org.bubbles.mbe"]
            if len(matches) > 1:
                fail("MBE-EXTENSION-DUPLICATE", "internal", "event contains duplicate MBE extensions", 2)
            if len(matches) == 1:
                if matches[0].get("payloadDigest") != event.get("objectDigest"):
                    fail("MBE-EXTENSION-MISMATCH", "internal", "MBE extension payload does not match event object", 2)
                records.append(self.store.require_object(event["objectDigest"]))
        return records

    def records(self) -> list[dict[str, Any]]:
        with self.store.locked():
            return self._records_locked()

    def current_head(self) -> dict[str, Any]:
        with self.store.locked():
            head = self.store.load_head()
            return {"sequence": head["sequence"], "eventDigest": head["eventDigest"]}

    def _append_locked(self, record: dict[str, Any], subject_kind: str, subject_id: str, occurrence_id: str | None = None, attempt_id: str | None = None, expected_predecessor: tuple[int, str] | None = None) -> dict[str, Any]:
        reject_sensitive(record)
        if record.get("contractType") not in {"cost-corpus-manifest", "cost-corpus-evaluation"}:
            try:
                contracts.validate_record(CONTRACT_REGISTRY, record)
            except ValueError as exc:
                fail("MBE-SCHEMA-DRIFT", "integrity", str(exc), 2)
        stored = self.store.put_object(record)
        head = self.store.load_head()
        expected_sequence, expected_digest = expected_predecessor or (head["sequence"], head["eventDigest"])
        hexadecimal = stored["objectDigest"][7:]
        ecf_subject_kind = subject_kind if subject_kind in {"budget", "session"} else f"x.mbe.domain.{subject_kind}"
        proposal = {
            "attemptId": f"att:mbe.{hexadecimal[:48]}",
            "contractType": "execution-control-event",
            "eventId": f"evt:mbe.{hexadecimal[:48]}",
            "eventType": "RECORD",
            "extensions": [{"namespace": "org.bubbles.mbe", "payloadDigest": stored["objectDigest"], "schemaVersion": VERSION}],
            "objectDigest": stored["objectDigest"],
            "occurrenceId": occurrence_id or f"occ:mbe.{hexadecimal[:48]}",
            "posture": "reference-enforce",
            "recordedAt": next((record[name] for name in ("at", "createdAt", "reservedAt", "settledAt", "decidedAt", "observedAt", "negotiatedAt", "quotedAt", "startedAt", "finishedAt", "openedAt", "evaluatedAt", "issuedAt", "consumedAt", "sealedAt", "verifiedAt", "closedAt", "recordedAt") if record.get(name) is not None), None),
            "schemaVersion": 2,
            "subject": {"id": subject_id, "kind": ecf_subject_kind},
            "supersedesEventId": None,
        }
        if proposal["recordedAt"] is None:
            fail("MBE-INTERNAL", "internal", "domain record lacks an event timestamp", 2)
        return self.store.append(expected_sequence, expected_digest, proposal)

    def _find(self, records: list[dict[str, Any]], contract_type: str, field: str, value: str) -> dict[str, Any]:
        matches = [record for record in records if record.get("contractType") == contract_type and record.get(field) == value]
        if len(matches) != 1:
            refused("MBE-IDENTITY-MISSING", f"exact {contract_type} identity is unavailable")
        return matches[0]

    def usage_record(self, *, record: dict[str, Any]) -> dict[str, Any]:
        """Persist one adapter-produced canonical usage-v2 record."""
        try:
            contracts.validate_record(CONTRACT_REGISTRY, record)
        except ValueError as exc:
            fail("MBE-SCHEMA-DRIFT", "integrity", str(exc), 2)
        contract_type = record["contractType"]
        identity_fields = {
            "host-session-identity": "sessionIdentityId",
            "usage-negotiation": "negotiationId",
            "usage-quote": "quoteId",
            "usage-snapshot": "snapshotId",
            "usage-receipt": "usageReceiptId",
            "usage-receipt-verification": "verificationId",
        }
        identity_field = identity_fields.get(contract_type)
        if identity_field is None:
            malformed("usage record contract is not adapter-produced")
        identity = identifier(record[identity_field], identity_field)
        with self.store.locked():
            records = self._records_locked()
            if any(item.get("contractType") == contract_type and item.get(identity_field) == identity for item in records):
                refused("MBE-IDENTITY-CONFLICT", "usage record identity already exists")
            if contract_type != "host-session-identity":
                session_id = record.get("sessionIdentityId")
                if session_id is not None:
                    self._find(records, "host-session-identity", "sessionIdentityId", session_id)
            if contract_type == "usage-receipt":
                prior = [item for item in records if item.get("contractType") == contract_type and item.get("intentId") == record["intentId"]]
                superseded = {item["supersedesReceiptId"] for item in prior if item["supersedesReceiptId"] is not None}
                leaves = [item for item in prior if item["usageReceiptId"] not in superseded]
                if record["revision"] == 1:
                    if record["supersedesReceiptId"] is not None or prior:
                        refused("MBE-RECEIPT-REVISION-CONFLICT", "initial receipt must be revision one with no predecessor")
                elif len(leaves) != 1 or record["supersedesReceiptId"] != leaves[0]["usageReceiptId"] or record["revision"] != leaves[0]["revision"] + 1:
                    refused("MBE-RECEIPT-REVISION-CONFLICT", "receipt correction must replace the unique current leaf")
            if contract_type == "usage-receipt-verification":
                receipt = self._find(records, "usage-receipt", "usageReceiptId", record["usageReceiptId"])
                if record["revision"] != receipt["revision"]:
                    refused("MBE-RECEIPT-REVISION-CONFLICT", "verification revision must equal its receipt revision")
                if record["supersedesVerificationId"] is not None:
                    predecessor = self._find(records, contract_type, "verificationId", record["supersedesVerificationId"])
                    if predecessor["revision"] + 1 != record["revision"]:
                        refused("MBE-RECEIPT-REVISION-CONFLICT", "verification correction must advance one revision")
            self._append_locked(record, "usage", identity)
        return record

    def dispatch_intent(self, *, goal_id: str, budget_id: str, epoch_id: str, session_identity_id: str, occurrence_id: str, attempt_id: str, agent: str, phase: str, action_class: str, action_family: str, input_digest: str, action_digest: str, work_boundary_id: str, repository_decision_id: str, created_at: str) -> dict[str, Any]:
        for value, label in ((goal_id, "goalId"), (budget_id, "budgetId"), (epoch_id, "epochId"), (session_identity_id, "sessionIdentityId"), (occurrence_id, "occurrenceId"), (attempt_id, "attemptId"), (agent, "agent"), (phase, "phase"), (action_class, "actionClass"), (action_family, "actionFamily"), (work_boundary_id, "workBoundaryId"), (repository_decision_id, "repositoryDecisionId")):
            identifier(value, label)
        digest(input_digest, "inputDigest")
        digest(action_digest, "actionDigest")
        timestamp(created_at, "createdAt")
        with self.store.locked():
            records = self._records_locked()
            policy = self._find(records, "goal-budget-policy", "budgetId", budget_id)
            epoch = self._find(records, "session-epoch", "epochId", epoch_id)
            if policy["goalId"] != goal_id or epoch["goalId"] != goal_id or epoch["budgetId"] != budget_id or epoch["hostSessionIdentityId"] != session_identity_id:
                refused("MBE-INTENT-BINDING-MISMATCH", "intent goal, budget, epoch, and exact session must agree")
            if any(item.get("contractType") == "dispatch-intent" and item.get("occurrenceId") == occurrence_id and item.get("attemptId") == attempt_id for item in records):
                refused("MBE-IDENTITY-CONFLICT", "attempt already has a dispatch intent")
            payload = {"goalId": goal_id, "budgetId": budget_id, "epochId": epoch_id, "sessionIdentityId": session_identity_id, "occurrenceId": occurrence_id, "attemptId": attempt_id, "agent": agent, "phase": phase, "actionClass": action_class, "actionFamily": action_family, "inputDigest": input_digest, "actionDigest": action_digest, "workBoundaryId": work_boundary_id, "repositoryDecisionId": repository_decision_id, "createdAt": created_at}
            record = lineaged("dispatch-intent", payload, "intentId", "din", self.store.load_head()["eventDigest"])
            self._append_locked(record, "command", record["intentId"], occurrence_id, attempt_id)
            return record

    def admission_fact(self, *, intent_id: str, fact_type: str, source_record_id: str, source_digest: str, value_digest: str, state: str, issued_at: str, expires_at: str) -> dict[str, Any]:
        identifier(intent_id, "intentId")
        enum(fact_type, ("phase-relevance", "risk-tier", "model-class", "tool-grant", "action-authorization", "retry-eligibility"), "factType")
        identifier(source_record_id, "sourceRecordId")
        digest(source_digest, "sourceDigest")
        digest(value_digest, "valueDigest")
        enum(state, ("verified", "invalid"), "state")
        timestamp(issued_at, "issuedAt")
        timestamp(expires_at, "expiresAt")
        if instant(expires_at) <= instant(issued_at):
            refused("MBE-ADMISSION-FACT-EXPIRED", "admission fact expiry must follow its issue time")
        with self.store.locked():
            records = self._records_locked()
            self._find(records, "dispatch-intent", "intentId", intent_id)
            if any(item.get("contractType") == "admission-fact" and item.get("intentId") == intent_id and item.get("factType") == fact_type for item in records):
                refused("MBE-IDENTITY-CONFLICT", "intent already has this admission fact type")
            payload = {"intentId": intent_id, "factType": fact_type, "sourceRecordId": source_record_id, "sourceDigest": source_digest, "valueDigest": value_digest, "state": state, "issuedAt": issued_at, "expiresAt": expires_at}
            record = lineaged("admission-fact", payload, "factId", "adf", self.store.load_head()["eventDigest"])
            self._append_locked(record, "command", record["factId"])
            return record

    def budget_open(self, *, goal_id: str, policy_digest: str, rollout_posture: str, dimensions: list[dict[str, Any]], opened_at: str, goal_deadline_at: str, max_occurrences: int, max_attempts_per_occurrence: int) -> dict[str, Any]:
        identifier(goal_id, "goalId")
        digest(policy_digest, "policyDigest")
        enum(rollout_posture, ROLLOUT_POSTURES, "rolloutPosture")
        timestamp(opened_at, "openedAt")
        timestamp(goal_deadline_at, "goalDeadlineAt")
        integer(max_occurrences, "maxOccurrences", 1, ecf.MAX_EVENT_COUNT)
        integer(max_attempts_per_occurrence, "maxAttemptsPerOccurrence", 1, ecf.MAX_EVENT_COUNT)
        if instant(goal_deadline_at) <= instant(opened_at):
            refused("MBE-GOAL-DEADLINE-EXPIRED", "goal deadline must follow budget open time")
        normalized = validate_dimensions(dimensions)
        attempt_capacity = max_occurrences * max_attempts_per_occurrence
        projected_records = 8 + attempt_capacity * 12
        projected_bytes = projected_records * MAX_DOMAIN_RECORD_BYTES
        capacity_plan = {
            "attemptCapacity": attempt_capacity,
            "projectedEvents": projected_records,
            "projectedObjects": projected_records,
            "projectedObjectBytes": projected_bytes,
        }
        if projected_records > ecf.MAX_EVENT_COUNT or projected_records > ecf.MAX_OBJECT_COUNT or projected_bytes > ecf.MAX_OBJECT_TOTAL_BYTES:
            refused("MBE-GOAL-CAPACITY-EXCEEDED", "conservative per-goal capacity exceeds ECF bounds")
        record = identified("goal-budget-policy", {
            "goalId": goal_id,
            "policyDigest": policy_digest,
            "rolloutPosture": rollout_posture,
            "openedAt": opened_at,
            "goalDeadlineAt": goal_deadline_at,
            "maxOccurrences": max_occurrences,
            "maxAttemptsPerOccurrence": max_attempts_per_occurrence,
            "dimensions": normalized,
            "capacityPlan": capacity_plan,
        }, "budgetId", "gbp")
        with self.store.locked():
            records = self._records_locked()
            if any(item.get("contractType") == "goal-budget-policy" and item.get("goalId") == goal_id for item in records):
                refused("MBE-BUDGET-ALREADY-OPEN", "goal already has a budget")
            self._append_locked(record, "budget", record["budgetId"])
            suffix = record["budgetId"].split(":")[-1][:32]
            open_occurrence = f"occ:budget.open.{suffix}"
            open_attempt = f"att:budget.open.{suffix}"
            open_event = self._budget_event(record, "OPEN", None, None, open_occurrence, open_attempt, ecf.typed_digest("budget-open-action", {"goalId": goal_id}), [], None, None, opened_at, "budget-open")
            self._append_locked(open_event, "budget", open_event["budgetEventId"], open_occurrence, open_attempt)
        return record

    def _budget_event(self, policy: dict[str, Any], event_type: str, reservation_id: str | None, settlement_id: str | None, occurrence_id: str, attempt_id: str, action_digest: str, amounts: list[dict[str, Any]], receipt_id: str | None, supersedes_event_id: str | None, at: str, reason: str) -> dict[str, Any]:
        payload = {"budgetId": policy["budgetId"], "goalId": policy["goalId"], "eventType": event_type, "reservationId": reservation_id, "settlementId": settlement_id, "occurrenceId": occurrence_id, "attemptId": attempt_id, "actionDigest": action_digest, "amounts": amounts, "usageReceiptId": receipt_id, "supersedesEventId": supersedes_event_id, "at": at, "reason": reason}
        return lineaged("budget-event", payload, "budgetEventId", "bev", self.store.load_head()["eventDigest"])

    def _budget_state(self, records: list[dict[str, Any]], budget_id: str) -> tuple[dict[str, Any], dict[str, Any]]:
        policy = self._find(records, "goal-budget-policy", "budgetId", budget_id)
        all_events = [record for record in records if record.get("contractType") == "budget-event" and record.get("budgetId") == budget_id]
        reservation_records = {record["reservationId"]: record for record in records if record.get("contractType") == "budget-reservation" and record.get("budgetId") == budget_id}
        by_id = {event["budgetEventId"]: event for event in all_events}
        superseded = {event["supersedesEventId"] for event in all_events if event["eventType"] == "CORRECT"}
        events = []
        for event in all_events:
            if event["budgetEventId"] in superseded:
                continue
            if event["eventType"] == "CORRECT":
                target = by_id.get(event["supersedesEventId"])
                if target is None or target["eventType"] == "CORRECT":
                    fail("MBE-INTERNAL", "internal", "correction target is not a settlement leaf", 2)
                event = {**event, "eventType": target["eventType"]}
            events.append(event)
        totals: dict[str, dict[str, int]] = {}
        holds: dict[str, dict[str, Any]] = {}
        closed = False
        reservations: dict[str, dict[str, Any]] = {}
        for event in events:
            kind = event["eventType"]
            if kind == "CLOSE":
                closed = True
            reservation_id = event["reservationId"]
            if kind == "RESERVE" and reservation_id is not None:
                reservations[reservation_id] = {"record": event, "state": "reserved", "debited": {}, "released": {}, "held": {}}
            if event.get("settlementId") is None and reservation_id in reservations:
                if kind == "HOLD":
                    reservations[reservation_id]["state"] = "held"
                    holds[reservation_id] = event
                    reservations[reservation_id]["held"] = {amount_key(row): row["amount"] for row in event["amounts"]}
                elif kind in ("DEBIT", "RELEASE"):
                    account = "debited" if kind == "DEBIT" else "released"
                    reservations[reservation_id][account] = {
                        amount_key(row): reservations[reservation_id][account].get(amount_key(row), 0) + row["amount"]
                        for row in event["amounts"]
                    }
                    reserved = {amount_key(row): row["amount"] for row in reservations[reservation_id]["record"]["amounts"]}
                    accounted = {
                        key: reservations[reservation_id]["debited"].get(key, 0) + reservations[reservation_id]["released"].get(key, 0) + reservations[reservation_id]["held"].get(key, 0)
                        for key in reserved
                    }
                    if all(accounted[key] == reserved[key] for key in reserved):
                        reservations[reservation_id]["state"] = "held" if any(reservations[reservation_id]["held"].values()) else "reconciled"
                        if reservations[reservation_id]["state"] == "reconciled":
                            holds.pop(reservation_id, None)
            if event.get("settlementId") is not None:
                continue
            for amount in event["amounts"]:
                key = amount_key(amount)
                bucket = totals.setdefault(key, {"reserved": 0, "debited": 0, "released": 0, "held": 0, "transferred": 0, "corrected": 0})
                number = amount["amount"]
                if kind == "RESERVE":
                    bucket["reserved"] += number
                elif kind == "DEBIT":
                    bucket["debited"] += number
                elif kind == "RELEASE":
                    bucket["released"] += number
                elif kind == "HOLD":
                    bucket["held"] += number
                if event.get("supersedesEventId") is not None:
                    bucket["corrected"] += number
        settlements = [record for record in records if record.get("contractType") == "budget-settlement" and record.get("budgetId") == budget_id]
        superseded_settlements = {record["supersedesSettlementId"] for record in settlements if record["supersedesSettlementId"] is not None}
        leaves = [record for record in settlements if record["settlementId"] not in superseded_settlements]
        if len({record["reservationId"] for record in leaves}) != len(leaves):
            fail("MBE-INTERNAL", "internal", "reservation has multiple current settlement leaves", 2)
        for settlement in leaves:
            reservation_id = settlement["reservationId"]
            if reservation_id not in reservations:
                fail("MBE-INTERNAL", "internal", "settlement references an unavailable reservation", 2)
            reservation_state = reservations[reservation_id]
            reservation_state["debited"] = {}
            reservation_state["released"] = {}
            reservation_state["held"] = {}
            for partition in settlement["partitions"]:
                key = amount_key(partition)
                reservation_state["debited"][key] = partition["debit"]
                reservation_state["released"][key] = partition["release"]
                reservation_state["held"][key] = partition["hold"]
                bucket = totals.setdefault(key, {"reserved": 0, "debited": 0, "released": 0, "held": 0, "transferred": 0, "corrected": 0})
                bucket["debited"] += partition["debit"]
                bucket["released"] += partition["release"]
                bucket["held"] += partition["hold"]
                if settlement["revision"] > 1:
                    bucket["corrected"] += partition["reserved"]
            if any(reservation_state["held"].values()):
                reservation_state["state"] = "held"
                holds[reservation_id] = settlement
            else:
                reservation_state["state"] = "reconciled"
                holds.pop(reservation_id, None)
        retry_decisions = {record["retryDecisionId"]: record for record in records if record.get("contractType") == "retry-decision"}
        for child in reservation_records.values():
            funding_id = child.get("fundingReservationId")
            retry_id = child.get("retryDecisionId")
            if funding_id is None or retry_id is None:
                continue
            parent = reservations.get(funding_id)
            retry = retry_decisions.get(retry_id)
            if parent is None or retry is None:
                fail("MBE-INTERNAL", "internal", "hold transfer references unavailable lineage", 2)
            for amount in retry["holdTransferAmounts"]:
                key = amount_key(amount)
                transferred = amount["amount"]
                if transferred > parent["held"].get(key, 0):
                    fail("MBE-INTERNAL", "internal", "hold transfer exceeds current parent hold", 2)
                parent["held"][key] -= transferred
                bucket = totals.setdefault(key, {"reserved": 0, "debited": 0, "released": 0, "held": 0, "transferred": 0, "corrected": 0})
                bucket["held"] -= transferred
                bucket["transferred"] += transferred
            if any(parent["held"].values()):
                parent["state"] = "held"
            else:
                parent["state"] = "reconciled"
                holds.pop(funding_id, None)
        return policy, {"closed": closed, "events": events, "holds": holds, "reservations": reservations, "totals": totals}

    def budget_snapshot(self, *, budget_id: str, at: str) -> dict[str, Any]:
        identifier(budget_id, "budgetId")
        timestamp(at, "at")
        with self.store.locked():
            policy, state = self._budget_state(self._records_locked(), budget_id)
            dimensions = []
            for row in policy["dimensions"]:
                key = row["dimension"] if row["dimension"] != "monetaryMinorUnits" else f"{row['dimension']}:{row['currency']}:{row['scale']}"
                totals = state["totals"].get(key, {"reserved": 0, "debited": 0, "released": 0, "held": 0, "transferred": 0, "corrected": 0})
                committed = totals["reserved"] - totals["released"] - totals["held"] - totals["transferred"]
                if committed < 0 or totals["debited"] < 0:
                    fail("MBE-INTERNAL", "internal", "budget counters moved backward", 2)
                held = totals["held"]
                dimensions.append({"dimension": row["dimension"], "state": "unmeasured" if row["state"] == "unconfigured" else "measured", "unit": row["unit"], "currency": row["currency"], "scale": row["scale"], "limit": row["limit"], "reserved": committed, "debited": totals["debited"], "held": held, "corrected": totals["corrected"]})
            head = self.store.load_head()
            payload = {"budgetId": budget_id, "goalId": policy["goalId"], "state": "closed" if state["closed"] else "active", "dimensions": dimensions, "ecfSequence": head["sequence"], "ecfHeadDigest": head["eventDigest"], "at": at}
            payload["snapshotId"] = f"bsp:{ecf.typed_digest('budget-snapshot', payload)[7:]}"
            snapshot = {"contractType": "budget-snapshot", "schemaVersion": VERSION, **payload}
            existing = [record for record in self._records_locked() if record.get("contractType") == "budget-snapshot" and record.get("snapshotId") == snapshot["snapshotId"]]
            if not existing:
                self._append_locked(snapshot, "budget", snapshot["snapshotId"])
                current = self.store.load_head()
                snapshot = {**snapshot, "ecfSequence": current["sequence"], "ecfHeadDigest": current["eventDigest"]}
            return snapshot

    def budget_reserve(self, *, goal_id: str, budget_id: str, epoch_id: str, session_identity_id: str, intent_id: str, occurrence_id: str, attempt_id: str, action_digest: str, adapter_id: str, negotiation_id: str, quote_id: str, quote_digest: str, dimension_set_digest: str, amounts: list[dict[str, Any]], expires_at: str, funding_reservation_id: str | None, retry_decision_id: str | None, expected_ecf_sequence: int, expected_ecf_head_digest: str, at: str) -> dict[str, Any]:
        for value, label in ((goal_id, "goalId"), (budget_id, "budgetId"), (epoch_id, "epochId"), (session_identity_id, "sessionIdentityId"), (intent_id, "intentId"), (occurrence_id, "occurrenceId"), (attempt_id, "attemptId"), (adapter_id, "adapterId"), (negotiation_id, "negotiationId"), (quote_id, "quoteId")):
            identifier(value, label)
        digest(action_digest, "actionDigest")
        digest(quote_digest, "quoteDigest")
        digest(dimension_set_digest, "dimensionSetDigest")
        if funding_reservation_id is not None:
            identifier(funding_reservation_id, "fundingReservationId")
        if retry_decision_id is not None:
            identifier(retry_decision_id, "retryDecisionId")
        integer(expected_ecf_sequence, "expectedEcfSequence", 0, ecf.MAX_EVENT_COUNT)
        digest(expected_ecf_head_digest, "expectedEcfHeadDigest")
        timestamp(expires_at, "expiresAt")
        timestamp(at, "at")
        if instant(expires_at) <= instant(at):
            refused("MBE-QUOTE-EXPIRED", "quote expired before reservation")
        normalized = validate_amounts(amounts, "amounts")
        with self.store.locked():
            records = self._records_locked()
            head = self.store.load_head()
            if head["sequence"] != expected_ecf_sequence or head["eventDigest"] != expected_ecf_head_digest:
                refused("MBE-RESERVATION-CONFLICT", "compare-and-append predecessor is stale")
            policy, state = self._budget_state(records, budget_id)
            if state["closed"]:
                refused("MBE-BUDGET-CLOSED", "budget is closed")
            if policy["goalId"] != goal_id or instant(at) > instant(policy["goalDeadlineAt"]):
                refused("MBE-GOAL-BINDING-MISMATCH", "reservation goal is wrong or its deadline has elapsed")
            epoch = self._find(records, "session-epoch", "epochId", epoch_id)
            intent = self._find(records, "dispatch-intent", "intentId", intent_id)
            negotiation = self._find(records, "usage-negotiation", "negotiationId", negotiation_id)
            quote = self._find(records, "usage-quote", "quoteId", quote_id)
            expected_bindings = {
                "goalId": goal_id,
                "budgetId": budget_id,
                "epochId": epoch_id,
                "sessionIdentityId": session_identity_id,
                "occurrenceId": occurrence_id,
                "attemptId": attempt_id,
                "actionDigest": action_digest,
            }
            if any(epoch.get(key) != value for key, value in expected_bindings.items() if key in epoch):
                refused("MBE-RESERVATION-BINDING-MISMATCH", "epoch binding mismatch")
            if any(intent.get(key) != value for key, value in expected_bindings.items()):
                refused("MBE-RESERVATION-BINDING-MISMATCH", "intent binding mismatch")
            if negotiation["intentId"] != intent_id or negotiation["adapterId"] != adapter_id or negotiation["sessionIdentityId"] != session_identity_id:
                refused("MBE-RESERVATION-BINDING-MISMATCH", "negotiation binding mismatch")
            quote_bindings = {**expected_bindings, "intentId": intent_id, "adapterId": adapter_id, "negotiationId": negotiation_id, "dimensionSetDigest": dimension_set_digest}
            if any(quote.get(key) != value for key, value in quote_bindings.items()):
                refused("MBE-RESERVATION-BINDING-MISMATCH", "quote binding mismatch")
            actual_quote_digest = ecf.typed_digest("usage-quote", quote)
            if quote_digest != actual_quote_digest or quote["expiresAt"] != expires_at or instant(quote["expiresAt"]) <= instant(at):
                refused("MBE-QUOTE-EXPIRED", "quote identity or expiry is invalid")
            configured = {}
            for row in policy["dimensions"]:
                if row["state"] == "configured":
                    key = row["dimension"] if row["dimension"] != "monetaryMinorUnits" else f"{row['dimension']}:{row['currency']}:{row['scale']}"
                    configured[key] = row
            if {amount_key(amount) for amount in normalized} != set(configured):
                refused("MBE-DIMENSION-SET-MISMATCH", "reservation must quote every and only configured dimension")
            quote_amounts = validate_amounts(quote["maximums"], "quote.maximums")
            if {amount_key(amount) for amount in quote_amounts} != set(configured) or quote_amounts != normalized:
                refused("MBE-DIMENSION-SET-MISMATCH", "configured, quoted, and reserved dimension sets and maxima must be equal")
            expected_dimension_digest = ecf.typed_digest("dimension-set", [amount_key(amount) for amount in normalized])
            if dimension_set_digest != expected_dimension_digest or quote["dimensionSetDigest"] != expected_dimension_digest:
                refused("MBE-DIMENSION-SET-MISMATCH", "dimension set digest is not canonical")
            reservation_payload = {"goalId": goal_id, "budgetId": budget_id, "epochId": epoch_id, "sessionIdentityId": session_identity_id, "intentId": intent_id, "occurrenceId": occurrence_id, "attemptId": attempt_id, "actionDigest": action_digest, "adapterId": adapter_id, "negotiationId": negotiation_id, "quoteId": quote_id, "quoteDigest": quote_digest, "dimensionSetDigest": dimension_set_digest, "amounts": normalized, "expiresAt": expires_at, "fundingReservationId": funding_reservation_id, "retryDecisionId": retry_decision_id, "state": "reserved", "reservedAt": at}
            reservation = identified("budget-reservation", reservation_payload, "reservationId", "res")
            reservation_id = reservation["reservationId"]
            retry: dict[str, Any] | None = None
            transfer_by_key: dict[str, int] = {}
            if retry_decision_id is None and funding_reservation_id is not None:
                refused("MBE-RETRY-UNAUTHORIZED", "funding reservation requires a retry decision")
            if retry_decision_id is not None:
                retry = self._find(records, "retry-decision", "retryDecisionId", retry_decision_id)
                if not retry["eligible"] or retry["goalId"] != goal_id or retry["budgetId"] != budget_id or retry["epochId"] != epoch_id or retry["sessionIdentityId"] != session_identity_id or retry["occurrenceId"] != occurrence_id or retry["nextAttemptId"] != attempt_id or retry["actionDigest"] != action_digest or retry["holdFundedReservationId"] != funding_reservation_id:
                    refused("MBE-RETRY-UNAUTHORIZED", "retry decision does not fund this exact attempt")
                if any(item.get("contractType") == "budget-reservation" and item.get("retryDecisionId") == retry_decision_id for item in records):
                    refused("MBE-RETRY-UNAUTHORIZED", "retry decision has already transferred its hold")
                transfer_amounts = validate_amounts(retry["holdTransferAmounts"], "retry.holdTransferAmounts")
                if transfer_amounts != normalized or retry["holdTransferDimensionSetDigest"] != dimension_set_digest:
                    refused("MBE-RETRY-UNAUTHORIZED", "retry transfer must exactly fund the child reservation")
                transfer_by_key = {amount_key(row): row["amount"] for row in transfer_amounts}
                funding = state["reservations"].get(funding_reservation_id)
                if funding is None or funding["state"] != "held":
                    refused("MBE-RETRY-UNAUTHORIZED", "retry funding reservation is not held")
                if not any(transfer_by_key.values()) or any(value > funding["held"].get(key, 0) for key, value in transfer_by_key.items()):
                    refused("MBE-RETRY-UNAUTHORIZED", "retry transfer exceeds the current parent hold")
            if reservation_id in state["reservations"] or any(
                    item["record"]["occurrenceId"] == occurrence_id
                    and item["record"]["attemptId"] == attempt_id
                    and item["record"]["actionDigest"] == action_digest
                    for item in state["reservations"].values()):
                refused("MBE-RESERVATION-CONFLICT", "reservation identity already exists")
            for existing_reservation in state["reservations"].values():
                if existing_reservation["state"] == "held" and existing_reservation["record"]["occurrenceId"] == occurrence_id and existing_reservation["record"]["actionDigest"] == action_digest and existing_reservation["record"]["reservationId"] != funding_reservation_id:
                    outstanding("MBE-USAGE-UNRESOLVED", "equivalent retry is blocked by an unresolved hold")
            for amount in normalized:
                key = amount_key(amount)
                row = configured.get(key)
                if row is None:
                    refused("MBE-DIMENSION-UNCONFIGURED", f"reservation counter is not configured: {key}")
                totals = state["totals"].get(key, {"reserved": 0, "released": 0, "held": 0, "transferred": 0})
                committed = totals["reserved"] - totals["released"] - totals["held"] - totals["transferred"]
                if committed + amount["amount"] > row["limit"]:
                    refused("MBE-BUDGET-EXHAUSTED", f"reservation exceeds independent cap: {key}")
            try:
                self._append_locked(reservation, "budget", reservation_id, occurrence_id, attempt_id, (expected_ecf_sequence, expected_ecf_head_digest))
                event = self._budget_event(policy, "RESERVE", reservation_id, None, occurrence_id, attempt_id, action_digest, normalized, None, None, at, "atomic-reservation")
                self._append_locked(event, "budget", event["budgetEventId"], occurrence_id, attempt_id)
            except ecf.EcfError as exc:
                if exc.code == "ECF-CONFLICT":
                    refused("MBE-RESERVATION-CONFLICT", "compare-and-append predecessor is stale")
                raise
            return reservation

    def _reconcile(self, *, budget_id: str, reservation_id: str | None, event_type: str, amounts: list[dict[str, Any]], occurrence_id: str, attempt_id: str, action_digest: str, usage_receipt_id: str | None, supersedes_event_id: str | None, at: str, reason: str) -> dict[str, Any]:
        enum(event_type, BUDGET_EVENTS, "eventType")
        identifier(budget_id, "budgetId")
        if reservation_id is not None:
            identifier(reservation_id, "reservationId")
        identifier(occurrence_id, "occurrenceId")
        identifier(attempt_id, "attemptId")
        digest(action_digest, "actionDigest")
        if usage_receipt_id is not None:
            identifier(usage_receipt_id, "usageReceiptId")
        if supersedes_event_id is not None:
            identifier(supersedes_event_id, "supersedesEventId")
        timestamp(at, "at")
        normalized = validate_amounts(amounts, "amounts", allow_empty=event_type in ("CLOSE",))
        with self.store.locked():
            records = self._records_locked()
            policy, state = self._budget_state(records, budget_id)
            if state["closed"] and event_type != "CORRECT":
                refused("MBE-BUDGET-CLOSED", "budget is closed")
            if event_type not in ("CLOSE", "CORRECT"):
                reservation = state["reservations"].get(reservation_id)
                if reservation is None:
                    refused("MBE-RESERVATION-MISSING", "reservation is unavailable")
                if reservation["state"] == "reconciled":
                    refused("MBE-RESERVATION-TERMINAL", "reservation is already reconciled")
                reserved_by_key = {amount_key(row): row["amount"] for row in reservation["record"]["amounts"]}
                for row in normalized:
                    key = amount_key(row)
                    already_accounted = reservation["debited"].get(key, 0) + reservation["released"].get(key, 0) + reservation["held"].get(key, 0)
                    if key not in reserved_by_key or already_accounted + row["amount"] > reserved_by_key[key]:
                        refused("MBE-RECEIPT-MISMATCH", "accounting amount exceeds or crosses reserved counter")
                    if row["dimension"] == "wallTimeMs" and event_type == "RELEASE" and row["amount"] != 0:
                        refused("MBE-RECEIPT-MISMATCH", "wall time cannot be released")
            if event_type == "CORRECT":
                targets = [item for item in state["events"] if item.get("budgetEventId") == supersedes_event_id]
                if supersedes_event_id is None or len(targets) != 1 or targets[0]["eventType"] not in ("DEBIT", "RELEASE", "HOLD"):
                    refused("MBE-RECEIPT-MISMATCH", "correction must name an existing budget event")
                target = targets[0]
                if reservation_id != target["reservationId"] or occurrence_id != target["occurrenceId"] or attempt_id != target["attemptId"] or action_digest != target["actionDigest"]:
                    refused("MBE-RECEIPT-MISMATCH", "correction must preserve settlement lineage")
                if {amount_key(row) for row in normalized} != {amount_key(row) for row in target["amounts"]}:
                    refused("MBE-DIMENSION-SET-MISMATCH", "correction must replace the complete target dimension set")
            if event_type == "CLOSE" and any(item["state"] != "reconciled" for item in state["reservations"].values()):
                outstanding("MBE-USAGE-UNRESOLVED", "budget cannot close while any reservation is absent or held")
            event = self._budget_event(policy, event_type, reservation_id, None, occurrence_id, attempt_id, action_digest, normalized, usage_receipt_id, supersedes_event_id, at, reason)
            self._append_locked(event, "budget", event["budgetEventId"], occurrence_id, attempt_id)
            return event

    def budget_debit(self, **kwargs: Any) -> dict[str, Any]:
        return self._reconcile(event_type="DEBIT", supersedes_event_id=None, **kwargs)

    def budget_release(self, **kwargs: Any) -> dict[str, Any]:
        return self._reconcile(event_type="RELEASE", supersedes_event_id=None, **kwargs)

    def budget_hold(self, **kwargs: Any) -> dict[str, Any]:
        return self._reconcile(event_type="HOLD", supersedes_event_id=None, **kwargs)

    def budget_correct(self, **kwargs: Any) -> dict[str, Any]:
        return self._reconcile(event_type="CORRECT", **kwargs)

    def budget_close(self, *, budget_id: str, occurrence_id: str, attempt_id: str, action_digest: str, at: str, reason: str) -> dict[str, Any]:
        return self._reconcile(budget_id=budget_id, reservation_id=None, event_type="CLOSE", amounts=[], occurrence_id=occurrence_id, attempt_id=attempt_id, action_digest=action_digest, usage_receipt_id=None, supersedes_event_id=None, at=at, reason=reason)

    def budget_settle(self, *, budget_id: str, reservation_id: str, permit_id: str, consumption_id: str, usage_receipt_id: str, receipt_verification_id: str, revision: int, supersedes_settlement_id: str | None, partitions: list[dict[str, Any]], terminal_state: str, settled_at: str) -> dict[str, Any]:
        for value, label in ((budget_id, "budgetId"), (reservation_id, "reservationId"), (permit_id, "permitId"), (consumption_id, "consumptionId"), (usage_receipt_id, "usageReceiptId"), (receipt_verification_id, "receiptVerificationId")):
            identifier(value, label)
        integer(revision, "revision", 1, MAX_AMOUNT)
        if supersedes_settlement_id is not None:
            identifier(supersedes_settlement_id, "supersedesSettlementId")
        enum(terminal_state, ("debit", "release", "hold"), "terminalState")
        timestamp(settled_at, "settledAt")
        if not isinstance(partitions, list) or not partitions or len(partitions) > MAX_ROWS:
            malformed("partitions must be a bounded non-empty array")
        normalized_partitions = []
        seen = set()
        for index, row in enumerate(partitions):
            value = exact_fields(row, {"dimension", "unit", "currency", "scale", "reserved", "debit", "release", "hold"}, f"partitions[{index}]")
            amount_template = {"dimension": value["dimension"], "amount": value["reserved"], "unit": value["unit"], "currency": value["currency"], "scale": value["scale"]}
            validate_amount(amount_template, f"partitions[{index}]")
            for key in ("reserved", "debit", "release", "hold"):
                integer(value[key], f"partitions[{index}].{key}")
            if value["debit"] + value["release"] + value["hold"] != value["reserved"]:
                refused("MBE-SETTLEMENT-PARTITION-MISMATCH", "every dimension must satisfy debit + release + hold = reserved")
            key = amount_key(amount_template)
            if key in seen:
                malformed("settlement partitions contain a duplicate dimension counter")
            seen.add(key)
            normalized_partitions.append(dict(value))
        normalized_partitions.sort(key=lambda row: amount_key({"dimension": row["dimension"], "currency": row["currency"], "scale": row["scale"]}))
        with self.store.locked():
            records = self._records_locked()
            policy, state = self._budget_state(records, budget_id)
            reservation = self._find(records, "budget-reservation", "reservationId", reservation_id)
            permit = self._find(records, "dispatch-permit", "permitId", permit_id)
            consumption = self._find(records, "permit-consumption", "consumptionId", consumption_id)
            receipt = self._find(records, "usage-receipt", "usageReceiptId", usage_receipt_id)
            verification = self._find(records, "usage-receipt-verification", "verificationId", receipt_verification_id)
            if verification["usageReceiptId"] != usage_receipt_id or verification["verdict"] != "valid":
                refused("MBE-RECEIPT-UNVERIFIED", "settlement requires a valid receipt verification")
            binding_fields = ("goalId", "budgetId", "epochId", "sessionIdentityId", "occurrenceId", "attemptId", "actionDigest", "intentId")
            for related in (permit, consumption, receipt):
                if any(related.get(key) != reservation.get(key) for key in binding_fields):
                    refused("MBE-RECEIPT-MISMATCH", "settlement graph binding mismatch")
            if permit["reservationId"] != reservation_id or consumption["permitId"] != permit_id or receipt["permitId"] != permit_id:
                refused("MBE-RECEIPT-MISMATCH", "permit, consumption, receipt, and reservation are not the same dispatch")
            reserved = {amount_key(row): row for row in reservation["amounts"]}
            partition_keys = {amount_key({"dimension": row["dimension"], "currency": row["currency"], "scale": row["scale"]}): row for row in normalized_partitions}
            if set(partition_keys) != set(reserved):
                refused("MBE-DIMENSION-SET-MISMATCH", "settlement must account every and only reserved dimensions")
            for key, source in reserved.items():
                row = partition_keys[key]
                if row["reserved"] != source["amount"] or row["unit"] != source["unit"]:
                    refused("MBE-SETTLEMENT-PARTITION-MISMATCH", "settlement reserved values must equal the reservation")
                if source["dimension"] == "wallTimeMs" and row["release"] != 0:
                    refused("MBE-SETTLEMENT-PARTITION-MISMATCH", "wall time cannot be released")
            nonzero_kinds = {kind for row in normalized_partitions for kind in ("debit", "release", "hold") if row[kind] > 0}
            if terminal_state not in nonzero_kinds:
                refused("MBE-SETTLEMENT-PARTITION-MISMATCH", "terminal state must identify a non-zero settlement partition")
            receipt_measurement = {amount_key(row): row["amount"] for row in validate_amounts(receipt["measurement"], "receipt.measurement", allow_empty=True)}
            for key, row in partition_keys.items():
                measured = receipt_measurement.get(key)
                if measured is not None and row["debit"] != measured:
                    refused("MBE-RECEIPT-MISMATCH", "debit must equal the verified measured amount")
            settlements = [item for item in records if item.get("contractType") == "budget-settlement" and item.get("reservationId") == reservation_id]
            superseded = {item["supersedesSettlementId"] for item in settlements if item["supersedesSettlementId"] is not None}
            leaves = [item for item in settlements if item["settlementId"] not in superseded]
            if revision == 1:
                if supersedes_settlement_id is not None or settlements:
                    refused("MBE-SETTLEMENT-REVISION-CONFLICT", "initial settlement must be revision one with no predecessor")
            elif len(leaves) != 1 or supersedes_settlement_id != leaves[0]["settlementId"] or revision != leaves[0]["revision"] + 1:
                refused("MBE-SETTLEMENT-REVISION-CONFLICT", "correction must replace the unique current settlement leaf")
            dimension_set_digest = ecf.typed_digest("dimension-set", list(sorted(partition_keys)))
            if dimension_set_digest != reservation["dimensionSetDigest"]:
                refused("MBE-DIMENSION-SET-MISMATCH", "settlement dimension digest differs from reservation")
            payload = {"revision": revision, "supersedesSettlementId": supersedes_settlement_id, "budgetId": budget_id, "goalId": reservation["goalId"], "reservationId": reservation_id, "intentId": reservation["intentId"], "permitId": permit_id, "consumptionId": consumption_id, "usageReceiptId": usage_receipt_id, "receiptVerificationId": receipt_verification_id, "occurrenceId": reservation["occurrenceId"], "attemptId": reservation["attemptId"], "actionDigest": reservation["actionDigest"], "dimensionSetDigest": dimension_set_digest, "partitions": normalized_partitions, "terminalState": terminal_state, "settledAt": settled_at}
            settlement = lineaged("budget-settlement", payload, "settlementId", "bst", self.store.load_head()["eventDigest"])
            self._append_locked(settlement, "budget", settlement["settlementId"], reservation["occurrenceId"], reservation["attemptId"])
            return settlement

    def retry_decide(self, *, goal_id: str, budget_id: str, epoch_id: str, session_identity_id: str, occurrence_id: str, prior_attempt_id: str, next_attempt_id: str, action_digest: str, failure_class: str, eligible: bool, hold_funded_reservation_id: str | None, hold_transfer_dimension_set_digest: str | None, hold_transfer_amounts: list[dict[str, Any]], decided_at: str) -> dict[str, Any]:
        for value, label in ((goal_id, "goalId"), (budget_id, "budgetId"), (epoch_id, "epochId"), (session_identity_id, "sessionIdentityId"), (occurrence_id, "occurrenceId"), (prior_attempt_id, "priorAttemptId"), (next_attempt_id, "nextAttemptId")):
            identifier(value, label)
        digest(action_digest, "actionDigest")
        enum(failure_class, ("none", "transient", "permanent", "unresolved"), "failureClass")
        boolean(eligible, "eligible")
        timestamp(decided_at, "decidedAt")
        if hold_funded_reservation_id is not None:
            identifier(hold_funded_reservation_id, "holdFundedReservationId")
        if hold_transfer_dimension_set_digest is not None:
            digest(hold_transfer_dimension_set_digest, "holdTransferDimensionSetDigest")
        transfers = validate_amounts(hold_transfer_amounts, "holdTransferAmounts", allow_empty=not eligible)
        with self.store.locked():
            records = self._records_locked()
            if eligible:
                if failure_class != "transient" or hold_funded_reservation_id is None or hold_transfer_dimension_set_digest is None:
                    refused("MBE-RETRY-UNAUTHORIZED", "eligible retry requires a transient failure and held funding")
                reservation = self._find(records, "budget-reservation", "reservationId", hold_funded_reservation_id)
                settlements = [item for item in records if item.get("contractType") == "budget-settlement" and item.get("reservationId") == hold_funded_reservation_id]
                superseded = {item["supersedesSettlementId"] for item in settlements if item["supersedesSettlementId"] is not None}
                leaves = [item for item in settlements if item["settlementId"] not in superseded]
                held = {amount_key({"dimension": row["dimension"], "currency": row["currency"], "scale": row["scale"]}): row["hold"] for row in leaves[0]["partitions"] if row["hold"] > 0}
                transfer_by_key = {amount_key(row): row["amount"] for row in transfers}
                expected_transfer_digest = ecf.typed_digest("dimension-set", sorted(transfer_by_key))
                if len(leaves) != 1 or not held or not any(transfer_by_key.values()) or hold_transfer_dimension_set_digest != expected_transfer_digest or any(value > held.get(key, 0) for key, value in transfer_by_key.items()) or reservation["goalId"] != goal_id or reservation["budgetId"] != budget_id or reservation["epochId"] != epoch_id or reservation["sessionIdentityId"] != session_identity_id or reservation["occurrenceId"] != occurrence_id or reservation["attemptId"] != prior_attempt_id or reservation["actionDigest"] != action_digest:
                    refused("MBE-RETRY-UNAUTHORIZED", "retry funding is not the current hold for this exact occurrence")
            elif hold_funded_reservation_id is not None or hold_transfer_dimension_set_digest is not None or transfers:
                refused("MBE-RETRY-UNAUTHORIZED", "ineligible retry cannot transfer held funding")
            if any(item.get("contractType") == "retry-decision" and item.get("occurrenceId") == occurrence_id and item.get("nextAttemptId") == next_attempt_id for item in records):
                refused("MBE-RETRY-UNAUTHORIZED", "next attempt already has a retry decision")
            payload = {"goalId": goal_id, "budgetId": budget_id, "epochId": epoch_id, "sessionIdentityId": session_identity_id, "occurrenceId": occurrence_id, "priorAttemptId": prior_attempt_id, "nextAttemptId": next_attempt_id, "actionDigest": action_digest, "failureClass": failure_class, "eligible": eligible, "holdFundedReservationId": hold_funded_reservation_id, "holdTransferDimensionSetDigest": hold_transfer_dimension_set_digest, "holdTransferAmounts": transfers, "decidedAt": decided_at}
            decision = lineaged("retry-decision", payload, "retryDecisionId", "rtd", self.store.load_head()["eventDigest"])
            self._append_locked(decision, "command", decision["retryDecisionId"], occurrence_id, next_attempt_id)
            return decision

    def admission_evaluate(self, *, intent_id: str, budget_id: str, epoch_id: str, session_identity_id: str, occurrence_id: str, attempt_id: str, action_digest: str, negotiation_id: str, quote_id: str, reservation_id: str, epoch_verification_id: str, fact_ids: list[str], evaluated_at: str) -> dict[str, Any]:
        for value, label in ((intent_id, "intentId"), (budget_id, "budgetId"), (epoch_id, "epochId"), (session_identity_id, "sessionIdentityId"), (occurrence_id, "occurrenceId"), (attempt_id, "attemptId"), (negotiation_id, "negotiationId"), (quote_id, "quoteId"), (reservation_id, "reservationId"), (epoch_verification_id, "epochVerificationId")):
            identifier(value, label)
        digest(action_digest, "actionDigest")
        timestamp(evaluated_at, "evaluatedAt")
        with self.store.locked():
            records = self._records_locked()
            policy, state = self._budget_state(records, budget_id)
            intent = self._find(records, "dispatch-intent", "intentId", intent_id)
            epoch = self._find(records, "session-epoch", "epochId", epoch_id)
            verification = self._find(records, "epoch-verification", "epochVerificationId", epoch_verification_id)
            negotiation = self._find(records, "usage-negotiation", "negotiationId", negotiation_id)
            quote = self._find(records, "usage-quote", "quoteId", quote_id)
            reservation = self._find(records, "budget-reservation", "reservationId", reservation_id)
            reasons: list[str] = []
            expected = {"intentId": intent_id, "goalId": policy["goalId"], "budgetId": budget_id, "epochId": epoch_id, "sessionIdentityId": session_identity_id, "occurrenceId": occurrence_id, "attemptId": attempt_id, "actionDigest": action_digest}
            for related in (intent, reservation):
                if any(related.get(key) != value for key, value in expected.items()):
                    reasons.append("MBE-RESERVATION-BINDING-MISMATCH")
                    break
            if state["closed"]:
                reasons.append("MBE-BUDGET-CLOSED")
            if state["reservations"].get(reservation_id, {}).get("state") != "reserved":
                reasons.append("MBE-RESERVATION-MISSING")
            if instant(reservation["expiresAt"]) <= instant(evaluated_at):
                reasons.append("MBE-QUOTE-EXPIRED")
            if epoch["epochVerificationId"] != epoch_verification_id or epoch["hostSessionIdentityId"] != session_identity_id or verification["verdict"] != "verified" or verification["boundaryId"] != epoch["openedByBoundaryId"]:
                reasons.append("MBE-EPOCH-BOUNDARY-UNVERIFIED")
            if negotiation["intentId"] != intent_id or quote["negotiationId"] != negotiation_id or reservation["quoteId"] != quote_id:
                reasons.append("MBE-RESERVATION-BINDING-MISMATCH")
            if not isinstance(fact_ids, list) or len(fact_ids) != len(set(fact_ids)):
                malformed("factIds must be a unique array")
            facts = [self._find(records, "admission-fact", "factId", identifier(value, "factId")) for value in fact_ids]
            if any(fact["intentId"] != intent_id for fact in facts):
                reasons.append("MBE-ADMISSION-FACT-BINDING-MISMATCH")
            if any(instant(fact["expiresAt"]) <= instant(evaluated_at) for fact in facts):
                reasons.append("MBE-ADMISSION-FACT-EXPIRED")
            fact_types = {fact["factType"] for fact in facts if fact["intentId"] == intent_id and fact["state"] == "verified"}
            required_types = {"phase-relevance", "risk-tier", "model-class", "tool-grant", "action-authorization", "retry-eligibility"}
            if fact_types != required_types or len(facts) != len(required_types):
                reasons.append("MBE-ADMISSION-FACT-INCOMPLETE")
            fact_refs = [{"factId": fact["factId"], "factType": fact["factType"], "sourceRecordId": fact["sourceRecordId"], "sourceDigest": fact["sourceDigest"], "valueDigest": fact["valueDigest"], "issuedAt": fact["issuedAt"], "expiresAt": fact["expiresAt"]} for fact in sorted(facts, key=lambda item: item["factType"])]
            verdict = "permit-eligible" if not reasons else "refused"
            payload = {**expected, "negotiationId": negotiation_id, "quoteId": quote_id, "reservationId": reservation_id, "epochVerificationId": epoch_verification_id, "factRefs": fact_refs, "predecessorDigest": self.store.load_head()["eventDigest"], "domainDigest": intent["domainDigest"], "verdict": verdict, "reasonCode": "MBE-ADMISSION-SATISFIED" if not reasons else reasons[0], "evaluatedAt": evaluated_at}
            decision = identified("admission-decision", payload, "decisionId", "adm")
            self._append_locked(decision, "command", decision["decisionId"], occurrence_id)
        return decision

    def permit_issue(self, *, decision_id: str, nonce: str, expires_at: str, issued_at: str, enforcement_kind: str) -> dict[str, Any]:
        for value, label in ((decision_id, "decisionId"), (nonce, "nonce")):
            identifier(value, label)
        timestamp(expires_at, "expiresAt")
        timestamp(issued_at, "issuedAt")
        if enforcement_kind == "host-native":
            unsupported("MBE-HOST-ENFORCEMENT-UNAVAILABLE", "host-native interception is unsupported")
        if enforcement_kind != "repository-reference":
            malformed("enforcementKind is outside its closed vocabulary")
        with self.store.locked():
            records = self._records_locked()
            decision = self._find(records, "admission-decision", "decisionId", decision_id)
            reservation = self._find(records, "budget-reservation", "reservationId", decision["reservationId"])
            quote = self._find(records, "usage-quote", "quoteId", decision["quoteId"])
            if decision["verdict"] != "permit-eligible":
                refused("MBE-PERMIT-INVALID", "decision is not permit eligible")
            if any(record.get("contractType") == "dispatch-permit" and (record.get("decisionId") == decision_id or record.get("reservationId") == decision["reservationId"]) for record in records):
                refused("MBE-PERMIT-INVALID", "a decision and reservation may issue at most one permit")
            if instant(expires_at) <= instant(issued_at):
                refused("MBE-QUOTE-EXPIRED", "permit expiry is not after issue time")
            payload = {"decisionId": decision_id, "reservationId": decision["reservationId"], "intentId": decision["intentId"], "goalId": decision["goalId"], "budgetId": decision["budgetId"], "epochId": decision["epochId"], "sessionIdentityId": decision["sessionIdentityId"], "occurrenceId": decision["occurrenceId"], "attemptId": decision["attemptId"], "actionDigest": decision["actionDigest"], "adapterId": reservation["adapterId"], "quoteId": decision["quoteId"], "quoteDigest": ecf.typed_digest("usage-quote", quote), "nonce": nonce, "expiresAt": expires_at, "issuedAt": issued_at, "enforcementKind": enforcement_kind, "oneUse": True}
            permit = identified("dispatch-permit", payload, "permitId", "dpm")
            self._append_locked(permit, "command", permit["permitId"], decision["occurrenceId"])
            return permit

    def permit_get(self, *, permit_id: str) -> dict[str, Any]:
        """Read-only lookup of one dispatch-permit record by permitId. Never
        mutates the ledger. Exists because IMP-056 SCOPE-4's gateway needs the
        full permit (reservationId, occurrenceId, enforcementKind, ...) to
        mint a mutable-dispatch-authorization, and the only record a caller
        otherwise holds by that point is the narrower permit-consumption
        shape passed to the broker."""
        identifier(permit_id, "permitId")
        with self.store.locked():
            records = self._records_locked()
            return self._find(records, "dispatch-permit", "permitId", permit_id)

    def permit_consume(self, *, permit_id: str, nonce: str, consumed_at: str, action_digest: str | None = None) -> dict[str, Any]:
        for value, label in ((permit_id, "permitId"), (nonce, "nonce")):
            identifier(value, label)
        timestamp(consumed_at, "consumedAt")
        if action_digest is not None:
            digest(action_digest, "actionDigest")
        with self.store.locked():
            records = self._records_locked()
            permit = self._find(records, "dispatch-permit", "permitId", permit_id)
            if action_digest is not None and action_digest != permit["actionDigest"]:
                refused("MBE-PERMIT-INVALID", "consumption action digest differs from permit")
            if any(record.get("contractType") == "permit-consumption" and record.get("permitId") == permit_id for record in records):
                refused("MBE-PERMIT-INVALID", "permit replay is forbidden")
            if permit["nonce"] != nonce:
                refused("MBE-PERMIT-INVALID", "permit binding mismatch")
            if instant(consumed_at) > instant(permit["expiresAt"]):
                refused("MBE-PERMIT-INVALID", "permit is expired")
            payload = {key: permit[key] for key in ("permitId", "decisionId", "reservationId", "intentId", "goalId", "budgetId", "epochId", "sessionIdentityId", "occurrenceId", "attemptId", "actionDigest", "adapterId", "quoteId", "quoteDigest", "nonce")}
            payload["consumedAt"] = consumed_at
            consumption = identified("permit-consumption", payload, "consumptionId", "dpc")
            self._append_locked(consumption, "command", consumption["consumptionId"], permit["occurrenceId"])
            return consumption

    # IMP-056 SCOPE-2. A launch-state record answers one question a permit
    # record cannot: did process creation actually happen? Permit consumption
    # proves authorization was spent; it says nothing about whether the broker
    # then crashed before, during, or after launching the child. Recording
    # intent BEFORE consuming (launch_pending) and a terminal fact AFTER
    # (launch_confirm/launch_deny) turns "the broker died somewhere in here"
    # from an unanswerable question into a durable, reconcilable record.
    def _launch_records_for_permit(self, records: list[dict[str, Any]], permit_id: str) -> list[dict[str, Any]]:
        # _records_locked() returns records in ledger append order, so the
        # last entry for a permit IS its current state -- no separate
        # predecessor-chain field is needed to answer "what happened last".
        return [record for record in records if record.get("contractType") == "dispatch-launch-state" and record.get("permitId") == permit_id]

    def launch_pending(self, *, permit_id: str, nonce: str, recorded_at: str) -> dict[str, Any]:
        identifier(permit_id, "permitId")
        identifier(nonce, "nonce")
        timestamp(recorded_at, "recordedAt")
        with self.store.locked():
            records = self._records_locked()
            permit = self._find(records, "dispatch-permit", "permitId", permit_id)
            if permit["nonce"] != nonce:
                refused("MBE-PERMIT-INVALID", "permit binding mismatch")
            if self._launch_records_for_permit(records, permit_id):
                refused("MBE-PERMIT-INVALID", "a permit may enter launch-pending at most once")
            payload = {"permitId": permit_id, "reservationId": permit["reservationId"], "occurrenceId": permit["occurrenceId"], "state": "launch-pending", "recordedAt": recorded_at}
            record = identified("dispatch-launch-state", payload, "launchStateId", "dls")
            self._append_locked(record, "command", record["launchStateId"], permit["occurrenceId"])
            return record

    def _launch_terminal(self, *, permit_id: str, state: str, recorded_at: str) -> dict[str, Any]:
        identifier(permit_id, "permitId")
        timestamp(recorded_at, "recordedAt")
        with self.store.locked():
            records = self._records_locked()
            permit = self._find(records, "dispatch-permit", "permitId", permit_id)
            launch_records = self._launch_records_for_permit(records, permit_id)
            if not launch_records:
                refused("MBE-PERMIT-INVALID", "no launch-pending record exists for this permit")
            latest = launch_records[-1]
            if latest["state"] != "launch-pending":
                refused("MBE-PERMIT-INVALID", f"launch state is already terminal ({latest['state']}); a second terminal record is forbidden")
            payload = {"permitId": permit_id, "reservationId": permit["reservationId"], "occurrenceId": permit["occurrenceId"], "state": state, "recordedAt": recorded_at}
            record = identified("dispatch-launch-state", payload, "launchStateId", "dls")
            self._append_locked(record, "command", record["launchStateId"], permit["occurrenceId"])
            return record

    def launch_confirm(self, *, permit_id: str, recorded_at: str) -> dict[str, Any]:
        """Process creation is durably confirmed. Never a claim about the
        child's exit code or output -- only that it started."""
        return self._launch_terminal(permit_id=permit_id, state="launch-confirmed", recorded_at=recorded_at)

    def launch_deny(self, *, permit_id: str, recorded_at: str) -> dict[str, Any]:
        """Authorization or launch preparation failed BEFORE process creation
        (e.g. the capability probe or snapshot refused). Distinct from
        launch-ambiguous: denial means creation provably did not happen."""
        return self._launch_terminal(permit_id=permit_id, state="launch-denied", recorded_at=recorded_at)

    def launch_reconcile(self, *, recorded_at: str) -> dict[str, Any]:
        """Recovery pass: every permit whose latest launch-state record is
        still launch-pending has no durable evidence of a terminal outcome
        (the broker never returned to record one -- most likely a crash
        between consuming the permit and confirming or denying the launch).
        Promote each to launch-ambiguous. Idempotent: a permit already at a
        terminal state is untouched, so running this twice changes nothing
        the second time. This is the ONLY way an ambiguous record is
        produced -- there is no direct launch-ambiguous entry point, because
        ambiguity is a recovery FINDING, never a claim a caller gets to make
        about its own launch."""
        timestamp(recorded_at, "recordedAt")
        with self.store.locked():
            records = self._records_locked()
            by_permit: dict[str, list[dict[str, Any]]] = {}
            for record in records:
                if record.get("contractType") == "dispatch-launch-state":
                    by_permit.setdefault(record["permitId"], []).append(record)
            promoted: list[str] = []
            for permit_id, launch_records in by_permit.items():
                if launch_records[-1]["state"] != "launch-pending":
                    continue
                permit = self._find(records, "dispatch-permit", "permitId", permit_id)
                payload = {"permitId": permit_id, "reservationId": permit["reservationId"], "occurrenceId": permit["occurrenceId"], "state": "launch-ambiguous", "recordedAt": recorded_at}
                record = identified("dispatch-launch-state", payload, "launchStateId", "dls")
                self._append_locked(record, "command", record["launchStateId"], permit["occurrenceId"])
                promoted.append(record["launchStateId"])
            return {"contractType": "dispatch-launch-reconciliation", "schemaVersion": VERSION, "promotedLaunchStateIds": promoted, "recordedAt": recorded_at}

    def epoch_boundary(self, *, goal_id: str, from_epoch_id: str, to_epoch_class: str, boundary_kind: str, previous_session_identity_id: str, next_session_identity_id: str, continuation_digest: str, budget_snapshot_id: str, host_proof_digest: str, observed_at: str) -> dict[str, Any]:
        for value, label in ((goal_id, "goalId"), (from_epoch_id, "fromEpochId"), (previous_session_identity_id, "previousSessionIdentityId"), (next_session_identity_id, "nextSessionIdentityId"), (budget_snapshot_id, "budgetSnapshotId")):
            identifier(value, label)
        enum(to_epoch_class, EPOCH_CLASSES, "toEpochClass")
        enum(boundary_kind, ("initial-host-checkpoint", "host-checkpoint", "new-session"), "boundaryKind")
        digest(continuation_digest, "continuationDigest")
        digest(host_proof_digest, "hostProofDigest")
        timestamp(observed_at, "observedAt")
        with self.store.locked():
            records = self._records_locked()
            self._find(records, "host-session-identity", "sessionIdentityId", next_session_identity_id)
            snapshot = self._find(records, "budget-snapshot", "snapshotId", budget_snapshot_id)
            if snapshot["goalId"] != goal_id:
                refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "boundary budget snapshot belongs to another goal")
            if boundary_kind == "initial-host-checkpoint":
                if from_epoch_id != "epoch:none" or previous_session_identity_id != next_session_identity_id:
                    refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "initial boundary must bind one independently identified host session")
            else:
                previous_epoch = self._find(records, "session-epoch", "epochId", from_epoch_id)
                if previous_epoch["goalId"] != goal_id or previous_epoch["hostSessionIdentityId"] != previous_session_identity_id:
                    refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "boundary predecessor does not match the prior epoch")
                self._find(records, "epoch-close", "epochId", from_epoch_id)
                if boundary_kind == "new-session" and previous_session_identity_id == next_session_identity_id:
                    refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "new-session boundary must change exact session identity")
            payload = {"goalId": goal_id, "fromEpochId": from_epoch_id, "toEpochClass": to_epoch_class, "boundaryKind": boundary_kind, "previousSessionIdentityId": previous_session_identity_id, "nextSessionIdentityId": next_session_identity_id, "continuationDigest": continuation_digest, "budgetSnapshotId": budget_snapshot_id, "hostProofDigest": host_proof_digest, "observedAt": observed_at}
            boundary = lineaged("epoch-boundary-receipt", payload, "boundaryId", "ebr", self.store.load_head()["eventDigest"])
            self._append_locked(boundary, "session", boundary["boundaryId"])
            return boundary

    def epoch_verify(self, *, boundary_id: str, host_proof: dict[str, Any], authority_path: str, verified_at: str) -> dict[str, Any]:
        identifier(boundary_id, "boundaryId")
        timestamp(verified_at, "verifiedAt")
        proof = validate_host_proof(host_proof, authority_path, verified_at)
        host_proof_digest = ecf.typed_digest("authenticated-host-proof", proof)
        with self.store.locked():
            records = self._records_locked()
            boundary = self._find(records, "epoch-boundary-receipt", "boundaryId", boundary_id)
            bindings = {"previousSessionIdentity": "previousSessionIdentityId", "nextSessionIdentity": "nextSessionIdentityId", "continuationDigest": "continuationDigest"}
            if boundary["hostProofDigest"] != host_proof_digest or any(proof[source] != boundary[target] for source, target in bindings.items()):
                refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "authenticated host proof does not exactly bind the boundary")
            payload = {"boundaryId": boundary_id, "goalId": boundary["goalId"], "previousSessionIdentityId": boundary["previousSessionIdentityId"], "nextSessionIdentityId": boundary["nextSessionIdentityId"], "continuationDigest": boundary["continuationDigest"], "hostProofDigest": host_proof_digest, "proofId": proof["proofId"], "verifierId": proof["verifierId"], "trustRootId": proof["trustRootId"], "issuedAt": proof["issuedAt"], "expiresAt": proof["expiresAt"], "verdict": "verified", "verifiedAt": verified_at}
            verification = lineaged("epoch-verification", payload, "epochVerificationId", "epv", self.store.load_head()["eventDigest"])
            self._append_locked(verification, "session", verification["epochVerificationId"])
            return verification

    def epoch_open(self, *, goal_id: str, budget_id: str, epoch_class: str, sequence: int, host_session_identity_id: str, opened_by_boundary_id: str, epoch_verification_id: str, continuation_digest: str, opened_at: str) -> dict[str, Any]:
        for value, label in ((goal_id, "goalId"), (budget_id, "budgetId"), (host_session_identity_id, "hostSessionIdentityId"), (opened_by_boundary_id, "openedByBoundaryId"), (epoch_verification_id, "epochVerificationId")):
            identifier(value, label)
        enum(epoch_class, EPOCH_CLASSES, "epochClass")
        integer(sequence, "sequence", 1, 4)
        digest(continuation_digest, "continuationDigest")
        timestamp(opened_at, "openedAt")
        if EPOCH_CLASSES[sequence - 1] != epoch_class:
            malformed("epoch sequence and class disagree")
        with self.store.locked():
            records = self._records_locked()
            policy = self._find(records, "goal-budget-policy", "budgetId", budget_id)
            boundary = self._find(records, "epoch-boundary-receipt", "boundaryId", opened_by_boundary_id)
            verification = self._find(records, "epoch-verification", "epochVerificationId", epoch_verification_id)
            if policy["goalId"] != goal_id or verification["verdict"] != "verified" or verification["boundaryId"] != opened_by_boundary_id:
                refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "epoch requires a verified boundary for its goal")
            if boundary["goalId"] != goal_id or boundary["toEpochClass"] != epoch_class or boundary["nextSessionIdentityId"] != host_session_identity_id or boundary["continuationDigest"] != continuation_digest:
                refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "epoch fields do not equal verified boundary fields")
            closed_epoch_ids = {record["epochId"] for record in records if record.get("contractType") == "epoch-close"}
            active = [record for record in records if record.get("contractType") == "session-epoch" and record.get("goalId") == goal_id and record.get("epochId") not in closed_epoch_ids]
            if active:
                refused("MBE-EPOCH-ACTIVE", "an epoch is already active")
            prior = [record for record in records if record.get("contractType") == "session-epoch" and record.get("goalId") == goal_id]
            if len(prior) != sequence - 1:
                refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "epoch sequence does not extend the exact goal chain")
            payload = {"goalId": goal_id, "budgetId": budget_id, "epochClass": epoch_class, "sequence": sequence, "hostSessionIdentityId": host_session_identity_id, "openedByBoundaryId": opened_by_boundary_id, "epochVerificationId": epoch_verification_id, "continuationDigest": continuation_digest, "state": "active", "openedAt": opened_at}
            epoch = lineaged("session-epoch", payload, "epochId", "sep", self.store.load_head()["eventDigest"])
            self._append_locked(epoch, "session", epoch["epochId"])
            return epoch

    def epoch_close(self, *, epoch_id: str, continuation_digest: str, closed_at: str) -> dict[str, Any]:
        identifier(epoch_id, "epochId")
        digest(continuation_digest, "continuationDigest")
        timestamp(closed_at, "closedAt")
        with self.store.locked():
            records = self._records_locked()
            epoch = self._find(records, "session-epoch", "epochId", epoch_id)
            if any(record.get("contractType") == "epoch-close" and record.get("epochId") == epoch_id for record in records):
                refused("MBE-EPOCH-CLOSED", "epoch is already closed")
            if epoch["continuationDigest"] != continuation_digest:
                refused("MBE-EPOCH-BOUNDARY-UNVERIFIED", "continuation digest changed")
            payload = {"epochId": epoch_id, "goalId": epoch["goalId"], "epochClass": epoch["epochClass"], "sequence": epoch["sequence"], "continuationDigest": continuation_digest, "closedAt": closed_at}
            close = lineaged("epoch-close", payload, "closeId", "sec", self.store.load_head()["eventDigest"])
            self._append_locked(close, "session", epoch_id)
            return close

    def corpus_seal(self, *, source_snapshot_digest: str, adapter_versions: list[str], rows: list[dict[str, Any]], sealed_at: str) -> dict[str, Any]:
        digest(source_snapshot_digest, "sourceSnapshotDigest")
        timestamp(sealed_at, "sealedAt")
        if not isinstance(adapter_versions, list) or not adapter_versions or len(adapter_versions) > 32:
            malformed("adapterVersions must be a bounded non-empty array")
        versions = sorted({identifier(item, "adapterVersion") for item in adapter_versions})
        if not isinstance(rows, list) or not rows or len(rows) > MAX_ROWS:
            malformed("corpus rows must be a bounded non-empty array")
        normalized = []
        for index, row in enumerate(rows):
            value = exact_fields(row, {"rowId", "sessionDigest", "mode", "phase", "epochClass", "riskClass", "modelClass", "toolFamily", "measurementStatus", "outcome", "evidenceDigest", "exclusionReason"}, f"rows[{index}]")
            identifier(value["rowId"], "rowId")
            digest(value["sessionDigest"], "sessionDigest")
            digest(value["evidenceDigest"], "evidenceDigest")
            enum(value["epochClass"], EPOCH_CLASSES, "epochClass")
            enum(value["measurementStatus"], ("measured", "partially-measured", "unmeasured", "invalid", "superseded"), "measurementStatus")
            for key in ("mode", "phase", "riskClass", "modelClass", "toolFamily", "outcome"):
                identifier(value[key], key)
            if value["exclusionReason"] is not None:
                identifier(value["exclusionReason"], "exclusionReason")
            normalized.append(dict(value))
        normalized.sort(key=lambda row: row["rowId"])
        if len({row["rowId"] for row in normalized}) != len(normalized):
            malformed("corpus row identities must be unique")
        payload = {"sourceSnapshotDigest": source_snapshot_digest, "adapterVersions": versions, "rows": normalized, "sealedAt": sealed_at, "state": "sealed"}
        manifest = identified("cost-corpus-manifest", payload, "corpusId", "ccm")
        with self.store.locked():
            records = self._records_locked()
            same_source = [record for record in records if record.get("contractType") == "cost-corpus-manifest" and record.get("sourceSnapshotDigest") == source_snapshot_digest]
            if same_source and same_source[0]["corpusId"] != manifest["corpusId"]:
                refused("MBE-CORPUS-SEAL-MISMATCH", "sealed source cannot change under the same snapshot identity")
            if not same_source:
                self._append_locked(manifest, "x.org.bubbles.corpus", manifest["corpusId"])
            return manifest

    def corpus_evaluate(self, *, corpus_id: str, candidate_policy_digest: str, quality_report_digest: str, track: str, requested_reduction: bool, evaluated_at: str) -> dict[str, Any]:
        identifier(corpus_id, "corpusId")
        digest(candidate_policy_digest, "candidatePolicyDigest")
        digest(quality_report_digest, "qualityReportDigest")
        timestamp(evaluated_at, "evaluatedAt")
        boolean(requested_reduction, "requestedReduction")
        if track != "counterfactual":
            unsupported("MBE-LIVE-SAVINGS-UNSUPPORTED", "MBE-1 supports frozen-corpus counterfactual evaluation only")
        if requested_reduction:
            unsupported("MBE-LIVE-SAVINGS-UNSUPPORTED", "counterfactual replay cannot publish live savings")
        with self.store.locked():
            records = self._records_locked()
            corpus = self._find(records, "cost-corpus-manifest", "corpusId", corpus_id)
            included = sum(row["exclusionReason"] is None for row in corpus["rows"])
            excluded = len(corpus["rows"]) - included
            statuses = {status: sum(row["measurementStatus"] == status for row in corpus["rows"] if row["exclusionReason"] is None) for status in ("measured", "partially-measured", "unmeasured", "invalid", "superseded")}
            exclusions = sorted(({"reason": reason, "count": sum(row["exclusionReason"] == reason for row in corpus["rows"])} for reason in {row["exclusionReason"] for row in corpus["rows"] if row["exclusionReason"] is not None}), key=lambda row: row["reason"])
            coverage = {
                "basis": "frozen-corpus-included-rows",
                "includedRows": included,
                "measuredRows": statuses["measured"],
                "partiallyMeasuredRows": statuses["partially-measured"],
                "unmeasuredRows": statuses["unmeasured"],
                "invalidRows": statuses["invalid"],
                "supersededRows": statuses["superseded"],
                "coverageBasisPoints": 0 if included == 0 else statuses["measured"] * 10_000 // included,
            }
            payload = {"corpusId": corpus_id, "candidatePolicyDigest": candidate_policy_digest, "qualityReportDigest": quality_report_digest, "track": "counterfactual", "causalSavingsClaim": "unsupported", "reduction": None, "measurementCoverage": coverage, "includedRows": included, "excludedRows": excluded, "exclusions": exclusions, "evaluatedAt": evaluated_at}
            evaluation = identified("cost-corpus-evaluation", payload, "evaluationId", "cce")
            self._append_locked(evaluation, "x.org.bubbles.corpus", corpus_id)
            return evaluation


def error_envelope(exc: MbeError) -> dict[str, Any]:
    return {"code": exc.code, "contractType": "measured-budget-error", "errorClass": exc.error_class, "message": str(exc)[:512], "schemaVersion": VERSION}


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(description="IMP-055 MBE-1 reference domain engine")
    result.add_argument("command", choices=("usage-record", "dispatch-intent", "admission-fact", "budget-open", "snapshot", "reserve", "debit", "release", "hold", "correct", "close", "budget-settle", "retry-decide", "admission-evaluate", "permit-issue", "permit-get", "permit-consume", "launch-pending", "launch-confirm", "launch-deny", "launch-reconcile", "epoch-boundary", "epoch-open", "epoch-verify", "epoch-close", "corpus-seal", "corpus-evaluate"))
    result.add_argument("--store-root", required=True)
    result.add_argument("--input", required=True)
    return result


def execute_cli(args: argparse.Namespace) -> dict[str, Any]:
    raw, _authority = ecf.read_external(ecf.absolute_path(args.input, "input"), ecf.MAX_INPUT_BYTES, "MBE input")
    values = ecf.parse_json(raw, "MBE input", canonical=False)
    if not isinstance(values, dict):
        malformed("MBE input must be an object")
    runtime = MeasuredBudgetRuntime(args.store_root)
    method_name = args.command.replace("-", "_")
    if method_name == "usage_record":
        return runtime.usage_record(record=values)
    if method_name == "snapshot":
        method_name = "budget_snapshot"
    elif method_name in ("reserve", "debit", "release", "hold", "correct", "close"):
        method_name = "budget_" + method_name
    return getattr(runtime, method_name)(**values)


def main() -> int:
    try:
        result = execute_cli(parser().parse_args())
        sys.stdout.buffer.write(ecf.canonical_line(result))
        return 0
    except MbeError as exc:
        sys.stderr.buffer.write(ecf.canonical_line(error_envelope(exc)))
        return exc.exit_code
    except ecf.EcfError as exc:
        sys.stderr.buffer.write(ecf.canonical_line({"code": exc.code, "contractType": "measured-budget-error", "errorClass": exc.kind, "message": str(exc)[:512], "schemaVersion": VERSION}))
        return exc.exit_code
    except (OSError, TypeError, ValueError) as exc:
        sys.stderr.buffer.write(ecf.canonical_line(error_envelope(MbeError("MBE-INTERNAL", "internal", f"operation failed: {exc}", 2))))
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
