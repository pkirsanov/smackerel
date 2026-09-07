#!/usr/bin/env python3
"""Descriptor-safe session-state, lock, and host-usage operations.

This helper is intentionally narrow. It uses only the Python standard library
and keeps repository path traversal, no-follow opens, and lock ownership in one
implementation shared by the session-state consumers.
"""

from __future__ import annotations

import argparse
import errno
import fcntl
import hashlib
import json
import math
import os
import secrets
import stat
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, NoReturn, Sequence

EXIT_USAGE = 2
EXIT_OPERATIONAL = 3
EXIT_ABSENT = 4
EXIT_UNSAFE = 5
EXIT_CHANGED = 6
EXIT_TIMEOUT = 7
EXIT_PARSE = 8


class StateIOError(Exception):
    """A closed helper failure with a stable exit and reason."""

    def __init__(self, exit_code: int, reason: str) -> None:
        super().__init__(reason)
        self.exit_code = exit_code
        self.reason = reason


@dataclass(frozen=True)
class FileIdentity:
    device: int
    inode: int
    mode: int
    links: int
    size: int
    mtime_ns: int
    ctime_ns: int

    @classmethod
    def from_stat(cls, value: os.stat_result) -> "FileIdentity":
        return cls(
            device=value.st_dev,
            inode=value.st_ino,
            mode=value.st_mode,
            links=value.st_nlink,
            size=value.st_size,
            mtime_ns=value.st_mtime_ns,
            ctime_ns=value.st_ctime_ns,
        )

    def same_object(self, other: "FileIdentity") -> bool:
        return (
            self.device == other.device
            and self.inode == other.inode
            and stat.S_IFMT(self.mode) == stat.S_IFMT(other.mode)
        )

    def same_snapshot(self, other: "FileIdentity") -> bool:
        return self == other


@dataclass
class ParentHandle:
    fd: int
    name: str

    def close(self) -> None:
        os.close(self.fd)


@dataclass(frozen=True)
class UsageArtifact:
    suffix: str
    data: bytes


def fail(exit_code: int, reason: str) -> NoReturn:
    raise StateIOError(exit_code, reason)


def validate_relative_path(raw: str, option: str) -> tuple[str, ...]:
    if not raw or raw.startswith("/") or "\x00" in raw:
        fail(EXIT_USAGE, f"invalid-{option}")
    parts = tuple(raw.split("/"))
    if any(part in ("", ".", "..") for part in parts):
        fail(EXIT_USAGE, f"invalid-{option}")
    return parts


def open_validated_root(raw_root: str) -> int:
    if not raw_root or not os.path.isabs(raw_root):
        fail(EXIT_USAGE, "invalid-root")
    flags = os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW
    try:
        fd = os.open(raw_root, flags)
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR):
            fail(EXIT_UNSAFE, "unsafe-root")
        fail(EXIT_OPERATIONAL, "root-open-failed")
    identity = FileIdentity.from_stat(os.fstat(fd))
    if not stat.S_ISDIR(identity.mode):
        os.close(fd)
        fail(EXIT_UNSAFE, "root-not-directory")
    return fd


def walk_parent(
    root_fd: int,
    parts: tuple[str, ...],
    *,
    create_missing: bool = False,
) -> ParentHandle:
    current = os.dup(root_fd)
    try:
        for component in parts[:-1]:
            try:
                next_fd = os.open(
                    component,
                    os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                    dir_fd=current,
                )
            except OSError as error:
                if error.errno == errno.ENOENT and create_missing:
                    try:
                        os.mkdir(component, mode=0o700, dir_fd=current)
                    except FileExistsError:
                        pass
                    except OSError:
                        fail(EXIT_OPERATIONAL, "parent-create-failed")
                    try:
                        next_fd = os.open(
                            component,
                            os.O_RDONLY
                            | os.O_DIRECTORY
                            | os.O_CLOEXEC
                            | os.O_NOFOLLOW,
                            dir_fd=current,
                        )
                    except OSError as open_error:
                        if open_error.errno in (errno.ELOOP, errno.ENOTDIR):
                            fail(EXIT_UNSAFE, "unsafe-created-parent")
                        fail(EXIT_OPERATIONAL, "created-parent-open-failed")
                elif error.errno in (errno.ELOOP, errno.ENOTDIR):
                    fail(EXIT_UNSAFE, "unsafe-parent-component")
                elif error.errno == errno.ENOENT:
                    fail(EXIT_ABSENT, "parent-absent")
                else:
                    fail(EXIT_OPERATIONAL, "parent-open-failed")
            next_identity = FileIdentity.from_stat(os.fstat(next_fd))
            if not stat.S_ISDIR(next_identity.mode):
                os.close(next_fd)
                fail(EXIT_UNSAFE, "parent-not-directory")
            os.close(current)
            current = next_fd
        return ParentHandle(current, parts[-1])
    except BaseException:
        os.close(current)
        raise


