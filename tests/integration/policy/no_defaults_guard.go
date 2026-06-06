//go:build integration || e2e

// Spec 067 Scope 4 — NO-DEFAULTS source guards.
//
// Two guards live here:
//
//   G067-A05  Python runtime SST read with non-empty fallback under
//             ml/app/. Matches os.getenv("KEY", "literal") and
//             os.environ.get("KEY", "literal") where the literal
//             fallback is non-empty. Empty-string fallbacks are
//             allowed because they delegate the failure decision to
//             the caller (the fail-loud pattern used across ml/app/).
//
//   G067-A06  Go runtime config read under internal/ that replaces a
//             missing SST value with a literal fallback. Matches the
//             classic two-step pattern:
//
//                 v := os.Getenv("KEY")
//                 if v == "" { v = "literal" }
//
//             and the inline if-init equivalent. Safe patterns that
//             append to an error slice or return an error in the
//             empty branch are NOT flagged because they do not
//             silently supply a literal value.
//
// Both guards honor the same source-side exception annotation
// machinery Scope 3 introduced (lineHasAcceptedException). Both
// guards point at .github/instructions/smackerel-no-defaults.instructions.md
// as the policy source so CI consumers can find the rule.

package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	noDefaultsPolicySource = ".github/instructions/smackerel-no-defaults.instructions.md"
	noDefaultsOwner        = "intent-policy-reviewer"
)

// pythonRuntimeFallback matches os.getenv("KEY", "literal") and
// os.environ.get("KEY", "literal") where the literal fallback is a
// non-empty double- or single-quoted string. The first submatch is
// the SST key; the second is the fallback literal (without quotes).
var pythonRuntimeFallback = regexp.MustCompile(
	`os\.(?:getenv|environ\.get)\(\s*["']([A-Z_][A-Z0-9_]*)["']\s*,\s*["']([^"']+)["']\s*\)`,
)

// goEnvAssign matches the first half of the two-step Go fallback
// pattern: a variable bound to os.Getenv("KEY"). The first submatch
// is the local identifier, the second is the SST key.
var goEnvAssign = regexp.MustCompile(
	`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:?=\s*os\.Getenv\(\s*"([A-Z_][A-Z0-9_]*)"\s*\)\s*$`,
)

// goEnvInlineIfInit matches the if-init form:
//
//	if v := os.Getenv("KEY"); v == "" { ... }
//
// First submatch is the local identifier, second is the SST key.
var goEnvInlineIfInit = regexp.MustCompile(
	`if\s+([A-Za-z_][A-Za-z0-9_]*)\s*:=\s*os\.Getenv\(\s*"([A-Z_][A-Z0-9_]*)"\s*\)\s*;\s*[A-Za-z_][A-Za-z0-9_]*\s*==\s*""\s*\{`,
)

// goLiteralAssignTemplate produces the pattern that matches
// `<name> = "non-empty-literal"` or `<name> := "non-empty-literal"`.
// We build it per-name so we never flag an unrelated assignment.
func goLiteralAssignRegex(name string) *regexp.Regexp {
	return regexp.MustCompile(
		`(?m)\b` + regexp.QuoteMeta(name) + `\s*:?=\s*"([^"]+)"`,
	)
}

// listPythonFiles returns the sorted set of .py files under root.
func listPythonFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "__pycache__" || name == "tests" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".py") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// PythonNoDefaultsGuard scans ml/app/ for runtime SST reads with
// non-empty fallback values (G067-A05).
func PythonNoDefaultsGuard(repo Root, baseline *Baseline, now time.Time, cfg PolicyConfig) ([]Violation, error) {
	root := filepath.Join(string(repo), "ml", "app")
	files, err := listPythonFiles(root)
	if err != nil {
		return nil, fmt.Errorf("no-defaults-python: walk %s: %w", root, err)
	}
	var out []Violation
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("no-defaults-python: read %s: %w", path, err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			// Skip comment-only lines and docstrings; the policy
			// doc itself quotes the forbidden form for illustration.
			trimmed := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			m := pythonRuntimeFallback.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			key := m[1]
			fallback := m[2]
			rel := relToRepo(path, repo)
			if waived, exc := lineHasAcceptedException(lines, i, baseline, now, cfg, "G067-A05", rel); waived {
				continue
			} else if exc != nil {
				out = append(out, *exc)
				continue
			}
			out = append(out, Violation{
				RuleID:       "G067-A05",
				RuleName:     "Python runtime SST read with non-empty fallback",
				Path:         rel,
				Line:         i + 1,
				Detail:       fmt.Sprintf("os.getenv/environ.get for %q in %s silently substitutes literal %q; required form is os.environ[%q] or an explicit fail-loud check", key, rel, fallback, key),
				PolicySource: noDefaultsPolicySource,
				Owner:        noDefaultsOwner,
				Resolution:   "replace the literal fallback with os.environ[KEY] (raises KeyError) or an explicit empty-value check that fails loud per smackerel-no-defaults policy",
			})
		}
	}
	return out, nil
}

