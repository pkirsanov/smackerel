# Feature: 027 — User Annotations & Interaction Tracking

> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Depends On:** Phase 2 Ingestion (spec 003)
> **Author:** bubbles.analyst
> **Date:** April 17, 2026
> **Status:** Draft

---

## Problem Statement

Smackerel captures and processes content automatically, but the user has almost no way to **tell the system what they think** about what was captured. The only user signal today is a binary `user_starred` boolean. There is no way to rate an artifact, add personal notes, tag it with custom labels, record that you actually used/made/visited/read it, or track how many times you've engaged with it.

This matters because the intelligence engine's recommendations, the digest assembly, and the knowledge lifecycle all need **user preference signals** to work well. Without them, the system treats a recipe the user made 20 times identically to one they glanced at once. A product they already bought looks the same as one they're still considering. The system cannot learn taste, preference, satisfaction, or usage patterns.

This gap is entirely domain-agnostic. Whether the artifact is a recipe, article, product, video, place, or idea — users need to annotate, rate, and track their interaction with it. Every downstream recommendation, ranking, and proactive suggestion depends on this signal.

---

## Outcome Contract

**Intent:** Users can annotate any artifact with ratings, notes, custom tags, and interaction records ("made it", "bought it", "visited", "read it"). These annotations feed into the intelligence engine to improve relevance scoring, recommendations, and digest curation. The annotation interface is zero-friction — a single Telegram reply or API call.

**Success Signal:** User sends a recipe link on Monday. On Wednesday, they reply to it with "made it, 4/5, needs more garlic". The system records: interaction type "made_it", rating 4, note "needs more garlic". When the user later asks for recipe suggestions, this recipe's high rating boosts similar recipes. When the system generates a shopping list, it knows this is a recipe the user actually makes. The annotation took 5 seconds.

**Hard Constraints:**
- Annotations are append-only interaction records, not overwrites — the user's history of engagement is preserved
- Rating scale is 1-5 (consistent across all artifact types)
- Custom tags are freeform strings, not from a predefined taxonomy
- Annotations are queryable via the search API (filter by rating, tag, interaction type)
- Annotation operations are available from every channel (Telegram, API, future web UI)
- Annotations never modify the original artifact content or LLM-generated fields — they live in a separate layer
- Interaction tracking counts (times_used, last_used) are automatically updated from annotation events

**Failure Condition:** If users can annotate but the intelligence engine ignores annotations (no impact on recommendations or relevance), the feature adds friction without value. If annotations require navigating to a separate interface or remembering complex syntax, adoption will be zero. If the annotation model is artifact-type-specific (different annotation schema for recipes vs. products), the generalization has failed.

---

## Goals

1. Define a universal annotation model that works for any artifact type
2. Implement annotation CRUD via REST API
3. Implement zero-friction annotation via Telegram (reply-to-artifact with rating/note/tag)
4. Track interaction history per artifact (times used, last used, usage pattern)
5. Feed annotation signals into the intelligence engine's relevance scoring
6. Make annotations searchable and filterable via the existing search infrastructure

---

## Non-Goals

- Collaborative annotations (multi-user, comments, shared tags)
- Annotation analytics dashboard or visualization
- Automated annotation inference (system guessing ratings from behavior)
- Annotation export or import
- Per-field annotation on domain-extracted data (e.g., annotating a single ingredient)

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|------------|-----------|-------------|
| User (Annotator) | Person adding annotations to artifacts | Record opinion, track usage, organize with tags | Write annotations on own artifacts |
| System (Intelligence) | Recommendation and relevance engine | Use annotations to improve scoring and suggestions | Read annotations |
| System (Search) | Query engine | Filter and rank results by annotation data | Read annotations |
| System (Digest) | Daily/weekly digest generator | Prioritize artifacts with high ratings and recent usage | Read annotations |

---

## Use Cases

### UC-001: Quick Rating via Telegram
- **Actor:** User (Annotator)
- **Preconditions:** User previously received an artifact confirmation or search result in Telegram
- **Main Flow:**
  1. User replies to a Telegram message containing an artifact with "4/5"
  2. Bot parses the rating and associates it with the artifact
  3. Bot confirms: "Rated ★★★★☆"
  4. Artifact's annotation record is updated with rating and timestamp
- **Alternative Flows:**
  - User sends "4/5 needs more salt" → rating 4 + note "needs more salt"
  - User sends just a note without rating → note stored, no rating change
  - User rates an artifact that was already rated → new rating is appended to history, latest becomes active
- **Postconditions:** Artifact has a rating and optional note in its annotation record

### UC-002: Record Usage Interaction
- **Actor:** User (Annotator)
- **Preconditions:** Artifact exists in the system
- **Main Flow:**
  1. User sends "made it" (or "bought it", "read it", "visited", "tried it") referencing an artifact
  2. System records an interaction event with type and timestamp
  3. Artifact's `times_used` counter increments; `last_used` updates
  4. System confirms the interaction
