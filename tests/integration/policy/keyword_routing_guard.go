//go:build integration || e2e

// Spec 067 Scope 3 — Keyword routing guard expansion.
//
// Two guards live here:
//
//   G067-A03  Forbidden keyword routing pattern in API user-request
//             paths. Any `regexp.MustCompile(...)` assigned to a
//             variable whose name implies intent/routing/classification
//             under internal/api/ is a violation. Anchored
//             `^...$` patterns are exempt (those are syntactic
//             validators, not routing classifiers — same carve-out
//             spec 037 §4.3 uses).
//
//   G067-A04  Free-text keyword map in user-request paths under
//             internal/telegram/ and internal/annotation/. Any
//             string-keyed map literal or field whose identifier
//             implies intent/routing/scenario/classification is a
//             violation. Carrier-style maps (request bodies,
//             metadata, tokens, headers) are not flagged because
//             their identifiers do not name routing.
//
// A source-side policy exception annotation immediately above the
// violating line waives the violation locally when the annotation is
// well-formed and the exception ID is present in the committed
// baseline (Scope 1 ratchet). The annotation grammar mirrors
// design.md → Exception Annotation Contracts:
//
//   // smackerel:policy-exception id=<ID> rule=<RuleID> owner=<owner> expires=<YYYY-MM-DD> reason="..."
//
// Missing metadata, expired exceptions, and unknown IDs are still
// reported as violations.

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
	keywordRoutingPolicySource = "specs/067-intent-driven-policy-enforcement/spec.md"
	keywordRoutingOwner        = "intent-policy-reviewer"
)

// routingIdentifierName matches variable/field names that imply
// routing, intent classification, or scenario selection. Narrow on
// purpose so request-body maps and unrelated regexes do not trip the
// guard.
var routingIdentifierName = regexp.MustCompile(`(?i)(intent|route|router|routing|classif|keyword|scenario)`)

// anchoredRegexShape matches `regexp.MustCompile("^...$")` and the
// backtick form — these are full-string format validators, not
// free-text routing classifiers.
var anchoredRegexShape = regexp.MustCompile("regexp\\.MustCompile\\(\\s*[`\"]\\^[^`\"]*\\$[`\"]\\s*\\)")

// regexRoutingDecl matches a regex assignment of shape
// `<name> ... = regexp.MustCompile(...)`. The first submatch is the
// identifier name being assigned. Covers `var X = ...`,
// `X = ...`, and `X := ...` forms.
var regexRoutingDecl = regexp.MustCompile(`(?:^|\s)([A-Za-z_][A-Za-z0-9_]*)\s*(?::?=)\s*regexp\.MustCompile\(`)

// stringKeyedMapDecl matches identifier declarations whose type is a
// string-keyed map. The first submatch is the identifier name and
// the second is the (best-effort) value type. Covers `var X = map[string]T{`,
// `X := map[string]T{`, and struct field `X map[string]T`.
var stringKeyedMapDecl = regexp.MustCompile(`(?:^|\s)([A-Za-z_][A-Za-z0-9_]*)\s+(?:=\s*)?map\[string\]([A-Za-z0-9_\.\*\[\]]+)`)

// stringKeyedMapShortDecl handles the `X := map[string]T{` form
// which the previous regex does not match cleanly because `:=`
// disambiguates value from type.
var stringKeyedMapShortDecl = regexp.MustCompile(`(?:^|\s)([A-Za-z_][A-Za-z0-9_]*)\s*:=\s*map\[string\]([A-Za-z0-9_\.\*\[\]]+)`)

// exceptionAnnotation parses a source-side policy exception comment.
// Accepts both Go (`//`) and Python (`#`) comment markers so the
// same baseline machinery covers both languages.
var exceptionAnnotation = regexp.MustCompile(`(?://|#)\s*smackerel:policy-exception\s+(.+)$`)

