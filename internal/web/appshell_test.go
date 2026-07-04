package web

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Spec 100 SCOPE-01 — the single-source app-shell nav must be resolvable in
// BOTH server template sets (knowledge-base + card-rewards) and must render the
// assistant + cards cross-surface links (SCN-100-01/02).

func TestAppShellNav_Present(t *testing.T) {
	// Knowledge-base template set.
	kb := NewHandler(nil, nil, time.Now())
	if kb.Templates.Lookup("app-shell-nav") == nil {
		t.Fatalf("app-shell-nav partial missing from the knowledge-base template set")
	}

	// Card-rewards template set (construction does not dereference the service).
	cards := NewCardRewardsWebHandler(nil)
	if cards.Templates.Lookup("app-shell-nav") == nil {
		t.Fatalf("app-shell-nav partial missing from the card-rewards template set")
	}

	// The partial renders the assistant-first cross-surface links.
	var buf bytes.Buffer
	if err := kb.Templates.ExecuteTemplate(&buf, "app-shell-nav", nil); err != nil {
		t.Fatalf("render app-shell-nav (kb): %v", err)
	}
	got := buf.String()
	for _, want := range []string{
		`href="/assistant"`, `href="/cards"`, `href="/knowledge"`,
		`href="/notifications"`, `href="/settings"`, `>Assistant<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("app-shell-nav missing %q; got: %s", want, got)
		}
	}

	// The shared "head" of each set embeds the partial (so every page that uses
	// the head gets the cross-surface bar).
	for name, tmplSet := range map[string]*Handler{"kb": kb} {
		var hb bytes.Buffer
		if err := tmplSet.Templates.ExecuteTemplate(&hb, "head", map[string]any{"Title": "Test"}); err != nil {
			t.Fatalf("render head (%s): %v", name, err)
		}
		if !strings.Contains(hb.String(), `href="/assistant"`) {
			t.Errorf("%s head does not embed the assistant link", name)
		}
	}

	var ch bytes.Buffer
	if err := cards.Templates.ExecuteTemplate(&ch, "head", map[string]any{"Title": "Card Rewards"}); err != nil {
		t.Fatalf("render card head: %v", err)
	}
	if !strings.Contains(ch.String(), `href="/assistant"`) {
		t.Errorf("card head does not embed the assistant link")
	}
}

// TestAppShellNav_NoInlineHandlers locks the CSP posture: the shared nav must
// carry no inline event handlers and no inline <script> (SCN-100-09).
func TestAppShellNav_NoInlineHandlers(t *testing.T) {
	for _, bad := range []string{"onclick=", "onload=", "onsubmit=", "onerror=", "<script"} {
		if strings.Contains(appShellNav, bad) {
			t.Errorf("appShellNav must not contain %q (CSP violation)", bad)
		}
	}
}
