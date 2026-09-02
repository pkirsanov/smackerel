package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	canonicalSynthesisSourceClass = "canonical-graph"
	maxSynthesisRetryBudget       = 10
	maxSynthesisDurationSeconds   = int64(^uint64(0)>>1) / int64(time.Second)
)

// SynthesisConfig is the fail-loud run policy shared by every synthesis
// trigger. All fields are resolved from the generated runtime environment.
type SynthesisConfig struct {
	ActorUserID           string
	DailyCron             string
	WeeklyCron            string
	RetryBudget           int
	RetryBackoff          time.Duration
	RetryMaxBackoff       time.Duration
	LeaseTTL              time.Duration
	PolicyVersion         string
	RequiredSourceClasses []string
	OptionalSourceClasses []string
	Retention             time.Duration
}

// MaxAttempts converts the configured retry budget into the total number of
// attempts while rejecting values outside the loader's bounded policy.
func (c SynthesisConfig) MaxAttempts() (int, error) {
	if c.RetryBudget < 0 || c.RetryBudget > maxSynthesisRetryBudget {
		return 0, fmt.Errorf("SYNTHESIS_RETRY_BUDGET must be between 0 and %d", maxSynthesisRetryBudget)
	}
	return c.RetryBudget + 1, nil
}

func loadSynthesisConfig() (SynthesisConfig, error) {
	var cfg SynthesisConfig
	var errs []string

	cfg.ActorUserID, errs = requiredTrimmedSynthesisString("SYNTHESIS_ACTOR_USER_ID", errs)
	cfg.DailyCron, errs = parseSynthesisCron("SYNTHESIS_DAILY_CRON", errs)
	cfg.WeeklyCron, errs = parseSynthesisCron("SYNTHESIS_WEEKLY_CRON", errs)
	cfg.RetryBudget, errs = parseSynthesisRetryBudget("SYNTHESIS_RETRY_BUDGET", errs)
	cfg.RetryBackoff, errs = parseSynthesisDuration("SYNTHESIS_RETRY_BACKOFF_SECONDS", errs)
	cfg.RetryMaxBackoff, errs = parseSynthesisDuration("SYNTHESIS_RETRY_MAX_BACKOFF_SECONDS", errs)
	cfg.LeaseTTL, errs = parseSynthesisDuration("SYNTHESIS_LEASE_SECONDS", errs)
	cfg.PolicyVersion, errs = requiredTrimmedSynthesisString("SYNTHESIS_POLICY_VERSION", errs)
	cfg.RequiredSourceClasses, errs = parseSynthesisSourceClasses("SYNTHESIS_REQUIRED_SOURCE_CLASSES", errs)
	cfg.OptionalSourceClasses, errs = parseSynthesisSourceClasses("SYNTHESIS_OPTIONAL_SOURCE_CLASSES", errs)
	cfg.Retention, errs = parseSynthesisDuration("SYNTHESIS_RETENTION_SECONDS", errs)

	if cfg.RetryBackoff > 0 && cfg.RetryMaxBackoff > 0 && cfg.RetryMaxBackoff < cfg.RetryBackoff {
		errs = append(errs, "SYNTHESIS_RETRY_MAX_BACKOFF_SECONDS (must be greater than or equal to SYNTHESIS_RETRY_BACKOFF_SECONDS)")
	}
	if len(cfg.RequiredSourceClasses) != 1 || cfg.RequiredSourceClasses[0] != canonicalSynthesisSourceClass {
		errs = append(errs, "SYNTHESIS_REQUIRED_SOURCE_CLASSES (must contain exactly canonical-graph)")
	}
	if len(cfg.OptionalSourceClasses) != 0 {
		errs = append(errs, "SYNTHESIS_OPTIONAL_SOURCE_CLASSES (must be an empty JSON list)")
	}

	if len(errs) > 0 {
		return SynthesisConfig{}, fmt.Errorf("missing or invalid required synthesis configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

func requiredTrimmedSynthesisString(key string, errs []string) (string, []string) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return "", append(errs, key+" (must be non-empty)")
	}
	return strings.TrimSpace(raw), errs
}

func parseSynthesisCron(key string, errs []string) (string, []string) {
	raw, ok := os.LookupEnv(key)
	expression := strings.TrimSpace(raw)
	if !ok || expression == "" {
		return "", append(errs, key+" (must be non-empty)")
	}
	if _, err := cron.ParseStandard(expression); err != nil {
		return "", append(errs, key+" (must be a valid cron expression)")
	}
	return expression, errs
}

func parseSynthesisRetryBudget(key string, errs []string) (int, []string) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return 0, append(errs, key)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > maxSynthesisRetryBudget {
		return 0, append(errs, fmt.Sprintf("%s (must be an integer between 0 and %d)", key, maxSynthesisRetryBudget))
	}
	return value, errs
}

func parseSynthesisDuration(key string, errs []string) (time.Duration, []string) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return 0, append(errs, key)
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return 0, append(errs, key+" (must be a positive integer number of seconds)")
	}
	if seconds > maxSynthesisDurationSeconds {
		return 0, append(errs, key+" (seconds overflow time.Duration)")
	}
	return time.Duration(seconds) * time.Second, errs
}

func parseSynthesisSourceClasses(key string, errs []string) ([]string, []string) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, append(errs, key)
	}

	var classes []string
	if err := json.Unmarshal([]byte(raw), &classes); err != nil || classes == nil {
		return nil, append(errs, key+" (must be a valid JSON list of strings)")
	}

	seen := make(map[string]struct{}, len(classes))
	for index, class := range classes {
		class = strings.TrimSpace(class)
		if class == "" {
			return nil, append(errs, key+" (must not contain empty values)")
		}
		if class != canonicalSynthesisSourceClass {
			return nil, append(errs, key+" (contains unknown source class "+class+")")
		}
		if _, duplicate := seen[class]; duplicate {
			return nil, append(errs, key+" (must not contain duplicate values)")
		}
		seen[class] = struct{}{}
		classes[index] = class
	}
	return classes, errs
}
