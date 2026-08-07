#!/usr/bin/env python3
"""Deterministic repository-local index for evidence-backed experience recall."""

from __future__ import annotations

import argparse
import collections
import datetime as dt
import hashlib
import json
import os
import re
import stat
import sys
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any, Dict, Iterable, List, Optional, Sequence, Tuple


SCHEMA_VERSION = 1
PROVIDER = "local-lexical"
PROVIDER_VERSION = "1"
EXTRACTOR = "experience-recall-index"
EXTRACTOR_VERSION = "1"
RUNTIME_PARTS = (".specify", "runtime", "experience-recall")
INDEX_NAME = "index.jsonl"
STATUS_NAME = "status.json"
MAX_SOURCE_BYTES = 2 * 1024 * 1024

KINDS = {"compacted-result", "lesson", "owner-decision", "finding", "outcome"}
LIFECYCLE_STATES = ("admitted", "superseded", "expired", "deleted")
TRUST_BY_KIND = {
    "compacted-result": {"executed-result", "historical-result"},
    "lesson": {"reviewed-lesson", "anchored-lesson"},
    "owner-decision": {"owner-approved"},
    "finding": {"historical-finding"},
    "outcome": {"historical-outcome"},
}
OUTCOMES = {"completed_owned", "completed_diagnostic", "route_required", "blocked"}
COMMAND_SEMANTICS = {
    "explicit-repository-root": (
        {"established", "confirmed", "switched"},
        {"repository-root"},
    ),
    "concrete-target": (
        {"established", "confirmed", "switched"},
        {"absolute-target", "relative-target"},
    ),
    "resolved-natural-language": (
        {"established", "confirmed", "switched"},
        {"natural-language"},
    ),
    "durable-work-boundary": ({"continued"}, {"inherited-boundary"}),
    "single-eligible-repository": ({"established"}, {"sole-eligible-repository"}),
}
PACKET_FIELDS = {
    "sessionId",
    "decisionId",
    "controlRevision",
    "controlPathDigest",
    "authority",
    "transition",
    "scopeKind",
    "scopeId",
    "targetKind",
    "pathVisibility",
    "actionable",
}
DIGEST_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
ALIAS_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
SESSION_RE = re.compile(r"^[A-Za-z0-9._:-]+$")
LESSON_ID_RE = re.compile(r"^lesson-[0-9a-f]{64}$")
LESSON_META_RE = re.compile(
    r"\s*<!--\s*bubbles-lesson-meta:(\{.*\})\s*-->\s*$"
)
LESSON_LINE_RE = re.compile(
    r"^-\s*problem:\s*(.*?);\s*root cause:\s*(.*?);\s*fix:\s*(.*?);\s*applies when:\s*(.*?)\s*$",
    re.IGNORECASE,
)
IMPROVEMENT_RE = re.compile(r"^IMP-[0-9]{3}-.+\.md$")
IMPROVEMENT_ID_RE = re.compile(r"^(IMP-[0-9]{3})-")
IMPROVEMENT_STATUS_TOKEN_RE = re.compile(
    r"^\*\*Status:\*\*\s*(PROPOSED|ACCEPTED|IN PROGRESS|APPLIED|REJECTED|SUPERSEDED)\b"
)
IMPROVEMENT_STATUS_RE = re.compile(
    r"^\*\*Status:\*\*\s*(PROPOSED|ACCEPTED|IN PROGRESS|APPLIED|REJECTED|SUPERSEDED)\s+([0-9]{4}-[0-9]{2}-[0-9]{2})\b"
)
SCOPE_HEADING_RE = re.compile(
    r"^###\s+(SCOPE-[A-Za-z0-9.-]+)\s+(?:-|\u2014)\s+(.+?)(?:\s+\([^)]*\))?\s*$"
)
DECISION_RE = re.compile(r"^\*\*Decision:\*\*\s*(\S.*)$")
TOKEN_RE = re.compile(r"[a-z0-9][a-z0-9._-]*")
TRANSCRIPT_TOKENS = {
    "chat-log",
    "chatlog",
    "conversation-log",
    "screenshot",
    "scrollback",
    "terminal-log",
    "transcript",
}
TRANSCRIPT_SUFFIXES = {".bmp", ".gif", ".jpeg", ".jpg", ".png", ".webp"}