def lstat_at(parent_fd: int, name: str) -> os.stat_result | None:
    try:
        return os.stat(name, dir_fd=parent_fd, follow_symlinks=False)
    except FileNotFoundError:
        return None
    except OSError:
        fail(EXIT_OPERATIONAL, "entry-stat-failed")


def require_stable_entry(
    parent_fd: int,
    name: str,
    opened: FileIdentity,
    *,
    require_single_link: bool,
) -> None:
    entry_stat = lstat_at(parent_fd, name)
    if entry_stat is None:
        fail(EXIT_CHANGED, "entry-disappeared")
    entry = FileIdentity.from_stat(entry_stat)
    if not opened.same_object(entry):
        fail(EXIT_CHANGED, "entry-replaced")
    if require_single_link and (opened.links != 1 or entry.links != 1):
        fail(EXIT_UNSAFE, "entry-link-count")


def open_regular_read(parent_fd: int, name: str) -> int:
    try:
        fd = os.open(name, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW, dir_fd=parent_fd)
    except FileNotFoundError:
        fail(EXIT_ABSENT, "source-absent")
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR):
            fail(EXIT_UNSAFE, "source-unsafe")
        fail(EXIT_OPERATIONAL, "source-open-failed")
    identity = FileIdentity.from_stat(os.fstat(fd))
    if not stat.S_ISREG(identity.mode):
        os.close(fd)
        fail(EXIT_UNSAFE, "source-not-regular")
    return fd


def read_stable_regular(parent_fd: int, name: str) -> tuple[bytes, FileIdentity]:
    fd = open_regular_read(parent_fd, name)
    try:
        before = FileIdentity.from_stat(os.fstat(fd))
        require_stable_entry(parent_fd, name, before, require_single_link=False)
        chunks: list[bytes] = []
        while True:
            try:
                chunk = os.read(fd, 1024 * 1024)
            except OSError:
                fail(EXIT_OPERATIONAL, "source-read-failed")
            if not chunk:
                break
            chunks.append(chunk)
        after = FileIdentity.from_stat(os.fstat(fd))
        if not before.same_snapshot(after):
            fail(EXIT_CHANGED, "source-changed")
        require_stable_entry(parent_fd, name, after, require_single_link=False)
        return b"".join(chunks), after
    finally:
        os.close(fd)


def write_all(fd: int, data: bytes, reason: str) -> None:
    offset = 0
    while offset < len(data):
        try:
            written = os.write(fd, data[offset:])
        except OSError:
            fail(EXIT_OPERATIONAL, reason)
        if written <= 0:
            fail(EXIT_OPERATIONAL, reason)
        offset += written


def write_private_destination(destination: str, data: bytes) -> None:
    if not destination or not os.path.isabs(destination):
        fail(EXIT_USAGE, "invalid-destination")
    destination_path = Path(destination)
    parent = destination_path.parent
    if not parent.is_dir():
        fail(EXIT_OPERATIONAL, "destination-parent-missing")
    if destination_path.exists() or destination_path.is_symlink():
        fail(EXIT_UNSAFE, "destination-exists")
    try:
        fd = os.open(
            destination,
            os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW,
            0o600,
        )
        try:
            write_all(fd, data, "destination-write-failed")
            os.fsync(fd)
        finally:
            os.close(fd)
    except StateIOError:
        try:
            os.unlink(destination)
        except OSError:
            pass
        raise
    except OSError:
        try:
            os.unlink(destination)
        except OSError:
            pass
        fail(EXIT_OPERATIONAL, "destination-write-failed")


