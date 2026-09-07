#!/usr/bin/env python3
"""Durable, provider-neutral Execution-Control Foundation store.

ECF v2 is a pre-release hardening cut. It rejects v1 store/event bytes rather
than silently attributing v2 guarantees to them. Local verification proves
consistency, not rollback resistance. Rollback resistance requires a checkpoint
retained outside this writable store. Filesystem checks mitigate ordinary
aliases and races, not a malicious process running as the same UID.
"""
from __future__ import annotations

import argparse
import contextlib
import datetime as dt
import hashlib
import importlib.util
import json
import os
import re
import secrets
import stat
import sys
import time
import unicodedata
from pathlib import Path
from typing import Any, Callable, Iterator, NamedTuple, NoReturn

try:
    import fcntl
except ImportError:
    fcntl = None  # type: ignore[assignment]

VERSION = 2
CANONICAL_PROFILE = "bubbles-canonical-json-v1"
LIMITS_PROFILE = "execution-control-limits-v2"
GENESIS = "sha256:" + hashlib.sha256(b"").hexdigest()
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]*$")
EVENT_ID = re.compile(r"^evt:[A-Za-z0-9][A-Za-z0-9._-]*$")
OCCURRENCE_ID = re.compile(r"^occ:[A-Za-z0-9][A-Za-z0-9._-]*$")
ATTEMPT_ID = re.compile(r"^att:[A-Za-z0-9][A-Za-z0-9._-]*$")
TRANSACTION_ID = re.compile(r"^txn:sha256:[0-9a-f]{64}$")
SUBJECT_KIND = re.compile(r"^(?:command|phase|run|stage|budget|session|x\.[a-z0-9]+(?:\.[a-z0-9][a-z0-9-]*){2,})$")
EXTENSION_NS = re.compile(r"^[a-z0-9][a-z0-9-]*(?:\.[a-z0-9][a-z0-9-]*){2,}$")
TIMESTAMP = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$")
MAX_OBJECT_BYTES = 1_048_576
MAX_INPUT_BYTES = 1_048_576
MAX_EVENT_BYTES = 65_536
MAX_PENDING_BYTES = 131_072
MAX_LEDGER_BYTES = 16_777_216
MAX_EVENT_COUNT = 10_000
MAX_OBJECT_COUNT = 10_000
MAX_OBJECT_TOTAL_BYTES = 67_108_864
MAX_OBJECT_PREFIX_ENTRIES = 256
MAX_OBJECT_FILES_PER_PREFIX = 10_000
MAX_CREATE_ONLY_TEMP_ENTRIES = 128
MAX_DEPTH = 32
MAX_MEMBERS = 256
MAX_ARRAY_ITEMS = 1_024
MAX_STRING_BYTES = 16_384
MAX_EXTENSIONS = 32
MAX_IDENTIFIER_BYTES = 128
MAX_READ_LIMIT = 1_000
MAX_INTEGER = 9_007_199_254_740_991
LOCK_WAIT_SECONDS = 10.0
EVENT_TYPES = frozenset(("RECORD", "CORRECT", "SUPERSEDE"))
POSTURES = frozenset(("off", "shadow", "reference-enforce", "unsupported"))
EVENT_FIELDS = frozenset(
    (
        "contractType",
        "schemaVersion",
        "sequence",
        "eventType",
        "eventId",
        "transactionId",
        "previousEventDigest",
        "eventDigest",
        "recordedAt",
        "occurrenceId",
        "attemptId",
        "posture",
        "subject",
        "objectDigest",
        "supersedesEventId",
        "extensions",
    )
)
PROPOSAL_FIELDS = EVENT_FIELDS - frozenset(("sequence", "transactionId", "previousEventDigest", "eventDigest"))
SUBJECT_FIELDS = frozenset(("kind", "id"))
EXTENSION_FIELDS = frozenset(("namespace", "schemaVersion", "payloadDigest"))
STORE_FIELDS = frozenset(("contractType", "schemaVersion", "storeId", "canonicalProfile", "limitsProfile"))
HEAD_FIELDS = frozenset(("contractType", "schemaVersion", "storeId", "sequence", "eventDigest", "canonicalProfile"))
PENDING_FIELDS = frozenset(("contractType", "schemaVersion", "storeId", "transactionId", "expectedSequence", "expectedHeadDigest", "expectedLedgerOffset", "expectedPrefixDigest", "eventLineDigest", "event"))
CHECKPOINT_FIELDS = frozenset(("contractType", "schemaVersion", "storeId", "sequence", "eventDigest", "canonicalProfile"))
AUTHENTICATED_CHECKPOINT_FIELDS = CHECKPOINT_FIELDS | frozenset(("authorityId", "trustRootId", "checkpointAt", "expiresAt", "authenticator"))
CHECKPOINT_AUTHORITY_PURPOSE = "execution-control-checkpoint"
CHECKPOINT_LIFETIME = dt.timedelta(minutes=5)


def load_security_authority() -> Any:
    path = Path(__file__).with_name("security-authority.py")
    spec = importlib.util.spec_from_file_location("execution_control_security_authority", path)
    if spec is None or spec.loader is None:
        raise RuntimeError("unable to load security authority")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


security = load_security_authority()


class ExternalAuthority(NamedTuple):
    file_identity: tuple[int, int]
    ancestor_identities: frozenset[tuple[int, int]]


class VerificationResult(NamedTuple):
    report: dict[str, Any]
    events: list[dict[str, Any]]
    ledger: bytes
    head: dict[str, Any]
    object_count: int
    object_bytes: int


_injected_fault_hook: Callable[[str, Path], None] | None = None


def install_imported_test_fault_hook(hook: Callable[[str, Path], None]) -> None:
    """Install a test-process-only fault hook; the executable CLI never calls this."""
    global _injected_fault_hook
    if __name__ == "__main__":
        invalid("fault hooks are unavailable in executable CLI mode")
    _injected_fault_hook = hook


def injected_fault(point: str, path: Path) -> None:
    if _injected_fault_hook is not None:
        _injected_fault_hook(point, path)


class EcfError(RuntimeError):
    def __init__(self, code: str, kind: str, message: str, exit_code: int) -> None:
        super().__init__(message)
        self.code, self.kind, self.exit_code = code, kind, exit_code


def error(code: str, kind: str, message: str, exit_code: int) -> NoReturn:
    raise EcfError(code, kind, message, exit_code)


def invalid(message: str) -> NoReturn:
    error("ECF-INPUT-INVALID", "input", message, 2)


def limited(message: str) -> NoReturn:
    error("ECF-LIMIT-EXCEEDED", "limit", message, 3)


def conflict(message: str) -> NoReturn:
    error("ECF-CONFLICT", "conflict", message, 4)


def corrupt(message: str) -> NoReturn:
    error("ECF-INTEGRITY", "integrity", message, 5)


def digest_bytes(data: bytes) -> str:
    return "sha256:" + hashlib.sha256(data).hexdigest()


