#!/usr/bin/env python3
"""Durable derived lifecycle ledger for evidence-backed experience recall.

The ledger is the authority for derived recall lifecycle state. The JSONL index
written by ``experience-recall-index.py`` is a projection of the ledger over the
rebuilt corpus. Deleting a recall record writes a source-anchor tombstone and
never touches the underlying source artifact.

Closed state machine: ``admitted -> superseded | expired | deleted``. An explicit
admit action reverses a recall-only transition.

Exit codes:
    0  operation completed
    1  engine failure or malformed derived state
    2  usage error
    6  lifecycle or export refusal
"""

from __future__ import annotations

import argparse
import contextlib
import fcntl
import importlib.util
import os
import stat
import sys
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any, Dict, Iterator, List, Optional, Sequence

LOCK_NAME = "lifecycle.lock"
SCHEMA_VERSION = 1
EXPORT_MIN_LIMIT = 1
EXPORT_MAX_LIMIT = 20
EXPORT_RECORD_FIELDS = (
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
)


def load_index_module() -> Any:
    """Load the sibling indexer as a module without importing by package name."""

    module_name = "bubbles_experience_recall_index"
    existing = sys.modules.get(module_name)
    if existing is not None:
        return existing
    module_path = Path(__file__).resolve().parent / "experience-recall-index.py"
    if not module_path.is_file():
        raise SystemExit("experience-recall-lifecycle: indexer module is missing")
    spec = importlib.util.spec_from_file_location(module_name, str(module_path))
    if spec is None or spec.loader is None:
        raise SystemExit("experience-recall-lifecycle: indexer module is not loadable")
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


index = load_index_module()
RecallError = index.RecallError
LIFECYCLE_STATES = index.LIFECYCLE_STATES


class RecallRefusal(RecallError):
    """Refused lifecycle or export request that is not an engine failure."""


def refuse(code: str, message: str) -> RecallRefusal:
    return RecallRefusal(code, message)


def fail(code: str, message: str) -> RecallError:
    return RecallError(code, message)


