// Package config — Spec 080 SCOPE-080-01: knowledge_graph_api.* SST.
//
// Boot-time validator for the Knowledge Graph Public API config block.
// Mirrors internal/config/surfacing.go: every KNOWLEDGE_GRAPH_API_*
// env var is read via os.LookupEnv with fail-loud semantics, and the
// consolidated error names every offender so operators see one
// actionable boot failure.
//
// NO in-source defaults (Gate G028, smackerel-no-defaults).
//
// The graphapi package has its own runtime loader at
// internal/api/graphapi/config.go for handler use; this file validates
// the same keys at boot so missing config aborts the process before
// any handler runs.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// KnowledgeGraphAPIConfig is the SST surface for spec 080 SCOPE-080-01.
type KnowledgeGraphAPIConfig struct {
	// ListDefaultLimit is the default page size for list endpoints.
	ListDefaultLimit int
	// ListMaxLimit is the hard ceiling for list-endpoint ?limit.
	ListMaxLimit int
	// TimeWindowMaxDays is the maximum span of /api/time queries.
	TimeWindowMaxDays int
	// EdgesDefaultLimit is the default page size for /api/graph/edges.
	EdgesDefaultLimit int
	// EdgesMaxLimit is the hard ceiling for /api/graph/edges ?limit.
	EdgesMaxLimit int
	// CursorSecretEnv names the env var that holds the HMAC cursor
	// signing key. The secret value itself is operator-injected via
	// spec 052; the env var named here MUST be populated at runtime
	// or graphapi.Config.LoadCursorSecret fails loud.
	CursorSecretEnv string
}

// Validate enforces non-negative limits and the cross-field invariant
// that default <= max for both list and edges paginators.
func (c *KnowledgeGraphAPIConfig) Validate() error {
	var errs []string
	if c.ListDefaultLimit < 1 {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT (must be >= 1, got %d)", c.ListDefaultLimit))
	}
	if c.ListMaxLimit < 1 {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT (must be >= 1, got %d)", c.ListMaxLimit))
	}
	if c.TimeWindowMaxDays < 1 {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS (must be >= 1, got %d)", c.TimeWindowMaxDays))
	}
	if c.EdgesDefaultLimit < 1 {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT (must be >= 1, got %d)", c.EdgesDefaultLimit))
	}
	if c.EdgesMaxLimit < 1 {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT (must be >= 1, got %d)", c.EdgesMaxLimit))
	}
	if c.ListDefaultLimit > 0 && c.ListMaxLimit > 0 && c.ListDefaultLimit > c.ListMaxLimit {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT (%d) must be <= KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT (%d)", c.ListDefaultLimit, c.ListMaxLimit))
	}
	if c.EdgesDefaultLimit > 0 && c.EdgesMaxLimit > 0 && c.EdgesDefaultLimit > c.EdgesMaxLimit {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT (%d) must be <= KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT (%d)", c.EdgesDefaultLimit, c.EdgesMaxLimit))
	}
	if strings.TrimSpace(c.CursorSecretEnv) == "" {
		errs = append(errs, "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV (must name a non-empty env var)")
	}
	if len(errs) > 0 {
		return fmt.Errorf("[F080-SST-INVALID] invalid knowledge_graph_api configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// loadKnowledgeGraphAPIConfig reads every KNOWLEDGE_GRAPH_API_* env
// var, validates fail-loud, and returns the populated config.
func loadKnowledgeGraphAPIConfig() (KnowledgeGraphAPIConfig, error) {
	var cfg KnowledgeGraphAPIConfig
	var errs []string

	cfg.ListDefaultLimit, errs = parseRequiredPositiveInt("KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT", errs)
	cfg.ListMaxLimit, errs = parseRequiredPositiveInt("KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT", errs)
	cfg.TimeWindowMaxDays, errs = parseRequiredPositiveInt("KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS", errs)
	cfg.EdgesDefaultLimit, errs = parseRequiredPositiveInt("KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT", errs)
	cfg.EdgesMaxLimit, errs = parseRequiredPositiveInt("KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT", errs)

	if v, ok := os.LookupEnv("KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV"); !ok {
		errs = append(errs, "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV (env var not set)")
	} else if v == "" {
		errs = append(errs, "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV (empty)")
	} else {
		cfg.CursorSecretEnv = v
	}

	if len(errs) > 0 {
		return KnowledgeGraphAPIConfig{}, fmt.Errorf("[F080-SST-MISSING] missing or invalid required knowledge_graph_api configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return KnowledgeGraphAPIConfig{}, err
	}
	return cfg, nil
}

// parseRequiredPositiveInt is a local LookupEnv-based parser so the
// fail-loud message distinguishes "env var not set" from "empty"
// (parsePositiveInt in drive.go uses os.Getenv and collapses both).
func parseRequiredPositiveInt(key string, errs []string) (int, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return 0, append(errs, key+" (env var not set)")
	}
	if v == "" {
		return 0, append(errs, key+" (empty)")
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, append(errs, fmt.Sprintf("%s (must be a positive integer, got %q)", key, v))
	}
	if n < 1 {
		return 0, append(errs, fmt.Sprintf("%s (must be >= 1, got %d)", key, n))
	}
	return n, errs
}