def validate_value(value: Any, depth: int = 0) -> None:
    if depth > MAX_DEPTH:
        limited(f"JSON depth exceeds {MAX_DEPTH}")
    if value is None or isinstance(value, bool):
        return
    if isinstance(value, int):
        if abs(value) > MAX_INTEGER:
            limited("integer exceeds interoperable JSON range")
        return
    if isinstance(value, float):
        invalid("floating-point JSON numbers are forbidden")
    if isinstance(value, str):
        if unicodedata.normalize("NFC", value) != value:
            invalid("JSON strings and keys must be NFC-normalized")
        if len(value.encode("utf-8")) > MAX_STRING_BYTES:
            limited(f"JSON string exceeds {MAX_STRING_BYTES} bytes")
        return
    if isinstance(value, list):
        if len(value) > MAX_ARRAY_ITEMS:
            limited(f"JSON array exceeds {MAX_ARRAY_ITEMS} items")
        for item in value:
            validate_value(item, depth + 1)
        return
    if isinstance(value, dict):
        if len(value) > MAX_MEMBERS:
            limited(f"JSON object exceeds {MAX_MEMBERS} members")
        for key, item in value.items():
            if not isinstance(key, str):
                invalid("JSON object keys must be strings")
            validate_value(key, depth + 1)
            validate_value(item, depth + 1)
        return
    invalid("value is outside Bubbles Canonical JSON v1")


def canonical_bytes(value: Any) -> bytes:
    validate_value(value)
    return json.dumps(value, ensure_ascii=False, allow_nan=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def canonical_line(value: Any) -> bytes:
    return canonical_bytes(value) + b"\n"


def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            invalid(f"duplicate JSON member is forbidden: {key}")
        result[key] = value
    return result


def reject_excessive_nesting(text: str, label: str) -> None:
    """Bound JSON nesting before the recursive standard-library decoder runs."""
    depth = 0
    in_string = False
    escaped = False
    for character in text:
        if in_string:
            if escaped:
                escaped = False
            elif character == "\\":
                escaped = True
            elif character == '"':
                in_string = False
            continue
        if character == '"':
            in_string = True
        elif character in "[{":
            depth += 1
            if depth > MAX_DEPTH + 1:
                limited(f"{label} JSON depth exceeds {MAX_DEPTH}")
        elif character in "]}":
            depth -= 1


def parse_json(data: bytes, label: str, canonical: bool = True) -> Any:
    if data.startswith(b"\xef\xbb\xbf"):
        invalid(f"{label} contains a forbidden UTF-8 BOM")
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        invalid(f"{label} is not UTF-8")
    reject_excessive_nesting(text, label)
    try:
        value = json.loads(
            text,
            object_pairs_hook=unique_object,
            parse_float=lambda _value: invalid("floating-point JSON numbers are forbidden"),
            parse_constant=lambda value: invalid(f"non-finite JSON number is forbidden: {value}"),
        )
    except json.JSONDecodeError as exc:
        invalid(f"{label} is not valid JSON: {exc.msg}")
    validate_value(value)
    if canonical and data != canonical_line(value):
        invalid(f"{label} is not Bubbles Canonical JSON v1 plus one LF")
    return value


def typed_digest(kind: str, payload: Any) -> str:
    return digest_bytes(canonical_bytes({"contractType": kind, "schemaVersion": VERSION, "payload": payload}))


def fields(value: Any, expected: frozenset[str], label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        invalid(f"{label} must be an object")
    actual = frozenset(value)
    if actual != expected:
        invalid(f"{label} field mismatch; missing={sorted(expected - actual)}, unknown={sorted(actual - expected)}")
    return value


def identifier(value: Any, label: str, pattern: re.Pattern[str] = ID) -> str:
    if not isinstance(value, str):
        invalid(f"{label} must be a string")
    if not value.isascii() or len(value.encode("ascii")) > MAX_IDENTIFIER_BYTES or not pattern.fullmatch(value):
        invalid(f"{label} violates its ASCII grammar or byte limit")
    return value


def digest(value: Any, label: str) -> str:
    if not isinstance(value, str) or not DIGEST.fullmatch(value):
        invalid(f"{label} must be a lowercase typed sha256 digest")
    return value


def enum(value: Any, allowed: frozenset[str], label: str) -> str:
    if not isinstance(value, str) or value not in allowed:
        invalid(f"{label} is outside its closed vocabulary")
    return value


def timestamp(value: Any) -> str:
    if not isinstance(value, str) or not TIMESTAMP.fullmatch(value):
        invalid("recordedAt must use canonical UTC YYYY-MM-DDTHH:MM:SS.mmmZ")
    try:
        dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%S.%fZ")
    except ValueError:
        invalid("recordedAt is not a valid UTC timestamp")
    return value


def calculate_event_digest(event: dict[str, Any]) -> str:
    material = dict(event)
    material.pop("eventDigest", None)
    return typed_digest("execution-control-event", material)


def calculate_transaction(sequence: int, head_digest: str, proposal: dict[str, Any]) -> str:
    material = {"expectedSequence": sequence, "expectedHeadDigest": head_digest, "proposal": proposal}
    return "txn:" + typed_digest("execution-control-transaction", material)


def validate_event(event: Any, proposal: bool = False) -> dict[str, Any]:
    value = fields(event, PROPOSAL_FIELDS if proposal else EVENT_FIELDS, "event")
    if value["contractType"] != "execution-control-event" or value["schemaVersion"] != VERSION:
        invalid("event version is unsupported; ECF v1 requires explicit migration")
    enum(value["eventType"], EVENT_TYPES, "eventType")
    enum(value["posture"], POSTURES, "posture")
    identifier(value["eventId"], "eventId", EVENT_ID)
    identifier(value["occurrenceId"], "occurrenceId", OCCURRENCE_ID)
    identifier(value["attemptId"], "attemptId", ATTEMPT_ID)
    timestamp(value["recordedAt"])
    subject = fields(value["subject"], SUBJECT_FIELDS, "subject")
    identifier(subject["kind"], "subject.kind", SUBJECT_KIND)
    identifier(subject["id"], "subject.id")
    digest(value["objectDigest"], "objectDigest")
    supersedes = value["supersedesEventId"]
    if supersedes is not None:
        identifier(supersedes, "supersedesEventId", EVENT_ID)
    if value["eventType"] == "RECORD" and supersedes is not None:
        invalid("RECORD must not supersede an event")
    if value["eventType"] != "RECORD" and supersedes is None:
        invalid("CORRECT and SUPERSEDE must supersede an event")
    extensions = value["extensions"]
    if not isinstance(extensions, list):
        invalid("extensions must be an array")
    if len(extensions) > MAX_EXTENSIONS:
        limited(f"extensions exceeds {MAX_EXTENSIONS} items")
    seen: set[str] = set()
    for index, extension in enumerate(extensions):
        item = fields(extension, EXTENSION_FIELDS, f"extensions[{index}]")
        namespace = identifier(item["namespace"], f"extensions[{index}].namespace", EXTENSION_NS)
        if namespace in seen:
            invalid("extension namespaces must be unique")
        seen.add(namespace)
        if not isinstance(item["schemaVersion"], int) or isinstance(item["schemaVersion"], bool) or item["schemaVersion"] < 1:
            invalid("extension schemaVersion must be a positive integer")
        digest(item["payloadDigest"], "extension payloadDigest")
    if not proposal:
        if not isinstance(value["sequence"], int) or isinstance(value["sequence"], bool) or value["sequence"] < 1:
            invalid("sequence must be a positive integer")
        identifier(value["transactionId"], "transactionId", TRANSACTION_ID)
        digest(value["previousEventDigest"], "previousEventDigest")
        digest(value["eventDigest"], "eventDigest")
        if value["eventDigest"] != calculate_event_digest(value):
            corrupt("eventDigest does not match canonical event material")
    return value


def absolute_path(text: Any, label: str) -> Path:
    if not isinstance(text, str) or not text or "\x00" in text:
        invalid(f"{label} must be a non-empty path")
    path = Path(text)
    if not path.is_absolute() or ".." in path.parts:
        invalid(f"{label} must be lexical absolute without parent traversal")
    return path


def reject_symlinks(path: Path, allow_missing: bool) -> None:
    current = Path(path.parts[0])
    for part in path.parts[1:]:
        current /= part
        try:
            metadata = os.lstat(current)
        except FileNotFoundError:
            if allow_missing:
                return
            error("ECF-IO", "io", f"required path is missing: {current.name}", 7)
        if stat.S_ISLNK(metadata.st_mode):
            corrupt(f"symlink path component is forbidden: {current.name}")


def validate_metadata(metadata: os.stat_result, label: str, directory: bool) -> None:
    expected_mode = 0o700 if directory else 0o600
    if (directory and not stat.S_ISDIR(metadata.st_mode)) or (not directory and not stat.S_ISREG(metadata.st_mode)):
        corrupt(f"{label} has the wrong file type")
    if metadata.st_uid != os.geteuid() or stat.S_IMODE(metadata.st_mode) != expected_mode:
        corrupt(f"{label} has the wrong owner or private mode")
    if not directory and metadata.st_nlink != 1:
        corrupt(f"{label} must have exactly one hardlink")


def open_directory(path: Path) -> tuple[int, os.stat_result]:
    before = os.lstat(path)
    validate_metadata(before, path.name or "/", True)
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0)
    descriptor = os.open(path, flags)
    after = os.fstat(descriptor)
    if (before.st_dev, before.st_ino) != (after.st_dev, after.st_ino):
        os.close(descriptor)
        corrupt(f"directory changed while opening: {path.name}")
    validate_metadata(after, path.name or "/", True)
    return descriptor, after


def open_directory_at(parent: int, name: str, label: str) -> tuple[int, os.stat_result]:
    before = os.stat(name, dir_fd=parent, follow_symlinks=False)
    validate_metadata(before, label, True)
    descriptor = os.open(name, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0), dir_fd=parent)
    after = os.fstat(descriptor)
    if (before.st_dev, before.st_ino) != (after.st_dev, after.st_ino):
        os.close(descriptor)
        corrupt(f"directory changed while opening: {label}")
    validate_metadata(after, label, True)
    return descriptor, after


