// Package openknowledge implements the open-ended knowledge agent
// (spec 064). It provides a pluggable Tool registry, a bounded agent
// loop, and a mechanical cite-back verifier that enforce the spec 061
// provenance contract for open-domain queries.
//
// The package is intentionally split so that the tool-registry capability
// foundation can evolve independently of the agent loop and the
// transport-specific surfaces. Each concrete Tool lives under
// internal/assistant/openknowledge/tools/.
//
// All runtime values (allowlist, budgets, provider selection) MUST be
// supplied by the caller; the package enforces the smackerel
// NO-DEFAULTS / fail-loud SST policy and never reads its own
// configuration from the environment.
package openknowledge
