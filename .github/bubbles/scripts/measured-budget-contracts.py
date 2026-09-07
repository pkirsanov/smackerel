#!/usr/bin/env python3
"""Validate IMP-055's contract registry and project deterministic draft-07 schemas."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
REGISTRY = ROOT / "registry" / "measured-budget-runtime-contracts.json"
SCHEMA_DIR = ROOT / "schemas"
ID_PATTERN = "^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$"
DIGEST_PATTERN = "^sha256:[0-9a-f]{64}$"
TIME_PATTERN = "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{3}Z$"
DIMENSIONS = [
    "modelRequestCount", "inputTokens", "outputTokens", "cacheWriteTokens",
    "cacheReadTokens", "providerCredits", "monetaryMinorUnits",
    "subagentDispatches", "webCalls", "browserCalls", "toolCalls",
    "retainedResultBytes", "wallTimeMs", "retries", "concurrency",
]

ENUMS: dict[str, list[Any]] = {
    "sessionScope": ["exact", "prefix", "unscoped"],
    "measurementStatus": ["measured", "partially-measured", "unmeasured", "invalid", "superseded"],
    "verificationVerdict": ["valid", "invalid", "unresolved", "unsupported"],
    "rolloutPosture": ["shadow", "reference-enforce"],
    "budgetEventType": ["OPEN", "RESERVE", "DEBIT", "RELEASE", "HOLD", "CORRECT", "CLOSE"],
    "budgetState": ["active", "closed"],
    "reservationState": ["reserved", "held", "reconciled"],
    "factType": ["phase-relevance", "risk-tier", "model-class", "tool-grant", "action-authorization", "retry-eligibility"],
    "factState": ["verified", "invalid"],
    "admissionVerdict": ["permit-eligible", "refused"],
    "enforcementKind": ["repository-reference", "host-native"],
    "settlementState": ["debit", "release", "hold"],
    "retryFailureClass": ["none", "transient", "permanent", "unresolved"],
    "epochClass": ["planning", "implementation", "verification", "certification"],
    "boundaryKind": ["initial-host-checkpoint", "host-checkpoint", "new-session"],
    "epochVerdict": ["verified", "invalid"],
    "epochState": ["active", "closed"],
    "launchState": ["launch-pending", "launch-confirmed", "launch-denied", "launch-ambiguous"],
}


def canonical(value: Any) -> bytes:
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n").encode()


def load_registry() -> dict[str, Any]:
    value = json.loads(REGISTRY.read_text(encoding="utf-8"))
    if value.get("contractType") != "measured-budget-runtime-contract-registry" or value.get("schemaVersion") != 1:
        raise ValueError("registry identity is invalid")
    contracts = value.get("contracts")
    if not isinstance(contracts, list) or not contracts:
        raise ValueError("registry contracts must be a non-empty array")
    names = [row.get("name") for row in contracts]
    types = [row.get("contractType") for row in contracts]
    if len(names) != len(set(names)) or len(types) != len(set(types)):
        raise ValueError("contract names and contractType values must be unique")
    graph = value.get("graph")
    if not isinstance(graph, list) or len(graph) != len(set(graph)):
        raise ValueError("typed graph must be an ordered unique array")
    graph_types = set(graph)
    known_types = set(types)
    if not graph_types <= known_types:
        raise ValueError(f"graph references unknown contracts: {sorted(graph_types-known_types)}")
    edges = value.get("graphEdges")
    if not isinstance(edges, list) or not edges:
        raise ValueError("graphEdges must be a non-empty array")
    positions = {name: index for index, name in enumerate(graph)}
    normalized_edges = []
    for edge in edges:
        if not isinstance(edge, list) or len(edge) != 2 or any(node not in graph_types for node in edge):
            raise ValueError(f"invalid graph edge: {edge}")
        if positions[edge[0]] >= positions[edge[1]]:
            raise ValueError(f"forward or circular graph edge: {edge}")
        normalized_edges.append(tuple(edge))
    if len(normalized_edges) != len(set(normalized_edges)):
        raise ValueError("graphEdges must be unique")
    for row in contracts:
        if row.get("version") not in (1, 2) or not isinstance(row.get("fields"), dict) or not row["fields"]:
            raise ValueError(f"contract row is incomplete: {row.get('name')}")
        if row.get("projection") not in {"dispatch-admission.schema.json", "usage-adapter-v2.schema.json", "session-epoch.schema.json"}:
            raise ValueError(f"unknown projection: {row.get('projection')}")
        if len(row["fields"]) != len(set(row["fields"])):
            raise ValueError(f"duplicate field in {row['name']}")
    aliases = value.get("sensitiveKeyAliases")
    if not isinstance(aliases, list) or not aliases:
        raise ValueError("sensitiveKeyAliases must be a non-empty array")
    normalized_aliases = ["".join(character for character in alias.casefold() if character.isalnum()) for alias in aliases]
    if any(not alias for alias in normalized_aliases) or len(normalized_aliases) != len(set(normalized_aliases)):
        raise ValueError("sensitiveKeyAliases must be unique after normalization")
    return value


def contract_row(registry: dict[str, Any], contract_type: str) -> dict[str, Any]:
    rows = [row for row in registry["contracts"] if row["contractType"] == contract_type]
    if len(rows) != 1:
        raise ValueError(f"contract authority is unavailable: {contract_type}")
    return rows[0]


def validate_record(registry: dict[str, Any], record: Any) -> dict[str, Any]:
    if not isinstance(record, dict):
        raise ValueError("runtime record must be an object")
    contract_type = record.get("contractType")
    if not isinstance(contract_type, str):
        raise ValueError("runtime record contractType is missing")
    row = contract_row(registry, contract_type)
    expected = {"contractType", "schemaVersion", *row["fields"]}
    if set(record) != expected:
        raise ValueError(
            f"{contract_type} field mismatch; missing={sorted(expected-set(record))}, "
            f"unknown={sorted(set(record)-expected)}"
        )
    if record["schemaVersion"] != row["version"]:
        raise ValueError(f"{contract_type} schemaVersion mismatch")
    return record


def field_schema(kind: str) -> dict[str, Any]:
    if kind == "id": return {"$ref": "#/definitions/id"}
    if kind == "digest": return {"$ref": "#/definitions/digest"}
    if kind == "time": return {"$ref": "#/definitions/time"}
    if kind == "integer": return {"type": "integer", "minimum": 0, "maximum": 9007199254740991}
    if kind == "positiveInteger": return {"type": "integer", "minimum": 1, "maximum": 9007199254740991}
    if kind == "boolean": return {"type": "boolean"}
    if kind == "true": return {"const": True}
    if kind == "nullableId": return {"oneOf": [{"type": "null"}, {"$ref": "#/definitions/id"}]}
    if kind == "nullableDigest": return {"oneOf": [{"type": "null"}, {"$ref": "#/definitions/digest"}]}
    if kind in ENUMS: return {"enum": ENUMS[kind]}
    if kind in {"integerArray", "idArray"}:
        item = {"type": "integer", "minimum": 1} if kind == "integerArray" else {"$ref": "#/definitions/id"}
        return {"type": "array", "maxItems": 256, "items": item}
    if kind in {"amounts", "amountsEmpty"}:
        result: dict[str, Any] = {"type": "array", "maxItems": 256, "items": {"$ref": "#/definitions/amount"}}
        if kind == "amounts": result["minItems"] = 1
        return result
    if kind == "capabilities": return {"type": "array", "minItems": 15, "maxItems": 15, "items": {"$ref": "#/definitions/capability"}}
    if kind == "dimensionPolicies": return {"type": "array", "minItems": 15, "maxItems": 15, "items": {"$ref": "#/definitions/dimensionPolicy"}}
    if kind == "snapshotDimensions": return {"type": "array", "minItems": 15, "maxItems": 15, "items": {"$ref": "#/definitions/snapshotDimension"}}
    if kind == "capacityPlan": return {"$ref": "#/definitions/capacityPlan"}
    if kind == "factRefs": return {"type": "array", "minItems": 6, "maxItems": 6, "items": {"$ref": "#/definitions/factRef"}}
    if kind == "partitions": return {"type": "array", "minItems": 1, "maxItems": 256, "items": {"$ref": "#/definitions/partition"}}
    raise ValueError(f"unknown field kind: {kind}")


def projection(registry: dict[str, Any], filename: str) -> dict[str, Any]:
    rows = sorted((row for row in registry["contracts"] if row["projection"] == filename), key=lambda row: row["name"])
    definitions: dict[str, Any] = {
        "id": {"type": "string", "pattern": ID_PATTERN},
        "digest": {"type": "string", "pattern": DIGEST_PATTERN},
        "time": {"type": "string", "pattern": TIME_PATTERN},
        "dimension": {"enum": DIMENSIONS},
        "amount": {"type": "object", "additionalProperties": False, "required": ["dimension", "amount", "unit", "currency", "scale"], "properties": {"dimension": {"$ref": "#/definitions/dimension"}, "amount": {"type": "integer", "minimum": 0, "maximum": 9007199254740991}, "unit": {"type": "string", "minLength": 1, "maxLength": 32}, "currency": {"oneOf": [{"type": "null"}, {"type": "string", "pattern": "^[A-Z]{3}$"}]}, "scale": {"oneOf": [{"type": "null"}, {"type": "integer", "minimum": 0, "maximum": 9}]} }},
        "capability": {"type": "object", "additionalProperties": False, "required": ["dimension", "mode", "preDispatchBound", "postDispatchActual"], "properties": {"dimension": {"$ref": "#/definitions/dimension"}, "mode": {"enum": ["native", "trusted-derived", "bounded-only", "unsupported"]}, "preDispatchBound": {"type": "boolean"}, "postDispatchActual": {"type": "boolean"}}},
        "dimensionPolicy": {"type": "object", "additionalProperties": False, "required": ["dimension", "state", "limit", "unit", "currency", "scale"], "properties": {"dimension": {"$ref": "#/definitions/dimension"}, "state": {"enum": ["configured", "unconfigured"]}, "limit": {"oneOf": [{"type": "null"}, {"type": "integer", "minimum": 0, "maximum": 9007199254740991}]}, "unit": {"type": "string", "minLength": 1, "maxLength": 32}, "currency": {"oneOf": [{"type": "null"}, {"type": "string", "pattern": "^[A-Z]{3}$"}]}, "scale": {"oneOf": [{"type": "null"}, {"type": "integer", "minimum": 0, "maximum": 9}]}}},
        "snapshotDimension": {"type": "object", "additionalProperties": False, "required": ["dimension", "state", "unit", "currency", "scale", "limit", "reserved", "debited", "held", "corrected"], "properties": {"dimension": {"$ref": "#/definitions/dimension"}, "state": {"enum": ["measured", "unmeasured"]}, "unit": {"type": "string"}, "currency": {"oneOf": [{"type": "null"}, {"type": "string", "pattern": "^[A-Z]{3}$"}]}, "scale": {"oneOf": [{"type": "null"}, {"type": "integer", "minimum": 0, "maximum": 9}]}, "limit": {"oneOf": [{"type": "null"}, {"type": "integer", "minimum": 0}]}, "reserved": {"type": "integer", "minimum": 0}, "debited": {"type": "integer", "minimum": 0}, "held": {"type": "integer", "minimum": 0}, "corrected": {"type": "integer", "minimum": 0}}},
        "capacityPlan": {"type": "object", "additionalProperties": False, "required": ["attemptCapacity", "projectedEvents", "projectedObjects", "projectedObjectBytes"], "properties": {"attemptCapacity": {"type": "integer", "minimum": 1}, "projectedEvents": {"type": "integer", "minimum": 1}, "projectedObjects": {"type": "integer", "minimum": 1}, "projectedObjectBytes": {"type": "integer", "minimum": 1}}},
        "factRef": {"type": "object", "additionalProperties": False, "required": ["factId", "factType", "sourceRecordId", "sourceDigest", "valueDigest", "issuedAt", "expiresAt"], "properties": {"factId": {"$ref": "#/definitions/id"}, "factType": {"enum": ENUMS["factType"]}, "sourceRecordId": {"$ref": "#/definitions/id"}, "sourceDigest": {"$ref": "#/definitions/digest"}, "valueDigest": {"$ref": "#/definitions/digest"}, "issuedAt": {"$ref": "#/definitions/time"}, "expiresAt": {"$ref": "#/definitions/time"}}},
        "partition": {"type": "object", "additionalProperties": False, "required": ["dimension", "unit", "currency", "scale", "reserved", "debit", "release", "hold"], "properties": {"dimension": {"$ref": "#/definitions/dimension"}, "unit": {"type": "string"}, "currency": {"oneOf": [{"type": "null"}, {"type": "string", "pattern": "^[A-Z]{3}$"}]}, "scale": {"oneOf": [{"type": "null"}, {"type": "integer", "minimum": 0, "maximum": 9}]}, "reserved": {"type": "integer", "minimum": 0}, "debit": {"type": "integer", "minimum": 0}, "release": {"type": "integer", "minimum": 0}, "hold": {"type": "integer", "minimum": 0}}},
    }
    for row in rows:
        properties = {"contractType": {"const": row["contractType"]}, "schemaVersion": {"const": row["version"]}}
        properties.update({name: field_schema(kind) for name, kind in row["fields"].items()})
        definitions[row["name"]] = {"type": "object", "additionalProperties": False, "required": list(properties), "properties": properties}
    return {"$schema": registry["draft"], "$id": f"https://github.com/pkirsanov/bubbles/schemas/{filename}", "title": f"Generated IMP-055 contracts: {filename}", "x-generated-from": "bubbles/registry/measured-budget-runtime-contracts.json", "oneOf": [{"$ref": f"#/definitions/{row['name']}"} for row in rows], "definitions": definitions}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("validate", "schema"))
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    try:
        registry = load_registry()
        if args.operation == "validate":
            print(f"measured-budget-contracts: OK ({len(registry['contracts'])} contracts, {len(registry['graph'])} graph nodes)")
            return 0
        if args.check == args.write:
            raise ValueError("schema requires exactly one of --check or --write")
        drift = []
        for filename in ("dispatch-admission.schema.json", "usage-adapter-v2.schema.json", "session-epoch.schema.json"):
            expected = canonical(projection(registry, filename))
            path = SCHEMA_DIR / filename
            if args.write:
                path.write_bytes(expected)
            elif not path.exists() or path.read_bytes() != expected:
                drift.append(filename)
        if drift:
            print("measured-budget-contracts: schema drift: " + ", ".join(drift), file=sys.stderr)
            return 1
        print("measured-budget-contracts: OK (deterministic schema projections match)")
        return 0
    except (OSError, UnicodeError, json.JSONDecodeError, ValueError) as exc:
        print(f"measured-budget-contracts: ERROR: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