def capture(args: argparse.Namespace) -> int:
    parts = validate_relative_path(args.relative_path, "relative-path")
    root_fd = open_validated_root(args.root)
    try:
        parent = walk_parent(root_fd, parts)
        try:
            data, identity = read_stable_regular(parent.fd, parent.name)
        finally:
            parent.close()
    finally:
        os.close(root_fd)
    write_private_destination(args.destination, data)
    result = {
        "status": "captured",
        "bytes": len(data),
        "revision": "sha256:" + hashlib.sha256(data).hexdigest(),
        "device": identity.device,
        "inode": identity.inode,
    }
    print(json.dumps(result, separators=(",", ":"), sort_keys=True))
    return 0


def usage_candidate_suffix(name: str, session_id: str | None) -> str | None:
    for suffix in (".jsonl", ".json"):
        if session_id is None:
            if name.endswith(suffix):
                return suffix
        elif name == session_id + suffix:
            return suffix
    return None


def open_usage_root(raw_root: str) -> tuple[int, FileIdentity] | None:
    if not raw_root or not os.path.isabs(raw_root) or "\x00" in raw_root:
        fail(EXIT_USAGE, "invalid-usage-root")
    flags = os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW
    try:
        fd = os.open(raw_root, flags)
    except FileNotFoundError:
        return None
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR):
            fail(EXIT_UNSAFE, "unsafe-usage-root")
        fail(EXIT_OPERATIONAL, "usage-root-open-failed")
    identity = FileIdentity.from_stat(os.fstat(fd))
    if not stat.S_ISDIR(identity.mode):
        os.close(fd)
        fail(EXIT_UNSAFE, "usage-root-not-directory")
    try:
        path_identity = FileIdentity.from_stat(os.stat(raw_root, follow_symlinks=False))
    except FileNotFoundError:
        os.close(fd)
        fail(EXIT_CHANGED, "usage-root-disappeared")
    except OSError:
        os.close(fd)
        fail(EXIT_OPERATIONAL, "usage-root-stat-failed")
    if not identity.same_snapshot(path_identity):
        os.close(fd)
        fail(EXIT_CHANGED, "usage-root-replaced")
    return fd, identity


def open_usage_directory(
    parent_fd: int,
    name: str,
    selected: FileIdentity,
) -> tuple[int, FileIdentity]:
    try:
        fd = os.open(
            name,
            os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=parent_fd,
        )
    except FileNotFoundError:
        fail(EXIT_CHANGED, "usage-directory-disappeared")
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR):
            fail(EXIT_UNSAFE, "unsafe-usage-directory")
        fail(EXIT_OPERATIONAL, "usage-directory-open-failed")
    opened = FileIdentity.from_stat(os.fstat(fd))
    if not stat.S_ISDIR(opened.mode):
        os.close(fd)
        fail(EXIT_UNSAFE, "usage-entry-not-directory")
    if not selected.same_snapshot(opened):
        os.close(fd)
        fail(EXIT_CHANGED, "usage-directory-replaced")
    require_stable_entry(parent_fd, name, opened, require_single_link=False)
    return fd, opened


