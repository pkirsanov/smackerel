package web

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
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

// TestAppShellNav_CrossSurfaceCoreParity locks the spec-100 cross-surface
// invariant that spec 100 left UNGUARDED: the app-shell nav is really TWO
// hand-maintained sources — the server nav (this package's appShellNav const)
// and the PWA nav (web/pwa/lib/appnav.js `ITEMS`) — and nothing kept the shared
// journey set in sync. A journey could silently disappear from one surface (the
// SR-04-adjacent chaos gap: one surface's nav being a strict subset of the
// other's).
//
// This guard encodes the INTENDED invariant, NOT nav equality: the two navs are
// deliberately different per surface (the server adds a `knowledge` deep link;
// the PWA adds capture/connectors/photos). It asserts only that every SHARED
// CORE journey is present on BOTH surfaces, while allowing surface-specific
// extras.
//
// The canonical core is the journey set the appShellNav doc comment names as
// "reachable from every surface" (assistant, knowledge/search, cards,
// notifications, settings), expressed via the stable data-nav / ITEMS `key`
// token each surface uses for that journey. `knowledge` (/knowledge) is a
// server-only DEEP link within the shared knowledge/search journey — the PWA
// reaches that journey via Search "/" — so it is a legitimate server extra, and
// its server presence is already independently guarded by TestAppShellNav_Present.
func TestAppShellNav_CrossSurfaceCoreParity(t *testing.T) {
	// The shared cross-surface core, by nav key. Derived by reading BOTH sources:
	// these are exactly the keys the server appShellNav (data-nav="…") and the PWA
	// ITEMS (key: "…") both carry. Adding a key here that only one surface has
	// would (correctly) fail — that is the point.
	coreKeys := []string{"assistant", "search", "cards", "notifications", "settings"}

	// Server side: the in-package appShellNav const carries data-nav="<key>".
	for _, key := range coreKeys {
		if !strings.Contains(appShellNav, `data-nav="`+key+`"`) {
			t.Errorf("server appShellNav dropped core journey %q (expected data-nav=%q); a shared-core journey must never disappear from the server surface", key, key)
		}
	}

	// PWA side: read the real source (web/pwa/lib/appnav.js) rather than a copy,
	// so the guard tracks the single source of truth, and assert each core
	// journey has an ITEMS entry keyed `key: "<key>"` (whitespace-tolerant).
	pwaNavPath := filepath.Join("..", "..", "web", "pwa", "lib", "appnav.js")
	raw, err := os.ReadFile(pwaNavPath)
	if err != nil {
		t.Fatalf("read PWA nav source %s: %v", pwaNavPath, err)
	}
	pwaNav := string(raw)
	for _, key := range coreKeys {
		pat := regexp.MustCompile(`key\s*:\s*"` + regexp.QuoteMeta(key) + `"`)
		if !pat.MatchString(pwaNav) {
			t.Errorf("PWA ITEMS (%s) dropped core journey %q (expected key:%q); a shared-core journey must never disappear from the PWA surface", pwaNavPath, key, key)
		}
	}
}