- **Alternative Flows:**
  - User sends "made it, loved it, 5/5" → interaction + rating + implicit positive note
  - User sends "bought it" for a product → interaction type "purchased"
- **Postconditions:** Interaction event logged; usage counters updated

### UC-003: Custom Tagging
- **Actor:** User (Annotator)
- **Preconditions:** Artifact exists in the system
- **Main Flow:**
  1. User sends "#weeknight #quick #kids-approved" referencing an artifact
  2. System parses hashtags and adds them as custom tags
  3. Tags are searchable and filterable
- **Alternative Flows:**
  - Tag already exists on artifact → no duplicate
  - User sends "#remove-quick" → tag "quick" removed from artifact
- **Postconditions:** Artifact has custom tags attached

### UC-004: Annotation-Aware Search
- **Actor:** User (Searcher)
- **Preconditions:** Artifacts with annotations exist
- **Main Flow:**
  1. User queries "my top rated recipes" or "things tagged weeknight"
  2. Search engine detects annotation filter intent
  3. Results filtered/boosted by annotation data
  4. Results show rating and usage count alongside standard fields
- **Postconditions:** Results reflect user's personal annotations

### UC-005: Intelligence Engine Uses Annotations
- **Actor:** System (Intelligence)
- **Preconditions:** User has rated and used multiple artifacts over time
- **Main Flow:**
  1. Intelligence engine reads annotation data during recommendation generation
  2. High-rated artifacts boost similar artifacts' relevance
  3. Frequently-used artifacts inform user preference profile
  4. Recently-used artifacts reduce immediate re-recommendation (variety signal)
  5. Never-used saved artifacts may trigger "you saved but never tried" resurfacing
- **Postconditions:** Recommendations reflect user preferences learned from annotations

---

## Business Scenarios

### BS-001: Rate a Recipe After Cooking
Given the user received a recipe artifact confirmation in Telegram
When the user replies with "made it 5/5 absolutely delicious"
Then the system records interaction type "made_it", rating 5, note "absolutely delicious"
And the artifact's times_used increments to reflect the new interaction
And the response confirms the annotation within 2 seconds

### BS-002: Tag an Article for Later Reference
Given the user has a saved article about investment strategies
When the user sends "#finance #long-term #revisit-quarterly" referencing the article
Then the artifact receives all three custom tags
And searching for "#finance" returns this article in results

### BS-003: Rate Without Prior Context
Given the user wants to rate an artifact they recall but don't have the Telegram message for
When the user sends "/rate pasta carbonara recipe 4/5 too salty last time"
Then the system searches for the best-matching artifact
And asks the user to confirm if ambiguous
And records the rating and note on the confirmed artifact

### BS-004: Usage History Informs Recommendations
Given the user has made 5 Italian recipes rated 4-5 stars in the past month
And the user has 3 Thai recipes saved but never made
When the system generates recipe recommendations
Then Italian recipes are weighted higher (proven preference)
And saved-but-unused Thai recipes are occasionally surfaced (exploration signal)
And recently-made Italian recipes are not immediately re-recommended (variety)

### BS-005: Annotation-Boosted Search Ranking
Given the user has 100 saved articles, 10 of which are rated 4+ stars
When the user searches "productivity tips"
Then articles rated 4+ by the user rank above unrated articles with similar semantic match
And the rating is displayed alongside each result

### BS-006: Never-Used Artifact Resurfacing
Given the user saved a product link 30 days ago but never interacted with it again
When the intelligence engine runs its daily resurfacing job
Then it may include this artifact in the digest with a prompt like "Still interested in [product]?"

### BS-007: Annotation via REST API
Given a client application wants to submit an annotation programmatically
When it sends `POST /api/artifacts/{id}/annotations` with `{rating: 4, note: "good", tags: ["quick"], interaction: "made_it"}`
Then the annotation is recorded identically to a Telegram annotation
And the response includes the updated annotation summary

### BS-008: Annotation on Artifact Still Processing
Given a user just captured a URL that hasn't finished ML processing
When the user immediately replies with "5/5 this looks great"
Then the annotation is stored against the artifact ID (which exists from capture)
And when processing completes, the annotation is already associated
And the user sees no error

### BS-009: Orphaned Annotations on Artifact Deletion
Given a user has 10 annotations on an artifact
When the artifact is deleted from the system
Then all associated annotations are cascade-deleted
And the materialized summary view is refreshed
And the intelligence engine stops using these annotations for recommendations

### BS-010: Annotation Flood Prevention
Given an automated client sends 1000 annotation requests in 10 seconds
When the system detects abnormal annotation volume
Then the existing API rate limiter (Throttle 100) applies
And earlier annotations are preserved
And excess requests receive 429 Too Many Requests

### BS-011: List Completion Creates Interaction Annotations
Given a user completes a shopping list generated from 3 recipes (spec 028)
When the list status changes to "completed"
Then the system auto-creates "made_it" interaction annotations on all 3 source recipe artifacts
And the intelligence engine records these as actual usage events
And the user's recipe preference profile is updated

