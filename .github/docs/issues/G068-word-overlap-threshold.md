# G068: stg_significant_words Exclusion List and 3-Word Overlap Threshold Prevent Legitimate DoD Matches

**Gate:** G068 (DoD-Gherkin Content Fidelity)  
**Component:** `bubbles/scripts/state-transition-guard.sh` (Check 22) and `bubbles/scripts/traceability-guard.sh` (Pass 2)  
**Severity:** Medium — causes false-positive gate failures on legitimate DoD items  
**Filed:** 2026-03-31

<!-- GENERATED:CAPABILITY_LEDGER_STATUS_START -->
**Ledger Status:** proposed
**Related Capability:** G068 DoD-Gherkin fidelity threshold tuning
**Competitive Pressure:** cline, roo-code
**Source Of Truth:** [Issue Status](../generated/issue-status.md)
<!-- GENERATED:CAPABILITY_LEDGER_STATUS_END -->

## Problem

The `stg_significant_words()` function strips words shorter than 4 characters AND excludes a hardcoded list of common words (`given`, `when`, `then`, `user`, `system`, `should`, `must`, `have`, etc.). The `stg_scenario_matches_dod()` function then requires a minimum overlap of **3 significant words** (when word_count > 3) between a Gherkin scenario title and a DoD item.

This combination causes legitimate DoD items to fail matching even when they contain the **exact scenario title**, because:

1. **Short but meaningful domain words are stripped.** Words like "API", "UI", "DB", "key", "tag", "set", "map", "run", "log" are < 4 chars and get removed.

2. **Common but scenario-defining words are excluded.** Words like "user", "system", "should", "must" appear in the exclusion list but are often the distinguishing words in scenario titles. A scenario like "User should see error when system is unavailable" has most of its significant words stripped.

3. **The 3-word threshold is too aggressive for short scenarios.** A scenario like "Catalog search returns filtered results" has only 3 significant words after filtering (`catalog`, `search`, `returns`, `filtered`, `results` — but `returns` is borderline). If the DoD item rephrases even slightly ("Search catalog is filtered correctly"), only 2 words survive matching → false failure.

## Example

```
Scenario: User validates catalog entry with missing fields
DoD item: - [ ] User validates catalog entry with missing fields → Evidence: [report.md#unit-validation]
```

After `stg_significant_words()`:
- Scenario words: `validates`, `catalog`, `entry`, `missing`, `fields` (5 words — "user" excluded, "with" < 4 chars)
- DoD item words: `validates`, `catalog`, `entry`, `missing`, `fields`, `evidence`, `report` 

This particular case would match (5 overlap ≥ 3 threshold). But consider:

```
Scenario: User can set API key for data source
```

After filtering: `data`, `source` (2 words — "user", "can", "set", "API", "key", "for" all stripped)
Threshold for 2 words = 2, so a DoD item needs BOTH words. Any rephrasing fails.

## Proposed Fix

1. **Lower the minimum word length from 4 to 3** — this preserves "API", "key", "tag", "set", "map", "run", "log" as significant words.

2. **Reduce the exclusion list** — remove domain-relevant words that appear in the current list:
   - Keep excluding: `given`, `when`, `then`, `with`, `from`, `into`, `onto`, `that`, `this`, `those`, `these`, `were`, `will`, `after`, `before`, `while`, `where`, `their`, `there`, `about`, `only`, `each`
   - **Stop excluding:** `user`, `users`, `system`, `should`, `must`, `have`, `has`, `been` — these are frequently the distinguishing words in Gherkin scenarios

3. **Consider a percentage-based threshold** instead of a fixed count — e.g., "at least 50% of significant words must appear in a DoD item" rather than a fixed 2-or-3 count. This scales better for both short and long scenarios.

## Affected Files

- `bubbles/scripts/state-transition-guard.sh` lines 1944-1990 (`stg_significant_words`, `stg_scenario_matches_dod`)
- `bubbles/scripts/traceability-guard.sh` lines 447-500 (equivalent logic)

## Workaround

Until fixed, agents can ensure DoD items use the **exact scenario title** verbatim rather than paraphrasing. But this defeats the purpose of G068's fuzzy matching, which is supposed to detect *rewrites*, not penalize *faithful copies*.
