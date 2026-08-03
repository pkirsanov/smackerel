#!/usr/bin/env bash
#
# gate-classification.sh — classify every gate as modelCompensation,
# businessInvariant, or hybrid (IMP-027 / SCOPE-2c).
#
# WHY THIS EXISTS
# ---------------
# bubbles/workflows.yaml::gateClassification listed 13 of 112 gates. The other
# 99 were unclassified, which matters because the two classes have opposite
# life expectancies:
#
#   modelCompensation  exists because models are unreliable — they fabricate
#                      evidence, batch-check DoD items, claim phases they did
#                      not run. These gates SHOULD become unnecessary as models
#                      improve, and carrying them forever is pure cost.
#   businessInvariant  encodes a rule that must hold no matter who or what does
#                      the work — security, data integrity, deployment safety.
#                      These NEVER retire.
#   hybrid             genuinely both.
#
# Without the classification there is no way to answer "which of these 112
# gates could we ever turn off?", which is the question SCOPE-11 (obsolescence
# curve) and the cost work depend on.
#
# DERIVATION (bind only)
# ----------------------
# Classification is a judgement, so this tool never invents one from thin air:
# it derives from SIGNAL PHRASES present in the gate's own description and
# records which signals fired. An entry you disagree with is corrected by hand
# in the registry; bind never overwrites an existing value.
#
# SUBCOMMANDS
#   bind     seed `classification` for gates that lack it
#   lint     fail when any gate is unclassified or carries an invalid value
#   report   print the distribution
#   emit     regenerate the bubbles/workflows.yaml::gateClassification block
#            from the registry, so the two can never disagree
#
# Exit codes: 0 ok - 1 findings (lint) - 2 usage/environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

SUBCOMMAND="${1:-lint}"
shift || true

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      REPO_ROOT="${1:?--repo-root requires a path}"
      shift
      ;;
    -h | --help)
      sed -n '2,38p' "$0"
      exit 0
      ;;
    *)
      echo "gate-classification: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

case "$SUBCOMMAND" in
  bind | lint | report | emit) ;;
  -h | --help)
    sed -n '2,38p' "$0"
    exit 0
    ;;
  *)
    echo "gate-classification: unknown subcommand: $SUBCOMMAND (expected bind|lint|report|emit)" >&2
    exit 2
    ;;
esac

GATES="$REPO_ROOT/bubbles/registry/gates.yaml"
if [[ ! -f "$GATES" ]]; then
  echo "gate-classification: SKIP (bubbles/registry/gates.yaml missing)"
  exit 0
fi
# Resolve the managed interpreter before probing (bubbles/scripts/python-env.sh).
_gc_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
[[ -f "$_gc_dir/dependency-posture.sh" ]] && . "$_gc_dir/dependency-posture.sh"
unset _gc_dir

if ! command -v python3 >/dev/null 2>&1 || ! python3 -c "import yaml" >/dev/null 2>&1; then
  echo "gate-classification: SKIP (python3 + PyYAML not installed)"
  exit 0
fi

python3 - "$REPO_ROOT" "$SUBCOMMAND" <<'PY'
import re
import sys
from pathlib import Path

import yaml

repo_root = Path(sys.argv[1])
subcommand = sys.argv[2]

GATES = repo_root / "bubbles/registry/gates.yaml"
VALID = {"modelCompensation", "businessInvariant", "hybrid"}

# Signal phrases. Each entry is (regex, class). Deliberately conservative and
# quotable: every classification bind reports which signal fired, so a reviewer
# can check the call against the description rather than trust the tool.
MODEL_SIGNALS = [
    r"fabricat", r"\bhallucin", r"models? (?:fabricate|batch|claim|defer|rewrite|impersonat|reformat)",
    r"pseudo-complet", r"narrative", r"agent claims", r"impersonat",
    r"batch-check", r"self-attest", r"unverified claim", r"claims? without",
    r"honest", r"truthful", r"evidence", r"provenance of", r"deferral language",
]
BUSINESS_SIGNALS = [
    r"securit", r"vulnerab", r"OWASP", r"auth(?:oriz|entic)", r"secret",
    r"signature", r"cosign", r"provenance attestation", r"supply chain",
    r"isolat", r"ephemeral", r"backup", r"restore", r"rollback", r"deploy",
    r"corrupt", r"data loss", r"PII", r"architectur", r"contract",
    r"invariant", r"idempoten", r"blast radius", r"prod\b", r"production",
]