### BS-012: Rating Parse Ambiguity
Given a user replies to an artifact with "4/5/2026"
When the annotation parser encounters this pattern
Then the system recognizes this as a date (not a 4/5 rating) due to the third segment
And no rating is recorded
And the full text is stored as a note

---

## Non-Functional Requirements

- **Latency:** Annotation operations complete in < 2 seconds
- **Storage:** Annotation history is append-only; storage grows linearly with user interactions (acceptable for single-user system)
- **Search integration:** Annotation filters add < 100ms to search query time
- **Channel parity:** Every annotation operation available via Telegram is also available via REST API
- **Backwards compatibility:** Artifacts without annotations continue to work identically; the existing `user_starred` field is preserved and treated as equivalent to a 5-star rating for ranking purposes

---

## Annotation Data Model (Conceptual)

```
annotations:
  id: unique identifier
  artifact_id: reference to artifact
  annotation_type: "rating" | "note" | "tag" | "interaction" | "status_change"
  rating: 1-5 (nullable, only for rating type)
  note: freeform text (nullable)
  tags: [string] (nullable, only for tag type)
  interaction_type: "made_it" | "bought_it" | "read_it" | "visited" | "tried_it" | "used_it" (nullable)
  created_at: timestamp

artifact annotation summary (materialized/cached):
  artifact_id: reference
  current_rating: latest rating value
  average_rating: mean of all ratings
  rating_count: number of ratings
  times_used: count of interaction events
  last_used: most recent interaction timestamp
  tags: deduplicated set of all tags
  notes_count: number of notes
```

---

## UI Wireframes

### Screen: Telegram Annotation Confirmation
**Actor:** User | **Channel:** Telegram | **Status:** New

```
┌─────────────────────────────────────────────────┐
│  User replies: "4/5 made it #weeknight great"   │
│                                                   │
│  ↓ Bot response:                                 │
│                                                   │
│  ✅ Annotated: Pasta Carbonara                   │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━                 │
│  ⭐ Rating: ★★★★☆ (4/5)                        │
│  🏷 Tags: #weeknight                            │
│  🍳 Interaction: Made it!                        │
│  📝 Note: "great"                                │
│                                                   │
│  Overall: ⭐ 4.0 avg | Made 3× | Tagged: 2      │
└─────────────────────────────────────────────────┘
```

**Interactions:**
- Reply-to any artifact message → parsed as annotation
- `/rate pasta carbonara 4/5` → search + disambiguate + annotate

**States:**
- Ambiguous artifact: "Did you mean: 1) Pasta Carbonara 2) Pasta Primavera?"
- Parse failure: "I couldn't parse a rating or interaction. Try: '4/5 made it'"
- Already rated: "Updated rating from 3 → 4. New average: 4.0"

### Screen: Telegram /rate Command with Disambiguation
**Actor:** User | **Channel:** Telegram | **Status:** New

```
┌─────────────────────────────────────────────────┐
│  User: /rate pasta 5/5                           │
│                                                   │
│  ↓ Bot response:                                 │
│                                                   │
│  Multiple matches for "pasta":                   │
│                                                   │
│  1️⃣ Pasta Carbonara (allrecipes.com)             │
│     ⭐ 4.0 | Made 3× | 2 days ago               │
│                                                   │
│  2️⃣ Pasta Primavera (epicurious.com)             │
│     ⭐ — | Never made | 1 week ago               │
│                                                   │
│  Reply with a number to select.                  │
└─────────────────────────────────────────────────┘
```

### Screen: API Annotation Summary Response
**Actor:** Client | **Channel:** REST API | **Status:** New

```json
GET /api/artifacts/art-001/annotations/summary

{
  "artifact_id": "art-001",
  "current_rating": 4,
  "average_rating": 4.0,
  "rating_count": 3,
  "times_used": 2,
  "last_used": "2026-04-15T18:30:00Z",
  "tags": ["weeknight", "quick", "italian"],
  "notes_count": 1
}
```

**Responsive:** N/A (API-only)
**Accessibility:** JSON responses include semantic field names

---

## Improvement Proposals

### IP-001: Mood/Context Tagging ⭐ Competitive Edge
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** No tool captures "I was feeling adventurous when I tried this" — enables mood-based recommendations
- **Actors Affected:** All users
- **Business Scenarios:** "Suggest something for a cozy evening" → system knows which artifacts the user associates with "cozy"

### IP-002: Photo Annotation
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Attach photos to usage events (photo of the dish you made, the product you bought)
- **Actors Affected:** All users
- **Business Scenarios:** User's personal recipe journal with photos builds automatically

### IP-003: Modification Tracking
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Track "I changed X" on recipes/procedures — system learns user preferences over time
- **Actors Affected:** Users who adapt saved content
- **Business Scenarios:** "I always add extra garlic" becomes a known preference across recipes
