#!/usr/bin/env python3
"""Bounded semantic classifier for G028 sensitive client storage operations."""

from __future__ import annotations

import ast
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path, PurePosixPath
from typing import Iterable, Optional


ALLOWED_CONFIG_FIELDS = {
    "path",
    "storage",
    "key",
    "provider",
    "credentialClass",
    "privilege",
    "lifetime",
}
PERSIST_METHODS = {
    "setItem",
    "multiSet",
    "setString",
    "setStringList",
    "setInt",
    "setBool",
    "setDouble",
    "putString",
    "putStringSet",
    "putInt",
    "putBoolean",
    "putFloat",
    "putLong",
    "set",
    "put",
    "add",
}
READ_METHODS = {
    "getItem",
    "multiGet",
    "getString",
    "getStringList",
    "getInt",
    "getBool",
    "getDouble",
    "getStringSet",
    "getBoolean",
    "getFloat",
    "getLong",
    "get",
    "open",
}
CLEANUP_METHODS = {"removeItem", "multiRemove", "remove", "delete", "clear"}
DIRECT_STORAGE_RECEIVERS = {
    "localStorage": "localStorage",
    "sessionStorage": "sessionStorage",
    "AsyncStorage": "AsyncStorage",
    "SharedPreferences": "SharedPreferences",
    "indexedDB": "indexedDB",
}
STORAGE_CALL = re.compile(
    r"\b("
    + "|".join(re.escape(receiver) for receiver in sorted(DIRECT_STORAGE_RECEIVERS))
    + r")\s*\.\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*\("
    + r"|\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\.\s*("
    + "|".join(
        re.escape(method)
        for method in sorted(PERSIST_METHODS | READ_METHODS | CLEANUP_METHODS)
    )
    + r")\s*\("
)
SHARED_PREFERENCES_TYPED = (
    re.compile(r"\bSharedPreferences\??\s+([A-Za-z_$][A-Za-z0-9_$]*)\b"),
    re.compile(r"\b([A-Za-z_$][A-Za-z0-9_$]*)\s*:\s*SharedPreferences\??\b"),
)
SHARED_PREFERENCES_ASSIGNMENT = re.compile(
    r"\b(?:const|final|var|let)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*"
    r"(?:await\s+)?SharedPreferences\s*\.\s*getInstance\s*\("
)
IDB_OBJECT_STORE_TYPED = (
    re.compile(r"\bIDBObjectStore\s+([A-Za-z_$][A-Za-z0-9_$]*)\b"),
    re.compile(r"\b([A-Za-z_$][A-Za-z0-9_$]*)\s*:\s*IDBObjectStore\b"),
)
IDB_OBJECT_STORE_ASSIGNMENT = re.compile(
    r"\b(?:const|final|var|let)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*"
    r"[^;\n]*\bobjectStore\s*\("
)
IMMUTABLE_DECLARATION = re.compile(
    r"\b(?:const|final)\s+(?:String\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*([^;\n]+);"
)
ZERO_ARGUMENT_FUNCTION_RETURN = re.compile(
    r"\bfunction\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(\s*\)\s*"
    r"\{\s*return\s+([^;\n{}]+)\s*;\s*\}"
)
ZERO_ARGUMENT_ARROW_RETURN = re.compile(
    r"\b(?:const|final)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*"
    r"\(\s*\)\s*=>\s*([^;\n{}]+)\s*;"
)
ZERO_ARGUMENT_CALL = re.compile(r"^([A-Za-z_$][A-Za-z0-9_$]*)\s*\(\s*\)$")
OBJECT_DECLARATION = re.compile(
    r"\b(?:const|let|var|final)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*\{([^{}]*)\}"
)
OBJECT_DELETE = re.compile(
    r"\bdelete\s+([A-Za-z_$][A-Za-z0-9_$]*)\.([A-Za-z_$][A-Za-z0-9_$]*)"
)
OBJECT_ASSIGNMENT = re.compile(
    r"\b([A-Za-z_$][A-Za-z0-9_$]*)\.([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*([^;\n]+)"
)
OBJECT_PROPERTY_REFERENCE = re.compile(
    r"\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\.\s*([A-Za-z_$][A-Za-z0-9_$]*)"
)
PROVIDER_KEY = re.compile(r"^marketProvider:([^:]+):apiKey$")
IDENTIFIER = re.compile(r"^[A-Za-z_$][A-Za-z0-9_$]*$")
HIGH_TRUST = re.compile(
    r"auth|login|session|jwt|bearer|refresh|access.?token|token|password|passphrase|"
    r"secret|private.?key|signing.?key|encryption.?key|cookie|payment|card|cvv|cvc|ssn",
    re.IGNORECASE,
)
MARKET_CREDENTIAL = re.compile(
    r"api[_-]?key|apikey|provider.?credential|market.?credential|credentials?",
    re.IGNORECASE,
)
WILDCARD_OR_REGEX = re.compile(r"[*?\[\]{}()|^$+\\]")


