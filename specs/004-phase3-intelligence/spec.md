# Feature: 004 — Phase 3: Intelligence (Synthesis + Alerts + Pre-Meeting Briefs)

> **Parent Spec:** [specs/001-smackerel-mvp](../001-smackerel-mvp/spec.md)
> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Phase:** 3 of 5
> **Depends On:** Phase 2 (Passive Ingestion)
> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft

---

## Problem Statement

Phases 1 and 2 make Smackerel a capable knowledge *store* — it captures, processes, and organizes information. But a store is passive; the user still has to know what to look for. The highest-value capability is **synthesis** — the system finding connections, patterns, and insights that the user would never generate on their own. This is what separates a knowledge store from a knowledge engine.

Phase 3 adds the intelligence layer: cross-domain synthesis, weekly synthesis digests, pre-meeting briefs, contextual alerts, commitment tracking, and proactive surfacing — transforming Smackerel from "remembers what you saved" to "tells you what you need to know."

---

## Outcome Contract

**Intent:** The system proactively discovers non-obvious connections across artifacts from different sources, tracks commitments and deadlines, delivers contextual pre-meeting briefs, and surfaces the right information at the right time — all without the user asking.

**Success Signal:** The user receives a weekly synthesis that identifies a genuine through-line between an article read on Monday, a YouTube video watched on Wednesday, and an email thread from Thursday — a connection the user never would have made. Before a meeting with David, the user gets a 2-sentence brief citing their last conversation and a pending follow-up. A bill reminder arrives 3 days before the due date.

**Hard Constraints:**
- Synthesis must cite source artifacts — no fabricated or hallucinated connections
- If no genuine connection exists, say so — never force spurious insights
- Pre-meeting briefs deliver 30 minutes before the event
- Maximum 2 contextual alerts per day — never spam
- Maximum 3 system-initiated prompts per week
- Weekly synthesis under 250 words
- Contradicting artifacts are flagged, not hidden

**Failure Condition:** If weekly syntheses consistently surface only surface-level topic overlaps ("you saved 3 articles about leadership"), or if pre-meeting briefs arrive after the meeting, or if commitment tracking misses obvious deadlines — the intelligence layer has failed.

---

## Goals

1. Build the synthesis engine that detects genuine cross-domain connections between artifacts
2. Implement weekly synthesis digest with required sections (connections, momentum, open loops, serendipity, patterns)
3. Implement pre-meeting brief delivery triggered by calendar events
4. Implement contextual alerts: bill reminders, commitment tracking, trip prep, return window alerts
5. Build commitment/promise detection from email content
6. Implement contradiction detection between artifacts
7. Implement pattern recognition across captures (behavioral, temporal, topical)

---

## Non-Goals

- Expertise mapping or self-knowledge (Phase 5)
- Content creation fuel / writing prompts (Phase 5)
- Learning path assembly (Phase 5)
- Location/travel intelligence beyond trip prep alerts (Phase 4)
- People intelligence deep analytics (Phase 4)
- Serendipity engine full implementation (Phase 5 — this phase includes one random resurface in weekly synthesis)

---

## Requirements

### R-301: Cross-Domain Synthesis Engine
- Runs daily as part of lifecycle cron + triggered after batch ingestion completes
- Identifies clusters of semantically related artifacts from different sources (email + article + video = different domains)
- For each candidate cluster:
  - LLM analyzes what the artifacts say *together* that none says alone (using Synthesis Prompt from design doc §15.5)
  - Assesses `has_genuine_connection` (true/false)
  - If genuine: generates 3-4 sentence through-line, identifies key tension (if any), suggests action
  - If surface-level only: discards silently
- Stores synthesis insights as first-class entities linked to source artifacts
- Synthesis always cites source artifacts by title and date — never fabricates
- Cap: evaluate max 20 clusters per daily run to limit LLM costs

