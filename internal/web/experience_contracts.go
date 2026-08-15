package web

// Spec 106 SCOPE-106-01 — mechanical enforcement of the shared visual contracts.
//
// The scope's Definition of Done requires that the typography, icon, control,
// stable-dimension, no-overlap, no-nested-card, contrast, forced-colors, and
// reduced-motion contracts be MECHANICALLY ENFORCEABLE. Prose in design.md is
// not enforcement: a rule that cannot fail a build is a rule nobody has to obey.
//
// Every rule below is therefore a pure function over source text that returns
// concrete violations. Purity is the load-bearing property, not a style choice:
// it lets the accompanying test feed each checker a deliberately violating
// snippet and assert the checker REJECTS it. A checker that has only ever been
// run against compliant input has not been shown to work at all.
//
// The rules are deliberately syntactic. They catch the mistake that is actually
// made — a hardcoded hex sneaking into a template, a second card wrapping a
// first, a font-size that scales with the viewport — and they make no claim
// about rendered geometry, which only a browser can judge. The Playwright
// specs own that half.

import (
	"fmt"
	"io/fs"
	"math"
	"regexp"
	"strconv"
	"strings"

	pwa "github.com/smackerel/smackerel/web/pwa"
)

// Contract rule identifiers. A violation always names one.
const (
	RuleTypography       = "typography"
	RuleIconOnlyControl  = "icon-only-control"
	RuleControlTarget    = "control-target"
	RuleStableDimensions = "stable-dimensions"
	RuleNoOverlap        = "no-overlap"
	RuleNoNestedCard     = "no-nested-card"
	RuleContrast         = "contrast"
	RuleForcedColors     = "forced-colors"
	RuleReducedMotion    = "reduced-motion"
)

// ExperienceContractViolation is one concrete, located rule breach.
type ExperienceContractViolation struct {
	Rule    string
	Surface string
	Line    int
	Detail  string
}

func (v ExperienceContractViolation) String() string {
	return fmt.Sprintf("%s: %s:%d: %s", v.Rule, v.Surface, v.Line, v.Detail)
}

// FormatViolations renders violations one per line for test failure output.
func FormatViolations(vs []ExperienceContractViolation) string {
	lines := make([]string, 0, len(vs))
	for _, v := range vs {
		lines = append(lines, v.String())
	}
	return strings.Join(lines, "\n")
}

// stripCSSComments removes /* ... */ so a rule documented in prose is never
// mistaken for a rule violated in code. Without this every checker would trip
// on its own explanatory comment, which is how a mechanical rule gets deleted
// for being "noisy" instead of being fixed.
func stripCSSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				// Unterminated comment: preserve newlines so line numbers hold.
				for _, r := range src[i:] {
					if r == '\n' {
						b.WriteByte('\n')
					}
				}
				return b.String()
			}
			for _, r := range src[i : i+2+end+2] {
				if r == '\n' {
					b.WriteByte('\n')
				}
			}
			i += 2 + end + 1
			continue
		}
		b.WriteByte(src[i])
	}
	return b.String()
}

// stripHTMLComments removes <!-- ... --> for the same reason.
func stripHTMLComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				return b.String()
			}
			for _, r := range src[i : i+4+end+3] {
				if r == '\n' {
					b.WriteByte('\n')
				}
			}
			i += 4 + end + 2
			continue
		}
		b.WriteByte(src[i])
	}
	return b.String()
}

func lineOf(src string, idx int) int {
	if idx < 0 || idx > len(src) {
		return 0
	}
	return strings.Count(src[:idx], "\n") + 1
}

// ---------------------------------------------------------------------------
// Typography
// ---------------------------------------------------------------------------

var (
	reViewportFont   = regexp.MustCompile(`font-size\s*:[^;{}]*?(\d*\.?\d+)\s*(vw|vh|vmin|vmax)\b`)
	reNegLetterSpace = regexp.MustCompile(`letter-spacing\s*:\s*-\s*\d`)
	reFontSizePx     = regexp.MustCompile(`font-size\s*:\s*(\d+(?:\.\d+)?)px`)
)