class ConfigError(ValueError):
    def __init__(self, message: str, line: int = 0) -> None:
        super().__init__(message)
        self.line = line


@dataclass(frozen=True)
class Approval:
    path: str
    storage: str
    key: str
    provider: str
    credential_class: str
    privilege: str
    lifetime: str


@dataclass
class ObjectState:
    sensitive_fields: set[str] = field(default_factory=set)
    forbidden_fields: set[str] = field(default_factory=set)
    sensitive_deletions: dict[str, frozenset[int]] = field(default_factory=dict)
    forbidden_deletions: dict[str, frozenset[int]] = field(default_factory=dict)
    provider: Optional[str] = None

    def delete_field(self, field_name: str, context: frozenset[int]) -> None:
        if field_name in self.sensitive_fields:
            self.sensitive_deletions[field_name] = context
        if field_name in self.forbidden_fields:
            self.forbidden_deletions[field_name] = context

    def taint_at(self, context: frozenset[int]) -> str:
        forbidden_remaining = any(
            field_name not in self.forbidden_deletions
            or not self.forbidden_deletions[field_name].issubset(context)
            for field_name in self.forbidden_fields
        )
        if forbidden_remaining:
            return "forbidden"
        sensitive_remaining = any(
            field_name not in self.sensitive_deletions
            or not self.sensitive_deletions[field_name].issubset(context)
            for field_name in self.sensitive_fields
        )
        if sensitive_remaining:
            return "market"
        return "none"


@dataclass
class Finding:
    path: str
    line: int
    reason: str
    storage: str
    operation: str
    key: str
    provider: str
    config_match: str

    def emit(self) -> None:
        values = (
            self.path,
            str(self.line),
            self.reason,
            self.storage,
            self.operation,
            self.key,
            self.provider,
            self.config_match,
        )
        print("FINDING\t" + "\t".join(sanitize_field(value) for value in values))


def sanitize_field(value: str) -> str:
    return str(value).replace("\t", " ").replace("\r", " ").replace("\n", " ")


def strip_source_comments(text: str) -> str:
    output: list[str] = []
    index = 0
    quote = ""
    block_comment = False
    while index < len(text):
        char = text[index]
        following = text[index + 1] if index + 1 < len(text) else ""
        if block_comment:
            if char == "*" and following == "/":
                output.extend((" ", " "))
                block_comment = False
                index += 2
            else:
                output.append("\n" if char == "\n" else " ")
                index += 1
            continue
        if quote:
            output.append(char)
            if char == "\\" and index + 1 < len(text):
                output.append(text[index + 1])
                index += 2
                continue
            if char == quote:
                quote = ""
            index += 1
            continue
        if char in {"'", '"', "`"}:
            quote = char
            output.append(char)
            index += 1
            continue
        if char == "/" and following == "/":
            output.extend((" ", " "))
            index += 2
            while index < len(text) and text[index] != "\n":
                output.append(" ")
                index += 1
            continue
        if char == "/" and following == "*":
            output.extend((" ", " "))
            block_comment = True
            index += 2
            continue
        output.append(char)
        index += 1
    return "".join(output)


def strip_yaml_comment(line: str) -> str:
    quote = ""
    escaped = False
    for index, char in enumerate(line):
        if escaped:
            escaped = False
            continue
        if quote == '"' and char == "\\":
            escaped = True
            continue
        if quote:
            if char == quote:
                quote = ""
            continue
        if char in {"'", '"'}:
            quote = char
        elif char == "#":
            return line[:index]
    return line


def parse_yaml_scalar(raw_value: str, line: int) -> str:
    value = raw_value.strip()
    if not value:
        raise ConfigError("empty configuration value", line)
    if value.startswith('"'):
        try:
            parsed = ast.literal_eval(value)
        except (SyntaxError, ValueError) as exc:
            raise ConfigError("invalid quoted configuration value", line) from exc
        if not isinstance(parsed, str) or not parsed:
            raise ConfigError("configuration value must be a nonempty string", line)
        return parsed
    if value.startswith("'"):
        if len(value) < 2 or not value.endswith("'"):
            raise ConfigError("invalid quoted configuration value", line)
        parsed = value[1:-1].replace("''", "'")
        if not parsed:
            raise ConfigError("configuration value must be nonempty", line)
        return parsed
    if value[0] in "[{&*!>|" or value.endswith(("]", "}")):
        raise ConfigError("complex YAML values are not permitted in this section", line)
    return value