### R-302: Weekly Synthesis Digest
- Generated weekly, configurable day/time (default: Sunday 4:00 PM)
- Under 250 words, plain text
- Required sections (skip any if nothing to report):
  1. **THIS WEEK:** Brief stats + notable ingestions (e.g., "47 artifacts processed: 23 emails, 8 articles, 6 videos...")
  2. **CONNECTION DISCOVERED:** Highest-value cross-domain insight from the week
  3. **TOPIC MOMENTUM:** Rising, steady, and declining topics with capture counts
  4. **OPEN LOOPS:** Unresolved commitments, stale action items, pending follow-ups
  5. **FROM THE ARCHIVE:** One random item from 6+ month old archived artifacts (serendipity)
  6. **PATTERNS NOTICED:** One behavioral observation (capture patterns, blind spots, interest shifts)
- Uses Weekly Synthesis Prompt (design doc §15.3)
- Delivered via configured channel (web UI, Telegram, Slack)
- Honest and direct tone — not cheerful fluff

### R-303: Pre-Meeting Brief Delivery
- Triggers 30 minutes before any calendar event with known attendee(s)
- Gathers context per attendee:
  - Last 3 email threads with this person
  - Shared topic interests
  - Pending commitments (things user promised them or they promised user)
  - Previous meeting notes (if any captured)
  - Any artifacts the attendee recommended or mentioned
- Generates brief: 2-3 sentences, actionable, with specific references
- Delivered via configured alert channel
- If attendee not in People registry: "No prior context. New contact."
- If no prior interactions: brief limited to event details
- Maximum 1 pre-meeting brief per meeting (no repeat alerts for same event)

### R-304: Contextual Alerts
- Event-driven, not scheduled
- Alert types:
  - **Bill reminder:** Bill detected from email, due in 3 days → alert with amount and source
  - **Commitment overdue:** User/contact promised something, deadline passed by 3+ days → reminder
  - **Trip prep:** Travel detected in calendar within 5 days → trip dossier prompt (full dossier in Phase 4)
  - **Return window:** Purchase detected with return policy, window closing in 4 days → alert
  - **Relationship cooling:** Interaction frequency with a contact dropped significantly → soft prompt
- Maximum 2 contextual alerts per day — batch if multiple
- Alerts can be dismissed, snoozed (1 day, 1 week), or acted on
- Delivered via configured alert channel

### R-305: Commitment/Promise Detection
- During email processing (Phase 2 pipeline), detect commitment language:
  - User-made promises: "I'll send you...", "I'll follow up on...", "Let me get back to you about..."
  - Contact-made promises: "I'll have that to you by...", "I'll send the report..."
  - Deadlines: explicit dates, "by Friday", "end of week", "tomorrow"
  - Explicit to-dos: "Can you review...", "Please check...", "Action item: ..."
- Create action item with: commitment text, type (user-promise/contact-promise/deadline/todo), expected date, linked person, status (open/resolved)
- Auto-resolve: if follow-up email from same thread detected, prompt user to confirm resolution
- Surface in daily digest under "TOP ACTIONS"
- Surface overdue items as contextual alerts (R-304)

### R-306: Contradiction Detection
- When synthesis engine finds artifacts that assert conflicting claims on the same topic:
  - Flag as contradiction with both positions stated
  - Include in weekly synthesis: "Two articles you saved disagree on X. Here are both positions."
  - Store contradiction as edge type between the artifacts
- Key difference highlighted (e.g., "They define productivity differently")
- Do not resolve or take sides — present both perspectives

### R-307: Pattern Recognition
- Analyze capture behavior for patterns:
  - **Topic distribution shifts:** "Last month 45% entertainment, 30% work, 15% learning"
  - **Capture frequency patterns:** "You do your deepest thinking on Wednesday mornings"
  - **Blind spot detection:** "You save a lot about product management but nothing about analytics"
  - **Commitment patterns:** "You've promised to send something to 3 people this week and haven't followed through"
  - **Interest acceleration:** "Leadership has become your fastest-growing topic — 12 captures in 3 weeks"
- Surface one pattern observation per weekly synthesis
- Cap: 1 behavioral observation per week (avoid being annoying)

### R-308: Enhanced Daily Digest
- Upgrade Phase 1 daily digest with intelligence layer data:
  - TOP ACTIONS: include commitment-tracked items with overdue flags
  - OVERNIGHT: include source-qualified ingestion summary (not just counts)
  - HOT TOPIC: include acceleration context ("4 new captures this week, up from 1/week")
  - TODAY: include pre-meeting brief previews for today's calendar
