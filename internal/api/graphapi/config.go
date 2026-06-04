// Package graphapi — SST config loader.
//
// Mirrors the internal/config/surfacing.go pattern: read every
// KNOWLEDGE_GRAPH_API_* env var via os.LookupEnv with fail-loud
// semantics; emit one consolidated error naming every missing or
// invalid key. NO in-source defaults (smackerel-no-defaults).
package graphapi

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the SST surface for spec 080 SCOPE-080-01. Sourced from
// config/smackerel.yaml → knowledge_graph_api.* via
// scripts/commands/config.sh; loaded into the central internal/config
// boot wiring and read directly here for the graphapi package's own
// runtime needs (cursor secret lookup, limit clamping).
type Config struct {
	// ListDefaultLimit is the default page size for list endpoints
	// when the caller omits ?limit.
	ListDefaultLimit int
	// ListMaxLimit is the hard ceiling for list-endpoint ?limit.
	ListMaxLimit int
	// TimeWindowMaxDays is the maximum span of /api/time queries.
	TimeWindowMaxDays int
	// EdgesDefaultLimit is the default page size for the edges
	// endpoint when the caller omits ?limit.
	EdgesDefaultLimit int
	// EdgesMaxLimit is the hard ceiling for edges-endpoint ?limit.
	EdgesMaxLimit int
	// CursorSecretEnv is the *name* of the env var that holds the
	// HMAC signing key. The secret value itself is operator-injected
	// (spec 052) and read at runtime by LoadCursorSecret.
	CursorSecretEnv string
}

// LoadConfig reads the KNOWLEDGE_GRAPH_API_* env vars and returns the
// populated Config plus the result of Validate(). Missing env vars
// (LookupEnv == false) are fail-loud; invalid values are joined into
// a single error naming every offender.
func LoadConfig() (Config, error) {
	var cfg Config
	var errs []string

	cfg.ListDefaultLimit, errs = lookupPositiveInt("KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT", errs)
	cfg.ListMaxLimit, errs = lookupPositiveInt("KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT", errs)
	cfg.TimeWindowMaxDays, errs = lookupPositiveInt("KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS", errs)
	cfg.EdgesDefaultLimit, errs = lookupPositiveInt("KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT", errs)
	cfg.EdgesMaxLimit, errs = lookupPositiveInt("KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT", errs)
	cfg.CursorSecretEnv, errs = lookupNonEmptyString("KNOWLEDGE_GRAPH_API_CURSOR_SECRET_ENV", errs)

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("[F080-SST-MISSING] missing or invalid required knowledge_graph_api configuration: %s", strings.Join(errs, ", "))
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate enforces the cross-field invariants documented on Config.
func (c Config) Validate() error {
	var errs []string
	if c.ListDefaultLimit > c.ListMaxLimit {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_LIST_DEFAULT_LIMIT (%d) must be <= KNOWLEDGE_GRAPH_API_LIST_MAX_LIMIT (%d)", c.ListDefaultLimit, c.ListMaxLimit))
	}
	if c.EdgesDefaultLimit > c.EdgesMaxLimit {
		errs = append(errs, fmt.Sprintf("KNOWLEDGE_GRAPH_API_EDGES_DEFAULT_LIMIT (%d) must be <= KNOWLEDGE_GRAPH_API_EDGES_MAX_LIMIT (%d)", c.EdgesDefaultLimit, c.EdgesMaxLimit))
	}
	if len(errs) > 0 {
		return fmt.Errorf("[F080-SST-INVALID] invalid knowledge_graph_api configuration: %s", strings.Join(errs, ", "))
	}
	return nil
}

// Limits projects the Config onto the runtime Limits envelope used
// by ClampLimit / ClampEdgesLimit.
func (c Config) Limits() Limits {
	return Limits{
		ListDefault:       c.ListDefaultLimit,
		ListMax:           c.ListMaxLimit,
		EdgesDefault:      c.EdgesDefaultLimit,
		EdgesMax:          c.EdgesMaxLimit,
		TimeWindowMaxDays: c.TimeWindowMaxDays,
	}
}

// LoadCursorSecret reads the cursor HMAC secret from the env var
// named by CursorSecretEnv. Missing or empty → fail-loud error. The
// caller wraps the bytes via NewCursorCodec.
func (c Config) LoadCursorSecret() ([]byte, error) {
	if c.CursorSecretEnv == "" {
		return nil, fmt.Errorf("[F080-SST-MISSING] knowledge_graph_api.cursor_secret_env is empty; cannot resolve cursor HMAC secret")
	}
	v, ok := os.LookupEnv(c.CursorSecretEnv)
	if !ok {
		return nil, fmt.Errorf("[F080-SST-MISSING] env var %q (named by knowledge_graph_api.cursor_secret_env) is not set", c.CursorSecretEnv)
	}
	if v == "" {
		return nil, fmt.Errorf("[F080-SST-INVALID] env var %q (named by knowledge_graph_api.cursor_secret_env) is empty", c.CursorSecretEnv)
	}
	return []byte(v), nil
}

func lookupPositiveInt(key string, errs []string) (int, []string) {
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

func lookupNonEmptyString(key string, errs []string) (string, []string) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", append(errs, key+" (env var not set)")
	}
	if v == "" {
		return "", append(errs, key+" (empty)")
	}
	return v, errs
}
