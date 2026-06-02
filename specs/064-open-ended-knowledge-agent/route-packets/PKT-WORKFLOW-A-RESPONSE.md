# PKT-WORKFLOW-A — RESPONSE (accepted as known drift)

- **Date:** 2026-06-02
- **Authorized by:** workflow owner (rescope decision)
- **Disposition:** Accepted as known drift; remaining findings transferred to downstream spec ownership.

## Decision

PKT-WORKFLOW-A raised six infrastructure findings blocking SCOPE-17
end-to-end execution. Finding #1 (`ml/app/routes/chat.py` real Ollama
dispatch path) was resolved in-session on 2026-05-31 — gemma3:4b
end_turn + tool_use turns succeed end-to-end against the dev stack,
and 475 ml tests pass.

Findings #2–#6 are **accepted as known drift** and transferred to
their respective owner specs:

| # | Finding                                                                 | Owner             |
|---|-------------------------------------------------------------------------|-------------------|
| 2 | `/v1/agent/invoke` capture-as-fallback wiring                           | spec 061 successor |
| 3 | `fixture-fabricated-cite` test mode in `chat.py` (adversarial G021 path)| spec 064 SCOPE-08 successor |
| 4 | Per-test per-query token-budget override knob                           | spec 064 SCOPE-09 successor |
| 5 | Per-test tool-allowlist override knob                                   | spec 064 SCOPE-09 successor |
| 6 | `smackerel.sh test e2e` exports `AGENT_INVOKE_URL` to Go container      | spec 023 successor |

These items are out-of-scope for spec 064 close-out and will be picked
up in successor specs. The E2E scaffolding shipped in
`tests/e2e/agent/openknowledge_e2e_test.go` (7 test functions covering
UC-064-A01..A06 + adversarial G021 fabricated-source) honestly skips
with explicit per-finding messages and `go vet -tags e2e` is clean;
each test activates automatically when its blocking finding lands in
the successor spec.

## Impact on spec 064

- Spec 064 SCOPE-17 transitions to `Done` with a known-drift
  annotation in its scope status (cross-spec findings now routed via
  this response packet).
- `state.json.transitionRequests[PKT-WORKFLOW-A].status` flips to
  `closed`.
- `state.json.reworkQueue[PKT-WORKFLOW-A-await].status` flips to
  `resolved`.

## Receiving-spec action

The successor specs listed above own findings #2–#6. The spec 064
E2E scaffolding remains in place and requires no SCOPE-17-side change
when those findings land.
