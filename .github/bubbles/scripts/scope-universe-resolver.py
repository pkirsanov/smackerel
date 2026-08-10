#!/usr/bin/env python3
"""scope-universe-resolver.py — BUG-026 (F001) current-scope resolver.

Reads a feature directory's ``state.json`` (schema version 3), validates the
current-scope contract fail-closed, and emits an immutable, tab-delimited
``ScopeRecord`` projection for ``traceability-guard.sh --current-scope``.

Contract (see improvements/BUG-026-.../design.md):

* argv: exactly ``FEATURE_DIR current-scope``. No other context, identity,
  status, state-path, or bypass argument is accepted.
* No environment variable selects context/identity/status/state path.
* ``FEATURE_DIR/state.json`` must be readable, valid JSON with NO duplicate
  keys, and top-level ``version`` MUST be the JSON number ``3``.
* ``certification.scopeProgress`` is the mandatory canonical registry.
  ``execution.scopeProgress`` is an optional overlay that, when present, must be
  a valid non-empty registry that AGREES with the canonical one (never a
  fallback).
* ``execution.currentScope`` resolves through a CLOSED alias set to exactly one
  record; that record's status must be ``in_progress`` or ``blocked``.
* Top-level ``status`` and ``certification.status`` must both be canonical and
  agree; current-scope accepts only ``in_progress`` or ``blocked``.
* ``execution.currentPhase`` must be a non-terminal phase token
  (``validate``/``audit``/``finalize`` refuse).
* Dependency edges resolve to canonical IDs; duplicate/unknown/self/cycle edges
  refuse. Every transitive prerequisite of the current scope must be ``done``.
* A record is OMITTED from the applicable universe iff it is a transitive
  descendant of the current scope (reachable via reverse dependency edges) AND
  its status is ``not_started``.

Exit codes: ``0`` success (records on stdout); ``2`` any contract/usage refusal
(reason on stderr, prefixed ``scope-universe-resolver:``). It never emits a
partial projection and never falls back to all-scope behavior.

Output protocol (tab-delimited, one record per line, deterministic order):

    RECORD<TAB>canonicalId<TAB>status<TAB>isCurrent<TAB>isDescendant<TAB>applicable
"""

import json
import sys

CANONICAL_STATUSES = ("not_started", "in_progress", "blocked", "done")
TERMINAL_PHASES = ("validate", "audit", "finalize")


def die(message):
    sys.stderr.write("scope-universe-resolver: " + message + "\n")
    raise SystemExit(2)


def reject_constant(value):
    # Reject NaN/Infinity/-Infinity, which json permits by default.
    die("state.json contains a non-finite JSON constant (" + str(value) + ")")


def reject_duplicate_keys(pairs):
    seen = {}
    for key, value in pairs:
        if key in seen:
            die("state.json contains a duplicate object key: " + repr(key))
        seen[key] = value
    return seen


def load_state(state_path):
    try:
        with open(state_path, "r", encoding="utf-8") as handle:
            text = handle.read()
    except OSError as exc:
        die("cannot read state at " + state_path + " (" + exc.__class__.__name__ + ")")
    try:
        decoder = json.JSONDecoder(
            object_pairs_hook=reject_duplicate_keys,
            parse_constant=reject_constant,
        )
        value, end = decoder.raw_decode(text)
    except ValueError:
        die("state.json is not valid JSON")
    if text[end:].strip():
        die("state.json has trailing non-JSON content")
    if not isinstance(value, dict):
        die("state.json top level must be a JSON object")
    return value


def require_object(value, label):
    if not isinstance(value, dict):
        die(label + " must be a JSON object")
    return value


def canonical_status(value, label):
    if value not in CANONICAL_STATUSES:
        die(label + " status is not canonical: " + repr(value))
    return value


def record_identity(entry, label):
    """Resolve the canonical identity from scopeId or scope (closed rule)."""
    scope_id = entry.get("scopeId")
    scope = entry.get("scope")
    if isinstance(scope_id, str) and scope_id.strip():
        return scope_id
    if isinstance(scope, bool):
        die(label + " scope must not be a boolean")
    if isinstance(scope, int):
        if scope <= 0:
            die(label + " numeric scope must be a positive integer")
        return str(scope)
    if isinstance(scope, str) and scope.strip():
        return scope
    die(label + " has no valid identity (scopeId or scope required)")


