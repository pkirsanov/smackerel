#!/usr/bin/env python3
"""Adversarial fcntl lock and first-use tests for ECF v2."""
from __future__ import annotations

import importlib.util
import ast
import multiprocessing
import os
import tempfile
import time
from pathlib import Path
from types import ModuleType
from unittest import mock

SCRIPT = Path(__file__).with_name("execution-control-store.py")
passes = failures = 0


def load_store() -> ModuleType:
    spec = importlib.util.spec_from_file_location("execution_control_store", SCRIPT)
    if spec is None or spec.loader is None:
        raise RuntimeError("unable to load store")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


store_module = load_store()


def check(condition: bool, label: str) -> None:
    global passes, failures
    if condition:
        passes += 1
        print(f"PASS: {label}")
    else:
        failures += 1
        print(f"FAIL: {label}")


def lock_worker(root: str, entered: multiprocessing.Queue, release: multiprocessing.Event) -> None:
    module = load_store()
    with module.Store(root).locked():
        entered.put(time.monotonic())
        release.wait(2)


def test_first_use() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary) / "store"
        with store_module.Store(str(root)).locked():
            pass
        check(
            all((root / name).exists() for name in ("store.json", "events.jsonl", "head.json", "lock")),
            "SEC-ECF-01 first use creates the complete protected layout",
        )


def test_contention_and_timeout() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        root = str(Path(temporary) / "store")
        entered: multiprocessing.Queue = multiprocessing.Queue()
        release = multiprocessing.Event()
        holder = multiprocessing.Process(target=lock_worker, args=(root, entered, release))
        holder.start()
        entered.get(timeout=2)
        old_wait = store_module.LOCK_WAIT_SECONDS
        store_module.LOCK_WAIT_SECONDS = 0.05
        timed_out = False
        try:
            with store_module.Store(root).locked():
                pass
        except store_module.EcfError as exc:
            timed_out = exc.code == "ECF-LOCK-TIMEOUT"
        finally:
            store_module.LOCK_WAIT_SECONDS = old_wait
            release.set()
            holder.join(2)
        check(timed_out and holder.exitcode == 0, "SEC-ECF-08 fcntl contention fails with stable timeout")


def test_unsupported_pre_mutation() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary) / "store"
        original = store_module.fcntl
        store_module.fcntl = None
        unsupported = False
        try:
            with store_module.Store(str(root)).locked():
                pass
        except store_module.EcfError as exc:
            unsupported = exc.code == "ECF-UNSUPPORTED-LOCK"
        finally:
            store_module.fcntl = original
        check(unsupported and not root.exists(), "SEC-ECF-08 missing fcntl refuses before mutation")


def test_concurrent_creator_is_validated() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        path = Path(temporary) / "child"
        original_mkdir = os.mkdir
        raced = False

        def racing_mkdir(name: object, mode: int = 0o777, *, dir_fd: int | None = None) -> None:
            nonlocal raced
            if not raced and name == path.name and dir_fd is not None:
                raced = True
                original_mkdir(name, 0o700, dir_fd=dir_fd)
                raise FileExistsError(str(name))
            original_mkdir(name, mode, dir_fd=dir_fd)

        with mock.patch.object(os, "mkdir", racing_mkdir):
            store_module.ensure_directory(path)
        check(path.is_dir() and (path.stat().st_mode & 0o777) == 0o700, "SEC-ECF-01 concurrent valid directory winner is accepted")


def first_lock_worker(root: str, ready: multiprocessing.Queue) -> None:
    module = load_store()
    with module.Store(root).locked():
        ready.put(True)


def test_first_lock_file_race() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        root = str(Path(temporary) / "store")
        ready: multiprocessing.Queue = multiprocessing.Queue()
        workers = [multiprocessing.Process(target=first_lock_worker, args=(root, ready)) for _ in range(4)]
        for worker in workers:
            worker.start()
        for _worker in workers:
            ready.get(timeout=3)
        for worker in workers:
            worker.join(3)
        lock = Path(root) / "lock"
        check(all(worker.exitcode == 0 for worker in workers) and lock.stat().st_nlink == 1, "ECF-ES-04 first lock-file race accepts one valid create-only winner")


def test_create_only_hardlink_crash_recovery() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        parent = Path(temporary)
        target = parent / "winner"
        temporary_link = parent / ".winner.tmp.123.0123456789abcdef"
        target.write_bytes(b"winner\n")
        target.chmod(0o600)
        os.link(target, temporary_link)
        descriptor, _ = store_module.open_regular(target)
        os.close(descriptor)
        check(target.stat().st_nlink == 1 and not temporary_link.exists(), "SEC-ECF-11 normal protected-file open recovers a recognized same-inode crash link")


def test_create_only_temp_bound_and_identity() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        parent = Path(temporary)
        target = parent / "winner"
        target.write_bytes(b"winner\n")
        target.chmod(0o600)
        for index in range(store_module.MAX_CREATE_ONLY_TEMP_ENTRIES):
            candidate = parent / f".winner.tmp.{index + 1}.{index:016x}"
            candidate.write_bytes(b"other\n")
            candidate.chmod(0o600)
        descriptor, _ = store_module.open_regular(target)
        os.close(descriptor)
        check(target.stat().st_nlink == 1, "SEC-ECF-06 create-only temporary entry boundary is accepted")
        plus_one = parent / f".winner.tmp.999.{store_module.MAX_CREATE_ONLY_TEMP_ENTRIES:016x}"
        plus_one.write_bytes(b"other\n")
        plus_one.chmod(0o600)
        bounded = False
        try:
            store_module.open_regular(target)
        except store_module.EcfError as exc:
            bounded = exc.code == "ECF-LIMIT-EXCEEDED"
        check(bounded and plus_one.exists(), "SEC-ECF-06 temporary boundary plus one refuses without deleting a different inode")


def test_no_fallback_source() -> None:
    source = SCRIPT.read_text(encoding="utf-8")
    tree = ast.parse(source)
    names = {node.name for node in ast.walk(tree) if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))}
    check("mkdir_lock" not in names, "SEC-ECF-08 executable source contains no mkdir/PID fallback")


def main() -> int:
    test_first_use()
    test_contention_and_timeout()
    test_unsupported_pre_mutation()
    test_concurrent_creator_is_validated()
    test_first_lock_file_race()
    test_create_only_hardlink_crash_recovery()
    test_create_only_temp_bound_and_identity()
    test_no_fallback_source()
    print(f"execution-control-lock-selftest: {passes} passed, {failures} failed")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