def validate_literal(value: str, label: str, line: int) -> None:
    if not value.strip() or WILDCARD_OR_REGEX.search(value):
        raise ConfigError(f"{label} must be one nonempty literal", line)


def validate_config_path(repo_root: Path, value: str, line: int) -> str:
    validate_literal(value, "path", line)
    if value.startswith("/") or value.startswith("~") or "\\" in value:
        raise ConfigError("path must be repo-relative POSIX syntax", line)
    pure_path = PurePosixPath(value)
    if value.startswith("./") or ".." in pure_path.parts or "." in pure_path.parts:
        raise ConfigError("path must be normalized without traversal", line)
    normalized = pure_path.as_posix()
    if normalized != value:
        raise ConfigError("path must already be normalized", line)
    candidate = repo_root.joinpath(*pure_path.parts)
    try:
        candidate.relative_to(repo_root)
    except ValueError as exc:
        raise ConfigError("path escapes the repository", line) from exc
    if candidate.exists():
        try:
            candidate.resolve().relative_to(repo_root.resolve())
        except ValueError as exc:
            raise ConfigError("path resolves outside the repository", line) from exc
    return normalized


def parse_project_config(config_path: Path, repo_root: Path) -> tuple[list[Approval], int]:
    if not config_path.is_file():
        return [], 0
    try:
        raw_text = config_path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        raise ConfigError("project configuration cannot be read") from exc
    if "sensitiveClientStorage" not in raw_text:
        return [], 0
    if "\t" in raw_text:
        raise ConfigError("tabs are not permitted in sensitive storage configuration")

    lines = raw_text.splitlines()
    scans_line = 0
    section_line = 0
    approvals_line = 0
    section_end = len(lines) + 1
    for line_number, raw_line in enumerate(lines, start=1):
        line = strip_yaml_comment(raw_line).rstrip()
        if not line.strip():
            continue
        indent = len(line) - len(line.lstrip(" "))
        stripped = line.strip()
        if stripped == "scans:" and indent == 0:
            if scans_line:
                raise ConfigError("duplicate scans section", line_number)
            scans_line = line_number
            continue
        if scans_line and line_number > scans_line and indent == 0:
            break
        if scans_line and stripped == "sensitiveClientStorage:" and indent == 2:
            if section_line:
                raise ConfigError("duplicate sensitiveClientStorage section", line_number)
            section_line = line_number
            continue
        if section_line and line_number > section_line and indent <= 2:
            section_end = line_number
            break
        if section_line and stripped.startswith("approvedSessionCredentials:") and indent == 4:
            if approvals_line:
                raise ConfigError("duplicate approvedSessionCredentials field", line_number)
            approvals_line = line_number
            suffix = stripped.split(":", 1)[1].strip()
            if suffix not in {"", "[]"}:
                raise ConfigError("approvedSessionCredentials must be a block list", line_number)
            continue
        if section_line and line_number > section_line and indent == 4:
            raise ConfigError("unknown sensitiveClientStorage field", line_number)

    if not scans_line or not section_line or not approvals_line:
        raise ConfigError("sensitiveClientStorage must use the documented nested schema", section_line)

    approval_suffix = strip_yaml_comment(lines[approvals_line - 1]).split(":", 1)[1].strip()
    if approval_suffix == "[]":
        return [], section_line

    item_maps: list[tuple[dict[str, str], dict[str, int]]] = []
    current: Optional[dict[str, str]] = None
    current_lines: dict[str, int] = {}
    for line_number in range(approvals_line + 1, section_end):
        line = strip_yaml_comment(lines[line_number - 1]).rstrip()
        if not line.strip():
            continue
        indent = len(line) - len(line.lstrip(" "))
        stripped = line.strip()
        if indent == 4:
            raise ConfigError("unknown sensitiveClientStorage field", line_number)
        if indent == 6 and stripped.startswith("- "):
            if current is not None:
                item_maps.append((current, current_lines))
            current = {}
            current_lines = {}
            field_text = stripped[2:].strip()
        elif indent == 8 and current is not None:
            field_text = stripped
        else:
            raise ConfigError("invalid approvedSessionCredentials indentation", line_number)
        if ":" not in field_text:
            raise ConfigError("approval fields require key: value syntax", line_number)
        key, raw_value = field_text.split(":", 1)
        key = key.strip()
        if key not in ALLOWED_CONFIG_FIELDS:
            raise ConfigError(f"unknown approval field: {key}", line_number)
        if key in current:
            raise ConfigError(f"duplicate approval field: {key}", line_number)
        current[key] = parse_yaml_scalar(raw_value, line_number)
        current_lines[key] = line_number
    if current is not None:
        item_maps.append((current, current_lines))
    if not item_maps:
        raise ConfigError("approvedSessionCredentials requires a list or []", approvals_line)

    approvals: list[Approval] = []
    seen_tuples: set[tuple[str, ...]] = set()
    seen_path_keys: set[tuple[str, str, str]] = set()
    for values, value_lines in item_maps:
        missing = ALLOWED_CONFIG_FIELDS - set(values)
        if missing:
            raise ConfigError(
                "approval entry is missing required fields: " + ",".join(sorted(missing)),
                min(value_lines.values(), default=approvals_line),
            )
        path = validate_config_path(repo_root, values["path"], value_lines["path"])
        validate_literal(values["key"], "key", value_lines["key"])
        validate_literal(values["provider"], "provider", value_lines["provider"])
        if values["storage"] != "sessionStorage":
            raise ConfigError("storage must be exactly sessionStorage", value_lines["storage"])
        if values["credentialClass"] != "third-party-market-data":
            raise ConfigError(
                "credentialClass must be exactly third-party-market-data",
                value_lines["credentialClass"],
            )
        if values["privilege"] != "low":
            raise ConfigError("privilege must be exactly low", value_lines["privilege"])
        if values["lifetime"] != "same-tab":
            raise ConfigError("lifetime must be exactly same-tab", value_lines["lifetime"])
        approval = Approval(
            path=path,
            storage=values["storage"],
            key=values["key"],
            provider=values["provider"],
            credential_class=values["credentialClass"],
            privilege=values["privilege"],
            lifetime=values["lifetime"],
        )
        identity = (
            approval.path,
            approval.storage,
            approval.key,
            approval.provider,
            approval.credential_class,
            approval.privilege,
            approval.lifetime,
        )
        path_key = (approval.path, approval.storage, approval.key)
        if identity in seen_tuples:
            raise ConfigError("duplicate approval tuple", value_lines["path"])
        if path_key in seen_path_keys:
            raise ConfigError("one path/storage/key tuple may approve only one provider", value_lines["path"])
        seen_tuples.add(identity)
        seen_path_keys.add(path_key)
        approvals.append(approval)
    return approvals, section_line


