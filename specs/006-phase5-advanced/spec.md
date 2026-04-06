# Feature: 006 — Phase 5: Advanced Intelligence (Expertise + Learning + Serendipity)

> **Parent Spec:** [specs/001-smackerel-mvp](../001-smackerel-mvp/spec.md)
> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Phase:** 5 of 5
> **Depends On:** Phase 3 (Intelligence)
> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft

---

## Problem Statement

Phases 1-4 make Smackerel a capable knowledge engine that captures, connects, and surfaces information. But the deepest promise is **self-knowledge** — the system knows what you know better than you do. After months of ingestion, the knowledge graph contains signals about expertise depth, learning trajectories, interest evolution, and information consumption patterns that no human could track manually.

Phase 5 extracts this meta-intelligence: expertise mapping (what you actually know vs. what you think you know), learning path assembly (turning scattered resources into structured curricula), content creation fuel (turning deep knowledge into original angles), subscription tracking (financial awareness), and the serendipity engine (resurfacing valuable old items that match current context).

---

## Outcome Contract

**Intent:** After 3+ months of ingestion, the system generates a personal expertise map showing knowledge depth and breadth across topics, auto-assembles learning paths from scattered resources, identifies writing angles from accumulated knowledge, tracks subscription spending from email patterns, and regularly resurfaces old valuable items that connect to current interests or upcoming events.

**Success Signal:** The user asks "what do I actually know?" and receives an expertise map showing their deepest topic (product strategy: 230 captures) and weakest relative to their role (data analytics: 12 captures). The system has assembled a TypeScript learning path from 8 scattered saves. A serendipity resurface from October matches next week's team offsite perfectly. Monthly synthesis shows $127/mo in subscriptions with 3 overlapping services.

**Hard Constraints:**
- Expertise assessment is based on capture volume, depth, and engagement — not quiz results
- Learning paths are assembled from the user's own saved resources — not external recommendations
- Content creation prompts reference specific saved artifacts — not generic advice
- Subscription tracking is observational (from email patterns) — no bank/card integration
- Serendipity is limited to 1 item per week — never a flood of old content

**Failure Condition:** If the expertise map shows topics with 2 captures as "deep expertise," or if learning paths include resources the user never saved, or if serendipity resurfaces irrelevant items with no connection to current context — this phase has failed.

---

## Goals

1. Build expertise mapping: topic depth, breadth, and growth trajectory analysis
2. Build learning path assembly: turn scattered resources into ordered curricula
3. Build content creation fuel: identify original writing angles from accumulated knowledge
4. Build subscription/spending tracker from email receipt/billing patterns
5. Build full serendipity engine: context-aware resurfacing of archived items
6. Build monthly self-knowledge report: expertise shifts, interest evolution, information diet
7. Detect repeated lookups and auto-create quick references

---

## Non-Goals

- Formal certification or testing of knowledge
- External content recommendations (only user's own saved resources)
- Financial advice or budgeting tools
- Bank account or credit card integration
- Spaced repetition system (post-MVP improvement proposal)
- Social features or expertise sharing
- Real-time learning analytics during video/article consumption

---

## Requirements

### R-501: Expertise Mapping
- **Depth Analysis per Topic:**
  - Capture count (total + time-weighted)
  - Source diversity: how many different sources contribute (email, video, article, notes)
  - Content depth: ratio of Full-tier to Light-tier processing (deeper content = more expertise)
  - Engagement: search hits, access count, time spans (sustained interest vs. spike)
  - Connection density: how linked this topic's artifacts are to each other and other topics
- **Breadth Analysis:**
  - Total topic count with >5 captures
  - Subtopic coverage within major topics
  - Cross-domain connections from this topic to others
- **Growth Trajectory:**
  - Capture velocity: accelerating, steady, decelerating, stopped
  - Momentum score trend over 6+ months
  - Comparison to 3-month-ago depth
- **Expertise Tiers:**
  - Novice: 1-5 captures, light engagement
  - Foundation: 6-20 captures, some depth
  - Intermediate: 21-50 captures, diverse sources, sustained engagement
  - Deep: 51-100 captures, cross-domain connections, repeated access
  - Expert: 100+ captures, high source diversity, long-term sustained, teaching-level synthesis
- **Blind Spot Detection:**
  - Compare expertise map to role/domain expectations (inferred from capture patterns)
  - "You save a lot about product management but almost nothing about analytics/metrics — widest blind spot"
- **Visual Output:**
  - Topic map with size proportional to depth, color indicating growth trajectory
  - Expertise tier per topic
  - Month-over-month evolution

### R-502: Learning Path Assembly
- **Trigger:** Topic has 5+ saved resources of learning nature (articles, videos, books, courses)
- **Assembly Process:**
  1. Gather all learning-type artifacts in the topic
  2. Classify by difficulty: beginner, intermediate, advanced (from LLM analysis of content)
  3. Order logically: foundational concepts first, then progressive complexity
  4. Remove duplicates (different articles teaching the same concept)
  5. Estimate total time (reading time + video duration)
- **Output per Learning Path:**
  - Topic name
  - Total resources and estimated time
  - Ordered list: resource title, type, difficulty, estimated time, key takeaway
  - Gaps identified: "No intermediate-level resources — consider finding a tutorial between X and Y"
- **Searchable:** "Show me the TypeScript learning path" → structured curriculum
- **Updates:** Path re-assembles when new resources are added to the topic
- **Action:** Mark resources as "completed" to track progress through the path

### R-503: Content Creation Fuel
- **Trigger:** Topic has 30+ captures with depth across sources
- **Analysis:**
  1. Identify unique perspectives and positions the user has accumulated
  2. Find areas where the user has contrarian or nuanced views (from artifact sentiment + synthesis)
  3. Detect conversations/threads where the user had original insights (from email/notes)
  4. Map the user's strongest supporting evidence per position
- **Output:** 3-5 original writing angle suggestions, each with:
  - Angle title
  - Why it's unique (what evidence/perspective the user has that others don't)
  - Key supporting artifacts (3-5 specific references)
  - Estimated word count / format suggestion (blog post, thread, essay)
