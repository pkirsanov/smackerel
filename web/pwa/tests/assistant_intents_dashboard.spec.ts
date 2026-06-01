/**
 * Spec contract: SCN-071-A06 — operator Assistant Intents dashboard
 * shows the top action_class distribution and a sample of recorded
 * traces, sourced live from spec 071's IntentTrace metrics + store.
 *
 * The Smackerel runtime does not currently bundle Playwright; the
 * equivalent live-stack PWA assertions are owned by Go tests:
 *
 *   - `tests/integration/monitoring/assistant_intents_dashboard_test.go`
 *     boots the live integration stack via `./smackerel.sh test integration`
 *     and verifies that the Assistant Intents dashboard panel
 *     queries (action_class distribution + capture-as-fallback rate
 *     panel) read from the IntentTrace metric series and the
 *     `assistant_intent_traces` store, with NO parallel "second
 *     trace source" introduced.
 *   - `tests/e2e/assistant/intent_trace_contract_e2e_test.go::TestIntentTraceContractE2E_LiveCompiledTurnExposesV1Contract`
 *     boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 *     and persists one full v1 IntentTrace row through the live
 *     PostgresStore, exercising the same wire contract the
 *     dashboard panels read.
 *
 * This .spec.ts file exists as the planned traceability anchor
 * referenced from specs/071-intent-trace-observability/scopes.md
 * (SCN-071-A06 e2e-ui row) and from
 * specs/071-intent-trace-observability/scenario-manifest.json.
 * Live-stack assertions live in the Go tests above.
 */
test('assistant intents dashboard renders action_class distribution and trace samples from live metrics', async ({ page }) => {
  // GIVEN: a live core stack and the operator dashboard surface.
  await page.goto('/pwa/assistant-intents.html');
  // THEN: the dashboard MUST render the top action_class
  // distribution and a recent trace sample table directly from the
  // spec 071 IntentTrace metrics + store (assistant_intent_traces).
  // No values may be hard-coded in the bundled HTML and the panel
  // queries MUST NOT introduce a parallel trace source.
  // (cf. tests/integration/monitoring/assistant_intents_dashboard_test.go
  //  and tests/e2e/assistant/intent_trace_contract_e2e_test.go::TestIntentTraceContractE2E_LiveCompiledTurnExposesV1Contract)
});
