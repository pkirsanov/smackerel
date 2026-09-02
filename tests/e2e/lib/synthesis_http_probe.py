#!/usr/bin/env python3
"""Bounded standard-library HTTP probe for the isolated C21 test network."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import sys
from urllib import error, parse, request


MAX_URL_BYTES = 2048
MAX_REQUEST_BODY_BYTES = 65536
MAX_RESPONSE_BODY_BYTES = 1048576
MAX_TOKEN_BYTES = 4096
MAX_TIMEOUT_SECONDS = 60.0


class ProbeInputError(ValueError):
    """Raised when a required probe input violates the closed contract."""


class ProbeArgumentParser(argparse.ArgumentParser):
    def error(self, message: str) -> None:
        raise ProbeInputError(message)


def parse_arguments(argv: list[str]) -> argparse.Namespace:
    parser = ProbeArgumentParser(add_help=True)
    parser.add_argument("--method", required=True, choices=("GET", "POST"))
    parser.add_argument("--url", required=True)
    parser.add_argument("--body", required=True)
    parser.add_argument("--auth-mode", required=True, choices=("bearer", "none"))
    parser.add_argument("--auth-token-file", required=True, type=Path)
    parser.add_argument("--timeout-seconds", required=True, type=float)
    return parser.parse_args(argv)


def validate_inputs(arguments: argparse.Namespace) -> None:
    encoded_url = arguments.url.encode("utf-8")
    encoded_body = arguments.body.encode("utf-8")
    parsed_url = parse.urlsplit(arguments.url)

    if not encoded_url or len(encoded_url) > MAX_URL_BYTES:
        raise ProbeInputError("URL length is outside the probe boundary")
    if parsed_url.scheme not in {"http", "https"} or not parsed_url.hostname:
        raise ProbeInputError("URL must identify an HTTP origin")
    if parsed_url.username is not None or parsed_url.password is not None:
        raise ProbeInputError("URL user information is forbidden")
    if len(encoded_body) > MAX_REQUEST_BODY_BYTES:
        raise ProbeInputError("request body exceeds the probe boundary")
    if arguments.method == "GET" and encoded_body:
        raise ProbeInputError("GET request body must be empty")
    if not 0 < arguments.timeout_seconds <= MAX_TIMEOUT_SECONDS:
        raise ProbeInputError("timeout is outside the probe boundary")


def read_auth_token(token_file: Path) -> str:
    try:
        token_size = token_file.stat().st_size
        token = token_file.read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise ProbeInputError("authentication token file is unreadable") from exc

    if not 0 < token_size <= MAX_TOKEN_BYTES or not token:
        raise ProbeInputError("authentication token file is invalid")
    return token


def read_response_body(response: object) -> str:
    payload = response.read(MAX_RESPONSE_BODY_BYTES + 1)
    if len(payload) > MAX_RESPONSE_BODY_BYTES:
        raise ProbeInputError("response body exceeds the probe boundary")
    return payload.decode("utf-8", errors="replace")


def perform_request(arguments: argparse.Namespace, auth_token: str) -> tuple[int, str]:
    headers = {"Accept": "application/json"}
    if arguments.auth_mode == "bearer":
        headers["Authorization"] = f"Bearer {auth_token}"

    encoded_body = arguments.body.encode("utf-8")
    request_body = encoded_body if arguments.method == "POST" else None
    if arguments.method == "POST":
        headers["Content-Type"] = "application/json"

    http_request = request.Request(
        arguments.url,
        data=request_body,
        headers=headers,
        method=arguments.method,
    )
    try:
        with request.urlopen(http_request, timeout=arguments.timeout_seconds) as response:
            return int(response.status), read_response_body(response)
    except error.HTTPError as exc:
        return int(exc.code), read_response_body(exc)


def emit_result(status: int, body: str) -> None:
    print(json.dumps({"status": status, "body": body}, separators=(",", ":")))


def main(argv: list[str]) -> int:
    try:
        arguments = parse_arguments(argv)
        validate_inputs(arguments)
        auth_token = read_auth_token(arguments.auth_token_file)
        status, body = perform_request(arguments, auth_token)
    except ProbeInputError:
        emit_result(0, "invalid probe input")
        return 2
    except (error.URLError, TimeoutError, OSError):
        emit_result(0, "HTTP transport failed")
        return 3
    except Exception:
        emit_result(0, "HTTP probe failed")
        return 4

    emit_result(status, body)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
