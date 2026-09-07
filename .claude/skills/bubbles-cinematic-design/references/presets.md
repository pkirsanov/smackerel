# Cinematic Design — Aesthetic Presets & Fixed Design System

> Reference for the `bubbles-cinematic-design` skill. Open when applying a cinematic
> design language to a UI surface. **Express every concrete value below through the
> host repo's design tokens** (see the skill's Composition rule) — these are the
> design vocabulary, not literal CSS to hardcode.

Each preset defines `palette`, `typography`, `identity` (overall feel), and
`imageMood` (image search keywords for hero/texture imagery). Apply via the
project's styling engine (e.g. its Tailwind/theme-token config), never inline hex.

## Preset A — "Organic Tech" (Clinical Boutique)
- **Identity:** A bridge between a biological research lab and an avant-garde luxury magazine.
- **Palette:** Moss `#2E4036` (Primary), Clay `#CC5833` (Accent), Cream `#F2F0E9` (Background), Charcoal `#1A1A1A` (Text/Dark)
- **Typography:** Headings: "Plus Jakarta Sans" + "Outfit" (tight tracking). Drama: "Cormorant Garamond" Italic. Data: `"IBM Plex Mono"`.
- **Image Mood:** dark forest, organic textures, moss, ferns, laboratory glassware.
- **Hero line pattern:** "[Concept noun] is the" (Bold Sans) / "[Power word]." (Massive Serif Italic)

## Preset B — "Midnight Luxe" (Dark Editorial)
- **Identity:** A private members' club meets a high-end watchmaker's atelier.
- **Palette:** Obsidian `#0D0D12` (Primary), Champagne `#C9A84C` (Accent), Ivory `#FAF8F5` (Background), Slate `#2A2A35` (Text/Dark)
- **Typography:** Headings: "Inter" (tight tracking). Drama: "Playfair Display" Italic. Data: `"JetBrains Mono"`.
- **Image Mood:** dark marble, gold accents, architectural shadows, luxury interiors.
- **Hero line pattern:** "[Aspirational noun] meets" (Bold Sans) / "[Precision word]." (Massive Serif Italic)

## Preset C — "Brutalist Signal" (Raw Precision)
- **Identity:** A control room for the future — no decoration, pure information density.
- **Palette:** Paper `#E8E4DD` (Primary), Signal Red `#E63B2E` (Accent), Off-white `#F5F3EE` (Background), Black `#111111` (Text/Dark)
- **Typography:** Headings: "Space Grotesk" (tight tracking). Drama: "DM Serif Display" Italic. Data: `"Space Mono"`.
- **Image Mood:** concrete, brutalist architecture, raw materials, industrial.
- **Hero line pattern:** "[Direct verb] the" (Bold Sans) / "[System noun]." (Massive Serif Italic)

## Preset D — "Vapor Clinic" (Neon Biotech)
- **Identity:** A genome sequencing lab inside a Tokyo nightclub.
- **Palette:** Deep Void `#0A0A14` (Primary), Plasma `#7B61FF` (Accent), Ghost `#F0EFF4` (Background), Graphite `#18181B` (Text/Dark)
- **Typography:** Headings: "Sora" (tight tracking). Drama: "Instrument Serif" Italic. Data: `"Fira Code"`.
- **Image Mood:** bioluminescence, dark water, neon reflections, microscopy.
- **Hero line pattern:** "[Tech noun] beyond" (Bold Sans) / "[Boundary word]." (Massive Serif Italic)

---

## Fixed Design System (applies to ALL presets and ALL pages)

These rules are what make the output premium. Express them through the project's
tokens and animation library — never bypass the repo's design system.

### Visual Texture
- Global CSS noise overlay using an inline SVG `<feTurbulence>` filter at **0.05 opacity** to eliminate flat digital gradients.
- Large border-radius system (e.g. `2rem`–`3rem`) for all major containers via the repo's radius tokens. No sharp corners on major surfaces.

### Micro-Interactions
- Buttons have a **"magnetic" feel**: subtle scale up (e.g. `1.03`) on hover with a spring / custom cubic-bezier easing.
- Buttons use overflow hiding with a sliding background layer for color transitions on hover.
- Links and interactive elements get a subtle lift (e.g. `translateY(-1px)`) on hover.

### Animation Lifecycle
- Use the project's designated animation library (e.g. GSAP, Framer Motion) — do not introduce a new one without explicit request.
- Ensure proper cleanup of animations on component unmount to prevent memory leaks.
- Default easing: smooth-out for entrances, in-out for morphs.
- Stagger: tight for text (e.g. `0.08s`), looser for cards/containers (e.g. `0.15s`).
