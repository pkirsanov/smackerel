//go:build integration

// Forbidden-pattern guard for spec 037 Scope 4 §4.3.
//
// Intent routing in Smackerel MUST be embedding-based. The design
// explicitly forbids regex matching of user input, switch/if-string
// chains, keyword maps, and hardcoded vendor/alias lists for the
// purpose of intent classification. This file is a CI-enforced static
// guard that scans the four code areas where such patterns would do
// the most damage:
//
//   - internal/agent/                  (the agent runtime itself)
//   - internal/telegram/dispatch*.go   (the Telegram intent surface)
//   - internal/api/intent*.go          (the REST agent invoke surface)
//   - internal/scheduler/              (the system-trigger surface)
//
// The guard rejects any file in those scopes containing:
//
//   - regexp.MustCompile (regex-based intent classification)
//   - switch <expr-on-input> / `if strings.Contains(input, "...")`
//     chains (keyword routing)
//   - map[string]ScenarioID (keyword → scenario lookup tables)
//
// Carve-out for `regexp.MustCompile`: fully-anchored patterns of shape
// `^...$` are SCHEMA validators (e.g., the scenario-id and version-slug
// validators in loader.go), not intent classifiers. Intent classifiers
// scan free-text input and never anchor end-to-end. The guard exempts
// the anchored shape so the loader's documented §2.2 syntactic checks
// pass while real intent regexes still trip the rule.
//
// Test files are exempt — adversarial regression fixtures and
// integration tests legitimately compile regexes for assertions about
// log output, etc. The guard targets production code paths only.
//
// If a future change reintroduces any pattern, this test fails with
// the offending file:line so the operator can see exactly what slipped
// past code review.
package agent_integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// forbiddenPatterns names each pattern and its compiled detector.
// Patterns are intentionally narrow: false positives would force
// reviewers to add `nolint`-style escape hatches and erode the guard's
// credibility. The "switch input" pattern matches `switch <ident>` only
// when the identifier name itself implies free-text input ("input",
// "raw", "rawInput", "text", "query", "msg", "message"); a future
// reviewer who deliberately wants to switch on a typed enum is not
// affected.
// anchoredRegexCompile detects `regexp.MustCompile(`^...$`)` — fully
// anchored patterns describe complete-string FORMAT validators (scenario
// id format, version slug format, etc.), which is the syntactic-input
// validation case explicitly carved out in design §2.2. Intent
// classifiers, by contrast, must scan free-text input for substrings
// or alternations and therefore never anchor end-to-end. The
// regex_intent_router rule below skips any line that matches this
// shape so the loader's scenario-id validator does not trip the guard
// while a real intent-routing regex (`(?i)(recipe|cooking|...)`,
// `\bbuy\b`, etc.) still does.
var anchoredRegexCompile = regexp.MustCompile("regexp\\.MustCompile\\(\\s*[`\"]\\^[^`\"]*\\$[`\"]\\s*\\)")

var forbiddenPatterns = []struct {
	name    string
	pattern *regexp.Regexp
	skip    func(line string) bool // optional per-rule exemption check
	rule    string
}{
	{
		name:    "regex_intent_router",
		pattern: regexp.MustCompile(`regexp\.MustCompile`),
		skip:    func(line string) bool { return anchoredRegexCompile.MatchString(line) },
		rule:    "regex matching for intent classification is forbidden in routing-scope packages (spec 037 §4.3)",
	},
	{
		name:    "switch_on_user_input",
		pattern: regexp.MustCompile(`(?m)^\s*switch\s+(input|raw|rawInput|text|query|msg|message)(\s|\.)`),
		rule:    "switch on free-text input for intent selection is forbidden (spec 037 §4.3)",
	},
	{
		name:    "strings_contains_intent_chain",
		pattern: regexp.MustCompile(`if\s+strings\.Contains\(\s*(input|raw|rawInput|text|query|msg|message)`),
		rule:    "if strings.Contains(input, ...) chains for intent selection are forbidden (spec 037 §4.3)",
	},
	{
		name:    "keyword_to_scenario_map",
		pattern: regexp.MustCompile(`map\[string\]ScenarioID`),
		rule:    "keyword maps for intent → scenario routing are forbidden (spec 037 §4.3)",
	},
}

// scopedPath describes one scoped area as a pair of (directory,
// optional file-name prefix). When prefix is non-empty, only files in
// that directory whose name starts with that prefix are scanned. This
// matches the design's narrow targeting: only `internal/telegram/dispatch*`
// (not the entire telegram package) and only `internal/api/intent*`.
type scopedPath struct {
	dir    string
	prefix string // empty = all .go files in dir
}

// scanLineForViolations returns the names of every forbidden pattern
// that fires on `line`. Returned slice is empty when the line is
// clean. Extracted from the directory scanner so the synthetic
// regression test (TestForbiddenRouterPatterns_DetectsSyntheticRouter)
// can exercise the rule set without writing to disk and so the guard's
// detection logic is provably non-tautological.
func scanLineForViolations(line string) []string {
	var hits []string
	for _, fp := range forbiddenPatterns {
		if fp.pattern.FindString(line) == "" {
			continue
		}
		if fp.skip != nil && fp.skip(line) {
			continue
		}
		hits = append(hits, fp.name)
	}
	return hits
}

