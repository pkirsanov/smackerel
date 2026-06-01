// configcatalog.go — spec 075 SCOPE-2 concrete RetiredCommandCatalog
// built from the LegacyRetirementConfig SST surface.
//
// The retired-command tokens are the keys of the SST
// notice_copy_per_command map; the replacement example and notice
// copy come from that same map and post_window_unknown_response_copy.
// Operators own the contents through config/smackerel.yaml; the
// catalog does not invent or expand the list.
package legacyretirement

import (
	"fmt"
	"sort"
	"strings"
)

// CatalogConfig is the subset of LegacyRetirementConfig the catalog
// needs. Declared as a small struct so this package does not import
// internal/config (avoiding an import cycle) and so tests can build
// catalogs without a full config object.
type CatalogConfig struct {
	NoticeCopyPerCommand          map[string]string
	PostWindowUnknownResponseCopy map[string]string
	Spec066IDPrefix               string
}

// configCatalog is the concrete RetiredCommandCatalog wired from SST.
type configCatalog struct {
	byToken map[string]RetiredCommand
	all     []RetiredCommand
}

// NewConfigCatalog returns a RetiredCommandCatalog whose entries are
// exactly the keys of cfg.NoticeCopyPerCommand. Every notice key must
// also appear in PostWindowUnknownResponseCopy; missing entries fail
// loud here so a misconfigured rollout cannot start.
func NewConfigCatalog(cfg CatalogConfig) (RetiredCommandCatalog, error) {
	if len(cfg.NoticeCopyPerCommand) == 0 {
		return nil, fmt.Errorf("legacyretirement: empty notice_copy_per_command; refuse to construct empty catalog")
	}
	if len(cfg.PostWindowUnknownResponseCopy) == 0 {
		return nil, fmt.Errorf("legacyretirement: empty post_window_unknown_response_copy; refuse to construct catalog without closed-window copy")
	}

	byToken := make(map[string]RetiredCommand, len(cfg.NoticeCopyPerCommand))
	tokens := make([]string, 0, len(cfg.NoticeCopyPerCommand))
	for token, notice := range cfg.NoticeCopyPerCommand {
		if strings.TrimSpace(token) == "" {
			return nil, fmt.Errorf("legacyretirement: notice_copy_per_command contains empty token")
		}
		if strings.TrimSpace(notice) == "" {
			return nil, fmt.Errorf("legacyretirement: notice_copy_per_command[%q] empty body", token)
		}
		unknown, ok := cfg.PostWindowUnknownResponseCopy[token]
		if !ok {
			return nil, fmt.Errorf("legacyretirement: post_window_unknown_response_copy missing entry for %q", token)
		}
		if strings.TrimSpace(unknown) == "" {
			return nil, fmt.Errorf("legacyretirement: post_window_unknown_response_copy[%q] empty body", token)
		}
		byToken[token] = RetiredCommand{
			Command:            token,
			ReplacementExample: unknown,
			NoticeCopy:         notice,
			Spec066ID:          spec066IDFor(cfg.Spec066IDPrefix, token),
		}
		tokens = append(tokens, token)
	}
	for token := range cfg.PostWindowUnknownResponseCopy {
		if _, ok := byToken[token]; !ok {
			return nil, fmt.Errorf("legacyretirement: post_window_unknown_response_copy has %q with no matching notice_copy_per_command entry", token)
		}
	}
	sort.Strings(tokens)
	all := make([]RetiredCommand, 0, len(tokens))
	for _, t := range tokens {
		all = append(all, byToken[t])
	}
	return &configCatalog{byToken: byToken, all: all}, nil
}

func (c *configCatalog) Lookup(token string) (RetiredCommand, bool) {
	rc, ok := c.byToken[token]
	return rc, ok
}

func (c *configCatalog) All() []RetiredCommand {
	out := make([]RetiredCommand, len(c.all))
	copy(out, c.all)
	return out
}

func spec066IDFor(prefix, token string) string {
	if prefix == "" {
		prefix = "spec066"
	}
	clean := strings.TrimPrefix(token, "/")
	return prefix + ":" + clean
}

// ClassifyToken returns the leading command token from raw user
// text, or "" if no leading token is present. The classifier looks
// only at the first whitespace-separated field and only honors
// tokens that start with "/". Raw text is NEVER persisted by this
// function — only the token is returned.
func ClassifyToken(rawText string) string {
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		return ""
	}
	// First whitespace-separated field; strip any trailing
	// "@botname" Telegram suffix on the command token.
	first := strings.Fields(trimmed)[0]
	if at := strings.Index(first, "@"); at > 0 {
		first = first[:at]
	}
	return first
}