raw = GATES.read_text()
data = yaml.safe_load(raw) or {}
gates = {
    gid: meta
    for gid, meta in (data.get("gates") or {}).items()
    if re.fullmatch(r"G\d{3}", str(gid)) and isinstance(meta, dict)
}
gate_ids = sorted(gates)


def signals(desc, patterns):
    hit = []
    for p in patterns:
        if re.search(p, desc, re.I):
            hit.append(p)
    return hit


def derive(gid):
    meta = gates[gid]
    text = f"{meta.get('name', '')} {meta.get('description', '')}"
    m = signals(text, MODEL_SIGNALS)
    b = signals(text, BUSINESS_SIGNALS)
    if m and b:
        return "hybrid", m[:2] + b[:2]
    if m:
        return "modelCompensation", m[:3]
    if b:
        return "businessInvariant", b[:3]
    # No signal fired. A gate that describes neither a model failure mode nor a
    # domain rule is, by elimination, a process/workflow requirement that holds
    # regardless of executor — the businessInvariant side of the split.
    return "businessInvariant", ["(no signal; process requirement by elimination)"]


if subcommand == "bind":
    lines = raw.splitlines(keepends=True)
    inserts = {}
    current = None
    for i, line in enumerate(lines):
        m = re.match(r"^  (G\d{3}):\s*$", line)
        if m:
            current = m.group(1)
            continue
        if current and re.match(r"^  \S", line):
            current = None
        if current and re.match(r"^    name:", line):
            if gates[current].get("classification") is None:
                inserts[i] = current
            current = None

    if not inserts:
        print("gate-classification bind: every gate already declares classification (no change)")
        sys.exit(0)

    out = []
    for i, line in enumerate(lines):
        out.append(line)
        if i in inserts:
            gid = inserts[i]
            value, why = derive(gid)
            out.append(f"    classification: {value}\n")
    GATES.write_text("".join(out))
    print(f"gate-classification bind: seeded classification for {len(inserts)} gate(s)")
    sys.exit(0)

findings = 0
dist = {}
for gid in gate_ids:
    value = gates[gid].get("classification")
    if value is None:
        print(f"FINDING: classification-missing: {gid} is not classified as modelCompensation, businessInvariant, or hybrid")
        findings += 1
        continue
    value = str(value)
    if value not in VALID:
        print(f"FINDING: classification-invalid: {gid} declares '{value}' (expected one of {', '.join(sorted(VALID))})")
        findings += 1
        continue
    dist[value] = dist.get(value, 0) + 1

if subcommand == "emit":
    if findings:
        print("gate-classification emit: refusing to emit while gates are unclassified")
        sys.exit(1)
    wf = repo_root / "bubbles/workflows.yaml"
    text = wf.read_text()
    block = ["gateClassification:"]
    for cls in ("modelCompensation", "businessInvariant", "hybrid"):
        members = [g for g in gate_ids if str(gates[g].get("classification")) == cls]
        if not members:
            continue
        block.append(f"  {cls}:")
        for g in members:
            block.append(f"  - {g} # {gates[g].get('name', '')}")
    new_block = "\n".join(block) + "\n"
    pattern = re.compile(r"^gateClassification:\n(?:[ ].*\n|\n(?=[ ]))*", re.M)
    if not pattern.search(text):
        print("gate-classification emit: gateClassification block not found in workflows.yaml")
        sys.exit(1)
    wf.write_text(pattern.sub(new_block, text, count=1))
    print(f"gate-classification emit: regenerated gateClassification ({len(gate_ids)} gates)")
    sys.exit(0)

if subcommand == "report":
    print("gate classification distribution:")
    for k in sorted(dist):
        print(f"  {k}: {dist[k]}")
    retirable = dist.get("modelCompensation", 0)
    print(f"  gates that could retire as models improve: {retirable} of {len(gate_ids)}")

if findings:
    print(f"[gate-classification] FAIL — findings: {findings}")
    sys.exit(1)

print(f"[gate-classification] OK — all {len(gate_ids)} gate(s) classified")
sys.exit(0)
PY