- **Prompt:** "Based on your 30+ captures about remote work, here are 5 original angles you could write about — grounded in specific things you've saved"
- **Frequency:** Generated on-demand or when topic crosses the 30-capture threshold

### R-504: Subscription/Spending Tracker
- **Source:** Email receipt and billing pattern detection (from Phase 2 Gmail connector)
- **Detection:**
  - Recurring charge emails: same sender, regular interval, consistent amount format
  - Subscription confirmation emails: "Your subscription to X", "Monthly billing: $Y"
  - Trial expiration warnings
  - Price change notifications
- **Registry:**
  - Service name, amount, billing frequency (monthly/annual)
  - Start date (first detected charge)
  - Status: active, cancelled, trial
  - Category: productivity, entertainment, learning, utilities, other
  - Source email artifact links
- **Analysis:**
  - Total monthly spend across all detected subscriptions
  - Overlap detection: "3 services — Grammarly, LanguageTool, ProWritingAid — overlap in functionality"
  - New subscription alerts: "You signed up for X this month ($9.99/mo)"
  - Unused subscription detection: no login/usage signals for 3+ months (if browser history enabled)
- **Searchable:** "How much am I spending on subscriptions?" → registry with total
- **Delivery:** Monthly subscription summary in the monthly synthesis (or on-demand)

### R-505: Serendipity Engine
- **Purpose:** Prevent valuable old knowledge from being buried forever
- **Selection Criteria:**
  - Age: 6+ months since last access
  - Quality: relevance_score was above average when active
  - Context match: bonus for items that connect to current hot topics, upcoming calendar events, or recent captures
  - User signal: starred/pinned items eligible even if younger
- **Frequency:** 1 item per week (included in weekly synthesis under "FROM THE ARCHIVE")
- **Context-Aware Matching:**
  - If archived item matches an upcoming calendar event → prioritize
  - If archived item is semantically similar to a recent hot-topic capture → prioritize
  - If archived item was from a person the user is meeting soon → prioritize
- **Presentation:**
  - "Remember this? [Date]. [Title/summary]. [Why it's relevant now, if applicable]."
  - If no context match: pure serendipity — "You saved this 8 months ago. Still relevant?"
- **User Response:**
  - Resurface → topic/artifact gets momentum boost, moved back to active
  - Dismiss → stays archived, less likely to resurface again
  - Delete → removed permanently

### R-506: Monthly Self-Knowledge Report
- **Frequency:** Monthly (1st of each month)
- **Content:**
  - **Expertise Shifts:** Topics that gained/lost depth this month
  - **Information Diet:** Breakdown by content type (articles, videos, emails, notes) and source
  - **Interest Evolution:** 6-month topic distribution trend (e.g., "Jan-Mar: heavy AI. Apr-Jun: business strategy.")
  - **Productivity Patterns:** Capture timing patterns ("You do your deepest thinking on Wednesday mornings")
  - **Subscription Summary:** Total spend, new/cancelled, overlap alerts
  - **Learning Progress:** Status on active learning paths
  - **Top Synthesis Insights:** Best cross-domain connections of the month
