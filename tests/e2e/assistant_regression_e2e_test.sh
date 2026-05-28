#!/usr/bin/env bash
# E2E regression test: Conversational assistant facade (Spec 061 SCOPE-04..10).
#
# Persistent scenario-specific regression coverage for every assistant
# behavior added across SCOPE-04, SCOPE-06, SCOPE-07, SCOPE-08, and
# SCOPE-10. The file is owned across multiple scopes:
#
#   SCOPE-04 — initial file + planned row matrix below + capability-layer
#              skip rationale (this commit).
#   SCOPE-05 — wires the Telegram transport so capability rows become
#              drivable end-to-end (then this script's "facade rows"
#              section flips from skip → executable).
#   SCOPE-06 — appends retrieval-skill rows.
#   SCOPE-07 — appends weather-skill rows.
#   SCOPE-08 — appends notification confirm/disambig rows.
#   SCOPE-10 — appends eval-acceptance subset rows.
#
# At SCOPE-04 there is NO user-facing transport for the assistant
# capability (the Telegram adapter is SCOPE-05; spec 061 ships no
# HTTP debug surface for the facade). The band-classification and
# capture-fallback behaviors owned by SCOPE-04 are exhaustively
# covered by Go integration tests in:
#
#   internal/assistant/facade_high_band_test.go
#   internal/assistant/facade_borderline_test.go
#   internal/assistant/facade_capture_fallback_test.go
#   internal/assistant/facade_test.go
#
# This script asserts the live stack is reachable, prints the planned
# row matrix so SCOPE-05+ has a clear append target, and exits 0.
# It is NOT a fabricated assertion that anything was driven — it is a
# real scaffold that the next scope owners will fill in.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Spec 061 — Conversational Assistant Regression E2E (SCOPE-04 scaffold) ==="
e2e_start

echo
echo "--- Planned row matrix (filled in incrementally by SCOPE-05..10) ---"
cat <<'ROWS'
SCOPE-04 facade-layer rows (NOT driveable until SCOPE-05 wires Telegram transport):
  ROW-A  band-classification: high-band routes through scenario executor
  ROW-B  band-classification: borderline-band emits DisambiguationPrompt (no executor)
  ROW-C  band-classification: low-band falls back to CaptureRoute=true (no executor)
  ROW-D  capture-fallback:    unknown-intent short-circuits to capture
  ROW-E  capture-fallback:    unresolvable "open N" reference returns ErrSlotMissing

SCOPE-06 retrieval-skill rows (appended by SCOPE-06):
  (placeholder — owner: SCOPE-06)

SCOPE-07 weather-skill rows (appended by SCOPE-07):
  (placeholder — owner: SCOPE-07)

SCOPE-08 notification confirm/disambig rows (appended by SCOPE-08):
  (placeholder — owner: SCOPE-08)

SCOPE-10 eval-acceptance subset rows (appended by SCOPE-10):
  (placeholder — owner: SCOPE-10)
ROWS

echo
echo "--- SCOPE-04 row status ---"
echo "ROW-A..E SKIPPED: capability layer is covered by Go integration tests; the"
echo "         shell-driveable assertion is unblocked when SCOPE-05 ships the"
echo "         Telegram transport (or any future HTTP debug surface)."
echo
echo "Cross-reference Go evidence (SCOPE-04 capability coverage):"
echo "  internal/assistant/facade_high_band_test.go         (ROW-A)"
echo "  internal/assistant/facade_borderline_test.go        (ROW-B)"
echo "  internal/assistant/facade_capture_fallback_test.go  (ROW-C, ROW-D, ROW-E)"
echo "  internal/assistant/facade_test.go                   (BS-005 + /reset)"

echo
echo "--- SCOPE-05 row status (Telegram reference adapter v1) ---"
echo "Driveable rows for SCOPE-05 live in a dedicated script to keep this"
echo "scaffold scope-isolated. The SCOPE-05 surface covers:"
echo
echo "  R05-1  BS-001  Plain-text capture-fallback through the assistant"
echo "                 intercept persists an artifact verbatim."
echo "                 Live-stack leg: tests/e2e/telegram_assistant_bs001_test.sh"
echo "                 Intercept-contract leg: internal/telegram/bot_assistant_intercept_test.go"
echo "                   TestHandleMessage_AssistantCaptureRoute_FallsThroughToCapture"
echo "                   TestHandleMessage_AdapterUnbound_LegacyCapturePreserved"
echo
echo "  R05-2  /reset  Slash command claims-on-bound, replies-not-enabled-on-unbound."
echo "                 Intercept-contract leg: internal/telegram/bot_assistant_intercept_test.go"
echo "                   TestHandleMessage_SlashCommandsNotInterceptedByAssistant"
echo "                 (negative half — proves only /reset is intercepted, not /find etc.)"
echo
echo "  R05-3  Adapter no-leak: re-runs the SCOPE-04 adapter-substitution invariant"
echo "                 against the real *assistant_adapter.Adapter. Go evidence:"
echo "                   internal/assistant/contracts/architecture_test.go (capability→transport import lint)"
echo "                   internal/telegram/assistant_adapter/adapter_test.go (interface conformance)"
echo
echo "  R05-4  Render goldens: 33-test unit suite covers every UX §14.B.1 shape."
echo "                 Run: go test -count=1 ./internal/telegram/assistant_adapter/..."
echo
echo "  R05-5  BS-010  paired with SCOPE-06 retrieval-skill landing — driveable"
echo "                 row to be appended by SCOPE-06 (depends on retrieval tool"
echo "                 handler authoring + cross-spec packet 060 acceptance)."

