package web

// Spec 106 SCOPE-106-01 — proof that the shared visual contracts are MECHANICALLY
// ENFORCEABLE, not merely written down.
//
// Each rule gets two tests:
//
//  1. A GREEN test against the REAL shipped surface. This is what makes the rule
//     load-bearing: breaking the contract in production source breaks the build.
//
//  2. An ADVERSARIAL test that feeds the checker input which violates the rule and
//     asserts it REJECTS that input. Without this half the suite is worthless — a
//     checker with an unreachable failure path passes every green test while
//     enforcing nothing, and that is precisely the failure mode "mechanically
//     enforceable" exists to rule out.
//
// The adversarial cases are written as minimal violating snippets rather than by
// mutating the real file, so a future refactor of the token source cannot
// accidentally disarm them.

import (
	"io/fs"
	"strings"
	"testing"

	pwa "github.com/smackerel/smackerel/web/pwa"
)

func readTokenSource(t *testing.T) string {
	t.Helper()
	data, err := fs.ReadFile(pwa.StaticFiles, ExperienceTokenSourcePath)
	if err != nil {
		t.Fatalf("token source %q is not embedded: %v", ExperienceTokenSourcePath, err)
	}
	return string(data)
}

// requireViolation asserts the checker rejected the adversarial input and that
// it attributed the rejection to the expected rule.
func requireViolation(t *testing.T, rule string, got []ExperienceContractViolation) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("adversarial input for rule %q was ACCEPTED; the checker cannot fail and enforces nothing", rule)
	}
	for _, v := range got {
		if v.Rule == rule {
			return
		}
	}
	t.Fatalf("adversarial input was rejected but not attributed to rule %q; got:\n%s", rule, FormatViolations(got))
}

// requireClean asserts the real shipped surface satisfies the rule.
func requireClean(t *testing.T, rule string, got []ExperienceContractViolation) {
	t.Helper()
	if len(got) > 0 {
		t.Fatalf("real shipped surface violates the %s contract:\n%s", rule, FormatViolations(got))
	}
}

// TestExperienceContractsAreMechanicallyEnforcedOnTheRealSurfaces is the single
// aggregate gate: every rule, every real shared surface, zero violations.
func TestExperienceContractsAreMechanicallyEnforcedOnTheRealSurfaces(t *testing.T) {
	violations, err := CheckExperienceContracts()
	if err != nil {
		t.Fatalf("contract check could not run: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("%d experience-contract violation(s) on the real shared surfaces:\n%s",
			len(violations), FormatViolations(violations))
	}
}

func TestTypographyContractIsEnforceable(t *testing.T) {
	requireClean(t, RuleTypography, CheckTypographyContract(ExperienceTokenSourcePath, readTokenSource(t)))

	t.Run("adversarial/viewport-scaled font", func(t *testing.T) {
		requireViolation(t, RuleTypography, CheckTypographyContract("adversarial.css",
			`.title { font-size: 4.5vw; }`))
	})
	t.Run("adversarial/negative letter-spacing", func(t *testing.T) {
		requireViolation(t, RuleTypography, CheckTypographyContract("adversarial.css",
			`.title { letter-spacing: -0.02em; }`))
	})
	t.Run("adversarial/below readable floor", func(t *testing.T) {
		requireViolation(t, RuleTypography, CheckTypographyContract("adversarial.css",
			`.tiny { font-size: 9px; }`))
	})
	t.Run("comment mentioning a violation is not a violation", func(t *testing.T) {
		requireClean(t, RuleTypography, CheckTypographyContract("doc.css",
			`/* never write font-size: 4vw or letter-spacing: -1px */ .a { font-size: 15px; }`))
	})
}