def walk_usage_directory(
    directory_fd: int,
    directory_identity: FileIdentity,
    *,
    inside_chat_sessions: bool,
    session_id: str | None,
    artifacts: list[UsageArtifact],
) -> None:
    try:
        with os.scandir(directory_fd) as entries:
            for entry in entries:
                name = entry.name
                try:
                    entry_stat = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
                except FileNotFoundError:
                    fail(EXIT_CHANGED, "usage-entry-disappeared")
                except OSError:
                    fail(EXIT_OPERATIONAL, "usage-entry-stat-failed")
                identity = FileIdentity.from_stat(entry_stat)
                suffix = (
                    usage_candidate_suffix(name, session_id)
                    if inside_chat_sessions
                    else None
                )
                if stat.S_ISLNK(identity.mode):
                    fail(EXIT_UNSAFE, "usage-symlink-entry")
                if suffix is not None:
                    if not stat.S_ISREG(identity.mode):
                        fail(EXIT_UNSAFE, "usage-candidate-not-regular")
                    data, opened = read_stable_regular(directory_fd, name)
                    if not identity.same_snapshot(opened):
                        fail(EXIT_CHANGED, "usage-candidate-replaced")
                    artifacts.append(UsageArtifact(suffix=suffix, data=data))
                    continue
                if stat.S_ISDIR(identity.mode):
                    child_fd, child_identity = open_usage_directory(
                        directory_fd,
                        name,
                        identity,
                    )
                    try:
                        walk_usage_directory(
                            child_fd,
                            child_identity,
                            inside_chat_sessions=(
                                inside_chat_sessions or name == "chatSessions"
                            ),
                            session_id=session_id,
                            artifacts=artifacts,
                        )
                        child_after = FileIdentity.from_stat(os.fstat(child_fd))
                        if not child_identity.same_snapshot(child_after):
                            fail(EXIT_CHANGED, "usage-directory-changed")
                        require_stable_entry(
                            directory_fd,
                            name,
                            child_after,
                            require_single_link=False,
                        )
                    finally:
                        os.close(child_fd)
    except StateIOError:
        raise
    except OSError:
        fail(EXIT_OPERATIONAL, "usage-traversal-failed")
    current = FileIdentity.from_stat(os.fstat(directory_fd))
    if not directory_identity.same_snapshot(current):
        fail(EXIT_CHANGED, "usage-directory-changed")


def collect_usage_artifacts(
    roots: Sequence[str],
    session_id: str | None,
) -> list[UsageArtifact]:
    artifacts: list[UsageArtifact] = []
    for raw_root in roots:
        opened = open_usage_root(raw_root)
        if opened is None:
            continue
        root_fd, root_identity = opened
        try:
            walk_usage_directory(
                root_fd,
                root_identity,
                inside_chat_sessions=False,
                session_id=session_id,
                artifacts=artifacts,
            )
            root_after = FileIdentity.from_stat(os.fstat(root_fd))
            if not root_identity.same_snapshot(root_after):
                fail(EXIT_CHANGED, "usage-root-changed")
            try:
                path_after = FileIdentity.from_stat(os.stat(raw_root, follow_symlinks=False))
            except FileNotFoundError:
                fail(EXIT_CHANGED, "usage-root-disappeared")
            except OSError:
                fail(EXIT_OPERATIONAL, "usage-root-stat-failed")
            if not root_identity.same_snapshot(path_after):
                fail(EXIT_CHANGED, "usage-root-changed")
        finally:
            os.close(root_fd)
    return artifacts


def parse_usage_documents(artifact: UsageArtifact) -> list[Any]:
    try:
        if artifact.suffix == ".jsonl":
            lines = artifact.data.split(b"\n")
            if lines and lines[-1] == b"":
                lines.pop()
            return [json.loads(line) for line in lines if line.strip()]
        return [json.loads(artifact.data)]
    except (UnicodeDecodeError, json.JSONDecodeError):
        fail(EXIT_PARSE, "usage-artifact-malformed")


def iter_json_objects(value: Any) -> Any:
    if isinstance(value, dict):
        yield value
        for nested in value.values():
            yield from iter_json_objects(nested)
    elif isinstance(value, list):
        for nested in value:
            yield from iter_json_objects(nested)


def non_negative_integer(value: Any) -> bool:
    return type(value) is int and value >= 0