def split_top_level(text: str, delimiter: str = ",") -> list[str]:
    parts: list[str] = []
    start = 0
    quote = ""
    escaped = False
    depth = 0
    for index, char in enumerate(text):
        if escaped:
            escaped = False
            continue
        if quote:
            if char == "\\":
                escaped = True
            elif char == quote:
                quote = ""
            continue
        if char in {"'", '"', "`"}:
            quote = char
        elif char in "([{":
            depth += 1
        elif char in ")]}" and depth:
            depth -= 1
        elif char == delimiter and depth == 0:
            parts.append(text[start:index].strip())
            start = index + 1
    parts.append(text[start:].strip())
    return parts


def find_closing_parenthesis(text: str, opening_index: int) -> int:
    depth = 0
    quote = ""
    escaped = False
    for index in range(opening_index, len(text)):
        char = text[index]
        if escaped:
            escaped = False
            continue
        if quote:
            if char == "\\":
                escaped = True
            elif char == quote:
                quote = ""
            continue
        if char in {"'", '"', "`"}:
            quote = char
        elif char == "(":
            depth += 1
        elif char == ")":
            depth -= 1
            if depth == 0:
                return index
    return -1


def literal_string(expression: str) -> Optional[str]:
    value = expression.strip()
    if len(value) < 2 or value[0] not in {"'", '"', "`"} or value[-1] != value[0]:
        return None
    if value[0] == "`":
        return None if "${" in value else value[1:-1]
    try:
        parsed = ast.literal_eval(value)
    except (SyntaxError, ValueError):
        return None
    return parsed if isinstance(parsed, str) else None