@contextlib.contextmanager
def ledger_lock(paths: Any) -> Iterator[None]:
    runtime = paths.runtime_dir(create=True)
    lock_path = runtime / LOCK_NAME
    if lock_path.is_symlink():
        raise fail("unsafe-derived-state", "lifecycle lock path is not a contained regular file")
    descriptor = os.open(str(lock_path), os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(descriptor, fcntl.LOCK_EX)
        yield
    finally:
        os.close(descriptor)


def atomic_write_pair(
    first: Path,
    first_payload: bytes,
    second: Path,
    second_payload: bytes,
) -> None:
    """Stage both derived files, then publish them with adjacent renames."""

    staged: List[Any] = []
    try:
        for path, payload in ((first, first_payload), (second, second_payload)):
            descriptor, temporary_name = tempfile.mkstemp(
                prefix=f".{path.name}.", dir=str(path.parent)
            )
            os.fchmod(descriptor, 0o600)
            with os.fdopen(descriptor, "wb") as handle:
                handle.write(payload)
                handle.flush()
                os.fsync(handle.fileno())
            staged.append((temporary_name, path))
        for temporary_name, path in staged:
            os.replace(temporary_name, path)
        staged = []
        try:
            directory_descriptor = os.open(str(first.parent), os.O_RDONLY)
            try:
                os.fsync(directory_descriptor)
            finally:
                os.close(directory_descriptor)
        except OSError:
            pass
    finally:
        for temporary_name, _ in staged:
            try:
                os.unlink(temporary_name)
            except FileNotFoundError:
                pass


def project_state(
    paths: Any,
    repository_alias: str,
    entries: Sequence[Dict[str, Any]],
) -> Dict[str, Any]:
    """Rewrite the derived index and status from the ledger's effective states."""

    runtime = paths.runtime_dir(create=False)
    status = dict(index.load_status(paths, repository_alias))
    records = index.load_records(paths, repository_alias)
    index.apply_lifecycle_overrides(records, index.effective_lifecycle_states(entries))
    index_payload = index.serialize_index(records)
    status["lifecycleCounts"] = index.counts_by_lifecycle(records)
    status["indexDigest"] = index.digest_bytes(index_payload)
    status_payload = (index.canonical_json(status) + "\n").encode("utf-8")
    atomic_write_pair(
        runtime / index.INDEX_NAME,
        index_payload,
        runtime / index.STATUS_NAME,
        status_payload,
    )
    return status


def load_projected_records(paths: Any, repository_alias: str) -> List[Dict[str, Any]]:
    try:
        return index.load_records(paths, repository_alias)
    except RecallError as error:
        if error.code == "index-missing":
            raise fail(
                "index-missing",
                "experience recall index has not been synchronized; run recall sync first",
            ) from error
        raise


def source_digest_if_present(paths: Any, anchor: Dict[str, Any]) -> Optional[str]:
    try:
        source = paths.resolve_file(anchor["relativePath"])
    except RecallError:
        return None
    return index.digest_file(source)


def validate_transition(current: str, target: str) -> None:
    if target == "admitted":
        if current == "admitted":
            raise refuse("invalid-transition", "record is already admitted")
        return
    if current != "admitted":
        raise refuse(
            "invalid-transition",
            f"the closed state machine only transitions an admitted record; current state is {current}",
        )


def transition(
    paths: Any,
    repository_alias: str,
    record_id: str,
    target_state: str,
    reason: Optional[str],
) -> Dict[str, Any]:
    if target_state not in LIFECYCLE_STATES:
        raise refuse("invalid-state", "requested lifecycle state is outside the closed state machine")
    if index.RECORD_ID_RE.fullmatch(record_id) is None:
        raise refuse("record-missing", "record id does not match the recall record identity form")
    if reason is not None and not index.printable_text(reason, index.MAX_LEDGER_REASON_CHARS):
        raise refuse("invalid-reason", "reason must be printable text within the contract bound")

    with ledger_lock(paths):
        records = load_projected_records(paths, repository_alias)
        record = next((item for item in records if item["recordId"] == record_id), None)
        if record is None:
            raise refuse("record-missing", "record id is not present in the derived index")
        anchor = dict(record["sourceAnchor"])
        digest_before = source_digest_if_present(paths, anchor)

        entries = index.read_ledger(paths, repository_alias, create=True)
        current_entry = index.effective_lifecycle_states(entries).get(record_id)
        current_state = current_entry["state"] if current_entry else record["lifecycle"]["state"]
        validate_transition(current_state, target_state)

        transitioned_at = index.current_timestamp()
        entry = {
            "contractType": index.LEDGER_CONTRACT,
            "schemaVersion": SCHEMA_VERSION,
            "sequence": len(entries) + 1,
            "repositoryAlias": repository_alias,
            "recordId": record_id,
            "anchorKey": index.anchor_key(
                repository_alias, anchor["relativePath"], anchor["selector"]
            ),
            "sourceAnchor": anchor,
            "state": target_state,
            "transitionedAt": transitioned_at,
            "reason": reason,
        }
        index.validate_ledger_entry(entry, entry["sequence"], repository_alias)
        appended = list(entries) + [entry]

        # The ledger is written first because it is the durable authority. A
        # failure after this point leaves a loud index/status mismatch that
        # `recall sync` repairs; it never silently loses the transition.
        index.atomic_write(
            index.ledger_path(paths, create=True), index.serialize_ledger(appended)
        )
        status = project_state(paths, repository_alias, appended)

        digest_after = source_digest_if_present(paths, anchor)
        source_preserved: Optional[bool] = None
        if digest_before is not None or digest_after is not None:
            source_preserved = digest_before == digest_after

    return {
        "contractType": "lifecycle-transition",
        "schemaVersion": SCHEMA_VERSION,
        "adapter": index.PROVIDER,
        "repositoryAlias": repository_alias,
        "recordId": record_id,
        "previousState": current_state,
        "state": target_state,
        "transitionedAt": transitioned_at,
        "reason": reason,
        "sourceAnchor": anchor,
        "sourcePreserved": source_preserved,
        "ledgerPath": "/".join(index.RUNTIME_PARTS + (index.LEDGER_NAME,)),
        "ledgerEntries": len(appended),
        "lifecycleCounts": status["lifecycleCounts"],
    }


def ledger_view(paths: Any, repository_alias: str) -> Dict[str, Any]:
    entries = index.read_ledger(paths, repository_alias)
    counts = {state: 0 for state in LIFECYCLE_STATES}
    for entry in index.effective_lifecycle_states(entries).values():
        counts[entry["state"]] += 1
    return {
        "contractType": "lifecycle-ledger",
        "schemaVersion": SCHEMA_VERSION,
        "adapter": index.PROVIDER,
        "repositoryAlias": repository_alias,
        "ledgerPath": "/".join(index.RUNTIME_PARTS + (index.LEDGER_NAME,)),
        "entryCount": len(entries),
        "effectiveCounts": counts,
        "entries": entries,
    }


def resolve_output(paths: Any, value: str) -> Path:
    try:
        normalized = paths.normalize_relative(value)
    except RecallError as error:
        raise refuse(
            "unsafe-output",
            "export destination must be a contained repository-relative path",
        ) from error
    parts = PurePosixPath(normalized).parts
    cursor = paths.root
    for part in parts[:-1]:
        cursor = cursor / part
        try:
            metadata = cursor.lstat()
        except FileNotFoundError as error:
            raise refuse(
                "unsafe-output", f"export destination directory is missing: {normalized}"
            ) from error
        if stat.S_ISLNK(metadata.st_mode):
            raise refuse("unsafe-output", f"export destination traverses a symlink: {normalized}")
        if not stat.S_ISDIR(metadata.st_mode):
            raise refuse(
                "unsafe-output", f"export destination parent is not a directory: {normalized}"
            )
    destination = cursor / parts[-1]
    if destination.is_symlink():
        raise refuse("unsafe-output", f"export destination is a symlink: {normalized}")
    if destination.exists() and not stat.S_ISREG(destination.lstat().st_mode):
        raise refuse("unsafe-output", f"export destination is not a regular file: {normalized}")
    try:
        cursor.resolve(strict=True).relative_to(paths.root)
    except ValueError as error:
        raise refuse(
            "unsafe-output", f"export destination resolves outside the repository: {normalized}"
        ) from error
    return destination


def export_projection(record: Dict[str, Any]) -> Dict[str, Any]:
    """Emit an allowlisted normalized projection so no source body can leak."""

    return {field: record[field] for field in EXPORT_RECORD_FIELDS}


def export_records(
    paths: Any,
    repository_alias: str,
    limit: int,
    record_ids: Sequence[str],
    kinds: Sequence[str],
    states: Sequence[str],
    output: Optional[str],
) -> List[Dict[str, Any]]:
    if limit < EXPORT_MIN_LIMIT or limit > EXPORT_MAX_LIMIT:
        raise refuse(
            "invalid-selection",
            f"export requires an explicit limit from {EXPORT_MIN_LIMIT} through {EXPORT_MAX_LIMIT}",
        )
    if any(kind not in index.KINDS for kind in kinds):
        raise refuse("invalid-selection", "export selection contains an unknown kind")
    if any(state not in LIFECYCLE_STATES for state in states):
        raise refuse("invalid-selection", "export selection contains an unknown lifecycle state")
    if any(index.RECORD_ID_RE.fullmatch(record_id) is None for record_id in record_ids):
        raise refuse("invalid-selection", "export selection contains an invalid record id")

    destination = resolve_output(paths, output) if output is not None else None
    records = load_projected_records(paths, repository_alias)
    known_ids = {record["recordId"] for record in records}
    if any(record_id not in known_ids for record_id in record_ids):
        raise refuse(
            "record-missing", "export selection names a record that is not in the derived index"
        )

    selected = [
        record
        for record in records
        if (not record_ids or record["recordId"] in set(record_ids))
        and (not kinds or record["kind"] in set(kinds))
        and (not states or record["lifecycle"]["state"] in set(states))
    ]
    selected.sort(key=lambda record: record["recordId"])

    if any(index.transcript_like(record["sourceAnchor"]["relativePath"]) for record in selected):
        raise refuse(
            "transcript-like-export",
            "export selection names a transcript-shaped source anchor that is outside the closed corpus",
        )

    if len(selected) > limit:
        raise refuse(
            "selection-exceeds-limit",
            f"export selection contains {len(selected)} records and the explicit limit is {limit}",
        )

    projection = [export_projection(record) for record in selected]
    if destination is not None:
        index.atomic_write(destination, (index.canonical_json(projection) + "\n").encode("utf-8"))
    return projection


def argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="experience-recall-lifecycle.py")
    commands = parser.add_subparsers(dest="command", required=True)

    def common(command: argparse.ArgumentParser) -> None:
        command.add_argument("--repo-root", required=True)
        command.add_argument("--repository-alias", required=True)

    set_state = commands.add_parser("set")
    common(set_state)
    set_state.add_argument("--record-id", required=True)
    set_state.add_argument("--state", required=True)
    set_state.add_argument("--reason")

    ledger = commands.add_parser("ledger")
    common(ledger)

    export = commands.add_parser("export")
    common(export)
    export.add_argument("--limit", required=True, type=int)
    export.add_argument("--record-id", action="append", default=[])
    export.add_argument("--kind", action="append", default=[])
    export.add_argument("--state", action="append", default=[])
    export.add_argument("--output")
    return parser