- Still under 150 words total
- Still uses SOUL.md personality (calm, direct, warm)

---

## User Scenarios (Gherkin)

### Cross-Domain Synthesis

```gherkin
Scenario: SC-I01 Genuine cross-domain connection detected
  Given the user saved an article about Team Topologies on Monday
  And watched a YouTube talk about Inverse Conway Maneuver on Wednesday
  And received an email thread about reorging the platform team on Friday
  When the synthesis engine runs at the end of the week
  Then it clusters these 3 artifacts by semantic similarity
  And detects they converge on "aligning team structure with system boundaries"
  And generates a 3-sentence through-line citing all three sources
  And surfaces this in the weekly synthesis under "CONNECTION DISCOVERED"

Scenario: SC-I02 Surface-level overlap discarded
  Given the user saved 3 articles that all mention "leadership" in passing
  But none of them make substantive arguments about leadership
  When the synthesis engine evaluates this cluster
  Then it determines has_genuine_connection = false
  And does not surface this as an insight
  And no synthesis entry is created

Scenario: SC-I03 Contradiction detected
  Given the user saved an HBR article arguing remote work improves productivity
  And saved a WSJ article citing studies showing remote work reduces productivity
  When the synthesis engine processes these
  Then it flags a contradiction on the topic of remote work and productivity
  And notes: "Key difference — they define 'productivity' differently"
  And includes both positions in the weekly synthesis

Scenario: SC-I04 Synthesis cites sources accurately
  Given the synthesis engine generates an insight about distributed systems
  When the insight is surfaced in the weekly synthesis
  Then every claim references specific artifact titles and dates
  And no information is fabricated or hallucinated
```

### Pre-Meeting Briefs

```gherkin
Scenario: SC-I05 Pre-meeting brief with full context
  Given the user has a meeting with David Kim in 30 minutes
  And the system has 3 email threads with David about acquisition strategy
  And the user promised David "the pricing analysis" 5 days ago
  When the pre-meeting alert fires
  Then the user receives: "Meeting with David in 30 min. Last discussed acquisition strategy. You owe him the pricing analysis (5 days overdue)."
  And the brief is delivered via the configured alert channel

Scenario: SC-I06 Pre-meeting brief with new contact
  Given the user has a meeting with "Jane Smith" in 30 minutes
  And Jane Smith is not in the People registry
  When the pre-meeting alert fires
  Then the user receives: "Meeting with Jane Smith in 30 min. No prior context — new contact."

Scenario: SC-I07 Pre-meeting brief with shared topic context
  Given the user has a meeting with Sarah in 30 minutes
  And Sarah recommended the book "Never Split the Difference" 2 months ago
  And both the user and Sarah have artifacts about negotiation
  When the pre-meeting alert fires
  Then the brief mentions the shared interest in negotiation
  And references Sarah's book recommendation

Scenario: SC-I08 No duplicate pre-meeting alerts
  Given the user has a 2 PM meeting with David
  And the system sent a pre-meeting brief at 1:30 PM
  When 2:00 PM arrives
  Then no second alert is sent for the same meeting
```

### Contextual Alerts

```gherkin
Scenario: SC-I09 Bill reminder 3 days before due
  Given the system detected an electric bill for $142 due April 15
  When April 12 arrives
  Then the system sends: "Electric bill ($142) due in 3 days"
  And the alert count for today does not exceed 2

Scenario: SC-I10 Commitment overdue alert
  Given the user promised Sarah the pricing article 5 days ago
  And no follow-up email has been detected
  When the commitment is 3+ days overdue
  Then the system sends: "You told Sarah you'd send the pricing article 5 days ago. Still open."

Scenario: SC-I11 Return window closing
  Given the system detected a headphone purchase with a 30-day return policy
  When the return window closes in 4 days
  Then the system sends: "Return window for the headphones closes in 4 days"

Scenario: SC-I12 Alert batching
  Given there are 3 contextual alerts pending for today
  When the system processes alerts
  Then it batches them into 2 or fewer deliveries
  And all 3 items are included in the batch

Scenario: SC-I13 Alert dismissal
  Given the user receives a bill reminder
  When the user dismisses or snoozes the alert
  Then the alert is not re-sent (dismissed) or re-sent after snooze period
  And the dismissal is logged
```

