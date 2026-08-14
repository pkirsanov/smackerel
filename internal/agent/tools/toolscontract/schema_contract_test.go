// Package toolscontract holds cross-tool contracts that no single tool package
// can assert about itself.
//
// BUG-061-012. Agent tool input schemas MUST NOT declare a caller identity.
// A tool that accepts `user_id` as an argument is asking the language model to
// name the principal, which is either decorative (retrieval, recipesearch —
// validated then discarded, so the schema misrepresents an access control that
// does not exist) or load-bearing (microtools/entity_resolve,
// notification/propose — the supplied value reaches a resolver or addresses a
// notification, so the model chooses whose data is touched).
//
// This test reads the tool sources as TEXT rather than reflecting over the
// registry, for the same reason internal/deploy/eval_lane_contract_test.go
// does: registration requires configured services, and a contract that only
// holds when the world is wired is not a contract. Reading source also means a
// tool added by a future author is covered the moment it lands, which an
// enumerated list of today's tools would not be.
package toolscontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// toolsRoot is the tree every agent tool lives under, relative to this package.
const toolsRoot = "../"

// callerIdentityProperty matches a JSON-schema property naming the caller.
// Spelling variants are included because the defect is the CONCEPT, not the
// literal `user_id`: renaming the field to `userId` would evade a literal check
// while leaving the model in charge of the identity.
var callerIdentityProperty = regexp.MustCompile(`"(user_id|userId|user|principal|actor|actor_user_id|on_behalf_of)"\s*:\s*\{`)

// schemaDeclaration matches the raw-JSON schema literals the tools declare.
// Used only by the stale-fixture guard below.
var schemaDeclaration = regexp.MustCompile(`(?s)json\.RawMessage\(` + "`" + `\{.*?"type"\s*:\s*"object"`)

// The ratchet that formerly carried microtools/entity_resolve and
// notification/propose is GONE. Both now resolve the principal from the request
// context (auth.SessionFromContext), so the check below stands bare: ANY agent
// tool schema naming the caller fails it, with no allowlist to add to. That is
// the terminal state BUG-061-012 was ratcheting toward — an allowlist that
// still exists is an allowlist someone can append to.

// TestToolSchemas_DeclareNoCallerIdentity is the contract. It fails when any
// agent tool schema names the caller.
func TestToolSchemas_DeclareNoCallerIdentity(t *testing.T) {
	var offenders []string
	var schemasSeen int

	err := filepath.Walk(toolsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path) //nolint:gosec // walking a fixed in-repo tree
		if rerr != nil {
			return rerr
		}
		src := string(b)

		schemasSeen += len(schemaDeclaration.FindAllString(src, -1))

		rel := filepath.ToSlash(path)
		for _, m := range callerIdentityProperty.FindAllStringSubmatch(src, -1) {
			offenders = append(offenders, rel+": "+m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", toolsRoot, err)
	}

	// Stale-fixture guard. If the walk stops finding schemas at all — a moved
	// tree, a renamed extension — the offender list goes empty and this test
	// would pass without asserting anything.
	if schemasSeen == 0 {
		t.Fatalf("found no tool input schemas under %s; the walk is broken, so a pass here would be vacuous", toolsRoot)
	}

	if len(offenders) > 0 {
		t.Errorf("agent tool input schemas declare a caller identity (%d):\n  %s\n"+
			"The model must not name the principal. Resolve it from the request context "+
			"(auth.SessionFromContext) instead, and delete the argument — including its "+
			"emptiness check, so the schema stops implying an access control it does not enforce.",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