def execute(arguments: argparse.Namespace) -> Any:
    paths = index.RepositoryPaths(arguments.repo_root)
    repository_alias = arguments.repository_alias
    if len(repository_alias) > 128 or index.ALIAS_RE.fullmatch(repository_alias) is None:
        raise fail("invalid-repository-alias", "repository alias does not satisfy the schema")
    if arguments.command == "set":
        return transition(
            paths,
            repository_alias,
            arguments.record_id,
            arguments.state,
            arguments.reason,
        )
    if arguments.command == "ledger":
        return ledger_view(paths, repository_alias)
    if arguments.command == "export":
        return export_records(
            paths,
            repository_alias,
            arguments.limit,
            arguments.record_id,
            arguments.kind,
            arguments.state,
            arguments.output,
        )
    raise fail("invalid-command", "unknown lifecycle command")


def main() -> int:
    parser = argument_parser()
    arguments = parser.parse_args()
    try:
        response = execute(arguments)
    except RecallRefusal as error:
        print(
            index.canonical_json(
                {
                    "contractType": "refusal",
                    "schemaVersion": SCHEMA_VERSION,
                    "adapter": index.PROVIDER,
                    "operation": arguments.command,
                    "code": error.code,
                    "reason": str(error),
                }
            )
        )
        print(f"experience-recall-lifecycle: {error.code}: {error}", file=sys.stderr)
        return 6
    except RecallError as error:
        print(f"experience-recall-lifecycle: {error.code}: {error}", file=sys.stderr)
        return 1
    except OSError as error:
        print(f"experience-recall-lifecycle: io-error: {error}", file=sys.stderr)
        return 1
    print(index.canonical_json(response))
    return 0


if __name__ == "__main__":
    sys.exit(main())