- **Delivery:** Via configured channel, under 500 words
- **Tone:** Reflective, honest, data-grounded — not motivational

### R-507: Repeated Lookup Detection
- **Detection:** User searches for the same concept 3+ times within 30 days
- **Action:**
  - "You've looked up 'TypeScript generics' 6 times. Creating a permanent quick reference."
  - Auto-generate a quick-reference artifact from the best-matching saved resources
  - Pin the quick reference for instant access
- **Quick Reference Content:**
  - Key concept extracted from saved artifacts
  - Links to the source artifacts
  - Compact format (1 screen)

### R-508: Seasonal Pattern Detection
- **Requires:** 6+ months of data
- **Detection:** Year-over-year or seasonal patterns in capture behavior
- **Examples:**
  - "November: Last year you started gift shopping Dec 15 and felt rushed. Here are 12 items people mentioned wanting this year."
  - "January: You typically increase fitness-related captures. 3 resources already saved."
  - "Q4: Your capture volume drops 30% in December. Build buffer in November."
- **Delivery:** Proactive notes in monthly synthesis when seasonal context applies

---

## User Scenarios (Gherkin)

### Expertise Mapping

```gherkin
Scenario: SC-A01 Expertise map generation
  Given the system has been running for 3+ months
  And the user has 500+ artifacts across 25 topics
  When the user requests their expertise map
  Then the system shows topics ranked by depth tier
  And the deepest topic shows: name, capture count, source diversity, expertise tier
  And the weakest topic relative to capture patterns shows as a blind spot
  And each topic has a growth trajectory indicator (accelerating/steady/decelerating)

Scenario: SC-A02 Expertise blind spot detection
  Given the user has 150 captures about "product management"
  And only 8 captures about "data analytics" despite frequent references to data in PM artifacts
  When the expertise map generates blind spot analysis
  Then it identifies: "You save a lot about product management but almost nothing about analytics/metrics"
  And categorizes data analytics as the widest blind spot relative to the user's domain

Scenario: SC-A03 Interest evolution over time
  Given the user has 6+ months of capture data
  When the monthly report analyzes interest distribution by quarter
  Then it identifies shifts: "Jan-Mar: 45% AI/ML. Apr-Jun: 35% business strategy, 25% AI/ML"
  And notes: "You're developing a niche at the intersection of AI and business strategy"

Scenario: SC-A04 Expertise tier progression
  Given the topic "distributed systems" had 15 captures 3 months ago (Foundation tier)
  And now has 55 captures with diverse sources (articles, videos, books, notes)
  When the expertise map updates
  Then the topic shows progression: Foundation → Deep
  And the monthly report notes the advancement
```

### Learning Paths

```gherkin
Scenario: SC-A05 Learning path auto-assembly
  Given the user has saved 8 TypeScript resources: 2 beginner articles, 3 intermediate videos, 2 advanced articles, 1 book
  When the learning path assembly runs for "TypeScript"
  Then it creates an ordered path: beginner articles first, then intermediate videos, then advanced
  And estimates total time (~6 hours)
  And each entry shows: title, type, difficulty, estimated time
  And identifies no gaps between difficulty levels

Scenario: SC-A06 Learning path with gap
  Given the user has saved 5 Rust resources: 3 beginner articles and 2 advanced articles
  And nothing at intermediate level
  When the learning path assembly runs
  Then it creates the ordered path with a noted gap
  And suggests: "No intermediate resources — consider finding a tutorial between basics and advanced ownership concepts"

Scenario: SC-A07 Learning path progress tracking
  Given the user has a TypeScript learning path with 8 resources
  And marks 3 as "completed"
  When the user views the learning path
  Then it shows 3/8 completed, 5 remaining
  And estimates remaining time
  And the next recommended resource is highlighted

Scenario: SC-A08 Learning path update on new resource
  Given a TypeScript learning path exists with 8 resources
  When the user captures a new TypeScript article at intermediate level
  Then the learning path re-assembles with 9 resources
  And inserts the new article at the appropriate difficulty position
```

### Content Creation Fuel

```gherkin
Scenario: SC-A09 Writing angle generation
  Given the user has 35 captures about "remote work" with diverse perspectives
  When the content creation fuel analysis runs
  Then it generates 3-5 writing angles
  And each angle has: title, uniqueness rationale, 3-5 supporting artifact references
  And at least one angle reflects a contrarian or nuanced position from the user's captures

Scenario: SC-A10 Writing angle with supporting evidence
  Given the user has a writing angle "The Hidden Cost of Async Communication"
  When the user explores this angle
  Then they see the 4 specific articles, 2 videos, and 1 personal note that support this thesis
  And extracted quotes/key ideas from each source
  And a suggested structure for the piece
```

