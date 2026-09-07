#!/usr/bin/env python3
"""Focused end-to-end and adversarial checks for ECF v2."""
from __future__ import annotations

import hashlib
import ast
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any
from unittest import mock

SCRIPT = Path(__file__).with_name("execution-control-store.py")
LOCK_SELFTEST = Path(__file__).with_name("execution-control-lock-selftest.py")
SCHEMA = SCRIPT.parent.parent / "schemas" / "execution-control-event.schema.json"
GENESIS = "sha256:" + hashlib.sha256(b"").hexdigest()
passes = failures = 0


def load_store_module() -> Any:
    spec = importlib.util.spec_from_file_location("execution_control_store_fault_driver", SCRIPT)
    if spec is None or spec.loader is None:
        raise RuntimeError("unable to load execution-control store")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def canonical(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode() + b"\n"


def check(condition: bool, label: str) -> None:
    global passes, failures
    if condition:
        passes += 1
        print(f"PASS: {label}")
    else:
        failures += 1
        print(f"FAIL: {label}")


def run(*args: str, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run([sys.executable, str(SCRIPT), *args], capture_output=True, timeout=8, env=env, check=False)


def fault_run(point: str, target_name: str, *args: str) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        [sys.executable, str(Path(__file__)), "--fault-driver", point, target_name, *args],
        capture_output=True,
        timeout=8,
        check=False,
    )


def write(path: Path, value: Any, raw: bytes | None = None) -> None:
    path.write_bytes(raw if raw is not None else canonical(value))


def authority(path: Path, authority_id: str = "authority:checkpoint.selftest", trust_root_id: str = "trust:checkpoint.selftest", key: str = "33") -> None:
    write(path, {"contractType":"security-hmac-authority","schemaVersion":1,"purpose":"execution-control-checkpoint","authorityId":authority_id,"trustRootId":trust_root_id,"keyHex":key * 32})
    path.chmod(0o600)


def proposal(number: int, object_digest: str, event_type: str = "RECORD", supersedes: str | None = None, occurrence: int | None = None) -> dict[str, Any]:
    occurrence_number = number if occurrence is None else occurrence
    return {
        "attemptId": f"att:{number}",
        "contractType": "execution-control-event",
        "eventId": f"evt:{number}",
        "eventType": event_type,
        "extensions": [],
        "objectDigest": object_digest,
        "occurrenceId": f"occ:{occurrence_number}",
        "posture": "shadow",
        "recordedAt": f"2026-08-27T00:00:{number:02d}.000Z",
        "schemaVersion": 2,
        "subject": {"id": f"subject-{occurrence_number}", "kind": "command"},
        "supersedesEventId": supersedes,
    }


def success(result: subprocess.CompletedProcess[bytes]) -> dict[str, Any]:
    if result.returncode != 0:
        raise AssertionError(result.stderr.decode())
    return json.loads(result.stdout)


def failure(result: subprocess.CompletedProcess[bytes], code: str) -> bool:
    if result.returncode == 0 or result.stdout:
        return False
    try:
        envelope = json.loads(result.stderr)
    except json.JSONDecodeError:
        return False
    return envelope.get("code") == code and envelope.get("schemaVersion") == 2 and len(result.stderr.splitlines()) == 1


def recovery_bytes(store: Path) -> tuple[bytes, bytes, bytes]:
    ledger = (store / "events.jsonl").read_bytes()
    head = (store / "head.json").read_bytes()
    pending = (store / "pending.json").read_bytes()
    return ledger, head, pending


def retarget_pending(module: Any, store: Path, events: list[dict[str, Any]]) -> None:
    previous = GENESIS
    for index, event in enumerate(events, 1):
        event["sequence"] = index
        event["previousEventDigest"] = previous
        event["eventDigest"] = module.calculate_event_digest(event)
        previous = event["eventDigest"]
    ledger = b"".join(module.canonical_line(event) for event in events)
    pending_path = store / "pending.json"
    pending = json.loads(pending_path.read_bytes())
    pending_event = pending["event"]
    pending["expectedSequence"] = len(events)
    pending["expectedHeadDigest"] = previous
    pending["expectedLedgerOffset"] = len(ledger)
    pending["expectedPrefixDigest"] = module.digest_bytes(ledger)
    pending_event["sequence"] = len(events) + 1
    pending_event["previousEventDigest"] = previous
    pending_proposal = {key: pending_event[key] for key in module.PROPOSAL_FIELDS}
    transaction = module.calculate_transaction(len(events), previous, pending_proposal)
    pending["transactionId"] = transaction
    pending_event["transactionId"] = transaction
    pending_event["eventDigest"] = module.calculate_event_digest(pending_event)
    pending["eventLineDigest"] = module.digest_bytes(module.canonical_line(pending_event))
    write(store / "events.jsonl", {}, raw=ledger)
    write(store / "head.json", dict(module.Store.genesis_head(pending["storeId"]), sequence=len(events), eventDigest=previous))
    write(pending_path, pending)
    for name in ("events.jsonl", "head.json", "pending.json"):
        os.chmod(store / name, 0o600)


def run_fault_driver() -> int:
    point, target_name, *command = sys.argv[2:]
    module = load_store_module()

    def crash(candidate: str, path: Path) -> None:
        if candidate == "before-ledger-write" and point == "torn-ledger":
            pending = json.loads((path.parent / "pending.json").read_bytes())
            event_line = module.canonical_line(pending["event"])
            descriptor = os.open(path, os.O_WRONLY | os.O_APPEND | getattr(os, "O_NOFOLLOW", 0))
            try:
                module.write_all(descriptor, event_line[: max(1, len(event_line) // 2)])
                os.fsync(descriptor)
            finally:
                os.close(descriptor)
            os._exit(92)
        if candidate != point:
            return
        if target_name != "-" and path.name != target_name:
            return
        exit_codes = {
            "create-only-after-link": 90,
            "after-pending": 91,
            "after-ledger-write": 93,
            "after-ledger-fsync": 94,
            "after-head": 95,
        }
        os._exit(exit_codes[candidate])

    module.install_imported_test_fault_hook(crash)
    sys.argv = [str(SCRIPT), *command]
    return module.main()


if len(sys.argv) > 1 and sys.argv[1] == "--fault-driver":
    raise SystemExit(run_fault_driver())


def main() -> int:
    lock_result = subprocess.run([sys.executable, str(LOCK_SELFTEST)], capture_output=True, timeout=20, check=False)
    sys.stdout.buffer.write(lock_result.stdout)
    sys.stderr.buffer.write(lock_result.stderr)
    check(lock_result.returncode == 0, "SEC-ECF-08 fcntl-only locking and first-use races pass")
    source_tree = ast.parse(SCRIPT.read_text(encoding="utf-8"))
    executable_main_definitions = [node for node in source_tree.body if isinstance(node, ast.FunctionDef) and node.name == "main"]
    check(len(executable_main_definitions) == 1, "ECF-ES-05 active AST exposes one reachable implementation body")
    with tempfile.TemporaryDirectory() as temporary:
        work = Path(temporary)
        store = work / "store"
        object_file = work / "object.json"
        write(object_file, {"beta": 2, "alpha": 1}, raw=b'{ "beta": 2, "alpha": 1 }\n')
        result = run("object-put", "--store-root", str(store), "--input", str(object_file))
        object_digest = success(result)["objectDigest"]
        check(object_digest.startswith("sha256:"), "SEC-ECF-01 content-addressed object is stored")
        check((store.stat().st_mode & 0o777) == 0o700, "SEC-ECF-01 store directory is owner-private")
        object_path = store / "objects" / object_digest[7:9] / object_digest[7:]
        check(object_path.read_bytes() == b'{"alpha":1,"beta":2}\n', "SEC-ECF-02 object bytes use canonical JSON plus LF")
        check((object_path.stat().st_mode & 0o777) == 0o600 and object_path.stat().st_nlink == 1, "SEC-ECF-07 protected object is private and single-link")

        malformed = work / "malformed.json"
        write(malformed, {}, raw=b'{"a":1,"a":2}\n')
        check(failure(run("object-put", "--store-root", str(store), "--input", str(malformed)), "ECF-INPUT-INVALID"), "SEC-ECF-02 duplicate members produce one bounded error envelope")
        write(malformed, {}, raw=b'\xef\xbb\xbf{}\n')
        check(failure(run("object-put", "--store-root", str(store), "--input", str(malformed)), "ECF-INPUT-INVALID"), "SEC-ECF-02 UTF-8 BOM is rejected")
        write(malformed, {}, raw='{"name":"e\u0301"}\n'.encode())
        check(failure(run("object-put", "--store-root", str(store), "--input", str(malformed)), "ECF-INPUT-INVALID"), "SEC-ECF-02 decomposed Unicode is rejected")
        write(malformed, {}, raw=b'{"value":1.5}\n')
        check(failure(run("object-put", "--store-root", str(store), "--input", str(malformed)), "ECF-INPUT-INVALID"), "SEC-ECF-02 floating-point JSON is rejected")
        write(malformed, {}, raw=b'{"value":9007199254740992}\n')
        check(failure(run("object-put", "--store-root", str(store), "--input", str(malformed)), "ECF-LIMIT-EXCEEDED"), "SEC-ECF-06 interoperable integer limit is enforced")
        write(malformed, {}, raw=("[" * 40 + "0" + "]" * 40 + "\n").encode())
        nested = run("object-put", "--store-root", str(store), "--input", str(malformed))
        check(failure(nested, "ECF-LIMIT-EXCEEDED") and b"Traceback" not in nested.stderr and len(nested.stderr) <= 1024, "ECF-SEC-001 deeply nested JSON yields one bounded canonical error")

        maximum_string = work / "maximum-string.json"
        write(maximum_string, "x" * 16_384)
        check(run("object-put", "--store-root", str(store), "--input", str(maximum_string)).returncode == 0, "ECF-ES-04 maximum string-byte boundary is accepted")
        write(maximum_string, "x" * 16_385)
        check(failure(run("object-put", "--store-root", str(store), "--input", str(maximum_string)), "ECF-LIMIT-EXCEEDED"), "ECF-ES-04 maximum string-byte boundary plus one is rejected")

        event_file = work / "event.json"
        write(event_file, proposal(1, object_digest))
        first = success(run("append", "--store-root", str(store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(event_file)))
        check(first["sequence"] == 1 and first["transactionId"].startswith("txn:sha256:"), "SEC-ECF-03 append assigns sequence and deterministic transaction identity")
        retry = success(run("append", "--store-root", str(store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(event_file)))
        check(retry == first and len((store / "events.jsonl").read_bytes().splitlines()) == 1, "SEC-ECF-03 exact retry is idempotent")
        check(failure(run("append", "--store-root", str(store), "--expected-sequence", "1", "--expected-head-digest", first["eventDigest"], "--event-file", str(event_file)), "ECF-CONFLICT"), "SEC-ECF-04 duplicate physical attempt is rejected")

        write(event_file, proposal(2, object_digest, "CORRECT", "evt:1", 1))
        second = success(run("append", "--store-root", str(store), "--expected-sequence", "1", "--expected-head-digest", first["eventDigest"], "--event-file", str(event_file)))
        check(second["sequence"] == 2, "SEC-ECF-09 linear correction appends without rewriting history")
        write(event_file, proposal(3, object_digest, "CORRECT", "evt:1", 1))
        check(failure(run("append", "--store-root", str(store), "--expected-sequence", "2", "--expected-head-digest", second["eventDigest"], "--event-file", str(event_file)), "ECF-INTEGRITY"), "SEC-ECF-09 lineage branching is rejected")
        duplicate_extensions = proposal(3, object_digest)
        duplicate_extensions["extensions"] = [
            {"namespace": "org.bubbles.test", "schemaVersion": 1, "payloadDigest": object_digest},
            {"namespace": "org.bubbles.test", "schemaVersion": 2, "payloadDigest": object_digest},
        ]
        write(event_file, duplicate_extensions)
        check(failure(run("append", "--store-root", str(store), "--expected-sequence", "2", "--expected-head-digest", second["eventDigest"], "--event-file", str(event_file)), "ECF-INPUT-INVALID"), "ECF-SF-01 duplicate extension namespaces are rejected by the normative runtime validator")
        invalid_calendar = proposal(3, object_digest)
        invalid_calendar["recordedAt"] = "2026-02-30T00:00:03.000Z"
        write(event_file, invalid_calendar)
        check(failure(run("append", "--store-root", str(store), "--expected-sequence", "2", "--expected-head-digest", second["eventDigest"], "--event-file", str(event_file)), "ECF-INPUT-INVALID"), "ECF-SF-01 invalid calendar timestamp is rejected")

        supersede = proposal(3, object_digest, "SUPERSEDE", "evt:2", 1)
        write(event_file, supersede)
        third = success(run("append", "--store-root", str(store), "--expected-sequence", "2", "--expected-head-digest", second["eventDigest"], "--event-file", str(event_file)))
        write(event_file, proposal(4, object_digest, "CORRECT", "evt:3", 1))
        check(failure(run("append", "--store-root", str(store), "--expected-sequence", "3", "--expected-head-digest", third["eventDigest"], "--event-file", str(event_file)), "ECF-INTEGRITY"), "ECF-ES-04 superseded lineage cannot be corrected or revived")

        verification = success(run("verify", "--store-root", str(store)))
        check(verification["localConsistency"] is True and verification["anchorStatus"] == "not-provided" and verification["rollbackResistance"] == "not-evaluated", "SEC-ECF-10 local verification does not claim rollback resistance")
        checkpoint_result = success(run("checkpoint", "--store-root", str(store)))
        checkpoint = work / "checkpoint.json"
        write(checkpoint, checkpoint_result)
        anchored = success(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint)))
        check(anchored["anchorStatus"] == "local-match" and anchored["rollbackResistance"] == "not-evaluated", "SEC-ECF-10 matching digest-only checkpoint proves only a local match")
        check(failure(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint), "--require-rollback-resistance"), "ECF-ROLLBACK-NOT-EVALUATED"), "SEC-08 required rollback resistance fails closed without configured authority")
        checkpoint_key = work / "checkpoint-authority.json"
        authority(checkpoint_key)
        authenticated_checkpoint = success(run("checkpoint", "--store-root", str(store), "--checkpoint-authority", str(checkpoint_key)))
        write(checkpoint, authenticated_checkpoint)
        authenticated = success(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint), "--checkpoint-authority", str(checkpoint_key), "--require-rollback-resistance"))
        check(authenticated["anchorStatus"] == "authenticated" and authenticated["rollbackResistance"] == "satisfied", "SEC-08 independently retained authenticated fresh checkpoint establishes rollback resistance")
        forged = dict(authenticated_checkpoint, authenticator="hmac-sha256:" + "0" * 64)
        write(checkpoint, forged)
        check(failure(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint), "--checkpoint-authority", str(checkpoint_key)), "ECF-AUTHORITY-INVALID"), "SEC-08 forged checkpoint authenticator is rejected")
        wrong_key = work / "wrong-checkpoint-authority.json"
        authority(wrong_key, "authority:wrong", "trust:wrong", "44")
        write(checkpoint, authenticated_checkpoint)
        check(failure(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint), "--checkpoint-authority", str(wrong_key)), "ECF-AUTHORITY-INVALID"), "SEC-08 untrusted checkpoint authority is rejected")
        for descendant in (store / "unrecognized" / "nested", store / "projections", store / "objects"):
            descendant.mkdir(mode=0o700, parents=True, exist_ok=True)
            internal_checkpoint = descendant / "checkpoint.json"
            write(internal_checkpoint, checkpoint_result)
            contained = run("verify", "--store-root", str(store), "--checkpoint", str(internal_checkpoint))
            check(failure(contained, "ECF-INTEGRITY"), f"SEC-ECF-10 checkpoint inside {descendant.relative_to(store)} cannot verify")
            internal_checkpoint.unlink()
            if descendant.name == "nested":
                descendant.rmdir()
                descendant.parent.rmdir()
        checkpoint_result["sequence"] = 1
        write(checkpoint, checkpoint_result)
        check(failure(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint)), "ECF-ANCHOR-MISMATCH"), "SEC-ECF-10 stale checkpoint is rejected")
        write(checkpoint, {"contractType": "execution-control-checkpoint"})
        check(failure(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint)), "ECF-INPUT-INVALID"), "ECF-ES-04 malformed checkpoint has stable facade rejection")
        valid_checkpoint = success(run("checkpoint", "--store-root", str(store)))
        malformed_checkpoints = (
            ("contractType", "other"),
            ("schemaVersion", 1),
            ("storeId", "bad store id"),
            ("sequence", -1),
            ("sequence", True),
            ("sequence", 10_001),
            ("eventDigest", "sha256:ABC"),
            ("canonicalProfile", "other"),
        )
        for field, bad_value in malformed_checkpoints:
            malformed_checkpoint = dict(valid_checkpoint)
            malformed_checkpoint[field] = bad_value
            write(checkpoint, malformed_checkpoint)
            rejected = run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint))
            check(failure(rejected, "ECF-INPUT-INVALID"), f"ECF-SEC-15 malformed checkpoint {field} is normatively rejected")
        extra_checkpoint = dict(valid_checkpoint, extra=True)
        write(checkpoint, extra_checkpoint)
        check(failure(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint)), "ECF-INPUT-INVALID"), "ECF-SEC-15 checkpoint exact-field closure is enforced")
        other_store_checkpoint = dict(valid_checkpoint, storeId="store:other")
        write(checkpoint, other_store_checkpoint)
        check(failure(run("verify", "--store-root", str(store), "--checkpoint", str(checkpoint)), "ECF-ANCHOR-MISMATCH"), "ECF-SEC-15 valid checkpoint for another store is an anchor mismatch")

        projection = success(run("project", "--store-root", str(store)))
        managed = store / "projections" / "integrity.json"
        check(managed.exists() and json.loads(managed.read_bytes()) == projection, "SEC-ECF-05 projection writes only its managed destination")
        check("subject-1" not in managed.read_text(), "SEC-ECF-05 projection excludes event identities")

        for index, point in enumerate(("after-pending", "torn-ledger", "after-ledger-write", "after-ledger-fsync", "after-head"), 10):
            crash_store = work / f"crash-store-{index}"
            crash_object = success(run("object-put", "--store-root", str(crash_store), "--input", str(object_file)))["objectDigest"]
            crash_event = work / f"crash-{index}.json"
            write(crash_event, proposal(index, crash_object))
            crashed = fault_run(point, "-", "append", "--store-root", str(crash_store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(crash_event))
            recovered = run("verify", "--store-root", str(crash_store))
            check(crashed.returncode in (91, 92, 93, 94, 95) and recovered.returncode == 0, f"SEC-ECF-11 crash recovery closes {point}")

        create_only_targets = ("lock", "store.json", "events.jsonl", "head.json")
        for index, target_name in enumerate(create_only_targets, 30):
            crash_store = work / f"create-crash-{index}"
            crashed = fault_run("create-only-after-link", target_name, "object-put", "--store-root", str(crash_store), "--input", str(object_file))
            reopened = run("object-put", "--store-root", str(crash_store), "--input", str(object_file))
            target = crash_store / target_name
            check(crashed.returncode == 90 and reopened.returncode == 0 and target.stat().st_nlink == 1, f"SEC-ECF-11 normal reopen converges after {target_name} link-publication crash")
        object_crash_store = work / "object-create-crash"
        expected_object_name = object_digest[7:]
        object_crashed = fault_run("create-only-after-link", expected_object_name, "object-put", "--store-root", str(object_crash_store), "--input", str(object_file))
        object_reopened = run("object-put", "--store-root", str(object_crash_store), "--input", str(object_file))
        crashed_object_path = object_crash_store / "objects" / expected_object_name[:2] / expected_object_name
        check(object_crashed.returncode == 90 and object_reopened.returncode == 0 and crashed_object_path.stat().st_nlink == 1, "SEC-ECF-11 normal object-put converges after object link-publication crash")

        pending_crash_store = work / "pending-create-crash"
        pending_object = success(run("object-put", "--store-root", str(pending_crash_store), "--input", str(object_file)))["objectDigest"]
        pending_event = work / "pending-create-event.json"
        write(pending_event, proposal(50, pending_object))
        pending_crashed = fault_run("create-only-after-link", "pending.json", "append", "--store-root", str(pending_crash_store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(pending_event))
        pending_reopened = run("append", "--store-root", str(pending_crash_store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(pending_event))
        check(pending_crashed.returncode == 90 and pending_reopened.returncode == 0 and not (pending_crash_store / "pending.json").exists(), "SEC-ECF-11 normal append converges after pending link-publication crash")

        store_module = load_store_module()
        for case in ("transaction", "sequence", "prefix", "line", "head"):
            malformed_store = work / f"malformed-pending-{case}"
            malformed_object = success(run("object-put", "--store-root", str(malformed_store), "--input", str(object_file)))["objectDigest"]
            malformed_event = work / f"malformed-pending-{case}.json"
            write(malformed_event, proposal(21, malformed_object))
            fault_run("after-pending", "-", "append", "--store-root", str(malformed_store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(malformed_event))
            pending_path = malformed_store / "pending.json"
            pending_value = json.loads(pending_path.read_bytes())
            if case == "transaction":
                pending_value["transactionId"] = "txn:sha256:" + "1" * 64
                pending_value["event"]["transactionId"] = pending_value["transactionId"]
                pending_value["event"]["eventDigest"] = store_module.calculate_event_digest(pending_value["event"])
                pending_value["eventLineDigest"] = store_module.digest_bytes(store_module.canonical_line(pending_value["event"]))
            elif case == "sequence":
                pending_value["expectedSequence"] = 1
            elif case == "prefix":
                pending_value["expectedPrefixDigest"] = "sha256:" + "2" * 64
            elif case == "line":
                pending_value["eventLineDigest"] = "sha256:" + "3" * 64
            else:
                head_value = json.loads((malformed_store / "head.json").read_bytes())
                head_value["eventDigest"] = "sha256:" + "4" * 64
                write(malformed_store / "head.json", head_value)
                os.chmod(malformed_store / "head.json", 0o600)
            write(pending_path, pending_value)
            os.chmod(pending_path, 0o600)
            ledger_before = (malformed_store / "events.jsonl").read_bytes()
            head_before = (malformed_store / "head.json").read_bytes()
            rejected = run("verify", "--store-root", str(malformed_store))
            unchanged = ledger_before == (malformed_store / "events.jsonl").read_bytes() and head_before == (malformed_store / "head.json").read_bytes()
            check(rejected.returncode in (4, 5) and unchanged, f"ECF-SEC-13 malformed pending {case} refuses before ledger/head mutation")

        def pending_over_two_events(name: str) -> tuple[Path, str]:
            recovery_store = work / name
            recovery_digest = success(run("object-put", "--store-root", str(recovery_store), "--input", str(object_file)))["objectDigest"]
            for number in (10, 11):
                write(event_file, proposal(number, recovery_digest))
                current_head = json.loads((recovery_store / "head.json").read_bytes())
                success(run("append", "--store-root", str(recovery_store), "--expected-sequence", str(current_head["sequence"]), "--expected-head-digest", current_head["eventDigest"], "--event-file", str(event_file)))
            write(event_file, proposal(12, recovery_digest))
            current_head = json.loads((recovery_store / "head.json").read_bytes())
            crashed = fault_run("after-pending", "-", "append", "--store-root", str(recovery_store), "--expected-sequence", str(current_head["sequence"]), "--expected-head-digest", current_head["eventDigest"], "--event-file", str(event_file))
            check(crashed.returncode == 91, f"ECF-SEC-16 {name} fixture reaches production pending recovery path")
            return recovery_store, recovery_digest

        for identity_field in ("eventId", "attemptId", "transactionId"):
            identity_store, _ = pending_over_two_events(f"recovery-duplicate-{identity_field}")
            identity_events = [json.loads(line) for line in (identity_store / "events.jsonl").read_bytes().splitlines()]
            identity_events[1][identity_field] = identity_events[0][identity_field]
            retarget_pending(store_module, identity_store, identity_events)
            before = recovery_bytes(identity_store)
            rejected = run("verify", "--store-root", str(identity_store))
            check(failure(rejected, "ECF-INTEGRITY") and recovery_bytes(identity_store) == before, f"ECF-SEC-16 duplicate committed {identity_field} refuses recovery without changing ledger/head/pending bytes")

        lineage_store, _ = pending_over_two_events("recovery-invalid-lineage")
        lineage_events = [json.loads(line) for line in (lineage_store / "events.jsonl").read_bytes().splitlines()]
        lineage_events[1]["eventType"] = "CORRECT"
        lineage_events[1]["supersedesEventId"] = "evt:unknown"
        retarget_pending(store_module, lineage_store, lineage_events)
        lineage_before = recovery_bytes(lineage_store)
        lineage_rejected = run("verify", "--store-root", str(lineage_store))
        check(failure(lineage_rejected, "ECF-INTEGRITY") and recovery_bytes(lineage_store) == lineage_before, "ECF-SEC-16 invalid committed lineage refuses recovery without changing ledger/head/pending bytes")

        for object_case in ("missing-object", "corrupt-object"):
            object_store, referenced_digest = pending_over_two_events(f"recovery-{object_case}")
            referenced_path = object_store / "objects" / referenced_digest[7:9] / referenced_digest[7:]
            if object_case == "missing-object":
                referenced_path.unlink()
            else:
                write(referenced_path, {"corrupt": True})
                os.chmod(referenced_path, 0o600)
            object_before = recovery_bytes(object_store)
            object_rejected = run("verify", "--store-root", str(object_store))
            check(object_rejected.returncode != 0 and recovery_bytes(object_store) == object_before, f"ECF-SEC-16 {object_case} in committed prefix refuses recovery without changing ledger/head/pending bytes")

        extension_file = work / "extension.json"
        write(extension_file, {"extension": "payload"})
        for extension_case in ("missing", "corrupt"):
            extension_store = work / f"recovery-{extension_case}-extension"
            primary_digest = success(run("object-put", "--store-root", str(extension_store), "--input", str(object_file)))["objectDigest"]
            extension_digest = success(run("object-put", "--store-root", str(extension_store), "--input", str(extension_file)))["objectDigest"]
            extension_event = proposal(13, primary_digest)
            extension_event["extensions"] = [{"namespace": "org.bubbles.recovery", "schemaVersion": 1, "payloadDigest": extension_digest}]
            write(event_file, extension_event)
            first_extension_event = success(run("append", "--store-root", str(extension_store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(event_file)))
            write(event_file, proposal(14, primary_digest))
            extension_crash = fault_run("after-pending", "-", "append", "--store-root", str(extension_store), "--expected-sequence", "1", "--expected-head-digest", first_extension_event["eventDigest"], "--event-file", str(event_file))
            extension_path = extension_store / "objects" / extension_digest[7:9] / extension_digest[7:]
            if extension_case == "missing":
                extension_path.unlink()
            else:
                write(extension_path, {"corrupt": "extension"})
                os.chmod(extension_path, 0o600)
            extension_before = recovery_bytes(extension_store)
            extension_rejected = run("verify", "--store-root", str(extension_store))
            check(extension_crash.returncode == 91 and extension_rejected.returncode != 0 and recovery_bytes(extension_store) == extension_before, f"ECF-SEC-16 {extension_case} committed extension object refuses recovery without changing ledger/head/pending bytes")

        convergence_store = work / "recovery-valid-convergence"
        convergence_digest = success(run("object-put", "--store-root", str(convergence_store), "--input", str(object_file)))["objectDigest"]
        write(event_file, proposal(15, convergence_digest))
        convergence_crash = fault_run("after-pending", "-", "append", "--store-root", str(convergence_store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(event_file))
        convergence = run("verify", "--store-root", str(convergence_store))
        convergence_head = json.loads((convergence_store / "head.json").read_bytes())
        check(convergence_crash.returncode == 91 and success(convergence)["eventCount"] == 1 and convergence_head["sequence"] == 1 and not (convergence_store / "pending.json").exists(), "ECF-SEC-16 valid pending recovery converges through the production verify path")

        scan_store = work / "single-scan-store"
        scan_instance = store_module.Store(str(scan_store))
        with scan_instance.locked():
            with mock.patch.object(scan_instance, "raw_events", wraps=scan_instance.raw_events) as raw_scan, mock.patch.object(scan_instance, "verify_objects", wraps=scan_instance.verify_objects) as object_scan:
                scan_object = scan_instance.put_object({"single": "scan"})["objectDigest"]
            check(raw_scan.call_count == 1 and object_scan.call_count == 1, "CR-006 object-put reuses one locked ledger/object verification result")
            with mock.patch.object(scan_instance, "raw_events", wraps=scan_instance.raw_events) as raw_scan, mock.patch.object(scan_instance, "verify_objects", wraps=scan_instance.verify_objects) as object_scan:
                scan_event = scan_instance.append(0, GENESIS, proposal(16, scan_object))
            check(raw_scan.call_count == 1 and object_scan.call_count == 1 and scan_event["sequence"] == 1, "CR-006 append reuses one locked ledger/object verification result")
            with mock.patch.object(scan_instance, "raw_events", wraps=scan_instance.raw_events) as raw_scan, mock.patch.object(scan_instance, "verify_objects", wraps=scan_instance.verify_objects) as object_scan:
                scan_projection = scan_instance.project()
            check(raw_scan.call_count == 1 and object_scan.call_count == 1 and scan_projection["eventCount"] == 1, "CR-006 project reuses one locked ledger/object verification result")

        prefix_bound_store = work / "prefix-bound-store"
        success(run("object-put", "--store-root", str(prefix_bound_store), "--input", str(object_file)))
        existing_prefixes = {path.name for path in (prefix_bound_store / "objects").iterdir()}
        for value in range(256):
            name = f"{value:02x}"
            if name not in existing_prefixes:
                (prefix_bound_store / "objects" / name).mkdir(mode=0o700)
        check(run("verify", "--store-root", str(prefix_bound_store)).returncode == 0, "SEC-ECF-06 object prefix boundary is accepted")
        (prefix_bound_store / "objects" / "plus-one").mkdir(mode=0o700)
        check(failure(run("verify", "--store-root", str(prefix_bound_store)), "ECF-LIMIT-EXCEEDED"), "SEC-ECF-06 object prefix boundary plus one is rejected before materialization")

        adversary_store = work / "object-adversary-store"
        adversary_digest = success(run("object-put", "--store-root", str(adversary_store), "--input", str(object_file)))["objectDigest"]
        adversary_path = adversary_store / "objects" / adversary_digest[7:9] / adversary_digest[7:]
        adversary_path.unlink()
        adversary_path.symlink_to(object_file)
        check(failure(run("verify", "--store-root", str(adversary_store)), "ECF-INTEGRITY"), "SEC-ECF-09 descriptor-relative object traversal rejects a symlink file")

        legacy = work / "legacy"
        legacy.mkdir(mode=0o700)
        write(legacy / "store.json", {"canonicalProfile": "bubbles-canonical-json-v1", "contractType": "execution-control-store", "limitsProfile": "execution-control-limits-v1", "schemaVersion": 1, "storeId": "store:legacy"})
        os.chmod(legacy / "store.json", 0o600)
        check(failure(run("verify", "--store-root", str(legacy)), "ECF-INTEGRITY"), "SEC-ECF-03 v1 store bytes require explicit migration")

        ambient_store = work / "ambient-fault-store"
        ambient_digest = success(run("object-put", "--store-root", str(ambient_store), "--input", str(object_file))) ["objectDigest"]
        ambient_event = work / "ambient-event.json"
        write(ambient_event, proposal(20, ambient_digest))
        ambient_environment = dict(os.environ, BUBBLES_ECF_TEST_MODE="ecf-v2-selftest", BUBBLES_ECF_TEST_FAULT="after-pending")
        ambient = run("append", "--store-root", str(ambient_store), "--expected-sequence", "0", "--expected-head-digest", GENESIS, "--event-file", str(ambient_event), env=ambient_environment)
        source_text = SCRIPT.read_text(encoding="utf-8")
        check(ambient.returncode == 0 and "BUBBLES_ECF_TEST_FAULT" not in source_text and "BUBBLES_ECF_TEST_MODE" not in source_text and "os._exit" not in source_text, "ECF-SEC-12 ordinary CLI argv/environment cannot activate an imported-only fault hook")

        schema = json.loads(SCHEMA.read_bytes())
        check(bool(schema.get("allOf")) and "normative" in schema.get("description", ""), "ECF-SF-01 Draft-07 dependency and normative-runtime boundary are explicit")

    print(f"execution-control-v2-selftest: {passes} passed, {failures} failed")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
