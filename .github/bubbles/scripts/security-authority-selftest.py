#!/usr/bin/env python3
"""Adversarial checks for the descriptor-bound HMAC authority helper."""
from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("security_authority_under_test", HERE / "security-authority.py")
assert SPEC is not None and SPEC.loader is not None
security = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(security)


class SecurityAuthorityTests(unittest.TestCase):
    def write_private(self, path: Path, value: object) -> None:
        path.write_text(json.dumps(value), encoding="utf-8")
        path.chmod(0o600)

    def authority(self, path: Path, key: str = "11", purpose: str = "dispatch-proof") -> None:
        self.write_private(path, {
            "contractType": "security-hmac-authority",
            "schemaVersion": 1,
            "purpose": purpose,
            "authorityId": "authority:selftest",
            "trustRootId": "trust:selftest",
            "keyHex": key * 32,
        })

    def payload(self) -> dict[str, object]:
        return {
            "actionDigest": "sha256:" + "1" * 64,
            "repositoryDecisionId": "rb:selftest:1",
            "repositoryRoot": "/repository/one",
            "sessionId": "session:one",
        }

    def test_authenticator_is_bound_to_key_action_repository_and_session(self) -> None:
        with tempfile.TemporaryDirectory() as root:
            authority_path = Path(root) / "authority.json"
            wrong_key_path = Path(root) / "wrong-key.json"
            self.authority(authority_path)
            self.authority(wrong_key_path, key="22")
            authority = security.load(str(authority_path), "dispatch-proof")
            payload = self.payload()
            authenticator = security.authenticator(authority, payload)
            self.assertTrue(security.verify(authority, payload, authenticator))
            self.assertFalse(security.verify(security.load(str(wrong_key_path), "dispatch-proof"), payload, authenticator))
            for field, value in (
                ("actionDigest", "sha256:" + "2" * 64),
                ("repositoryDecisionId", "rb:selftest:2"),
                ("repositoryRoot", "/repository/two"),
                ("sessionId", "session:two"),
            ):
                changed = dict(payload)
                changed[field] = value
                self.assertFalse(security.verify(authority, changed, authenticator), field)

    def test_purpose_and_private_file_contract_fail_closed(self) -> None:
        with tempfile.TemporaryDirectory() as root:
            authority_path = Path(root) / "authority.json"
            self.authority(authority_path)
            with self.assertRaisesRegex(security.AuthorityError, "contract mismatch"):
                security.load(str(authority_path), "other-purpose")
            authority_path.chmod(0o644)
            with self.assertRaisesRegex(security.AuthorityError, "owner-private"):
                security.load(str(authority_path), "dispatch-proof")

    def test_final_and_intermediate_symlinks_are_refused(self) -> None:
        with tempfile.TemporaryDirectory() as root:
            root_path = Path(root)
            authority_path = root_path / "authority.json"
            self.authority(authority_path)
            final_alias = root_path / "authority-alias.json"
            final_alias.symlink_to(authority_path)
            with self.assertRaisesRegex(security.AuthorityError, "symlinks"):
                security.load(str(final_alias), "dispatch-proof")
            real_directory = root_path / "real"
            real_directory.mkdir()
            nested_authority = real_directory / "authority.json"
            self.authority(nested_authority)
            directory_alias = root_path / "alias"
            directory_alias.symlink_to(real_directory, target_is_directory=True)
            with self.assertRaisesRegex(security.AuthorityError, "symlinks"):
                security.load(str(directory_alias / "authority.json"), "dispatch-proof")

    def test_open_descriptor_remains_bound_during_atomic_path_replacement(self) -> None:
        with tempfile.TemporaryDirectory() as root:
            authority_path = Path(root) / "authority.json"
            replacement_path = Path(root) / "replacement.json"
            self.authority(authority_path, key="11")
            self.authority(replacement_path, key="22")
            original_read = os.read
            replaced = False

            def replace_then_read(descriptor: int, size: int) -> bytes:
                nonlocal replaced
                if not replaced:
                    os.replace(replacement_path, authority_path)
                    replaced = True
                return original_read(descriptor, size)

            with mock.patch.object(security.os, "read", side_effect=replace_then_read):
                loaded = security.load(str(authority_path), "dispatch-proof")
            self.assertEqual(loaded["keyHex"], "11" * 32)
            self.assertEqual(security.load(str(authority_path), "dispatch-proof")["keyHex"], "22" * 32)

    def test_cli_verify_rejects_replay_in_changed_context(self) -> None:
        with tempfile.TemporaryDirectory() as root:
            root_path = Path(root)
            authority_path = root_path / "authority.json"
            payload_path = root_path / "payload.json"
            self.authority(authority_path)
            payload = self.payload()
            self.write_private(payload_path, payload)
            signed = subprocess.run(
                [sys.executable, str(HERE / "security-authority.py"), "sign", "dispatch-proof", str(authority_path), str(payload_path)],
                capture_output=True, text=True, timeout=5, check=False,
            )
            self.assertEqual(signed.returncode, 0, signed.stderr)
            authenticated = dict(payload, authenticator=json.loads(signed.stdout)["authenticator"])
            self.write_private(payload_path, authenticated)
            verified = subprocess.run(
                [sys.executable, str(HERE / "security-authority.py"), "verify", "dispatch-proof", str(authority_path), str(payload_path)],
                capture_output=True, text=True, timeout=5, check=False,
            )
            self.assertEqual(verified.returncode, 0, verified.stderr)
            authenticated["sessionId"] = "session:replayed"
            self.write_private(payload_path, authenticated)
            replay = subprocess.run(
                [sys.executable, str(HERE / "security-authority.py"), "verify", "dispatch-proof", str(authority_path), str(payload_path)],
                capture_output=True, text=True, timeout=5, check=False,
            )
            self.assertEqual(replay.returncode, 4)
            self.assertIn("authenticator is invalid", replay.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)