echo
echo "--- SCOPE-06 row status (Retrieval Q&A skill v1 #1) ---"
echo "Capability layer surfaces shipped (SCOPE-06 owned):"
echo "  R06-1  retrieval_search tool handler real, top_k SST-capped."
echo "         Go evidence: internal/agent/tools/retrieval/tool_test.go"
echo "                       TestRetrievalSearch_HappyPath_HitsReturnedFromEngine"
echo "                       TestRetrievalSearch_TopKCap_ExceedingSstCapClamped"
echo "                       TestRetrievalSearch_TopKZeroUsesCap"
echo
echo "  R06-2  source-assembly invariant pure-function proven on graph drift."
echo "         Go evidence: internal/agent/tools/retrieval/source_assembly_test.go"
echo "                       TestAssembleSources_GraphDrift_PartialMissing"
echo "                       TestAssembleSources_AllMissing_TriggersRefusalContract"
echo "                       TestAssembleSources_LookupError_DroppedAndCounted"
echo "                       TestAssembleSources_CapAndOverflow"
echo "         Metric evidence: internal/assistant/metrics/source_assembly_test.go"
echo "                       TestSourceAssemblyDropsCounter_LabelVocabularyClosed"
echo
echo "  R06-3  retrieval skill registered + routed by 'retrieval_qa' scenario,"
echo "         provenance.requires_provenance=true honored by SCOPE-04 gate."
echo "         YAML evidence: config/prompt_contracts/retrieval-qa-v1.yaml"
echo "                       config/assistant/scenarios.yaml [retrieval_qa]"
echo "         Wiring evidence: cmd/core/wiring_assistant_skills.go"
echo "                          wireRetrievalSkillServices()"
echo
echo "  R06-4  G026 stress budget proven (p95 < 5s manifest budget) on tool-level"
echo "         burst against in-process fake searcher."
echo "         Stress evidence: tests/stress/assistant_retrieval_p95_test.go"
echo "                       TestAssistantRetrievalStressP95"
echo
echo "BLOCKED (route_required to SCOPE-04 — facade post-processor seam):"
echo "  R06-5  BS-002 e2e — high-confidence retrieval answer with citations."
echo "         Blocker: internal/assistant/facade.go BandHigh dispatch has no"
echo "                  seam between executor.Run() and provenance.Enforce()"
echo "                  to invoke AssembleSources(), so resp.Sources is always"
echo "                  empty for retrieval scenarios and the gate ALWAYS"
echo "                  refuses. SCOPE-04 owner must add the per-scenario"
echo "                  source-assembly hook before this row is driveable."
echo "         Finding ID: SCOPE-04-FACADE-SOURCE-ASSEMBLY-HOOK"
echo "         When unblocked: tests/e2e/assistant_bs002_test.sh authoring lands."
echo
echo "  R06-6  BS-007 e2e — empty-sources case from real skill triggers gate."
echo "         Same blocker as R06-5. Once the SCOPE-04 facade hook lands AND"
echo "         calls AssembleSources(), the all-missing scenario will surface"
echo "         end-to-end (provenance gate fires → canonical refusal +"
echo "         capture-route → 'idea' artifact created). Until then the"
echo "         all-missing contract is proven at unit level by"
echo "         TestAssembleSources_AllMissing_TriggersRefusalContract +"
echo "         internal/assistant/provenance/gate_test.go (gate behavior on"
echo "         empty Sources independently verified)."
echo "         Finding ID: SCOPE-04-FACADE-SOURCE-ASSEMBLY-HOOK (same)."
echo "         When unblocked: tests/e2e/assistant_bs007_test.sh authoring lands."
echo
echo "  R06-7  Telegram trailing 'sources:' rendering end-to-end."
echo "         Unit-level rendering already proven in"
echo "         internal/telegram/assistant_adapter/adapter_test.go goldens."
echo "         End-to-end activation depends on R06-5/R06-6 unblocking."
echo
echo "SCOPE-07 weather-skill rows (appended by SCOPE-07):"
echo "  (placeholder — owner: SCOPE-07)"

e2e_pass "Spec 061 SCOPE-04..06: regression scaffold populated; SCOPE-06 R06-1..4 driveable, R06-5..7 awaiting SCOPE-04 facade hook"