def open_regular(path: Path, flags: int = os.O_RDONLY) -> tuple[int, os.stat_result]:
    parent, parent_before = open_directory(path.parent)
    try:
        recover_create_only_hardlink_at(parent, path.name)
        before = os.stat(path.name, dir_fd=parent, follow_symlinks=False)
        validate_metadata(before, path.name, False)
        descriptor = os.open(path.name, flags | getattr(os, "O_NOFOLLOW", 0), dir_fd=parent)
        after = os.fstat(descriptor)
        parent_after = os.fstat(parent)
        if (before.st_dev, before.st_ino) != (after.st_dev, after.st_ino):
            os.close(descriptor)
            corrupt(f"path changed while opening: {path.name}")
        if (parent_before.st_dev, parent_before.st_ino) != (parent_after.st_dev, parent_after.st_ino):
            os.close(descriptor)
            corrupt(f"parent changed while opening: {path.name}")
        validate_metadata(after, path.name, False)
        return descriptor, after
    finally:
        os.close(parent)


def bounded_read(descriptor: int, maximum: int, label: str) -> bytes:
    chunks: list[bytes] = []
    total = 0
    while True:
        chunk = os.read(descriptor, min(65_536, maximum + 1 - total))
        if not chunk:
            return b"".join(chunks)
        total += len(chunk)
        if total > maximum:
            limited(f"{label} exceeds {maximum} bytes")
        chunks.append(chunk)


def read_regular(path: Path, maximum: int, label: str) -> bytes:
    descriptor, metadata = open_regular(path)
    try:
        if metadata.st_size > maximum:
            limited(f"{label} exceeds {maximum} bytes")
        return bounded_read(descriptor, maximum, label)
    finally:
        os.close(descriptor)


def read_external(path: Path, maximum: int, label: str) -> tuple[bytes, ExternalAuthority]:
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0)
    parent = os.open(path.anchor, flags)
    ancestors: set[tuple[int, int]] = set()
    try:
        root_metadata = os.fstat(parent)
        ancestors.add((root_metadata.st_dev, root_metadata.st_ino))
        for part in path.parent.parts[1:]:
            before_parent = os.stat(part, dir_fd=parent, follow_symlinks=False)
            if not stat.S_ISDIR(before_parent.st_mode):
                corrupt(f"{label} parent component is not a directory")
            child = os.open(part, flags, dir_fd=parent)
            metadata_parent = os.fstat(child)
            if (before_parent.st_dev, before_parent.st_ino) != (metadata_parent.st_dev, metadata_parent.st_ino):
                os.close(child)
                corrupt(f"{label} parent changed while opening")
            os.close(parent)
            parent = child
            ancestors.add((metadata_parent.st_dev, metadata_parent.st_ino))
        before = os.stat(path.name, dir_fd=parent, follow_symlinks=False)
        if not stat.S_ISREG(before.st_mode) or before.st_uid != os.geteuid() or before.st_nlink != 1:
            corrupt(f"{label} must be an owner-owned regular file with one hardlink")
        descriptor = os.open(path.name, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0), dir_fd=parent)
        metadata = os.fstat(descriptor)
        if (before.st_dev, before.st_ino) != (metadata.st_dev, metadata.st_ino):
            os.close(descriptor)
            corrupt(f"{label} changed while opening")
        if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid != os.geteuid() or metadata.st_nlink != 1:
            os.close(descriptor)
            corrupt(f"{label} failed descriptor identity checks")
        if metadata.st_size > maximum:
            os.close(descriptor)
            limited(f"{label} exceeds {maximum} bytes")
        try:
            data = bounded_read(descriptor, maximum, label)
        finally:
            os.close(descriptor)
        return data, ExternalAuthority((metadata.st_dev, metadata.st_ino), frozenset(ancestors))
    finally:
        os.close(parent)


