# Smackerel — Product Overview (curated self-knowledge)

<!--
  Spec 104 SCOPE-05 — the ONLY partly-curated self-knowledge facet.
  This file is the single, bounded, reviewed source for smackerel's
  product-overview capability entries. It is embedded into the core binary
  (//go:embed in docsource.go) because the runtime image ships only the
  binary, not docs/. Keep it minimal + factual; it is grounded in
  docs/smackerel.md §1 (Vision) and MUST stay honest to the real product.

  Each "## <anchor>" H2 below is a declared section in curatedDocSections
  (docsource.go). Renaming/removing a heading without updating that list
  fails loud at ingest time — no silent drift.
-->

## What Smackerel Is

Smackerel is a passive intelligence layer across your entire digital life — a
personal "second brain". Instead of asking you to file and tag things at the
moment you are busiest, it observes what you encounter, captures anything you
explicitly flag with near-zero friction, processes every input (summary,
entities, key ideas, action items, transcripts), connects everything into a
living knowledge graph, and lets you find it later by meaning rather than by
remembering where you put it. It runs locally and self-hosted — you own your
data.

## What You Can Do With It

- Capture anything in under a few seconds from any device or channel (share,
  paste, or voice) — no folder or tag decision required.
- Ask vague, natural-language questions like "that pricing video" or "what did
  Sarah recommend" and get the right result by semantic search, not keywords.
- Let it observe passive sources (email, videos, maps, calendar, browsing,
  notes, purchases) and process each artifact into a summary with tags and
  cross-links.
- Read a short daily digest (the "smackerel") of what mattered while you were
  away — small, frequent, and actionable, never a guilt-inducing backlog.
- Receive rare, genuinely-useful surfaced prompts (pre-meeting briefs, trip
  prep, reminders) — the system asks only when it cannot figure something out.

## How It Works

You live your life; the system watches, absorbs, processes, and connects. Inputs
flow through an ingestion layer (passive sources + active capture) into a
processing pipeline that summarizes and extracts structure, then into a
PostgreSQL + pgvector knowledge graph. Retrieval is semantic: queries are
embedded and matched by vector similarity, so vague questions still land on the
right artifact. Knowledge has a lifecycle — hot topics are promoted, cold ones
decay — and a synthesis layer finds patterns and surfaces the right thing at the
right time. You interact by flagging things, asking questions, reading the daily
digest, and answering the occasional prompt.