func TestForbiddenRouterPatterns_ScopedDirectories(t *testing.T) {
	repoRoot := repoRootForTests(t)

	scopes := []scopedPath{
		{dir: filepath.Join(repoRoot, "internal", "agent"), prefix: ""},
		{dir: filepath.Join(repoRoot, "internal", "telegram"), prefix: "dispatch"},
		{dir: filepath.Join(repoRoot, "internal", "api"), prefix: "intent"},
		{dir: filepath.Join(repoRoot, "internal", "scheduler"), prefix: ""},
	}

	type violation struct {
		file    string
		line    int
		match   string
		ruleMsg string
	}
	var violations []violation
	scanned := 0

	for _, sc := range scopes {
		entries, err := os.ReadDir(sc.dir)
		if err != nil {
			if os.IsNotExist(err) {
				// The scheduler / api packages may not exist yet at
				// this scope's introduction; missing dir is an empty
				// scan, not a hard failure.
				continue
			}
			t.Fatalf("read scoped dir %s: %v", sc.dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			if strings.HasSuffix(e.Name(), "_test.go") {
				continue // test files are exempt
			}
			if sc.prefix != "" && !strings.HasPrefix(e.Name(), sc.prefix) {
				continue
			}
			full := filepath.Join(sc.dir, e.Name())
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("read %s: %v", full, err)
			}
			scanned++
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				for _, fp := range forbiddenPatterns {
					if fp.pattern.FindString(line) == "" {
						continue
					}
					if fp.skip != nil && fp.skip(line) {
						continue
					}
					relFile, _ := filepath.Rel(repoRoot, full)
					violations = append(violations, violation{
						file:    relFile,
						line:    i + 1,
						match:   strings.TrimSpace(fp.pattern.FindString(line)),
						ruleMsg: fp.rule,
					})
				}
			}
		}
	}

	if scanned == 0 {
		t.Fatal("forbidden-pattern guard scanned zero files; scope paths are misconfigured (the guard would silently always pass)")
	}
	if len(violations) > 0 {
		var b strings.Builder
		b.WriteString("forbidden routing patterns detected in scoped paths:\n")
		for _, v := range violations {
			b.WriteString("  ")
			b.WriteString(v.file)
			b.WriteString(":")
			b.WriteString(itoa(v.line))
			b.WriteString(": ")
			b.WriteString(v.match)
			b.WriteString(" — ")
			b.WriteString(v.ruleMsg)
			b.WriteString("\n")
		}
		t.Fatal(b.String())
	}
}

// TestForbiddenRouterPatterns_DetectsSyntheticRouter is the
// non-tautological proof that the guard actually bites. Each
// fixture is a representative regression that the design's §4.3
// rules are intended to prevent. If any fixture stops triggering
// the rule it claims to trip, this test fails — the guard cannot
// silently degrade to "always pass" because each rule has at least
// one positive sample here.
func TestForbiddenRouterPatterns_DetectsSyntheticRouter(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantRules []string
	}{
		{
			name:      "non_anchored_intent_regex",
			line:      `var recipeIntent = regexp.MustCompile(` + "`" + `(?i)(recipe|cooking|allrecipes)` + "`" + `)`,
			wantRules: []string{"regex_intent_router"},
		},
		{
			name:      "switch_on_input_variable",
			line:      `	switch input {`,
			wantRules: []string{"switch_on_user_input"},
		},
		{
			name:      "strings_contains_chain_on_query",
			line:      `	if strings.Contains(query, "expense") {`,
			wantRules: []string{"strings_contains_intent_chain"},
		},
		{
			name:      "keyword_to_scenario_id_map",
			line:      `var router = map[string]ScenarioID{}`,
			wantRules: []string{"keyword_to_scenario_map"},
		},
		// Negative cases: anchored format validators must NOT trip.
		{
			name:      "anchored_id_validator_exempt",
			line:      `var scenarioIDPattern = regexp.MustCompile(` + "`" + `^[a-z][a-z0-9_]*$` + "`" + `)`,
			wantRules: nil,
		},
		{
			name:      "switch_on_typed_value_exempt",
			line:      `	switch outcome.Class {`,
			wantRules: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanLineForViolations(tc.line)
			if len(got) != len(tc.wantRules) {
				t.Fatalf("rule count: got %v, want %v (line=%q)", got, tc.wantRules, tc.line)
			}
			for i, want := range tc.wantRules {
				if got[i] != want {
					t.Fatalf("rule[%d]: got %q, want %q (line=%q)", i, got[i], want, tc.line)
				}
			}
		})
	}
}

// repoRootForTests walks up from the test binary's working dir until
// it finds go.mod, returning that absolute path. Tests run from the
// package dir (tests/integration/agent), so we walk up two levels.
func repoRootForTests(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root (go.mod) from %s", wd)
	return ""
}

// itoa avoids the strconv import for a single small integer formatting
// site. Keeping the import surface small makes the guard easier to
// audit.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