def ensure_directory(path: Path) -> None:
    if path.exists() or path.is_symlink():
        descriptor, _ = open_directory(path)
        os.close(descriptor)
        return
    parent, _ = open_directory(path.parent)
    try:
        try:
            os.mkdir(path.name, 0o700, dir_fd=parent)
            os.fsync(parent)
        except FileExistsError:
            pass
    finally:
        os.close(parent)
    descriptor, _ = open_directory(path)
    os.close(descriptor)


def write_all(descriptor: int, data: bytes) -> None:
    offset = 0
    while offset < len(data):
        count = os.write(descriptor, data[offset:])
        if count < 1:
            error("ECF-IO", "io", "write made no progress", 7)
        offset += count


def atomic_write(path: Path, data: bytes, create_only: bool = False) -> None:
    parent, parent_identity = open_directory(path.parent)
    temporary = f".{path.name}.tmp.{os.getpid()}.{secrets.token_hex(8)}"
    descriptor = -1
    try:
        if create_only:
            try:
                os.stat(path.name, dir_fd=parent, follow_symlinks=False)
            except FileNotFoundError:
                pass
            else:
                conflict(f"create-only target exists: {path.name}")
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
        descriptor = os.open(temporary, flags, 0o600, dir_fd=parent)
        os.fchmod(descriptor, 0o600)
        write_all(descriptor, data)
        os.fsync(descriptor)
        os.close(descriptor)
        descriptor = -1
        current = os.fstat(parent)
        if (parent_identity.st_dev, parent_identity.st_ino) != (current.st_dev, current.st_ino):
            corrupt(f"parent changed during write: {path.name}")
        if create_only:
            try:
                os.link(temporary, path.name, src_dir_fd=parent, dst_dir_fd=parent, follow_symlinks=False)
            except FileExistsError:
                conflict(f"create-only target exists: {path.name}")
            injected_fault("create-only-after-link", path)
            os.unlink(temporary, dir_fd=parent)
        else:
            os.replace(temporary, path.name, src_dir_fd=parent, dst_dir_fd=parent)
        os.fsync(parent)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        with contextlib.suppress(FileNotFoundError):
            os.unlink(temporary, dir_fd=parent)
        os.close(parent)
    descriptor, _ = open_regular(path)
    os.close(descriptor)


def unlink_synced(path: Path) -> None:
    descriptor, _ = open_regular(path)
    os.close(descriptor)
    parent, _ = open_directory(path.parent)
    try:
        os.unlink(path.name, dir_fd=parent)
        os.fsync(parent)
    finally:
        os.close(parent)


def create_or_validate(path: Path, data: bytes) -> None:
    """Create a protected file or accept a concurrent valid winner."""
    try:
        atomic_write(path, data, True)
    except EcfError as exc:
        if exc.code != "ECF-CONFLICT":
            raise
        recover_create_only_hardlink(path)
        descriptor, _ = open_regular(path)
        os.close(descriptor)


def recover_create_only_hardlink(path: Path) -> None:
    """Remove a same-inode temp link left by a crash after create-only publish."""
    parent, _ = open_directory(path.parent)
    try:
        recover_create_only_hardlink_at(parent, path.name)
    finally:
        os.close(parent)


def recover_create_only_hardlink_at(parent: int, target_name: str) -> None:
    """Recover only recognized, same-inode links while retaining parent authority."""
    try:
        target = os.stat(target_name, dir_fd=parent, follow_symlinks=False)
    except FileNotFoundError:
        return
    if not stat.S_ISREG(target.st_mode) or target.st_uid != os.geteuid():
        corrupt(f"create-only winner has invalid identity: {target_name}")
    pattern = re.compile(rf"^[.]{re.escape(target_name)}[.]tmp[.][0-9]+[.][0-9a-f]{{16}}$")
    matched = 0
    removed = False
    with os.scandir(parent) as entries:
        for entry in entries:
            if not pattern.fullmatch(entry.name):
                continue
            matched += 1
            if matched > MAX_CREATE_ONLY_TEMP_ENTRIES:
                limited(f"create-only temporary entries exceed {MAX_CREATE_ONLY_TEMP_ENTRIES}")
            metadata = entry.stat(follow_symlinks=False)
            if stat.S_ISREG(metadata.st_mode) and (metadata.st_dev, metadata.st_ino) == (target.st_dev, target.st_ino):
                os.unlink(entry.name, dir_fd=parent)
                removed = True
    if removed:
        os.fsync(parent)