def normalize_usage_records(
    artifacts: Sequence[UsageArtifact],
    *,
    strict: bool = True,
) -> list[dict[str, Any]]:
    host_usage_fields = {
        "promptTokens",
        "completionTokens",
        "copilotCredits",
        "modelId",
        "promptTokenDetails",
    }
    records: list[dict[str, Any]] = []
    for artifact in artifacts:
        for document in parse_usage_documents(artifact):
            for value in iter_json_objects(document):
                if not host_usage_fields.intersection(value):
                    continue
                prompt_tokens = value.get("promptTokens")
                if not non_negative_integer(prompt_tokens):
                    if strict:
                        fail(EXIT_PARSE, "usage-prompt-tokens-invalid")
                    continue
                completion_tokens = value.get("completionTokens", 0)
                if completion_tokens is None:
                    completion_tokens = 0
                if not non_negative_integer(completion_tokens):
                    if strict:
                        fail(EXIT_PARSE, "usage-completion-tokens-invalid")
                    continue
                credits = value.get("copilotCredits", value.get("credits", 0))
                if credits is None:
                    credits = 0
                if (
                    type(credits) not in (int, float)
                    or credits < 0
                    or not math.isfinite(credits)
                ):
                    if strict:
                        fail(EXIT_PARSE, "usage-credits-invalid")
                    continue
                model = value.get("modelId", value.get("model"))
                if model is not None and not isinstance(model, str):
                    if strict:
                        fail(EXIT_PARSE, "usage-model-invalid")
                    continue
                records.append(
                    {
                        "requestId": value.get("requestId", value.get("id")),
                        "at": value.get("timestamp", value.get("requestTime")),
                        "model": model,
                        "promptTokens": prompt_tokens,
                        "completionTokens": completion_tokens,
                        "credits": credits,
                        "toolResultBytes": value.get("toolResultBytes"),
                        "compactionCheckpoints": value.get("compactionCheckpoints"),
                    }
                )
    return records


def emit_json(value: Any) -> None:
    print(json.dumps(value, ensure_ascii=True, separators=(",", ":"), sort_keys=True))


def parse_usage(args: argparse.Namespace) -> int:
    exact_projection = args.projection in ("requests", "session")
    if exact_projection and (args.session_id is None or not args.session_id):
        fail(EXIT_USAGE, "missing-usage-session-id")
    artifacts = collect_usage_artifacts(
        args.root,
        args.session_id if exact_projection else None,
    )
    if args.projection == "status":
        records = normalize_usage_records(artifacts, strict=False)
        if not records:
            emit_json(
                {
                    "adapter": "vscode-copilot",
                    "measured": False,
                    "reason": (
                        "no chatSessions artifact found; set BUBBLES_USAGE_VSCODE_ROOT"
                        if not artifacts
                        else "artifact found but carries no promptTokens field"
                    ),
                }
            )
        else:
            emit_json(
                {
                    "adapter": "vscode-copilot",
                    "files": len(artifacts),
                    "measured": True,
                    "records": len(records),
                }
            )
        return 0
    if len(artifacts) != 1:
        emit_json([] if args.projection == "requests" else {})
        return 0
    records = normalize_usage_records(artifacts)
    if args.projection == "requests":
        emit_json(records)
        return 0
    if not records:
        emit_json({})
        return 0
    prompt_tokens = [record["promptTokens"] for record in records]
    emit_json(
        {
            "artifactCount": 1,
            "completionTokens": sum(record["completionTokens"] for record in records),
            "credits": sum(record["credits"] for record in records),
            "identityMatch": "exact",
            "maxPromptTokens": max(prompt_tokens),
            "models": sorted(
                {record["model"] for record in records if record["model"] is not None}
            ),
            "promptTokens": sum(prompt_tokens),
            "requests": len(records),
            "sessionId": args.session_id,
        }
    )
    return 0


def json_string(args: argparse.Namespace) -> int:
    print(
        json.dumps(
            args.value,
            ensure_ascii=True,
            separators=(",", ":"),
        )
    )
    return 0


def run_child(command: Sequence[str]) -> int:
    if not command:
        fail(EXIT_USAGE, "missing-command")
    try:
        completed = subprocess.run(command, check=False)
    except OSError:
        fail(EXIT_OPERATIONAL, "child-exec-failed")
    return completed.returncode


def validate_timeout(raw: int) -> float:
    if raw <= 0:
        fail(EXIT_USAGE, "invalid-timeout")
    return float(raw)


def acquire_flock(fd: int, timeout_seconds: float) -> None:
    deadline = time.monotonic() + timeout_seconds
    while True:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return
        except BlockingIOError:
            if time.monotonic() >= deadline:
                fail(EXIT_TIMEOUT, "lock-timeout")
            time.sleep(0.05)
        except OSError:
            fail(EXIT_OPERATIONAL, "flock-failed")


