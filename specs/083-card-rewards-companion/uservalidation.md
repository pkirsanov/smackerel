# User Validation Checklist

> Items are checked `[x]` by default when created (already validated via the
> planning effort). The owner unchecks `[ ]` an item to report that the planned
> behavior is wrong or missing — an unchecked item is a blocking, owner-reported
> planning gap that `bubbles.validate` / the next planning pass must resolve.

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Build-vs-absorb decision recorded as authoritative seed context (Option 1: absorb CCManager into smackerel; not re-litigated)
- [x] Feature is placed under docs/smackerel.md §16.8 Financial Awareness (Light Touch), recommendation-only
- [x] §1.6 / Principle 10 QF Companion Boundary confirmed NOT crossed (no trades, no investment advice, no account integration)
- [x] Regex scraper replacement by strict-schema LLM extraction is planned (no silent fallback; needs_verification on failure/low-confidence)
- [x] Multiple data sources + reconciliation with confidence signal planned
- [x] Go rewrite planned (Constitution C2); Python app + Render hosting retired
- [x] Hosting on <home-lab-host> via smackerel's EXISTING home-lab deploy adapter (no new knb product adapter)
- [x] All data planned in PostgreSQL (one-graph / PostgreSQL-only); JSON→PG one-time migration is a dedicated scope
- [x] Google Calendar (CalDAV) preserved as the primary delivery surface (reusing the internal/mealplan CalendarBridge pattern)
- [x] Full Web UI parity with CCManager's ~22 screens is a first-class deliverable across two dedicated e2e-ui scopes
- [x] Config flows through config/smackerel.yaml SST, fail-loud, no defaults (Gate G028 / smackerel-no-defaults)
- [x] Release train set to mvp; card_rewards flag declared (bundle edit deferred to delivery, owned by bubbles.train)
- [x] Open questions for the owner are captured in spec.md (sources, Ollama model, CalDAV target, PWA scope, card art, bonus spend entry)