class Store:
    def __init__(self, root: str) -> None:
        self.root = absolute_path(root, "store-root")
        self.metadata = self.root / "store.json"
        self.events = self.root / "events.jsonl"
        self.head = self.root / "head.json"
        self.pending = self.root / "pending.json"
        self.objects = self.root / "objects"
        self.projections = self.root / "projections"
        self.lock_file = self.root / "lock"

    def protected_identities(self) -> set[tuple[int, int]]:
        result: set[tuple[int, int]] = set()
        for path in (self.metadata, self.events, self.head, self.pending, self.lock_file):
            try:
                metadata = os.lstat(path)
            except FileNotFoundError:
                continue
            result.add((metadata.st_dev, metadata.st_ino))
        return result

    def contains_external(self, authority: ExternalAuthority) -> bool:
        descriptor, metadata = open_directory(self.root)
        os.close(descriptor)
        return (metadata.st_dev, metadata.st_ino) in authority.ancestor_identities

    def prepare_root(self) -> None:
        reject_symlinks(self.root, True)
        ensure_directory(self.root)
        ensure_directory(self.objects)
        ensure_directory(self.projections)

    def initialize(self) -> None:
        if not self.metadata.exists() and not self.metadata.is_symlink():
            metadata = {
                "canonicalProfile": CANONICAL_PROFILE,
                "contractType": "execution-control-store",
                "limitsProfile": LIMITS_PROFILE,
                "schemaVersion": VERSION,
                "storeId": "store:" + secrets.token_hex(24),
            }
            create_or_validate(self.metadata, canonical_line(metadata))
        metadata = self.load_metadata()
        if not self.events.exists() and not self.events.is_symlink():
            create_or_validate(self.events, b"")
        if not self.head.exists() and not self.head.is_symlink():
            create_or_validate(self.head, canonical_line(self.genesis_head(metadata["storeId"])))

    def load_metadata(self) -> dict[str, Any]:
        raw = read_regular(self.metadata, MAX_PENDING_BYTES, "store metadata")
        value = fields(parse_json(raw, "store metadata"), STORE_FIELDS, "store metadata")
        if value["contractType"] != "execution-control-store" or value["schemaVersion"] != VERSION:
            corrupt("store metadata version is unsupported; ECF v1 requires explicit migration")
        if value["canonicalProfile"] != CANONICAL_PROFILE or value["limitsProfile"] != LIMITS_PROFILE:
            corrupt("store metadata profile is unsupported")
        identifier(value["storeId"], "storeId")
        return value

    @staticmethod
    def genesis_head(store_id: str) -> dict[str, Any]:
        return {"canonicalProfile": CANONICAL_PROFILE, "contractType": "execution-control-head", "eventDigest": GENESIS, "schemaVersion": VERSION, "sequence": 0, "storeId": store_id}

    @contextlib.contextmanager
    def locked(self) -> Iterator[None]:
        if fcntl is None:
            error("ECF-UNSUPPORTED-LOCK", "unsupported", "POSIX fcntl locking is required", 8)
        self.prepare_root()
        if not self.lock_file.exists() and not self.lock_file.is_symlink():
            create_or_validate(self.lock_file, b"")
        descriptor, _ = open_regular(self.lock_file, os.O_RDWR)
        deadline = time.monotonic() + LOCK_WAIT_SECONDS
        try:
            while True:
                try:
                    fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    break
                except BlockingIOError:
                    if time.monotonic() >= deadline:
                        error("ECF-LOCK-TIMEOUT", "lock", "timed out acquiring execution-control lock", 6)
                    time.sleep(0.02)
            self.initialize()
            yield
        finally:
            with contextlib.suppress(OSError):
                fcntl.flock(descriptor, fcntl.LOCK_UN)
            os.close(descriptor)

    def load_head(self) -> dict[str, Any]:
        value = fields(parse_json(read_regular(self.head, MAX_PENDING_BYTES, "head"), "head"), HEAD_FIELDS, "head")
        metadata = self.load_metadata()
        if value["contractType"] != "execution-control-head" or value["schemaVersion"] != VERSION:
            corrupt("head contract or version is invalid")
        if value["storeId"] != metadata["storeId"] or value["canonicalProfile"] != CANONICAL_PROFILE:
            corrupt("head store identity or canonical profile is invalid")
        if not isinstance(value["sequence"], int) or isinstance(value["sequence"], bool) or value["sequence"] < 0:
            corrupt("head sequence is invalid")
        digest(value["eventDigest"], "head eventDigest")
        return value

    def raw_events(self) -> bytes:
        return read_regular(self.events, MAX_LEDGER_BYTES, "events ledger")

    def load_events(self) -> list[dict[str, Any]]:
        return self.validate_ledger_bytes(self.raw_events(), "events ledger")

    def validate_ledger_bytes(self, raw: bytes, label: str) -> list[dict[str, Any]]:
        if raw and not raw.endswith(b"\n"):
            corrupt(f"{label} has a torn final line")
        lines = raw.splitlines()
        if len(lines) > MAX_EVENT_COUNT:
            limited(f"{label} exceeds {MAX_EVENT_COUNT} events")
        result = []
        for number, line in enumerate(lines, 1):
            if not line:
                corrupt(f"{label} contains empty line {number}")
            if len(line) + 1 > MAX_EVENT_BYTES:
                limited(f"event line {number} exceeds {MAX_EVENT_BYTES} bytes")
            result.append(validate_event(parse_json(line + b"\n", f"event line {number}")))
        return result

    def object_path(self, object_digest: str) -> Path:
        digest(object_digest, "objectDigest")
        hexadecimal = object_digest[7:]
        return self.objects / hexadecimal[:2] / hexadecimal

    def require_object(self, object_digest: str) -> Any:
        path = self.object_path(object_digest)
        prefix, _ = open_directory(path.parent)
        try:
            value = self.require_object_at(prefix, path.name)
        finally:
            os.close(prefix)
        return value

    def require_object_at(self, prefix: int, name: str) -> Any:
        recover_create_only_hardlink_at(prefix, name)
        before = os.stat(name, dir_fd=prefix, follow_symlinks=False)
        validate_metadata(before, name, False)
        descriptor = os.open(name, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0), dir_fd=prefix)
        try:
            metadata = os.fstat(descriptor)
            if (before.st_dev, before.st_ino) != (metadata.st_dev, metadata.st_ino):
                corrupt("object changed while opening")
            validate_metadata(metadata, name, False)
            if metadata.st_size > MAX_OBJECT_BYTES:
                limited(f"object exceeds {MAX_OBJECT_BYTES} bytes")
            value = parse_json(bounded_read(descriptor, MAX_OBJECT_BYTES, "object"), "object")
        finally:
            os.close(descriptor)
        object_digest = "sha256:" + name
        if typed_digest("execution-control-object", value) != object_digest:
            corrupt("object content address is invalid")
        return value

    def verify_objects(self) -> tuple[int, int]:
        count = total = 0
        objects, _ = open_directory(self.objects)
        try:
            prefix_names: list[str] = []
            with os.scandir(objects) as prefixes:
                for prefix_entry in prefixes:
                    if len(prefix_names) >= MAX_OBJECT_PREFIX_ENTRIES:
                        limited(f"object prefix entries exceed {MAX_OBJECT_PREFIX_ENTRIES}")
                    prefix_names.append(prefix_entry.name)
            for prefix_name in prefix_names:
                if not re.fullmatch(r"[0-9a-f]{2}", prefix_name):
                    corrupt("object prefix directory is invalid")
                prefix, _ = open_directory_at(objects, prefix_name, prefix_name)
                try:
                    initial_names: list[str] = []
                    with os.scandir(prefix) as paths:
                        for entry in paths:
                            if len(initial_names) >= MAX_OBJECT_FILES_PER_PREFIX + MAX_CREATE_ONLY_TEMP_ENTRIES:
                                limited("object prefix entries exceed file and recovery bounds")
                            initial_names.append(entry.name)
                    temporary_pattern = re.compile(r"^[.]([0-9a-f]{64})[.]tmp[.][0-9]+[.][0-9a-f]{16}$")
                    for initial_name in initial_names:
                        temporary_match = temporary_pattern.fullmatch(initial_name)
                        if temporary_match is None:
                            continue
                        recover_create_only_hardlink_at(prefix, temporary_match.group(1))
                        try:
                            os.stat(initial_name, dir_fd=prefix, follow_symlinks=False)
                        except FileNotFoundError:
                            continue
                        corrupt("object temporary entry is not a recoverable publication link")
                    object_names: list[str] = []
                    with os.scandir(prefix) as paths:
                        for entry in paths:
                            if len(object_names) >= MAX_OBJECT_FILES_PER_PREFIX:
                                limited(f"object files per prefix exceed {MAX_OBJECT_FILES_PER_PREFIX}")
                            object_names.append(entry.name)
                    for object_name in object_names:
                        metadata = os.stat(object_name, dir_fd=prefix, follow_symlinks=False)
                        object_digest = "sha256:" + object_name
                        if object_name[:2] != prefix_name or not DIGEST.fullmatch(object_digest):
                            corrupt("object filename is invalid")
                        validate_metadata(metadata, object_name, False)
                        count += 1
                        total += metadata.st_size
                        if count > MAX_OBJECT_COUNT or total > MAX_OBJECT_TOTAL_BYTES:
                            limited("object store exceeds count or total-byte limit")
                        self.require_object_at(prefix, object_name)
                finally:
                    os.close(prefix)
        finally:
            os.close(objects)
        return count, total

    def validate_lineage(self, events: list[dict[str, Any]]) -> None:
        by_id: dict[str, dict[str, Any]] = {}
        active: dict[tuple[str, str, str], str | None] = {}
        for event in events:
            key = (event["subject"]["kind"], event["subject"]["id"], event["occurrenceId"])
            if event["eventType"] == "RECORD":
                if key in active:
                    corrupt("duplicate or revived lineage root is forbidden")
                active[key] = event["eventId"]
            else:
                target_id = event["supersedesEventId"]
                target = by_id.get(target_id)
                if target is None:
                    corrupt("lineage target is unknown")
                target_key = (target["subject"]["kind"], target["subject"]["id"], target["occurrenceId"])
                if target_key != key or active.get(key) != target_id:
                    corrupt("lineage target crosses identity or branches from an inactive event")
                active[key] = event["eventId"] if event["eventType"] == "CORRECT" else None
            by_id[event["eventId"]] = event

    def verify_event_prefix(self, raw: bytes, expected_head: dict[str, Any], label: str) -> list[dict[str, Any]]:
        events = self.validate_ledger_bytes(raw, label)
        previous = GENESIS
        event_ids: set[str] = set()
        attempt_ids: set[str] = set()
        transaction_ids: set[str] = set()
        for index, event in enumerate(events, 1):
            if event["sequence"] != index or event["previousEventDigest"] != previous:
                corrupt(f"{label} sequence or predecessor chain is invalid")
            if event["eventId"] in event_ids or event["attemptId"] in attempt_ids or event["transactionId"] in transaction_ids:
                corrupt("event, attempt, and transaction identities must be unique")
            event_ids.add(event["eventId"])
            attempt_ids.add(event["attemptId"])
            transaction_ids.add(event["transactionId"])
            self.require_object(event["objectDigest"])
            for extension in event["extensions"]:
                self.require_object(extension["payloadDigest"])
            previous = event["eventDigest"]
        self.validate_lineage(events)
        if expected_head["sequence"] != len(events) or expected_head["eventDigest"] != previous:
            corrupt(f"head does not match the {label} tip")
        return events

    def recover(self) -> None:
        if not self.pending.exists() and not self.pending.is_symlink():
            return
        pending = fields(parse_json(read_regular(self.pending, MAX_PENDING_BYTES, "pending"), "pending"), PENDING_FIELDS, "pending")
        metadata = self.load_metadata()
        event = validate_event(pending["event"])
        if pending["contractType"] != "execution-control-pending" or pending["schemaVersion"] != VERSION:
            corrupt("pending contract or version is invalid")
        identifier(pending["storeId"], "pending storeId")
        transaction_id = identifier(pending["transactionId"], "pending transactionId", TRANSACTION_ID)
        if pending["storeId"] != metadata["storeId"] or transaction_id != event["transactionId"]:
            corrupt("pending store or transaction identity mismatch")
        expected_offset = pending["expectedLedgerOffset"]
        expected_sequence = pending["expectedSequence"]
        if not isinstance(expected_offset, int) or isinstance(expected_offset, bool) or expected_offset < 0:
            corrupt("pending offset is invalid")
        if not isinstance(expected_sequence, int) or isinstance(expected_sequence, bool) or expected_sequence < 0:
            corrupt("pending sequence is invalid")
        expected_head = digest(pending["expectedHeadDigest"], "expectedHeadDigest")
        prefix_digest = digest(pending["expectedPrefixDigest"], "expectedPrefixDigest")
        line_digest = digest(pending["eventLineDigest"], "eventLineDigest")
        event_line = canonical_line(event)
        if digest_bytes(event_line) != line_digest:
            corrupt("pending event line digest mismatch")
        if event["sequence"] != expected_sequence + 1 or event["previousEventDigest"] != expected_head:
            corrupt("pending event sequence or predecessor mismatch")
        proposal = {key: event[key] for key in PROPOSAL_FIELDS}
        recomputed_transaction = calculate_transaction(expected_sequence, expected_head, proposal)
        if transaction_id != recomputed_transaction or event["transactionId"] != recomputed_transaction:
            corrupt("pending transaction digest does not match canonical proposal")
        descriptor, _ = open_regular(self.events, os.O_RDWR)
        try:
            raw = bounded_read(descriptor, MAX_LEDGER_BYTES, "events ledger")
            if len(raw) < expected_offset or digest_bytes(raw[:expected_offset]) != prefix_digest:
                conflict("pending ledger prefix does not match")
            suffix = raw[expected_offset:]
            if not event_line.startswith(suffix):
                conflict("pending ledger suffix diverges")
            old_head = self.genesis_head(metadata["storeId"])
            old_head.update(sequence=expected_sequence, eventDigest=expected_head)
            new_head = dict(old_head)
            new_head.update(sequence=event["sequence"], eventDigest=event["eventDigest"])
            self.verify_event_prefix(raw[:expected_offset], old_head, "pending ledger prefix")
            self.verify_objects()
            self.verify_event_prefix(raw[:expected_offset] + event_line, new_head, "pending recovery candidate")
            head = self.load_head()
            if head != old_head and head != new_head:
                conflict("pending recovery found a conflicting head")
            if head == new_head and suffix != event_line:
                conflict("pending recovery found an advanced head with an incomplete ledger")
            if suffix != event_line:
                os.ftruncate(descriptor, expected_offset)
                os.fsync(descriptor)
                os.lseek(descriptor, expected_offset, os.SEEK_SET)
                write_all(descriptor, event_line)
                os.fsync(descriptor)
        finally:
            os.close(descriptor)
        if head == old_head:
            atomic_write(self.head, canonical_line(new_head))
        unlink_synced(self.pending)

    def checkpoint(self, authority_path: str | None = None) -> dict[str, Any]:
        head = self.load_head()
        result = {"canonicalProfile": CANONICAL_PROFILE, "contractType": "execution-control-checkpoint", "eventDigest": head["eventDigest"], "schemaVersion": VERSION, "sequence": head["sequence"], "storeId": head["storeId"]}
        if authority_path is not None:
            authority = security.load(authority_path, CHECKPOINT_AUTHORITY_PURPOSE)
            now = dt.datetime.now(dt.timezone.utc)
            result.update(authorityId=authority["authorityId"], trustRootId=authority["trustRootId"], checkpointAt=now.isoformat(timespec="milliseconds").replace("+00:00", "Z"), expiresAt=(now + CHECKPOINT_LIFETIME).isoformat(timespec="milliseconds").replace("+00:00", "Z"))
            result["authenticator"] = security.authenticator(authority, result)
        return result

    def validate_checkpoint(self, checkpoint: Any, authority_path: str | None = None) -> tuple[dict[str, Any], str]:
        expected_fields = AUTHENTICATED_CHECKPOINT_FIELDS if authority_path is not None else CHECKPOINT_FIELDS
        value = fields(checkpoint, expected_fields, "checkpoint")
        if value["contractType"] != "execution-control-checkpoint":
            invalid("checkpoint contractType is invalid")
        if value["schemaVersion"] != VERSION:
            invalid("checkpoint version is unsupported")
        store_id = identifier(value["storeId"], "checkpoint storeId")
        if value["canonicalProfile"] != CANONICAL_PROFILE:
            invalid("checkpoint canonicalProfile is invalid")
        sequence = value["sequence"]
        if not isinstance(sequence, int) or isinstance(sequence, bool) or sequence < 0 or sequence > MAX_EVENT_COUNT:
            invalid("checkpoint sequence is outside the supported range")
        digest(value["eventDigest"], "checkpoint eventDigest")
        if store_id != self.load_metadata()["storeId"]:
            error("ECF-ANCHOR-MISMATCH", "anchor", "caller checkpoint belongs to a different store", 9)
        if authority_path is None:
            return value, "not-evaluated"
        try:
            authority = security.load(authority_path, CHECKPOINT_AUTHORITY_PURPOSE)
        except (security.AuthorityError, OSError) as exc:
            error("ECF-AUTHORITY-INVALID", "authority", f"checkpoint authority is invalid: {exc}", 10)
        if value["authorityId"] != authority["authorityId"] or value["trustRootId"] != authority["trustRootId"]:
            error("ECF-AUTHORITY-INVALID", "authority", "checkpoint authority or trust root does not match configured authority", 10)
        checkpoint_at = timestamp(value["checkpointAt"])
        expires_at = timestamp(value["expiresAt"])
        checkpoint_time = dt.datetime.strptime(checkpoint_at, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=dt.timezone.utc)
        expiry_time = dt.datetime.strptime(expires_at, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=dt.timezone.utc)
        now = dt.datetime.now(dt.timezone.utc)
        if expiry_time <= checkpoint_time or expiry_time - checkpoint_time > CHECKPOINT_LIFETIME or now < checkpoint_time or now > expiry_time:
            error("ECF-CHECKPOINT-STALE", "freshness", "checkpoint freshness window is invalid or expired", 11)
        unsigned = dict(value)
        supplied = unsigned.pop("authenticator")
        if not security.verify(authority, unsigned, supplied):
            error("ECF-AUTHORITY-INVALID", "authority", "checkpoint authenticator is invalid", 10)
        return value, "satisfied"

    def verified(self, checkpoint: dict[str, Any] | None = None, authority_path: str | None = None, require_rollback_resistance: bool = False) -> VerificationResult:
        self.recover()
        ledger = self.raw_events()
        head = self.load_head()
        events = self.verify_event_prefix(ledger, head, "events ledger")
        object_count, object_bytes = self.verify_objects()
        anchor_status = "not-provided"
        rollback_resistance = "not-evaluated"
        if checkpoint is not None:
            validated_checkpoint, rollback_resistance = self.validate_checkpoint(checkpoint, authority_path)
            local_checkpoint = self.checkpoint()
            compared_checkpoint = {key: validated_checkpoint[key] for key in CHECKPOINT_FIELDS}
            if compared_checkpoint != local_checkpoint:
                error("ECF-ANCHOR-MISMATCH", "anchor", "caller checkpoint does not match local head", 9)
            anchor_status = "authenticated" if rollback_resistance == "satisfied" else "local-match"
        if require_rollback_resistance and rollback_resistance != "satisfied":
            error("ECF-ROLLBACK-NOT-EVALUATED", "authority", "rollback resistance requires an authenticated fresh checkpoint and configured authority", 10)
        report = {"anchorStatus": anchor_status, "canonicalProfile": CANONICAL_PROFILE, "contractType": "execution-control-verification", "eventCount": len(events), "headDigest": head["eventDigest"], "limitsProfile": LIMITS_PROFILE, "localConsistency": True, "objectBytes": object_bytes, "objectCount": object_count, "rollbackResistance": rollback_resistance, "schemaVersion": VERSION, "storeId": head["storeId"]}
        return VerificationResult(report, events, ledger, head, object_count, object_bytes)

    def verify(self, checkpoint: dict[str, Any] | None = None, authority_path: str | None = None, require_rollback_resistance: bool = False) -> dict[str, Any]:
        return self.verified(checkpoint, authority_path, require_rollback_resistance).report

    def put_object(self, value: Any) -> dict[str, Any]:
        verification = self.verified()
        payload = canonical_line(value)
        if len(payload) > MAX_OBJECT_BYTES:
            limited(f"object exceeds {MAX_OBJECT_BYTES} bytes")
        object_digest = typed_digest("execution-control-object", value)
        path = self.object_path(object_digest)
        ensure_directory(path.parent)
        if path.exists() or path.is_symlink():
            self.require_object(object_digest)
            return {"objectDigest": object_digest, "stored": False}
        if verification.object_count + 1 > MAX_OBJECT_COUNT or verification.object_bytes + len(payload) > MAX_OBJECT_TOTAL_BYTES:
            limited("object insertion exceeds store limit")
        atomic_write(path, payload, True)
        return {"objectDigest": object_digest, "stored": True}

    def append(self, expected_sequence: int, expected_digest: str, proposal: dict[str, Any]) -> dict[str, Any]:
        verification = self.verified()
        events = verification.events
        transaction = calculate_transaction(expected_sequence, expected_digest, proposal)
        for existing in events:
            if existing["transactionId"] == transaction:
                reconstructed = {key: existing[key] for key in PROPOSAL_FIELDS}
                if reconstructed == proposal and existing["sequence"] == expected_sequence + 1 and existing["previousEventDigest"] == expected_digest:
                    return existing
                conflict("transaction identity is ambiguous")
        head = verification.head
        if head["sequence"] != expected_sequence or head["eventDigest"] != expected_digest:
            conflict("compare-and-append predecessor is stale")
        if len(events) >= MAX_EVENT_COUNT:
            limited(f"events ledger exceeds {MAX_EVENT_COUNT} events")
        if proposal["eventId"] in {event["eventId"] for event in events} or proposal["attemptId"] in {event["attemptId"] for event in events}:
            conflict("eventId or attemptId already exists")
        event = dict(proposal)
        event.update(sequence=expected_sequence + 1, transactionId=transaction, previousEventDigest=expected_digest)
        event["eventDigest"] = calculate_event_digest(event)
        validate_event(event)
        self.validate_lineage(events + [event])
        self.require_object(event["objectDigest"])
        for extension in event["extensions"]:
            self.require_object(extension["payloadDigest"])
        event_line = canonical_line(event)
        ledger = verification.ledger
        if len(event_line) > MAX_EVENT_BYTES or len(ledger) + len(event_line) > MAX_LEDGER_BYTES:
            limited("event or ledger byte limit exceeded")
        pending = {"contractType": "execution-control-pending", "event": event, "eventLineDigest": digest_bytes(event_line), "expectedHeadDigest": expected_digest, "expectedLedgerOffset": len(ledger), "expectedPrefixDigest": digest_bytes(ledger), "expectedSequence": expected_sequence, "schemaVersion": VERSION, "storeId": head["storeId"], "transactionId": transaction}
        pending_line = canonical_line(pending)
        if len(pending_line) > MAX_PENDING_BYTES:
            limited("pending transaction byte limit exceeded")
        atomic_write(self.pending, pending_line, True)
        injected_fault("after-pending", self.pending)
        descriptor, _ = open_regular(self.events, os.O_WRONLY | os.O_APPEND)
        try:
            injected_fault("before-ledger-write", self.events)
            write_all(descriptor, event_line)
            injected_fault("after-ledger-write", self.events)
            os.fsync(descriptor)
            injected_fault("after-ledger-fsync", self.events)
        finally:
            os.close(descriptor)
        new_head = dict(head)
        new_head.update(sequence=event["sequence"], eventDigest=event["eventDigest"])
        atomic_write(self.head, canonical_line(new_head))
        injected_fault("after-head", self.head)
        unlink_synced(self.pending)
        return event

    def project(self) -> dict[str, Any]:
        verification = self.verified()
        events = verification.events
        projection = {"anchorStatus": "not-provided", "canonicalProfile": CANONICAL_PROFILE, "contractType": "execution-control-integrity-projection", "eventCount": len(events), "eventTypeCounts": {name: sum(event["eventType"] == name for event in events) for name in sorted(EVENT_TYPES)}, "headDigest": verification.report["headDigest"], "limitsProfile": LIMITS_PROFILE, "localConsistency": True, "objectBytes": verification.object_bytes, "objectCount": verification.object_count, "postureCounts": {name: sum(event["posture"] == name for event in events) for name in sorted(POSTURES)}, "schemaVersion": VERSION, "storeId": verification.report["storeId"]}
        atomic_write(self.projections / "integrity.json", canonical_line(projection))
        return projection