func TestStableDimensionContractIsEnforceable(t *testing.T) {
	requireClean(t, RuleStableDimensions, CheckStableDimensionContract(ExperienceTokenSourcePath, readTokenSource(t)))

	t.Run("adversarial/font-relative shell rail", func(t *testing.T) {
		src := strings.Replace(readTokenSource(t), "--shell-rail-width: 240px;", "--shell-rail-width: 18rem;", 1)
		requireViolation(t, RuleStableDimensions, CheckStableDimensionContract("adversarial.css", src))
	})
	t.Run("adversarial/missing required token", func(t *testing.T) {
		requireViolation(t, RuleStableDimensions, CheckStableDimensionContract("adversarial.css",
			`:root { --control-height: 40px; }`))
	})
}

func TestControlTargetContractIsEnforceable(t *testing.T) {
	requireClean(t, RuleControlTarget, CheckControlTargetContract(ExperienceTokenSourcePath, readTokenSource(t)))

	t.Run("adversarial/target below the coarse-pointer minimum", func(t *testing.T) {
		src := strings.Replace(readTokenSource(t), "--touch-target-min: 44px;", "--touch-target-min: 20px;", 1)
		requireViolation(t, RuleControlTarget, CheckControlTargetContract("adversarial.css", src))
	})
	t.Run("adversarial/no icon-only geometry", func(t *testing.T) {
		requireViolation(t, RuleControlTarget, CheckControlTargetContract("adversarial.css",
			`:root { --touch-target-min: 44px; }`))
	})
}

func TestNoOverlapContractIsEnforceable(t *testing.T) {
	requireClean(t, RuleNoOverlap, CheckNoOverlapContract(ExperienceTokenSourcePath, readTokenSource(t)))

	t.Run("adversarial/negative margin", func(t *testing.T) {
		requireViolation(t, RuleNoOverlap, CheckNoOverlapContract("adversarial.css",
			`.row { margin-top: -8px; }`))
	})
	t.Run("adversarial/fixed height cannot grow", func(t *testing.T) {
		requireViolation(t, RuleNoOverlap, CheckNoOverlapContract("adversarial.css",
			".chip {\n  height: 24px;\n}"))
	})
	t.Run("adversarial/unannotated overflow hidden", func(t *testing.T) {
		requireViolation(t, RuleNoOverlap, CheckNoOverlapContract("adversarial.css",
			`.pane { overflow: hidden; }`))
	})
	t.Run("min-height is allowed", func(t *testing.T) {
		requireClean(t, RuleNoOverlap, CheckNoOverlapContract("ok.css",
			".chip {\n  min-height: 24px;\n}"))
	})
}

func TestNoNestedCardContractIsEnforceable(t *testing.T) {
	t.Run("real embedded documents", func(t *testing.T) {
		docs, err := fs.Glob(pwa.StaticFiles, "*.html")
		if err != nil || len(docs) == 0 {
			t.Fatalf("no embedded documents to check (err=%v)", err)
		}
		for _, doc := range docs {
			data, readErr := fs.ReadFile(pwa.StaticFiles, doc)
			if readErr != nil {
				t.Fatalf("cannot read %q: %v", doc, readErr)
			}
			requireClean(t, RuleNoNestedCard, CheckNoNestedCardContract(doc, string(data)))
		}
	})

	t.Run("adversarial/card inside card", func(t *testing.T) {
		requireViolation(t, RuleNoNestedCard, CheckNoNestedCardContract("adversarial.html",
			`<div class="card"><section class="card">nested</section></div>`))
	})
	t.Run("adversarial/card inside card through an intermediate wrapper", func(t *testing.T) {
		requireViolation(t, RuleNoNestedCard, CheckNoNestedCardContract("adversarial.html",
			`<div class="card"><div class="row"><div class="card">deep</div></div></div>`))
	})
	t.Run("adversarial/card-styled landmark", func(t *testing.T) {
		requireViolation(t, RuleNoNestedCard, CheckNoNestedCardContract("adversarial.html",
			`<main class="card">page</main>`))
	})
	t.Run("sibling cards are allowed", func(t *testing.T) {
		requireClean(t, RuleNoNestedCard, CheckNoNestedCardContract("ok.html",
			`<div class="card">one</div><div class="card">two</div>`))
	})
	t.Run("a card after a closed card is allowed", func(t *testing.T) {
		requireClean(t, RuleNoNestedCard, CheckNoNestedCardContract("ok.html",
			`<section><div class="card"><p>one</p></div><div class="card"><p>two</p></div></section>`))
	})
}

