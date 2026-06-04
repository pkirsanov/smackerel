// Package graphapi provides the shared primitives for the spec 080
// Knowledge Graph Public API (the 8 read-only JSON endpoints declared
// by spec 080 §1). Scopes 02-04 mount the actual handlers; this scope
// (SCOPE-080-01) ships the foundation surfaces every later scope
// consumes.
//
// Foundation primitives shipped here:
//
//   - CrossLink — the explainable cross-link contract
//     {targetKind, targetId, targetLabel, reason}. The reason field is
//     server-derived only; clients MUST NOT synthesize it. See spec 080
//     design.md §2 "Cross-Link Contract".
//   - Cursor codec — opaque, HMAC-signed, version-tagged pagination
//     cursors that survive concurrent inserts. The HMAC key is read
//     from the env var named by knowledge_graph_api.cursor_secret_env
//     (SST, fail-loud, smackerel-no-defaults). See design.md §5.
//   - Limits — fail-loud SST envelope for list / edges / time-window
//     clamps. See design.md §6.
//   - Errors — uniform {error:{code,message,field}} envelope with the
//     closed-set of error codes used across the 8 endpoints. See
//     design.md §8.
//   - Reasons — initial reason taxonomy plus template renderer used by
//     handlers + the resolveEdges resolver (Scope 04). See design.md
//     §2 "Reason Taxonomy".
//
// SCOPE-080-01 ships the package, the config plumbing, and the
// auth-scope registration. No router wiring lands in this scope.
package graphapi
