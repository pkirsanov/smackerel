// Package deploy — HL-RESCAN-012 / Gate G028 (NO-DEFAULTS / fail-loud SST).
//
// This file holds the static-file invariant tests for the dev-stack
// `docker-compose.yml` (NOT the deploy contract; that lives in
// compose_contract_test.go). The contract:
//
//	No `${VAR:-default}` substitution form may appear in active YAML lines
//	(non-comment) of `docker-compose.yml`. The allowlist is now EMPTY —
//	every substitution must use a fail-loud form.
//
// Forbidden form (Gate G028):
//
//	${VAR:-default}    # silently falls back when VAR is unset OR empty
//
// Required form (any of):
//
//	${VAR:?error msg}  # fails loud when VAR is unset OR empty
//	${VAR?error msg}   # fails loud when VAR is unset (empty allowed)
//	${VAR}             # substitutes VAR's value (empty if VAR is empty)
//
// Discovered: home-lab readiness re-scan finding HL-RESCAN-012 (P3),
// 2026-05-14. The pre-fix `docker-compose.yml` carried 14 occurrences of
// the forbidden `${VAR:-default}` form across 10 unique vars. Fix
// (BUG-029-003 + BUG-029-005):
//   - Build metadata (SMACKEREL_VERSION / SMACKEREL_COMMIT /
//     SMACKEREL_BUILD_TIME) is now SST-emitted with shell-env override
//     resolved at config-generate time; compose uses `${X:?...}`.
//   - Image refs (SMACKEREL_CORE_IMAGE / SMACKEREL_ML_IMAGE) use
//     `${X?...}` (empty allowed for build-from-source dev pattern).
//   - Env-file path (SMACKEREL_ENV_FILE) uses `${X:?...}`.
//   - Connector-fixture mount paths (BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR
//     / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR) are now SST-emitted
//     non-empty (repo-default fallback at config-generate time per
//     BUG-029-003 DD-2 precedent) and compose uses `${X:?...}`. The
//     container-internal mount paths are architectural constants written
//     as bare literals in the `environment:` block (matching the
//     AGENT_SCENARIO_DIR / PROMPT_CONTRACTS_DIR convention) — not subject
//     to Gate G028 because they carry no SST resolution. The connector
//     startup gate is now the boolean `<Connector>_ENABLED` only
//     (decoupled from path-emptiness; BUG-029-005).
//
// References:
//   - .github/instructions/smackerel-no-defaults.instructions.md (Gate G028)
//   - specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/
//   - specs/029-devops-pipeline/bugs/BUG-029-005-connector-volume-mount-fail-loud-sweep/
//   - specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-005-prometheus-default-fallback-bind-adversarial-coverage/spec.md
//     (sister contract on deploy/compose.deploy.yml — pattern this file mirrors)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// devComposeDefaultFallbackAllowlist is the exact set of var names that
// MAY appear in `${VAR:-default}` form in `docker-compose.yml`. Any var
// outside this list using the forbidden form fails the test.
//
// BUG-029-005: the allowlist is now EMPTY. The 4 connector-fixture
// mount paths (BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR / BROWSER_HISTORY_PATH
// / TWITTER_ARCHIVE_DIR) previously here were converted to fail-loud
// `${X:?...}` form once the SST started emitting them non-empty with a
// repo-default fallback (per BUG-029-003 DD-2 precedent). The
// per-var-gating semantic is still proven adversarially by
// `TestDevComposeContract_AdversarialAllowlistRespected` using a
// synthetic test-local allowlist.
//
// Adding to this list requires a matching inline comment in
// `docker-compose.yml` explaining the coupling that prevents conversion
// to fail-loud form, AND a follow-up bug-packet under
// specs/029-devops-pipeline/bugs/ tracking the eventual refactor.
var devComposeDefaultFallbackAllowlist = map[string]string{}

