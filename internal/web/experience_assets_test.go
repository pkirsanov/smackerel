package web

// Spec 106 SCOPE-106-01 — XP106-01-U.
//
// TestExperienceAssetManifestLocksSourcesLicensesBytesTokensAndAppearanceEnums
// is the unit proof that the source-locked visual foundation is real, not
// fabricated: every declared asset's SHA-256 is recomputed from the actual
// embedded bytes and must match; the semantic token source obeys the mechanical
// rules; external (not-yet-same-origin) dependencies are recorded honestly with
// NO digest; and the appearance codec enforces its closed enums with fail-loud,
// no-default behavior.

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"

	pwa "github.com/smackerel/smackerel/web/pwa"
)

func TestExperienceAssetManifestLocksSourcesLicensesBytesTokensAndAppearanceEnums(t *testing.T) {
	m, err := BuildExperienceAssetManifest()
	if err != nil {
		t.Fatalf("BuildExperienceAssetManifest returned error: %v", err)
	}
	if m.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", m.Version)
	}
	if len(m.Assets) == 0 {
		t.Fatal("manifest has zero locked assets")
	}

	// --- Every locked asset: complete metadata + REAL byte digest --------------
	seenPaths := map[string]bool{}
	for _, a := range m.Assets {
		if a.ServedPath == "" || a.EmbedPath == "" || a.Source == "" || a.License == "" ||
			a.SHA256 == "" || a.MediaType == "" || a.CSPClass == "" || a.SWPolicy == "" {
			t.Errorf("asset %+v has an empty required field", a)
		}
		if a.Size <= 0 {
			t.Errorf("asset %s has non-positive size %d", a.ServedPath, a.Size)
		}
		if a.License != firstPartyLicense {
			t.Errorf("asset %s license = %q, want first-party %q", a.ServedPath, a.License, firstPartyLicense)
		}
		if !strings.HasPrefix(a.ServedPath, "/pwa/") {
			t.Errorf("asset %s is not served same-origin under /pwa/", a.ServedPath)
		}
		if seenPaths[a.ServedPath] {
			t.Errorf("asset %s is declared twice (no duplicate copies allowed)", a.ServedPath)
		}
		seenPaths[a.ServedPath] = true

		// A locked static asset must never sit under a network-only class.
		if IsNetworkOnlyPath(a.ServedPath) {
			t.Errorf("asset %s is precached but classified network-only", a.ServedPath)
		}
		if a.SWPolicy != SWPrecacheImmutable {
			t.Errorf("asset %s SW policy = %q, want %q", a.ServedPath, a.SWPolicy, SWPrecacheImmutable)
		}

		// REAL byte-locking: recompute the digest from the embed and require match.
		data, readErr := fs.ReadFile(pwa.StaticFiles, a.EmbedPath)
		if readErr != nil {
			t.Errorf("declared locked asset %s is not embedded: %v", a.EmbedPath, readErr)
			continue
		}
		sum := sha256.Sum256(data)
		want := hex.EncodeToString(sum[:])
		if a.SHA256 != want {
			t.Errorf("asset %s digest mismatch: manifest=%s recomputed=%s", a.ServedPath, a.SHA256, want)
		}
		if a.Size != int64(len(data)) {
			t.Errorf("asset %s size mismatch: manifest=%d actual=%d", a.ServedPath, a.Size, len(data))
		}
	}

	// The two foundation assets MUST be locked.
	for _, must := range []string{"/pwa/experience-tokens.css", "/pwa/experience-appearance.js"} {
		if !seenPaths[must] {
			t.Errorf("foundation asset %s is not locked in the manifest", must)
		}
	}

	// CacheIdentity is a non-empty, deterministic aggregate digest.
	if len(m.CacheIdentity) != 64 {
		t.Errorf("CacheIdentity = %q, want a 64-hex sha256", m.CacheIdentity)
	}
	if again := computeCacheIdentity(m.Assets); again != m.CacheIdentity {
		t.Errorf("CacheIdentity not deterministic: %s vs %s", m.CacheIdentity, again)
	}
	// Adversarial: mutating a byte must move the identity (proves it is content-derived).
	mutated := append([]ExperienceAsset(nil), m.Assets...)
	mutated[0].SHA256 = strings.Repeat("0", 64)
	if computeCacheIdentity(mutated) == m.CacheIdentity {
		t.Error("CacheIdentity did not change when a locked digest changed")
	}

	// --- External dependencies recorded honestly, WITHOUT a digest -------------
	extByName := map[string]ExternalExperienceDependency{}
	for _, d := range m.ExternalDependencies {
		extByName[d.Name] = d
	}
	if htmx, ok := extByName["htmx.org@1.9.12"]; !ok {
		t.Error("htmx is not recorded as an external dependency (BUG-002-006 owned)")
	} else if htmx.Status != ExtStatusPendingSameOrigin || htmx.Owner != "BUG-002-006" {
		t.Errorf("htmx external record wrong: %+v", htmx)
	}
	for _, font := range []string{"IBM Plex Sans", "Source Serif 4", "IBM Plex Mono"} {
		d, ok := extByName[font]
		if !ok {
			t.Errorf("font %q is not recorded as an external dependency", font)
			continue
		}
		if d.Status != ExtStatusNotVendored {
			t.Errorf("font %q status = %q, want %q (no fabricated digest)", font, d.Status, ExtStatusNotVendored)
		}
	}
	// A font MUST NOT appear as a source-locked ExperienceAsset (no fabricated bytes).
	for _, a := range m.Assets {
		if a.CSPClass == CSPClassFont {
			t.Errorf("a font is locked as a same-origin asset (%s) but no font bytes are vendored", a.ServedPath)
		}
	}

	// --- Semantic token source obeys the mechanical rules ----------------------
	tokens, err := fs.ReadFile(pwa.StaticFiles, "experience-tokens.css")
	if err != nil {
		t.Fatalf("token source not embedded: %v", err)
	}
	tokenSrc := string(tokens)
	for _, name := range []string{
		"--space-1", "--space-4", "--radius-sm", "--radius-xl",
		"--shell-rail-width", "--control-height", "--touch-target-min",
		"--font-sans", "--text-sm", "--focus-ring-width", "--motion-duration",
		"--color-bg", "--color-fg", "--color-accent",
	} {
		if !strings.Contains(tokenSrc, name) {
			t.Errorf("token source is missing required token %s", name)
		}
	}
	// 4px scale + 2-8px radii literals present.
	for _, v := range []string{"4px", "8px", "2px", "6px", "44px"} {
		if !strings.Contains(tokenSrc, v) {
			t.Errorf("token source is missing expected dimension %s", v)
		}
	}
	// Both light and dark color roles defined.
	if !strings.Contains(tokenSrc, `data-theme="dark"`) {
		t.Error("token source has no explicit dark color role")
	}
	if !strings.Contains(tokenSrc, "prefers-color-scheme: dark") {
		t.Error("token source does not follow the OS for the system theme")
	}
	// Mechanical rule: no viewport-width font scaling.
	if regexp.MustCompile(`font-size\s*:[^;]*\b\d*\.?\d+v[wh]`).MatchString(tokenSrc) {
		t.Error("token source uses viewport-width font scaling (forbidden)")
	}
	// Mechanical rule: no negative letter-spacing.
	if regexp.MustCompile(`letter-spacing\s*:\s*-`).MatchString(tokenSrc) {
		t.Error("token source uses negative letter-spacing (forbidden)")
	}

	// --- Appearance codec: closed enums, fail-loud, no default -----------------
	// Valid round-trip.
	pref := AppearancePreference{Theme: ThemeDark, Density: DensityCompact}
	raw, err := SerializeAppearanceCookie(pref)
	if err != nil {
		t.Fatalf("serialize valid preference failed: %v", err)
	}
	if raw != "v1:dark:compact" {
		t.Errorf("serialized = %q, want v1:dark:compact", raw)
	}
	got, diag := ParseAppearanceCookie(raw)
	if diag != AppearanceClean || got != pref {
		t.Errorf("round-trip = (%+v, %q), want (%+v, clean)", got, diag, pref)
	}

	// Missing -> explicit initial state + preference_missing (no silent default).
	got, diag = ParseAppearanceCookie("")
	if got != InitialAppearance() || diag != AppearanceMissing {
		t.Errorf("missing cookie = (%+v, %q), want (initial, preference_missing)", got, diag)
	}

	// Adversarial invalid inputs -> initial + preference_invalid, never coerced.
	for _, bad := range []string{
		"v1:dark",          // too few parts
		"v2:dark:compact",  // wrong version
		"v1:sepia:compact", // out-of-enum theme
		"v1:dark:spacious", // out-of-enum density
		"dark:compact",     // no version
		"v1::compact",      // empty theme
	} {
		got, diag = ParseAppearanceCookie(bad)
		if got != InitialAppearance() || diag != AppearanceInvalid {
			t.Errorf("invalid cookie %q = (%+v, %q), want (initial, preference_invalid)", bad, got, diag)
		}
	}

	// Serialize fails loud on an out-of-enum value.
	if _, err := SerializeAppearanceCookie(AppearancePreference{Theme: "sepia", Density: DensityComfortable}); err == nil {
		t.Error("serialize accepted an out-of-enum theme (must fail loud)")
	}

	// Explicit positive retention: zero/negative fail loud, no default.
	if err := (AppearanceCookieConfig{RetentionSeconds: 0}).Validate(); err == nil {
		t.Error("zero retention accepted (must be explicit positive)")
	}
	if err := (AppearanceCookieConfig{RetentionSeconds: -1}).Validate(); err == nil {
		t.Error("negative retention accepted (must be explicit positive)")
	}
	if _, err := NewAppearanceCookie(pref, AppearanceCookieConfig{RetentionSeconds: 0, Secure: true}); err == nil {
		t.Error("NewAppearanceCookie accepted zero retention (must fail loud)")
	}

	// Production cookie attributes: Path=/, SameSite=Lax, not HttpOnly, Secure honored.
	cookie, err := NewAppearanceCookie(pref, AppearanceCookieConfig{RetentionSeconds: 60 * 60 * 24 * 180, Secure: true})
	if err != nil {
		t.Fatalf("NewAppearanceCookie failed for valid input: %v", err)
	}
	if cookie.Name != AppearanceCookieName || cookie.Path != "/" ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.HttpOnly || !cookie.Secure ||
		cookie.MaxAge <= 0 {
		t.Errorf("cookie attributes wrong: %+v", cookie)
	}
	// The cookie value carries no credential/business content — only the closed payload.
	if cookie.Value != "v1:dark:compact" {
		t.Errorf("cookie value = %q, want the closed appearance payload only", cookie.Value)
	}

	// data-* attribute pair matches the parsed preference.
	th, den := pref.HTMLDataAttributes()
	if th != "dark" || den != "compact" {
		t.Errorf("HTMLDataAttributes = (%q,%q), want (dark,compact)", th, den)
	}

	// Network-only classification.
	if !IsNetworkOnlyPath("/api/capture") || !IsNetworkOnlyPath("/v1/anything") {
		t.Error("network-only prefixes not honored")
	}
	if IsNetworkOnlyPath("/pwa/experience-tokens.css") {
		t.Error("a static same-origin asset was misclassified network-only")
	}
}