def aliases_for(entry, canonical_id):
    """Closed derived alias set (no free-form state alias list is honored)."""
    aliases = {canonical_id}
    scope = entry.get("scope")
    if isinstance(scope, int) and not isinstance(scope, bool) and scope > 0:
        aliases.add(str(scope))
        aliases.add("%02d" % scope)
    if isinstance(scope, str) and scope.strip():
        aliases.add(scope.strip())
    scope_id = entry.get("scopeId")
    if isinstance(scope_id, str) and scope_id.strip():
        aliases.add(scope_id.strip())
    scope_dir = entry.get("scopeDir")
    if isinstance(scope_dir, str) and scope_dir.strip():
        norm = scope_dir.strip().rstrip("/")
        aliases.add(norm)
        aliases.add(norm.rsplit("/", 1)[-1])
        aliases.add(norm + "/scope.md")
    return aliases


def build_registry(entries, label):
    """Validate a scopeProgress array into an ordered list of records."""
    if not isinstance(entries, list) or not entries:
        die(label + " must be a non-empty JSON array")
    records = []
    seen_ids = set()
    for index, entry in enumerate(entries):
        entry_label = label + "[" + str(index) + "]"
        require_object(entry, entry_label)
        canonical_id = record_identity(entry, entry_label)
        if canonical_id in seen_ids:
            die("duplicate canonical scope id: " + repr(canonical_id))
        seen_ids.add(canonical_id)
        status = canonical_status(entry.get("status"), entry_label)
        depends_raw = entry.get("dependsOn", [])
        if not isinstance(depends_raw, list):
            die(entry_label + " dependsOn must be a JSON array")
        depends = []
        for dep in depends_raw:
            if not isinstance(dep, str) or not dep.strip():
                die(entry_label + " dependsOn entries must be non-empty strings")
            if dep in depends:
                die(entry_label + " dependsOn has a duplicate edge: " + repr(dep))
            depends.append(dep)
        scope_dir = entry.get("scopeDir")
        if scope_dir is not None:
            if not isinstance(scope_dir, str) or not scope_dir.strip():
                die(entry_label + " scopeDir must be a non-empty string when present")
            scope_dir = scope_dir.strip().rstrip("/")
        else:
            scope_dir = ""
        records.append(
            {
                "canonicalId": canonical_id,
                "status": status,
                "dependsOn": depends,
                "scopeDir": scope_dir,
                "aliases": aliases_for(entry, canonical_id),
                "isCurrent": False,
                "isDescendant": False,
            }
        )
    return records


def resolve_alias(records, token, label):
    """Resolve one alias token to exactly one record or refuse."""
    matches = [r for r in records if token in r["aliases"]]
    if len(matches) == 0:
        die(label + " resolves to no scope record: " + repr(token))
    if len(matches) > 1:
        die(label + " is ambiguous across records: " + repr(token))
    return matches[0]


def resolve_dependency_edges(records):
    """Map each dependsOn alias to a canonical id; refuse unknown/self edges."""
    by_id = {r["canonicalId"]: r for r in records}
    for record in records:
        resolved = []
        for dep in record["dependsOn"]:
            target = resolve_alias(records, dep, "dependency edge")
            if target["canonicalId"] == record["canonicalId"]:
                die("self dependency edge on " + repr(record["canonicalId"]))
            if target["canonicalId"] in resolved:
                die("duplicate resolved dependency on " + repr(record["canonicalId"]))
            resolved.append(target["canonicalId"])
        record["resolvedDependsOn"] = resolved
    return by_id


def detect_cycles(records, by_id):
    """Depth-first cycle detection over the validated dependency graph."""
    WHITE, GRAY, BLACK = 0, 1, 2
    color = {r["canonicalId"]: WHITE for r in records}

    def visit(node_id):
        color[node_id] = GRAY
        for dep_id in by_id[node_id]["resolvedDependsOn"]:
            if color[dep_id] == GRAY:
                die("dependency cycle detected at " + repr(node_id))
            if color[dep_id] == WHITE:
                visit(dep_id)
        color[node_id] = BLACK

    for record in records:
        if color[record["canonicalId"]] == WHITE:
            visit(record["canonicalId"])


def transitive_prerequisites(current, by_id):
    """All scopes the current scope (transitively) depends on."""
    seen = set()
    stack = list(by_id[current["canonicalId"]]["resolvedDependsOn"])
    while stack:
        node_id = stack.pop()
        if node_id in seen:
            continue
        seen.add(node_id)
        stack.extend(by_id[node_id]["resolvedDependsOn"])
    return seen


