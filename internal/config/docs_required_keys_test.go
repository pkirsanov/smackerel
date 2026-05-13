package config

import (
	"os"
	"strings"
	"testing"
)

// Spec 051 SCN-051-S03 / FR-051-006 — docs-static lint. Operator-facing
// docs MUST name every canonical AUTH_* env var produced by the spec
// 044 contract and MUST NOT mention any retired alias from the spec
// 051 forbidden-aliases list. This test runs on every
// `./smackerel.sh test unit --go` invocation and is the regression gate
// for any future contract additions or deletions.
//
// New canonical key: add it to canonicalAuthKeys below AND add a row
// to docs/Deployment.md and docs/Operations.md before this test will
// pass.
//
// Removed alias: add it to forbiddenAuthAliases below AND grep
// docs/Deployment.md and docs/Operations.md to scrub any remaining
// references.

// canonicalAuthKeys are the env-var names the spec 044 PASETO v4 / Ed25519
// contract requires every operator-facing doc to name explicitly. Spec 051
// adds AUTH_BOOTSTRAP_TOKEN as a hard requirement at config-load time.
var canonicalAuthKeys = []string{
	"AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
	"AUTH_SIGNING_ACTIVE_KEY_ID",
	"AUTH_AT_REST_HASHING_KEY",
	"AUTH_BOOTSTRAP_TOKEN",
}

// forbiddenAuthAliases are env-var names and config paths that referred
// to the pre-spec-044 HMAC-based auth contract (or to never-shipped
// proposals during spec 051 R11 reconciliation). Their presence in
// operator-facing docs is a contract violation: it tells the operator
// to set values the runtime never reads and risks them shipping a
// broken deployment.
var forbiddenAuthAliases = []string{
	"auth.signing.hmac_key",
	"auth.signing.issuer",
	"AUTH_SIGNING_HMAC_KEY",
	"AUTH_SIGNING_ISSUER",
	"signing_secret",
	"at_rest_hmac_key",
	"bootstrap_secret",
	"enrollment_token",
}

// docsToScan are the operator-facing docs the lint pins. Adding a new
// entry here means the doc becomes contractually bound to name every
// canonical key.
var docsToScan = []string{
	"../../docs/Deployment.md",
	"../../docs/Operations.md",
}

// TestDocs_NameAllCanonicalAuthKeys asserts each canonical AUTH_* key
// appears at least once in each operator-facing doc. Failure means a
// new key was added to the runtime contract without updating the docs
// (or an existing key was accidentally deleted from a doc table).
func TestDocs_NameAllCanonicalAuthKeys(t *testing.T) {
	for _, docPath := range docsToScan {
		body, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("docs-static lint cannot read %s: %v", docPath, err)
		}
		for _, key := range canonicalAuthKeys {
			if !strings.Contains(string(body), key) {
				t.Errorf("docs-static lint: %s does NOT name canonical auth key %q (spec 051 FR-051-006)", docPath, key)
			}
		}
	}
}

// TestDocs_DoNotMentionForbiddenAliases asserts no operator-facing doc
// mentions any retired alias. The test is case-sensitive on purpose:
// the alias forms above are the literal strings R11 reconciliation
// retired; partial matches (e.g., generic English words containing
// "issuer") are not in scope.
func TestDocs_DoNotMentionForbiddenAliases(t *testing.T) {
	for _, docPath := range docsToScan {
		body, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("docs-static lint cannot read %s: %v", docPath, err)
		}
		for _, alias := range forbiddenAuthAliases {
			if strings.Contains(string(body), alias) {
				t.Errorf("docs-static lint: %s contains forbidden auth alias %q (spec 051 FR-051-006). Replace with the spec 044 canonical key.", docPath, alias)
			}
		}
	}
}

// TestDocs_CanaryReadsBaseline is the docs-static canary required by
// Scope 3 DoD: prove the lint reads a baseline that DOES contain every
// canonical key today. This is a sanity probe so a docs file rename or
// move doesn't silently turn the lint into a no-op (the file-not-found
// path is already covered above; this canary asserts every doc has
// non-trivial content).
func TestDocs_CanaryReadsBaseline(t *testing.T) {
	for _, docPath := range docsToScan {
		body, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("docs-static canary cannot read %s: %v", docPath, err)
		}
		if len(body) < 1024 {
			t.Errorf("docs-static canary: %s is suspiciously short (%d bytes); the lint may be a no-op against a stub file", docPath, len(body))
		}
	}
}
