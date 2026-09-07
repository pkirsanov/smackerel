#!/usr/bin/env python3
"""Render Bubbles' Copilot-format agents/prompts/skills/instructions into
Claude Code's native subagent + slash-command format.

Source of truth stays the Copilot-format files under <source>/agents,
<source>/prompts, <source>/skills, <source>/instructions, and the three
registries that already define mode -> phase -> agent ownership:
  - <source>/bubbles/agent-capabilities.yaml   (ownsPhases, workflowModeGrants)
  - <source>/bubbles/registry/required-specialists.yaml (mode -> phases)
  - <source>/bubbles/workflows/modes.yaml      (mode -> phaseOrder, fallback)

This script only renders a second output target (.claude/) alongside the
existing .github/ one. It never modifies .github/ or the source files.

Requires: yq (mikefarah v4+) to read the YAML registries as JSON, python3 stdlib
for everything else (no PyYAML dependency).
"""
from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

FRONTMATTER_RE = re.compile(r"^---\n(.*?)\n---\n(.*)$", re.DOTALL)
AGENT_FILE_RE = re.compile(r"^bubbles\.(.+)\.agent\.md$")
HANDOFF_AGENT_RE = re.compile(r"^\s*agent:\s*(bubbles\.[a-zA-Z0-9_-]+)\s*$", re.MULTILINE)

# Coarse translation from Copilot's coarse tools: vocabulary to Claude Code
# tool names. "agent" is handled separately via the computed Agent(...) grant.
TOOL_MAP = {
    "read": ["Read", "Glob", "Grep"],
    "search": ["Grep", "Glob"],
    "edit": ["Edit", "Write"],
    "todo": ["TodoWrite"],
    "web": ["WebFetch", "WebSearch"],
    "execute": ["Bash"],
    "bubbles": ["mcp__bubbles"],
    "playwright": ["mcp__playwright"],
}

DEFAULT_RUNNER_BASELINE = [
    "Read", "Grep", "Glob", "Edit", "Write", "Bash", "TodoWrite",
    "WebFetch", "WebSearch", "mcp__bubbles",
]


def slug(agent_key: str) -> str:
    """bubbles.plan -> bubbles-plan (Claude Code subagent names are lowercase+hyphen only)."""
    return agent_key.replace(".", "-")


def yq_json(path: Path):
    proc = subprocess.run(["yq", "-o=json", str(path)], capture_output=True, text=True)
    if proc.returncode != 0:
        sys.exit(f"render_claude_code: yq failed reading {path}:\n{proc.stderr}")
    return json.loads(proc.stdout)


def parse_agent_md(path: Path):
    text = path.read_text(encoding="utf-8")
    m = FRONTMATTER_RE.match(text)
    if not m:
        sys.exit(f"render_claude_code: {path} has no --- frontmatter block")
    fm_text, body = m.group(1), m.group(2)

    description = None
    disable_model_invocation = False
    source_tools: list[str] = []
    for line in fm_text.splitlines():
        if line.startswith("description:"):
            description = line.split(":", 1)[1].strip().strip('"').strip("'")
        elif line.startswith("disable-model-invocation:"):
            disable_model_invocation = line.split(":", 1)[1].strip().lower() == "true"
        elif line.startswith("tools:"):
            bracket = re.search(r"\[(.*)\]", line)
            if bracket:
                source_tools = [t.strip() for t in bracket.group(1).split(",") if t.strip()]

    if not description:
        sys.exit(f"render_claude_code: {path} missing description: in frontmatter")

    handoff_targets = sorted(set(HANDOFF_AGENT_RE.findall(fm_text)))

    return {
        "description": description,
        "disable_model_invocation": disable_model_invocation,
        "source_tools": source_tools,
        "handoff_targets": handoff_targets,
        "body": body.strip("\n"),
    }


def resolve_mode_phases(mode: str, required_specialists: dict, modes_yaml: dict) -> list[str]:
    if mode in required_specialists:
        return required_specialists[mode]
    node = modes_yaml.get(mode)
    if not node:
        return []
    return node.get("phaseOrder", [])