// GoNoDefaultsGuard scans internal/ for runtime config reads that
// replace a missing SST value with a literal fallback (G067-A06).
func GoNoDefaultsGuard(repo Root, baseline *Baseline, now time.Time, cfg PolicyConfig) ([]Violation, error) {
	root := filepath.Join(string(repo), "internal")
	files, err := listGoFiles(root)
	if err != nil {
		return nil, fmt.Errorf("no-defaults-go: walk %s: %w", root, err)
	}
	var out []Violation
	const window = 6
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("no-defaults-go: read %s: %w", path, err)
		}
		lines := strings.Split(string(data), "\n")
		rel := relToRepo(path, repo)
		for i, line := range lines {
			// Skip pure-comment lines so the policy doc text
			// embedded in comments is not flagged.
			if strings.HasPrefix(strings.TrimLeft(line, " \t"), "//") {
				continue
			}

			// Two-step pattern: `v := os.Getenv("KEY")` followed
			// within `window` lines by `if v == ""` and an
			// assignment to v of a non-empty literal in the block.
			if m := goEnvAssign.FindStringSubmatch(line); m != nil {
				name := m[1]
				key := m[2]
				if v := findGoFallbackAssignment(lines, i+1, window, name); v != nil {
					if waived, exc := lineHasAcceptedException(lines, i, baseline, now, cfg, "G067-A06", rel); waived {
						continue
					} else if exc != nil {
						out = append(out, *exc)
						continue
					}
					out = append(out, Violation{
						RuleID:       "G067-A06",
						RuleName:     "Go runtime SST read with literal fallback",
						Path:         rel,
						Line:         i + 1,
						Detail:       fmt.Sprintf("os.Getenv(%q) in %s falls back to literal %q; required form is fail-loud (append to error slice / return error)", key, rel, *v),
						PolicySource: noDefaultsPolicySource,
						Owner:        noDefaultsOwner,
						Resolution:   "remove the literal fallback and fail loud when the SST value is empty per smackerel-no-defaults policy",
					})
					continue
				}
			}

			// Inline if-init: `if v := os.Getenv("KEY"); v == "" { ... }`.
			if m := goEnvInlineIfInit.FindStringSubmatch(line); m != nil {
				key := m[2]
				if v := findGoFallbackLiteralAny(lines, i+1, window); v != nil {
					if waived, exc := lineHasAcceptedException(lines, i, baseline, now, cfg, "G067-A06", rel); waived {
						continue
					} else if exc != nil {
						out = append(out, *exc)
						continue
					}
					out = append(out, Violation{
						RuleID:       "G067-A06",
						RuleName:     "Go runtime SST read with literal fallback",
						Path:         rel,
						Line:         i + 1,
						Detail:       fmt.Sprintf("if-init os.Getenv(%q) in %s assigns literal %q in the empty branch; required form is fail-loud", key, rel, *v),
						PolicySource: noDefaultsPolicySource,
						Owner:        noDefaultsOwner,
						Resolution:   "remove the literal fallback and fail loud when the SST value is empty per smackerel-no-defaults policy",
					})
					continue
				}
			}
		}
	}
	return out, nil
}

// findGoFallbackAssignment looks ahead up to `window` lines for an
// `if <name> == ""` block; if found, scans the block body (up to the
// closing `}` or window end) for `<name> = "literal"` where the
// literal is non-empty. Returns the literal or nil.
func findGoFallbackAssignment(lines []string, start, window int, name string) *string {
	end := start + window
	if end > len(lines) {
		end = len(lines)
	}
	emptyCheck := regexp.MustCompile(`if\s+` + regexp.QuoteMeta(name) + `\s*==\s*""\s*\{`)
	assign := goLiteralAssignRegex(name)
	for j := start; j < end; j++ {
		if !emptyCheck.MatchString(lines[j]) {
			continue
		}
		// Walk forward from the if-line looking for the
		// assignment, stopping at the matching closing brace (best
		// effort: stop at first `}` at the if-line's indent or
		// after `window` lines).
		bodyEnd := j + window
		if bodyEnd > len(lines) {
			bodyEnd = len(lines)
		}
		for k := j + 1; k < bodyEnd; k++ {
			if strings.TrimSpace(lines[k]) == "}" {
				break
			}
			if m := assign.FindStringSubmatch(lines[k]); m != nil && m[1] != "" {
				v := m[1]
				return &v
			}
		}
	}
	return nil
}

// findGoFallbackLiteralAny is the inline-if-init variant: from `start`,
// scan up to `window` lines looking for any `<ident> = "literal"`
// assignment with a non-empty literal before the matching `}`.
func findGoFallbackLiteralAny(lines []string, start, window int) *string {
	end := start + window
	if end > len(lines) {
		end = len(lines)
	}
	anyAssign := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.\[\]]*\s*:?=\s*"([^"]+)"`)
	for j := start; j < end; j++ {
		if strings.TrimSpace(lines[j]) == "}" {
			return nil
		}
		if m := anyAssign.FindStringSubmatch(lines[j]); m != nil {
			v := m[1]
			return &v
		}
	}
	return nil
}
