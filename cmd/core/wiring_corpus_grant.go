package main

import (
	"fmt"
	"os"
)

// Spec 108 Scope 02 — fail-loud resolution of the corpus-grant enforcement
// stage. This is the ONLY place the stage is resolved; it runs exactly once
// in run(), before any listener binds, and there is no per-route override
// (R-108-FL6).

// corpusGrantEnforcementEnvVar is the generated-env key carrying the stage.
// Declared as auth.corpus_grant_enforcement in config/smackerel.yaml and
// emitted into config/generated/<env>.env by `./smackerel.sh config generate`.
const corpusGrantEnforcementEnvVar = "SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT"

// The two accepted booleans (spec.md §Register 4 closed vocabulary). Matching
// is EXACT: lenient parsers such as strconv.ParseBool also accept "1", "0",
// "t", "TRUE" and friends, which would smuggle extra acceptance paths past
// R-108-FL5's two-value rule.
const (
	corpusGrantObserveValue = "false" // OBSERVE — evaluate and count, never deny
	corpusGrantEnforceValue = "true"  // ENFORCE — deny ungranted corpus reads
)

// Stage names, uppercase per the closed vocabulary. Lowercase variants and
// synonyms (dry-run, monitor, audit, warn, permissive, shadow, log-only, …)
// are banned by spec.md §Register 4.
const (
	corpusGrantStageObserve = "OBSERVE"
	corpusGrantStageEnforce = "ENFORCE"
)

// corpusGrantEnforcementStage renders a resolved stage for logging.
func corpusGrantEnforcementStage(enforce bool) string {
	if enforce {
		return corpusGrantStageEnforce
	}
	return corpusGrantStageObserve
}

// corpusGrantEnforcementEnv snapshots the process environment for
// resolveCorpusGrantEnforcement. os.LookupEnv (not os.Getenv) preserves the
// absent-versus-empty distinction so the resolver operates on the real input
// rather than a collapsed one; both still refuse.
func corpusGrantEnforcementEnv() map[string]string {
	env := make(map[string]string, 1)
	if raw, present := os.LookupEnv(corpusGrantEnforcementEnvVar); present {
		env[corpusGrantEnforcementEnvVar] = raw
	}
	return env
}

// resolveCorpusGrantEnforcement returns true for ENFORCE and false for
// OBSERVE, or an error that aborts startup.
//
// It has NO default. An absent or empty value is REFUSED-BOOT (SCN-108-C03)
// and any other value is REFUSED-BOOT naming the offending value
// (SCN-108-C05). REFUSED-BOOT is a boot refusal, not a third mode — which is
// what preserves R-108-FL6's two-mode rule. The returned bool is meaningless
// when the error is non-nil; callers MUST abort rather than read it, because
// treating the zero value as OBSERVE is exactly the silent stage selection
// R-108-FL5 forbids.
func resolveCorpusGrantEnforcement(env map[string]string) (bool, error) {
	raw, present := env[corpusGrantEnforcementEnvVar]
	if !present || raw == "" {
		return false, fmt.Errorf(
			"%s is absent or empty — refusing to start (spec 108 R-108-FL5 / SCN-108-C03: REFUSED-BOOT is a boot refusal, not a third mode, and silently selecting %s or %s from a missing value is forbidden). Set auth.corpus_grant_enforcement in config/smackerel.yaml and run ./smackerel.sh config generate",
			corpusGrantEnforcementEnvVar, corpusGrantStageObserve, corpusGrantStageEnforce)
	}

	switch raw {
	case corpusGrantObserveValue:
		return false, nil
	case corpusGrantEnforceValue:
		return true, nil
	default:
		return false, fmt.Errorf(
			"%s has value %q, which is not one of the two accepted booleans %q (%s) or %q (%s) — refusing to start (spec 108 R-108-FL5 / SCN-108-C05: a malformed value selects neither stage)",
			corpusGrantEnforcementEnvVar, raw,
			corpusGrantObserveValue, corpusGrantStageObserve,
			corpusGrantEnforceValue, corpusGrantStageEnforce)
	}
}
