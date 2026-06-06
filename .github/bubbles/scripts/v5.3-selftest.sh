#!/usr/bin/env bash
#
# bubbles/scripts/v5.3-selftest.sh
#
# Selftest for v5.3 / G1: framework-validate runs cleanly from a downstream
# install tree.
#
# Asserts:
#   T1. framework-validate detects install-mode=downstream when run from a
#       synthesized `.github/`-style tree (no `install.sh` / `VERSION` at
#       the repo root).
#   T2. framework-validate detects install-mode=source when run from the
#       framework source repo (the tree we're in).
#   T3. The 9 framework-source-only selftests SKIP cleanly (do not FAIL)
#       under install-mode=downstream. Names checked: capability-ledger,
#       capability-freshness, competitive-docs, interop-apply,
#       release-manifest-freshness, release-manifest-selftest,
#       release-manifest-purity, install-provenance, trust-doctor.
#   T4. spec-review-handoff-selftest runs and passes under a synthesized
#       downstream tree (proves the per-selftest dual-resolve path).
#   T5. workflow-delegation-selftest runs and passes under a synthesized
#       downstream tree (proves the per-selftest dual-resolve path).
#
# Exit 0 = all assertions pass. Exit 1 = at least one failed.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

# --- T2: source-mode detection on the framework repo itself ---
if [[ -f "$ROOT_DIR/install.sh" && -f "$ROOT_DIR/VERSION" ]]; then
  src_out="$(bash "$SCRIPT_DIR/framework-validate.sh" 2>&1 | head -5 || true)"
  if grep -q "Install mode: source" <<<"$src_out"; then
    pass "T2: framework-validate reports install-mode=source from framework repo"
  else
    fail "T2: framework-validate did NOT report install-mode=source from framework repo (head: ${src_out:0:200})"
  fi
else
  echo "SKIP: T2 (this selftest is not running from a framework source tree)"
fi

# Build a minimal downstream-shaped tree that contains just enough for the
# install-mode detector to report "downstream" and just enough for the three
# selftests we want to exercise.
tmp_root="$(mktemp -d -t bubbles-v5.3-selftest.XXXXXX)"
trap 'rm -rf "$tmp_root"' EXIT INT TERM

mkdir -p "$tmp_root/.github/bubbles/scripts"
mkdir -p "$tmp_root/.github/bubbles/schemas"
mkdir -p "$tmp_root/.github/bubbles/registry"
mkdir -p "$tmp_root/.github/agents/bubbles_shared"
mkdir -p "$tmp_root/.github/docs/guides"
mkdir -p "$tmp_root/.github/docs/generated"

# Copy ONLY the scripts the v5.3 selftest exercises. We do NOT copy install.sh
# / VERSION / README.md into the synthesized tree (that's the whole point of
# the downstream-install fixture).
for s in framework-validate.sh spec-review-handoff-selftest.sh \
         workflow-delegation-selftest.sh \
         capability-ledger-selftest.sh capability-freshness-selftest.sh \
         competitive-docs-selftest.sh interop-apply-selftest.sh \
         generate-release-manifest.sh release-manifest-selftest.sh \
         release-manifest-purity-selftest.sh install-provenance-selftest.sh \
         trust-doctor-selftest.sh runtime-lease-selftest.sh \
         trust-metadata.sh; do
  if [[ -f "$ROOT_DIR/bubbles/scripts/$s" ]]; then
    cp "$ROOT_DIR/bubbles/scripts/$s" "$tmp_root/.github/bubbles/scripts/$s"
    chmod +x "$tmp_root/.github/bubbles/scripts/$s"
  fi
done

# install-source.json sentinel — what install.sh writes downstream.
cat >"$tmp_root/.github/bubbles/.install-source.json" <<'EOF'
{ "source": "synthetic-v5.3-selftest", "sourceGitSha": "0000000000000000000000000000000000000000" }
EOF

# Asset fixtures needed by the three downstream-resolvable selftests.
# spec-review-handoff-selftest.sh reads from agents/ + bubbles/workflows.yaml.
# workflow-delegation-selftest.sh reads from docs/guides/WORKFLOW_MODES.md
# and bubbles/{workflows.yaml, agent-capabilities.yaml} too.
# We just copy the real ones from the framework source.
mkdir -p "$tmp_root/.github/bubbles"
for f in workflows.yaml agent-capabilities.yaml; do
  if [[ -f "$ROOT_DIR/bubbles/$f" ]]; then
    cp "$ROOT_DIR/bubbles/$f" "$tmp_root/.github/bubbles/$f"
  fi
done
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml,
# not inline in workflows.yaml. The mode-resolver composes the two, so the
# synthesized tree MUST carry the workflows/ registry dir (modes.yaml +
# aliases.yaml) or mode resolution returns nothing downstream.
if [[ -d "$ROOT_DIR/bubbles/workflows" ]]; then
  mkdir -p "$tmp_root/.github/bubbles/workflows"
  cp -R "$ROOT_DIR/bubbles/workflows/." "$tmp_root/.github/bubbles/workflows/"