### Commitment Tracking

```gherkin
Scenario: SC-I14 User-made promise detection
  Given the user sends an email containing "I'll send you the report by Friday"
  When the system processes this outgoing email
  Then it creates an action item: type=user-promise, text="send report", person="recipient", deadline="Friday"
  And the action item appears in the next daily digest

Scenario: SC-I15 Contact-made promise detection
  Given a colleague emails "I'll have the budget numbers to you by end of week"
  When the system processes this incoming email
  Then it creates an action item: type=contact-promise, text="budget numbers", person="colleague", deadline="end of week"
  And tracks it until a follow-up is detected

Scenario: SC-I16 Auto-resolve prompt on follow-up
  Given the user promised to send a report by Friday
  And on Thursday, the user sends an email with an attachment in the same thread
  When the system processes this follow-up email
  Then it detects the possible resolution
  And either auto-resolves the commitment or prompts: "Did you send the report to Sarah? Mark as done?"

Scenario: SC-I17 Multiple overdue commitments pattern
  Given the user has 3 overdue commitments in the same week
  When the pattern recognition engine runs
  Then the weekly synthesis includes: "You've promised to send something to 3 different people this week and haven't followed through. Pattern?"
```

### Weekly Synthesis

```gherkin
Scenario: SC-I18 Full weekly synthesis
  Given the system processed 47 artifacts during the week
  And discovered 1 genuine cross-domain connection
  And detected system design topic acceleration (8 captures, +200% vs last month)
  And the user has 2 open commitments
  And a random archived item from October exists
  When the weekly synthesis fires on Sunday at 4 PM
  Then it generates a synthesis under 250 words with all 6 sections
  And the CONNECTION DISCOVERED section describes the through-line with source citations
  And the TOPIC MOMENTUM section shows rising/steady/declining topics
  And the OPEN LOOPS section lists the 2 overdue commitments
  And FROM THE ARCHIVE resurfaces the October item
  And PATTERNS NOTICED includes one behavioral observation

Scenario: SC-I19 Quiet week synthesis
  Given the system processed only 5 artifacts this week
  And no cross-domain connections were found
  And no topic momentum changes occurred
  When the weekly synthesis fires
  Then sections with nothing to report are skipped
  And the synthesis is shorter but still includes THIS WEEK stats and FROM THE ARCHIVE

Scenario: SC-I20 Serendipity resurface
  Given the topic "Alan Kay quotes" has been archived for 8 months
  And it contains an artifact: "The best way to predict the future is to invent it — save for team offsite intro"
  And the user has a "Team Offsite" calendar event next week
  When the weekly synthesis runs
  Then FROM THE ARCHIVE resurfaces: "Remember this? Oct 15: 'The best way to predict the future is to invent it.' Your offsite is next week."
```

### Enhanced Daily Digest

```gherkin
Scenario: SC-I21 Digest with commitment-tracked action items
  Given 2 commitments are overdue (pricing report to Sarah, budget review)
  And 3 emails were processed overnight with 1 needing attention
  And the topic "distributed systems" went hot this week
  And the user has a 2 PM meeting with David
  When the morning digest fires
  Then TOP ACTIONS lists the 2 overdue commitments with days-overdue context
  And OVERNIGHT mentions the 1 email needing attention (from David's proposal)
  And HOT TOPIC notes distributed systems acceleration
  And TODAY previews the meeting with David: "2 PM — David Kim (last discussed: acquisition strategy, you owe: pricing analysis)"

Scenario: SC-I22 Digest stays under 150 words with intelligence data
  Given there are 5 action items, 3 hot topics, and 4 calendar events
  When the digest is generated
  Then it prioritizes the top 2 action items and 1 hot topic mention
  And stays under 150 words
  And links to full views for "more details"
```

---

## Acceptance Criteria

