package notification

import (
	"regexp"
	"sort"
)

type RedactionScanState struct {
	Categories []string
}

var redactionPatterns = []struct {
	category string
	pattern  *regexp.Regexp
}{
	{category: "bearer_token", pattern: regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[^\s&]+`)},
	{category: "query_token", pattern: regexp.MustCompile(`(?i)(token|api[_-]?key|secret)=([^\s&]+)`)},
	{category: "password", pattern: regexp.MustCompile(`(?i)password=([^\s&]+)`)},
	{category: "secret_fragment", pattern: regexp.MustCompile(`(?i)\bsecret[-_][A-Za-z0-9._-]+\b`)},
}

func RedactText(input string) (string, RedactionScanState) {
	output := input
	seen := map[string]struct{}{}
	for _, rule := range redactionPatterns {
		if rule.pattern.MatchString(output) {
			seen[rule.category] = struct{}{}
			output = rule.pattern.ReplaceAllStringFunc(output, func(match string) string {
				return "[redacted:" + rule.category + "]"
			})
		}
	}
	categories := make([]string, 0, len(seen))
	for category := range seen {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return output, RedactionScanState{Categories: categories}
}

func RedactionStateMap(state RedactionScanState) map[string]any {
	return map[string]any{"categories": append([]string(nil), state.Categories...), "status": "redacted"}
}

func RedactStringMap(values map[string]string) (map[string]string, map[string]any) {
	redacted := make(map[string]string, len(values))
	categories := map[string]struct{}{}
	for key, value := range values {
		text, state := RedactText(value)
		redacted[key] = text
		for _, category := range state.Categories {
			categories[category] = struct{}{}
		}
	}
	stateCategories := make([]string, 0, len(categories))
	for category := range categories {
		stateCategories = append(stateCategories, category)
	}
	sort.Strings(stateCategories)
	return redacted, map[string]any{"categories": stateCategories, "status": "redacted"}
}