def build_phase_owners(agents_meta: dict) -> dict[str, list[str]]:
    """phase -> [agent_key, ...] ; a phase may legitimately have more than one owner
    (e.g. 'bootstrap' is owned by both bubbles.design and bubbles.plan) so this
    unions rather than picking one, erring toward granting availability."""
    owners: dict[str, list[str]] = {}
    for agent_key, meta in agents_meta.items():
        for phase in (meta.get("ownsPhases") or []):
            owners.setdefault(phase, [])
            if agent_key not in owners[phase]:
                owners[phase].append(agent_key)
    return owners


def compute_runner_allowlist(runner_key, grant, required_specialists, modes_yaml, phase_owners, all_modes, pure_top_level):
    modes = grant.get("modes", [])
    excluded = set(grant.get("excludedModes", []))
    candidate_modes = all_modes if modes == ["*"] else modes
    candidate_modes = [m for m in candidate_modes if m not in excluded]

    agents: set[str] = set()
    for mode in candidate_modes:
        for phase in resolve_mode_phases(mode, required_specialists, modes_yaml):
            for owner in phase_owners.get(phase, []):
                # "one dispatching agent per run: the active top-level runner"
                # (workflow-delegation-core.md) — a domain/phase-owning runner
                # (e.g. bubbles.stabilize, bubbles.train) is a legitimate dispatch
                # target, but the three modes:["*"] top-level runners never are,
                # even when they nominally own a bookend phase like "finalize".
                if owner != runner_key and owner not in pure_top_level:
                    agents.add(owner)
    return agents