### Subscription Tracking

```gherkin
Scenario: SC-A11 Subscription registry from email patterns
  Given the system has detected recurring charge emails from Netflix, Spotify, GitHub, Grammarly, LanguageTool, and ProWritingAid
  When the user asks "how much am I spending on subscriptions"
  Then the system shows: 6 active subscriptions, ~$67/month
  And each entry shows: service, amount, frequency, start date, category

Scenario: SC-A12 Overlap detection
  Given the user has subscriptions to Grammarly ($12/mo), LanguageTool ($5/mo), and ProWritingAid ($8/mo)
  When the subscription analysis runs
  Then it flags: "3 services overlap in functionality (writing/grammar checking). Combined cost: $25/mo"

Scenario: SC-A13 New subscription alert
  Given the user receives a subscription confirmation email for a new service at $14.99/mo
  When the system processes this email
  Then it adds the service to the subscription registry
  And the next monthly synthesis mentions: "New subscription: Service X ($14.99/mo)"

Scenario: SC-A14 Trial expiration warning
  Given the system detected a trial start email for a service 12 days ago
  And the trial is 14 days
  When the trial is 2 days from expiration
  Then the system alerts: "Trial for X expires in 2 days — cancel or subscribe?"
```

### Serendipity Engine

```gherkin
Scenario: SC-A15 Context-matched serendipity
  Given the artifact "The best way to predict the future is to invent it — Alan Kay, save for team offsite intro" was saved 8 months ago
  And the user has a "Team Offsite" calendar event next week
  When the serendipity engine selects the weekly archive item
  Then it prioritizes this artifact due to calendar context match
  And presents: "Remember this? Oct 15: 'The best way to predict the future is to invent it.' Your offsite is next week."

Scenario: SC-A16 Topic-matched serendipity
  Given the topic "systems thinking" is currently hot
  And an archived article about "feedback loops in complex systems" was saved 10 months ago
  When the serendipity engine runs
  Then it detects the semantic connection to the hot topic
  And presents the archived article with context: "This connects to your current systems thinking interest"

Scenario: SC-A17 Pure serendipity (no context match)
  Given no archived items match current hot topics or upcoming events
  When the serendipity engine runs
  Then it selects a high-quality archived item at random
  And presents: "You saved this 8 months ago. Still relevant?"
  And the user can choose: resurface, dismiss, or delete

Scenario: SC-A18 Serendipity user response
  Given the user receives a serendipity resurface of an archived TypeScript article
  When the user chooses "resurface"
  Then the article's topic gets a momentum boost
  And the article moves back to active status
  And the system notes: "User tends to return to TypeScript periodically"
```

### Repeated Lookups

```gherkin
Scenario: SC-A19 Quick reference creation
  Given the user has searched for "TypeScript generics" 6 times in the past month
  When the repeated lookup detector flags this
  Then the system generates a quick reference from the best-matching saved resources
  And pins it for instant access
  And notifies: "You've looked up TypeScript generics 6 times. Here's a pinned quick reference."

Scenario: SC-A20 Quick reference content
  Given a quick reference was created for "Python decorators"
  When the user views it
  Then it shows a compact summary extracted from their 3 best saved resources about decorators
  And links to the source artifacts
  And fits on one screen
```

### Seasonal Patterns

```gherkin
Scenario: SC-A21 Gift shopping seasonal prompt
  Given the system has 12+ months of data
  And last December the user started gift shopping on Dec 15 and captured 5 stressed notes about being rushed
  And this year, 12 artifacts mention items people want
  When November arrives
  Then the monthly synthesis includes: "Last year you started gift shopping Dec 15 — here are 12 items people mentioned wanting this year"

Scenario: SC-A22 Capture volume seasonal pattern
  Given the system has detected December capture volume drops 30% year-over-year
  When November's monthly synthesis generates
  Then it includes: "Your capture volume typically drops 30% in December. Plan accordingly."
```

### Monthly Self-Knowledge Report

```gherkin
Scenario: SC-A23 Monthly report generation
  Given it is the 1st of the month
  And the system has 3+ months of data
  When the monthly report generates
  Then it includes: expertise shifts, information diet breakdown, subscription summary, learning path status
  And is under 500 words
  And is data-grounded with specific numbers (not vague observations)

Scenario: SC-A24 Productivity pattern in monthly report
  Given the system has analyzed capture timing across the month
  When the monthly report generates
  Then it identifies: "Your idea-dense capture windows are Wednesday mornings and Sunday evenings"
  And this is based on actual artifact creation timestamps and quality
```

