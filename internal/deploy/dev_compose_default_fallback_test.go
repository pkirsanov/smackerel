// Package deploy — HL-RESCAN-012 / Gate G028 (NO-DEFAULTS / fail-loud SST).
//
// This file holds the static-file invariant test for the dev-stack
// `docker-compose.yml` (NOT the deploy contract; that lives in
// compose_contract_test.go). The contract:
//
//	No `${VAR:-default}` substitution form may appear in active YAML lines
//	(non-comment) of `docker-compose.yml`, except for an explicit
//	allowlist of intentional dev-fixture volume-mount paths whose
//	"empty value = no import" coupling with the connector code makes the
//	conversion to `${VAR:?...}` fail-loud form a separate refactor.
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
// the forbidden `${VAR:-default}` form across 10 unique vars. Fix:
//   - Build metadata (SMACKEREL_VERSION / SMACKEREL_COMMIT /
//     SMACKEREL_BUILD_TIME) is now SST-emitted with shell-env override
//     resolved at config-generate time; compose uses `${X:?...}`.
//   - Image refs (SMACKEREL_CORE_IMAGE / SMACKEREL_ML_IMAGE) use
//     `${X?...}` (empty allowed for build-from-source dev pattern).
//   - Env-file path (SMACKEREL_ENV_FILE) uses `${X:?...}`.
//   - Connector-fixture mount paths (BOOKMARKS_IMPORT_DIR / MAPS_IMPORT_DIR
//     / BROWSER_HISTORY_PATH / TWITTER_ARCHIVE_DIR) remain on `${X:-...}`
//     pending a follow-up refactor that decouples the mount path from the
//     "empty = no import" connector signal.
//
// References:
//   - .github/instructions/smackerel-no-defaults.instructions.md (Gate G028)
//   - specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/
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
// Each entry is justified inline. Adding to this list requires a
// matching inline comment in `docker-compose.yml` explaining the
// coupling that prevents conversion to fail-loud form, AND a follow-up
// bug-packet under specs/029-devops-pipeline/bugs/ tracking the
// eventual refactor.
var devComposeDefaultFallbackAllowlist = map[string]string{
	"BOOKMARKS_IMPORT_DIR": "connector-fixture mount; empty value = no import (consumed via ${X:+/data/...} env override)",
	"MAPS_IMPORT_DIR":      "connector-fixture mount; empty value = no import (consumed via ${X:+/data/...} env override)",
	"BROWSER_HISTORY_PATH": "connector-fixture mount; empty value = no import (consumed via ${X:+/data/...} env override)",
	"TWITTER_ARCHIVE_DIR":  "connector-fixture mount; empty value = no import (consumed via ${X:+/data/...} env override)",
}

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
// novel contexts. Specifically: a fixture using
// `${BOOKMARKS_IMPORT_DIR:-./elsewhere}` (allowlist member, but with a
// different default value than the live file) MUST pass the lint. This
// proves the allowlist is keyed on var name only — adding a new
// allowlisted occurrence does NOT silently smuggle the broader
// forbidden form back in for OTHER vars.
//
// Negative half: simultaneously injecting a non-allowlisted forbidden
// form proves the helper still rejects that one. Combined, the two
// halves prove the allowlist gate is per-var, not per-line.
func TestDevComposeContract_AdversarialAllowlistRespected(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    volumes:
      # this allowlisted var is allowed even with a different default
      - ${BOOKMARKS_IMPORT_DIR:-./somewhere/else}:/data/bookmarks-import:ro
    environment:
      # this non-allowlisted var must still be flagged
      ROGUE: ${ROGUE_VAR:-rogue-default}
`
	unauthorized := findDevComposeUnauthorizedDefaultFallbacks([]byte(fixture), devComposeDefaultFallbackAllowlist)
	if len(unauthorized) == 0 {
		t.Fatal("adversarial contract test failed: helper accepted the entire fixture; the rogue ${ROGUE_VAR:-rogue-default} form should have been rejected (the contract is tautological — the allowlist gate appears to short-circuit the entire scan instead of being per-var)")
	}
	joined := strings.Join(unauthorized, "\n")
	if !strings.Contains(joined, "${ROGUE_VAR:-rogue-default}") {
		t.Fatalf("adversarial contract test failed: rejection list did NOT contain the rogue ${ROGUE_VAR:-rogue-default} form; got:\n%s", joined)
	}
	if strings.Contains(joined, "BOOKMARKS_IMPORT_DIR") {
		t.Fatalf("adversarial contract test failed: allowlisted ${BOOKMARKS_IMPORT_DIR:-...} form appeared in rejection list; allowlist is not respected per-var; got:\n%s", joined)
	}
	t.Logf("adversarial OK: allowlisted ${BOOKMARKS_IMPORT_DIR:-...} accepted, rogue ${ROGUE_VAR:-rogue-default} rejected; full unauthorized list:\n%s", joined)
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