func TestIconOnlyControlContractIsEnforceable(t *testing.T) {
	t.Run("real embedded documents", func(t *testing.T) {
		docs, err := fs.Glob(pwa.StaticFiles, "*.html")
		if err != nil || len(docs) == 0 {
			t.Fatalf("no embedded documents to check (err=%v)", err)
		}
		for _, doc := range docs {
			data, readErr := fs.ReadFile(pwa.StaticFiles, doc)
			if readErr != nil {
				t.Fatalf("cannot read %q: %v", doc, readErr)
			}
			requireClean(t, RuleIconOnlyControl, CheckIconOnlyControlContract(doc, string(data)))
		}
	})

	t.Run("adversarial/icon-only button with no accessible name", func(t *testing.T) {
		requireViolation(t, RuleIconOnlyControl, CheckIconOnlyControlContract("adversarial.html",
			`<button class="icon-only" title="Delete"><svg viewBox="0 0 16 16"></svg></button>`))
	})
	t.Run("adversarial/icon-only button with no tooltip", func(t *testing.T) {
		requireViolation(t, RuleIconOnlyControl, CheckIconOnlyControlContract("adversarial.html",
			`<button class="icon-only" aria-label="Delete"><svg viewBox="0 0 16 16"></svg></button>`))
	})
	t.Run("adversarial/empty aria-label does not count as a name", func(t *testing.T) {
		requireViolation(t, RuleIconOnlyControl, CheckIconOnlyControlContract("adversarial.html",
			`<button aria-label="" title="Delete"><svg viewBox="0 0 16 16"></svg></button>`))
	})
	t.Run("icon plus visible text needs no aria-label", func(t *testing.T) {
		requireClean(t, RuleIconOnlyControl, CheckIconOnlyControlContract("ok.html",
			`<button><svg viewBox="0 0 16 16"></svg> Delete</button>`))
	})
	t.Run("fully labelled icon-only control passes", func(t *testing.T) {
		requireClean(t, RuleIconOnlyControl, CheckIconOnlyControlContract("ok.html",
			`<button class="icon-only" aria-label="Delete" title="Delete"><svg viewBox="0 0 16 16"></svg></button>`))
	})
}

func TestForcedColorsContractIsEnforceable(t *testing.T) {
	requireClean(t, RuleForcedColors, CheckForcedColorsContract(ExperienceTokenSourcePath, readTokenSource(t)))

	t.Run("adversarial/no forced-colors block at all", func(t *testing.T) {
		requireViolation(t, RuleForcedColors, CheckForcedColorsContract("adversarial.css",
			`:root { --color-fg: #111111; }`))
	})
	t.Run("adversarial/forced-color-adjust none", func(t *testing.T) {
		requireViolation(t, RuleForcedColors, CheckForcedColorsContract("adversarial.css",
			`@media (forced-colors: active) { .card { forced-color-adjust: none; } }`))
	})
	t.Run("adversarial/literal color inside forced-colors", func(t *testing.T) {
		requireViolation(t, RuleForcedColors, CheckForcedColorsContract("adversarial.css",
			`@media (forced-colors: active) { .card { border: 1px solid #ff0000; } }`))
	})
	t.Run("system color keywords are allowed", func(t *testing.T) {
		requireClean(t, RuleForcedColors, CheckForcedColorsContract("ok.css",
			`@media (forced-colors: active) { .card { border: 1px solid ButtonBorder; } }`))
	})
}

