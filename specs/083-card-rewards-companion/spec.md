# Feature: 083 Card Rewards Companion

**Status:** Specs Hardened (planning-only; ceiling per state.json `product-to-planning`)

> **Planning-only notice.** This feature folder was produced by the
> `product-to-planning` workflow (analyst → ux → design → plan → harden).
> It stops at the `specs_hardened` ceiling. NO implementation code, NO
> migration, NO config edits, and NO feature-flag bundle edits have been
> made. Delivery is a separate, later workflow.

> **Absorption notice.** This feature folds the standalone **CCManager**
> app (`~/CCManager` — single-user Python/Flask credit-card
> rotating-category tracker on Render free tier) INTO smackerel as a
> native in-product feature: a **credit-card rewards companion** ("card
> rewards"). The standalone Python app and its Render hosting are retired
> once this feature ships. The build-vs-absorb decision (Option 1: absorb)
> and the product-principle clearance were made by the owner before this
> planning effort and are treated as authoritative seed context here.

## Problem Statement

The owner runs a separate Python/Flask app (CCManager) to manage credit-card
cashback optimization: which card earns the most in which spend category this
quarter, when rotating 5% categories change, which selectable-category cards
need quarterly re-enrollment, and what monthly recommendation events to push
to Google Calendar. That app:

1. **Scrapes "Doctor of Credit" pages with brittle regex** (`CCManager/scripts/scraper.py`)
   to extract quarterly 5% categories for Discover / Chase Freedom. When the
   regex misses, it **silently falls back** to last-known JSON or a hardcoded
   "check the website" placeholder — the user is never told the data is stale
   or unverified. There is **zero LLM usage** today.
2. Resolves user card names to a ~21-card database (`CCManager/scripts/card_resolver.py`,
   `CCManager/data/cards-database.json`).
3. Runs an optimizer (`CCManager/scripts/optimizer.py`) mapping spend categories → best card.
4. Writes monthly recommendation events to Google Calendar (`CCManager/scripts/calendar_sync.py`).
5. Exposes a rich Flask CRUD web UI (`CCManager/web/app.py` + ~22 Jinja templates
   under `CCManager/web/templates/`).

Its state is flat JSON files in `CCManager/data/` (`cards-database.json`,
`user-cards.json`, `user-offers.json`, `user-selections.json`,
`rotating-categories.json`, `config.json`, `run-history.json`,
`latest-report.json`, `usage-stats.json`, `monthly-recommendations/`).

This is a second app to host, secure, back up, and maintain — on a free tier
that is being retired. Worse, the regex scraper is the exact kind of brittle,
silently-degrading heuristic smackerel was built to replace with
schema-bound LLM extraction (`docs/smackerel.md` §17.2). Meanwhile
smackerel already owns every primitive this feature needs: a connector
framework, a local LLM gateway (host Ollama on evo-x2), a scheduler, a
CalDAV delivery bridge, PostgreSQL + pgvector storage, and a server-rendered
Web UI. Credit-card cashback awareness is explicitly welcomed by
`docs/smackerel.md` §16.8 "Financial Awareness (Light Touch)".

## Outcome Contract

**Intent:** A single-user credit-card rewards companion lives inside
smackerel. It maintains the user's card wallet, offers, selectable-category
choices, and sign-up bonuses; refreshes rotating 5% categories **via
schema-bound LLM extraction from one or more sources** (replacing the regex
scraper); reconciles multiple sources into a confidence-scored, lifecycle-aware
rotating-category record; runs an optimizer that maps spend categories → best
card; generates monthly recommendations; and pushes those recommendations and
quarterly re-enrollment reminders to **Google Calendar via CalDAV** (the
primary consumption surface, preserved from CCManager). A full server-rendered
**Web UI** provides parity with CCManager for entering, browsing, and managing
all of it. All data lives in PostgreSQL; the standalone app and its JSON files
are migrated once and then retired.

**Success Signal:** On evo-x2 home-lab, the daily refresh job fetches the
configured sources, the LLM extractor returns a schema-valid rotating-category
record for `discover-it` Q3 2026 with a confidence score and source citation,
the reconciler upserts it with `lifecycle_state = upcoming`, and the user sees
it on the Card Rewards dashboard. On the 1st of the month the optimizer
produces "Use Discover it for Restaurants this quarter (5%)" and a CalDAV event
appears on the user's Google Calendar. When a source page changes shape and the
LLM cannot extract a confident result, the record is flagged
`needs_verification = true` and surfaced on the dashboard for manual
confirmation — it is **never** silently replaced by a stale value or a
placeholder.

**Hard Constraints:**
- **Recommendation-only, light-touch.** The feature recommends which card to
  use for a spend category. It NEVER initiates a transaction, applies for a
  card, or gives investment advice. (`docs/smackerel.md` §16.8; does not cross
  the §1.6 / Principle 10 QF boundary — see Product Principle Alignment.)
- **LLM extraction replaces regex.** Rotating-category extraction MUST use a
  strict-JSON-schema LLM call (validated before storage, `docs/smackerel.md`
  §17.2). Malformed or low-confidence output MUST flag `needs_verification`,
  NOT silently fall back to stale data or a placeholder.
- **Source-qualified.** Every extracted category MUST retain its source
  (URL + source name + issuer hint) and extraction provenance (Principle 4).
- **One graph, one store.** All card data lives in PostgreSQL (Principle 5 /
  PostgreSQL-only governance). No parallel JSON store, no SQLite, no embedded DB.
- **Config SST, fail-loud.** All tunables (sources, cron schedules, Ollama
  model/endpoint, confidence threshold, CalDAV target) flow through
  `config/smackerel.yaml`. When the feature is enabled, missing required config
  is a fatal startup error — no in-source defaults (Gate G028 /
  `smackerel-no-defaults`).
- **Go-first; the model call lives in the Python sidecar (C2).** Runtime code
  is Go EXCEPT the LLM model-gateway call, which lives in the Python ML sidecar
  (`ml/app/`) per Constitution C2 ("Python is reserved for ... model gateway
  work"), consistent with `drive_classify.py` / `intelligence.py`. The Go side
  orchestrates and schema-validates the sidecar response; NO new Go→Ollama
  client is introduced. The Python app is not ported line-by-line; its behavior
  is re-expressed in idiomatic Go plus one sidecar extraction route.
- **Calendar primary, digest secondary.** Google Calendar (CalDAV) is the
  primary delivery surface (preserved from CCManager). Telegram/digest delivery
  is an additive bonus, not a replacement.
- **Web UI is first-class.** A server-rendered Web UI (smackerel
  `internal/web` Go-template paradigm) must provide full CRUD parity with
  CCManager's ~22 screens. No functionality may be lost.
- **One-time migration.** CCManager's JSON files migrate once into PostgreSQL,
  idempotently; after that the JSON files are read-only history.

**Failure Condition:** If the rotating-category refresh silently serves stale
or placeholder data when extraction fails (reproducing the CCManager regex
failure mode), the feature has failed its core modernization goal. If the user
must keep CCManager running to enter/browse/manage cards because the smackerel
Web UI lacks parity, absorption has failed. If card data ends up in a second
store outside PostgreSQL, the one-graph constraint is violated.

## Goals

- G1: Maintain the card catalog, user wallet, offers, selectable-category
  selections, and sign-up bonuses in PostgreSQL with full CRUD.
- G2: Replace the regex scraper with schema-bound LLM rotating-category
  extraction, with confidence + `needs_verification` signaling and manual
  override.
- G3: Reconcile multiple sources into one lifecycle-aware rotating-category
  record (upcoming → active → expired) per Principle 3.
- G4: Run an optimizer that maps spend categories → best card given base +
  rotating + offers + selections + limits + bonuses, and generate monthly
  recommendations.
- G5: Deliver monthly recommendations and quarterly re-enrollment reminders to
  Google Calendar via CalDAV (reusing the meal-plan CalendarBridge pattern).
- G6: Provide a server-rendered Web UI with full parity to CCManager's screens.
- G7: Migrate CCManager's JSON state into PostgreSQL once, idempotently.
- G8: Log every refresh, extraction, optimization, and calendar sync to a run
  history / audit surface (Principle 8 transparency).

## Non-Goals

- Automated transactions, card applications, balance/statement integration, or
  any bank-account connection.
- Investment advice or anything that crosses the QF companion boundary
  (Principle 10 / §1.6).
- Real-time spend tracking from card feeds (the optimizer works from stored
  card/category/offer data, not live transactions).
- Multi-user / shared wallets (smackerel is single-user; §1.5).
- Porting CCManager's Python code line-by-line or preserving its JSON file
  layout at runtime.
- Building a new knb deploy adapter — this ships via smackerel's EXISTING
  `deploy/` home-lab adapter (see Offline / Host note).
- Image scraping for card art (`CCManager/scripts/card_image_scraper.py`) — out
  of scope for v1; optional future enhancement.

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| User (Wallet Owner) | The single smackerel user managing their cards | Add/edit cards, offers, selections, bonuses; verify extracted categories; read recommendations | Full CRUD on all card-rewards data |
| User (Spender) | Same person, day-to-day | "Which card for groceries this quarter?"; see this month's recommendations on the dashboard and on Google Calendar | Read recommendations, trigger optimize/sync |
| System (Source Connector) | Scheduled fetcher of rotating-category source pages | Fetch configured sources, emit source-attributed raw observations | Read external source URLs (read-only), write raw artifacts |
| System (LLM Extractor) | Schema-bound category extractor | Convert a raw source observation into a validated rotating-category record with confidence | Call host Ollama, write observations |
| System (Reconciler) | Multi-source merge + lifecycle engine | Merge per-source observations into one record; advance lifecycle by date | Read observations, write rotating_categories |
| System (Optimizer) | Best-card recommendation engine | Map spend categories → best card; generate monthly recommendations | Read card data, write recommendations |
| System (Calendar Bridge) | CalDAV event writer | Push monthly recommendations + re-enrollment reminders to Google Calendar | Read recommendations, write CalDAV |
| Operator | Person deploying/operating smackerel on evo-x2 | Configure sources/model/CalDAV; trigger manual scrape/sync; read run history | Config + admin triggers |

---

## Use Cases

### UC-001: Manage the Card Wallet

- **Actor:** User (Wallet Owner)
- **Preconditions:** Card catalog seeded (from migration or discovery)
- **Main Flow:**
  1. User opens the Web UI "My Cards" page.
  2. User adds a card by short description ("citi custom cash") → discovery
     resolves it against the catalog → user confirms.
  3. User can add a custom card not in the catalog, edit a card, add a per-card
     note, toggle activation, or remove a card.
  4. Changes persist to PostgreSQL and are reflected on the dashboard.
- **Alternative Flows:**
  - A1: Description matches multiple catalog cards → UI shows candidates to
    disambiguate.
  - A2: Description matches nothing → UI offers "add custom card" flow.
  - A3: Removing a card cascades to its offers/selections/bonuses (with
    confirmation).
- **Postconditions:** Wallet reflects the user's real cards.

### UC-002: Refresh Rotating Categories via LLM Extraction

- **Actor:** System (Source Connector + LLM Extractor + Reconciler)
- **Preconditions:** Feature enabled; sources, Ollama model, and confidence
  threshold configured.
- **Main Flow:**
  1. The daily scheduler job triggers the card-rewards connector.
  2. The connector fetches each configured source URL and emits one
     source-attributed raw observation per (source, card hint).
  3. For each raw observation, the LLM extractor calls host Ollama with a
     **strict JSON schema** and validates the response before storage.
  4. Valid extractions are stored as per-source observations with a confidence
     score and verbatim source evidence.
  5. The reconciler merges observations for the same (card, period) into one
     `rotating_categories` record, sets `lifecycle_state`, and computes an
     aggregate confidence.
  6. The run is logged to run history with sources attempted/succeeded and
     categories extracted.
- **Alternative Flows:**
  - A1: LLM returns malformed JSON → response is logged and discarded; the
    observation is NOT stored; the run is marked `partial`; the existing
    record is preserved and flagged `needs_verification` (NOT silently
    overwritten with stale or placeholder data).
  - A2: Confidence below the configured threshold → record stored but flagged
    `needs_verification = true` and surfaced on the dashboard.
  - A3: Two sources disagree on categories for the same (card, period) →
    reconciled record is flagged `needs_verification = true` with the lower
    confidence; both observations are retained for audit.
  - A4: A source page references a card not in the catalog → that observation
    is skipped with an audit note; it is NOT mismapped to a known card.
  - A5: A record carries a manual override → extraction NEVER overwrites it;
    the new observation is recorded for audit only.
- **Postconditions:** Rotating-category records are current OR explicitly
  flagged for verification — never silently stale.

### UC-003: Manage Selectable-Category Selections and Re-Enrollment

- **Actor:** User (Wallet Owner)
- **Preconditions:** User holds a selectable-category card (e.g., Citi Custom
  Cash, US Bank Cash+, BofA).
- **Main Flow:**
  1. User opens "Selections", picks the active category(ies) for each
     selectable card and the enrollment period.
  2. For tiered cards (US Bank Cash+), the user sets per-tier categories.
  3. The system tracks enrollment windows and, when a quarterly re-enrollment
     is due, raises a pending action on the dashboard and a calendar reminder.
- **Alternative Flows:**
  - A1: Selection exceeds the card's `num_categories` → UI rejects with a clear
    message.
  - A2: Re-enrollment window opens → dashboard shows "Re-enroll your Cash+
    categories for Q3" and a CalDAV reminder is created.
- **Postconditions:** Selections are current; re-enrollment is never missed
  silently.

### UC-004: Manage Offers and Sign-up Bonuses

- **Actor:** User (Wallet Owner)
- **Main Flow:**
  1. User adds a promotional offer (category, rate, limit, shared-limit group,
     date window, activation flag) and can edit/remove/toggle it.
  2. User adds a sign-up bonus (spend bonus with required spend + deadline, or
     first-year rate bonus) and tracks progress.
  3. Offers and bonuses feed the optimizer.
- **Alternative Flows:**
  - A1: Offer is in a shared/combined limit group → optimizer treats the group
    limit as one pool.
  - A2: Bonus deadline approaches with spend incomplete → dashboard surfaces a
    pending action.
- **Postconditions:** Offers and bonuses are reflected in optimization.

### UC-005: Generate Monthly Recommendations and Sync to Calendar

- **Actor:** System (Optimizer + Calendar Bridge)
- **Preconditions:** Wallet, active rotating categories, offers, and selections
  exist; CalDAV configured.
- **Main Flow:**
  1. On the configured monthly cron, the optimizer maps each tracked spend
     category to the best card (base + rotating + offers + selections + limits +
     bonus context) and writes `card_recommendations` for the period.
  2. The Calendar Bridge creates/updates one CalDAV event per recommendation
     (and per due re-enrollment) on the user's Google Calendar.
  3. The run is logged with events written.
- **Alternative Flows:**
  - A1: CalDAV not configured → recommendations still generated and shown in
    the Web UI; calendar sync skipped with a notice.
  - A2: User starred/unstarred a category → starred overrides take precedence
    in the recommendation list.
  - A3: A recommendation changes between months → the corresponding CalDAV
    event is updated (stable UID), not duplicated.
- **Postconditions:** Recommendations are visible in the Web UI and on Google
  Calendar.

### UC-006: Query and Override Recommendations

- **Actor:** User (Spender)
- **Main Flow:**
  1. User asks the dashboard / assistant "which card for groceries this
     quarter?" and gets the recommended card with its rate and reason.
  2. User can manually add/edit a category recommendation, star/unstar
     categories, and set starred overrides.
- **Alternative Flows:**
  - A1: No recommendation for the queried category → "No recommendation yet —
    run optimize or add the category."
- **Postconditions:** User sees an accurate, explainable recommendation.

### UC-007: Migrate CCManager Data and Operate the Refresh

- **Actor:** Operator
- **Preconditions:** CCManager JSON files available at a configured path.
- **Main Flow:**
  1. Operator runs the one-time migration, which reads CCManager's JSON files
     and seeds the PostgreSQL tables idempotently (re-running is safe).
  2. Operator configures sources, Ollama model/endpoint, confidence threshold,
     and CalDAV target in `config/smackerel.yaml`.
  3. Operator can trigger "scrape now" and "sync calendar now" from the Web UI
     admin page and read the run history / extraction audit.
- **Alternative Flows:**
  - A1: Migration run twice → no duplicate rows (idempotent upserts keyed on
    natural keys).
  - A2: A JSON file is missing/partial → migration imports what it can and logs
    what it skipped; it does not abort the whole import.
- **Postconditions:** smackerel holds all CCManager data; CCManager + Render can
  be retired.

---

## Functional Requirements

| ID | Requirement | Maps to |
|----|-------------|---------|
| FR-CR-001 | The system SHALL store a card catalog (issuer, type, annual fee, base/rotating/selectable benefits, perks, aliases) in PostgreSQL. | G1 |
| FR-CR-002 | The system SHALL store the user's wallet (held cards) with nickname, note, and activation state. | G1, UC-001 |
| FR-CR-003 | The system SHALL resolve a free-text card description to catalog candidates (discovery) and support custom (non-catalog) cards. | UC-001 |
| FR-CR-004 | The system SHALL store offers (category, rate, limit, shared-limit group, date window, activation flag) and support shared/combined limits. | UC-004 |
| FR-CR-005 | The system SHALL store selectable-category selections, including tiered and quarterly re-enrollment tracking. | UC-003 |
| FR-CR-006 | The system SHALL store sign-up bonuses (spend bonuses with required spend + deadline; first-year rate bonuses) with progress. | UC-004 |
| FR-CR-007 | The system SHALL refresh rotating categories using a strict-JSON-schema LLM extraction call, validated before storage. | G2, UC-002, §17.2 |
| FR-CR-008 | The system SHALL retain per-source provenance (URL, source name, issuer hint) and verbatim evidence for every extraction. | Principle 4, UC-002 |
| FR-CR-009 | The system SHALL reconcile multiple source observations into one rotating-category record with aggregate confidence and a `needs_verification` flag. | G3, UC-002 |
| FR-CR-010 | The system SHALL NEVER silently serve stale or placeholder rotating-category data when extraction fails or is low-confidence; it MUST flag for verification instead. | Failure Condition, UC-002 A1/A2 |
| FR-CR-011 | The system SHALL support manual override of any rotating-category record; extraction MUST NOT overwrite an override. | UC-002 A5 |
| FR-CR-012 | The system SHALL advance rotating-category lifecycle (upcoming → active → expired) by date. | Principle 3, G3 |
| FR-CR-013 | The system SHALL run an optimizer mapping spend categories → best card using base + rotating + offers + selections + limits + bonus context. | G4, UC-005 |
| FR-CR-014 | The system SHALL generate monthly recommendations and persist them per period, honoring starred overrides. | G4, UC-005/UC-006 |
| FR-CR-015 | The system SHALL deliver monthly recommendations and due re-enrollment reminders to Google Calendar via CalDAV using stable UIDs (update, not duplicate). | G5, UC-005 |
| FR-CR-016 | The system SHALL provide a server-rendered Web UI with full CRUD parity to CCManager (wallet, offers, selections, bonuses, categories, recommendations, rotating verify, report, run history/admin). | G6, UC-001/003/004/006/007 |
| FR-CR-017 | The system SHALL provide a one-time, idempotent migration of CCManager JSON state into PostgreSQL. | G7, UC-007 |
| FR-CR-018 | The system SHALL log every refresh, extraction, optimization, migration, and calendar sync to a run-history/audit surface. | G8, Principle 8 |
| FR-CR-019 | The system SHALL expose manual "scrape now" and "sync calendar now" triggers in the admin UI. | UC-007 |
| FR-CR-020 | All card-rewards tunables SHALL originate from `config/smackerel.yaml`; when enabled, missing required values SHALL be a fatal startup error (no defaults). | Gate G028, smackerel-no-defaults |

## Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NFR-CR-001 | Go-first per Constitution C2: the connector, store, optimizer, reconciler, scheduler jobs, CalDAV bridge, and Web UI are Go. The **model-gateway (Ollama) call is owned by the Python ML sidecar** (`ml/app/`), like the existing `drive_classify.py` / `intelligence.py` — C2 reserves "model gateway work" for the sidecar. `internal/cardrewards/extract.go` orchestrates the call over the existing Go↔sidecar HTTP contract (pattern: `internal/agent/embedder/sidecar`) and validates the response; it does NOT embed a direct Go→Ollama client. On home-lab the sidecar targets the evo-x2 host Ollama endpoint. |
| NFR-CR-002 | All persistence is PostgreSQL (PostgreSQL-only governance). No SQLite/embedded/file store at runtime. |
| NFR-CR-003 | LLM extraction enforces a strict output schema and validates JSON before storage; malformed responses are logged and discarded (§17.2). |
| NFR-CR-004 | Network egress is limited to: reading configured public source pages (read-only) and CalDAV writes to the user's chosen calendar. No other outbound writes. |
| NFR-CR-005 | The daily refresh and monthly recommendation jobs are idempotent and safe to re-run; manual triggers reuse the same code paths. |
| NFR-CR-006 | The Web UI follows smackerel's `internal/web` Go `html/template` + chi paradigm with the existing bearer/session auth and CSP posture (spec 044/070). |
| NFR-CR-007 | Extraction and reconciliation are bounded by configured timeouts; a slow/failed source degrades that source only, not the whole run. |
| NFR-CR-008 | **Cross-project tech consistency.** This feature introduces NO new runtime dependency, language, or framework. It reuses smackerel's established stack verbatim: `jackc/pgx` (PostgreSQL), `go-chi/chi` + `html/template` (Web), `robfig/cron` via `internal/scheduler`, `santhosh-tekuri/jsonschema` (strict-schema validation), `go-shiori/go-readability` + `net/http` via `internal/extract` (fetch/clean), the Python ML sidecar (model gateway), and the `internal/mealplan` CalDAV bridge pattern — all already in `go.mod` / the repo. Any deviation (a new HTTP/HTML/LLM/DB library, a Go-side model client, or a non-Go runtime surface beyond the one sidecar route) is a consistency violation requiring explicit owner sign-off with a recorded rationale. |

---

## Product Principle Alignment

> Citations use the ratified numbering in
> [`docs/Product-Principles.md`](../../docs/Product-Principles.md) (binding via
> `.github/instructions/product-principles.instructions.md`, ratified
> 2026-06-03), cross-referenced to [`docs/smackerel.md`](../../docs/smackerel.md).

| Principle / Section | How this feature aligns |
|---------------------|-------------------------|
| **§16.8 Financial Awareness (Light Touch)** (`docs/smackerel.md`) | This is the explicit home for the feature. Credit-card cashback optimization is consumer-spend awareness — the same class as the shipped subscription registry, bill reminders, and purchase tracking listed in §16.8. "Use the Chase card for groceries this quarter" is a recommendation, exactly like the shipped meal-plan recommendations. |
| **Principle 3 — Knowledge Breathes** | Rotating categories have a real lifecycle (upcoming → active → expired) driven by date. The knowledge surface is live, not static — expired quarters decay out of recommendations automatically. |
| **Principle 4 — Source-Qualified Processing** | Every extracted category retains its source URL, source name, issuer hint, and verbatim evidence. The connector preserves source metadata on each raw observation; reconciliation tracks per-source provenance. |
| **Principle 5 — One Graph, Many Views** | All card data lives in one PostgreSQL store. Raw source observations are smackerel artifacts in the same graph; the relational card tables are projections. No parallel store, no SQLite. |
| **Principle 8 — Trust Through Transparency** | Every refresh/extraction/optimize/sync is logged to run history. Recommendations cite the rate and reason; extractions cite the source. Low-confidence/conflicting data is shown with a `needs_verification` badge rather than hidden. |
| **Principle 10 — QF Companion Boundary (NON-NEGOTIABLE)** | **NOT crossed.** Principle 10 / §1.6 governs QF *investment* decisions (trade approval, mandate change, execution, investment advice). This feature does none of those: it makes consumer-spend cashback recommendations only. It initiates no transactions, touches no bank/brokerage account, and gives no investment advice. It is firmly inside the §16.8 light-touch financial-awareness lane and outside the §1.6 boundary. |

### Constitution / Governance Alignment

| Rule | Compliance |
|------|------------|
| Constitution C2 (Go-first) | All runtime code is Go; Python app is re-expressed, not ported. |
| PostgreSQL-only | All persistence in PostgreSQL; migration is one-time JSON→PG. |
| Config SST / no-defaults (Gate G028, `smackerel-no-defaults`) | All tunables in `config/smackerel.yaml`; fail-loud when enabled and unset. |
| §17.2 strict LLM output schemas | Extraction validates JSON before storage; malformed → discarded. |
| Release trains | Targets `mvp` train (home-lab); flag declared (see Release Train). |

---

## Release Train

- **Target train:** `mvp` (phase `active`, `target_slot: home-lab`) per
  [`config/release-trains.yaml`](../../config/release-trains.yaml). This feature
  is hosted on evo-x2 home-lab, which the `mvp` train targets.
- **Default-off behavior on other trains:** The feature is gated by a single
  flag `card_rewards`, default-ON only in the `mvp` train bundle and default-OFF
  in every other train bundle. On trains where the flag is OFF, no card-rewards
  connector, scheduler job, Web UI route, or migration runs.
- **Flag wiring is deferred to delivery.** Per the release-train policy, the
  flag bundle files (`config/feature-flags.<train>.yaml`) are owned by
  `bubbles.train` and are edited during implementation, NOT during this
  planning-only effort. `state.json` records `flagsIntroduced: ["card_rewards"]`
  so delivery wires it correctly.
- The flag is read from an env var with NO fallback default (fail-fast when the
  feature is enabled but the flag env var is missing), per the release-train
  and `smackerel-no-defaults` policies.

---

## Offline / Host Note

- **Hosting:** This feature ships as part of smackerel on evo-x2 home-lab via
  smackerel's **EXISTING** deploy adapter
  ([`deploy/compose.deploy.yml`](../../deploy/compose.deploy.yml) and the
  knb-side `smackerel/home-lab/` overlay). **No new knb product adapter is
  required** — card rewards is just more smackerel.
- **LLM locality:** Rotating-category extraction targets the evo-x2 **host
  Ollama** endpoint (per `knb/docs/HomeLabServices.md`, models such as
  `gpt-oss:20b` / `qwen3-coder:30b` at `http://<host-tailnet-ip>:11435`). LLM
  extraction is therefore local, free, and requires no external API keys —
  consistent with Principle 9 ("Own your data") and §17.2.
- **Network touchpoints:** Only (a) reading configured public source pages
  (read-only ingestion) and (b) CalDAV writes to the user's Google Calendar
  (the user's chosen delivery surface, same trust posture as the existing
  `caldav` connector). Core optimization, recommendation, and Web UI browsing
  work entirely from stored data; only the daily refresh needs network. A
  missed refresh window does not break the feature — stored data remains usable
  and the lifecycle engine keeps advancing (Principle 12, "design for restart").

---

## Open Questions (for owner review)

1. **Source set for extraction.** CCManager scraped "Doctor of Credit". Which
   sources should be configured (DoC + issuer pages?) and is fetching them
   acceptable under their terms? Design assumes a configurable list with at
   least DoC + each issuer's official rotating-categories page.
2. **Ollama model choice on evo-x2.** Which host model for structured
   extraction (e.g., `gpt-oss:20b` vs `qwen3-coder:30b`)? Affects latency and
   schema-adherence quality. Design leaves it a required config key.
3. **CalDAV target.** Reuse the existing `caldav` connector's Google Calendar
   credentials, or a dedicated card-rewards calendar? Design assumes the
   existing CalDAV credentials with a dedicated event category/UID prefix.
4. **PWA surface scope.** Is a read-only "this month's card recommendations"
   card in the mobile/PWA assistant (spec 073/077) wanted in v1, or is the
   server-rendered Web UI sufficient? Design treats the PWA card as an optional
   stretch within the relevant UI scope.
5. **Card-art images.** CCManager has a card-image scraper. Confirmed out of
   scope for v1 — confirm that is acceptable.
6. **Sign-up bonus spend progress.** Without live transaction data, spend
   progress is manually entered. Confirm manual entry is acceptable for v1
   (no card-feed integration is in scope).

---

## Links

- [design.md](design.md) — architecture, schema, LLM contract, migration, UI IA
- [scopes.md](scopes.md) — dependency-ordered scopes, Gherkin, test plans, DoD
- [uservalidation.md](uservalidation.md) — user acceptance checklist
- [report.md](report.md) — execution evidence (planning scaffold)
- Source app: `~/CCManager` (to be retired)
- Reused primitives: `internal/connector/connector.go`, `internal/mealplan/calendar.go`, `internal/scheduler/`, `internal/web/handler.go`, `config/smackerel.yaml`