// parseExceptionAnnotation extracts an Exception from a `//
// smackerel:policy-exception ...` line. Returns ok=false if the
// line is not an annotation. Returns a partially-populated Exception
// when the annotation is present but missing fields (so the caller
// can route ValidateException for a structured error).
func parseExceptionAnnotation(line string) (Exception, bool) {
	m := exceptionAnnotation.FindStringSubmatch(line)
	if m == nil {
		return Exception{}, false
	}
	e := Exception{}
	// Tokenise key=value or key="quoted value" pairs.
	body := strings.TrimSpace(m[1])
	tokens := tokeniseAnnotation(body)
	for _, t := range tokens {
		eq := strings.IndexByte(t, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(t[:eq])
		v := strings.Trim(strings.TrimSpace(t[eq+1:]), `"`)
		switch k {
		case "id":
			e.ID = v
		case "rule":
			e.RuleID = v
		case "owner":
			e.Owner = v
		case "expires", "expires_on":
			e.ExpiresOn = v
		case "reason":
			e.Reason = v
		}
	}
	return e, true
}

// tokeniseAnnotation splits an annotation body into key=value
// tokens, respecting double-quoted values that contain whitespace.
func tokeniseAnnotation(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
			cur.WriteByte(c)
		case c == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// listGoFiles returns the sorted set of non-test Go files under
// root, recursively. Test files (_test.go) and vendored paths are
// skipped.
func listGoFiles(root string) ([]string, error) {
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
			if name == "vendor" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
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

// KeywordRoutingGuard scans internal/api/ under repo for forbidden
// keyword/regex routing patterns (G067-A03). Source-side exceptions
// are honored when present in the supplied baseline AND valid per
// ValidateException; otherwise the violation is reported.
func KeywordRoutingGuard(repo Root, baseline *Baseline, now time.Time, cfg PolicyConfig) ([]Violation, error) {
	apiRoot := filepath.Join(string(repo), "internal", "api")
	files, err := listGoFiles(apiRoot)
	if err != nil {
		return nil, fmt.Errorf("keyword-routing: walk %s: %w", apiRoot, err)
	}
	var out []Violation
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("keyword-routing: read %s: %w", path, err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			m := regexRoutingDecl.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			ident := m[1]
			if !routingIdentifierName.MatchString(ident) {
				continue
			}
			if anchoredRegexShape.MatchString(line) {
				continue
			}
			rel := relToRepo(path, repo)
			if waived, exc := lineHasAcceptedException(lines, i, baseline, now, cfg, "G067-A03", rel); waived {
				continue
			} else if exc != nil {
				out = append(out, *exc)
				continue
			}
			out = append(out, Violation{
				RuleID:       "G067-A03",
				RuleName:     "Forbidden keyword routing pattern",
				Path:         rel,
				Line:         i + 1,
				Detail:       fmt.Sprintf("regex %q in %s appears to drive request routing; replace with compiled intent (spec 068) or add a structured policy exception", ident, rel),
				PolicySource: keywordRoutingPolicySource,
				Owner:        keywordRoutingOwner,
				Resolution:   "route the request through the spec 068 intent.Compiler instead of a free-text regex classifier",
			})
		}
	}
	return out, nil
}

// KeywordMapGuard scans internal/telegram/ and internal/annotation/
// for forbidden free-text keyword maps (G067-A04).
func KeywordMapGuard(repo Root, baseline *Baseline, now time.Time, cfg PolicyConfig) ([]Violation, error) {
	var files []string
	for _, sub := range []string{"telegram", "annotation"} {
		fs, err := listGoFiles(filepath.Join(string(repo), "internal", sub))
		if err != nil {
			return nil, fmt.Errorf("keyword-map: walk internal/%s: %w", sub, err)
		}
		files = append(files, fs...)
	}
	sort.Strings(files)
	var out []Violation
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("keyword-map: read %s: %w", path, err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			ident, valueType, ok := matchStringKeyedMap(line)
			if !ok {
				continue
			}
			if !routingIdentifierName.MatchString(ident) {
				continue
			}
			rel := relToRepo(path, repo)
			if waived, exc := lineHasAcceptedException(lines, i, baseline, now, cfg, "G067-A04", rel); waived {
				continue
			} else if exc != nil {
				out = append(out, *exc)
				continue
			}
			out = append(out, Violation{
				RuleID:       "G067-A04",
				RuleName:     "Forbidden free-text keyword map",
				Path:         rel,
				Line:         i + 1,
				Detail:       fmt.Sprintf("identifier %q (map[string]%s) in %s appears to map free-text keys to routing/scenario values", ident, valueType, rel),
				PolicySource: keywordRoutingPolicySource,
				Owner:        keywordRoutingOwner,
				Resolution:   "remove the keyword map and route the surface through compiled intent (spec 068); diagnostic-only retention requires a structured policy exception",
			})
		}
	}
	return out, nil
}

// matchStringKeyedMap returns the identifier name and value type of
// a string-keyed map declaration on the line, or ok=false.
func matchStringKeyedMap(line string) (string, string, bool) {
	if m := stringKeyedMapShortDecl.FindStringSubmatch(line); m != nil {
		return m[1], m[2], true
	}
	if m := stringKeyedMapDecl.FindStringSubmatch(line); m != nil {
		return m[1], m[2], true
	}
	return "", "", false
}

// lineHasAcceptedException inspects the previous non-blank line for
// a `smackerel:policy-exception` annotation. Returns (waived=true,
// nil) when the annotation is well-formed, IDs match the supplied
// rule, the exception is present in the baseline, and
// ValidateException passes. Returns (waived=false, *Violation) when
// an annotation is present but malformed/expired/unknown — the
// caller should append that Violation instead of the routing
// violation. Returns (false, nil) when no annotation is present.
func lineHasAcceptedException(lines []string, idx int, baseline *Baseline, now time.Time, cfg PolicyConfig, ruleID, relPath string) (bool, *Violation) {
	for j := idx - 1; j >= 0; j-- {
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		exc, ok := parseExceptionAnnotation(t)
		if !ok {
			return false, nil
		}
		exc.Path = relPath
		if exc.RuleID != ruleID {
			v := Violation{
				RuleID:       ruleID,
				RuleName:     "policy-exception rule mismatch",
				Path:         relPath,
				Line:         idx + 1,
				Detail:       fmt.Sprintf("source annotation cites rule %q but applies to %q", exc.RuleID, ruleID),
				PolicySource: keywordRoutingPolicySource,
				Owner:        exc.Owner,
				Resolution:   "correct the rule field in the // smackerel:policy-exception annotation",
			}
			return false, &v
		}
		if vErr := ValidateException(exc, now, cfg); vErr != nil {
			vErr.Line = idx + 1
			vErr.Path = relPath
			return false, vErr
		}
		if baseline == nil {
			v := Violation{
				RuleID:       "G067-A07",
				RuleName:     "policy-exception not in baseline",
				Path:         relPath,
				Line:         idx + 1,
				Detail:       fmt.Sprintf("source annotation %q has no committed baseline entry", exc.ID),
				PolicySource: keywordRoutingPolicySource,
				Owner:        exc.Owner,
				Resolution:   "bump policy-exception-baseline.json in the same commit",
			}
			return false, &v
		}
		found := false
		for _, b := range baseline.Exceptions {
			if b.ID == exc.ID {
				found = true
				break
			}
		}
		if !found {
			v := Violation{
				RuleID:       "G067-A07",
				RuleName:     "policy-exception not in baseline",
				Path:         relPath,
				Line:         idx + 1,
				Detail:       fmt.Sprintf("source annotation %q has no committed baseline entry", exc.ID),
				PolicySource: keywordRoutingPolicySource,
				Owner:        exc.Owner,
				Resolution:   "bump policy-exception-baseline.json in the same commit",
			}
			return false, &v
		}
		return true, nil
	}
	return false, nil
}

// relToRepo trims the repo-root prefix from p so guard output is
// portable across checkouts.
func relToRepo(p string, repo Root) string {
	rel, err := filepath.Rel(string(repo), p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(rel)
}
