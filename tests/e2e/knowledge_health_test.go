// T8-05 (SCN-025-24): GET /api/health → knowledge section present when knowledge layer enabled.
// T8-06 (Regression): Existing health fields (api, postgres, nats, ml_sidecar, ollama) unchanged.
//
// Requires full live stack with knowledge layer enabled.
// Run with: ./smackerel.sh test e2e
package e2e
