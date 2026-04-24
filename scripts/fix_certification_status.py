#!/usr/bin/env python3
"""Bulk-align state.json certification.status with top-level status.

Category A governance fix: when status == "done" and certification.status == "certified",
update certification.status to "done" so artifact-lint passes the equality check.

Idempotent. Prints one line per file modified or skipped.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path


def find_targets(repo_root: Path) -> list[Path]:
    specs = repo_root / "specs"
    targets: list[Path] = []
    targets.extend(sorted(specs.glob("*/state.json")))
    targets.extend(sorted(specs.glob("*/bugs/*/state.json")))
    return targets


def fix_file(path: Path) -> str:
    raw = path.read_text(encoding="utf-8")
    try:
        data = json.loads(raw)
    except json.JSONDecodeError as exc:
        return f"SKIP (invalid json: {exc}): {path}"

    status = data.get("status")
    cert = data.get("certification")
    if not isinstance(cert, dict):
        return f"SKIP (no certification block): {path}"

    cert_status = cert.get("status")
    if status != "done":
        return f"SKIP (status={status!r}): {path}"
    if cert_status == "done":
        return f"OK   (already aligned): {path}"
    if cert_status != "certified":
        return f"SKIP (cert.status={cert_status!r}): {path}"

    cert["status"] = "done"
    new_text = json.dumps(data, indent=2, ensure_ascii=False) + "\n"
    path.write_text(new_text, encoding="utf-8")
    return f"FIX  (certified->done): {path}"


def main() -> int:
    repo_root = Path(__file__).resolve().parents[1]
    targets = find_targets(repo_root)
    fixed = 0
    for path in targets:
        result = fix_file(path)
        print(result)
        if result.startswith("FIX"):
            fixed += 1
    print(f"---\nTotal files scanned: {len(targets)}\nTotal files fixed:   {fixed}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