class Parser(argparse.ArgumentParser):
    def error(self, message: str) -> NoReturn:
        error("ECF-USAGE-INVALID", "usage", message, 2)


def parser() -> Parser:
    result = Parser(description="Bubbles Execution-Control Foundation store")
    subs = result.add_subparsers(dest="command", required=True, parser_class=Parser)
    put = subs.add_parser("object-put")
    put.add_argument("--store-root", required=True)
    put.add_argument("--input", required=True)
    append = subs.add_parser("append")
    append.add_argument("--store-root", required=True)
    append.add_argument("--expected-sequence", required=True)
    append.add_argument("--expected-head-digest", required=True)
    append.add_argument("--event-file", required=True)
    read = subs.add_parser("read")
    read.add_argument("--store-root", required=True)
    read.add_argument("--from-sequence", required=True)
    read.add_argument("--limit", required=True)
    verify = subs.add_parser("verify")
    verify.add_argument("--store-root", required=True)
    verify.add_argument("--checkpoint")
    verify.add_argument("--checkpoint-authority")
    verify.add_argument("--require-rollback-resistance", action="store_true")
    checkpoint = subs.add_parser("checkpoint")
    checkpoint.add_argument("--store-root", required=True)
    checkpoint.add_argument("--checkpoint-authority")
    project = subs.add_parser("project")
    project.add_argument("--store-root", required=True)
    return result