// minReadableFontPx is the smallest font-size any shared surface may declare.
// Compact density tightens spacing, never text (design.md "Density").
const minReadableFontPx = 12

// CheckTypographyContract rejects viewport-scaled font sizes, negative letter
// spacing, and text below the readable floor.
func CheckTypographyContract(surface, src string) []ExperienceContractViolation {
	clean := stripCSSComments(src)
	var out []ExperienceContractViolation

	for _, m := range reViewportFont.FindAllStringSubmatchIndex(clean, -1) {
		out = append(out, ExperienceContractViolation{
			Rule: RuleTypography, Surface: surface, Line: lineOf(clean, m[0]),
			Detail: "font-size scales with the viewport (" + clean[m[2]:m[5]] + "); text must not resize with the window",
		})
	}
	for _, m := range reNegLetterSpace.FindAllStringIndex(clean, -1) {
		out = append(out, ExperienceContractViolation{
			Rule: RuleTypography, Surface: surface, Line: lineOf(clean, m[0]),
			Detail: "negative letter-spacing is forbidden",
		})
	}
	for _, m := range reFontSizePx.FindAllStringSubmatchIndex(clean, -1) {
		px, err := strconv.ParseFloat(clean[m[2]:m[3]], 64)
		if err != nil {
			continue
		}
		if px < minReadableFontPx {
			out = append(out, ExperienceContractViolation{
				Rule: RuleTypography, Surface: surface, Line: lineOf(clean, m[0]),
				Detail: fmt.Sprintf("font-size %gpx is below the %dpx readable floor", px, minReadableFontPx),
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Stable dimensions
// ---------------------------------------------------------------------------

// stableDimensionTokens must resolve to an absolute px length. If a shell rail
// or control height were expressed in `em`/`ch`/`%`, a font-size change would
// silently re-flow the shell — the exact layout jitter this scope exists to
// prevent.
var stableDimensionTokens = []string{
	"--shell-rail-width",
	"--shell-rail-width-collapsed",
	"--shell-header-height",
	"--shell-mobile-bar-height",
	"--control-height",
	"--control-height-compact",
	"--touch-target-min",
}

var reFontRelative = regexp.MustCompile(`^\s*\d*\.?\d+\s*(em|ex|ch|rem|%|vw|vh|vmin|vmax)\s*$`)

// CheckStableDimensionContract requires every shell/control dimension token to
// be declared in absolute px.
func CheckStableDimensionContract(surface, src string) []ExperienceContractViolation {
	clean := stripCSSComments(src)
	var out []ExperienceContractViolation
	for _, token := range stableDimensionTokens {
		re := regexp.MustCompile(regexp.QuoteMeta(token) + `\s*:\s*([^;]+);`)
		ms := re.FindAllStringSubmatchIndex(clean, -1)
		if len(ms) == 0 {
			out = append(out, ExperienceContractViolation{
				Rule: RuleStableDimensions, Surface: surface,
				Detail: "required stable-dimension token " + token + " is not declared",
			})
			continue
		}
		for _, m := range ms {
			val := strings.TrimSpace(clean[m[2]:m[3]])
			if reFontRelative.MatchString(val) {
				out = append(out, ExperienceContractViolation{
					Rule: RuleStableDimensions, Surface: surface, Line: lineOf(clean, m[0]),
					Detail: token + " = " + val + " is font/viewport relative; shell and control dimensions must be absolute px",
				})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Control targets
// ---------------------------------------------------------------------------

var reTouchTargetMin = regexp.MustCompile(`--touch-target-min\s*:\s*(\d+(?:\.\d+)?)px`)

// wcagMinTouchTargetPx is the WCAG 2.2 AA (2.5.8) minimum pointer target.
const wcagMinTouchTargetPx = 24

// coarsePointerTargetPx is the coarse-pointer minimum the design commits to.
const coarsePointerTargetPx = 44

// CheckControlTargetContract requires a declared coarse-pointer minimum that
// clears both the WCAG floor and the design commitment.
func CheckControlTargetContract(surface, src string) []ExperienceContractViolation {
	clean := stripCSSComments(src)
	m := reTouchTargetMin.FindStringSubmatchIndex(clean)
	if m == nil {
		return []ExperienceContractViolation{{
			Rule: RuleControlTarget, Surface: surface,
			Detail: "--touch-target-min is not declared in px; coarse-pointer targets are unenforced",
		}}
	}
	px, err := strconv.ParseFloat(clean[m[2]:m[3]], 64)
	if err != nil {
		return []ExperienceContractViolation{{
			Rule: RuleControlTarget, Surface: surface, Line: lineOf(clean, m[0]),
			Detail: "--touch-target-min is not a parseable px length",
		}}
	}
	var out []ExperienceContractViolation
	if px < wcagMinTouchTargetPx {
		out = append(out, ExperienceContractViolation{
			Rule: RuleControlTarget, Surface: surface, Line: lineOf(clean, m[0]),
			Detail: fmt.Sprintf("--touch-target-min %gpx is below the WCAG 2.5.8 floor of %dpx", px, wcagMinTouchTargetPx),
		})
	}
	if px < coarsePointerTargetPx {
		out = append(out, ExperienceContractViolation{
			Rule: RuleControlTarget, Surface: surface, Line: lineOf(clean, m[0]),
			Detail: fmt.Sprintf("--touch-target-min %gpx is below the %dpx coarse-pointer commitment", px, coarsePointerTargetPx),
		})
	}
	if !strings.Contains(clean, ".icon-only") {
		out = append(out, ExperienceContractViolation{
			Rule: RuleControlTarget, Surface: surface,
			Detail: "no .icon-only geometry is declared; icon-only controls would have no enforced hit area",
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// No overlap
// ---------------------------------------------------------------------------

var (
	reNegativeMargin = regexp.MustCompile(`margin(?:-top|-right|-bottom|-left)?\s*:\s*[^;{}]*(?:^|[\s:])-\s*\d`)
	reFixedHeight    = regexp.MustCompile(`(?m)^\s*height\s*:\s*(\d+(?:\.\d+)?)px\s*;`)
	reOverflowHidden = regexp.MustCompile(`overflow\s*:\s*hidden`)
)

// CheckNoOverlapContract rejects the two static shapes that reliably produce
// overlapping or clipped content on a shared surface: a negative margin pulling
// a box over its neighbour, and a fixed pixel height that cannot grow when a
// long label, a translated string, or a 200%-zoom root font makes the content
// taller.
//
// It cannot prove the absence of overlap — only a rendered layout can, which is
// why the Playwright canaries exist. It removes the two causes that are visible
// in the stylesheet.
func CheckNoOverlapContract(surface, src string) []ExperienceContractViolation {
	clean := stripCSSComments(src)
	var out []ExperienceContractViolation
	for _, m := range reNegativeMargin.FindAllStringIndex(clean, -1) {
		out = append(out, ExperienceContractViolation{
			Rule: RuleNoOverlap, Surface: surface, Line: lineOf(clean, m[0]),
			Detail: "negative margin pulls content over its neighbour: " + strings.TrimSpace(clean[m[0]:m[1]]),
		})
	}
	for _, m := range reFixedHeight.FindAllStringSubmatchIndex(clean, -1) {
		out = append(out, ExperienceContractViolation{
			Rule: RuleNoOverlap, Surface: surface, Line: lineOf(clean, m[0]),
			Detail: "fixed height " + clean[m[2]:m[3]] + "px cannot grow with content; use min-height so long or zoomed text does not clip",
		})
	}
	if reOverflowHidden.MatchString(clean) && !strings.Contains(clean, "overflow-clip-safe") {
		for _, m := range reOverflowHidden.FindAllStringIndex(clean, -1) {
			out = append(out, ExperienceContractViolation{
				Rule: RuleNoOverlap, Surface: surface, Line: lineOf(clean, m[0]),
				Detail: "overflow:hidden clips focus rings and overflowing text; annotate the reviewed exception with an /* overflow-clip-safe */ marker",
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// No nested card / no card-styled page section
// ---------------------------------------------------------------------------

var (
	reTagOpen        = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9-]*)((?:"[^"]*"|'[^']*'|[^>"'])*)>`)
	reTagClose       = regexp.MustCompile(`</([a-zA-Z][a-zA-Z0-9-]*)\s*>`)
	reClassAttr      = regexp.MustCompile(`\bclass\s*=\s*"([^"]*)"|\bclass\s*=\s*'([^']*)'`)
	voidHTMLElements = map[string]bool{
		"area": true, "base": true, "br": true, "col": true, "embed": true,
		"hr": true, "img": true, "input": true, "link": true, "meta": true,
		"param": true, "source": true, "track": true, "wbr": true,
	}
	// pageSectionTags are landmark elements that must never themselves be a card.
	pageSectionTags = map[string]bool{
		"main": true, "nav": true, "header": true, "footer": true, "body": true,
	}
)

func classesOf(attrs string) []string {
	m := reClassAttr.FindStringSubmatch(attrs)
	if m == nil {
		return nil
	}
	val := m[1]
	if val == "" {
		val = m[2]
	}
	return strings.Fields(val)
}

func isCardClass(classes []string) bool {
	for _, c := range classes {
		if c == "card" || c == "card-surface" {
			return true
		}
	}
	return false
}

type htmlFrame struct {
	tag    string
	isCard bool
}

// CheckNoNestedCardContract walks the markup and rejects a card inside a card,
// and a landmark element carrying a card class.
//
// It tracks an explicit element stack rather than pattern-matching text because
// "a card inside a card" is a nesting fact, and a regex over flat text cannot
// tell a nested card from two sibling cards.
func CheckNoNestedCardContract(surface, src string) []ExperienceContractViolation {
	clean := stripHTMLComments(src)
	var out []ExperienceContractViolation
	var stack []htmlFrame
	cardDepth := 0

	type ev struct {
		idx   int
		open  bool
		tag   string
		attrs string
	}
	var events []ev
	for _, m := range reTagOpen.FindAllStringSubmatchIndex(clean, -1) {
		events = append(events, ev{idx: m[0], open: true, tag: strings.ToLower(clean[m[2]:m[3]]), attrs: clean[m[4]:m[5]]})
	}
	for _, m := range reTagClose.FindAllStringSubmatchIndex(clean, -1) {
		events = append(events, ev{idx: m[0], open: false, tag: strings.ToLower(clean[m[2]:m[3]])})
	}
	// Interleave in document order.
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].idx < events[j-1].idx; j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}

	for _, e := range events {
		if !e.open {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].tag == e.tag {
					for k := len(stack) - 1; k >= i; k-- {
						if stack[k].isCard {
							cardDepth--
						}
					}
					stack = stack[:i]
					break
				}
			}
			continue
		}
		if voidHTMLElements[e.tag] || strings.HasSuffix(strings.TrimSpace(e.attrs), "/") {
			continue
		}
		classes := classesOf(e.attrs)
		card := isCardClass(classes)
		if card && pageSectionTags[e.tag] {
			out = append(out, ExperienceContractViolation{
				Rule: RuleNoNestedCard, Surface: surface, Line: lineOf(clean, e.idx),
				Detail: "<" + e.tag + "> is a page landmark and must not be card-styled",
			})
		}
		if card && cardDepth > 0 {
			out = append(out, ExperienceContractViolation{
				Rule: RuleNoNestedCard, Surface: surface, Line: lineOf(clean, e.idx),
				Detail: "card <" + e.tag + "> is nested inside another card; a card is a leaf container",
			})
		}
		if card {
			cardDepth++
		}
		stack = append(stack, htmlFrame{tag: e.tag, isCard: card})
	}
	return out
}

// ---------------------------------------------------------------------------
// Icon-only controls
// ---------------------------------------------------------------------------

var reControlOpen = regexp.MustCompile(`(?s)<(button|a)((?:"[^"]*"|'[^']*'|[^>"'])*)>(.*?)</\s*(?:button|a)\s*>`)

var reAttrPresent = func(attrs, name string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*=\s*("[^"]*[^"\s][^"]*"|'[^']*[^'\s][^']*')`)
	return re.MatchString(attrs)
}

var reStripTags = regexp.MustCompile(`(?s)<[^>]*>`)

// CheckIconOnlyControlContract requires that a control whose only content is an
// icon carries BOTH an accessible name and a hover/focus tooltip.
//
// "Icon-only" is detected as: the control contains an icon element or icon class
// and, after removing markup, has no visible text. Such a control is invisible
// to a screen reader and ambiguous to a sighted user, so design.md requires both
// `aria-label` (or `aria-labelledby`) and `title`.
func CheckIconOnlyControlContract(surface, src string) []ExperienceContractViolation {
	clean := stripHTMLComments(src)
	var out []ExperienceContractViolation
	for _, m := range reControlOpen.FindAllStringSubmatchIndex(clean, -1) {
		tag := clean[m[2]:m[3]]
		attrs := clean[m[4]:m[5]]
		inner := clean[m[6]:m[7]]

		hasIcon := strings.Contains(inner, "<svg") ||
			strings.Contains(inner, "<use") ||
			strings.Contains(inner, `class="icon`) ||
			strings.Contains(inner, `class='icon`)
		if !hasIcon {
			continue
		}
		visible := strings.TrimSpace(reStripTags.ReplaceAllString(inner, ""))
		if visible != "" {
			continue // Icon plus text: labelled by its own text.
		}
		named := reAttrPresent(attrs, "aria-label") || reAttrPresent(attrs, "aria-labelledby")
		tooltip := reAttrPresent(attrs, "title")
		if !named {
			out = append(out, ExperienceContractViolation{
				Rule: RuleIconOnlyControl, Surface: surface, Line: lineOf(clean, m[0]),
				Detail: "icon-only <" + tag + "> has no accessible name (aria-label or aria-labelledby)",
			})
		}
		if !tooltip {
			out = append(out, ExperienceContractViolation{
				Rule: RuleIconOnlyControl, Surface: surface, Line: lineOf(clean, m[0]),
				Detail: "icon-only <" + tag + "> has no hover/focus tooltip (title)",
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Forced colors
// ---------------------------------------------------------------------------

var (
	reForcedColorsBlock  = regexp.MustCompile(`@media\s*\(\s*forced-colors\s*:\s*active\s*\)\s*\{`)
	reForcedColorAdjNone = regexp.MustCompile(`forced-color-adjust\s*:\s*none`)
	reLiteralColor       = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\s*\(|\bhsla?\s*\(`)
)

// CheckForcedColorsContract requires a forced-colors block that leaves the
// platform in control: it must exist (so boundaries and focus survive), it must
// not set `forced-color-adjust: none`, and it must contain no literal color.
func CheckForcedColorsContract(surface, src string) []ExperienceContractViolation {
	clean := stripCSSComments(src)
	loc := reForcedColorsBlock.FindStringIndex(clean)
	if loc == nil {
		return []ExperienceContractViolation{{
			Rule: RuleForcedColors, Surface: surface,
			Detail: "no @media (forced-colors: active) block; boundaries and focus are unspecified in a forced-colors mode",
		}}
	}
	body, endIdx, ok := braceBody(clean, loc[1]-1)
	if !ok {
		return []ExperienceContractViolation{{
			Rule: RuleForcedColors, Surface: surface, Line: lineOf(clean, loc[0]),
			Detail: "@media (forced-colors: active) block is unterminated",
		}}
	}
	_ = endIdx
	var out []ExperienceContractViolation
	if m := reForcedColorAdjNone.FindStringIndex(body); m != nil {
		out = append(out, ExperienceContractViolation{
			Rule: RuleForcedColors, Surface: surface, Line: lineOf(clean, loc[1]+m[0]),
			Detail: "forced-color-adjust:none takes color control away from the platform",
		})
	}
	for _, m := range reLiteralColor.FindAllStringIndex(body, -1) {
		out = append(out, ExperienceContractViolation{
			Rule: RuleForcedColors, Surface: surface, Line: lineOf(clean, loc[1]+m[0]),
			Detail: "literal color " + strings.TrimSpace(body[m[0]:m[1]]) + " inside forced-colors; use a CSS system color keyword so the platform substitutes it",
		})
	}
	return out
}

// braceBody returns the text between the brace at openIdx and its match.
func braceBody(src string, openIdx int) (string, int, bool) {
	if openIdx < 0 || openIdx >= len(src) || src[openIdx] != '{' {
		return "", 0, false
	}
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[openIdx+1 : i], i, true
			}
		}
	}
	return "", 0, false
}

// ---------------------------------------------------------------------------
// Reduced motion
// ---------------------------------------------------------------------------

var (
	reReducedMotionBlock = regexp.MustCompile(`@media\s*\(\s*prefers-reduced-motion\s*:\s*reduce\s*\)\s*\{`)
	reMotionDuration     = regexp.MustCompile(`--motion-duration[a-z-]*\s*:\s*(\d+(?:\.\d+)?)(ms|s)`)
)

// CheckReducedMotionContract requires a reduce block that actually collapses
// every motion-duration token to zero. A block that exists but leaves a
// non-zero duration still animates, which is the failure users report.
func CheckReducedMotionContract(surface, src string) []ExperienceContractViolation {
	clean := stripCSSComments(src)
	loc := reReducedMotionBlock.FindStringIndex(clean)
	if loc == nil {
		return []ExperienceContractViolation{{
			Rule: RuleReducedMotion, Surface: surface,
			Detail: "no @media (prefers-reduced-motion: reduce) block; motion is not user-controlled",
		}}
	}
	body, _, ok := braceBody(clean, loc[1]-1)
	if !ok {
		return []ExperienceContractViolation{{
			Rule: RuleReducedMotion, Surface: surface, Line: lineOf(clean, loc[0]),
			Detail: "@media (prefers-reduced-motion: reduce) block is unterminated",
		}}
	}

	declaredOutside := map[string]bool{}
	for _, m := range reMotionDuration.FindAllStringSubmatchIndex(clean, -1) {
		if m[0] > loc[1] && m[0] < loc[1]+len(body) {
			continue
		}
		name := strings.TrimSpace(clean[m[0] : strings.Index(clean[m[0]:], ":")+m[0]])
		declaredOutside[name] = true
	}

	var out []ExperienceContractViolation
	collapsed := map[string]bool{}
	for _, m := range reMotionDuration.FindAllStringSubmatchIndex(body, -1) {
		name := strings.TrimSpace(body[m[0] : strings.Index(body[m[0]:], ":")+m[0]])
		val, err := strconv.ParseFloat(body[m[2]:m[3]], 64)
		if err != nil {
			continue
		}
		if val != 0 {
			out = append(out, ExperienceContractViolation{
				Rule: RuleReducedMotion, Surface: surface, Line: lineOf(clean, loc[1]+m[0]),
				Detail: fmt.Sprintf("%s stays at %g%s under prefers-reduced-motion; it must collapse to 0", name, val, body[m[4]:m[5]]),
			})
			continue
		}
		collapsed[name] = true
	}
	for name := range declaredOutside {
		if !collapsed[name] {
			out = append(out, ExperienceContractViolation{
				Rule: RuleReducedMotion, Surface: surface,
				Detail: name + " is declared but never collapsed under prefers-reduced-motion",
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Contrast
// ---------------------------------------------------------------------------

// wcagTextContrast is the WCAG 2.2 AA ratio for normal body text (1.4.3).
const wcagTextContrast = 4.5

// wcagNonTextContrast is the WCAG 2.2 AA ratio for UI components and focus
// indicators (1.4.11).
const wcagNonTextContrast = 3.0

// contrastPair is one foreground role that must be legible on one background role.
type contrastPair struct {
	fg, bg string
	min    float64
}

// requiredContrastPairs is the closed set of role pairings the product actually
// renders. Decorative separators are deliberately absent: WCAG does not require
// 3:1 of a hairline divider, and asserting it would force the design to draw
// heavy borders to satisfy a checker rather than a user.
var requiredContrastPairs = []contrastPair{
	{"--color-fg", "--color-bg", wcagTextContrast},
	{"--color-fg", "--color-surface", wcagTextContrast},
	{"--color-fg", "--color-surface-raised", wcagTextContrast},
	{"--color-fg-muted", "--color-bg", wcagTextContrast},
	{"--color-fg-muted", "--color-surface", wcagTextContrast},
	{"--color-accent-fg", "--color-accent", wcagTextContrast},
	{"--color-success", "--color-surface", wcagTextContrast},
	{"--color-warning", "--color-surface", wcagTextContrast},
	{"--color-danger", "--color-surface", wcagTextContrast},
	{"--color-info", "--color-surface", wcagTextContrast},
	{"--color-focus", "--color-bg", wcagNonTextContrast},
	{"--color-focus", "--color-surface", wcagNonTextContrast},
}

var reHexDecl = regexp.MustCompile(`(--color-[a-z-]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;`)

// themePalette is the resolved color roles for one appearance theme.
type themePalette struct {
	name  string
	roles map[string]string
}

// parseThemePalettes resolves the light palette (the `:root` base) and the dark
// palette (base overlaid with the `[data-theme="dark"]` overrides).
//
// Overlay rather than independent parse is the point: a dark block that forgets
// to override one role inherits the light value, and that inherited value is
// what the user sees. Parsing the dark block alone would silently skip exactly
// the role most likely to be wrong.
func parseThemePalettes(src string) (light, dark themePalette) {
	clean := stripCSSComments(src)
	light = themePalette{name: "light", roles: map[string]string{}}
	dark = themePalette{name: "dark", roles: map[string]string{}}

	rootIdx := strings.Index(clean, ":root {")
	if rootIdx < 0 {
		rootIdx = strings.Index(clean, ":root{")
	}
	if rootIdx >= 0 {
		if body, _, ok := braceBody(clean, strings.Index(clean[rootIdx:], "{")+rootIdx); ok {
			for _, m := range reHexDecl.FindAllStringSubmatch(body, -1) {
				light.roles[m[1]] = strings.ToLower(m[2])
			}
		}
	}
	for k, v := range light.roles {
		dark.roles[k] = v
	}

	darkSel := regexp.MustCompile(`:root\[data-theme="dark"\]\s*\{`)
	for _, loc := range darkSel.FindAllStringIndex(clean, -1) {
		body, _, ok := braceBody(clean, loc[1]-1)
		if !ok {
			continue
		}
		for _, m := range reHexDecl.FindAllStringSubmatch(body, -1) {
			dark.roles[m[1]] = strings.ToLower(m[2])
		}
	}
	return light, dark
}

func srgbChannel(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// relativeLuminance implements WCAG 2.x relative luminance for an #rrggbb color.
func relativeLuminance(hex string) (float64, bool) {
	h := strings.TrimPrefix(strings.ToLower(hex), "#")
	if len(h) != 6 {
		return 0, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, false
	}
	r := srgbChannel(float64((v>>16)&0xff) / 255)
	g := srgbChannel(float64((v>>8)&0xff) / 255)
	b := srgbChannel(float64(v&0xff) / 255)
	return 0.2126*r + 0.7152*g + 0.0722*b, true
}

// ContrastRatio returns the WCAG contrast ratio between two #rrggbb colors.
func ContrastRatio(fg, bg string) (float64, bool) {
	lf, ok1 := relativeLuminance(fg)
	lb, ok2 := relativeLuminance(bg)
	if !ok1 || !ok2 {
		return 0, false
	}
	hi, lo := lf, lb
	if lo > hi {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05), true
}

// CheckContrastContract computes real WCAG ratios for every required role pair
// in both the light and dark palettes.
func CheckContrastContract(surface, src string) []ExperienceContractViolation {
	light, dark := parseThemePalettes(src)
	var out []ExperienceContractViolation
	for _, palette := range []themePalette{light, dark} {
		if len(palette.roles) == 0 {
			out = append(out, ExperienceContractViolation{
				Rule: RuleContrast, Surface: surface,
				Detail: "no color roles resolved for the " + palette.name + " theme",
			})
			continue
		}
		for _, pair := range requiredContrastPairs {
			fg, okFg := palette.roles[pair.fg]
			bg, okBg := palette.roles[pair.bg]
			if !okFg || !okBg {
				out = append(out, ExperienceContractViolation{
					Rule: RuleContrast, Surface: surface,
					Detail: fmt.Sprintf("%s theme is missing %s or %s", palette.name, pair.fg, pair.bg),
				})
				continue
			}
			ratio, ok := ContrastRatio(fg, bg)
			if !ok {
				out = append(out, ExperienceContractViolation{
					Rule: RuleContrast, Surface: surface,
					Detail: fmt.Sprintf("%s theme has an unparseable color for %s on %s", palette.name, pair.fg, pair.bg),
				})
				continue
			}
			if ratio < pair.min {
				out = append(out, ExperienceContractViolation{
					Rule: RuleContrast, Surface: surface,
					Detail: fmt.Sprintf("%s theme: %s (%s) on %s (%s) is %.2f:1, below the required %.1f:1",
						palette.name, pair.fg, fg, pair.bg, bg, ratio, pair.min),
				})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Aggregate
// ---------------------------------------------------------------------------

// ExperienceTokenSourcePath is the embed path of the single semantic token source.
const ExperienceTokenSourcePath = "experience-tokens.css"

// CheckExperienceContracts runs every mechanical rule against the real embedded
// shared surfaces: the token source for the CSS-expressed contracts, and every
// embedded PWA document for the markup-expressed contracts.
//
// It returns an error only when a required surface is absent, which is itself a
// failure: a contract with no surface to check is not satisfied, it is unmeasured.
func CheckExperienceContracts() ([]ExperienceContractViolation, error) {
	tokens, err := fs.ReadFile(pwa.StaticFiles, ExperienceTokenSourcePath)
	if err != nil {
		return nil, fmt.Errorf("experience contracts: token source %q is not embedded: %w", ExperienceTokenSourcePath, err)
	}
	tokenSrc := string(tokens)

	var out []ExperienceContractViolation
	out = append(out, CheckTypographyContract(ExperienceTokenSourcePath, tokenSrc)...)
	out = append(out, CheckStableDimensionContract(ExperienceTokenSourcePath, tokenSrc)...)
	out = append(out, CheckControlTargetContract(ExperienceTokenSourcePath, tokenSrc)...)
	out = append(out, CheckForcedColorsContract(ExperienceTokenSourcePath, tokenSrc)...)
	out = append(out, CheckReducedMotionContract(ExperienceTokenSourcePath, tokenSrc)...)
	out = append(out, CheckContrastContract(ExperienceTokenSourcePath, tokenSrc)...)
	out = append(out, CheckNoOverlapContract(ExperienceTokenSourcePath, tokenSrc)...)

	docs, err := fs.Glob(pwa.StaticFiles, "*.html")
	if err != nil {
		return nil, fmt.Errorf("experience contracts: cannot enumerate embedded documents: %w", err)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("experience contracts: no embedded PWA documents to check")
	}
	for _, doc := range docs {
		data, readErr := fs.ReadFile(pwa.StaticFiles, doc)
		if readErr != nil {
			return nil, fmt.Errorf("experience contracts: cannot read %q: %w", doc, readErr)
		}
		src := string(data)
		out = append(out, CheckNoNestedCardContract(doc, src)...)
		out = append(out, CheckIconOnlyControlContract(doc, src)...)
	}
	return out, nil
}
