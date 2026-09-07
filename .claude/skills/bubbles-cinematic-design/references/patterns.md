# Cinematic Design — Interaction-Pattern Library

> Reference for the `bubbles-cinematic-design` skill. Open when building the pages a
> feature's `spec.md` requests. Adapt each pattern to the project's component
> architecture and build it with the repo's **shared components + design tokens**
> (see the skill's Composition rule). Never hardcode colors/spacing — use tokens.

## A. The Floating Island (Navbar)
A fixed pill-shaped container, horizontally centered.
- **Morphing logic:** transparent with light text at the top of the page; transitions to a blurred, semi-transparent background with primary-colored text and a subtle border once scrolled past the hero.
- Contains: logo (brand name as text), navigation links (via the project's router), CTA button (accent token).

## B. The Opening Shot (Hero Section)
- Full viewport height. Full-bleed background image (matching the preset's `imageMood`) with a heavy **primary-to-black gradient overlay**.
- **Layout:** content pushed to the **bottom-left third**.
- **Typography:** large scale contrast following the preset's hero line pattern — first part in the bold sans heading token, second part in the massive serif-italic drama token (3–5× size difference).
- **Animation:** staggered fade-up for all text parts and the CTA.

## C. Interactive Functional Artifacts (Features)
Cards derived from value propositions. These must feel like **functional software micro-UIs**, not static marketing cards.
- **Diagnostic Shuffler:** 3 overlapping cards that cycle vertically with a spring-bounce transition.
- **Telemetry Typewriter:** a monospace live-text feed typed character-by-character with a blinking accent-colored cursor.
- **Cursor Protocol Scheduler:** a weekly grid where an animated SVG cursor enters, moves to a day cell, clicks, activates the day, then moves to a "Save" button before fading out.

## D. The Manifesto (Philosophy / About)
- Full-width section with the **dark token** as background.
- A parallaxing organic-texture image at low opacity behind the text.
- **Typography:** two contrasting statements — "Most [industry] focuses on: [common approach]." (neutral, smaller) vs "We focus on: [differentiated approach]." (massive, drama serif italic, accent-colored keyword).
- **Animation:** scroll-triggered word-by-word or line-by-line reveal.

## E. Sticky Stacking Archive (Protocol / How it Works)
Full-screen cards that stack on scroll.
- **Stacking interaction:** scroll-linked (e.g. ScrollTrigger pin). As a new card scrolls in, the card underneath scales down slightly, blurs, and fades.
- **Visuals:** each card gets a unique canvas/SVG animation (rotating geometric motif, scanning laser-line, pulsing waveform).

## F. Membership / Pricing
- Three-tier pricing grid.
- **Middle card pops:** primary-token background with an accent CTA; slightly larger scale or highlighted border.

## G. The Terminal Footer
- Deep dark-token background, large top border radius.
- Grid layout: brand name + tagline, navigation columns, legal links.
- **"System Operational" status indicator** with a pulsing green dot and monospace label.

---

## Technical Requirements & Routing
- **Tech-agnostic:** use the framework, styling engine, and routing mechanism the host repo specifies. Do NOT assume React or Tailwind unless the project config says so.
- **Routing:** wire all navbar and footer links using the project's native routing.
- **No placeholders:** every card, label, and animation must be fully implemented and functional; use real image URLs matching the `imageMood`.
- **Responsive:** mobile-first. Stack cards vertically on mobile, reduce hero font sizes, collapse the navbar into a minimal version.

## Execution Directive
"Do not build a website; build a digital instrument. Every scroll should feel intentional, every animation weighted and professional. Eradicate all generic AI patterns. Prove the work by passing the repository's quality gates." — applied through the project's design system, never around it.