def decimal(value: Any, label: str, minimum: int, maximum: int) -> int:
    if not isinstance(value, str) or not re.fullmatch(r"0|[1-9][0-9]*", value):
        invalid(f"{label} must be a canonical non-negative decimal integer")
    result = int(value)
    if result < minimum or result > maximum:
        limited(f"{label} is outside [{minimum}, {maximum}]")
    return result


def execute(args: argparse.Namespace) -> dict[str, Any]:
    store = Store(args.store_root)
    if args.command == "object-put":
        raw, input_authority = read_external(absolute_path(args.input, "input"), MAX_INPUT_BYTES, "object input")
        value = parse_json(raw, "object input", canonical=False)
        with store.locked():
            if input_authority.file_identity in store.protected_identities() or store.contains_external(input_authority):
                corrupt("object input aliases store internals")
            return store.put_object(value)
    if args.command == "append":
        sequence = decimal(args.expected_sequence, "expected-sequence", 0, MAX_EVENT_COUNT)
        head_digest = digest(args.expected_head_digest, "expected-head-digest")
        raw, input_authority = read_external(absolute_path(args.event_file, "event-file"), MAX_EVENT_BYTES, "event proposal")
        proposal = validate_event(parse_json(raw, "event proposal"), True)
        with store.locked():
            if input_authority.file_identity in store.protected_identities() or store.contains_external(input_authority):
                corrupt("event input aliases store internals")
            return store.append(sequence, head_digest, proposal)
    if args.command == "read":
        start = decimal(args.from_sequence, "from-sequence", 1, MAX_EVENT_COUNT)
        limit = decimal(args.limit, "limit", 1, MAX_READ_LIMIT)
        with store.locked():
            verification = store.verify()
            events = [event for event in store.load_events() if event["sequence"] >= start][:limit]
            return {"anchorStatus": "not-provided", "contractType": "execution-control-read", "events": events, "headDigest": verification["headDigest"], "schemaVersion": VERSION, "storeId": verification["storeId"]}
    if args.command == "verify":
        checkpoint = None
        checkpoint_authority = None
        trust_authority = None
        if args.checkpoint:
            raw, checkpoint_authority = read_external(absolute_path(args.checkpoint, "checkpoint"), MAX_PENDING_BYTES, "checkpoint")
            checkpoint = parse_json(raw, "checkpoint")
        if args.checkpoint_authority and not args.checkpoint:
            invalid("checkpoint-authority requires checkpoint")
        if args.checkpoint_authority:
            _, trust_authority = read_external(absolute_path(args.checkpoint_authority, "checkpoint-authority"), 16_384, "checkpoint authority")
        with store.locked():
            if checkpoint_authority is not None and store.contains_external(checkpoint_authority):
                corrupt("checkpoint must be retained outside the writable store hierarchy")
            if trust_authority is not None and store.contains_external(trust_authority):
                corrupt("checkpoint authority must be retained outside the writable store hierarchy")
            return store.verify(checkpoint, args.checkpoint_authority, args.require_rollback_resistance)
    if args.command == "checkpoint":
        trust_authority = None
        if args.checkpoint_authority:
            _, trust_authority = read_external(absolute_path(args.checkpoint_authority, "checkpoint-authority"), 16_384, "checkpoint authority")
        with store.locked():
            if trust_authority is not None and store.contains_external(trust_authority):
                corrupt("checkpoint authority must be retained outside the writable store hierarchy")
            store.verify()
            return store.checkpoint(args.checkpoint_authority)
    if args.command == "project":
        with store.locked():
            return store.project()
    invalid("unsupported command")


def emit_error(exc: EcfError) -> int:
    envelope = {"code": exc.code, "contractType": "execution-control-error", "errorClass": exc.kind, "message": str(exc)[:512], "schemaVersion": VERSION}
    sys.stderr.buffer.write(canonical_line(envelope))
    sys.stderr.buffer.flush()
    return exc.exit_code


def main() -> int:
    try:
        result = execute(parser().parse_args())
        sys.stdout.buffer.write(canonical_line(result))
        sys.stdout.buffer.flush()
        return 0
    except EcfError as exc:
        return emit_error(exc)
    except (OSError, ValueError, TypeError) as exc:
        return emit_error(EcfError("ECF-IO", "io", f"operation failed: {exc}", 7))


if __name__ == "__main__":
    raise SystemExit(main())
