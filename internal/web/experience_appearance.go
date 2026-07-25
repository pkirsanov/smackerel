package web

// Spec 106 SCOPE-106-01 — AppearancePreferenceCodec.
//
// One closed, benign appearance preference shared by the server head and the PWA
// pre-paint asset (web/pwa/experience-appearance.js). The server parses the cookie
// BEFORE rendering and emits data-theme/data-density on <html>; the static PWA
// pages resolve the SAME cookie synchronously before the stylesheet. Both sides
// parse the identical "v1:<theme>:<density>" contract.
//
// Invariants (design.md "Appearance Preference", smackerel-no-defaults):
//   - closed enums: theme ∈ {system,light,dark}, density ∈ {comfortable,compact};
//   - missing preference -> explicit initial state (system/comfortable), diagnostic "preference_missing";
//   - invalid version/value -> explicit initial state, diagnostic "preference_invalid";
//   - NO `value || hiddenDefault` and NO localStorage authority;
//   - carries NO user id, session, route, graph view, prompt, card, provider, or readiness value;
//   - retention duration is an EXPLICIT POSITIVE value (fail-loud, never a silent default);
//   - Path=/, SameSite=Lax, Secure required in production.

import (
	"fmt"
	"net/http"
	"strings"
)

// AppearanceTheme is the closed theme enum.
type AppearanceTheme string

const (
	ThemeSystem AppearanceTheme = "system"
	ThemeLight  AppearanceTheme = "light"
	ThemeDark   AppearanceTheme = "dark"
)

// AppearanceDensity is the closed density enum.
type AppearanceDensity string

const (
	DensityComfortable AppearanceDensity = "comfortable"
	DensityCompact     AppearanceDensity = "compact"
)

// AppearanceDiagnostic classifies a parse outcome. Empty means clean.
type AppearanceDiagnostic string

const (
	AppearanceClean   AppearanceDiagnostic = ""
	AppearanceMissing AppearanceDiagnostic = "preference_missing"
	AppearanceInvalid AppearanceDiagnostic = "preference_invalid"
)

// AppearanceCookieName is the closed, non-sensitive cookie name.
const AppearanceCookieName = "smk_appearance"

// appearanceCookieVersion is the payload version prefix.
const appearanceCookieVersion = "v1"

// AppearancePreference is the resolved, valid preference.
type AppearancePreference struct {
	Theme   AppearanceTheme
	Density AppearanceDensity
}

// InitialAppearance is the UX-required explicit initial state used for a missing
// or invalid preference. It is a positive, named value — not a hidden fallback.
func InitialAppearance() AppearancePreference {
	return AppearancePreference{Theme: ThemeSystem, Density: DensityComfortable}
}

func validTheme(t AppearanceTheme) bool {
	return t == ThemeSystem || t == ThemeLight || t == ThemeDark
}

func validDensity(d AppearanceDensity) bool {
	return d == DensityComfortable || d == DensityCompact
}

// ParseAppearanceCookie parses the raw cookie value into a preference plus a
// diagnostic. It fails loud on any malformed input by returning the explicit
// initial state with a diagnostic — it never coerces a bad value into a nearest
// valid enum, and never applies a silent default.
func ParseAppearanceCookie(raw string) (AppearancePreference, AppearanceDiagnostic) {
	if strings.TrimSpace(raw) == "" {
		return InitialAppearance(), AppearanceMissing
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || parts[0] != appearanceCookieVersion {
		return InitialAppearance(), AppearanceInvalid
	}
	theme := AppearanceTheme(parts[1])
	density := AppearanceDensity(parts[2])
	if !validTheme(theme) || !validDensity(density) {
		return InitialAppearance(), AppearanceInvalid
	}
	return AppearancePreference{Theme: theme, Density: density}, AppearanceClean
}

// SerializeAppearanceCookie renders the closed "v1:<theme>:<density>" payload.
// It fails loud if the preference carries an out-of-enum value.
func SerializeAppearanceCookie(p AppearancePreference) (string, error) {
	if !validTheme(p.Theme) {
		return "", fmt.Errorf("appearance: invalid theme %q", p.Theme)
	}
	if !validDensity(p.Density) {
		return "", fmt.Errorf("appearance: invalid density %q", p.Density)
	}
	return appearanceCookieVersion + ":" + string(p.Theme) + ":" + string(p.Density), nil
}

// AppearanceCookieConfig carries the deployment-supplied cookie attributes. The
// retention is an explicit positive SST value; there is no default.
type AppearanceCookieConfig struct {
	// RetentionSeconds is the explicit positive Max-Age. Must be > 0.
	RetentionSeconds int
	// Secure MUST be true in production (design.md); the caller supplies it from
	// the resolved environment posture, never a hidden default.
	Secure bool
}

// Validate enforces the explicit-positive-retention, fail-loud contract.
func (c AppearanceCookieConfig) Validate() error {
	if c.RetentionSeconds <= 0 {
		return fmt.Errorf("appearance: RetentionSeconds must be an explicit positive value, got %d", c.RetentionSeconds)
	}
	return nil
}

// NewAppearanceCookie builds the Set-Cookie for a preference. It validates both
// the config (explicit positive retention) and the preference (closed enums)
// before emitting; either failure is loud. The cookie is intentionally NOT
// HttpOnly so the synchronous pre-paint asset can read it before first paint; it
// is benign and carries no credential or business value.
func NewAppearanceCookie(p AppearancePreference, cfg AppearanceCookieConfig) (*http.Cookie, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	value, err := SerializeAppearanceCookie(p)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     AppearanceCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   cfg.RetentionSeconds,
		Secure:   cfg.Secure,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

// HTMLDataAttributes returns the (data-theme, data-density) pair the server head
// stamps on <html> so the server render matches the PWA pre-paint exactly.
func (p AppearancePreference) HTMLDataAttributes() (theme string, density string) {
	return string(p.Theme), string(p.Density)
}
