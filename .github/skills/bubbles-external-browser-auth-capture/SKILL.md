---
name: bubbles-external-browser-auth-capture
description: Drive a REAL EXTERNAL browser via the configured browser-automation MCP tools and hand the login step to the HUMAN when you must read or capture web content that needs interactive human authentication, cookie/consent gating, SSO/OAuth login, or is JS/bot-gated so a headless fetch fails. Human-in-the-loop — the agent pauses for the user to sign in directly in the browser window, then resumes and extracts via the accessibility snapshot or DOM. Use when asked to "read a video transcript", "log into a site and read a page", "capture content behind an interactive login", or "drive an external browser and let the user sign in". Negative trigger: do NOT use the internal VS Code Simple Browser / webview for auth-gated capture.
---

# External Browser Auth Capture (Human-in-the-Loop)

## Core Principle
When you must read web content behind **interactive human auth** (SSO/OAuth, cookie/consent walls, bot checks) **or** content a headless fetch cannot retrieve (client-rendered, gated), drive a **real external browser** via the configured browser-automation MCP tools and hand the **login step to the human**. This is a human-in-the-loop technique: the agent opens and steers the browser, then **pauses and waits for the user to sign in**, then resumes and extracts. The agent never handles the user's credentials.

## Why NOT the internal VS Code browser
Do not use the internal VS Code browser (Simple Browser / `simpleBrowser.show` / any webview) for auth-gated capture:
- The agent has **no automation handle** into the VS Code webview — it cannot click, snapshot the accessibility tree, or extract the DOM.
- OAuth/SSO popups and third-party cookies frequently **break inside the embedded webview**.
- There is **no persistent real user profile**, so a login does not stick and bot detection is more aggressive.
- A plain `fetch_webpage` / headless fetch **fails on login-gated or client-rendered pages** (e.g. the YouTube transcript panel is rendered client-side and is often gated).

## Why an external automated browser
- A **headed, real Chromium/Chrome window** the human can see and interact with.
- A **persistent user-data-dir** — the human logs in **once** and the session survives subsequent navigations.
- The agent steers it with the configured browser tool family: navigate, accessibility snapshot, click, DOM evaluation, wait, and close. Refer to the tool family **generically** because the MCP server alias/prefix varies per environment.

## Triage first (decision tree)
```
Does the target need interactive auth OR is it JS/bot-gated?
├── No  → a plain fetch_webpage on the public/static page works
│         → use THAT (cheaper, no browser). STOP.
└── Yes → escalate to the external automated browser (below).
```

## Workflow
1. **Triage.** If `fetch_webpage` on a public/static page returns the content, use it — do not open a browser. Only escalate when the target needs interactive auth or is JS/bot-gated.
2. **Open the external browser** and `browser_navigate` to the target. Confirm it is the **real external window**, NOT the internal VS Code browser.
3. **Dismiss consent/cookie dialogs** — `browser_snapshot` to find the control, then `browser_click`.
4. **Hand off to the human for login.** Tell the user plainly, e.g. *"A browser window has opened — please sign in to your account in that window, then tell me when you're done."* Pause with a user-question/wait tool (e.g. `vscode_askQuestions`, or a wait) until the user confirms. **NEVER** ask the user to paste credentials into chat; **NEVER** type the user's password yourself. The human authenticates **directly in the browser**.
5. **Resume** after the user confirms, then navigate to the specific content.
6. **Extract:**
   - **YouTube transcript (primary worked example):** open the video, then open the transcript panel — expand the description and click the **"Show transcript"** control, or use the **"⋯" (more actions) menu → "Show transcript"**. Then collect the segments with `browser_evaluate`:
     ```js
     () => Array.from(document.querySelectorAll('ytd-transcript-segment-renderer')).map(s => s.innerText.replace(/\s+/g,' ').trim()).join('\n')
     ```
     Segments carry **timestamps + text** — capture both if the caller needs timing.
   - **General pages:** prefer `browser_snapshot` (accessibility tree) for structured reading; use `browser_evaluate` for specific DOM text.
7. **Persist to a file (if needed)** with IDE file tools — **never** shell redirection (terminal-discipline).
8. **Close** with `browser_close` when done — unless the human wants the session kept for follow-ups.

## Rules / non-negotiables
- **NEVER** use the internal VS Code Simple Browser / webview for auth-gated capture — always the configured external automated browser.
- **NEVER** request, type, echo, or handle the user's credentials/secrets — the human authenticates in the browser (mirrors terminal-discipline's "never ask the user to paste secrets").
- Only capture content the **authenticated human is entitled to access**; respect site ToS/robots; do not defeat paywalls the user has no right to; do not scrape at abusive rates.
- Prefer the accessibility `browser_snapshot` over screenshots for **reading** content.
- This is human-in-the-loop: the agent **waits** for the human login and does **not fabricate** having read content it could not actually access (anti-fabrication).

## When NOT to use
- **Public / static / un-gated pages** → use `fetch_webpage` (no browser needed).
- **Content available via an official API/export** the user already has → prefer the API.
- **Any path that would require the user to paste secrets into chat** → never; use in-browser login instead.
- **Previewing a localhost dev page** you only need to LOOK at (not extract from, not auth) → the internal Simple Browser is fine there; this skill is specifically for **automated extraction behind interactive auth**.

## Works well with
- `bubbles-long-running-commands` — how to wait without polling while the human logs in.
- `bubbles-evidence-capture` — record what was actually extracted as evidence.
- terminal-discipline instruction — file writes via IDE tools; never echo/handle secrets.
