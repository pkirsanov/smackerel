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
//
// The file set is derived the same way: every non-test Go file in the module
// that CALLS a registration function is a candidate, wherever it lives. An
// earlier revision walked only internal/agent/tools/, which covered 8 of the
// 22 registration sites and let internal/assistant/openknowledge/agenttool
// declare a caller identity while this test stayed green. Deriving the set
// from the registration call removes the assumption that tools live in one
// directory.
package toolscontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// moduleRoot is the repository root relative to this package. The walk is
// restricted to .go files, so non-Go trees (specs/, docs/, config/) that quote
// schema fragments in prose cannot produce a finding.
const moduleRoot = "../../../../"

// skipDirs are trees that hold no first-party registered tool. Excluded to keep
// the walk fast and to avoid asserting a contract over vendored third-party
// source we do not own.
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true,
	"specs": true, "docs": true, "web": true, "ml": true,
}

// registersATool matches a call that puts a tool into an agent registry. A file
// that does not register anything cannot contribute a tool schema, so it is not
// examined — that is what keeps this a tool contract rather than a repo-wide
// grep for the string "user_id".
var registersATool = regexp.MustCompile(`agent\.RegisterTool\(|agent\.Register\(|RegisterTool\(agent\.Tool\{`)

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
	var registrarsSeen int

	err := filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path) //nolint:gosec // walking a fixed in-repo tree
		if rerr != nil {
			return rerr
		}
		src := string(b)
		if !registersATool.MatchString(src) {
			return nil
		}
		registrarsSeen++

		schemasSeen += len(schemaDeclaration.FindAllString(src, -1))

		rel := filepath.ToSlash(strings.TrimPrefix(path, moduleRoot))
		for _, m := range callerIdentityProperty.FindAllStringSubmatch(src, -1) {
			offenders = append(offenders, rel+": "+m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", moduleRoot, err)
	}

	// Stale-fixture guard. If the walk stops finding registrars or schemas at
	// all — a moved tree, a renamed extension, a renamed registration function
	// — the offender list goes empty and this test would pass without
	// asserting anything. The registrar floor is deliberately well below the 22
	// sites present when this was written, so the guard trips on a broken walk
	// rather than on ordinary tool churn.
	if registrarsSeen < 10 {
		t.Fatalf("found only %d files calling a tool registrar under %s; the walk is broken, so a pass here would be vacuous", registrarsSeen, moduleRoot)
	}
	if schemasSeen == 0 {
		t.Fatalf("found no tool input schemas under %s; the walk is broken, so a pass here would be vacuous", moduleRoot)
	}

	if len(offenders) > 0 {
		t.Errorf("agent tool input schemas declare a caller identity (%d):\n  %s\n"+
			"The model must not name the principal. Resolve it from the request context "+
			"(auth.SessionFromContext) instead, and delete the argument — including its "+
			"emptiness check, so the schema stops implying an access control it does not enforce.",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
