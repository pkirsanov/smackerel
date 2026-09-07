#!/usr/bin/env python3
"""Small provider-free HMAC authority used by hermetic security contracts.

The authority is opt-in. Its private JSON configuration is retained outside the
protected payload/store and must be an owner-private, single-link regular file.
"""
from __future__ import annotations

import hashlib
import hmac
import json
import os
import stat
import sys
from pathlib import Path
from typing import Any


class AuthorityError(ValueError):
    pass


def canonical(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def private_bytes(path_text: str, label: str, maximum_bytes: int = 16_384) -> bytes:
    path = Path(path_text)
    if not path.is_absolute() or ".." in path.parts:
        raise AuthorityError(f"{label} path must be lexical absolute")
    nofollow = getattr(os, "O_NOFOLLOW", 0)
    cloexec = getattr(os, "O_CLOEXEC", 0)
    directory = getattr(os, "O_DIRECTORY", 0)
    descriptor = os.open(path.anchor, os.O_RDONLY | directory | cloexec)
    try:
        for part in path.parts[1:-1]:
            next_descriptor = os.open(
                part, os.O_RDONLY | directory | nofollow | cloexec,
                dir_fd=descriptor,
            )
            os.close(descriptor)
            descriptor = next_descriptor
        file_descriptor = os.open(
            path.name, os.O_RDONLY | nofollow | cloexec,
            dir_fd=descriptor,
        )
    except OSError as exc:
        raise AuthorityError(f"{label} path cannot contain symlinks and must be readable") from exc
    finally:
        os.close(descriptor)
    try:
        metadata = os.fstat(file_descriptor)
        if (not stat.S_ISREG(metadata.st_mode) or metadata.st_uid != os.geteuid()
                or stat.S_IMODE(metadata.st_mode) != 0o600 or metadata.st_nlink != 1):
            raise AuthorityError(f"{label} must be an owner-private single-link regular file")
        chunks: list[bytes] = []
        size = 0
        while size <= maximum_bytes:
            chunk = os.read(file_descriptor, min(65_536, maximum_bytes + 1 - size))
            if not chunk:
                break
            chunks.append(chunk)
            size += len(chunk)
        if size > maximum_bytes:
            raise AuthorityError(f"{label} exceeds {maximum_bytes} bytes")
        return b"".join(chunks)
    finally:
        os.close(file_descriptor)


def load(path_text: str, purpose: str) -> dict[str, str]:
    data = private_bytes(path_text, f"{purpose} authority")
    try:
        value = json.loads(data)
    except (json.JSONDecodeError, UnicodeError) as exc:
        raise AuthorityError(f"{purpose} authority is malformed JSON") from exc
    fields = {"contractType", "schemaVersion", "purpose", "authorityId", "trustRootId", "keyHex"}
    if not isinstance(value, dict) or set(value) != fields:
        raise AuthorityError(f"{purpose} authority fields are not closed")
    if value["contractType"] != "security-hmac-authority" or value["schemaVersion"] != 1 or value["purpose"] != purpose:
        raise AuthorityError(f"{purpose} authority contract mismatch")
    for field in ("authorityId", "trustRootId"):
        if not isinstance(value[field], str) or not value[field] or len(value[field]) > 128 or not value[field].isascii():
            raise AuthorityError(f"{purpose} authority identity is invalid")
    key_hex = value["keyHex"]
    if not isinstance(key_hex, str) or len(key_hex) < 64 or len(key_hex) > 256:
        raise AuthorityError(f"{purpose} authority key is invalid")
    try:
        bytes.fromhex(key_hex)
    except ValueError as exc:
        raise AuthorityError(f"{purpose} authority key is invalid") from exc
    return value


def authenticator(authority: dict[str, str], payload: Any) -> str:
    key = bytes.fromhex(authority["keyHex"])
    return "hmac-sha256:" + hmac.new(key, canonical(payload), hashlib.sha256).hexdigest()


def verify(authority: dict[str, str], payload: Any, supplied: Any) -> bool:
    return isinstance(supplied, str) and hmac.compare_digest(authenticator(authority, payload), supplied)


def main(argv: list[str]) -> int:
    if len(argv) != 5 or argv[1] not in {"sign", "verify"}:
        print("usage: security-authority.py sign|verify PURPOSE AUTHORITY.json PAYLOAD.json", file=sys.stderr)
        return 2
    operation, purpose, authority_path, payload_path = argv[1:]
    try:
        authority = load(authority_path, purpose)
        payload = json.loads(private_bytes(payload_path, "security payload"))
        if operation == "sign":
            print(json.dumps({"authorityId": authority["authorityId"], "trustRootId": authority["trustRootId"],
                              "authenticator": authenticator(authority, payload)}, sort_keys=True, separators=(",", ":")))
            return 0
        supplied = payload.pop("authenticator", None) if isinstance(payload, dict) else None
        if not verify(authority, payload, supplied):
            raise AuthorityError(f"{purpose} authenticator is invalid")
        print(json.dumps({"authorityId": authority["authorityId"], "trustRootId": authority["trustRootId"],
                          "verdict": "valid"}, sort_keys=True, separators=(",", ":")))
        return 0
    except (AuthorityError, OSError, json.JSONDecodeError) as exc:
        print(f"security-authority: {exc}", file=sys.stderr)
        return 4


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