def resolve_expression(
    expression: str,
    declarations: dict[str, str],
    helper_returns: dict[str, str],
    unstable: set[str],
    cache: dict[str, Optional[str]],
    stack: Optional[set[str]] = None,
) -> Optional[str]:
    value = expression.strip()
    literal = literal_string(value)
    if literal is not None:
        return literal
    if IDENTIFIER.fullmatch(value):
        if value in unstable or value not in declarations:
            return None
        if value in cache:
            return cache[value]
        active = set() if stack is None else set(stack)
        if value in active or len(active) >= 8:
            return None
        active.add(value)
        cache[value] = resolve_expression(
            declarations[value], declarations, helper_returns, unstable, cache, active
        )
        return cache[value]
    helper_call = ZERO_ARGUMENT_CALL.fullmatch(value)
    if helper_call:
        helper_name = helper_call.group(1)
        cache_key = f"helper:{helper_name}"
        if helper_name in unstable or helper_name not in helper_returns:
            return None
        if cache_key in cache:
            return cache[cache_key]
        active = set() if stack is None else set(stack)
        if cache_key in active or len(active) >= 8:
            return None
        active.add(cache_key)
        cache[cache_key] = resolve_expression(
            helper_returns[helper_name],
            declarations,
            helper_returns,
            unstable,
            cache,
            active,
        )
        return cache[cache_key]
    if value.startswith("`") and value.endswith("`"):
        template = value[1:-1]
        unresolved = False

        def replace(match: re.Match[str]) -> str:
            nonlocal unresolved
            replacement = resolve_expression(
                match.group(1), declarations, helper_returns, unstable, cache, stack
            )
            if replacement is None:
                unresolved = True
                return ""
            return replacement

        resolved_template = re.sub(r"\$\{\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*\}", replace, template)
        return None if unresolved or "${" in resolved_template else resolved_template
    pieces = split_top_level(value, "+")
    if len(pieces) > 1:
        resolved_pieces = [
            resolve_expression(piece, declarations, helper_returns, unstable, cache, stack)
            for piece in pieces
        ]
        if all(piece is not None for piece in resolved_pieces):
            return "".join(piece or "" for piece in resolved_pieces)
    return None


def declaration_state(
    text: str,
) -> tuple[dict[str, str], dict[str, str], set[str], dict[str, Optional[str]]]:
    declarations: dict[str, str] = {}
    declaration_spans: dict[str, tuple[int, int]] = {}
    helper_returns: dict[str, str] = {}
    unstable: set[str] = set()
    for match in IMMUTABLE_DECLARATION.finditer(text):
        name = match.group(1)
        if name in declarations:
            unstable.add(name)
        declarations[name] = match.group(2).strip()
        declaration_spans[name] = match.span()
    for pattern in (ZERO_ARGUMENT_FUNCTION_RETURN, ZERO_ARGUMENT_ARROW_RETURN):
        for match in pattern.finditer(text):
            name = match.group(1)
            if name in helper_returns:
                unstable.add(name)
            helper_returns[name] = match.group(2).strip()
            declaration_spans.setdefault(name, match.span())
    for name, declaration_span in declaration_spans.items():
        assignment = re.compile(rf"\b{re.escape(name)}\s*(?:=|\+=|-=|\+\+|--)")
        for match in assignment.finditer(text):
            if not (declaration_span[0] <= match.start() < declaration_span[1]):
                unstable.add(name)
                break
    return declarations, helper_returns, unstable, {}


def infer_storage_handles(text: str) -> dict[str, str]:
    handles: dict[str, str] = {}
    for pattern in SHARED_PREFERENCES_TYPED:
        for match in pattern.finditer(text):
            handles[match.group(1)] = "SharedPreferences"
    for match in SHARED_PREFERENCES_ASSIGNMENT.finditer(text):
        handles[match.group(1)] = "SharedPreferences"
    for pattern in IDB_OBJECT_STORE_TYPED:
        for match in pattern.finditer(text):
            handles[match.group(1)] = "indexedDB"
    for match in IDB_OBJECT_STORE_ASSIGNMENT.finditer(text):
        handles[match.group(1)] = "indexedDB"
    return handles


def classify_identifier(value: str) -> str:
    if HIGH_TRUST.search(value):
        return "forbidden"
    if MARKET_CREDENTIAL.search(value):
        return "market"
    return "none"


def expression_identifiers(expression: str) -> list[str]:
    return re.findall(r"\b[A-Za-z_$][A-Za-z0-9_$]*\b", expression)


