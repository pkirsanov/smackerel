#!/usr/bin/env bash
# Gate attribution across an agent's effective closure.
# Contract + measured findings: agents/bubbles_shared/operating-baseline.md (R3).
#
# R3 blocks closure reduction until a held-out eval shows zero gate-detection
# regression. On hardware where the closure exceeds the model context, that eval
# cannot see the whole bundle (measured: 114,476 tokens vs a 32,768 window), so a
# whole-bundle verdict is not obtainable.
#
# This answers the decidable part DETERMINISTICALLY instead: which modules are the
# SOLE carrier of a gate reference. Removing such a module orphans that gate — the
# text disappears from the closure entirely — which is the concrete failure R3
# guards against, and it needs no model at all.
#
# SCOPE AND LIMITS, stated plainly:
#   - This counts gate-id REFERENCES, not semantic definitions. A module that only
#     mentions a gate in passing still counts as a carrier.
#   - Zero sole-gates means removal orphans NO gate id. It does NOT prove routing
#     is unaffected; cross-module context can still matter. It narrows candidates,
#     it does not certify them.
#   - A module carrying sole gates is a hard NO for removal. That direction IS
#     conclusive: the reference would simply be gone.
#
# Usage: gate-attribution.sh <agent.md> [--json]
#        gate-attribution.sh <agent.md> --ondemand <mod.md,mod.md,...> [--json]
#
# --ondemand models SCOPE-2's actual proposal: modules move to ON-DEMAND loading,
# they are not deleted. A gate stays safe if its text is still REACHABLE — either
# a carrier remains always-loaded, or an always-loaded file still points at the
# on-demand carrier so the agent knows to fetch it. A gate with no remaining
# carrier and no pointer is UNREACHABLE, and that is the real SCOPE-2 hazard.

set -euo pipefail

AGENT=""
FORMAT="text"
ONDEMAND=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --json) FORMAT="--json"; shift ;;
    --ondemand) ONDEMAND="${2:-}"; shift 2 ;;
    -h | --help) AGENT=""; break ;;
    *) AGENT="$1"; shift ;;
  esac
done

if [[ -z "$AGENT" ]]; then
  cat >&2 <<'EOF'
gate-attribution.sh — which closure modules solely carry a gate reference
Usage: gate-attribution.sh <path/to/agent.md> [--json]
       gate-attribution.sh <path/to/agent.md> --ondemand <mod.md,...> [--json]
EOF
  exit 0
fi

[[ -r "$AGENT" ]] || { echo "gate-attribution: cannot read $AGENT" >&2; exit 1; }

BUBBLES_GA_AGENT="$AGENT" BUBBLES_GA_FORMAT="$FORMAT" BUBBLES_GA_ONDEMAND="$ONDEMAND" python3 - <<'PY'
import collections, json, os, re, sys

agent = os.path.abspath(os.environ["BUBBLES_GA_AGENT"])
as_json = os.environ["BUBBLES_GA_FORMAT"] == "--json"
shared = os.path.join(os.path.dirname(agent), "bubbles_shared")

seen, queue, mods, bodies = set(), [agent], {}, {}
while queue:
    path = queue.pop(0)
    if path in seen or not os.path.isfile(path):
        continue
    seen.add(path)
    body = open(path, encoding="utf-8", errors="replace").read()
    name = os.path.basename(path)
    bodies[name] = body
    mods[name] = (len(body), set(re.findall(r"\bG\d{3}\b", body)))
    for ref in re.findall(r"bubbles_shared/([A-Za-z0-9._-]+)\.md", body):
        queue.append(os.path.join(shared, ref + ".md"))

carriers = collections.defaultdict(set)
for name, (_, gates) in mods.items():
    for gate in gates:
        carriers[gate].add(name)

root = os.path.basename(agent)