// devComposeDefaultFallbackRegex matches the forbidden `${VAR:-default}`
// substitution form. `\$` escapes the dollar sign in the regex; `[^}]*`
// is a non-greedy match for the default value.
var devComposeDefaultFallbackRegex = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*):-[^}]*\}`)

// findDevComposeUnauthorizedDefaultFallbacks scans yamlBytes line by
// line and returns a sorted, de-duplicated list of `${VAR:-...}`
// occurrences whose VAR name is NOT in allowlist. Each entry has the
// form "<lineNum>: <full match>". Comment-only lines (first non-space
// character is `#`) are skipped because in-comment documentation of the
// forbidden form is allowed (and in fact valuable for reviewers).
//
// The function is a pure helper so the adversarial sub-tests can feed
// it synthetic fixtures and prove RED→GREEN behavior.
func findDevComposeUnauthorizedDefaultFallbacks(yamlBytes []byte, allowlist map[string]string) []string {
	var unauthorized []string
	seen := make(map[string]struct{})
	for lineNum, rawLine := range strings.Split(string(yamlBytes), "\n") {
		trimmed := strings.TrimLeft(rawLine, " \t")
		if strings.HasPrefix(trimmed, "#") {
			continue // pure comment line; documentation of the forbidden form is allowed
		}
		matches := devComposeDefaultFallbackRegex.FindAllStringSubmatch(rawLine, -1)
		for _, m := range matches {
			fullMatch := m[0]
			varName := m[1]
			if _, ok := allowlist[varName]; ok {
				continue
			}
			key := fmt.Sprintf("%d:%s", lineNum+1, fullMatch)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			unauthorized = append(unauthorized, key)
		}
	}
	sort.Strings(unauthorized)
	return unauthorized
}

// TestDevComposeContract_NoUnauthorizedDefaultFallbacks reads the live
// `docker-compose.yml` from the repo root and asserts every
// `${VAR:-default}` occurrence in active (non-comment) YAML lines is
// either (a) on the explicit allowlist or (b) absent. This is the
// invariant that locks the dev-compose against accidental
// reintroduction of the forbidden Gate G028 form.
func TestDevComposeContract_NoUnauthorizedDefaultFallbacks(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "docker-compose.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live dev compose file %q: %v", composePath, err)
	}
	unauthorized := findDevComposeUnauthorizedDefaultFallbacks(yamlBytes, devComposeDefaultFallbackAllowlist)
	if len(unauthorized) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "docker-compose.yml violates Gate G028 (NO-DEFAULTS / fail-loud SST policy) — HL-RESCAN-012:\n")
		for _, u := range unauthorized {
			fmt.Fprintf(&b, "  - %s (use ${VAR:?error} fail-loud form, or add VAR to devComposeDefaultFallbackAllowlist with justification)\n", u)
		}
		t.Fatal(b.String())
	}
	t.Logf("contract OK: docker-compose.yml has zero unauthorized ${VAR:-default} forms (allowlist size = %d, all allowlist entries justified inline in the YAML and in the test source)", len(devComposeDefaultFallbackAllowlist))
}