def render_agent_and_command(
    agent_key: str,
    parsed: dict,
    grants: dict,
    required_specialists: dict,
    modes_yaml: dict,
    phase_owners: dict,
    all_modes: list[str],
    pure_top_level: set[str],
    agents_dir: Path,
    commands_dir: Path,
):
    slug_name = slug(agent_key)
    description = parsed["description"]
    has_agent_tool = "agent" in parsed["source_tools"]
    grant = grants.get(agent_key)

    agent_targets: set[str] = set()
    if grant is not None:
        agent_targets |= compute_runner_allowlist(
            agent_key, grant, required_specialists, modes_yaml, phase_owners, all_modes, pure_top_level
        )
    if has_agent_tool:
        # Agents whose Copilot source explicitly grants "agent" (dispatch/handoff
        # capability) keep that capability here even if they're not a workflow-mode
        # runner (e.g. bubbles.super routes to runners/utilities via handoffs, not
        # phase ownership) — union in its own curated handoff targets as the signal.
        agent_targets |= set(parsed["handoff_targets"]) - {agent_key}

    is_runner_like = grant is not None or has_agent_tool

    if is_runner_like:
        if parsed["source_tools"]:
            base_tools: list[str] = []
            for t in parsed["source_tools"]:
                base_tools.extend(TOOL_MAP.get(t, []))
        else:
            base_tools = list(DEFAULT_RUNNER_BASELINE)
        # de-dupe, keep order
        seen = set()
        base_tools = [t for t in base_tools if not (t in seen or seen.add(t))]

        if agent_targets:
            agent_grant = f"Agent({', '.join(slug(a) for a in sorted(agent_targets))})"
            tools_value = ", ".join([agent_grant] + base_tools)
        else:
            tools_value = ", ".join(base_tools)
        tools_frontmatter = f"tools: {tools_value}\n"
    else:
        tools_frontmatter = "disallowedTools: [Agent]\n"

    agent_md = (
        "---\n"
        f"name: {slug_name}\n"
        f"description: {json.dumps(description)}\n"
        "model: inherit\n"
        f"{tools_frontmatter}"
        "---\n\n"
        f"{parsed['body']}\n"
    )
    (agents_dir / f"{slug_name}.md").write_text(agent_md, encoding="utf-8")

    disable_line = "disable-model-invocation: true\n" if parsed["disable_model_invocation"] else ""
    command_md = (
        "---\n"
        f"description: {json.dumps(description)}\n"
        "context: fork\n"
        f"agent: {slug_name}\n"
        "argument-hint: [request]\n"
        f"{disable_line}"
        "---\n\n"
        "$ARGUMENTS\n"
    )
    (commands_dir / f"{slug_name}.md").write_text(command_md, encoding="utf-8")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--source", required=True, help="Bubbles source checkout root (contains agents/, skills/, bubbles/)")
    ap.add_argument("--dest", required=True, help="Downstream project root to write .claude/ into")
    args = ap.parse_args()

    source = Path(args.source).resolve()
    dest = Path(args.dest).resolve()

    if shutil.which("yq") is None:
        sys.exit("render_claude_code: yq (mikefarah v4+) is required to render Claude Code output; install it and re-run")

    agent_caps = yq_json(source / "bubbles/agent-capabilities.yaml")
    required_specialists = yq_json(source / "bubbles/registry/required-specialists.yaml")["modes"]
    modes_yaml = yq_json(source / "bubbles/workflows/modes.yaml")["modes"]

    agents_meta = agent_caps["agents"]
    grants = agent_caps.get("workflowModeGrants", {}).get("agents", {})
    phase_owners = build_phase_owners(agents_meta)
    all_modes = sorted(set(required_specialists) | set(modes_yaml))
    pure_top_level = {k for k, g in grants.items() if g.get("modes") == ["*"]}

    agents_dir = dest / ".claude/agents"
    commands_dir = dest / ".claude/commands"
    agents_dir.mkdir(parents=True, exist_ok=True)
    commands_dir.mkdir(parents=True, exist_ok=True)

    agent_files = sorted((source / "agents").glob("bubbles.*.agent.md"))
    if not agent_files:
        sys.exit(f"render_claude_code: no agents/bubbles.*.agent.md found under {source}")

    current_slugs = {
        slug(f"bubbles.{AGENT_FILE_RE.match(f.name).group(1)}") for f in agent_files
    }

    count = 0
    for agent_file in agent_files:
        m = AGENT_FILE_RE.match(agent_file.name)
        if not m:
            continue
        agent_key = f"bubbles.{m.group(1)}"
        parsed = parse_agent_md(agent_file)
        render_agent_and_command(
            agent_key, parsed, grants, required_specialists, modes_yaml,
            phase_owners, all_modes, pure_top_level, agents_dir, commands_dir,
        )
        count += 1

    # Prune orphaned generated files (renamed/removed upstream source agent) —
    # only ever touches files matching bubbles-*.md, never operator-authored ones.
    for generated_dir in (agents_dir, commands_dir):
        for f in generated_dir.glob("bubbles-*.md"):
            if f.stem not in current_slugs:
                f.unlink()
                print(f"render_claude_code: pruned orphaned {f}")

    skills_src = source / "skills"
    if skills_src.is_dir():
        skills_dst = dest / ".claude/skills"
        skills_dst.mkdir(parents=True, exist_ok=True)
        current_skill_names = {p.name for p in skills_src.glob("bubbles-*") if p.is_dir()}
        for skill_dir in sorted(skills_src.glob("bubbles-*")):
            if skill_dir.is_dir():
                shutil.copytree(skill_dir, skills_dst / skill_dir.name, dirs_exist_ok=True)
        for existing in skills_dst.glob("bubbles-*"):
            if existing.is_dir() and existing.name not in current_skill_names:
                shutil.rmtree(existing)
                print(f"render_claude_code: pruned orphaned {existing}")

    instr_src = source / "instructions"
    if instr_src.is_dir():
        instr_dst = dest / ".claude/instructions"
        instr_dst.mkdir(parents=True, exist_ok=True)
        current_instr_names = {p.name for p in instr_src.glob("bubbles-*.instructions.md")}
        for f in instr_src.glob("bubbles-*.instructions.md"):
            shutil.copy2(f, instr_dst / f.name)
        for existing in instr_dst.glob("bubbles-*.instructions.md"):
            if existing.name not in current_instr_names:
                existing.unlink()
                print(f"render_claude_code: pruned orphaned {existing}")

    print(f"render_claude_code: wrote {count} agent(s) + {count} command(s) under {dest}/.claude/")


if __name__ == "__main__":
    main()
