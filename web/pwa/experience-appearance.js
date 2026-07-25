/*
 * Smackerel Coherent Product Experience — pre-paint appearance resolver
 * (spec 106, SCOPE-106-01).
 *
 * Loaded SYNCHRONOUSLY in <head> BEFORE experience-tokens.css so the resolved
 * theme/density is applied to <html> before first paint (no flash). This is the
 * client mirror of the Go AppearancePreferenceCodec (internal/web/experience_assets.go):
 * both parse the SAME closed cookie contract.
 *
 * Contract (design.md "Appearance Preference"):
 *   cookie value = "v1:<theme>:<density>"
 *   theme   ∈ { system, light, dark }
 *   density ∈ { comfortable, compact }
 *   - missing        -> explicit initial state system/comfortable (data-appearance-source="missing")
 *   - invalid ver/val-> explicit initial state + data-appearance-diagnostic="preference_invalid"
 *   - NO `value || default` silent fallback, NO localStorage authority.
 *   - carries NO credential, session, route, or business value.
 *
 * The appearance cookie is the ONLY authority. localStorage is never read or
 * written for appearance. Forced-colors and reduced-motion stay platform-controlled.
 */
(function () {
  "use strict";

  var COOKIE_NAME = "smk_appearance";
  var VERSION = "v1";
  var THEMES = { system: 1, light: 1, dark: 1 };
  var DENSITIES = { comfortable: 1, compact: 1 };
  var INITIAL_THEME = "system";
  var INITIAL_DENSITY = "comfortable";

  function readCookie(name) {
    var pairs = (document.cookie || "").split(";");
    for (var i = 0; i < pairs.length; i++) {
      var p = pairs[i].trim();
      var eq = p.indexOf("=");
      if (eq > -1 && p.slice(0, eq) === name) {
        return decodeURIComponent(p.slice(eq + 1));
      }
    }
    return null;
  }

  // Returns { theme, density, source, diagnostic }. Fail-loud on invalid: never
  // silently coerces a bad value into a "nearest" theme — it resets to the
  // explicit initial state and records the diagnostic.
  function resolve(raw) {
    if (raw === null || raw === "") {
      return { theme: INITIAL_THEME, density: INITIAL_DENSITY, source: "missing", diagnostic: "" };
    }
    var parts = raw.split(":");
    if (parts.length !== 3 || parts[0] !== VERSION ||
        !THEMES[parts[1]] || !DENSITIES[parts[2]]) {
      return { theme: INITIAL_THEME, density: INITIAL_DENSITY, source: "invalid", diagnostic: "preference_invalid" };
    }
    return { theme: parts[1], density: parts[2], source: "cookie", diagnostic: "" };
  }

  var r = resolve(readCookie(COOKIE_NAME));
  var el = document.documentElement;
  el.setAttribute("data-theme", r.theme);
  el.setAttribute("data-density", r.density);
  el.setAttribute("data-appearance-source", r.source);
  if (r.diagnostic) {
    el.setAttribute("data-appearance-diagnostic", r.diagnostic);
  } else {
    el.removeAttribute("data-appearance-diagnostic");
  }

  // Expose the parsed result read-only for the appearance control + tests. This
  // is NOT an authority store — it is a snapshot of the cookie-derived state.
  window.__smkAppearance = Object.freeze({
    theme: r.theme,
    density: r.density,
    source: r.source,
    diagnostic: r.diagnostic,
    cookieName: COOKIE_NAME,
    version: VERSION
  });
})();