// TestDevComposeContract_AdversarialUnauthorizedDefaultFallback proves
// the helper REJECTS a synthetic fixture that injects a forbidden
// `${SMACKEREL_VERSION:-dev}` form (one of the exact regression targets
// HL-RESCAN-012 surfaced and the BUG-029-003 fix removed). This is the
// adversarial guarantee that the contract is not tautological.
//
// The sub-cases enumerate (a) build-metadata default-fallback, (b)
// env-file default-fallback, (c) image-ref default-fallback, and (d)
// a previously-allowlisted var used in a NEW context where the
// allowlist would not protect it (proves the allowlist is keyed on var
// name, not line position).
func TestDevComposeContract_AdversarialUnauthorizedDefaultFallback(t *testing.T) {
	cases := []struct {
		name           string
		fixture        string
		expectVarToken string
	}{
		{
			name: "build-metadata default-fallback (SMACKEREL_VERSION:-dev)",
			fixture: `services:
  smackerel-core:
    build:
      args:
        VERSION: ${SMACKEREL_VERSION:-dev}
`,
			expectVarToken: "${SMACKEREL_VERSION:-dev}",
		},
		{
			name: "env-file default-fallback (SMACKEREL_ENV_FILE:-config/generated/dev.env)",
			fixture: `services:
  smackerel-core:
    env_file:
      - ${SMACKEREL_ENV_FILE:-config/generated/dev.env}
`,
			expectVarToken: "${SMACKEREL_ENV_FILE:-config/generated/dev.env}",
		},
		{
			name: "image-ref default-fallback (SMACKEREL_CORE_IMAGE:-)",
			fixture: `services:
  smackerel-core:
    image: ${SMACKEREL_CORE_IMAGE:-}
`,
			expectVarToken: "${SMACKEREL_CORE_IMAGE:-}",
		},
		{
			name: "novel default-fallback (SOME_NEW_VAR:-some-default) — proves allowlist failure-by-default",
			fixture: `services:
  smackerel-core:
    environment:
      SOME_NEW_VAR: ${SOME_NEW_VAR:-some-default}
`,
			expectVarToken: "${SOME_NEW_VAR:-some-default}",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			unauthorized := findDevComposeUnauthorizedDefaultFallbacks([]byte(tc.fixture), devComposeDefaultFallbackAllowlist)
			if len(unauthorized) == 0 {
				t.Fatalf("adversarial contract test failed: forbidden form %q was accepted (the contract is tautological — it would NOT catch a regression to Gate G028 / HL-RESCAN-012)", tc.expectVarToken)
			}
			joined := strings.Join(unauthorized, "\n")
			if !strings.Contains(joined, tc.expectVarToken) {
				t.Fatalf("adversarial contract test failed: helper rejected the fixture but the rejection list did NOT contain the expected token %q; got:\n%s", tc.expectVarToken, joined)
			}
			t.Logf("adversarial OK: forbidden form %q is rejected; full unauthorized list:\n%s", tc.expectVarToken, joined)
		})
	}
}

