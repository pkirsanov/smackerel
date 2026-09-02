#!/usr/bin/env python3
"""Unit contract for the C21 same-network HTTP probe helper."""

from __future__ import annotations

import ast
import http.server
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import unittest


HELPER = Path(__file__).with_name("synthesis_http_probe.py")
ALLOWED_IMPORT_ROOTS = {
    "__future__",
    "argparse",
    "json",
    "pathlib",
    "sys",
    "urllib",
}


class ProbeHandler(http.server.BaseHTTPRequestHandler):
    observed_authorization: str | None = None
    observed_body = ""

    def do_GET(self) -> None:
        self._respond()

    def do_POST(self) -> None:
        content_length = int(self.headers.get("Content-Length", "0"))
        type(self).observed_body = self.rfile.read(content_length).decode("utf-8")
        self._respond()

    def _respond(self) -> None:
        type(self).observed_authorization = self.headers.get("Authorization")
        if self.path == "/http-error":
            payload = b'{"error":{"code":"EXPECTED_REJECTION"}}'
            self.send_response(422)
        else:
            payload = json.dumps(
                {
                    "method": self.command,
                    "authorizationPresent": self.headers.get("Authorization") is not None,
                    "requestBodyBytes": len(type(self).observed_body.encode("utf-8")),
                },
                separators=(",", ":"),
            ).encode("utf-8")
            self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format: str, *args: object) -> None:
        del format, args


class SynthesisHTTPProbeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ProbeHandler)
        cls.server_thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.server_thread.start()
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.server_thread.join(timeout=2)

    def setUp(self) -> None:
        ProbeHandler.observed_authorization = None
        ProbeHandler.observed_body = ""
        self.temp_dir = tempfile.TemporaryDirectory()
        self.token = "c21-unit-secret-that-must-not-be-printed"
        self.token_file = Path(self.temp_dir.name) / "token"
        self.token_file.write_text(self.token, encoding="utf-8")
        os.chmod(self.token_file, 0o600)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def run_probe(
        self,
        *,
        method: str,
        path: str,
        body: str,
        auth_mode: str,
        timeout_seconds: str = "2",
        base_url: str | None = None,
    ) -> subprocess.CompletedProcess[str]:
        probe_base_url = self.base_url if base_url is None else base_url
        return subprocess.run(
            [
                sys.executable,
                str(HELPER),
                "--method",
                method,
                "--url",
                f"{probe_base_url}{path}",
                "--body",
                body,
                "--auth-mode",
                auth_mode,
                "--auth-token-file",
                str(self.token_file),
                "--timeout-seconds",
                timeout_seconds,
            ],
            check=False,
            capture_output=True,
            text=True,
            timeout=5,
        )

    def assert_single_safe_json(self, result: subprocess.CompletedProcess[str]) -> dict[str, object]:
        self.assertNotIn(self.token, result.stdout)
        self.assertNotIn(self.token, result.stderr)
        self.assertEqual(result.stdout.count("\n"), 1)
        return json.loads(result.stdout)

    def test_uses_only_approved_standard_library_imports(self) -> None:
        tree = ast.parse(HELPER.read_text(encoding="utf-8"), filename=str(HELPER))
        import_roots = set()
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                import_roots.update(alias.name.split(".", 1)[0] for alias in node.names)
            elif isinstance(node, ast.ImportFrom) and node.module is not None:
                import_roots.add(node.module.split(".", 1)[0])
        self.assertTrue(import_roots)
        self.assertEqual(import_roots - ALLOWED_IMPORT_ROOTS, set())

    def test_bearer_post_transports_body_without_printing_token(self) -> None:
        body = '{"cadence":"daily"}'
        result = self.run_probe(method="POST", path="/ok", body=body, auth_mode="bearer")
        payload = self.assert_single_safe_json(result)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(payload["status"], 200)
        self.assertEqual(ProbeHandler.observed_authorization, f"Bearer {self.token}")
        self.assertEqual(ProbeHandler.observed_body, body)
        self.assertEqual(json.loads(str(payload["body"]))["requestBodyBytes"], len(body))

    def test_none_auth_mode_omits_authorization_header(self) -> None:
        result = self.run_probe(method="GET", path="/ok", body="", auth_mode="none")
        payload = self.assert_single_safe_json(result)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(payload["status"], 200)
        self.assertIsNone(ProbeHandler.observed_authorization)
        self.assertFalse(json.loads(str(payload["body"]))["authorizationPresent"])

    def test_http_error_preserves_status_and_body(self) -> None:
        result = self.run_probe(method="GET", path="/http-error", body="", auth_mode="bearer")
        payload = self.assert_single_safe_json(result)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(payload["status"], 422)
        self.assertEqual(payload["body"], '{"error":{"code":"EXPECTED_REJECTION"}}')

    def test_out_of_range_timeout_fails_as_one_safe_json_result(self) -> None:
        result = self.run_probe(
            method="GET",
            path="/ok",
            body="",
            auth_mode="bearer",
            timeout_seconds="0",
        )
        payload = self.assert_single_safe_json(result)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(payload, {"status": 0, "body": "invalid probe input"})

    def test_transport_failure_emits_one_safe_json_result_and_exit_three(self) -> None:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as unavailable_socket:
            unavailable_socket.bind(("127.0.0.1", 0))
            unavailable_port = unavailable_socket.getsockname()[1]
            result = self.run_probe(
                method="GET",
                path="/unavailable",
                body="",
                auth_mode="bearer",
                timeout_seconds="1",
                base_url=f"http://127.0.0.1:{unavailable_port}",
            )

        payload = self.assert_single_safe_json(result)
        self.assertEqual(result.returncode, 3)
        self.assertEqual(payload, {"status": 0, "body": "HTTP transport failed"})


if __name__ == "__main__":
    unittest.main(verbosity=2)