def flock_run(args: argparse.Namespace) -> int:
    timeout_seconds = validate_timeout(args.timeout_seconds)
    parts = validate_relative_path(args.relative_lock, "relative-lock")
    root_fd = open_validated_root(args.root)
    fd = -1
    try:
        parent = walk_parent(root_fd, parts, create_missing=True)
        try:
            existing = lstat_at(parent.fd, parent.name)
            if existing is not None:
                existing_identity = FileIdentity.from_stat(existing)
                if not stat.S_ISREG(existing_identity.mode):
                    fail(EXIT_UNSAFE, "lock-not-regular")
                if existing_identity.links != 1:
                    fail(EXIT_UNSAFE, "lock-link-count")
            try:
                fd = os.open(
                    parent.name,
                    os.O_CREAT | os.O_RDWR | os.O_CLOEXEC | os.O_NOFOLLOW,
                    0o600,
                    dir_fd=parent.fd,
                )
            except OSError as error:
                if error.errno in (errno.ELOOP, errno.ENOTDIR):
                    fail(EXIT_UNSAFE, "lock-unsafe")
                fail(EXIT_OPERATIONAL, "lock-open-failed")
            opened = FileIdentity.from_stat(os.fstat(fd))
            if not stat.S_ISREG(opened.mode):
                fail(EXIT_UNSAFE, "lock-not-regular")
            if opened.links != 1:
                fail(EXIT_UNSAFE, "lock-link-count")
            if existing is not None and not existing_identity.same_object(opened):
                fail(EXIT_CHANGED, "lock-replaced-before-open")
            acquire_flock(fd, timeout_seconds)
            locked = FileIdentity.from_stat(os.fstat(fd))
            require_stable_entry(parent.fd, parent.name, locked, require_single_link=True)
            return run_child(args.command)
        finally:
            if fd >= 0:
                try:
                    fcntl.flock(fd, fcntl.LOCK_UN)
                except OSError:
                    pass
                os.close(fd)
            parent.close()
    finally:
        os.close(root_fd)


def process_is_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def read_holder(lock_fd: int) -> tuple[int, str]:
    try:
        holder_fd = os.open(
            "holder.json",
            os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=lock_fd,
        )
    except FileNotFoundError:
        fail(EXIT_CHANGED, "holder-missing")
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR):
            fail(EXIT_UNSAFE, "holder-unsafe")
        fail(EXIT_OPERATIONAL, "holder-open-failed")
    try:
        before = FileIdentity.from_stat(os.fstat(holder_fd))
        if not stat.S_ISREG(before.mode) or before.links != 1:
            fail(EXIT_UNSAFE, "holder-not-private-regular")
        require_stable_entry(lock_fd, "holder.json", before, require_single_link=True)
        chunks: list[bytes] = []
        size = 0
        while True:
            try:
                chunk = os.read(holder_fd, 4096)
            except OSError:
                fail(EXIT_OPERATIONAL, "holder-read-failed")
            if not chunk:
                break
            chunks.append(chunk)
            size += len(chunk)
            if size > 65536:
                fail(EXIT_PARSE, "holder-too-large")
        after = FileIdentity.from_stat(os.fstat(holder_fd))
        if not before.same_snapshot(after):
            fail(EXIT_CHANGED, "holder-changed")
        require_stable_entry(lock_fd, "holder.json", after, require_single_link=True)
        raw = b"".join(chunks)
    finally:
        os.close(holder_fd)
    try:
        value = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError):
        fail(EXIT_PARSE, "holder-malformed")
    if (
        not isinstance(value, dict)
        or set(value) != {"schemaVersion", "pid", "token"}
        or type(value["schemaVersion"]) is not int
        or value["schemaVersion"] != 1
        or type(value["pid"]) is not int
        or value["pid"] <= 0
        or not isinstance(value["token"], str)
        or not value["token"]
    ):
        fail(EXIT_PARSE, "holder-invalid")
    return value["pid"], value["token"]


