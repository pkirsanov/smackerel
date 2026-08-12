//go:build integration

// Call-site contract for the shared-namespace lock (BUG-104-001 findings F5, A2, A3, A4).
//
// The exclusion guards in nslock_test.go prove the MECHANISM works: a second
// session cannot take a held lock, and distinct namespaces do not contend.
// They do not prove the mechanism is USED. Deleting an
// `nslock.AcquireSelfKnowledge(t, pool)` call leaves those guards green while
// silently removing protection.
//
// This file closes that gap with TWO independent checks, because auditing the
// first draft showed a single discovery-based check was NOT sufficient:
//
//  1. A NAMED floor. The known contenders are asserted by path. A purely
//     discovery-based check missed two of the three originally-failing tests
//     (they INSERT via `insertEmbeddedArtifact`, whose SQL lives in a third
//     file) and held two others only through COMMENT text — rewording a log
//     message would have dropped them from the set. A cardinality-only floor
//     (`len >= 4`) could also pass while matching the wrong four files.
//
//  2. DISCOVERY, to catch contenders nobody thought to add to the named list.
//     This is the half that survives future authors.
//
// Both are required: (1) is exact but needs maintenance, (2) is automatic but
// evadable. Each alone has a demonstrated hole.

package nslock_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// knownContenders are files that write or wipe the shared `smackerel_self`
// namespace and MUST therefore acquire the lock. Asserted by name so removing
// a call is detectable even when the file would not be rediscovered — the
// evasions found in audit findings A2 and A3.
var knownContenders = []string{
	"tests/integration/selfknowledge/ingest_test.go",
	"tests/integration/openknowledge/semantic_searcher_test.go",
	"tests/integration/openknowledge/self_knowledge_tool_test.go",
	"tests/integration/openknowledge/self_knowledge_provenance_test.go",
	"tests/integration/knowledge_stats_test.go",
	"tests/e2e/openknowledge/self_knowledge_ask_e2e_test.go",
}

// mutatesArtifacts matches a write against the artifacts table. Reads are not
// contenders: a SELECT cannot destroy another writer's rows.
//
// UPDATE and CopyFrom are included per audit finding A3: an UPDATE rewriting
// `source_id` or `content_hash` corrupts another package's assertions just as
// effectively as a DELETE.
var mutatesArtifacts = regexp.MustCompile(`(?i)(INSERT\s+INTO\s+artifacts|DELETE\s+FROM\s+artifacts|UPDATE\s+artifacts|CopyFrom|TRUNCATE\s+TABLE[\s\S]{0,400}?\bartifacts\b)`)

// namesNamespace matches BOTH the string literal and the exported constant.
// The primary offender's DELETE binds `selfknowledge.SelfKnowledgeNamespace`,
// so a literal-only match found that file only through its comments (A3).
var namesNamespace = regexp.MustCompile(`smackerel_self|SelfKnowledgeNamespace`)

// TestNamespaceLock_KnownContendersAcquireTheLock asserts the named floor.
// This is the check that catches a deleted call in a file the discovery pass
// cannot see.
func TestNamespaceLock_KnownContendersAcquireTheLock(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	for _, rel := range knownContenders {
		path := filepath.Join(repoRoot, filepath.FromSlash(rel))
		b, err := os.ReadFile(path) //nolint:gosec // fixed in-repo test path
		if err != nil {
			t.Errorf("known contender %s is unreadable (%v). If it was renamed or deleted, update knownContenders deliberately — dropping it silently is how namespace protection rots", rel, err)
			continue
		}
		if !strings.Contains(string(b), "nslock.Acquire") {
			t.Errorf("%s writes to the shared `smackerel_self` namespace but does not call nslock.Acquire…; it can wipe or race another writer's rows", rel)
		}
	}
}

// TestNamespaceLock_DiscoveredContendersAcquireTheLock catches contenders that
// nobody added to knownContenders.
func TestNamespaceLock_DiscoveredContendersAcquireTheLock(t *testing.T) {
	root := filepath.Join("..", "..", "..", "tests")

	var missing []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// The nslock package itself names the namespace by definition and is
		// the provider, not a consumer.
		if strings.Contains(filepath.ToSlash(path), "/tests/integration/nslock/") {
			return nil
		}

		b, rerr := os.ReadFile(path) //nolint:gosec // walking a fixed in-repo test tree
		if rerr != nil {
			return rerr
		}
		src := string(b)

		if !namesNamespace.MatchString(src) || !mutatesArtifacts.MatchString(src) {
			return nil
		}
		if !strings.Contains(src, "nslock.Acquire") {
			missing = append(missing, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	if len(missing) > 0 {
		t.Errorf("these files mutate `artifacts` while naming the shared `smackerel_self` namespace but never call nslock.Acquire…: %v\n"+
			"Add nslock.AcquireSelfKnowledge(t, pool) to each, and add them to knownContenders so a future deletion is caught by name too.", missing)
	}
}