fi
if [[ -f "$ROOT_DIR/docs/guides/WORKFLOW_MODES.md" ]]; then
  cp "$ROOT_DIR/docs/guides/WORKFLOW_MODES.md" "$tmp_root/.github/docs/guides/WORKFLOW_MODES.md"
fi

# Agents tree (selftests grep into these markdown files).
agent_files=(
  bubbles.workflow.agent.md
  bubbles.super.agent.md
  bubbles.iterate.agent.md
  bubbles.goal.agent.md
  bubbles.sprint.agent.md
  bubbles.bug.agent.md
  bubbles.spec-review.agent.md
)
for f in "${agent_files[@]}"; do
  if [[ -f "$ROOT_DIR/agents/$f" ]]; then
    cp "$ROOT_DIR/agents/$f" "$tmp_root/.github/agents/$f"
  fi
done
for f in workflow-delegation-core.md workflow-orchestration-core.md workflow-input-bootstrap.md; do
  if [[ -f "$ROOT_DIR/agents/bubbles_shared/$f" ]]; then
    cp "$ROOT_DIR/agents/bubbles_shared/$f" "$tmp_root/.github/agents/bubbles_shared/$f"
  fi
done

# --- T1: downstream-mode detection ---
ds_out="$(bash "$tmp_root/.github/bubbles/scripts/framework-validate.sh" 2>&1 | head -5 || true)"
if grep -q "Install mode: downstream" <<<"$ds_out"; then
  pass "T1: framework-validate reports install-mode=downstream from synthesized .github/ tree"
else
  fail "T1: framework-validate did NOT report install-mode=downstream (head: ${ds_out:0:200})"
fi

# --- T3: framework-source-only selftests SKIP under downstream mode ---
ds_full="$(bash "$tmp_root/.github/bubbles/scripts/framework-validate.sh" 2>&1 || true)"
self_only_labels=(
  "Capability ledger selftest"
  "Capability freshness selftest"
  "Competitive docs selftest"
  "Interop apply selftest"
  "Release manifest freshness"
  "Release manifest selftest"
  "Release manifest purity selftest"
  "Install provenance selftest"
  "Trust doctor selftest"
  "Portable surface agnosticity"
  "Cheatsheet generator selftest (v6.0 / B7)"
  "Installer manifest check (v6.0 / B9)"
  "Installer manifest selftest (v6.0 / B9)"
)
t3_failures=0
for label in "${self_only_labels[@]}"; do
  if grep -Fq "SKIP: $label (framework-source-only" <<<"$ds_full"; then
    :
  else
    fail "T3: '$label' was not SKIPPED under install-mode=downstream"
    t3_failures=$((t3_failures + 1))
  fi
done
if [[ $t3_failures -eq 0 ]]; then
  pass "T3: all ${#self_only_labels[@]} framework-source-only selftests SKIPPED under install-mode=downstream"
fi

# Also assert no FAIL line for those same labels (defense against silent regression).
for label in "${self_only_labels[@]}"; do
  if grep -Fq "FAIL: $label" <<<"$ds_full"; then
    fail "T3b: '$label' FAILED instead of SKIPPING under install-mode=downstream"
  fi
done

# --- T4: spec-review-handoff-selftest passes under downstream tree ---
sr_out="$(bash "$tmp_root/.github/bubbles/scripts/spec-review-handoff-selftest.sh" 2>&1 || true)"
sr_rc=$?
if [[ $sr_rc -eq 0 ]] && grep -q "spec-review-handoff-selftest: PASSED" <<<"$sr_out"; then
  pass "T4: spec-review-handoff-selftest passes under synthesized downstream tree"
else
  fail "T4: spec-review-handoff-selftest FAILED under downstream tree (rc=$sr_rc; tail: $(tail -3 <<<"$sr_out"))"
fi

# --- T5: workflow-delegation-selftest passes under downstream tree ---
wd_out="$(bash "$tmp_root/.github/bubbles/scripts/workflow-delegation-selftest.sh" 2>&1 || true)"
wd_rc=$?
if [[ $wd_rc -eq 0 ]] && grep -q "workflow-delegation selftest passed" <<<"$wd_out"; then
  pass "T5: workflow-delegation-selftest passes under synthesized downstream tree"
else
  fail "T5: workflow-delegation-selftest FAILED under downstream tree (rc=$wd_rc; tail: $(tail -3 <<<"$wd_out"))"
fi

if [[ $failures -gt 0 ]]; then
  echo
  echo "v5.3-selftest FAILED with $failures issue(s)."
  exit 1
fi

echo
echo "v5.3-selftest passed: framework-validate runs cleanly from a downstream install tree."