def open_lock_directory(parent_fd: int, name: str) -> tuple[int, FileIdentity]:
    try:
        fd = os.open(
            name,
            os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=parent_fd,
        )
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR):
            fail(EXIT_UNSAFE, "mkdir-lock-unsafe")
        if error.errno == errno.ENOENT:
            fail(EXIT_CHANGED, "mkdir-lock-disappeared")
        fail(EXIT_OPERATIONAL, "mkdir-lock-open-failed")
    identity = FileIdentity.from_stat(os.fstat(fd))
    if not stat.S_ISDIR(identity.mode):
        os.close(fd)
        fail(EXIT_UNSAFE, "mkdir-lock-not-directory")
    require_stable_entry(parent_fd, name, identity, require_single_link=False)
    return fd, identity


def claim_stale_directory(parent_fd: int, name: str) -> bool:
    lock_fd, judged = open_lock_directory(parent_fd, name)
    try:
        pid, token = read_holder(lock_fd)
        if process_is_alive(pid):
            return False
        current = FileIdentity.from_stat(os.fstat(lock_fd))
        if not judged.same_snapshot(current):
            return False
        require_stable_entry(parent_fd, name, current, require_single_link=False)
        claim = f".{name}.stale.{os.getpid()}.{secrets.token_hex(8)}"
        try:
            os.rename(name, claim, src_dir_fd=parent_fd, dst_dir_fd=parent_fd)
        except FileNotFoundError:
            return False
        except OSError:
            fail(EXIT_OPERATIONAL, "stale-claim-failed")
        claim_fd, claimed = open_lock_directory(parent_fd, claim)
        try:
            if not judged.same_object(claimed):
                fail(EXIT_CHANGED, "stale-claim-identity-changed")
            claimed_pid, claimed_token = read_holder(claim_fd)
            if claimed_pid != pid or claimed_token != token:
                fail(EXIT_CHANGED, "stale-holder-changed")
            os.unlink("holder.json", dir_fd=claim_fd)
        finally:
            os.close(claim_fd)
        try:
            os.rmdir(claim, dir_fd=parent_fd)
        except OSError:
            fail(EXIT_OPERATIONAL, "stale-claim-remove-failed")
        return True
    finally:
        os.close(lock_fd)


def create_owned_lock(parent_fd: int, name: str) -> tuple[int, FileIdentity, str]:
    try:
        os.mkdir(name, mode=0o700, dir_fd=parent_fd)
    except FileExistsError:
        fail(EXIT_CHANGED, "mkdir-lock-exists")
    except OSError:
        fail(EXIT_OPERATIONAL, "mkdir-lock-create-failed")
    lock_fd, identity = open_lock_directory(parent_fd, name)
    token = secrets.token_hex(32)
    payload = json.dumps(
        {"schemaVersion": 1, "pid": os.getpid(), "token": token},
        separators=(",", ":"),
        sort_keys=True,
    ).encode("utf-8") + b"\n"
    try:
        holder_fd = os.open(
            "holder.json",
            os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC | os.O_NOFOLLOW,
            0o600,
            dir_fd=lock_fd,
        )
        try:
            write_all(holder_fd, payload, "holder-write-failed")
            os.fsync(holder_fd)
        finally:
            os.close(holder_fd)
        require_stable_entry(parent_fd, name, identity, require_single_link=False)
        holder_pid, holder_token = read_holder(lock_fd)
        if holder_pid != os.getpid() or holder_token != token:
            fail(EXIT_CHANGED, "owned-holder-mismatch")
        return lock_fd, identity, token
    except BaseException:
        try:
            os.unlink("holder.json", dir_fd=lock_fd)
        except OSError:
            pass
        os.close(lock_fd)
        try:
            os.rmdir(name, dir_fd=parent_fd)
        except OSError:
            pass
        raise


def release_owned_lock(
    parent_fd: int,
    name: str,
    lock_fd: int,
    identity: FileIdentity,
    token: str,
) -> None:
    current = FileIdentity.from_stat(os.fstat(lock_fd))
    if not identity.same_object(current):
        fail(EXIT_CHANGED, "owned-lock-identity-changed")
    require_stable_entry(parent_fd, name, current, require_single_link=False)
    holder_pid, holder_token = read_holder(lock_fd)
    if holder_pid != os.getpid() or holder_token != token:
        fail(EXIT_CHANGED, "owned-holder-changed")
    try:
        os.unlink("holder.json", dir_fd=lock_fd)
        os.close(lock_fd)
        os.rmdir(name, dir_fd=parent_fd)
    except OSError:
        fail(EXIT_OPERATIONAL, "owned-lock-release-failed")