class RecallError(Exception):
    """Expected refusal with a stable reason code."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


def canonical_json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=True, separators=(",", ":"), sort_keys=True)


def digest_bytes(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def digest_file(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as handle:
        while True:
            chunk = handle.read(65536)
            if not chunk:
                break
            hasher.update(chunk)
    return "sha256:" + hasher.hexdigest()


def normalized_text(value: Any, limit: int) -> str:
    if not isinstance(value, str):
        return ""
    normalized = " ".join(value.split())
    return normalized[:limit]


def unique_strings(values: Iterable[Any], limit: int, item_limit: int) -> List[str]:
    normalized = {
        normalized_text(value, item_limit)
        for value in values
        if normalized_text(value, item_limit)
    }
    return sorted(normalized)[:limit]


def valid_timestamp(value: Any) -> bool:
    if not isinstance(value, str) or not value:
        return False
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return False
    return parsed.tzinfo is not None


def mtime_timestamp(path: Path) -> str:
    observed = dt.datetime.fromtimestamp(path.stat().st_mtime, tz=dt.timezone.utc)
    return observed.replace(microsecond=0).strftime("%Y-%m-%dT%H:%M:%SZ")


def current_timestamp() -> str:
    return dt.datetime.now(tz=dt.timezone.utc).replace(microsecond=0).strftime(
        "%Y-%m-%dT%H:%M:%SZ"
    )


def read_text(path: Path) -> str:
    if path.stat().st_size > MAX_SOURCE_BYTES:
        raise RecallError("source-too-large", f"source exceeds {MAX_SOURCE_BYTES} bytes")
    try:
        return path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as error:
        raise RecallError("unreadable-source", str(error)) from error


class RepositoryPaths:
    def __init__(self, root: str) -> None:
        candidate = Path(root)
        if candidate.is_symlink():
            raise RecallError("invalid-repository-root", "repository root must not be a symlink")
        if not candidate.is_absolute() or not candidate.is_dir():
            raise RecallError("invalid-repository-root", "repository root must be an absolute directory")
        self.root = candidate.resolve(strict=True)

    def normalize_relative(self, value: str) -> str:
        if not isinstance(value, str) or not value or "\\" in value:
            raise RecallError("unsafe-anchor", "anchor path must be non-empty repository-relative POSIX text")
        pure = PurePosixPath(value)
        if pure.is_absolute() or any(part in {"", ".", ".."} for part in pure.parts):
            raise RecallError("unsafe-anchor", "anchor path is absolute or contains an unsafe segment")
        normalized = pure.as_posix()
        if len(normalized) > 1024:
            raise RecallError("unsafe-anchor", "anchor path exceeds the contract bound")
        return normalized

    def relative_from_pointer(self, value: Any) -> str:
        if not isinstance(value, str) or not value:
            raise RecallError("missing-anchor", "rawPointer is missing")
        pointer = Path(value)
        if any(part == ".." for part in pointer.parts):
            raise RecallError("unsafe-anchor", "rawPointer contains a parent traversal")
        if pointer.is_absolute():
            try:
                relative = pointer.relative_to(self.root)
            except ValueError as error:
                raise RecallError("unsafe-anchor", "rawPointer is outside the repository") from error
            return self.normalize_relative(relative.as_posix())
        return self.normalize_relative(value)

    def resolve_file(self, relative_path: str) -> Path:
        normalized = self.normalize_relative(relative_path)
        cursor = self.root
        for part in PurePosixPath(normalized).parts:
            cursor = cursor / part
            try:
                metadata = cursor.lstat()
            except FileNotFoundError as error:
                raise RecallError("missing-anchor", f"anchor source is missing: {normalized}") from error
            if stat.S_ISLNK(metadata.st_mode):
                raise RecallError("unsafe-anchor", f"anchor traverses a symlink: {normalized}")
        if not stat.S_ISREG(cursor.stat().st_mode):
            raise RecallError("missing-anchor", f"anchor is not a regular file: {normalized}")
        try:
            cursor.resolve(strict=True).relative_to(self.root)
        except ValueError as error:
            raise RecallError("unsafe-anchor", f"anchor resolves outside the repository: {normalized}") from error
        return cursor

    def runtime_dir(self, create: bool) -> Path:
        cursor = self.root
        for part in RUNTIME_PARTS:
            cursor = cursor / part
            if cursor.is_symlink():
                raise RecallError("unsafe-derived-state", "derived-state path is not a contained directory")
            if cursor.exists():
                if not cursor.is_dir():
                    raise RecallError("unsafe-derived-state", "derived-state path is not a contained directory")
            elif create:
                cursor.mkdir(mode=0o700)
            else:
                raise RecallError("index-missing", "experience recall index has not been synchronized")
        return cursor


def transcript_like(relative_path: str) -> bool:
    path = PurePosixPath(relative_path)
    if path.suffix.lower() in TRANSCRIPT_SUFFIXES:
        return True
    for part in path.parts:
        lowered = part.lower()
        stem = lowered.rsplit(".", 1)[0]
        if stem in TRANSCRIPT_TOKENS:
            return True
        tokens = {token for token in re.split(r"[^a-z0-9]+", stem) if token}
        if "transcript" in tokens or "scrollback" in tokens or "screenshot" in tokens:
            return True
    return False


def binding_is_valid(record: Dict[str, Any], paths: RepositoryPaths, alias: str) -> bool:
    if record.get("repositoryRoot") != str(paths.root) or record.get("repositoryAlias") != alias:
        return False
    resolution = record.get("repositoryResolution")
    if not isinstance(resolution, dict) or set(resolution) != PACKET_FIELDS:
        return False
    if not isinstance(resolution.get("sessionId"), str) or not SESSION_RE.fullmatch(
        resolution["sessionId"]
    ):
        return False
    if not isinstance(resolution.get("controlRevision"), int) or resolution["controlRevision"] < 1:
        return False
    if not isinstance(resolution.get("controlPathDigest"), str) or not DIGEST_RE.fullmatch(
        resolution["controlPathDigest"]
    ):
        return False
    if resolution.get("pathVisibility") != "local" or resolution.get("actionable") is not True:
        return False
    decision_id = f"rb:{resolution['sessionId']}:{resolution['controlRevision']}"
    scope_kind = resolution.get("scopeKind")
    if scope_kind == "goal-node":
        scope_id = resolution.get("scopeId")
        return bool(
            isinstance(scope_id, str)
            and scope_id
            and resolution.get("authority") == "scoped-scenario-node"
            and resolution.get("transition") == "scoped-override"
            and resolution.get("targetKind") == "goal-node"
            and resolution.get("decisionId") == f"{decision_id}:node:{scope_id}"
        )
    if scope_kind != "command" or resolution.get("scopeId") is not None:
        return False
    authority = resolution.get("authority")
    semantics = COMMAND_SEMANTICS.get(authority)
    return bool(
        semantics
        and resolution.get("transition") in semantics[0]
        and resolution.get("targetKind") in semantics[1]
        and resolution.get("decisionId") == decision_id
    )


def evidence_refs_valid(value: Any) -> bool:
    if isinstance(value, str):
        return bool(value.strip())
    return bool(
        isinstance(value, list)
        and value
        and all(isinstance(item, str) and item.strip() for item in value)
    )


def structured_envelope(text: str) -> Optional[Dict[str, Any]]:
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError:
        parsed = None
    if isinstance(parsed, dict):
        return parsed
    if "## RESULT-ENVELOPE" not in text:
        return None
    for block in re.findall(r"```(?:json)?\s*(.*?)```", text, flags=re.DOTALL):
        try:
            parsed = json.loads(block)
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, dict):
            return parsed
    raise RecallError(
        "result-envelope-unsupported",
        "RESULT-ENVELOPE source is not supported structured JSON",
    )


def source_anchor(
    relative_path: str,
    selector: str,
    content_digest: str,
    observed_at: str,
) -> Dict[str, Any]:
    return {
        "relativePath": relative_path,
        "selector": normalized_text(selector, 512),
        "contentDigest": content_digest,
        "observedAt": observed_at,
    }


def stable_record_id(repository_alias: str, kind: str, identity: str) -> str:
    material = "\x00".join((repository_alias, kind, identity)).encode("utf-8")
    return "recall-" + hashlib.sha256(material).hexdigest()


def make_record(
    *,
    repository_alias: str,
    kind: str,
    identity: str,
    summary: str,
    identifiers: Iterable[Any],
    phrases: Iterable[Any],
    tags: Iterable[Any],
    anchor: Dict[str, Any],
    source_trust: str,
    admitted_at: str,
    spec_ref: Optional[str] = None,
    scope_ref: Optional[str] = None,
    scenario_refs: Iterable[Any] = (),
) -> Dict[str, Any]:
    return {
        "contractType": "record",
        "schemaVersion": SCHEMA_VERSION,
        "recordId": stable_record_id(repository_alias, kind, identity),
        "kind": kind,
        "summary": normalized_text(summary, 2000),
        "searchableFields": {
            "identifiers": unique_strings(identifiers, 100, 256),
            "phrases": unique_strings(phrases, 100, 512),
            "tags": unique_strings(tags, 100, 128),
        },
        "repositoryAlias": repository_alias,
        "specRef": normalized_text(spec_ref, 512) if spec_ref else None,
        "scopeRef": normalized_text(scope_ref, 512) if scope_ref else None,
        "scenarioRefs": unique_strings(scenario_refs, 100, 128),
        "sourceAnchor": anchor,
        "sourceTrust": source_trust,
        "recallAuthority": "advisory",
        "freshness": {
            "contractType": "freshness",
            "state": "fresh",
            "sourceDigest": anchor["contentDigest"],
            "checkedAt": anchor["observedAt"],
            "reason": None,
        },
        "lifecycle": {
            "contractType": "lifecycle",
            "state": "admitted",
            "admittedAt": admitted_at,
            "supersededAt": None,
            "expiredAt": None,
            "deletedAt": None,
        },
        "provenance": {
            "extractor": EXTRACTOR,
            "extractorVersion": EXTRACTOR_VERSION,
            "provider": PROVIDER,
            "providerVersion": PROVIDER_VERSION,
            "derivedAt": admitted_at,
        },
    }


def validate_record(record: Dict[str, Any]) -> None:
    required = {
        "contractType",
        "schemaVersion",
        "recordId",
        "kind",
        "summary",
        "searchableFields",
        "repositoryAlias",
        "specRef",
        "scopeRef",
        "scenarioRefs",
        "sourceAnchor",
        "sourceTrust",
        "recallAuthority",
        "freshness",
        "lifecycle",
        "provenance",
    }
    if set(record) != required:
        raise RecallError("invalid-record", "record fields do not match schema version 1")
    kind = record.get("kind")
    if record.get("contractType") != "record" or record.get("schemaVersion") != 1 or kind not in KINDS:
        raise RecallError("invalid-record", "record discriminator is invalid")
    if record.get("sourceTrust") not in TRUST_BY_KIND[kind] or record.get("recallAuthority") != "advisory":
        raise RecallError("invalid-record", "record trust or authority is invalid")
    if not isinstance(record.get("recordId"), str) or len(record["recordId"]) > 128:
        raise RecallError("invalid-record", "record id is invalid")
    if not isinstance(record.get("summary"), str) or not record["summary"] or len(record["summary"]) > 2000:
        raise RecallError("invalid-record", "record summary is invalid")
    alias = record.get("repositoryAlias")
    if not isinstance(alias, str) or len(alias) > 128 or not ALIAS_RE.fullmatch(alias):
        raise RecallError("invalid-record", "repository alias is invalid")
    for ref_name in ("specRef", "scopeRef"):
        ref = record.get(ref_name)
        if ref is not None and (not isinstance(ref, str) or not ref or len(ref) > 512):
            raise RecallError("invalid-record", f"{ref_name} is invalid")
    scenarios = record.get("scenarioRefs")
    if not isinstance(scenarios, list) or len(scenarios) > 100 or len(scenarios) != len(set(scenarios)):
        raise RecallError("invalid-record", "scenario references are invalid")
    searchable = record.get("searchableFields")
    if not isinstance(searchable, dict) or set(searchable) != {"identifiers", "phrases", "tags"}:
        raise RecallError("invalid-record", "searchable fields are invalid")
    for key, item_limit in (("identifiers", 256), ("phrases", 512), ("tags", 128)):
        values = searchable.get(key)
        if not isinstance(values, list) or len(values) > 100 or len(values) != len(set(values)):
            raise RecallError("invalid-record", f"searchable {key} are invalid")
        if any(not isinstance(value, str) or not value or len(value) > item_limit for value in values):
            raise RecallError("invalid-record", f"searchable {key} contain an invalid value")
    anchor = record.get("sourceAnchor")
    if not isinstance(anchor, dict) or set(anchor) != {
        "relativePath",
        "selector",
        "contentDigest",
        "observedAt",
    }:
        raise RecallError("invalid-record", "source anchor is invalid")
    if not isinstance(anchor.get("selector"), str) or not anchor["selector"] or len(anchor["selector"]) > 512:
        raise RecallError("invalid-record", "source selector is invalid")
    if not isinstance(anchor.get("contentDigest"), str) or not DIGEST_RE.fullmatch(anchor["contentDigest"]):
        raise RecallError("invalid-record", "source digest is invalid")
    if not valid_timestamp(anchor.get("observedAt")):
        raise RecallError("invalid-record", "source observation time is invalid")
    freshness = record.get("freshness")
    if not isinstance(freshness, dict) or freshness != {
        "contractType": "freshness",
        "state": "fresh",
        "sourceDigest": anchor["contentDigest"],
        "checkedAt": anchor["observedAt"],
        "reason": None,
    }:
        raise RecallError("invalid-record", "freshness metadata is invalid")
    lifecycle = record.get("lifecycle")
    if not isinstance(lifecycle, dict) or lifecycle.get("contractType") != "lifecycle":
        raise RecallError("invalid-record", "lifecycle metadata is invalid")
    if set(lifecycle) != {"contractType", "state", "admittedAt", "supersededAt", "expiredAt", "deletedAt"}:
        raise RecallError("invalid-record", "lifecycle fields are invalid")
    if lifecycle.get("state") != "admitted" or not valid_timestamp(lifecycle.get("admittedAt")):
        raise RecallError("invalid-record", "lifecycle state is invalid")
    if any(lifecycle.get(key) is not None for key in ("supersededAt", "expiredAt", "deletedAt")):
        raise RecallError("invalid-record", "admitted lifecycle has a transition timestamp")
    provenance = record.get("provenance")
    if not isinstance(provenance, dict) or set(provenance) != {
        "extractor",
        "extractorVersion",
        "provider",
        "providerVersion",
        "derivedAt",
    }:
        raise RecallError("invalid-record", "provenance fields are invalid")
    if provenance.get("extractor") != EXTRACTOR or provenance.get("provider") != PROVIDER:
        raise RecallError("invalid-record", "provenance producer is invalid")
    if not valid_timestamp(provenance.get("derivedAt")):
        raise RecallError("invalid-record", "provenance time is invalid")


class IndexBuilder:
    def __init__(self, paths: RepositoryPaths, repository_alias: str) -> None:
        if len(repository_alias) > 128 or not ALIAS_RE.fullmatch(repository_alias):
            raise RecallError("invalid-repository-alias", "repository alias does not satisfy the schema")
        self.paths = paths
        self.repository_alias = repository_alias
        self.records: Dict[str, Dict[str, Any]] = {}
        self.exclusions: collections.Counter[str] = collections.Counter()
        self.candidate_count = 0

    def exclude(self, reason: str) -> None:
        self.exclusions[reason] += 1

    def add(self, record: Dict[str, Any]) -> None:
        validate_record(record)
        existing = self.records.get(record["recordId"])
        if existing is None:
            self.records[record["recordId"]] = record
        elif existing != record:
            self.exclude("duplicate-record-id")

    def build(self) -> Tuple[List[Dict[str, Any]], Dict[str, Any]]:
        self.add_compacted_history()
        self.add_lessons()
        self.add_improvement_decisions()
        records = [self.records[key] for key in sorted(self.records)]
        status = {
            "contractType": "status",
            "schemaVersion": SCHEMA_VERSION,
            "adapter": PROVIDER,
            "providerVersion": PROVIDER_VERSION,
            "repositoryAlias": self.repository_alias,
            "state": "ready",
            "indexPath": "/".join(RUNTIME_PARTS + (INDEX_NAME,)),
            "recordCount": len(records),
            "candidateCount": self.candidate_count,
            "countsByKind": counts_by_kind(records),
            "lifecycleCounts": counts_by_lifecycle(records),
            "excludedCount": sum(self.exclusions.values()),
            "exclusions": {key: self.exclusions[key] for key in sorted(self.exclusions)},
            "builtAt": max(
                (record["provenance"]["derivedAt"] for record in records),
                default=None,
            ),
        }
        return records, status

    def add_compacted_history(self) -> None:
        relative = ".specify/memory/bubbles.session.json"
        try:
            session_path = self.paths.resolve_file(relative)
        except RecallError as error:
            if error.code == "missing-anchor":
                return
            raise
        try:
            session = json.loads(read_text(session_path))
        except json.JSONDecodeError:
            self.exclude("malformed-session")
            return
        history = session.get("compactedHistory", []) if isinstance(session, dict) else []
        if not isinstance(history, list):
            self.exclude("malformed-session")
            return
        for compacted in history:
            self.candidate_count += 1
            try:
                self.add_compacted_record(compacted)
            except RecallError as error:
                self.exclude(error.code)

    def add_compacted_record(self, compacted: Any) -> None:
        if not isinstance(compacted, dict) or not binding_is_valid(
            compacted, self.paths, self.repository_alias
        ):
            raise RecallError("invalid-repository-packet", "compacted result is not repository-bound")
        if not evidence_refs_valid(compacted.get("evidenceRefs")):
            raise RecallError("missing-evidence-refs", "compacted result has no evidence references")
        relative = self.paths.relative_from_pointer(compacted.get("rawPointer"))
        if transcript_like(relative):
            raise RecallError("transcript-like-input", "rawPointer names an excluded input family")
        raw_path = self.paths.resolve_file(relative)
        raw_digest = digest_file(raw_path)
        declared_digest = compacted.get("sourceDigest")
        if declared_digest is not None and declared_digest != raw_digest:
            raise RecallError("digest-mismatch", "compacted source digest does not match")
        observed_at = compacted.get("timestamp")
        if not valid_timestamp(observed_at):
            observed_at = mtime_timestamp(raw_path)
        raw_text = read_text(raw_path)
        envelope = structured_envelope(raw_text)
        if envelope is not None and "evidenceRefs" in envelope and not evidence_refs_valid(
            envelope.get("evidenceRefs")
        ):
            raise RecallError("missing-evidence-refs", "source envelope evidence references are invalid")
        agent = normalized_text(compacted.get("agent"), 128)
        outcome = normalized_text(compacted.get("outcome"), 64)
        if not agent or outcome not in OUTCOMES:
            raise RecallError("malformed-compacted-result", "compacted result lacks agent or outcome")
        if envelope is not None:
            if envelope.get("agent") not in (None, agent) or envelope.get("outcome") not in (None, outcome):
                raise RecallError("source-record-mismatch", "source envelope differs from compacted metadata")
        anchor = source_anchor(relative, "result-envelope", raw_digest, observed_at)
        spec_ref = normalized_text(compacted.get("featureDir"), 512) or None
        scope_ref = self.scope_value(compacted.get("scopeIds"))
        trust = "executed-result" if outcome == "completed_owned" else "historical-result"
        compacted_summary = f"{agent} returned {outcome}"
        if scope_ref:
            compacted_summary += f" for {scope_ref}"
        blocked_reason = normalized_text(compacted.get("blockedReason"), 512)
        phrases = [compacted_summary, blocked_reason]
        self.add(
            make_record(
                repository_alias=self.repository_alias,
                kind="compacted-result",
                identity=f"{relative}\x00result-envelope",
                summary=compacted_summary,
                identifiers=[agent, outcome, spec_ref, scope_ref],
                phrases=phrases,
                tags=["compacted-result", outcome],
                anchor=anchor,
                source_trust=trust,
                admitted_at=observed_at,
                spec_ref=spec_ref,
                scope_ref=scope_ref,
            )
        )
        envelope_summary = normalized_text(
            envelope.get("summary") if isinstance(envelope, dict) else "", 2000
        )
        outcome_summary = envelope_summary or compacted_summary
        self.add(
            make_record(
                repository_alias=self.repository_alias,
                kind="outcome",
                identity=f"{relative}\x00outcome",
                summary=outcome_summary,
                identifiers=[agent, outcome, spec_ref, scope_ref],
                phrases=[outcome_summary, blocked_reason],
                tags=["outcome", outcome],
                anchor=source_anchor(relative, "result-envelope:outcome", raw_digest, observed_at),
                source_trust="historical-outcome",
                admitted_at=observed_at,
                spec_ref=spec_ref,
                scope_ref=scope_ref,
            )
        )
        if isinstance(envelope, dict):
            self.add_findings(envelope, relative, raw_digest, observed_at, spec_ref, scope_ref)

    @staticmethod
    def scope_value(value: Any) -> Optional[str]:
        if isinstance(value, str):
            return normalized_text(value, 512) or None
        if isinstance(value, list):
            joined = ",".join(str(item) for item in value if isinstance(item, str))
            return normalized_text(joined, 512) or None
        return None

    def add_findings(
        self,
        envelope: Dict[str, Any],
        relative: str,
        raw_digest: str,
        observed_at: str,
        spec_ref: Optional[str],
        default_scope_ref: Optional[str],
    ) -> None:
        findings = envelope.get("findings", [])
        if not isinstance(findings, list):
            return
        addressed = set(envelope.get("addressedFindings", [])) if isinstance(
            envelope.get("addressedFindings"), list
        ) else set()
        unresolved = set(envelope.get("unresolvedFindings", [])) if isinstance(
            envelope.get("unresolvedFindings"), list
        ) else set()
        for finding in findings:
            if not isinstance(finding, dict):
                continue
            finding_id = normalized_text(finding.get("id"), 128)
            summary = normalized_text(finding.get("summary"), 2000)
            owner = normalized_text(finding.get("owner"), 128)
            severity = normalized_text(finding.get("severity"), 32)
            if not finding_id or not summary or not owner or severity not in {"info", "warn", "blocker"}:
                continue
            disposition = "addressed" if finding_id in addressed else "unresolved" if finding_id in unresolved else "reported"
            finding_scope = normalized_text(finding.get("scopeRef"), 512) or default_scope_ref
            scenario = normalized_text(finding.get("scenarioRef"), 128)
            selector = f"result-envelope:finding:{finding_id}"[:512]
            self.add(
                make_record(
                    repository_alias=self.repository_alias,
                    kind="finding",
                    identity=f"{relative}\x00finding\x00{finding_id}",
                    summary=summary,
                    identifiers=[finding_id, owner, severity, finding_scope, scenario],
                    phrases=[summary],
                    tags=["finding", severity, disposition],
                    anchor=source_anchor(relative, selector, raw_digest, observed_at),
                    source_trust="historical-finding",
                    admitted_at=observed_at,
                    spec_ref=spec_ref,
                    scope_ref=finding_scope,
                    scenario_refs=[scenario],
                )
            )

    def add_lessons(self) -> None:
        relative = ".specify/memory/lessons.md"
        try:
            lessons_path = self.paths.resolve_file(relative)
        except RecallError as error:
            if error.code == "missing-anchor":
                return
            raise
        for line in read_text(lessons_path).splitlines():
            if not re.match(r"^-\s*problem:", line, flags=re.IGNORECASE):
                continue
            self.candidate_count += 1
            try:
                self.add_lesson(line)
            except RecallError as error:
                self.exclude(error.code)

    def add_lesson(self, line: str) -> None:
        marker = LESSON_META_RE.search(line)
        if marker is None:
            raise RecallError("unanchored-lesson", "legacy lesson has no recall anchor")
        visible = line[: marker.start()].rstrip()
        match = LESSON_LINE_RE.fullmatch(visible)
        if match is None:
            raise RecallError("malformed-lesson", "lesson fields do not match the supported line format")
        try:
            metadata = json.loads(marker.group(1))
        except json.JSONDecodeError as error:
            raise RecallError("malformed-lesson-metadata", str(error)) from error
        if not isinstance(metadata, dict) or metadata.get("schemaVersion") != 1:
            raise RecallError("malformed-lesson-metadata", "lesson metadata version is invalid")
        lesson_id = metadata.get("lessonId")
        captured_at = metadata.get("capturedAt")
        review_state = metadata.get("reviewState")
        if not isinstance(lesson_id, str) or not LESSON_ID_RE.fullmatch(lesson_id):
            raise RecallError("malformed-lesson-metadata", "lesson id is invalid")
        if not valid_timestamp(captured_at):
            raise RecallError("malformed-lesson-metadata", "lesson capture time is invalid")
        if review_state not in {"anchored", "reviewed"}:
            raise RecallError("unanchored-lesson", "lesson is not anchored for recall")
        anchor_meta = metadata.get("sourceAnchor")
        if not isinstance(anchor_meta, dict):
            raise RecallError("unanchored-lesson", "lesson source anchor is absent")
        if metadata.get("repositoryAlias") != self.repository_alias:
            raise RecallError("repository-mismatch", "lesson repository alias differs from the active index")
        required_anchor = {"relativePath", "selector", "contentDigest", "observedAt"}
        if set(anchor_meta) != required_anchor:
            raise RecallError("malformed-lesson-metadata", "lesson source anchor fields are invalid")
        relative = self.paths.normalize_relative(anchor_meta.get("relativePath"))
        source_path = self.paths.resolve_file(relative)
        content_digest = anchor_meta.get("contentDigest")
        if not isinstance(content_digest, str) or not DIGEST_RE.fullmatch(content_digest):
            raise RecallError("malformed-lesson-metadata", "lesson source digest is invalid")
        if digest_file(source_path) != content_digest:
            raise RecallError("digest-mismatch", "lesson source digest does not match")
        selector = normalized_text(anchor_meta.get("selector"), 512)
        observed_at = anchor_meta.get("observedAt")
        if not selector or not valid_timestamp(observed_at):
            raise RecallError("malformed-lesson-metadata", "lesson source selector or time is invalid")
        problem, root_cause, fix, applies_when = (
            normalized_text(value, 512) for value in match.groups()
        )
        if not all((problem, root_cause, fix, applies_when)):
            raise RecallError("malformed-lesson", "lesson has an empty structured field")
        trust = "reviewed-lesson" if review_state == "reviewed" else "anchored-lesson"
        summary = f"Problem: {problem}. Fix: {fix}. Applies when: {applies_when}"
        self.add(
            make_record(
                repository_alias=self.repository_alias,
                kind="lesson",
                identity=lesson_id,
                summary=summary,
                identifiers=[lesson_id],
                phrases=[problem, root_cause, fix, applies_when],
                tags=["lesson", review_state],
                anchor=source_anchor(relative, selector, content_digest, observed_at),
                source_trust=trust,
                admitted_at=captured_at,
            )
        )

    def add_improvement_decisions(self) -> None:
        directory = self.paths.root / "improvements"
        if directory.is_symlink() or not directory.is_dir():
            return
        for path in sorted(directory.iterdir(), key=lambda item: item.name):
            if path.is_symlink() or not path.is_file() or not IMPROVEMENT_RE.fullmatch(path.name):
                continue
            relative = f"improvements/{path.name}"
            text = read_text(path)
            lines = text.splitlines()
            status_token = next(
                (
                    IMPROVEMENT_STATUS_TOKEN_RE.match(line)
                    for line in lines
                    if IMPROVEMENT_STATUS_TOKEN_RE.match(line)
                ),
                None,
            )
            if status_token is None or status_token.group(1) not in {
                "ACCEPTED",
                "IN PROGRESS",
                "APPLIED",
            }:
                continue
            status_match = next(
                (
                    IMPROVEMENT_STATUS_RE.match(line)
                    for line in lines
                    if IMPROVEMENT_STATUS_RE.match(line)
                ),
                None,
            )
            if status_match is None:
                unsupported_count = sum(1 for line in lines if DECISION_RE.match(line))
                self.candidate_count += unsupported_count
                self.exclusions["owner-decision-unsupported"] += unsupported_count
                continue
            improvement_match = IMPROVEMENT_ID_RE.match(path.name)
            if improvement_match is None:
                continue
            improvement_id = improvement_match.group(1)
            source_digest = digest_file(path)
            observed_at = status_match.group(2) + "T00:00:00Z"
            if not valid_timestamp(observed_at):
                continue
            for index, line in enumerate(lines):
                heading = SCOPE_HEADING_RE.match(line)
                if heading is None:
                    continue
                decision = None
                for candidate in lines[index + 1 :]:
                    if candidate.startswith("### "):
                        break
                    decision_match = DECISION_RE.match(candidate)
                    if decision_match:
                        decision = normalized_text(decision_match.group(1), 2000)
                        break
                if not decision:
                    continue
                self.candidate_count += 1
                scope_id = heading.group(1)
                title = normalized_text(heading.group(2), 512)
                selector = f"scope:{scope_id.lower()};field:decision"
                self.add(
                    make_record(
                        repository_alias=self.repository_alias,
                        kind="owner-decision",
                        identity=f"{relative}\x00{selector}",
                        summary=decision,
                        identifiers=[improvement_id, scope_id],
                        phrases=[title, decision],
                        tags=["owner-decision", status_match.group(1).lower().replace(" ", "-")],
                        anchor=source_anchor(relative, selector, source_digest, observed_at),
                        source_trust="owner-approved",
                        admitted_at=observed_at,
                        scope_ref=scope_id,
                    )
                )


def atomic_write(path: Path, payload: bytes) -> None:
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=str(path.parent))
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(payload)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary_name, path)
        try:
            directory_descriptor = os.open(str(path.parent), os.O_RDONLY)
            try:
                os.fsync(directory_descriptor)
            finally:
                os.close(directory_descriptor)
        except OSError:
            pass
    except BaseException:
        try:
            os.close(descriptor)
        except OSError:
            pass
        try:
            os.unlink(temporary_name)
        except FileNotFoundError:
            pass
        raise


def serialize_index(records: Sequence[Dict[str, Any]]) -> bytes:
    if not records:
        return b""
    return ("\n".join(canonical_json(record) for record in records) + "\n").encode("utf-8")


def counts_by_kind(records: Sequence[Dict[str, Any]]) -> Dict[str, int]:
    counts = collections.Counter(record["kind"] for record in records)
    return {key: counts[key] for key in sorted(counts)}


def counts_by_lifecycle(records: Sequence[Dict[str, Any]]) -> Dict[str, int]:
    counts = collections.Counter(record["lifecycle"]["state"] for record in records)
    return {state: counts[state] for state in LIFECYCLE_STATES}


def source_digest_for_records(records: Sequence[Dict[str, Any]]) -> str:
    source_material = "\n".join(
        f"{record['sourceAnchor']['relativePath']}\x00{record['sourceAnchor']['selector']}\x00{record['sourceAnchor']['contentDigest']}"
        for record in records
    ).encode("utf-8")
    return digest_bytes(source_material)


def sync_index(paths: RepositoryPaths, repository_alias: str) -> Dict[str, Any]:
    records, status = IndexBuilder(paths, repository_alias).build()
    index_payload = serialize_index(records)
    status["indexDigest"] = digest_bytes(index_payload)
    status["sourceDigest"] = source_digest_for_records(records)
    runtime = paths.runtime_dir(create=True)
    atomic_write(runtime / INDEX_NAME, index_payload)
    atomic_write(runtime / STATUS_NAME, (canonical_json(status) + "\n").encode("utf-8"))
    response = dict(status)
    response["synced"] = True
    return response


def load_status(paths: RepositoryPaths, repository_alias: str) -> Dict[str, Any]:
    runtime = paths.runtime_dir(create=False)
    status_path = runtime / STATUS_NAME
    if status_path.is_symlink() or not status_path.is_file():
        raise RecallError("index-missing", "experience recall status is missing")
    try:
        status = json.loads(read_text(status_path))
    except json.JSONDecodeError as error:
        raise RecallError("index-invalid", "experience recall status is malformed") from error
    if not isinstance(status, dict) or status.get("repositoryAlias") != repository_alias:
        raise RecallError("index-invalid", "experience recall status has the wrong repository")
    return status


def load_records(paths: RepositoryPaths, repository_alias: str) -> List[Dict[str, Any]]:
    runtime = paths.runtime_dir(create=False)
    index_path = runtime / INDEX_NAME
    if index_path.is_symlink() or not index_path.is_file():
        raise RecallError("index-missing", "experience recall index is missing")
    payload = index_path.read_bytes()
    status = load_status(paths, repository_alias)
    if status.get("indexDigest") != digest_bytes(payload):
        raise RecallError("index-invalid", "experience recall index digest does not match status")
    try:
        lines = payload.decode("utf-8").splitlines()
    except UnicodeDecodeError as error:
        raise RecallError("index-invalid", "experience recall index is not valid UTF-8") from error
    records: List[Dict[str, Any]] = []
    for line_number, line in enumerate(lines, start=1):
        try:
            record = json.loads(line)
        except json.JSONDecodeError as error:
            raise RecallError("index-invalid", f"invalid JSONL record at line {line_number}") from error
        if not isinstance(record, dict):
            raise RecallError("index-invalid", f"non-object JSONL record at line {line_number}")
        validate_record(record)
        if record["repositoryAlias"] != repository_alias:
            raise RecallError("index-invalid", "index contains a record for another repository")
        paths.normalize_relative(record["sourceAnchor"]["relativePath"])
        records.append(record)
    if [record["recordId"] for record in records] != sorted(record["recordId"] for record in records):
        raise RecallError("index-invalid", "index record ordering is not deterministic")
    if len({record["recordId"] for record in records}) != len(records):
        raise RecallError("index-invalid", "index contains duplicate record ids")
    validate_status_against_records(status, records, repository_alias)
    return records


def validate_status_against_records(
    status: Dict[str, Any],
    records: Sequence[Dict[str, Any]],
    repository_alias: str,
) -> None:
    fixed_fields = {
        "contractType": "status",
        "schemaVersion": SCHEMA_VERSION,
        "adapter": PROVIDER,
        "providerVersion": PROVIDER_VERSION,
        "repositoryAlias": repository_alias,
        "state": "ready",
        "indexPath": "/".join(RUNTIME_PARTS + (INDEX_NAME,)),
    }
    if any(status.get(key) != value for key, value in fixed_fields.items()):
        raise RecallError("index-invalid", "experience recall status metadata is inconsistent")

    actual_counts = counts_by_kind(records)
    recorded_counts = status.get("countsByKind")
    if not isinstance(recorded_counts, dict) or any(
        key not in KINDS or type(count) is not int or count < 1
        for key, count in recorded_counts.items()
    ):
        raise RecallError("index-invalid", "experience recall status kind counts are malformed")
    record_count = status.get("recordCount")
    if type(record_count) is not int or record_count != len(records) or recorded_counts != actual_counts:
        raise RecallError("index-invalid", "experience recall status counts do not match the index")
    if status.get("lifecycleCounts") != counts_by_lifecycle(records):
        raise RecallError("index-invalid", "experience recall lifecycle counts do not match the index")

    exclusions = status.get("exclusions")
    if not isinstance(exclusions, dict) or any(
        not isinstance(reason, str)
        or not reason
        or type(count) is not int
        or count < 1
        for reason, count in exclusions.items()
    ):
        raise RecallError("index-invalid", "experience recall status exclusions are malformed")
    excluded_count = status.get("excludedCount")
    if type(excluded_count) is not int or excluded_count != sum(exclusions.values()):
        raise RecallError("index-invalid", "experience recall status exclusion count is inconsistent")
    candidate_count = status.get("candidateCount")
    if type(candidate_count) is not int or candidate_count < 0:
        raise RecallError("index-invalid", "experience recall status candidate count is malformed")

    built_at = max(
        (record["provenance"]["derivedAt"] for record in records),
        default=None,
    )
    if status.get("builtAt") != built_at:
        raise RecallError("index-invalid", "experience recall status build time is inconsistent")
    if status.get("sourceDigest") != source_digest_for_records(records):
        raise RecallError("index-invalid", "experience recall status source digest is inconsistent")


def aggregate_digest(parts: Iterable[str]) -> str:
    return digest_bytes("\n".join(parts).encode("utf-8"))


def assess_freshness(paths: RepositoryPaths, records: Sequence[Dict[str, Any]]) -> Dict[str, Any]:
    checked_at = current_timestamp()
    current_parts: List[str] = []
    stale = 0
    unavailable = 0
    for record in records:
        anchor = record["sourceAnchor"]
        try:
            source = paths.resolve_file(anchor["relativePath"])
        except RecallError:
            unavailable += 1
            continue
        current_digest = digest_file(source)
        current_parts.append(
            f"{anchor['relativePath']}\x00{anchor['selector']}\x00{current_digest}"
        )
        if current_digest != anchor["contentDigest"]:
            stale += 1
    if unavailable:
        return {
            "contractType": "freshness",
            "state": "unknown",
            "sourceDigest": None,
            "checkedAt": checked_at,
            "reason": f"{unavailable} source anchor(s) are unavailable",
        }
    source_digest = aggregate_digest(current_parts)
    if stale:
        return {
            "contractType": "freshness",
            "state": "stale",
            "sourceDigest": source_digest,
            "checkedAt": checked_at,
            "reason": f"{stale} source anchor digest(s) changed",
        }
    return {
        "contractType": "freshness",
        "state": "fresh",
        "sourceDigest": source_digest,
        "checkedAt": checked_at,
        "reason": None,
    }


def unsynchronized_freshness() -> Dict[str, Any]:
    return {
        "contractType": "freshness",
        "state": "unknown",
        "sourceDigest": None,
        "checkedAt": current_timestamp(),
        "reason": "index has not been synchronized",
    }


def provider_freshness(paths: RepositoryPaths, repository_alias: str) -> Dict[str, Any]:
    try:
        records = load_records(paths, repository_alias)
    except RecallError as error:
        if error.code != "index-missing":
            raise
        return unsynchronized_freshness()
    return assess_freshness(paths, records)


def provider_status(paths: RepositoryPaths, repository_alias: str) -> Dict[str, Any]:
    try:
        status = load_status(paths, repository_alias)
        records = load_records(paths, repository_alias)
    except RecallError as error:
        if error.code != "index-missing":
            raise
        return {
            "contractType": "status",
            "schemaVersion": SCHEMA_VERSION,
            "adapter": PROVIDER,
            "providerVersion": PROVIDER_VERSION,
            "repositoryAlias": repository_alias,
            "state": "missing",
            "indexPath": "/".join(RUNTIME_PARTS + (INDEX_NAME,)),
            "recordCount": 0,
            "candidateCount": 0,
            "countsByKind": {},
            "lifecycleCounts": {state: 0 for state in LIFECYCLE_STATES},
            "excludedCount": 0,
            "exclusions": {},
            "builtAt": None,
            "freshness": unsynchronized_freshness(),
        }
    response = dict(status)
    response["freshness"] = assess_freshness(paths, records)
    return response


def tokenize(value: str) -> set[str]:
    return set(TOKEN_RE.findall(value.lower()))


def score_record(record: Dict[str, Any], query: str) -> Dict[str, float]:
    query_normalized = normalized_text(query, 1000).lower()
    query_tokens = tokenize(query_normalized)
    identifiers = [value.lower() for value in record["searchableFields"]["identifiers"]]
    phrases = [value.lower() for value in record["searchableFields"]["phrases"]]
    tags = [value.lower() for value in record["searchableFields"]["tags"]]
    exact_identifier = 10.0 * sum(
        1 for value in identifiers if value == query_normalized or value in query_tokens
    )
    exact_phrase = 8.0 * sum(
        1 for value in phrases if value == query_normalized or query_normalized in value
    )
    identifier_tokens = set().union(*(tokenize(value) for value in identifiers)) if identifiers else set()
    phrase_tokens = set().union(*(tokenize(value) for value in phrases)) if phrases else set()
    summary_tokens = tokenize(record["summary"])
    token_overlap = float(
        4 * len(query_tokens & identifier_tokens)
        + 2 * len(query_tokens & phrase_tokens)
        + len(query_tokens & summary_tokens)
    )
    tag_overlap = float(3 * len(query_tokens & set(tags)))
    total = exact_identifier + exact_phrase + token_overlap + tag_overlap
    return {
        "exactIdentifier": exact_identifier,
        "exactPhrase": exact_phrase,
        "tokenOverlap": token_overlap,
        "tagOverlap": tag_overlap,
        "total": total,
    }


def search_records(
    paths: RepositoryPaths,
    repository_alias: str,
    text: str,
    limit: int,
    kinds: Sequence[str],
    trusts: Sequence[str],
    spec_ref: Optional[str],
    scope_ref: Optional[str],
) -> List[Dict[str, Any]]:
    if not text.strip() or len(text) > 1000:
        raise RecallError("invalid-query", "query text must contain 1 to 1000 characters")
    if limit < 1 or limit > 20:
        raise RecallError("invalid-query", "query limit must be between 1 and 20")
    if any(kind not in KINDS for kind in kinds):
        raise RecallError("invalid-query", "query contains an unknown kind")
    known_trusts = set().union(*TRUST_BY_KIND.values())
    if any(trust not in known_trusts for trust in trusts):
        raise RecallError("invalid-query", "query contains an unknown trust class")
    records = load_records(paths, repository_alias)
    freshness = assess_freshness(paths, records)
    if freshness["state"] != "fresh":
        raise RecallError(f"index-{freshness['state']}", freshness["reason"] or "index is not fresh")
    scored: List[Tuple[Tuple[float, float, float, str], Dict[str, Any]]] = []
    for record in records:
        if record["lifecycle"]["state"] != "admitted" or record["freshness"]["state"] != "fresh":
            continue
        if kinds and record["kind"] not in kinds:
            continue
        if trusts and record["sourceTrust"] not in trusts:
            continue
        if spec_ref is not None and record["specRef"] != spec_ref:
            continue
        if scope_ref is not None and record["scopeRef"] != scope_ref:
            continue
        score = score_record(record, text)
        if score["total"] <= 0:
            continue
        result = {
            "contractType": "result",
            "schemaVersion": SCHEMA_VERSION,
            "recordId": record["recordId"],
            "kind": record["kind"],
            "snippet": record["summary"][:1024],
            "repositoryAlias": record["repositoryAlias"],
            "specRef": record["specRef"],
            "scopeRef": record["scopeRef"],
            "scenarioRefs": record["scenarioRefs"],
            "sourceAnchor": record["sourceAnchor"],
            "sourceTrust": record["sourceTrust"],
            "recallAuthority": record["recallAuthority"],
            "freshness": record["freshness"],
            "lifecycle": record["lifecycle"],
            "provenance": record["provenance"],
            "score": score,
        }
        ordering = (
            -score["total"],
            -score["exactIdentifier"],
            -score["exactPhrase"],
            record["recordId"],
        )
        scored.append((ordering, result))
    scored.sort(key=lambda item: item[0])
    return [result for _, result in scored[:limit]]


def read_record(paths: RepositoryPaths, repository_alias: str, record_id: str) -> Dict[str, Any]:
    records = load_records(paths, repository_alias)
    record = next((item for item in records if item["recordId"] == record_id), None)
    if record is None:
        raise RecallError("record-missing", "record id is not present in the index")
    anchor = record["sourceAnchor"]
    source = paths.resolve_file(anchor["relativePath"])
    if digest_file(source) != anchor["contentDigest"]:
        raise RecallError("record-stale", "record source digest changed")
    return record


def provider_capabilities() -> Dict[str, Any]:
    return {
        "contractType": "provider",
        "schemaVersion": SCHEMA_VERSION,
        "adapter": PROVIDER,
        "providerVersion": PROVIDER_VERSION,
        "capabilities": {
            "search": "derived",
            "read": "derived",
            "status": "derived",
            "freshness": "derived",
            "sync": "derived",
            "export": "unsupported",
            "delete": "unsupported",
            "capabilities": "native",
        },
        "networkAccess": False,
        "automaticInstall": False,
        "defaultBinaryPath": None,
    }


def common_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--repo-root", required=True)
    parser.add_argument("--repository-alias", required=True)


def argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="experience-recall-index.py")
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ("sync", "status", "freshness"):
        command = commands.add_parser(name)
        common_arguments(command)
    search = commands.add_parser("search")
    common_arguments(search)
    search.add_argument("--text", required=True)
    search.add_argument("--limit", type=int, default=5)
    search.add_argument("--kind", action="append", default=[])
    search.add_argument("--trust", action="append", default=[])
    search.add_argument("--spec-ref")
    search.add_argument("--scope-ref")
    read = commands.add_parser("read")
    common_arguments(read)
    read.add_argument("--record-id", required=True)
    commands.add_parser("capabilities")
    return parser


def execute(arguments: argparse.Namespace) -> Any:
    if arguments.command == "capabilities":
        return provider_capabilities()
    paths = RepositoryPaths(arguments.repo_root)
    repository_alias = arguments.repository_alias
    if arguments.command == "sync":
        return sync_index(paths, repository_alias)
    if arguments.command == "status":
        return provider_status(paths, repository_alias)
    if arguments.command == "freshness":
        return provider_freshness(paths, repository_alias)
    if arguments.command == "search":
        return search_records(
            paths,
            repository_alias,
            arguments.text,
            arguments.limit,
            arguments.kind,
            arguments.trust,
            arguments.spec_ref,
            arguments.scope_ref,
        )
    if arguments.command == "read":
        return read_record(paths, repository_alias, arguments.record_id)
    raise RecallError("invalid-command", "unknown index command")


def main() -> int:
    parser = argument_parser()
    arguments = parser.parse_args()
    try:
        response = execute(arguments)
    except RecallError as error:
        print(f"experience-recall-index: {error.code}: {error}", file=sys.stderr)
        return 1
    except OSError as error:
        print(f"experience-recall-index: io-error: {error}", file=sys.stderr)
        return 1
    print(canonical_json(response))
    return 0


if __name__ == "__main__":
    sys.exit(main())