def transitive_descendants(current, records, by_id):
    """All scopes that (transitively) depend on the current scope."""
    reverse = {r["canonicalId"]: [] for r in records}
    for record in records:
        for dep_id in record["resolvedDependsOn"]:
            reverse[dep_id].append(record["canonicalId"])
    seen = set()
    stack = list(reverse[current["canonicalId"]])
    while stack:
        node_id = stack.pop()
        if node_id in seen:
            continue
        seen.add(node_id)
        stack.extend(reverse[node_id])
    return seen


def registries_agree(canonical, overlay):
    """Both registries must carry the same canonical scope set + facts."""
    canonical_by_id = {r["canonicalId"]: r for r in canonical}
    overlay_by_id = {r["canonicalId"]: r for r in overlay}
    if set(canonical_by_id) != set(overlay_by_id):
        die("execution.scopeProgress scope set disagrees with certification.scopeProgress")
    for canonical_id, record in canonical_by_id.items():
        other = overlay_by_id[canonical_id]
        if record["status"] != other["status"]:
            die("execution/certification status disagree for " + repr(canonical_id))
        if record["resolvedDependsOn"] != other["resolvedDependsOn"]:
            die("execution/certification dependsOn disagree for " + repr(canonical_id))


def main(argv):
    if len(argv) != 2:
        die("usage: scope-universe-resolver.py FEATURE_DIR current-scope")
    feature_dir, context = argv
    if context != "current-scope":
        die("only the literal context 'current-scope' is accepted")

    state = load_state(feature_dir.rstrip("/") + "/state.json")

    version = state.get("version")
    if isinstance(version, bool) or not isinstance(version, int) or version != 3:
        die("state.json version must be the JSON number 3")

    certification = require_object(state.get("certification"), "certification")
    packet_status = canonical_status(state.get("status"), "top-level status")
    cert_status = canonical_status(certification.get("status"), "certification.status")
    if packet_status != cert_status:
        die("top-level status and certification.status disagree")
    if packet_status not in ("in_progress", "blocked"):
        die("current-scope context requires packet status in_progress or blocked")

    execution = state.get("execution")
    if execution is not None:
        require_object(execution, "execution")
        phase = execution.get("currentPhase")
        if not isinstance(phase, str) or not phase.strip():
            die("execution.currentPhase must be a non-empty string")
        if phase in TERMINAL_PHASES:
            die("execution.currentPhase must be non-terminal (got " + repr(phase) + ")")

    canonical = build_registry(certification.get("scopeProgress"), "certification.scopeProgress")

    if execution is not None and "scopeProgress" in execution:
        overlay = build_registry(execution.get("scopeProgress"), "execution.scopeProgress")
        by_id = resolve_dependency_edges(canonical)
        resolve_dependency_edges(overlay)
        registries_agree(canonical, overlay)
    else:
        by_id = resolve_dependency_edges(canonical)

    detect_cycles(canonical, by_id)

    current_token = None
    if execution is not None:
        current_token = execution.get("currentScope")
    if not isinstance(current_token, (str, int)) or isinstance(current_token, bool):
        die("execution.currentScope is required and must be a string or positive integer")
    current = resolve_alias(canonical, str(current_token), "execution.currentScope")
    current["isCurrent"] = True
    if current["status"] not in ("in_progress", "blocked"):
        die("current scope status must be in_progress or blocked")

    prereqs = transitive_prerequisites(current, by_id)
    for prereq_id in prereqs:
        if by_id[prereq_id]["status"] != "done":
            die("transitive prerequisite " + repr(prereq_id) + " of current scope is not done")

    descendants = transitive_descendants(current, canonical, by_id)
    for record in canonical:
        record["isDescendant"] = record["canonicalId"] in descendants

    lines = []
    for record in canonical:
        applicable = not (record["isDescendant"] and record["status"] == "not_started")
        lines.append(
            "RECORD\t%s\t%s\t%s\t%s\t%s\t%s"
            % (
                record["canonicalId"],
                record["status"],
                "true" if record["isCurrent"] else "false",
                "true" if record["isDescendant"] else "false",
                "true" if applicable else "false",
                record["scopeDir"],
            )
        )
    sys.stdout.write("\n".join(lines) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