ondemand_raw = os.environ.get("BUBBLES_GA_ONDEMAND", "").strip()
if ondemand_raw:
    requested = [m.strip() for m in ondemand_raw.split(",") if m.strip()]
    unknown = [m for m in requested if m not in mods]
    if unknown:
        sys.stderr.write("gate-attribution: not in closure: %s\n" % ", ".join(unknown))
        sys.exit(1)
    if root in requested:
        sys.stderr.write("gate-attribution: the agent file itself cannot be on-demand\n")
        sys.exit(1)

    moved = set(requested)
    core = [m for m in mods if m not in moved]

    unreachable, pointer_only = [], []
    for gate in sorted(carriers):
        remaining = carriers[gate] - moved
        if remaining:
            continue
        # No always-loaded carrier: survives only if some core file points at one.
        pointers = sorted(
            {c for c in core for m in carriers[gate] if m in bodies and m in bodies[c]}
        )
        (pointer_only if pointers else unreachable).append((gate, sorted(carriers[gate]), pointers))

    freed = sum(mods[m][0] for m in moved)
    if as_json:
        print(json.dumps({
            "agent": root, "onDemand": sorted(moved), "bytesFreed": freed,
            "unreachableGates": [{"gate": g, "carriers": c} for g, c, _ in unreachable],
            "pointerOnlyGates": [{"gate": g, "carriers": c, "pointers": p} for g, c, p in pointer_only],
            "safe": not unreachable,
        }, indent=2, sort_keys=True))
        sys.exit(1 if unreachable else 0)

    print(f"agent          : {root}")
    print(f"on-demand      : {', '.join(sorted(moved))}")
    print(f"bytes freed    : {freed}")
    print()
    if unreachable:
        print(f"UNREACHABLE — no always-loaded carrier and no pointer ({len(unreachable)} gates):")
        for g, c, _ in unreachable:
            print(f"  {g}  carried only by: {', '.join(c)}")
        print()
    if pointer_only:
        print(f"REACHABLE VIA POINTER ONLY — agent must follow the link ({len(pointer_only)} gates):")
        for g, c, p in pointer_only:
            print(f"  {g}  in {', '.join(c)}  <- pointed to by {', '.join(p)}")
        print()
    if unreachable:
        print("VERDICT: UNSAFE. Add a pointer from an always-loaded file, or keep the module loaded.")
    else:
        print("VERDICT: every gate stays reachable. Pointer-only gates still need the")
        print("routing eval — reachable is not the same as actually followed.")
    sys.exit(1 if unreachable else 0)

sole = {g: next(iter(m)) for g, m in carriers.items() if len(m) == 1}
owner = collections.Counter(sole.values())

blocked = sorted(
    ((mods[m][0], m, c) for m, c in owner.items() if m != root), reverse=True
)
candidates = sorted(
    ((b, m) for m, (b, _) in mods.items() if m not in owner and m != root), reverse=True
)

if as_json:
    print(json.dumps({
        "agent": root,
        "modules": len(mods),
        "gatesReferenced": len(carriers),
        "soleCarriedGates": len(sole),
        "blockedModules": [{"module": m, "bytes": b, "soleGates": c} for b, m, c in blocked],
        "candidateModules": [{"module": m, "bytes": b} for b, m in candidates],
        "candidateBytes": sum(b for b, _ in candidates),
    }, indent=2, sort_keys=True))
    sys.exit(0)

print(f"agent                : {root}")
print(f"closure modules      : {len(mods)}")
print(f"gate ids referenced  : {len(carriers)}")
print(f"sole-carried gates   : {len(sole)}")
print()
print("NOT REMOVABLE — sole carrier of a gate reference:")
for b, m, c in blocked:
    print(f"  {b:7d}B  {c:3d} sole-gate(s)  {m}")
print()
print(f"CANDIDATES — carry no sole gate ({len(candidates)} modules, {sum(b for b,_ in candidates)} bytes):")
for b, m in candidates:
    print(f"  {b:7d}B  {m}")
print()
print("Candidates are NARROWED, not certified: zero sole-gates proves no gate id is")
print("orphaned, not that routing is unchanged. Confirm each with the routing eval.")
PY
