# Bubbles Framework Change Proposal

- Title: Adopt structural YAML rewrite for goal/sprint/iterate/workflow/bug agents
- Slug: framework-agents-rewrite
- Created: 2026-04-26
- Created From: smackerel
- Requested Upstream Repo: bubbles

## Summary

Adopt the structural YAML rewrite of five orchestrator/router agents (`bubbles.bug`, `bubbles.goal`, `bubbles.iterate`, `bubbles.sprint`, `bubbles.workflow`) into the upstream Bubbles framework. The rewrite replaces freeform prose instructions with a deterministic YAML body that declares an explicit tool allowlist and a phase router, eliminating ambiguity about which specialists may be dispatched and in what order. The current framework checksums for these files no longer match, and downstream `cli.sh doctor` reports persistent framework-managed file drift even though the local intent is correct and committed.

## Why This Must Be Upstream

These agents are listed in the Bubbles `.checksums` manifest and explicitly enforced by `bash .github/bubbles/scripts/cli.sh doctor` as **framework-managed files** that downstream repos "must not directly author changes in." Doctor's own remediation guidance (`.github/bubbles/scripts/cli.sh` line 200, `require_downstream_repo_for_framework_proposal`) is to file an upstream proposal rather than carry permanent local divergence. The fix cannot live in `.github/bubbles-project.yaml` or `specs/` because those project-owned surfaces do not override agent definitions — agents are loaded from `.github/agents/` and validated against framework checksums regardless of project config.

## Current Downstream Limitation

Doctor permanently reports `1 failed` (5 framework-managed file drift errors) every run in this consumer repo (smackerel). This:

1. Masks any *new* drift that might appear in unrelated framework files — operators learn to ignore the failure.
2. Blocks clean status output for `/bubbles.status` and `/bubbles.setup` refresh flows.
3. Forces every refresh of the framework layer to re-overwrite the locally-rewritten agents, which then must be re-applied by hand.
4. Creates ambiguity about whether the rewritten agents will be supported by future framework changes (e.g., new phase router schema, new ownership-lint rules).

## Proposed Bubbles Change

Adopt the rewritten agent bodies from this consumer repo into the upstream Bubbles source, regenerate `.github/bubbles/.checksums`, and ship via the standard install/refresh path.

Concretely:

1. Copy the five rewritten files from this repo into the Bubbles source repo at the equivalent paths under its agents/ directory.
2. Re-run the upstream checksum generator so the new bodies become the canonical framework-managed checksums.
3. Verify upstream `agent-ownership-lint.sh`, `agent-customization` post-apply validation (YAML frontmatter, handoff target existence, no circular handoffs, ≤200 char descriptions, shared pattern reference, agent-common.md presence) all pass against the new bodies.
4. Cut a Bubbles patch release (e.g. 3.6.2) that includes the new files and updated `.checksums`.
5. Document the structural YAML body convention (tool allowlist + phase router + zero prose) in upstream Bubbles agent authoring docs so future framework agents follow the same pattern.

## Affected Framework Paths

The five drifted files (downstream paths shown; upstream paths are equivalent under the Bubbles source repo's agents/ tree):

| Downstream path | Expected checksum (current upstream) | Actual checksum (rewritten local body) | Originating local commit |
|---|---|---|---|
| `.github/agents/bubbles.bug.agent.md` | `48c154d2…b5cbef` | `79fe31df…b19b77` | `e2effa8` chore(bubbles): framework drift sync |
| `.github/agents/bubbles.goal.agent.md` | `5e6b7ac3…ba5103c` | `9cbf9d28…2cb286bf` | `c346efd` bubbles: structural YAML rewrite for goal+sprint agents — tool allowlist, phase router, zero prose |
| `.github/agents/bubbles.iterate.agent.md` | `c07e4ca2…2795a754d1` | `4aa321d4…afdaeb24f9` | `e2effa8` chore(bubbles): framework drift sync |
| `.github/agents/bubbles.sprint.agent.md` | `e94e6719…2fd31af2` | `bf894c78…44afc3016` | `c346efd` bubbles: structural YAML rewrite for goal+sprint agents — tool allowlist, phase router, zero prose |
| `.github/agents/bubbles.workflow.agent.md` | `9148a84f…d05dd2473` | `b02880b2…368d7fd48f` | `e2effa8` chore(bubbles): framework drift sync |

Also affected once adopted:

- `.github/bubbles/.checksums` — regenerated to record the new canonical bodies
- `.github/bubbles/release-manifest.json` — version bump (e.g., `3.6.1` → `3.6.2`)
- Upstream Bubbles authoring docs (new convention guidance for orchestrator agent YAML bodies)

## Expected Downstream Outcome

After the upstream Bubbles change ships and this repo runs the standard refresh flow:

1. `bash .github/bubbles/scripts/cli.sh doctor` reports `0 failed` for framework-managed file drift on these five files.
2. Local agents continue to behave as they do today (no behavior change in the consumer repo).
3. Future framework refreshes do not re-overwrite the rewritten agents — they are now the canonical upstream bodies.
4. Other downstream repos that adopt the new Bubbles release inherit the structural YAML convention for these orchestrator/router agents automatically.
5. `/bubbles.setup` refresh in this repo reports clean drift status.

## Acceptance Criteria

- [ ] Upstream Bubbles source repo contains the rewritten bodies for `bubbles.bug`, `bubbles.goal`, `bubbles.iterate`, `bubbles.sprint`, `bubbles.workflow`
- [ ] Upstream `.checksums` regenerated; new checksums match the actual values listed in the table above
- [ ] Upstream agent-ownership-lint passes against the new bodies
- [ ] Upstream YAML frontmatter validation passes (description ≤200 chars, valid handoff targets, no circular handoffs, references `agent-common.md`)
- [ ] Bubbles patch release published (e.g., 3.6.2) with these changes
- [ ] Installer/refresh flow distributes the change to consumer repos
- [ ] After running the refresh in smackerel, `bash .github/bubbles/scripts/cli.sh doctor` exits `0` with no framework-managed file drift on these five paths
- [ ] Upstream agent-authoring docs document the structural YAML body convention (tool allowlist + phase router + zero prose)

## Notes

- Do not edit `.github/bubbles/**`, `.github/agents/bubbles*`, or other framework-managed files locally.
- Implement the framework fix in the Bubbles source repo, then refresh this repo via install/refresh.
- The rewritten bodies are already committed in this consumer repo (`c346efd`, `e2effa8`) and may be lifted directly as the upstream contribution.
- Until adoption, the framework drift on these five files is **expected and accepted** — operators should not revert the local bodies, as doing so would discard intentional design (deterministic tool allowlists and phase routers).