// TestDevComposeContract_AdversarialAllowlistRespected proves the
// helper ACCEPTS allowlisted var names even when they appear in
// novel contexts. The package-global allowlist is EMPTY (per
// BUG-029-005), so this test uses a SYNTHETIC test-local allowlist
// to prove the per-var gating semantic still works.
//
// Fixture injects two synthetic forbidden forms:
//   - `${ROGUE_VAR_A:-...}` — ALLOWLISTED in the test-local allowlist
//     (must NOT be flagged)
//   - `${ROGUE_VAR_B:-...}` — NOT allowlisted (must STILL be flagged)
//
// Combined: proves the allowlist gate is per-var, not per-line and not
// short-circuiting the whole scan.
func TestDevComposeContract_AdversarialAllowlistRespected(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    environment:
      # this allowlisted var (per test-local allowlist) must be ignored
      A_KEY: ${ROGUE_VAR_A:-allowlisted-default}
      # this non-allowlisted var must still be flagged
      B_KEY: ${ROGUE_VAR_B:-rogue-default}
`
	syntheticAllowlist := map[string]string{
		"ROGUE_VAR_A": "synthetic test-local allowlist entry to prove per-var gating",
	}
	unauthorized := findDevComposeUnauthorizedDefaultFallbacks([]byte(fixture), syntheticAllowlist)
	if len(unauthorized) == 0 {
		t.Fatal("adversarial contract test failed: helper accepted the entire fixture; the rogue ${ROGUE_VAR_B:-rogue-default} form should have been rejected (the contract is tautological — the allowlist gate appears to short-circuit the entire scan instead of being per-var)")
	}
	joined := strings.Join(unauthorized, "\n")
	if !strings.Contains(joined, "${ROGUE_VAR_B:-rogue-default}") {
		t.Fatalf("adversarial contract test failed: rejection list did NOT contain the rogue ${ROGUE_VAR_B:-rogue-default} form; got:\n%s", joined)
	}
	if strings.Contains(joined, "ROGUE_VAR_A") {
		t.Fatalf("adversarial contract test failed: allowlisted ${ROGUE_VAR_A:-...} form appeared in rejection list; allowlist is not respected per-var; got:\n%s", joined)
	}
	t.Logf("adversarial OK: allowlisted ${ROGUE_VAR_A:-...} accepted, rogue ${ROGUE_VAR_B:-rogue-default} rejected; full unauthorized list:\n%s", joined)
}

// TestDevComposeContract_AdversarialCommentLinesIgnored proves the
// helper SKIPS comment-only lines (first non-space char is `#`). This
// matters because `docker-compose.yml` documents the forbidden form
// in inline comments (e.g. spec 042 / HL-RESCAN-012 commentary), and
// the lint MUST NOT flag those documentation references.
//
// Adversarial half: the same forbidden form in an active YAML line
// MUST still be flagged — proves the comment-skip is line-scope only,
// not file-scope.
func TestDevComposeContract_AdversarialCommentLinesIgnored(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    # documentation reference — forbidden form ${SOME_VAR:-some-default} described in a comment
    image: ${SMACKEREL_CORE_IMAGE?must be set in env file}
  smackerel-ml:
    image: ${ROGUE_VAR:-rogue-default}
`
	unauthorized := findDevComposeUnauthorizedDefaultFallbacks([]byte(fixture), devComposeDefaultFallbackAllowlist)
	if len(unauthorized) == 0 {
		t.Fatal("adversarial contract test failed: helper accepted the entire fixture; the active ${ROGUE_VAR:-rogue-default} form on smackerel-ml should have been rejected")
	}
	joined := strings.Join(unauthorized, "\n")
	if !strings.Contains(joined, "${ROGUE_VAR:-rogue-default}") {
		t.Fatalf("adversarial contract test failed: rejection list did NOT contain the active ${ROGUE_VAR:-rogue-default} form; got:\n%s", joined)
	}
	if strings.Contains(joined, "${SOME_VAR:-some-default}") {
		t.Fatalf("adversarial contract test failed: comment-line ${SOME_VAR:-some-default} reference appeared in rejection list; comment-line skip is broken; got:\n%s", joined)
	}
	t.Logf("adversarial OK: comment-line documentation reference ignored, active forbidden form rejected; full unauthorized list:\n%s", joined)
}

// --------------------------------------------------------------------
// BUG-029-005: positive-assertion contracts on the 4 connector mount
// paths and 4 container-internal env overrides.
//
// The default-fallback lint above is a NEGATIVE assertion (forbids the
// `${X:-default}` form). It does NOT catch a regression that replaces a
// fail-loud `${X:?...}` substitution with a bare `${X}` (which silently
// substitutes empty when X is unset, leading to a broken Compose
// document or worse, a quietly-empty mount source). The two tests below
// are POSITIVE assertions: they require the live file to contain the
// exact fail-loud forms with required attribution.
// --------------------------------------------------------------------

// failLoudVolumeMountSpec is the per-var contract for a connector
// volume-mount substitution. The live file must contain a line with:
//
//   - ${<EnvVar>:?<error message containing all attribution tokens>}:<containerPath>:ro
//
// The error message must contain all `attribution` tokens (e.g. "Gate
// G028", "HL-RESCAN-012", and at least one operator-fix-path hint).
type failLoudVolumeMountSpec struct {
	envVar        string
	containerPath string
	attribution   []string
}

var failLoudVolumeMountSpecs = []failLoudVolumeMountSpec{
	{envVar: "BOOKMARKS_IMPORT_DIR", containerPath: "/data/bookmarks-import", attribution: []string{"Gate G028", "HL-RESCAN-012", "./smackerel.sh"}},
	{envVar: "MAPS_IMPORT_DIR", containerPath: "/data/maps-import", attribution: []string{"Gate G028", "HL-RESCAN-012", "./smackerel.sh"}},
	{envVar: "BROWSER_HISTORY_PATH", containerPath: "/data/browser-history/History", attribution: []string{"Gate G028", "HL-RESCAN-012", "./smackerel.sh"}},
	{envVar: "TWITTER_ARCHIVE_DIR", containerPath: "/data/twitter-archive", attribution: []string{"Gate G028", "HL-RESCAN-012", "./smackerel.sh"}},
}

// findFailLoudVolumeMountViolations scans yamlBytes for the 4
// connector-volume-mount substitutions and returns a list of contract
// violations. A violation is any of:
//   - the spec.envVar is not present in fail-loud `${X:?...}` form
//   - the fail-loud error message is missing any of the required
//     attribution tokens
//   - the container path does not match the spec
//
// Comment-only lines are skipped (forbidden-form documentation in
// inline comments must not satisfy the contract either).
func findFailLoudVolumeMountViolations(yamlBytes []byte, specs []failLoudVolumeMountSpec) []string {
	var violations []string
	text := string(yamlBytes)
	for _, spec := range specs {
		// Per-var fail-loud form: `${ENVVAR:?<msg>}:<containerPath>:ro`
		// (the `:ro` suffix is part of the live-file contract).
		// We build the regex dynamically per spec so each var's container path
		// is enforced positionally (no cross-line/cross-var smuggling possible).
		envRe := regexp.MustCompile(`\$\{` + regexp.QuoteMeta(spec.envVar) + `:\?([^}]*)\}:` + regexp.QuoteMeta(spec.containerPath) + `:ro`)
		matched := false
		for lineNum, rawLine := range strings.Split(text, "\n") {
			trimmed := strings.TrimLeft(rawLine, " \t")
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			m := envRe.FindStringSubmatch(rawLine)
			if m == nil {
				continue
			}
			matched = true
			errMsg := m[1]
			for _, token := range spec.attribution {
				if !strings.Contains(errMsg, token) {
					violations = append(violations, fmt.Sprintf("line %d: var %s fail-loud error message missing required attribution token %q (got %q)", lineNum+1, spec.envVar, token, errMsg))
				}
			}
		}
		if !matched {
			violations = append(violations, fmt.Sprintf("var %s: no fail-loud volume-mount line matched the contract — expected exactly `- ${%s:?...}:%s:ro` (BUG-029-005)", spec.envVar, spec.envVar, spec.containerPath))
		}
	}
	return violations
}

// TestDevComposeContract_FailLoudVolumeMounts is the LIVE-FILE positive
// assertion that the 4 connector-volume-mount lines in
// `docker-compose.yml` use the fail-loud `${X:?...}` form with the
// required Gate G028 / HL-RESCAN-012 / operator-fix-path attribution.
//
// This complements the NEGATIVE forbidden-form lint above (which would
// NOT catch a regression that replaces `${X:?...}` with bare `${X}`).
func TestDevComposeContract_FailLoudVolumeMounts(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "docker-compose.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live dev compose file %q: %v", composePath, err)
	}
	violations := findFailLoudVolumeMountViolations(yamlBytes, failLoudVolumeMountSpecs)
	if len(violations) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "docker-compose.yml violates BUG-029-005 fail-loud volume-mount contract (Gate G028 / HL-RESCAN-012):\n")
		for _, v := range violations {
			fmt.Fprintf(&b, "  - %s\n", v)
		}
		t.Fatal(b.String())
	}
	t.Logf("contract OK: docker-compose.yml carries 4 fail-loud volume-mount substitutions with full Gate G028 / HL-RESCAN-012 / operator-fix-path attribution")
}

// TestDevComposeContract_FailLoudVolumeMounts_Adversarial proves the
// helper REJECTS each of the 3 known regression modes for the
// fail-loud volume-mount contract:
//   - `${X:-default}` — silent default fallback (Gate G028 forbidden form, BUG-029-003 / 005 fix target)
//   - `${X?msg}` — fail when unset but accepts empty value (weaker than `:?`)
//   - `${X}` — bare substitution, silently empty when X is unset/empty
//
// Each sub-case feeds a synthetic fixture containing one of the
// regression forms and asserts the helper produces a violation naming
// the regression.
func TestDevComposeContract_FailLoudVolumeMounts_Adversarial(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
	}{
		{
			name: "regression to ${X:-default} silent default fallback",
			fixture: `services:
  smackerel-core:
    volumes:
      - ${BOOKMARKS_IMPORT_DIR:-./data/bookmarks-import}:/data/bookmarks-import:ro
      - ${MAPS_IMPORT_DIR:?MAPS_IMPORT_DIR must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/maps-import:ro
      - ${BROWSER_HISTORY_PATH:?BROWSER_HISTORY_PATH must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/browser-history/History:ro
      - ${TWITTER_ARCHIVE_DIR:?TWITTER_ARCHIVE_DIR must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/twitter-archive:ro
`,
		},
		{
			name: "regression to ${X?msg} fail-on-unset (accepts empty)",
			fixture: `services:
  smackerel-core:
    volumes:
      - ${BOOKMARKS_IMPORT_DIR:?BOOKMARKS_IMPORT_DIR must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/bookmarks-import:ro
      - ${MAPS_IMPORT_DIR?weaker form — accepts empty}:/data/maps-import:ro
      - ${BROWSER_HISTORY_PATH:?BROWSER_HISTORY_PATH must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/browser-history/History:ro
      - ${TWITTER_ARCHIVE_DIR:?TWITTER_ARCHIVE_DIR must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/twitter-archive:ro
`,
		},
		{
			name: "regression to bare ${X} substitution",
			fixture: `services:
  smackerel-core:
    volumes:
      - ${BOOKMARKS_IMPORT_DIR:?BOOKMARKS_IMPORT_DIR must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/bookmarks-import:ro
      - ${MAPS_IMPORT_DIR:?MAPS_IMPORT_DIR must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/maps-import:ro
      - ${BROWSER_HISTORY_PATH}:/data/browser-history/History:ro
      - ${TWITTER_ARCHIVE_DIR:?TWITTER_ARCHIVE_DIR must be set in env file (run ./smackerel.sh config generate or ./smackerel.sh up) — Gate G028 / HL-RESCAN-012}:/data/twitter-archive:ro
`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			violations := findFailLoudVolumeMountViolations([]byte(tc.fixture), failLoudVolumeMountSpecs)
			if len(violations) == 0 {
				t.Fatalf("adversarial contract test failed: helper accepted the regression fixture %q (the contract is tautological — it would NOT catch a regression to weaker substitution forms)", tc.name)
			}
			t.Logf("adversarial OK: regression form %q rejected; violations:\n  %s", tc.name, strings.Join(violations, "\n  "))
		})
	}
}

// envOverrideConstantSpec is the per-var contract for a container-internal
// environment override. The live file must contain a YAML mapping line
// with the var bound to a bare-literal container path (no `${...}`
// substitution), matching the AGENT_SCENARIO_DIR / PROMPT_CONTRACTS_DIR
// architectural-constant convention.
type envOverrideConstantSpec struct {
	envVar    string
	literalRH string
}

var envOverrideConstantSpecs = []envOverrideConstantSpec{
	{envVar: "BOOKMARKS_IMPORT_DIR", literalRH: "/data/bookmarks-import"},
	{envVar: "MAPS_IMPORT_DIR", literalRH: "/data/maps-import"},
	{envVar: "BROWSER_HISTORY_PATH", literalRH: "/data/browser-history/History"},
	{envVar: "TWITTER_ARCHIVE_DIR", literalRH: "/data/twitter-archive"},
}

// findEnvOverrideConstantViolations scans yamlBytes for the 4
// container-internal env overrides and returns a list of contract
// violations. A violation is any of:
//   - the spec.envVar is not present as `<envVar>: <literalRH>` (no
//     `${...}` substitution, no quotes, exact bare-literal match)
//   - the var is present but bound to a `${...}` substitution (the
//     pre-fix regression form)
func findEnvOverrideConstantViolations(yamlBytes []byte, specs []envOverrideConstantSpec) []string {
	var violations []string
	text := string(yamlBytes)
	for _, spec := range specs {
		// Required: `<indent><envVar>: <literalRH>` (trailing whitespace
		// or end-of-line tolerated; no `${...}` substitution permitted).
		requiredRe := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(spec.envVar) + `:\s+` + regexp.QuoteMeta(spec.literalRH) + `\s*$`)
		// Forbidden regression form: `<indent><envVar>: ${...}` (any
		// substitution form whatsoever — the architectural-constant
		// pattern is bare-literal).
		forbiddenRe := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(spec.envVar) + `:\s+\$\{`)
		requiredMatched := false
		for lineNum, rawLine := range strings.Split(text, "\n") {
			trimmed := strings.TrimLeft(rawLine, " \t")
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			if requiredRe.MatchString(rawLine) {
				requiredMatched = true
			}
			if forbiddenRe.MatchString(rawLine) {
				violations = append(violations, fmt.Sprintf("line %d: var %s is bound to a ${...} substitution; container-internal mount paths are architectural constants and must be bare-literal `%s: %s` (BUG-029-005)", lineNum+1, spec.envVar, spec.envVar, spec.literalRH))
			}
		}
		if !requiredMatched {
			violations = append(violations, fmt.Sprintf("var %s: no env-override line matched the bare-literal contract `%s: %s` (BUG-029-005)", spec.envVar, spec.envVar, spec.literalRH))
		}
	}
	return violations
}

// TestComposeEnvOverrides_ContainerInternalConstants is the LIVE-FILE
// positive assertion that the 4 container-internal env overrides in
// `docker-compose.yml` are written as bare-literal container paths (no
// `${...}` substitution), matching the AGENT_SCENARIO_DIR /
// PROMPT_CONTRACTS_DIR architectural-constant convention.
//
// Rationale (BUG-029-005 DD-3): the container-internal mount path is a
// build-time constant tied to the `volumes:` target. It carries no
// SST resolution. Writing it as a `${X:+/data/...}` substitution
// dependent on the SST host path (a) creates a semantic dependency
// between two facts that are actually independent, and (b) is
// inconsistent with the AGENT_SCENARIO_DIR / PROMPT_CONTRACTS_DIR
// pattern that already exists for the prompt-contracts mount.
func TestComposeEnvOverrides_ContainerInternalConstants(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "docker-compose.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live dev compose file %q: %v", composePath, err)
	}
	violations := findEnvOverrideConstantViolations(yamlBytes, envOverrideConstantSpecs)
	if len(violations) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "docker-compose.yml violates BUG-029-005 container-internal-constant env-override contract:\n")
		for _, v := range violations {
			fmt.Fprintf(&b, "  - %s\n", v)
		}
		t.Fatal(b.String())
	}
	t.Logf("contract OK: docker-compose.yml carries 4 bare-literal container-internal env overrides (architectural-constant convention)")
}

// TestComposeEnvOverrides_ContainerInternalConstants_Adversarial proves
// the helper REJECTS a fixture that reintroduces the pre-fix
// `${X:+/data/...}` substitution form for one of the env overrides.
func TestComposeEnvOverrides_ContainerInternalConstants_Adversarial(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    environment:
      BOOKMARKS_IMPORT_DIR: ${BOOKMARKS_IMPORT_DIR:+/data/bookmarks-import}
      MAPS_IMPORT_DIR: /data/maps-import
      BROWSER_HISTORY_PATH: /data/browser-history/History
      TWITTER_ARCHIVE_DIR: /data/twitter-archive
`
	violations := findEnvOverrideConstantViolations([]byte(fixture), envOverrideConstantSpecs)
	if len(violations) == 0 {
		t.Fatal("adversarial contract test failed: helper accepted the regression fixture; the ${BOOKMARKS_IMPORT_DIR:+/data/bookmarks-import} substitution form should have been rejected (the contract is tautological — it would NOT catch a regression to the pre-fix coupled-substitution form)")
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "BOOKMARKS_IMPORT_DIR") {
		t.Fatalf("adversarial contract test failed: violation list did NOT mention BOOKMARKS_IMPORT_DIR; got:\n%s", joined)
	}
	t.Logf("adversarial OK: pre-fix ${BOOKMARKS_IMPORT_DIR:+/data/bookmarks-import} substitution form rejected; violations:\n%s", joined)
}