| ID | Criterion | Maps to Scenario | Test Type |
|----|-----------|------------------|-----------|
| AC-I01 | Cross-domain connection detected from 3 different-source artifacts | SC-I01 | E2E |
| AC-I02 | Surface-level overlaps are not surfaced as insights | SC-I02 | Integration |
| AC-I03 | Contradicting artifacts flagged with both positions | SC-I03 | Integration |
| AC-I04 | Synthesis cites source artifacts by title and date; no fabrication | SC-I04 | Integration |
| AC-I05 | Pre-meeting brief delivered 30 min before event with full context | SC-I05 | E2E |
| AC-I06 | New contact gets "no prior context" brief | SC-I06 | Unit |
| AC-I07 | Shared topic context included in pre-meeting brief | SC-I07 | Integration |
| AC-I08 | No duplicate pre-meeting alerts for same event | SC-I08 | Unit |
| AC-I09 | Bill reminder sent 3 days before due date | SC-I09 | Integration |
| AC-I10 | Overdue commitment alert after 3+ days | SC-I10 | Integration |
| AC-I11 | Return window alert sent 4 days before closing | SC-I11 | Integration |
| AC-I12 | Alerts batched to max 2 per day | SC-I12 | Unit |
| AC-I13 | Dismissed/snoozed alerts not re-sent inappropriately | SC-I13 | Integration |
| AC-I14 | User-made promises detected from email text | SC-I14 | Integration |
| AC-I15 | Contact-made promises detected and tracked | SC-I15 | Integration |
| AC-I16 | Follow-up email triggers auto-resolve prompt | SC-I16 | Integration |
| AC-I17 | Multiple overdue commitments flagged as pattern | SC-I17 | Integration |
| AC-I18 | Weekly synthesis generated with all 6 sections under 250 words | SC-I18 | E2E |
| AC-I19 | Quiet week synthesis gracefully skips empty sections | SC-I19 | Integration |
| AC-I20 | Serendipity resurface matches upcoming calendar events | SC-I20 | Integration |
| AC-I21 | Daily digest includes commitment-tracked items and meeting previews | SC-I21 | Integration |
| AC-I22 | Enhanced digest stays under 150 words despite more data | SC-I22 | Unit |

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Read weekly synthesis | Power User | Web UI → Weekly / Telegram | 1. Open synthesis | All sections present, under 250 words, cited sources | Weekly synthesis view |
| View synthesis connections | Solo User | Weekly synthesis → click connection | 1. Click through-line link | Source artifacts shown with highlights | Connection detail |
| View commitments | Solo User | Web UI → Actions | 1. View open commitments | List of open promises/deadlines with status and linked person | Actions view |
| Resolve commitment | Solo User | Actions → commitment | 1. Mark as resolved | Commitment closed, removed from future digests/alerts | Actions view |
| Dismiss alert | Solo User | Alert notification | 1. Dismiss or snooze | Alert won't re-send (or re-sends after snooze period) | Alert |
| View pre-meeting brief | Solo User | Alert → meeting brief | 1. Open brief | Context, email threads, shared topics, pending commitments | Brief view |
| View contradictions | Power User | Weekly synthesis → contradiction | 1. Click contradiction link | Both artifacts shown with opposing claims highlighted | Contradiction view |

---

## Non-Functional Requirements

| Requirement | Target | Rationale |
|-------------|--------|-----------|
| Synthesis engine runtime | < 5 min for daily run (20 cluster evaluations) | Must complete before morning digest |
| Pre-meeting brief delivery | Exactly 30 min before event (±2 min) | Must arrive before the meeting, not after |
| Alert delivery latency | < 1 min from trigger to delivery | Contextual alerts must feel real-time |
| Weekly synthesis generation | < 2 min | Must complete before delivery time |
| Commitment detection precision | > 80% (avoid false positives on casual language) | "I'll think about it" is NOT a commitment |
| Alert rate | Maximum 2 per day, 3 system prompts per week | Invisible by default — never spam |
| Synthesis quality | Genuine connections only — no surface-level overlaps | User trust depends on quality over quantity |
| LLM cost per synthesis run | < $0.50 per daily run (cloud LLM) | Sustainable for individual use |