def expression_taint(
    expression: str,
    objects: dict[str, ObjectState],
    key: Optional[str],
    context: frozenset[int],
    declarations: dict[str, str],
    helper_returns: dict[str, str],
    unstable: set[str],
    key_expression: str = "",
) -> str:
    key_taint = classify_identifier(key or "")
    if key_taint == "forbidden":
        return "forbidden"
    taints: list[str] = [key_taint]
    pending: list[tuple[str, int]] = [(expression, 0)]
    if key is None:
        pending.append((key_expression, 0))
    visited_sources: set[str] = set()
    while pending:
        source_expression, depth = pending.pop()
        for identifier in expression_identifiers(source_expression):
            if identifier in objects:
                taints.append(objects[identifier].taint_at(context))
                continue
            taints.append(classify_identifier(identifier))
            if depth >= 8 or identifier in unstable or identifier in visited_sources:
                continue
            if identifier in declarations:
                visited_sources.add(identifier)
                pending.append((declarations[identifier], depth + 1))
            elif identifier in helper_returns:
                visited_sources.add(identifier)
                pending.append((helper_returns[identifier], depth + 1))
    if any(taint == "forbidden" for taint in taints):
        return "forbidden"
    if any(taint == "market" for taint in taints):
        return "market"
    return "none"


def provider_from_key(key: Optional[str]) -> Optional[str]:
    if key is None:
        return None
    match = PROVIDER_KEY.fullmatch(key)
    return match.group(1) if match else None


def control_block_ranges(text: str) -> list[tuple[int, int]]:
    control_keywords = {"if", "else", "for", "while", "switch", "catch", "try", "finally", "do"}
    ranges: list[tuple[int, int]] = []
    brace_stack: list[tuple[int, bool]] = []
    parenthesis_stack: list[int] = []
    closing_to_opening: dict[int, int] = {}
    quote = ""
    escaped = False

    for index, char in enumerate(text):
        if escaped:
            escaped = False
            continue
        if quote:
            if char == "\\":
                escaped = True
            elif char == quote:
                quote = ""
            continue
        if char in {"'", '"', "`"}:
            quote = char
            continue
        if char == "(":
            parenthesis_stack.append(index)
            continue
        if char == ")":
            if parenthesis_stack:
                closing_to_opening[index] = parenthesis_stack.pop()
            continue
        if char == "{":
            cursor = index - 1
            while cursor >= 0 and text[cursor].isspace():
                cursor -= 1
            keyword_text = text[: cursor + 1]
            if cursor in closing_to_opening:
                keyword_text = text[: closing_to_opening[cursor]].rstrip()
            keyword_match = re.search(r"([A-Za-z_$][A-Za-z0-9_$]*)\s*$", keyword_text)
            is_control = bool(keyword_match and keyword_match.group(1) in control_keywords)
            brace_stack.append((index, is_control))
            continue
        if char == "}" and brace_stack:
            opening, is_control = brace_stack.pop()
            ranges.append((opening, index))
    return ranges


def control_context_at(
    text: str,
    position: int,
    ranges: list[tuple[int, int]],
    conditional_statement: bool = False,
) -> frozenset[int]:
    context = {opening for opening, closing in ranges if opening < position < closing}
    if conditional_statement:
        statement_start = max(
            text.rfind(";", 0, position),
            text.rfind("{", 0, position),
            text.rfind("}", 0, position),
            text.rfind("\n", 0, position),
        ) + 1
        statement_prefix = text[statement_start:position]
        if re.search(r"\b(?:if|for|while)\s*\(.*\)\s*$", statement_prefix, re.DOTALL) or re.search(
            r"(?:&&|\|\||\?)\s*$", statement_prefix
        ):
            context.add(-(position + 1))
    return frozenset(context)


def object_events(text: str) -> list[tuple[int, str, re.Match[str]]]:
    events: list[tuple[int, str, re.Match[str]]] = []
    events.extend((match.start(), "declare", match) for match in OBJECT_DECLARATION.finditer(text))
    events.extend((match.start(), "delete", match) for match in OBJECT_DELETE.finditer(text))
    events.extend((match.start(), "assign", match) for match in OBJECT_ASSIGNMENT.finditer(text))
    events.extend((match.start(), "storage", match) for match in STORAGE_CALL.finditer(text))
    for match in OBJECT_PROPERTY_REFERENCE.finditer(text):
        object_name = match.group(1)
        prefix = text[max(0, match.start() - 16) : match.start()]
        if object_name in {
            "localStorage",
            "sessionStorage",
            "AsyncStorage",
            "SharedPreferences",
            "indexedDB",
        } or re.search(r"\bdelete\s*$", prefix):
            continue
        events.append((match.start(), "observe", match))
    return sorted(events, key=lambda event: (event[0], event[1] != "declare"))