def mkdir_run(args: argparse.Namespace) -> int:
    timeout_seconds = validate_timeout(args.timeout_seconds)
    parts = validate_relative_path(args.relative_lock, "relative-lock")
    root_fd = open_validated_root(args.root)
    parent: ParentHandle | None = None
    lock_fd = -1
    owned_identity: FileIdentity | None = None
    token = ""
    try:
        parent = walk_parent(root_fd, parts, create_missing=True)
        deadline = time.monotonic() + timeout_seconds
        while True:
            entry = lstat_at(parent.fd, parent.name)
            if entry is None:
                try:
                    lock_fd, owned_identity, token = create_owned_lock(parent.fd, parent.name)
                    break
                except StateIOError as error:
                    if error.exit_code != EXIT_CHANGED:
                        raise
            else:
                entry_identity = FileIdentity.from_stat(entry)
                if not stat.S_ISDIR(entry_identity.mode):
                    fail(EXIT_UNSAFE, "mkdir-lock-not-directory")
                try:
                    if claim_stale_directory(parent.fd, parent.name):
                        continue
                except StateIOError as error:
                    if error.exit_code != EXIT_CHANGED:
                        raise
            if time.monotonic() >= deadline:
                fail(EXIT_TIMEOUT, "lock-timeout")
            time.sleep(0.05)
        child_rc = run_child(args.command)
        if owned_identity is None:
            fail(EXIT_OPERATIONAL, "owned-lock-identity-missing")
        release_owned_lock(parent.fd, parent.name, lock_fd, owned_identity, token)
        lock_fd = -1
        return child_rc
    finally:
        if lock_fd >= 0:
            try:
                os.close(lock_fd)
            except OSError:
                pass
        if parent is not None:
            parent.close()
        os.close(root_fd)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="session-state-io.py")
    subparsers = parser.add_subparsers(dest="operation", required=True)

    capture_parser = subparsers.add_parser("capture")
    capture_parser.add_argument("--root", required=True)
    capture_parser.add_argument("--relative-path", required=True)
    capture_parser.add_argument("--destination", required=True)
    capture_parser.set_defaults(handler=capture)

    usage_parser = subparsers.add_parser("parse-usage")
    usage_parser.add_argument(
        "--projection",
        required=True,
        choices=("requests", "session", "status"),
    )
    usage_parser.add_argument("--session-id")
    usage_parser.add_argument("--root", action="append", required=True)
    usage_parser.set_defaults(handler=parse_usage)

    json_string_parser = subparsers.add_parser("json-string")
    json_string_parser.add_argument("value")
    json_string_parser.set_defaults(handler=json_string)

    for name, handler in (("flock-run", flock_run), ("mkdir-run", mkdir_run)):
        lock_parser = subparsers.add_parser(name)
        lock_parser.add_argument("--root", required=True)
        lock_parser.add_argument("--relative-lock", required=True)
        lock_parser.add_argument("--timeout-seconds", required=True, type=int)
        lock_parser.add_argument("command", nargs=argparse.REMAINDER)
        lock_parser.set_defaults(handler=handler)

    return parser


def main(argv: Sequence[str]) -> int:
    parser = build_parser()
    try:
        args = parser.parse_args(argv)
        if hasattr(args, "command"):
            if args.command and args.command[0] == "--":
                args.command = args.command[1:]
            if not args.command:
                fail(EXIT_USAGE, "missing-command")
        return int(args.handler(args))
    except StateIOError as error:
        print(f"session-state-io: status=error reason={error.reason}", file=sys.stderr)
        return error.exit_code
    except OSError:
        print(
            "session-state-io: status=error reason=unexpected-io-failure",
            file=sys.stderr,
        )
        return EXIT_OPERATIONAL


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
