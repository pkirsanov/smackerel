//go:build integration || e2e

// Spec 067 Scope 1 — policy-exception baseline loader and ratchet.
//
// The committed policy-exception-baseline.json at the repo root
// records every accepted policy exception by ID. Guards consult this
// loader to:
//
//   1. ratchet exception growth: any current accepted exception whose
//      ID is not in the baseline is "unreviewed growth" and the
//      baseline guard MUST fail (G067-A07);
//   2. expire stale exceptions: any exception whose expires_on is in
//      the past, or beyond ExceptionMaxAgeDays from now, is a
//      violation regardless of baseline membership.
//
// The loader has no fallback path: a missing or malformed baseline
// file is a bootstrap error, NOT a silent pass.

package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Exception is one accepted policy-exception entry.
type Exception struct {
	ID        string `json:"id"`
	RuleID    string `json:"rule_id"`
	Path      string `json:"path,omitempty"`
	Owner     string `json:"owner"`
	Reason    string `json:"reason"`
	ExpiresOn string `json:"expires_on"` // YYYY-MM-DD
}

// Baseline is the committed accepted-exceptions list.
type Baseline struct {
	SchemaVersion string      `json:"schema_version"`
	Policy        string      `json:"policy"`
	Exceptions    []Exception `json:"exceptions"`
}

// LoadBaseline reads the committed baseline file. Missing/malformed is
// a bootstrap error (no silent pass).
func LoadBaseline(path string) (*Baseline, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("policy-exception-baseline: read %s: %w", path, err)
	}
	var b Baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("policy-exception-baseline: parse %s: %w", path, err)
	}
	if b.SchemaVersion == "" {
		return nil, fmt.Errorf("policy-exception-baseline: missing schema_version in %s", path)
	}
	return &b, nil
}

// ValidateException returns a Violation if e is missing required
// metadata or has expired (relative to now and cfg.ExceptionMaxAgeDays).
// Returns nil if e is valid.
func ValidateException(e Exception, now time.Time, cfg PolicyConfig) *Violation {
	const policySource = "specs/067-intent-driven-policy-enforcement/spec.md"
	if e.ID == "" || e.RuleID == "" || e.Owner == "" || e.Reason == "" || e.ExpiresOn == "" {
		return &Violation{
			RuleID:       "G067-A07",
			RuleName:     "policy-exception missing required metadata",
			Path:         e.Path,
			Detail:       fmt.Sprintf("exception %q missing one of id/rule_id/owner/reason/expires_on", e.ID),
			PolicySource: policySource,
			Owner:        e.Owner,
			Resolution:   "fill in the missing field(s) or remove the exception",
		}
	}
	expires, err := time.Parse("2006-01-02", e.ExpiresOn)
	if err != nil {
		return &Violation{
			RuleID:       "G067-A07",
			RuleName:     "policy-exception expires_on malformed",
			Path:         e.Path,
			Detail:       fmt.Sprintf("exception %q expires_on=%q is not YYYY-MM-DD", e.ID, e.ExpiresOn),
			PolicySource: policySource,
			Owner:        e.Owner,
			Resolution:   "set expires_on to a YYYY-MM-DD date within policy.policy_exception_max_age_days from issue date",
		}
	}
	if !expires.After(now) {
		return &Violation{
			RuleID:       "G067-A07",
			RuleName:     "policy-exception expired",
			Path:         e.Path,
			Detail:       fmt.Sprintf("exception %q expired on %s", e.ID, e.ExpiresOn),
			PolicySource: policySource,
			Owner:        e.Owner,
			Resolution:   "remove the exception or replace it with a fresh reviewed entry",
		}
	}
	maxDelta := time.Duration(cfg.ExceptionMaxAgeDays) * 24 * time.Hour
	if expires.Sub(now) > maxDelta {
		return &Violation{
			RuleID:       "G067-A07",
			RuleName:     "policy-exception exceeds policy.policy_exception_max_age_days",
			Path:         e.Path,
			Detail:       fmt.Sprintf("exception %q expires_on=%s is more than %d days from now", e.ID, e.ExpiresOn, cfg.ExceptionMaxAgeDays),
			PolicySource: policySource,
			Owner:        e.Owner,
			Resolution:   fmt.Sprintf("set expires_on to within %d days from now", cfg.ExceptionMaxAgeDays),
		}
	}
	return nil
}

// RatchetExceptions compares the current accepted exception set
// (typically discovered by scanning scenario YAMLs + source
// annotations) against the committed baseline. Any current exception
// whose ID is NOT in the baseline is "unreviewed growth" and produces
// a G067-A07 violation. Expired or malformed exceptions also produce
// violations. Returns (violations, delta).
func RatchetExceptions(baseline *Baseline, current []Exception, now time.Time, cfg PolicyConfig) ([]Violation, ExceptionDelta) {
	const policySource = "specs/067-intent-driven-policy-enforcement/spec.md"
	accepted := map[string]struct{}{}
	for _, e := range baseline.Exceptions {
		accepted[e.ID] = struct{}{}
	}
	var violations []Violation
	for _, e := range current {
		if v := ValidateException(e, now, cfg); v != nil {
			violations = append(violations, *v)
			continue
		}
		if _, ok := accepted[e.ID]; !ok {
			violations = append(violations, Violation{
				RuleID:       "G067-A07",
				RuleName:     "policy-exception not in baseline",
				Path:         e.Path,
				Detail:       fmt.Sprintf("accepted exception %q (rule %s) is not present in the committed baseline", e.ID, e.RuleID),
				PolicySource: policySource,
				Owner:        e.Owner,
				Resolution:   "bump policy-exception-baseline.json in the same commit, with reviewer approval",
			})
		}
	}
	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].RuleID != violations[j].RuleID {
			return violations[i].RuleID < violations[j].RuleID
		}
		return violations[i].Detail < violations[j].Detail
	})
	delta := ExceptionDelta{
		BaselineCount: len(baseline.Exceptions),
		CurrentCount:  len(current),
	}
	switch {
	case delta.CurrentCount > delta.BaselineCount:
		delta.DeltaStatus = "grew"
	case delta.CurrentCount < delta.BaselineCount:
		delta.DeltaStatus = "shrunk"
	default:
		delta.DeltaStatus = "unchanged"
	}
	return violations, delta
}