def update_object_declaration(
    match: re.Match[str],
    objects: dict[str, ObjectState],
    declarations: dict[str, str],
    helper_returns: dict[str, str],
    unstable: set[str],
    cache: dict[str, Optional[str]],
) -> None:
    state = ObjectState()
    for field_text in split_top_level(match.group(2)):
        if ":" not in field_text:
            continue
        field_name, field_value = field_text.split(":", 1)
        field_name = field_name.strip().strip("'\"")
        taint = classify_identifier(field_name + " " + field_value)
        if taint == "forbidden":
            state.forbidden_fields.add(field_name)
        elif taint == "market":
            state.sensitive_fields.add(field_name)
        if field_name == "provider":
            state.provider = resolve_expression(
                field_value, declarations, helper_returns, unstable, cache
            )
    objects[match.group(1)] = state


def update_object_assignment(
    match: re.Match[str],
    objects: dict[str, ObjectState],
    declarations: dict[str, str],
    helper_returns: dict[str, str],
    unstable: set[str],
    cache: dict[str, Optional[str]],
) -> None:
    object_name, field_name, field_value = match.groups()
    state = objects.setdefault(object_name, ObjectState())
    state.sensitive_fields.discard(field_name)
    state.forbidden_fields.discard(field_name)
    state.sensitive_deletions.pop(field_name, None)
    state.forbidden_deletions.pop(field_name, None)
    taint = classify_identifier(field_name + " " + field_value)
    if taint == "forbidden":
        state.forbidden_fields.add(field_name)
    elif taint == "market":
        state.sensitive_fields.add(field_name)
    if field_name == "provider":
        state.provider = resolve_expression(
            field_value, declarations, helper_returns, unstable, cache
        )


def config_match_for(
    approvals: Iterable[Approval], path: str, storage: str, key: Optional[str], provider: Optional[str]
) -> str:
    if key is None or provider is None:
        return "unresolved"
    for approval in approvals:
        if (
            approval.path == path
            and approval.storage == storage
            and approval.key == key
            and approval.provider == provider
        ):
            return "exact"
    same_boundary = [
        approval
        for approval in approvals
        if approval.path == path and approval.storage == storage
    ]
    if same_boundary and any(approval.provider != provider for approval in same_boundary):
        return "provider-mismatch"
    return "absent"


def operation_finding(
    *,
    path: str,
    line: int,
    storage: str,
    operation: str,
    key: Optional[str],
    provider: Optional[str],
    taint: str,
    approvals: list[Approval],
) -> Optional[Finding]:
    key_display = key if key is not None else "unresolved"
    provider_display = provider if provider is not None else "unresolved"
    config_match = config_match_for(approvals, path, storage, key, provider)
    if operation in {"remove", "clear"}:
        return None
    if taint == "none":
        return None
    if taint == "forbidden":
        reason = "FORBIDDEN_SECRET_CLASS"
    elif operation == "unresolved":
        reason = "SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED"
    elif storage != "sessionStorage":
        reason = "DURABLE_CREDENTIAL_STORAGE"
    elif key is None or provider is None:
        reason = "SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED"
    elif config_match == "provider-mismatch":
        reason = "SESSION_PROVIDER_UNKNOWN"
    elif config_match != "exact":
        reason = "SESSION_CREDENTIAL_UNAPPROVED"
    else:
        return None
    return Finding(
        path=path,
        line=line,
        reason=reason,
        storage=storage,
        operation=operation,
        key=key_display,
        provider=provider_display,
        config_match=config_match,
    )