func TestReducedMotionContractIsEnforceable(t *testing.T) {
	requireClean(t, RuleReducedMotion, CheckReducedMotionContract(ExperienceTokenSourcePath, readTokenSource(t)))

	t.Run("adversarial/no reduce block", func(t *testing.T) {
		requireViolation(t, RuleReducedMotion, CheckReducedMotionContract("adversarial.css",
			`:root { --motion-duration: 200ms; }`))
	})
	t.Run("adversarial/reduce block leaves motion running", func(t *testing.T) {
		requireViolation(t, RuleReducedMotion, CheckReducedMotionContract("adversarial.css",
			`:root { --motion-duration: 200ms; }
			 @media (prefers-reduced-motion: reduce) { :root { --motion-duration: 80ms; } }`))
	})
	t.Run("adversarial/reduce block forgets one declared token", func(t *testing.T) {
		requireViolation(t, RuleReducedMotion, CheckReducedMotionContract("adversarial.css",
			`:root { --motion-duration: 200ms; --motion-duration-fast: 120ms; }
			 @media (prefers-reduced-motion: reduce) { :root { --motion-duration: 0ms; } }`))
	})
}

func TestContrastContractIsEnforceable(t *testing.T) {
	requireClean(t, RuleContrast, CheckContrastContract(ExperienceTokenSourcePath, readTokenSource(t)))

	t.Run("ratio math matches the WCAG reference values", func(t *testing.T) {
		// Black on white is the definitional maximum, 21:1.
		if got, ok := ContrastRatio("#000000", "#ffffff"); !ok || got < 20.99 || got > 21.01 {
			t.Fatalf("ContrastRatio(#000000,#ffffff) = %.4f (ok=%v), want 21.00", got, ok)
		}
		// Identical colors are the definitional minimum, 1:1.
		if got, ok := ContrastRatio("#3f51b5", "#3f51b5"); !ok || got < 0.999 || got > 1.001 {
			t.Fatalf("ContrastRatio(same,same) = %.4f (ok=%v), want 1.00", got, ok)
		}
		// The ratio is symmetric: order of arguments must not change the verdict.
		a, _ := ContrastRatio("#767676", "#ffffff")
		b, _ := ContrastRatio("#ffffff", "#767676")
		if a != b {
			t.Fatalf("contrast is not symmetric: %.4f vs %.4f", a, b)
		}
		// #767676 on white is the canonical WCAG AA boundary case, ~4.54:1.
		if a < 4.5 || a > 4.6 {
			t.Fatalf("ContrastRatio(#767676,#ffffff) = %.4f, want ~4.54", a)
		}
	})

	t.Run("adversarial/light body text too close to its background", func(t *testing.T) {
		src := strings.Replace(readTokenSource(t), "--color-fg: #1b1b19;", "--color-fg: #b8b8b4;", 1)
		requireViolation(t, RuleContrast, CheckContrastContract("adversarial.css", src))
	})
	t.Run("adversarial/dark theme regression is caught independently of light", func(t *testing.T) {
		src := strings.Replace(readTokenSource(t), "--color-fg-muted: #9a9aa2;", "--color-fg-muted: #3a3a40;", 1)
		got := CheckContrastContract("adversarial.css", src)
		requireViolation(t, RuleContrast, got)
		if !strings.Contains(FormatViolations(got), "dark theme") {
			t.Fatalf("dark-only regression was not attributed to the dark theme:\n%s", FormatViolations(got))
		}
	})
	t.Run("adversarial/focus ring below the non-text minimum", func(t *testing.T) {
		src := strings.Replace(readTokenSource(t), "--color-focus: #2f5d50;", "--color-focus: #eeeeec;", 1)
		requireViolation(t, RuleContrast, CheckContrastContract("adversarial.css", src))
	})
	t.Run("adversarial/a role the dark block forgot to override is still judged", func(t *testing.T) {
		// Break a light role that the dark block does NOT override. The dark
		// palette inherits it, so both themes must report the violation. This is
		// what an independent per-block parse would miss.
		src := strings.Replace(readTokenSource(t), "--color-surface-raised: #ffffff;", "--color-surface-raised: #1c1c1a;", 1)
		got := CheckContrastContract("adversarial.css", src)
		requireViolation(t, RuleContrast, got)
		if !strings.Contains(FormatViolations(got), "light theme") {
			t.Fatalf("inherited-role regression was not reported for the light theme:\n%s", FormatViolations(got))
		}
	})
}