---

## Acceptance Criteria

| ID | Criterion | Maps to Scenario | Test Type |
|----|-----------|------------------|-----------|
| AC-A01 | Expertise map shows topics ranked by depth with correct tiers | SC-A01 | E2E |
| AC-A02 | Blind spots identified relative to user's domain | SC-A02 | Integration |
| AC-A03 | Interest evolution tracked across quarters | SC-A03 | Integration |
| AC-A04 | Expertise tier progression detected accurately | SC-A04 | Unit |
| AC-A05 | Learning path assembled in correct difficulty order | SC-A05 | Integration |
| AC-A06 | Learning path identifies difficulty gaps | SC-A06 | Integration |
| AC-A07 | Learning path tracks completion progress | SC-A07 | Integration |
| AC-A08 | New resources auto-inserted into existing learning paths | SC-A08 | Integration |
| AC-A09 | Writing angles generated with specific artifact references | SC-A09 | Integration |
| AC-A10 | Supporting evidence mapped to writing angle with extracted quotes | SC-A10 | Integration |
| AC-A11 | Subscription registry built from email patterns | SC-A11 | E2E |
| AC-A12 | Overlapping subscriptions detected and flagged | SC-A12 | Integration |
| AC-A13 | New subscriptions detected and added to registry | SC-A13 | Integration |
| AC-A14 | Trial expiration warning sent 2 days before end | SC-A14 | Integration |
| AC-A15 | Calendar-context serendipity prioritized correctly | SC-A15 | Integration |
| AC-A16 | Topic-matched serendipity connects to current hot topic | SC-A16 | Integration |
| AC-A17 | Random serendipity selects high-quality archived items | SC-A17 | Integration |
| AC-A18 | Resurface response boosts topic momentum | SC-A18 | Unit |
| AC-A19 | Repeated lookup detected and quick reference created | SC-A19 | Integration |
| AC-A20 | Quick reference is compact and links to source artifacts | SC-A20 | Unit |
| AC-A21 | Seasonal gift shopping pattern detected and surfaced | SC-A21 | Integration |
| AC-A22 | Capture volume seasonal pattern reported | SC-A22 | Integration |
| AC-A23 | Monthly report generated under 500 words with required sections | SC-A23 | E2E |
| AC-A24 | Productivity timing patterns identified from capture timestamps | SC-A24 | Integration |

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| View expertise map | Solo User | Web UI → Expertise | 1. Open expertise tab | Topic map with depth tiers, growth indicators, blind spots | Expertise map |
| Drill into topic depth | Solo User | Expertise → click topic | 1. Click topic | Source breakdown, capture timeline, connected topics | Topic depth view |
| View learning path | Solo User | Web UI → Learning | 1. Select topic | Ordered path with difficulty, time, progress | Learning path view |
| Mark resource completed | Solo User | Learning path → resource | 1. Check "completed" | Progress updates, next resource highlighted | Learning path view |
| View writing angles | Power User | Web UI → Create | 1. Select topic | 3-5 angles with supporting artifacts | Content creation view |
| View subscriptions | Solo User | Web UI → Finance | 1. Open subscriptions | Registry with amounts, overlaps, total | Subscription view |
| View monthly report | Solo User | Web UI → Reports / Telegram | 1. Open report | Expertise shifts, diet, patterns, subscriptions | Monthly report |
| Respond to serendipity | Solo User | Weekly synthesis → archive item | 1. Resurface / Dismiss / Delete | Item status updated accordingly | Inline in synthesis |
| View quick reference | Solo User | Search results → pinned | 1. Open pinned item | Compact reference with source links | Quick reference view |

---

## Non-Functional Requirements

| Requirement | Target | Rationale |
|-------------|--------|-----------|
| Expertise map generation | < 30 sec for 10,000 artifacts, 100 topics | On-demand viewing must be responsive |
| Learning path assembly | < 10 sec per topic | Near-instant when user requests |
| Content creation analysis | < 1 min per topic (LLM-intensive) | Acceptable for on-demand generation |
| Subscription detection precision | > 90% (avoid false positives from one-time purchases) | Registry must be trustworthy |
| Serendipity relevance | > 50% of resurfaces should feel relevant or interesting | User must not dismiss serendipity as noise |
| Monthly report generation | < 5 min | Must complete before scheduled delivery |
| Data maturity requirement | 90+ days of ingestion for expertise map, 180+ days for seasonal patterns | Features degrade gracefully with less data |
| Quick reference quality | Compact, single-screen, covers the concept from saved resources | Must be immediately useful |