def analyze_file(path: Path, repo_root: Path, approvals: list[Approval]) -> list[Finding]:
    try:
        raw_text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError):
        relative = os.path.relpath(path, repo_root).replace(os.sep, "/")
        return [
            Finding(
                path=relative,
                line=0,
                reason="SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED",
                storage="unknown",
                operation="parse",
                key="unresolved",
                provider="unresolved",
                config_match="unresolved",
            )
        ]
    text = strip_source_comments(raw_text)
    declarations, helper_returns, unstable, cache = declaration_state(text)
    storage_handles = infer_storage_handles(text)
    objects: dict[str, ObjectState] = {}
    findings: list[Finding] = []
    relative = os.path.relpath(path, repo_root).replace(os.sep, "/")
    control_ranges = control_block_ranges(text)
    for event_position, event_type, match in object_events(text):
        event_context = control_context_at(
            text,
            event_position,
            control_ranges,
            conditional_statement=event_type == "delete",
        )
        if event_type == "declare":
            update_object_declaration(
                match, objects, declarations, helper_returns, unstable, cache
            )
            continue
        if event_type == "delete":
            object_name, field_name = match.groups()
            state = objects.setdefault(object_name, ObjectState())
            state.delete_field(field_name, event_context)
            continue
        if event_type == "observe":
            object_name, field_name = match.groups()
            state = objects.setdefault(object_name, ObjectState())
            field_taint = classify_identifier(field_name)
            if field_taint == "forbidden":
                state.forbidden_fields.add(field_name)
            elif field_taint == "market":
                state.sensitive_fields.add(field_name)
            continue
        if event_type == "assign":
            update_object_assignment(
                match, objects, declarations, helper_returns, unstable, cache
            )
            continue

        direct_receiver, direct_method, handle_receiver, handle_method = match.groups()
        receiver = direct_receiver or handle_receiver
        method = direct_method or handle_method
        storage = DIRECT_STORAGE_RECEIVERS.get(receiver, storage_handles.get(receiver))
        if storage is None:
            continue
        closing = find_closing_parenthesis(text, match.end() - 1)
        line = text.count("\n", 0, match.start()) + 1
        if closing < 0:
            findings.append(
                Finding(
                    path=relative,
                    line=line,
                    reason="SENSITIVE_STORAGE_CLASSIFICATION_UNRESOLVED",
                    storage=storage,
                    operation="unresolved",
                    key="unresolved",
                    provider="unresolved",
                    config_match="unresolved",
                )
            )
            continue
        arguments = split_top_level(text[match.end() : closing])
        indexed_db_write = (
            storage == "indexedDB"
            and receiver != "indexedDB"
            and method in {"put", "add"}
        )
        key_index = 1 if indexed_db_write else 0
        key_expression = (
            arguments[key_index]
            if len(arguments) > key_index and arguments[key_index]
            else ""
        )
        key = (
            resolve_expression(key_expression, declarations, helper_returns, unstable, cache)
            if key_expression
            else None
        )
        provider = provider_from_key(key)
        if method in CLEANUP_METHODS:
            operation = "clear" if method == "clear" else "remove"
            taint = "none"
        elif method in READ_METHODS:
            operation = "read"
            taint = expression_taint(
                key_expression,
                objects,
                key,
                event_context,
                declarations,
                helper_returns,
                unstable,
                key_expression,
            )
        elif method in PERSIST_METHODS:
            operation = "persist"
            if indexed_db_write:
                value_expression = arguments[0] if arguments else ""
            else:
                value_expression = arguments[1] if len(arguments) > 1 else " ".join(arguments)
            taint = expression_taint(
                value_expression,
                objects,
                key,
                event_context,
                declarations,
                helper_returns,
                unstable,
                key_expression,
            )
            for identifier in expression_identifiers(value_expression):
                state = objects.get(identifier)
                if state and state.provider:
                    provider = state.provider
                    break
        else:
            operation = "unresolved"
            taint = expression_taint(
                " ".join(arguments),
                objects,
                key,
                event_context,
                declarations,
                helper_returns,
                unstable,
                key_expression,
            )
        finding = operation_finding(
            path=relative,
            line=line,
            storage=storage,
            operation=operation,
            key=key,
            provider=provider,
            taint=taint,
            approvals=approvals,
        )
        if finding:
            findings.append(finding)
    return findings


def main(argv: list[str]) -> int:
    if len(argv) < 5 or argv[1] != "--repo-root" or argv[3] != "--config":
        print("ERROR\tusage\t0\tinvalid helper invocation", file=sys.stderr)
        return 2
    repo_root = Path(argv[2]).resolve()
    config_path = Path(argv[4]).resolve()
    source_paths = [Path(value) for value in argv[5:]]
    try:
        approvals, _ = parse_project_config(config_path, repo_root)
    except ConfigError as exc:
        Finding(
            path=os.path.relpath(config_path, repo_root).replace(os.sep, "/"),
            line=exc.line,
            reason="SENSITIVE_STORAGE_CONFIG_INVALID",
            storage="configuration",
            operation="parse",
            key="unresolved",
            provider="unresolved",
            config_match="invalid",
        ).emit()
        approvals = []
    for source_path in source_paths:
        for finding in analyze_file(source_path, repo_root, approvals):
            finding.emit()
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))