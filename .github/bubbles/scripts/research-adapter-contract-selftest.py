#!/usr/bin/env python3
"""Shared process-boundary contract tests for repository research adapters."""
from __future__ import annotations

import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
DISABLED = ROOT / "adapters" / "research" / "disabled.sh"
LOCAL = ROOT / "adapters" / "research" / "local-command.sh"


class AdapterContractTests(unittest.TestCase):
    def run_adapter(self, script: Path, args: list[str]) -> subprocess.CompletedProcess[bytes]:
        return subprocess.run(["bash", str(script), *args], cwd="/", env={}, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5, check=False)

    def write_json(self, path: Path, value: object, mode: int = 0o600) -> None:
        path.write_text(json.dumps(value), encoding="utf-8")
        path.chmod(mode)

    def config(self, root: Path, argv: list[str], **changes: object) -> Path:
        value = {
            "contractType": "research-local-command-config",
            "schemaVersion": 1,
            "argv": argv,
            "environment": {},
            "environmentAllowlist": [],
            "maxInputBytes": 4096,
            "maxOutputBytes": 4096,
            "maxErrorBytes": 4096,
            "timeoutMs": 1000,
            "cancelFile": None,
        }
        value.update(changes)
        path = root / "adapter.json"
        self.write_json(path, value)
        return path

    def request(self, root: Path, config: Path) -> Path:
        path = root / "request.json"
        self.write_json(path, {"configPath": str(config), "input": {"contractType": "fixture", "schemaVersion": 1}})
        return path

    def candidate_argv(self, extra: dict | None = None) -> list[str]:
        candidate = {"contractType": "research-model-candidate", "schemaVersion": 1, "narrative": "candidate", "claimIds": [], "limitations": []}
        if extra:
            candidate.update(extra)
        return [sys.executable, "-c", "import json; print(json.dumps(" + repr(candidate) + "))"]

    def test_disabled_adapter_reports_unavailable_unmeasured(self) -> None:
        completed = self.run_adapter(DISABLED, [])
        self.assertEqual(completed.returncode, 0, completed.stderr)
        result = json.loads(completed.stdout)
        self.assertEqual((result["status"], result["measurement"]), ("unavailable", "unmeasured"))
        self.assertIsNone(result["candidate"])

    def test_disabled_adapter_rejects_unknown_arguments(self) -> None:
        completed = self.run_adapter(DISABLED, ["--provider", "forbidden"])
        self.assertNotEqual(completed.returncode, 0)
        self.assertNotIn(b"forbidden", completed.stdout)

    def test_local_adapter_normalizes_same_candidate_contract(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            request = self.request(root, self.config(root, self.candidate_argv()))
            completed = self.run_adapter(LOCAL, ["--input", str(request)])
        self.assertEqual(completed.returncode, 0, completed.stderr)
        result = json.loads(completed.stdout)
        self.assertEqual(result["contractType"], "research-model-candidate")
        self.assertEqual(set(result), {"contractType", "schemaVersion", "narrative", "claimIds", "limitations"})

    def test_local_adapter_does_not_invoke_a_shell(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            marker = root / "must-not-exist"
            argv = [sys.executable, "-c", "import json,sys; print(json.dumps({'contractType':'research-model-candidate','schemaVersion':1,'narrative':sys.argv[1],'claimIds':[],'limitations':[]}))", f"literal;touch {marker}"]
            request = self.request(root, self.config(root, argv))
            completed = self.run_adapter(LOCAL, ["--input", str(request)])
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertFalse(marker.exists())
            self.assertIn("literal;touch", json.loads(completed.stdout)["narrative"])

    def test_local_adapter_rejects_model_control_fields(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            request = self.request(root, self.config(root, self.candidate_argv({"tool": "shell"})))
            completed = self.run_adapter(LOCAL, ["--input", str(request)])
        self.assertNotEqual(completed.returncode, 0)
        error = json.loads(completed.stderr)
        self.assertEqual(error["code"], "RER-INPUT-INVALID")

    def test_local_adapter_rejects_world_readable_config(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = self.config(root, self.candidate_argv()); config.chmod(0o644)
            request = self.request(root, config)
            completed = self.run_adapter(LOCAL, ["--input", str(request)])
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(json.loads(completed.stderr)["code"], "RER-ROUTE-UNAVAILABLE")

    def test_local_adapter_bounds_output(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = self.config(root, [sys.executable, "-c", "print('x' * 1000)"], maxOutputBytes=10)
            request = self.request(root, config)
            completed = self.run_adapter(LOCAL, ["--input", str(request)])
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(json.loads(completed.stderr)["code"], "RER-SOURCE-LIMIT")

    def test_local_adapter_bounds_deadline(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = self.config(root, [sys.executable, "-c", "import time; time.sleep(3)"], timeoutMs=20)
            request = self.request(root, config)
            completed = self.run_adapter(LOCAL, ["--input", str(request)])
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(json.loads(completed.stderr)["errorClass"], "timeout")

    def test_adapter_output_never_contains_configured_environment_value(self) -> None:
        canary = "PRIVATE-CANARY-7d820a"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = self.config(root, self.candidate_argv(), environment={"CANARY": canary}, environmentAllowlist=["CANARY"])
            request = self.request(root, config)
            completed = self.run_adapter(LOCAL, ["--input", str(request)])
        self.assertEqual(completed.returncode, 0, completed.stderr)
        self.assertNotIn(canary.encode(), completed.stdout + completed.stderr)


if __name__ == "__main__":
    suite = unittest.defaultTestLoader.loadTestsFromModule(sys.modules[__name__])
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f"research-adapter-contract-selftest: tests={result.testsRun} failures={len(result.failures)} errors={len(result.errors)}")
    raise SystemExit(0 if result.wasSuccessful() else 1)
